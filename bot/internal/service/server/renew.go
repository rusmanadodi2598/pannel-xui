// Package serversvc also hosts the panel renewal gateway (FR-05, v1.38 fix).
//
// @file      internal/service/server/renew.go
// @for       RenewClient: extend a client's expiry on the panel preserving its
// full spec verbatim (credential, quota, ipLimit, flow, reverse/xhttp, ...).
// @uses      context, encoding/json, fmt, time, internal/repository/xui
// @reason    x-ui's updateClient REPLACES the client object wholesale: a bare
// {email, enable, expiryTime} spec fails validation ("empty client ID") and, if
// it passed, would silently wipe quota/ipLimit/flow (staging E2E v1.37 found
// both). Renewal patches the client's raw settings JSON in place so NO field —
// including ones not modelled by ClientSpec (vless `reverse`/xhttp) — is lost.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-12
package serversvc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/xui"
)

// RenewClient extends an existing client's expiry on the panel (FR-05).
// clientID is the per-protocol panel key (domain.PanelClientKey: vless/vmess→id,
// trojan→password, shadowsocks→email, hysteria→auth). The client is located by
// email inside the inbound settings JSON and its raw spec is re-sent with only
// enable + expiryTime changed — x-ui's updateClient replaces the whole client
// object, so a partial spec would wipe the quota/ipLimit/flow (v1.38 fix).
func (s *Service) RenewClient(ctx context.Context, serverID int64, clientID, email, protocol string, newExpiry time.Time) error {
	client, err := s.panelFactory(ctx, serverID)
	if err != nil {
		return err
	}
	inbounds, err := client.GetInbounds(ctx)
	if err != nil {
		return fmt.Errorf("listing panel inbounds: %w", err)
	}
	inbound, raw, ok := findClientInbound(inbounds, email)
	if !ok {
		return fmt.Errorf("client %s not found on server %d", email, serverID)
	}
	patched, err := patchClientSpec(raw, newExpiry)
	if err != nil {
		return fmt.Errorf("patching client %s spec: %w", email, err)
	}
	return client.UpdateClientRaw(ctx, xui.UpdateClientRawPayload{
		InboundID: inbound.ID,
		ClientID:  clientID,
		Client:    patched,
	})
}

// findClientInbound locates the inbound whose settings JSON contains the client
// email and returns the inbound plus the client's raw spec (the panel is the
// source of truth for renewal merges).
func findClientInbound(inbounds []xui.Inbound, email string) (xui.Inbound, json.RawMessage, bool) {
	for _, in := range inbounds {
		raw, ok := clientSpecInInbound(in, email)
		if ok {
			return in, raw, true
		}
	}
	return xui.Inbound{}, nil, false
}

// clientSpecInInbound parses an inbound's settings JSON ({"clients":[...]}) and
// returns the raw client spec with the given email, if present.
func clientSpecInInbound(in xui.Inbound, email string) (json.RawMessage, bool) {
	var payload struct {
		Clients []json.RawMessage `json:"clients"`
	}
	if err := json.Unmarshal([]byte(in.Settings), &payload); err != nil {
		return nil, false
	}
	for _, raw := range payload.Clients {
		var probe struct {
			Email string `json:"email"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil {
			continue
		}
		if probe.Email == email {
			return raw, true
		}
	}
	return nil, false
}

// patchClientSpec mutates the client's raw JSON in place: enable → true,
// expiryTime → the new expiry. Every other field (credential, totalGB,
// limitIp, flow, subId, reset, reverse/xhttp, ...) is preserved verbatim.
func patchClientSpec(raw json.RawMessage, newExpiry time.Time) (json.RawMessage, error) {
	var spec map[string]interface{}
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil, err
	}
	spec["enable"] = true
	spec["expiryTime"] = newExpiry.UnixMilli()
	return json.Marshal(spec)
}
