// Package xui_test covers the panel REST client against the mock server.
//
// @file      internal/repository/xui/client_test.go
// @for       Login, session cache, auto-relogin, addClient golden payload, error mapping.
// @uses      testing, context, net/http, encoding/json, strings, time
// @reason    Locks the panel API contract (PRD §15.2/§15.3) verified from source.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package xui_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/xui"
)

func TestLogin_GivenValidCredentials_ThenSessionCookieStored(t *testing.T) {
	m := newMockPanel("admin", "secret")
	_, c := newTestClient(t, m, newFakeCache())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Login(ctx); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if m.logins != 1 {
		t.Errorf("logins = %d, want 1", m.logins)
	}
}

func TestLogin_GivenWrongPassword_ThenAuthError(t *testing.T) {
	// Panel expects "secret"; the client is built with a different password.
	m := newMockPanel("admin", "secret")
	srv := httptest.NewServer(m.Handler())
	t.Cleanup(srv.Close)
	c := xui.NewClient(xui.ServerConfig{
		BaseURL:  srv.URL,
		APIPath:  "/",
		Username: "admin",
		Password: "wrong-password",
		Timeout:  5 * time.Second,
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := c.Login(ctx)
	if err == nil {
		t.Fatal("login with wrong password must fail")
	}
	if xe, ok := err.(*xui.XUIError); !ok || xe.Code != xui.CodeAuth {
		t.Errorf("error = %v, want XUIError AUTH", err)
	}
}

func TestRequest_GivenCachedSession_ThenSkipsLogin(t *testing.T) {
	m := newMockPanel("admin", "secret")
	m.register("/xui/API/inbounds/", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(w, http.StatusOK, true, "", []xui.Inbound{{ID: 7, Protocol: "vless", Port: 443}})
	})
	cache := newFakeCache()
	_, c := newTestClient(t, m, cache)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Seed a valid cached session (matches mock's xui-session- prefix) so no login is needed.
	_ = cache.Set(ctx, 1, "x-ui=xui-session-seeded", time.Hour)

	inbounds, err := c.GetInbounds(ctx)
	if err != nil {
		t.Fatalf("GetInbounds: %v", err)
	}
	if m.logins != 0 {
		t.Errorf("logins = %d, want 0 (cached session used)", m.logins)
	}
	if len(inbounds) != 1 || inbounds[0].ID != 7 {
		t.Errorf("inbounds = %+v, want [id=7]", inbounds)
	}
}

func TestRequest_GivenExpiredSession_ThenAutoReloginAndRetry(t *testing.T) {
	m := newMockPanel("admin", "secret")
	m.register("/xui/API/inbounds/", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(w, http.StatusOK, true, "", []xui.Inbound{{ID: 1, Protocol: "trojan"}})
	})
	m.expireAfterLogins = 1 // only the first session works; second login is fresh

	_, c := newTestClient(t, m, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	inbounds, err := c.GetInbounds(ctx)
	if err != nil {
		t.Fatalf("GetInbounds after relogin: %v", err)
	}
	if len(inbounds) != 1 {
		t.Fatalf("inbounds = %+v", inbounds)
	}
	if m.logins != 2 {
		t.Errorf("logins = %d, want 2 (initial + relogin on 401)", m.logins)
	}
}

func TestAddClient_GivenVlessClient_ThenGoldenPayload(t *testing.T) {
	m := newMockPanel("admin", "secret")
	var gotForm map[string][]string
	m.register("/xui/API/inbounds/addClient", func(w http.ResponseWriter, r *http.Request) {
		gotForm = parseForm(r)
		writeEnvelope(w, http.StatusOK, true, "Client(s) added", nil)
	})
	_, c := newTestClient(t, m, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := c.AddClient(ctx, xui.AddClientPayload{
		InboundID: 5,
		Client: xui.ClientSpec{
			ID:         "11111111-2222-3333-4444-555555555555",
			Email:      "user123-1712345678@vpn.id",
			LimitIP:    2,
			TotalGB:    30 * 1024 * 1024 * 1024,
			ExpiryTime: 1750000000000,
			Enable:     true,
			Flow:       "xtls-rprx-vision",
			SubID:      "11111111",
			TgID:       "123",
			Reset:      0,
		},
	})
	if err != nil {
		t.Fatalf("AddClient: %v", err)
	}

	if gotForm["id"][0] != "5" {
		t.Errorf("form id = %q, want 5", gotForm["id"])
	}
	var settings struct {
		Clients []map[string]any `json:"clients"`
	}
	if err := json.Unmarshal([]byte(gotForm["settings"][0]), &settings); err != nil {
		t.Fatalf("settings not valid JSON: %v", err)
	}
	if len(settings.Clients) != 1 {
		t.Fatalf("clients len = %d, want 1", len(settings.Clients))
	}
	cl := settings.Clients[0]
	if cl["id"] != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("client id = %v", cl["id"])
	}
	if cl["email"] != "user123-1712345678@vpn.id" {
		t.Errorf("client email = %v", cl["email"])
	}
	if cl["limitIp"] != float64(2) {
		t.Errorf("limitIp = %v, want 2", cl["limitIp"])
	}
	if cl["totalGB"] != float64(30*1024*1024*1024) {
		t.Errorf("totalGB = %v", cl["totalGB"])
	}
	if cl["expiryTime"] != float64(1750000000000) {
		t.Errorf("expiryTime = %v", cl["expiryTime"])
	}
	if cl["flow"] != "xtls-rprx-vision" {
		t.Errorf("flow = %v", cl["flow"])
	}
	if cl["subId"] != "11111111" {
		t.Errorf("subId = %v", cl["subId"])
	}
}

func TestAddClient_GivenTrojanClient_ThenPasswordCredential(t *testing.T) {
	m := newMockPanel("admin", "secret")
	var settings string
	m.register("/xui/API/inbounds/addClient", func(w http.ResponseWriter, r *http.Request) {
		settings = parseForm(r)["settings"][0]
		writeEnvelope(w, http.StatusOK, true, "Client(s) added", nil)
	})
	_, c := newTestClient(t, m, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := c.AddClient(ctx, xui.AddClientPayload{
		InboundID: 3,
		Client:    xui.ClientSpec{Password: "trojan-pass-123", Email: "t1@vpn.id", LimitIP: 1},
	})
	if err != nil {
		t.Fatalf("AddClient: %v", err)
	}
	if !strings.Contains(settings, `"password":"trojan-pass-123"`) {
		t.Errorf("trojan settings missing password field: %s", settings)
	}
	if strings.Contains(settings, `"id"`) {
		t.Errorf("trojan settings must not include id: %s", settings)
	}
}
