// Package telegram test covers the FR-08 AC-1 account-list status display.
//
// @file      internal/service/telegram/menu_account_list_test.go
// @for       accountListStatus (Aktif/Hampir Habis/Expired), smart remaining
// time, trial badge in AccountsText.
// @uses      testing, strings, time, internal/repository/postgres
// @reason    Status & sisa waktu di list adalah kontrak AC-1 (parity reference
// ✅/⚠️/❌ sebagai teks polos — icon policy; threshold kuota parity AC-3).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"strings"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

func listClient(trial, active bool, used, limit int64, expires *time.Time) postgres.ClientView {
	return postgres.ClientView{
		VPNClient: postgres.VPNClient{
			Email: "a@vpn.kt", Protocol: "vless", IsTrial: trial, IsActive: active,
			TrafficUsed: used, TrafficLimit: limit, ExpiresAt: expires,
		},
		ServerName: "ID-01",
	}
}

func TestAccountListStatus_GivenActiveLowUsage_ThenAktif(t *testing.T) {
	now := time.Now()
	future := now.Add(10 * 24 * time.Hour)
	if got := accountListStatus(listClient(false, true, 10, 100, &future), now); got != "Aktif" {
		t.Errorf("active low usage = %q, want Aktif", got)
	}
}

func TestAccountListStatus_GivenActiveHighUsage_ThenHampirHabis(t *testing.T) {
	now := time.Now()
	future := now.Add(10 * 24 * time.Hour)
	if got := accountListStatus(listClient(false, true, 95, 100, &future), now); got != "Hampir Habis" {
		t.Errorf("active 95%% usage = %q, want Hampir Habis (AC-3 threshold)", got)
	}
}

func TestAccountListStatus_GivenInactive_ThenHampirHabis(t *testing.T) {
	now := time.Now()
	future := now.Add(10 * 24 * time.Hour)
	if got := accountListStatus(listClient(false, false, 0, 0, &future), now); got != "Hampir Habis" {
		t.Errorf("inactive = %q, want Hampir Habis (reference disabled state)", got)
	}
}

func TestAccountListStatus_GivenExpired_ThenExpired(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Hour)
	// Flag IsExpired langsung maupun lewat tanggal lewat → Expired.
	if got := accountListStatus(listClient(false, true, 0, 0, &past), now); got != "Expired" {
		t.Errorf("past expiry = %q, want Expired", got)
	}
	c := listClient(false, true, 95, 100, nil)
	c.IsExpired = true
	if got := accountListStatus(c, now); got != "Expired" {
		t.Errorf("IsExpired flag = %q, want Expired (menang atas kuota)", got)
	}
}

func TestAccountRemaining_GivenDays_ThenDaysLabel(t *testing.T) {
	now := time.Now()
	future := now.Add(5 * 24 * time.Hour) // deterministik: 5 hari → \"sisa 6 hari\"
	if got := accountRemaining(listClient(false, true, 0, 0, &future), now); got != "sisa 6 hari" {
		t.Errorf("remaining days = %q, want sisa 6 hari", got)
	}
}

func TestAccountRemaining_GivenUnderDay_ThenHoursLabel(t *testing.T) {
	now := time.Now()
	twoHours := now.Add(2 * time.Hour)
	if got := accountRemaining(listClient(true, true, 0, 0, &twoHours), now); got != "sisa 2 jam" {
		t.Errorf("remaining 2h = %q, want sisa 2 jam (trial smart display)", got)
	}
	halfHour := now.Add(30 * time.Minute)
	if got := accountRemaining(listClient(true, true, 0, 0, &halfHour), now); got != "sisa 1 jam" {
		t.Errorf("remaining 30m = %q, want sisa 1 jam (floor min 1)", got)
	}
}

func TestAccountRemaining_GivenExpired_ThenSudahHabis(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Minute)
	if got := accountRemaining(listClient(false, true, 0, 0, &past), now); got != "sudah habis" {
		t.Errorf("past expiry = %q, want sudah habis", got)
	}
}

func TestAccountsText_GivenTrialHighUsage_ThenTrialBadgeAndHampirHabis(t *testing.T) {
	now := time.Now()
	future := now.Add(45 * time.Minute)
	clients := []postgres.ClientView{listClient(true, true, 98, 100, &future)}
	text := AccountsText(clients, 1, 1, now)
	for _, want := range []string{"Trial · Hampir Habis", "sisa 1 jam"} {
		if !strings.Contains(text, want) {
			t.Errorf("AccountsText missing %q in:\n%s", want, text)
		}
	}
	// Icon policy: badge trial adalah teks polos, bukan emoji di body copy.
	if strings.Contains(text, "🎁") {
		t.Errorf("trial badge must be text-only (icon policy):\n%s", text)
	}
}
