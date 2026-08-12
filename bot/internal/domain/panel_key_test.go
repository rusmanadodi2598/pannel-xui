// Package domain test covers the panel client key mapping (v1.38).
//
// @file      internal/domain/panel_key_test.go
// @for       PanelClientKey: the credential x-ui's API identifies a client by,
// per protocol (renew + delete rely on it).
// @uses      testing
// @reason    The key drives panel updateClient/delClient path params — a wrong
// key fails "empty client ID" (staging E2E v1.37) — so the mapping is locked.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     domain
// @stability stable
// @since     2026-08-12
package domain

import "testing"

func TestPanelClientKey_GivenProtocol_ThenCredentialField(t *testing.T) {
	cases := []struct {
		name     string
		protocol string
		uuid     string
		password string
		email    string
		want     string
	}{
		{"vless→uuid", "vless", "uuid-1", "pw", "a@vpn.kt", "uuid-1"},
		{"vmess→uuid", "vmess", "uuid-2", "pw", "a@vpn.kt", "uuid-2"},
		{"trojan→password", "trojan", "", "pw-1", "a@vpn.kt", "pw-1"},
		{"hysteria→password(auth)", "hysteria", "", "auth-1", "a@vpn.kt", "auth-1"},
		{"shadowsocks→email", "shadowsocks", "", "ss-pw", "s@vpn.kt", "s@vpn.kt"},
		{"case-insensitive", "VLESS", "uuid-3", "pw", "a@vpn.kt", "uuid-3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PanelClientKey(tc.protocol, tc.uuid, tc.password, tc.email); got != tc.want {
				t.Errorf("PanelClientKey(%q) = %q, want %q", tc.protocol, got, tc.want)
			}
		})
	}
}
