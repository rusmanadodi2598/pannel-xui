// Package telegram test covers the FR-08 AC-4 delete confirmation views.
//
// @file      internal/service/telegram/menu_account_delete_test.go
// @for       Golden: confirm text warning, Ya/Batal keyboard, success text.
// @uses      testing, strings, time, github.com/go-telegram/bot/models,
// internal/repository/postgres
// @reason    Deletion is destructive — the copy (irreversible warning) and the
// 2-step callback contract are product-critical (FR-08 AC-4, AGENTS.md §2.1).
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

func TestAccountDeleteText_GivenClient_ThenWarningAndDetails(t *testing.T) {
	expiry := time.Now().Add(10 * 24 * time.Hour)
	c := postgres.ClientView{
		VPNClient:  postgres.VPNClient{ID: 3, Email: "del@vpn.kt", Protocol: "vless", ExpiresAt: &expiry},
		ServerName: "ID-01", FlagEmoji: "🇮🇩",
	}
	text := AccountDeleteText(c, time.Now())
	for _, want := range []string{"Konfirmasi Hapus Akun", "del@vpn.kt", "VLESS", "tidak bisa dikembalikan"} {
		if !strings.Contains(text, want) {
			t.Errorf("AccountDeleteText missing %q in:\\n%s", want, text)
		}
	}
	// Icon policy: body copy stays emoji-free (⚠️ must not appear).
	if strings.Contains(text, "⚠️") || strings.Contains(text, "❌") {
		t.Errorf("delete confirm body must be emoji-free (icon policy):\n%s", text)
	}
}

func TestAccountDeleteKeyboard_GivenID_ThenConfirmAndCancel(t *testing.T) {
	kb := AccountDeleteKeyboard(3)
	markup, ok := kb.(models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("keyboard type = %T", kb)
	}
	confirm := markup.InlineKeyboard[0][0]
	if confirm.Text != "Ya, Hapus" || confirm.CallbackData != "account:delete_confirm:3" {
		t.Errorf("confirm button = %+v, want Ya, Hapus → account:delete_confirm:3", confirm)
	}
	// v1.42: [Ya, Hapus, Batal] satu baris.
	cancel := markup.InlineKeyboard[0][1]
	if cancel.Text != "Batal" || cancel.CallbackData != "account:view:3" {
		t.Errorf("cancel button = %+v, want Batal → account:view:3", cancel)
	}
	// Icon policy: the destructive action button stays icon-free.
	if strings.Contains(confirm.Text, "✅") || strings.Contains(confirm.Text, "🗑️") {
		t.Errorf("confirm must be icon-free: %q", confirm.Text)
	}
}

func TestAccountDeletedText_GivenClient_ThenConfirmsRemoval(t *testing.T) {
	c := postgres.ClientView{VPNClient: postgres.VPNClient{Email: "del@vpn.kt"}, ServerName: "ID-01"}
	text := AccountDeletedText(c)
	if !strings.Contains(text, "del@vpn.kt") || !strings.Contains(text, "tidak bisa dipulihkan") {
		t.Errorf("deleted text = %q", text)
	}
}
