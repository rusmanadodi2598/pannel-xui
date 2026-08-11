// Package xui also hosts the TLS transport helpers.
//
// @file      internal/repository/xui/tls.go
// @for       TLS transport config (InsecureSkipVerify for self-signed panels).
// @uses      crypto/tls
// @reason    Keeps client.go focused; insecure mode is per-server config, default off (PRD §15.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package xui

import "crypto/tls"

// insecureTLS returns a TLS config that skips verification.
// Only used when the server explicitly opts in (ServerConfig.Insecure).
func insecureTLS() *tls.Config {
	return &tls.Config{InsecureSkipVerify: true} //nolint:gosec // deliberate, per-server opt-in
}
