// Package telegram test covers the FR-04 AC admin-group notice (v1.41).
//
// @file      internal/service/telegram/menu_order_notice_test.go
// @for       AdminOrderNoticeText: purchase/renewal labels + full payload fields.
// @uses      testing, strings, internal/domain
// @reason    The admin notice is ops telemetry — guards the copy contract so a
// completed paid order always reports a complete, readable notice.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-12
package telegram

import (
	"strings"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
)

func TestAdminOrderNoticeText_GivenPurchase_ThenAllFieldsRendered(t *testing.T) {
	expiry := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	text := AdminOrderNoticeText("KTS-1-VPN", domain.OrderTypePurchase, "Budi", "ID 30 Hari",
		"u@vpn.kt", domain.Money(7000), domain.Money(43000), expiry)
	for _, want := range []string{"Beli VPN", "KTS-1-VPN", "Budi", "ID 30 Hari", "Rp 7.000", "u@vpn.kt", "11 Sep 2026", "Rp 43.000"} {
		if !strings.Contains(text, want) {
			t.Errorf("notice missing %q in:\n%s", want, text)
		}
	}
}

func TestAdminOrderNoticeText_GivenRenewal_ThenRenewalLabel(t *testing.T) {
	text := AdminOrderNoticeText("KTS-2-VPN", domain.OrderTypeRenewal, "Sari", "SG 30 Hari",
		"r@vpn.kt", domain.Money(12000), domain.Money(20000), time.Now())
	if !strings.Contains(text, "Perpanjang") {
		t.Errorf("renewal notice missing 'Perpanjang' label:\n%s", text)
	}
}

// TestAdminOrderNoticeText_GivenSensitiveFields_ThenNeverLeaked asserts the
// masking contract: the admin-group notice carries only ops data (order id,
// labels, amounts, account EMAIL, expiry). Credentials — UUID/password/config
// link/sub URL — are never part of OrderNotice and must never appear in text.
func TestAdminOrderNoticeText_GivenSensitiveFields_ThenNeverLeaked(t *testing.T) {
	text := AdminOrderNoticeText("KTS-3-VPN", domain.OrderTypePurchase, "Budi", "ID 30 Hari",
		"u@vpn.kt", domain.Money(7000), domain.Money(43000), time.Now())
	for _, secret := range []string{
		"uuid", "UUID", "password", "Password",
		"vless://", "vmess://", "trojan://", "wss://",
		"sub", "/sub/", "config", "Config",
	} {
		if strings.Contains(text, secret) {
			t.Errorf("notice leaked %q — credentials/config must never reach the admin group:\n%s", secret, text)
		}
	}
	// Email is the only identifier, and it renders in full (product decision).
	if !strings.Contains(text, "u@vpn.kt") {
		t.Errorf("notice must render the account email in full:\n%s", text)
	}
}
