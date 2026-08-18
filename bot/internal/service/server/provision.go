// Package serversvc also hosts the two-phase client provisioning (FR-04, v1.47).
//
// @file      internal/service/server/provision.go
// @for       FR-04: prepare (read-only) + commit (addClient) split so the order
// flow can persist the row and debit BEFORE the panel mutation.
// @uses      context, fmt, time, internal/domain, internal/repository/xui
// @reason    Split from trial.go/server.go for the §1.1 line limit; the
// prepare/commit split removes the "panel client without DB row" orphan — money
// and the DB row move before the panel mutation, and a panel failure only needs
// a refund + row delete (debit-first, parity renewal v1.37).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-17
package serversvc

import (
	"context"
	"fmt"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/xui"
)

// PrepareClient prepares a client for a purchase WITHOUT mutating the panel:
// it lists inbounds, resolves the target inbound, generates credentials and
// builds the share link. The order flow persists the returned record, debits,
// then calls CommitClient — a panel failure then only needs refund + row delete.
func (s *Service) PrepareClient(ctx context.Context, serverID int64, inboundID int, email, protocol string, days int, trafficGB, ipLimit int64) (domain.PreparedClient, error) {
	return s.prepareClient(ctx, serverID, inboundID, email, protocol, trafficGB, ipLimit, time.Now().AddDate(0, 0, days).UnixMilli())
}

// CommitClient provisions a client prepared by PrepareClient on the panel
// (addClient). The spec is rebuilt from the prepared record so the panel sees
// exactly the credentials the bot persisted (same subId, expiry, quota).
func (s *Service) CommitClient(ctx context.Context, serverID int64, p domain.PreparedClient) error {
	client, err := s.panelFactory(ctx, serverID)
	if err != nil {
		return err
	}
	spec := xui.ClientSpec{
		Email:      p.Panel.Email,
		LimitIP:    int(p.IPLimit),
		TotalGB:    p.TrafficGB,
		ExpiryTime: p.ExpiryMs,
		Enable:     true,
		SubID:      p.Panel.SubID,
	}
	switch p.Panel.Protocol {
	case "vless", "vmess":
		spec.ID = p.Panel.UUID
	case "hysteria", "hysteria2":
		spec.Auth = p.Panel.Password // panel's Hysteria uses the auth field
	default: // trojan, shadowsocks, unknown
		spec.Password = p.Panel.Password
	}
	if err := client.AddClient(ctx, xui.AddClientPayload{InboundID: p.Panel.InboundID, Client: spec}); err != nil {
		return fmt.Errorf("panel addClient: %w", err)
	}
	return nil
}

// prepareClient is the shared prepare step (purchase days + trial hours): it
// resolves the inbound and generates credentials + share link WITHOUT mutating
// the panel (read-only GetInbounds). Returns the bot-side record plus the exact
// panel commit parameters so the commit reuses the same subId/expiry/quota.
func (s *Service) prepareClient(ctx context.Context, serverID int64, inboundID int, email, protocol string, trafficGB, ipLimit, expiryMs int64) (domain.PreparedClient, error) {
	client, err := s.panelFactory(ctx, serverID)
	if err != nil {
		return domain.PreparedClient{}, err
	}
	inbounds, err := client.GetInbounds(ctx)
	if err != nil {
		return domain.PreparedClient{}, fmt.Errorf("listing panel inbounds: %w", err)
	}
	inbound, ok := matchInboundByID(inbounds, inboundID)
	if !ok {
		inbound, ok = matchInbound(inbounds, protocol)
	}
	if !ok {
		return domain.PreparedClient{}, fmt.Errorf("no enabled %s inbound on server %d", protocol, serverID)
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

	inboundNetwork, inboundPath := InboundStream(inbound.StreamSettings)
	pc := domain.PanelClient{
		InboundID:      inbound.ID,
		Email:          email,
		UUID:           spec.ID,
		Password:       credential,
		Protocol:       protocol,
		InboundNetwork: inboundNetwork,
		InboundPath:    inboundPath,
		SubID:          spec.SubID, // FR-13: sub URL dibangun dari subId yang sama
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
	return domain.PreparedClient{
		Panel:     pc,
		ExpiryMs:  expiryMs,
		TrafficGB: trafficGB,
		IPLimit:   ipLimit,
	}, nil
}
