// Package ordersvc also hosts the FR-13 subscription URL builder.
//
// @file      internal/service/order/subscription.go
// @for       FR-13: build subscription URLs from the panel sub server config.
// @uses      strings
// @reason    The order flow persists the sub URL at provisioning time; the
// builder is a pure value so it is trivially unit-testable and the telegram
// views never need config access (URLs ship only via the .txt export).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-12
package ordersvc

import "strings"

// SubLinks carries the FR-13 panel sub server settings (Opsi 2: same domain as
// the panel, different port — the panel sub server defaults to :2096 with
// subPath /sub/ and subJsonPath /json/). It is wired from config at boot.
type SubLinks struct {
	BaseURL  string // public sub base, e.g. https://id2.kentangtechstore.net:2096
	LinkPath string // panel subPath (default /sub)
	JSONPath string // panel subJsonPath (default /json); empty = JSON disabled
}

// URL returns the standard subscription URL for a subId
// ({base}/{LinkPath}/{subId}), or "" when the feature is unconfigured.
// Base and path may carry trailing slashes — the join is canonical.
// join builds {base}/{path}/{subId} with a canonical join; returns "" when
// any segment is empty after trimming (guards a bare "/" path from producing
// a double slash — SubLinks is a public type usable outside config).
func (l SubLinks) join(path, subID string) string {
	if l.BaseURL == "" || strings.Trim(path, "/") == "" || subID == "" {
		return ""
	}
	return strings.TrimRight(l.BaseURL, "/") + "/" + strings.Trim(path, "/") + "/" + subID
}

// URL returns the standard subscription URL for a subId
// ({base}/{LinkPath}/{subId}), or "" when the feature is unconfigured.
func (l SubLinks) URL(subID string) string {
	return l.join(l.LinkPath, subID)
}

// JSONURL returns the JSON/Clash subscription URL for a subId, or "" when the
// JSON sub is disabled or unconfigured.
func (l SubLinks) JSONURL(subID string) string {
	return l.join(l.JSONPath, subID)
}

// SetSubLinks wires the FR-13 sub server config (called once at composition).
// Kept as a setter so the variadic notifier in New stays backward compatible.
func (s *Service) SetSubLinks(l SubLinks) { s.subLinks = l }
