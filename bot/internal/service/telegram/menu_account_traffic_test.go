// Package telegram test covers the FR-08 AC-3 traffic page.
//
// @file      internal/service/telegram/menu_account_traffic_test.go
// @for       Status colours, progress bar, bytes formatting, refresh keyboard.
// @uses      testing, strings, time, github.com/go-telegram/bot/models,
// internal/repository/postgres
// @reason    The status thresholds (🟢/🟡/🔴) are the AC contract — regressions
// would mislead users about quota exhaustion.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

func trafficViewClient(used, limit int64) postgres.ClientView {
	now := time.Now()
	return postgres.ClientView{
		VPNClient: postgres.VPNClient{
			Email: "a@vpn.kt", TrafficUsed: used, TrafficLimit: limit,
			TrafficUp: 300, TrafficDown: 700, LastSync: &now,
		},
	}
}

func TestAccountTrafficText_GivenHighUsage_ThenRedStatusAndBar(t *testing.T) {
	text := AccountTrafficText(trafficViewClient(95, 100), time.Now())
	for _, want := range []string{"Detail Traffic", "a@vpn.kt", "🔴", "Hampir Habis",
		"Upload: 300.00 B", "Download: 700.00 B", "Terakhir Sync:"} {
		if !strings.Contains(text, want) {
			t.Errorf("AccountTrafficText missing %q in:\n%s", want, text)
		}
	}
	if !strings.Contains(text, "█████████░") || !strings.Contains(text, "95.0%") {
		t.Errorf("progress bar wrong in:\n%s", text)
	}
	if !strings.Contains(text, "Kuota: 100.00 B") || !strings.Contains(text, "Sisa: 5.00 B") {
		t.Errorf("quota/remaining wrong in:\n%s", text)
	}
}

func TestAccountTrafficText_GivenMediumUsage_ThenYellowStatus(t *testing.T) {
	text := AccountTrafficText(trafficViewClient(75, 100), time.Now())
	if !strings.Contains(text, "🟡") || !strings.Contains(text, "Perhatian") {
		t.Errorf("medium usage must be yellow:\n%s", text)
	}
	if !strings.Contains(text, "███████░░░") {
		t.Errorf("75%% bar wrong in:\n%s", text)
	}
}

func TestAccountTrafficText_GivenLowUsage_ThenGreenStatus(t *testing.T) {
	text := AccountTrafficText(trafficViewClient(10, 100), time.Now())
	if !strings.Contains(text, "🟢") || !strings.Contains(text, "Normal") {
		t.Errorf("low usage must be green:\n%s", text)
	}
}

func TestAccountTrafficText_GivenExhausted_ThenFullBarAndRed(t *testing.T) {
	text := AccountTrafficText(trafficViewClient(150, 100), time.Now())
	if !strings.Contains(text, "🔴") || !strings.Contains(text, "100.0%") {
		t.Errorf("exhausted usage must clamp to red 100%%:\n%s", text)
	}
	if !strings.Contains(text, "██████████") || !strings.Contains(text, "Sisa: 0 B") {
		t.Errorf("exhausted bar/sisa wrong (Sisa clamps to 0, never negative) in:\n%s", text)
	}
}

func TestAccountTrafficText_GivenUnlimited_ThenNoBarAndUnlimitedLabel(t *testing.T) {
	text := AccountTrafficText(trafficViewClient(1000, 0), time.Now())
	if !strings.Contains(text, "Unlimited") || !strings.Contains(text, "🟢") {
		t.Errorf("unlimited label/status missing:\n%s", text)
	}
	if strings.Contains(text, "[█") || strings.Contains(text, "Sisa:") {
		t.Errorf("unlimited must not render a bar or Sisa:\n%s", text)
	}
}

func TestAccountTrafficText_GivenNeverSynced_ThenBelumPernah(t *testing.T) {
	c := trafficViewClient(10, 100)
	c.LastSync = nil
	text := AccountTrafficText(c, time.Now())
	if !strings.Contains(text, "Belum pernah") {
		t.Errorf("never-synced label missing:\n%s", text)
	}
}

func TestTrafficBytes_ThenBinaryUnits(t *testing.T) {
	for _, tt := range []struct {
		in   int64
		want string
	}{
		{0, "0 B"}, {512, "512.00 B"}, {1024, "1.00 KB"},
		{5 * 1024 * 1024, "5.00 MB"}, {2 * 1024 * 1024 * 1024, "2.00 GB"},
	} {
		if got := trafficBytes(tt.in); got != tt.want {
			t.Errorf("trafficBytes(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestAccountTrafficKeyboard_GivenID_ThenRefreshAndBackToDetail(t *testing.T) {
	kb := AccountTrafficKeyboard(3)
	markup, ok := kb.(models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("keyboard type = %T", kb)
	}
	if markup.InlineKeyboard[0][0].CallbackData != "account:traffic:3" ||
		markup.InlineKeyboard[0][0].Text != "Refresh" {
		t.Errorf("refresh button = %+v, want account:traffic:3", markup.InlineKeyboard[0][0])
	}
	// v1.42: [Refresh, Kembali] satu baris.
	if markup.InlineKeyboard[0][1].CallbackData != "account:view:3" {
		t.Errorf("back button = %q, want account:view:3", markup.InlineKeyboard[0][1].CallbackData)
	}
	// Icon policy: the Refresh action button stays icon-free.
	if strings.ContainsAny(markup.InlineKeyboard[0][0].Text, "📄🔒🔓🗑️🔄") {
		t.Errorf("refresh button must be icon-free: %q", markup.InlineKeyboard[0][0].Text)
	}
}
