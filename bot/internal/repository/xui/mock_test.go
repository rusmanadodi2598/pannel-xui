// Package xui_test hosts the mock panel server and shared helpers.
//
// @file      internal/repository/xui/mock_test.go
// @for       httptest mock of the panel: /login (x-ui cookie), /xui/API/* handlers.
// @uses      testing, net/http, net/http/httptest, net/url, strings, encoding/json, sync, time
// @reason    Lets client tests run without a real panel (AGENTS.md §2.1).
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
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/xui"
)

// mockPanel simulates the fork panel behavior verified from source:
// - /login: form username/password → sets "x-ui" cookie (web/session/session.go)
// - API routes under /xui/API/* require the cookie; else 401 (pureJsonMsg).
type mockPanel struct {
	mu       sync.Mutex
	username string
	password string
	logins   int
	// session holds the cookie value issued by the most recent login; sessionValid
	// reports whether that session is accepted by API routes (false = stale).
	session      string
	sessionValid bool
	// API handlers registered by the test.
	routes map[string]func(w http.ResponseWriter, r *http.Request)
	// Invalidates the session after N logins (0 = always valid).
	expireAfterLogins int
}

func newMockPanel(username, password string) *mockPanel {
	return &mockPanel{
		username: username,
		password: password,
		routes:   map[string]func(w http.ResponseWriter, r *http.Request){},
	}
}

func (m *mockPanel) register(path string, h func(w http.ResponseWriter, r *http.Request)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.routes[path] = h
}

func (m *mockPanel) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			m.handleLogin(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/xui/API") {
			http.NotFound(w, r)
			return
		}
		if !m.hasValidSession(r) {
			writeEnvelope(w, http.StatusUnauthorized, false, "login again", nil)
			return
		}
		m.mu.Lock()
		h, ok := m.routes[r.URL.Path]
		m.mu.Unlock()
		if !ok {
			writeEnvelope(w, http.StatusNotFound, false, "no such route", nil)
			return
		}
		h(w, r)
	})
}

func (m *mockPanel) handleLogin(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logins++

	// Simulate session expiry: after expireAfterLogins logins, API rejects.
	// The client relogs on 401; mock counts logins to let tests force that path.
	if r.Form.Get("username") != m.username || r.Form.Get("password") != m.password {
		writeEnvelope(w, http.StatusOK, false, "wrong username or password", nil)
		return
	}
	// A stable session cookie value per login (no re-computation later).
	// With expireAfterLogins=N, the first N logins issue stale sessions so the
	// client must relogin on 401 (simulates panel session expiry).
	m.session = "xui-session-" + strconv.Itoa(m.logins)
	m.sessionValid = m.expireAfterLogins == 0 || m.logins > m.expireAfterLogins
	http.SetCookie(w, &http.Cookie{Name: "x-ui", Value: m.session, Path: "/"})
	writeEnvelope(w, http.StatusOK, true, "login success", nil)
}

func (m *mockPanel) hasValidSession(r *http.Request) bool {
	c, err := r.Cookie("x-ui")
	if err != nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.expireAfterLogins > 0 {
		// Only the latest session works, and only if it is marked valid.
		return m.session != "" && c.Value == m.session && m.sessionValid
	}
	return strings.HasPrefix(c.Value, "xui-session-")
}

// msgEnvelope mirrors entity.Msg (web/controller/util.go: jsonMsgObj).
type msgEnvelope struct {
	Success bool            `json:"success"`
	Msg     string          `json:"msg"`
	Obj     json.RawMessage `json:"obj"`
}

// writeEnvelope serializes a panel envelope with an optional raw obj payload.
func writeEnvelope(w http.ResponseWriter, status int, success bool, msg string, obj any) {
	var raw json.RawMessage
	if obj != nil {
		raw, _ = json.Marshal(obj)
	}
	env := msgEnvelope{Success: success, Msg: msg, Obj: raw}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(env)
}

// newTestClient spins a mock panel and returns its URL + a configured client.
func newTestClient(t *testing.T, m *mockPanel, cache xui.SessionCache) (string, *xui.Client) {
	t.Helper()
	srv := httptest.NewServer(m.Handler())
	t.Cleanup(srv.Close)
	cfg := xui.ServerConfig{
		BaseURL:    srv.URL,
		APIPath:    "/",
		Username:   m.username,
		Password:   m.password,
		Timeout:    5 * time.Second,
		ServerID:   1,
		SessionTTL: time.Hour,
	}
	return srv.URL, xui.NewClient(cfg, cache)
}

// fakeCache is an in-memory SessionCache for tests.
type fakeCache struct {
	mu   sync.Mutex
	data map[int64]string
}

func newFakeCache() *fakeCache {
	return &fakeCache{data: map[int64]string{}}
}

func (f *fakeCache) Get(_ context.Context, id int64) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.data[id], nil
}

func (f *fakeCache) Set(_ context.Context, id int64, v string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[id] = v
	return nil
}

func (f *fakeCache) Del(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.data, id)
	return nil
}

// parseForm is a convenience for handler assertions.
func parseForm(r *http.Request) url.Values {
	_ = r.ParseForm()
	return r.Form
}
