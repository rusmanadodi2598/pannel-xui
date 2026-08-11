// Package serversvc manages X-UI panel instances (FR-10, M4).
//
// @file      internal/service/server/server.go
// @for       Seed encrypted panels, buyable list, country picker, panel gateway.
// @uses      context, fmt, internal/config, internal/repository/postgres, internal/repository/xui, internal/crypto, internal/domain
// @reason    Multi-panel support: credentials encrypted at rest; services never
// touch panel secrets directly (PRD §15.1, FR-10).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package serversvc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kentangtech/bot-order/internal/config"
	"github.com/kentangtech/bot-order/internal/crypto"
	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	"github.com/kentangtech/bot-order/internal/repository/xui"
)

// Store is the server persistence seam (postgres.ServerRepo implements it).
type Store interface {
	UpsertSeed(ctx context.Context, s postgres.VPNServer) error
	ListBuyable(ctx context.Context) ([]postgres.ServerView, error)
	GetByID(ctx context.Context, id int64) (*postgres.VPNServer, error)
}

// SessionCache is the xui.SessionCache seam (Redis adapter in repository/redis).
type SessionCache interface {
	Get(ctx context.Context, serverID int64) (string, error)
	Set(ctx context.Context, serverID int64, cookie string, ttl time.Duration) error
	Del(ctx context.Context, serverID int64) error
}

// Service seeds panels and exposes the panel gateway for order fulfillment.
type Service struct {
	store Store
	box   *crypto.SecretBox
	cache SessionCache
}

// New builds the server service. cache may be nil (no session persistence).
func New(store Store, box *crypto.SecretBox, cache SessionCache) *Service {
	return &Service{store: store, box: box, cache: cache}
}

// EnsureSeeded upserts every PANEL_N_* seed with its password encrypted
// (AES-256-GCM) — idempotent, safe to run at every boot.
func (s *Service) EnsureSeeded(ctx context.Context, seeds []config.ServerSeed) error {
	for _, seed := range seeds {
		enc, err := s.box.Encrypt(seed.Password)
		if err != nil {
			return fmt.Errorf("encrypting panel %s password: %w", seed.Name, err)
		}
		protocols, _ := json.Marshal(seed.Protocols)
		row := postgres.VPNServer{
			Name:        seed.Name,
			Host:        seed.Host,
			Port:        seed.Port,
			Username:    seed.Username,
			PasswordEnc: enc,
			APIPath:     seed.APIPath,
			UseSSL:      seed.UseSSL,
			InsecureTLS: seed.InsecureTLS,
			CountryCode: seed.CountryCode,
			FlagEmoji:   seed.FlagEmoji,
			Location:    seed.Location,
			Priority:    seed.Priority,
			IsActive:    true,
			IsOpen:      true,
			Protocols:   string(protocols),
			UpdatedAt:   time.Now(),
		}
		if err := s.store.UpsertSeed(ctx, row); err != nil {
			return fmt.Errorf("seeding panel %s: %w", seed.Name, err)
		}
	}
	return nil
}

// ListBuyable returns panels shown in the buy menu (active & open).
func (s *Service) ListBuyable(ctx context.Context) ([]postgres.ServerView, error) {
	return s.store.ListBuyable(ctx)
}

// PickForCountry returns the first buyable server id for a country, or
// ErrNoServer when none is available (FR-03 step 1).
func (s *Service) PickForCountry(ctx context.Context, country string) (int64, error) {
	servers, err := s.store.ListBuyable(ctx)
	if err != nil {
		return 0, err
	}
	for _, sv := range servers {
		if strings.EqualFold(sv.CountryCode, country) {
			return sv.ID, nil
		}
	}
	return 0, ErrNoServer{Country: country}
}

// ErrNoServer reports that no open server exists for a country.
type ErrNoServer struct{ Country string }

func (e ErrNoServer) Error() string { return "no open server for country " + e.Country }

