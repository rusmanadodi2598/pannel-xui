// Package serversvc_test covers the renewal merge fix (FR-05, v1.38).
//
// @file      internal/service/server/renew_test.go
// @for       Unit tests: RenewClient must re-send the client's FULL raw panel
// spec (credential + quota + ipLimit + flow + reverse) and only bump
// enable/expiryTime.
// @uses      testing, context, encoding/json, github.com/kentangtech/bot-order/internal/crypto,
// github.com/kentangtech/bot-order/internal/repository/postgres, internal/repository/xui
// @reason    x-ui updateClient replaces the whole client object; the E2E found
// the old bare-spec renewal failed "empty client ID" (and would wipe quota if
// it passed) — regression tests lock the raw merge behavior.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-12
package serversvc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
	"github.com/kentangtech/bot-order/internal/repository/xui"
)

// settingsWithClient builds an inbound settings JSON carrying one raw client.
func settingsWithClient(client map[string]interface{}) string {
	raw, err := json.Marshal(map[string]interface{}{"clients": []interface{}{client}})
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func renewService(t *testing.T, panel *fakePanel) *Service {
	t.Helper()
	box := testBox(t)
	svc := New(&fakeServerStore{byID: &postgres.VPNServer{
		ID: 9, Name: "ID-01", CountryCode: "ID", Host: "h", Port: 1, Username: "u",
		PasswordEnc: mustEncrypt(t, box, "p"), APIPath: "/",
	}}, box, nil)
	svc.panelFactory = func(context.Context, int64) (inboundLister, error) { return panel, nil }
	return svc
}

// updatedClient decodes the raw spec the fake panel captured on updateClient.
func updatedClient(t *testing.T, panel *fakePanel) map[string]interface{} {
	t.Helper()
	if panel.updated == nil {
		t.Fatal("updateClient was not called")
	}
	var spec map[string]interface{}
	if err := json.Unmarshal(panel.updated.Client, &spec); err != nil {
		t.Fatalf("decoding updated client raw: %v", err)
	}
	return spec
}

func TestRenewClient_GivenVlessClient_ThenRawSpecPreservedAndExpiryBumped(t *testing.T) {
	panel := &fakePanel{inbounds: []xui.Inbound{{
		ID: 6, Protocol: "vless", Enable: true, Port: 20006,
		Settings: settingsWithClient(map[string]interface{}{
			"id": "uuid-1", "email": "a@vpn.kt", "totalGB": float64(100), "limitIp": 2,
			"flow": "xtls-rprx-vision", "subId": "sub-1", "reset": 30, "enable": true,
			"expiryTime": float64(time.Now().Add(10 * 24 * time.Hour).UnixMilli()),
			// vless reverse (xhttp) must survive the round-trip verbatim.
			"reverse": map[string]interface{}{"tag": "reverse-tag", "port": 443},
		}),
	}}}
	svc := renewService(t, panel)

	newExpiry := time.Now().Add(40 * 24 * time.Hour)
	if err := svc.RenewClient(context.Background(), 9, "uuid-1", "a@vpn.kt", "vless", newExpiry); err != nil {
		t.Fatalf("RenewClient: %v", err)
	}
	if panel.updated.InboundID != 6 {
		t.Errorf("inbound = %d, want 6", panel.updated.InboundID)
	}
	if panel.updated.ClientID != "uuid-1" {
		t.Errorf("client id = %q, want uuid-1", panel.updated.ClientID)
	}
	spec := updatedClient(t, panel)
	if spec["id"] != "uuid-1" {
		t.Errorf("spec.id = %v, want uuid-1 (credential preserved)", spec["id"])
	}
	if spec["totalGB"] != float64(100) || spec["limitIp"] != float64(2) || spec["flow"] != "xtls-rprx-vision" || spec["subId"] != "sub-1" || spec["reset"] != float64(30) {
		t.Errorf("quota/ipLimit/flow/subId/reset wiped: %+v", spec)
	}
	if _, ok := spec["reverse"].(map[string]interface{}); !ok {
		t.Errorf("vless reverse/xhttp field lost in round-trip: %+v", spec["reverse"])
	}
	if int64(spec["expiryTime"].(float64)) != newExpiry.UnixMilli() {
		t.Errorf("expiry = %v, want %d", spec["expiryTime"], newExpiry.UnixMilli())
	}
	if spec["enable"] != true {
		t.Error("enable must stay true on renewal")
	}
}

func TestRenewClient_GivenTrojanExpired_ThenReEnabledAndPasswordKept(t *testing.T) {
	panel := &fakePanel{inbounds: []xui.Inbound{{
		ID: 7, Protocol: "trojan", Enable: true, Port: 20007,
		Settings: settingsWithClient(map[string]interface{}{
			"password": "pw-1", "email": "t@vpn.kt", "totalGB": float64(50), "enable": false,
			"expiryTime": 1, // expired — renewal re-enables
		}),
	}}}
	svc := renewService(t, panel)

	if err := svc.RenewClient(context.Background(), 9, "pw-1", "t@vpn.kt", "trojan", time.Now().Add(30*24*time.Hour)); err != nil {
		t.Fatalf("RenewClient: %v", err)
	}
	spec := updatedClient(t, panel)
	if spec["password"] != "pw-1" {
		t.Errorf("spec.password = %v, want pw-1", spec["password"])
	}
	if spec["enable"] != true {
		t.Error("renewal must re-enable an expired client")
	}
}

func TestRenewClient_GivenShadowsocks_ThenEmailIsThePanelKey(t *testing.T) {
	// x-ui keys ss clients by EMAIL (not password) — the clientID the caller
	// passes must be the email and the spec must carry it too.
	panel := &fakePanel{inbounds: []xui.Inbound{{
		ID: 8, Protocol: "shadowsocks", Enable: true, Port: 20008,
		Settings: settingsWithClient(map[string]interface{}{
			"email": "s@vpn.kt", "password": "ss-pw", "totalGB": float64(30), "enable": true,
			"expiryTime": float64(time.Now().Add(5 * 24 * time.Hour).UnixMilli()),
		}),
	}}}
	svc := renewService(t, panel)

	if err := svc.RenewClient(context.Background(), 9, "s@vpn.kt", "s@vpn.kt", "shadowsocks", time.Now().Add(35*24*time.Hour)); err != nil {
		t.Fatalf("RenewClient: %v", err)
	}
	if panel.updated.ClientID != "s@vpn.kt" {
		t.Errorf("client id = %q, want s@vpn.kt (ss key is the email)", panel.updated.ClientID)
	}
	spec := updatedClient(t, panel)
	if spec["email"] != "s@vpn.kt" {
		t.Errorf("spec.email = %v, want s@vpn.kt", spec["email"])
	}
	if spec["password"] != "ss-pw" {
		t.Errorf("ss password wiped: %v", spec["password"])
	}
}

func TestRenewClient_GivenHysteria_ThenAuthKeyAndAuthPreserved(t *testing.T) {
	// Hysteria credentials are stored in the bot's password column (provision
	// maps auth → Password) but the panel keys the client by `auth` — the path
	// param must be the auth and the spec must keep it.
	panel := &fakePanel{inbounds: []xui.Inbound{{
		ID: 9, Protocol: "hysteria", Enable: true, Port: 20009,
		Settings: settingsWithClient(map[string]interface{}{
			"auth": "auth-1", "email": "h@vpn.kt", "totalGB": float64(10), "enable": true,
			"expiryTime": float64(time.Now().Add(3 * 24 * time.Hour).UnixMilli()),
		}),
	}}}
	svc := renewService(t, panel)

	if err := svc.RenewClient(context.Background(), 9, "auth-1", "h@vpn.kt", "hysteria", time.Now().Add(33*24*time.Hour)); err != nil {
		t.Fatalf("RenewClient: %v", err)
	}
	if panel.updated.ClientID != "auth-1" {
		t.Errorf("client id = %q, want auth-1 (hysteria key is auth)", panel.updated.ClientID)
	}
	spec := updatedClient(t, panel)
	if spec["auth"] != "auth-1" {
		t.Errorf("spec.auth = %v, want auth-1 preserved", spec["auth"])
	}
}

func TestRenewClient_GivenClientNotFound_ThenError(t *testing.T) {
	panel := &fakePanel{inbounds: []xui.Inbound{{
		ID: 6, Protocol: "vless", Enable: true, Port: 20006,
		Settings: settingsWithClient(map[string]interface{}{"id": "uuid-1", "email": "other@vpn.kt"}),
	}}}
	svc := renewService(t, panel)

	err := svc.RenewClient(context.Background(), 9, "uuid-1", "a@vpn.kt", "vless", time.Now())
	if err == nil {
		t.Fatal("RenewClient = nil, want error when client email is missing")
	}
	if panel.updated != nil {
		t.Error("updateClient must not be called for a missing client")
	}
}
