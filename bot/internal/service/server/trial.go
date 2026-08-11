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
func (s *Service) CreateTrialClient(ctx context.Context, serverID int64, email, protocol string, hours int, trafficGB, ipLimit int64) (domain.PanelClient, error) {
	expiry := time.Now().Add(time.Duration(hours) * time.Hour).UnixMilli()
	return s.provisionClient(ctx, serverID, email, protocol, trafficGB, ipLimit, expiry)
}

// provisionClient picks the server's enabled inbound matching the protocol and
// adds the client with the given absolute expiry (ms epoch). Returns the
// created credential — shared by purchase (days) and trial (hours).
func (s *Service) provisionClient(ctx context.Context, serverID int64, email, protocol string, trafficGB, ipLimit int64, expiryMs int64) (domain.PanelClient, error) {
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

	spec := xui.ClientSpec{
		Email:      email,
		LimitIP:    int(ipLimit),
		TotalGB:    trafficGB,
		ExpiryTime: expiryMs,
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
