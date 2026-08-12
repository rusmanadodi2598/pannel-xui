// Package serversvc also hosts the buy-flow inbound listing (FR-03, M7 fix).
//
// @file      internal/service/server/inbound.go
// @for       Expose real panel inbounds (server + protocol) to the buy flow.
// @uses      context, fmt, strings, internal/repository/xui
// @reason    The order flow must render actual panel inbounds (vless reality,
// vless ws, vmess, trojan, shadowsocks, grpc, ...) — never a hardcoded list.
//
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package serversvc

import (
	"context"
	"fmt"
	"strings"

	"github.com/kentangtech/bot-order/internal/repository/xui"
)

// inboundLister is the panel surface the buy/provision/delete/renew paths need
// (xui.Client implements it): list inbounds for the picker, add clients on the
// exact inbound the user chose, update a client's raw spec (renewal — preserves
// every field) and delete clients (FR-08 AC-4).
type inboundLister interface {
	GetInbounds(ctx context.Context) ([]xui.Inbound, error)
	AddClient(ctx context.Context, p xui.AddClientPayload) error
	UpdateClientRaw(ctx context.Context, p xui.UpdateClientRawPayload) error
	DeleteClient(ctx context.Context, inboundID int, clientID string) error
}

// InboundOption is one enabled inbound of a panel, shown in the buy flow
// (FR-03 step: pick server + protocol before picking a plan).
type InboundOption struct {
	ServerID   int64
	ServerName string
	Country    string
	InboundID  int
	Protocol   string
	Remark     string
	Port       int
}

// ListInbounds fetches the enabled inbounds of one panel (FR-03). The user
// picks server + protocol from real panel state — never a hardcoded list.
func (s *Service) ListInbounds(ctx context.Context, serverID int64) ([]InboundOption, error) {
	client, err := s.panelFactory(ctx, serverID)
	if err != nil {
		return nil, err
	}
	inbounds, err := client.GetInbounds(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing panel inbounds: %w", err)
	}
	srv, err := s.store.GetByID(ctx, serverID)
	if err != nil {
		return nil, err
	}
	out := make([]InboundOption, 0, len(inbounds))
	for _, in := range inbounds {
		if !in.Enable || in.Port <= 0 {
			continue
		}
		out = append(out, InboundOption{
			ServerID:   serverID,
			ServerName: srv.Name,
			Country:    srv.CountryCode,
			InboundID:  in.ID,
			Protocol:   strings.ToLower(in.Protocol),
			Remark:     in.Remark,
			Port:       in.Port,
		})
	}
	return out, nil
}
