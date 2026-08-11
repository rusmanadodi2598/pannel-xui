// Package xui also hosts the panel REST client core.
//
// @file      internal/repository/xui/client.go
// @for       Authenticated REST client: login, session cache, auto-relogin, envelope decode.
// @uses      context, net/http, net/url, encoding/json, fmt, strings, sync, io
// @reason    Single place implementing panel auth & envelope contract (PRD §15.1/§15.6).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package xui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// cookieName is the panel session cookie (gin-contrib sessions, web/web.go).
const cookieName = "x-ui"

// Client talks to one X-UI panel over REST (login + cookie session).
type Client struct {
	cfg   ServerConfig
	hc    *http.Client
	cache SessionCache

	mu     sync.Mutex
	cookie string // "x-ui=<value>"
	loaded bool   // true once a cookie has been loaded from cache
}

// NewClient builds a panel client. cache may be nil (no session persistence).
func NewClient(cfg ServerConfig, cache SessionCache) *Client {
	return &Client{
		cfg:   cfg,
		hc:    newHTTPClient(cfg),
		cache: cache,
	}
}

func newHTTPClient(cfg ServerConfig) *http.Client {
	transport := &http.Transport{}
	if cfg.Insecure {
		transport.TLSClientConfig = insecureTLS()
	}
	return &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout,
	}
}

// base returns the panel base URL including api path (no trailing slash).
func (c *Client) base() string {
	apiPath := c.cfg.APIPath
	if apiPath == "" {
		apiPath = "/"
	}
	return strings.TrimRight(c.cfg.BaseURL, "/") + "/" + strings.TrimLeft(apiPath, "/")
}

// Login authenticates with username/password (form POST /login) and stores the
// x-ui session cookie, cached in Redis when ServerID > 0 and cache != nil.
func (c *Client) Login(ctx context.Context) error {
	form := url.Values{}
	form.Set("username", c.cfg.Username)
	form.Set("password", c.cfg.Password)

	loginURL := c.base() + "login"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("xui login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.hc.Do(req)
	if err != nil {
		return &XUIError{Code: CodeNetwork, Message: fmt.Sprintf("login: %v", err), StatusCode: 0}
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var envelope APIResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return &XUIError{Code: CodeUnknown, Message: "login: invalid response", StatusCode: resp.StatusCode}
	}
	if !envelope.Success {
		return &XUIError{Code: CodeAuth, Message: "login rejected: " + envelope.Msg, StatusCode: resp.StatusCode}
	}

	cookie := extractCookie(resp.Cookies(), cookieName)
	if cookie == "" {
		return &XUIError{Code: CodeAuth, Message: "login ok but no session cookie set", StatusCode: resp.StatusCode}
	}

	c.mu.Lock()
	c.cookie = cookieName + "=" + cookie
	c.loaded = true
	c.mu.Unlock()

	if c.cache != nil && c.cfg.ServerID > 0 {
		// Best-effort: a Redis hiccup must not fail the successful login.
		_ = c.cache.Set(ctx, c.cfg.ServerID, c.cookie, c.cfg.SessionTTL)
	}
	return nil
}

// ensureSession returns the current cookie, logging in when absent.
// All mutex sections are short and released before any I/O (Login/cache) to
// avoid deadlocks; duplicate concurrent logins are harmless (idempotent).
func (c *Client) ensureSession(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.cookie != "" {
		cookie := c.cookie
		c.mu.Unlock()
		return cookie, nil
	}
	c.mu.Unlock()

	if !c.loaded && c.cache != nil && c.cfg.ServerID > 0 {
		c.mu.Lock()
		c.loaded = true
		c.mu.Unlock()

		cached, err := c.cache.Get(ctx, c.cfg.ServerID)
		if err == nil && cached != "" {
			c.mu.Lock()
			c.cookie = cached
			c.mu.Unlock()
			return cached, nil
		}
	}

	if err := c.Login(ctx); err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cookie, nil
}

// invalidate clears the in-memory cookie (and cache) so the next request relogs.
func (c *Client) invalidate(ctx context.Context) {
	c.mu.Lock()
	c.cookie = ""
	c.mu.Unlock()
	if c.cache != nil && c.cfg.ServerID > 0 {
		_ = c.cache.Del(ctx, c.cfg.ServerID)
	}
}

// do performs an authenticated request and decodes the panel envelope.
// On 401/403 it forces a re-login once and retries (PRD §15.1).
func (c *Client) do(ctx context.Context, method, path string, form url.Values, out any) error {
	return c.doOnce(ctx, method, path, form, out, true)
}

// doOnce implements do; allowRetry gates the single re-login attempt.
func (c *Client) doOnce(ctx context.Context, method, path string, form url.Values, out any, allowRetry bool) error {
	cookie, err := c.ensureSession(ctx)
	if err != nil {
		return err
	}

	reqURL := c.base() + strings.TrimLeft(path, "/")
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return fmt.Errorf("xui request: %w", err)
	}
	req.Header.Set("Cookie", cookie)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return &XUIError{Code: CodeNetwork, Message: err.Error(), StatusCode: 0}
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		if !allowRetry {
			// Session rejected again after a fresh login — hard failure.
			return &XUIError{Code: CodeAuth, Message: "session rejected", StatusCode: resp.StatusCode}
		}
		c.invalidate(ctx)
		return c.doOnce(ctx, method, path, form, out, false) // retry once with fresh login
	}

	var envelope APIResponse
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return &XUIError{Code: CodeUnknown, Message: "invalid panel response", StatusCode: resp.StatusCode}
	}
	if !envelope.Success {
		code := classify(envelope.Msg)
		return &XUIError{Code: code, Message: envelope.Msg, StatusCode: resp.StatusCode}
	}

	if out != nil && len(envelope.Obj) > 0 && string(envelope.Obj) != "null" {
		if err := json.Unmarshal(envelope.Obj, out); err != nil {
			return &XUIError{Code: CodeUnknown, Message: fmt.Sprintf("decode obj: %v", err), StatusCode: resp.StatusCode}
		}
	}
	return nil
}

func extractCookie(cookies []*http.Cookie, name string) string {
	for _, c := range cookies {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}
