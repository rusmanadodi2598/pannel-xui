// Package config also hosts the FR-13 subscription settings (split for §1.1).
//
// @file      internal/config/subscription.go
// @for       FR-13: panel sub server settings (URL prefix + paths, Opsi 2).
// @uses      fmt, net/url, strings
// @reason    Keeps config.go under 250 lines while centralizing the public sub
// URL that the order flow persists into vpn_clients (FR-13 AC).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     config
// @stability stable
// @since     2026-08-12
package config

import (
	"fmt"
	"net/url"
	"strings"
)

// FR-13 subscription defaults. SUB_ENABLED/SUB_JSON_ENABLED default false so a
// bare .env boots without a sub server; the paths match the panel's settings
// defaults (web/service/setting.go: subPath=/sub/, subJsonPath=/json/).
const (
	DefaultSubEnabled     = false
	DefaultSubPath        = "/sub/"
	DefaultSubJSONEnabled = false
	DefaultSubJSONPath    = "/json/"
)

// applySubscription parses the SUB_* env vars into the config.
func (c *Config) applySubscription() error {
	var err error
	if c.SubEnabled, err = parseBoolEnv("SUB_ENABLED", DefaultSubEnabled); err != nil {
		return err
	}
	c.SubBaseURL = getEnv("SUB_BASE_URL", "")
	if c.SubPath, err = normalizeSubPath(getEnv("SUB_PATH", DefaultSubPath)); err != nil {
		return err
	}
	if c.SubJSONEnabled, err = parseBoolEnv("SUB_JSON_ENABLED", DefaultSubJSONEnabled); err != nil {
		return err
	}
	if c.SubJSONPath, err = normalizeSubPath(getEnv("SUB_JSON_PATH", DefaultSubJSONPath)); err != nil {
		return err
	}
	return nil
}

// normalizeSubPath trims trailing slashes so the URL join in ordersvc.SubLinks
// always yields /sub/{subId} regardless of how the env value is written.
func normalizeSubPath(raw string) (string, error) {
	p := strings.TrimSpace(raw)
	p = strings.TrimRight(p, "/")
	if p == "" || !strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("sub path must start with '/' and be non-empty, got %q", raw)
	}
	return p, nil
}

// validateSubscription enforces the FR-13 invariants: when the feature is on,
// the public base URL must be an http(s) URL and both paths must be valid.
func (c *Config) validateSubscription() error {
	if c.SubPath == "" || !strings.HasPrefix(c.SubPath, "/") {
		return fmt.Errorf("SUB_PATH must start with '/': %q", c.SubPath)
	}
	if c.SubJSONPath == "" || !strings.HasPrefix(c.SubJSONPath, "/") {
		return fmt.Errorf("SUB_JSON_PATH must start with '/': %q", c.SubJSONPath)
	}
	if !c.SubEnabled {
		return nil
	}
	if c.SubBaseURL == "" {
		return fmt.Errorf("SUB_BASE_URL is required when SUB_ENABLED=true")
	}
	u, err := url.Parse(c.SubBaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("SUB_BASE_URL must be an http(s) URL, got %q", c.SubBaseURL)
	}
	return nil
}