// PanelClient builds an authenticated X-UI client for one server, decrypting
// its stored password (session cookie cached in Redis when cache is set).
func (s *Service) PanelClient(ctx context.Context, serverID int64) (*xui.Client, error) {
	srv, err := s.store.GetByID(ctx, serverID)
	if err != nil {
		return nil, err
	}
	password, err := s.box.Decrypt(srv.PasswordEnc)
	if err != nil {
		return nil, fmt.Errorf("decrypting panel %s credentials: %w", srv.Name, err)
	}
	scheme := "http"
	if srv.UseSSL {
		scheme = "https"
	}
	cfg := xui.ServerConfig{
		BaseURL:    fmt.Sprintf("%s://%s:%d", scheme, srv.Host, srv.Port),
		APIPath:    srv.APIPath,
		Username:   srv.Username,
		Password:   password,
		Insecure:   srv.InsecureTLS, // opt-in per panel; default secure (review fix)
		Timeout:    30 * time.Second,
		ServerID:   srv.ID,
		SessionTTL: 12 * time.Hour,
	}
	return xui.NewClient(cfg, s.cache), nil
}

// CreateClient provisions a client on the panel for a purchase (FR-04).
// It picks the server's enabled inbound matching the protocol and adds the
// client with the computed expiry. Returns the created credential.
func (s *Service) CreateClient(ctx context.Context, serverID int64, email, protocol string, days int, trafficGB, ipLimit int64) (domain.PanelClient, error) {
	client, err := s.PanelClient(ctx, serverID)
	if err != nil {
		return domain.PanelClient{}, err
	}
	inbounds, err := client.GetInbounds(ctx)
	if err != nil {
		return domain.PanelClient{}, fmt.Errorf("listing panel inbounds: %w", err)
	}
	inbound, ok := matchInbound(inbounds, protocol)
	if !ok {
		return domain.PanelClient{}, fmt.Errorf("no enabled %s inbound on server %d", protocol, serverID)
	}

	expiry := time.Now().AddDate(0, 0, days).UnixMilli()
	spec := xui.ClientSpec{
		Email:      email,
		LimitIP:    int(ipLimit),
		TotalGB:    trafficGB,
		ExpiryTime: expiry,
		Enable:     true,
	}
	switch protocol {
	case "vless", "vmess":
		spec.ID = domain.NewUUID()
	case "trojan", "shadowsocks":
		spec.Password = domain.NewSecret(16)
	}

	if err := client.AddClient(ctx, xui.AddClientPayload{InboundID: inbound.ID, Client: spec}); err != nil {
		return domain.PanelClient{}, fmt.Errorf("panel addClient: %w", err)
	}
	return domain.PanelClient{
		InboundID: inbound.ID,
		Email:     email,
		UUID:      spec.ID,
		Password:  spec.Password,
		Protocol:  protocol,
	}, nil
}

// RenewClient extends an existing client's expiry on the panel (FR-05).
// The client's protocol (stored on the DB row) selects the target inbound.
func (s *Service) RenewClient(ctx context.Context, serverID int64, clientID, email, protocol string, newExpiry time.Time) error {
	client, err := s.PanelClient(ctx, serverID)
	if err != nil {
		return err
	}
	inbounds, err := client.GetInbounds(ctx)
	if err != nil {
		return fmt.Errorf("listing panel inbounds: %w", err)
	}
	inbound, ok := matchInbound(inbounds, protocol)
	if !ok {
		for i := range inbounds {
			if inbounds[i].Enable && inbounds[i].Port > 0 {
				inbound = inbounds[i]
				ok = true
				break
			}
		}
	}
	if !ok {
		return fmt.Errorf("no enabled inbound on server %d", serverID)
	}
	spec := xui.ClientSpec{Email: email, Enable: true, ExpiryTime: newExpiry.UnixMilli()}
	return client.UpdateClient(ctx, xui.UpdateClientPayload{InboundID: inbound.ID, ClientID: clientID, Client: spec})
}

// matchInbound finds the first enabled inbound with the given protocol.
func matchInbound(inbounds []xui.Inbound, protocol string) (xui.Inbound, bool) {
	for _, in := range inbounds {
		if in.Enable && in.Port > 0 && strings.EqualFold(in.Protocol, protocol) {
			return in, true
		}
	}
	return xui.Inbound{}, false
}
