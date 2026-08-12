// Package serversvc test covers the batch client disable (AGENTS.md §2.1).
//
// @file      internal/service/server/disable_test.go
// @for       Unit tests: DisableClients key-per-protocol, enable=false patch, missing-client skip.
// @uses      testing, context, encoding/json, errors, github.com/kentangtech/bot-order/internal/repository/xui
// @reason    The trial-cleanup worker disables expired clients through this
// gateway; a wrong key or a lossy spec would fail x-ui validation (v1.37/38
// lesson) — the per-protocol key mapping is locked here.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-12
package serversvc

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kentangtech/bot-order/internal/repository/xui"
)

var errPanelDown = errors.New("panel down")

// settingsJSON wraps client fields into an inbound settings payload.
func settingsJSON(fields map[string]any) string {
	raw, _ := json.Marshal(map[string]any{"clients": []any{fields}})
	return string(raw)
}

func TestDisableClients_GivenVlessClient_ThenKeyIsIDAndEnablePatchedFalse(t *testing.T) {
	panel := &fakePanel{inbounds: []xui.Inbound{{
		ID: 5, Protocol: "vless",
		Settings: settingsJSON(map[string]any{"id": "uuid-1", "email": "t1@vpn.kt", "enable": true, "totalGB": 1}),
	}}}
	svc := &Service{}
	svc.panelFactory = func(context.Context, int64) (inboundLister, error) { return panel, nil }

	failed, err := svc.DisableClients(context.Background(), 1, []string{"t1@vpn.kt"})
	if err != nil {
		t.Fatalf("DisableClients: %v", err)
	}
	if len(failed) != 0 {
		t.Fatalf("failed = %v, want none", failed)
	}
	if panel.updated == nil {
		t.Fatal("updateClient was not called")
	}
	if panel.updated.InboundID != 5 || panel.updated.ClientID != "uuid-1" {
		t.Errorf("updateClient = (inbound %d, key %q), want (5, uuid-1)",
			panel.updated.InboundID, panel.updated.ClientID)
	}
	var spec map[string]any
	if err := json.Unmarshal(panel.updated.Client, &spec); err != nil {
		t.Fatalf("patched spec unmarshal: %v", err)
	}
	if spec["enable"] != false {
		t.Errorf("enable = %v, want false", spec["enable"])
	}
	if spec["id"] != "uuid-1" {
		t.Errorf("id = %v, want uuid-1 (credential preserved)", spec["id"])
	}
}

func TestDisableClients_GivenTrojanClient_ThenKeyIsPassword(t *testing.T) {
	panel := &fakePanel{inbounds: []xui.Inbound{{
		ID: 6, Protocol: "trojan",
		Settings: settingsJSON(map[string]any{"password": "pw-trojan", "email": "t2@vpn.kt", "enable": true}),
	}}}
	svc := &Service{}
	svc.panelFactory = func(context.Context, int64) (inboundLister, error) { return panel, nil }

	if _, err := svc.DisableClients(context.Background(), 1, []string{"t2@vpn.kt"}); err != nil {
		t.Fatalf("DisableClients: %v", err)
	}
	if panel.updated == nil || panel.updated.ClientID != "pw-trojan" {
		t.Errorf("key = %v, want pw-trojan (trojan → password)", panel.updated.ClientID)
	}
}

func TestDisableClients_GivenHysteriaClient_ThenKeyIsAuth(t *testing.T) {
	panel := &fakePanel{inbounds: []xui.Inbound{{
		ID: 7, Protocol: "hysteria",
		Settings: settingsJSON(map[string]any{"auth": "auth-hy", "email": "t3@vpn.kt", "enable": true}),
	}}}
	svc := &Service{}
	svc.panelFactory = func(context.Context, int64) (inboundLister, error) { return panel, nil }

	if _, err := svc.DisableClients(context.Background(), 1, []string{"t3@vpn.kt"}); err != nil {
		t.Fatalf("DisableClients: %v", err)
	}
	if panel.updated == nil || panel.updated.ClientID != "auth-hy" {
		t.Errorf("key = %v, want auth-hy (hysteria → auth)", panel.updated.ClientID)
	}
}

func TestDisableClients_GivenClientMissingOnPanel_ThenSkippedNoError(t *testing.T) {
	panel := &fakePanel{inbounds: []xui.Inbound{{
		ID: 5, Protocol: "vless",
		Settings: settingsJSON(map[string]any{"id": "uuid-1", "email": "t1@vpn.kt", "enable": true}),
	}}}
	svc := &Service{}
	svc.panelFactory = func(context.Context, int64) (inboundLister, error) { return panel, nil }

	failed, err := svc.DisableClients(context.Background(), 1, []string{"ghost@vpn.kt"})
	if err != nil {
		t.Fatalf("DisableClients: %v", err)
	}
	if len(failed) != 0 {
		t.Errorf("failed = %v, want none (missing client counts as disabled)", failed)
	}
	if panel.updated != nil {
		t.Error("updateClient must not be called for a missing client")
	}
}

func TestDisableClients_GivenPanelFailure_ThenAllEmailsFailed(t *testing.T) {
	svc := &Service{}
	svc.panelFactory = func(context.Context, int64) (inboundLister, error) {
		return errInboundLister{err: errPanelDown}, nil
	}

	failed, err := svc.DisableClients(context.Background(), 1, []string{"a@vpn.kt", "b@vpn.kt"})
	if err == nil {
		t.Fatal("DisableClients = nil, want error")
	}
	if len(failed) != 2 {
		t.Errorf("failed = %v, want both emails", failed)
	}
}

// errInboundLister is a panel surface whose GetInbounds always fails.
type errInboundLister struct{ err error }

func (e errInboundLister) GetInbounds(context.Context) ([]xui.Inbound, error) { return nil, e.err }
func (e errInboundLister) GetOnlineClients(context.Context) ([]xui.OnlineUser, error) {
	return nil, e.err
}
func (e errInboundLister) AddClient(context.Context, xui.AddClientPayload) error { return e.err }
func (e errInboundLister) UpdateClientRaw(context.Context, xui.UpdateClientRawPayload) error {
	return e.err
}
func (e errInboundLister) DeleteClient(context.Context, int, string) error { return e.err }

var _ inboundLister = errInboundLister{}
