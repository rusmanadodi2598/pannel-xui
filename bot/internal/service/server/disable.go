// Package serversvc also hosts client disabling (PRD trial-cleanup worker).
//
// @file      internal/service/server/disable.go
// @for       DisableClients: flip enable=false on the panel for expired trial clients.
// @uses      context, encoding/json, fmt, strings, internal/repository/xui
// @reason    x-ui's updateClient REPLACES the whole client object, so the raw
// spec is patched in place (only enable changes) — quota/ipLimit/flow and
// unmapped fields survive, same contract as RenewClient (v1.38). Clients
// already missing on the panel count as disabled (nothing to disable).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-12
package serversvc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kentangtech/bot-order/internal/repository/xui"
)

// DisableClients disables several clients by email on one panel with a single
// GetInbounds call (anti N+1 §1.7 — the trial-cleanup sweep processes up to
// batch clients). The panel is the source of truth, so a panel error returns
// the affected emails — the caller then skips marking those DB rows.
// Returns the emails that could NOT be disabled (panel error), or nil when
// every client was disabled (or already gone from the panel).
func (s *Service) DisableClients(ctx context.Context, serverID int64, emails []string) ([]string, error) {
	if len(emails) == 0 {
		return nil, nil
	}
	client, err := s.panelFactory(ctx, serverID)
	if err != nil {
		return emails, err
	}
	inbounds, err := client.GetInbounds(ctx)
	if err != nil {
		return emails, fmt.Errorf("listing panel inbounds: %w", err)
	}
	var failed []string
	for _, email := range emails {
		inbound, raw, ok := findClientInbound(inbounds, email)
		if !ok {
			// Client already removed from the panel — nothing left to disable.
			continue
		}
		key, err := clientKeyFromSpec(raw, inbound.Protocol)
		if err != nil {
			failed = append(failed, email)
			continue
		}
		patched, err := patchClientEnable(raw, false)
		if err != nil {
			failed = append(failed, email)
			continue
		}
		if err := client.UpdateClientRaw(ctx, xui.UpdateClientRawPayload{
			InboundID: inbound.ID,
			ClientID:  key,
			Client:    patched,
		}); err != nil {
			failed = append(failed, email)
		}
	}
	if len(failed) > 0 {
		return failed, fmt.Errorf("disabling %d/%d client(s) on server %d", len(failed), len(emails), serverID)
	}
	return nil, nil
}

// clientKeyFromSpec extracts the per-protocol panel key from the client's raw
// spec — the credential x-ui's updateClient URL needs. Same mapping as
// domain.PanelClientKey (vless/vmess→id, trojan→password, shadowsocks→email,
// hysteria→auth), applied to the panel's own payload so no DB round-trip is
// needed.
func clientKeyFromSpec(raw json.RawMessage, protocol string) (string, error) {
	var spec struct {
		ID       string `json:"id"`
		Password string `json:"password"`
		Auth     string `json:"auth"`
		Email    string `json:"email"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		return "", err
	}
	switch strings.ToLower(protocol) {
	case "vless", "vmess":
		return spec.ID, nil
	case "shadowsocks":
		return spec.Email, nil
	case "hysteria", "hysteria2":
		return spec.Auth, nil
	default: // trojan and anything else keyed by password
		return spec.Password, nil
	}
}

// patchClientEnable mutates the client's raw JSON in place: only enable flips,
// every other field (credential, totalGB, limitIp, flow, subId, reset, ...)
// is preserved verbatim.
func patchClientEnable(raw json.RawMessage, enable bool) (json.RawMessage, error) {
	var spec map[string]interface{}
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil, err
	}
	spec["enable"] = enable
	return json.Marshal(spec)
}
