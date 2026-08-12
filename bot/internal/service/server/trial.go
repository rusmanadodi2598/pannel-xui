// Package serversvc also hosts trial provisioning (FR-07).
//
// @file      internal/service/server/trial.go
// @for       FR-07 AC-2: create trial account (expiry in hours) + shared addClient helper.
// @uses      context, fmt, time, internal/domain, internal/repository/xui
// @reason    Trial uses the same addClient endpoint with an hour-based expiry
// (addTrialClient does not exist in this fork — PRD §3.2/§15.2); keeping the
// shared provisionClient helper avoids duplicating inbound matching & spec build.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-11
package serversvc

import (
	"context"
	"fmt"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/xui"
)

// CreateTrialClient provisions a short-lived trial account (FR-07 AC-2):
// expiry = now + hours, quota = trafficGB, limit = ipLimit (defaults 1 jam/1 GB/1 IP).
func (s *Service) CreateTrialClient(ctx context.Context, serverID int64, inboundID int, email, protocol string, hours int, trafficGB, ipLimit int64) (domain.PanelClient, error) {
	expiry := time.Now().Add(time.Duration(hours) * time.Hour).UnixMilli()
	return s.provisionClient(ctx, serverID, inboundID, email, protocol, trafficGB, ipLimit, expiry)
}

// provisionClient picks the server's inbound and adds the client with the
// given absolute expiry (ms epoch). inboundID > 0 pins the exact inbound the
// user chose in the buy flow; 0 falls back to the first enabled inbound
// matching the protocol. Returns the created credential — shared by purchase
// (days) and trial (hours).
func (s *Service) provisionClient(ctx context.Context, serverID int64, inboundID int, email, protocol string, trafficGB, ipLimit int64, expiryMs int64) (domain.PanelClient, error) {
	client, err := s.panelFactory(ctx, serverID)
	if err != nil {
		return domain.PanelClient{}, err
	}
	inbounds, err := client.GetInbounds(ctx)
	if err != nil {
		return domain.PanelClient{}, fmt.Errorf("listing panel inbounds: %w", err)
	}
	inbound, ok := matchInboundByID(inbounds, inboundID)
	if !ok {
		inbound, ok = matchInbound(inbounds, protocol)
	}
	if !ok {
		return domain.PanelClient{}, fmt.Errorf("no enabled %s inbound on server %d", protocol, serverID)
	}

	spec := xui.ClientSpec{
		Email:      email,
		LimitIP:    int(ipLimit),
		TotalGB:    trafficGB,
		ExpiryTime: expiryMs,
		Enable:     true,
		SubID:      domain.NewUUID(), // panel subscription id (sub server opt-in)
	}
	var credential string
	switch protocol {
	case "vless", "vmess":
		spec.ID = domain.NewUUID()
		credential = spec.ID
	case "trojan", "shadowsocks":
		spec.Password = domain.NewSecret(16)
		credential = spec.Password
	case "hysteria", "hysteria2":
		spec.Auth = domain.NewSecret(16) // panel's Hysteria uses the auth field
		credential = spec.Auth
	default:
		// Unfamiliar protocol: fall back to password so addClient never sends
		// an empty credential (panel returns a client even for odd protocols).
		spec.Password = domain.NewSecret(16)
		credential = spec.Password
	}

	if err := client.AddClient(ctx, xui.AddClientPayload{InboundID: inbound.ID, Client: spec}); err != nil {
		return domain.PanelClient{}, fmt.Errorf("panel addClient: %w", err)
	}
	inboundNetwork, inboundPath := InboundStream(inbound.StreamSettings)
	pc := domain.PanelClient{
		InboundID:      inbound.ID,
		Email:          email,
		UUID:           spec.ID,
		Password:       credential,
		Protocol:       protocol,
		InboundNetwork: inboundNetwork,
		InboundPath:    inboundPath,
	}
	// Build the share link from the inbound's settings + the server host.
	// The panel's /sub/ endpoint may be disabled, so the bot generates the
	// same link itself (M7 detail/export feature). Host lookup failure only
	// skips the link — the client was already provisioned successfully.
	if srv, err := s.store.GetByID(ctx, serverID); err == nil {
		pc.ConfigLink = ShareLink(protocol, inbound, srv.Host, ClientCred{
			UUID: spec.ID, Password: credential, Auth: spec.Auth, Flow: spec.Flow,
		})
	}
	return pc, nil
}
