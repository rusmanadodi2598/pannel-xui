// Package telegram test covers the admin views (FR-11, M7 hardening).
//
// @file      internal/service/telegram/menu_admin_test.go
// @for       Admin menu copy: price list, detail, broadcast preview, ban/unban confirm.
// @uses      testing, strings, github.com/go-telegram/bot/models, internal/domain,
// internal/repository/postgres
// @reason    Admin copy carries callback payloads (plan code + days) — any
// shape change breaks the handler routing (M7 coverage gap).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

func TestAdminPriceKeyboard_GivenDisabledPlan_ThenMarkedAndCallback(t *testing.T) {
	kb := AdminPriceKeyboard([]domain.VpnPlan{plan("ID", "Indonesia", 30, 7500, false)})
	markup, ok := kb.(models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("keyboard type = %T", kb)
	}
	btn := markup.InlineKeyboard[0][0]
	if !strings.HasPrefix(btn.Text, "🚫") {
		t.Errorf("disabled plan text = %q, want 🚫 marker", btn.Text)
	}
	if btn.CallbackData != "admin:plan:ID:30" {
		t.Errorf("callback = %q, want admin:plan:ID:30", btn.CallbackData)
	}
}

func TestAdminPlanDetailText_GivenDisabled_ThenStatusNonaktif(t *testing.T) {
	text := AdminPlanDetailText(plan("ID", "Indonesia", 30, 7500, false))
	if !strings.Contains(text, "Status: Nonaktif") || !strings.Contains(text, "Rp 7.500") {
		t.Errorf("detail = %q, want Nonaktif + price", text)
	}
}

func TestAdminSetPricePrompt_GivenPlan_ThenMentionsCodeAndDays(t *testing.T) {
	text := AdminSetPricePrompt("ID", 30)
	if !strings.Contains(text, "ID 30 Hari") || !strings.Contains(text, "/cancel") {
		t.Errorf("prompt = %q", text)
	}
}

func TestAdminPlanToggledText_GivenDisabled_ThenDinonaktifkan(t *testing.T) {
	text := AdminPlanToggledText("ID", 30, false)
	if !strings.Contains(text, "dinonaktifkan") {
		t.Errorf("toggled = %q, want dinonaktifkan", text)
	}
}

func TestAdminBroadcastPreviewText_GivenText_ThenQuoted(t *testing.T) {
	text := AdminBroadcastPreviewText("Promo minggu ini")
	if !strings.Contains(text, "Promo minggu ini") || !strings.Contains(text, "Kirim pesan ini ke semua user") {
		t.Errorf("preview = %q", text)
	}
}

func TestBroadcastDoneText_GivenCounts_ThenReport(t *testing.T) {
	text := BroadcastDoneText(99, 1)
	if !strings.Contains(text, "Terkirim: 99") || !strings.Contains(text, "Gagal: 1") {
		t.Errorf("done = %q, want 99/1 report", text)
	}
}

func TestAdminBanConfirmText_GivenRegisteredUser_ThenShowsName(t *testing.T) {
	u := &postgres.User{TelegramID: 42, FirstName: "Budi"}
	text := AdminBanConfirmText(u, 42)
	if !strings.Contains(text, "Budi (ID 42)") {
		t.Errorf("ban confirm = %q, want name + id", text)
	}
}

func TestAdminUserLabel_GivenNil_ThenNotRegistered(t *testing.T) {
	if got := adminUserLabel(nil); got != "User tidak terdaftar" {
		t.Errorf("label(nil) = %q, want 'User tidak terdaftar'", got)
	}
	u := &postgres.User{TelegramID: 7, Username: "nadia"}
	if got := adminUserLabel(u); got != "@nadia" {
		t.Errorf("label(username) = %q, want @nadia", got)
	}
	u2 := &postgres.User{TelegramID: 8}
	if got := adminUserLabel(u2); got != "User 8" {
		t.Errorf("label(fallback) = %q, want 'User 8'", got)
	}
}

func TestAdminBanConfirmKeyboard_GivenID_ThenConfirmCallback(t *testing.T) {
	kb := AdminBanConfirmKeyboard(99)
	markup, _ := kb.(models.InlineKeyboardMarkup)
	if markup.InlineKeyboard[0][0].CallbackData != "admin:ban:confirm:99" {
		t.Errorf("callback = %q, want admin:ban:confirm:99", markup.InlineKeyboard[0][0].CallbackData)
	}
}

func TestAdminUnbanConfirmKeyboard_GivenID_ThenConfirmCallback(t *testing.T) {
	kb := AdminUnbanConfirmKeyboard(88)
	markup, _ := kb.(models.InlineKeyboardMarkup)
	if markup.InlineKeyboard[0][0].CallbackData != "admin:unban:confirm:88" {
		t.Errorf("callback = %q, want admin:unban:confirm:88", markup.InlineKeyboard[0][0].CallbackData)
	}
}

func TestAdminMenuKeyboard_GivenLayout_ThenFourActionsPlusHome(t *testing.T) {
	kb := AdminMenuKeyboard()
	markup, _ := kb.(models.InlineKeyboardMarkup)
	if len(markup.InlineKeyboard) != 5 {
		t.Fatalf("rows = %d, want 5 (price/broadcast/ban/unban/home)", len(markup.InlineKeyboard))
	}
	if markup.InlineKeyboard[0][0].CallbackData != CallbackAdminPrice {
		t.Errorf("first = %q, want admin:price", markup.InlineKeyboard[0][0].CallbackData)
	}
}
