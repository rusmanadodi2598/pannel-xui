// Package telegram test covers the FR-15 static help & ToS views.
//
// @file      internal/service/telegram/menu_help_test.go
// @for       Golden assertions: help callback-data contract, layout, icon policy.
// @uses      testing, strings, github.com/go-telegram/bot/models
// @reason    Locks the FR-15 callback contract (help:*) and the icon policy
// (icons only on navigation buttons) so future handlers rely on it.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestHelpMenuKeyboard_GivenLayout_ThenCategoriesPlusHome(t *testing.T) {
	kb, ok := HelpMenuKeyboard().(models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("HelpMenuKeyboard must return InlineKeyboardMarkup")
	}
	var got []string
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			got = append(got, btn.Text+"="+btn.CallbackData)
		}
	}
	want := []string{
		"Cara Order=help:order",
		"Cara Top Up=help:topup",
		"Disclaimer & ToS=help:disclaimer",
		"Info=help:info",
		"🏠 Menu Utama=menu:home",
	}
	if len(got) != len(want) {
		t.Fatalf("buttons = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("button[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestHelpPageKeyboards_GivenPages_ThenBackToHelpAndHome(t *testing.T) {
	pages := map[string]models.ReplyMarkup{
		"order":  HelpOrderKeyboard(),
		"topup":  HelpTopupKeyboard(),
		"disc":   HelpDisclaimerKeyboard(),
		"tosacc": HelpTosAccountKeyboard(),
		"tospay": HelpTosPaymentKeyboard(),
		"info":   HelpInfoKeyboard(),
	}
	for name, markup := range pages {
		kb, ok := markup.(models.InlineKeyboardMarkup)
		if !ok {
			t.Errorf("%s keyboard must be InlineKeyboardMarkup", name)
			continue
		}
		var cbs []string
		for _, row := range kb.InlineKeyboard {
			for _, btn := range row {
				cbs = append(cbs, btn.CallbackData)
			}
		}
		joined := strings.Join(cbs, "|")
		if !strings.Contains(joined, CallbackHome) {
			t.Errorf("%s keyboard missing home button: %v", name, cbs)
		}
	}
}

func TestHelpTexts_GivenBuilders_ThenNonEmpty(t *testing.T) {
	for name, text := range map[string]string{
		"menu":        HelpMenuText(),
		"order":       HelpOrderText(),
		"topup":       HelpTopupText(),
		"disclaimer":  HelpDisclaimerText(),
		"tos_account": HelpTosAccountText(),
		"tos_payment": HelpTosPaymentText(),
		"info":        HelpInfoText(),
	} {
		if strings.TrimSpace(text) == "" {
			t.Errorf("%s text is empty", name)
		}
	}
}

func TestHelpTexts_GivenContent_ThenNoEmojiInBody(t *testing.T) {
	// Icon policy (v1.24): icons only on navigation buttons — body copy is plain.
	bodies := []string{
		HelpMenuText(), HelpOrderText(), HelpTopupText(),
		HelpDisclaimerText(), HelpTosAccountText(), HelpTosPaymentText(), HelpInfoText(),
	}
	for _, body := range bodies {
		for _, icon := range []string{"🛒", "💳", "📜", "ℹ️", "✅", "1️⃣", "🚫", "💰", "🛡️", "⚡", "📋", "🤖", "👥", "📢", "📌", "🔗", "🟢", "🟡", "🔴", "🔵", "🟣", "⚠️", "💡", "❓"} {
			if strings.Contains(body, icon) {
				t.Errorf("body must not contain emoji %q (icon policy)", icon)
			}
		}
	}
}

func TestHelpShortcuts_GivenPage_ThenActionButtonTextOnly(t *testing.T) {
	// Icon policy: action buttons are text-only — no emoji on shortcut buttons.
	cases := map[string]models.ReplyMarkup{
		"order": HelpOrderKeyboard(),
		"topup": HelpTopupKeyboard(),
	}
	for name, markup := range cases {
		kb := markup.(models.InlineKeyboardMarkup)
		first := kb.InlineKeyboard[0][0]
		if strings.Contains(first.Text, "🏠") || strings.Contains(first.Text, "⬅️") {
			t.Errorf("%s shortcut button must be text-only, got %q", name, first.Text)
		}
		if first.CallbackData != CallbackBuy && first.CallbackData != CallbackTopup {
			t.Errorf("%s shortcut callback = %q, want buy/topup", name, first.CallbackData)
		}
	}
}

func TestHelpTosKeyboards_GivenLayout_ThenCrossNavTextOnly(t *testing.T) {
	// Icon policy: cross-nav action buttons (Ketentuan Akun/Pembayaran) are
	// text-only; only back/home navigation buttons carry icons.
	kb := HelpTosAccountKeyboard().(models.InlineKeyboardMarkup)
	cross := kb.InlineKeyboard[0][0]
	if cross.Text != "Ketentuan Pembayaran" || cross.CallbackData != CallbackHelpTosPayment {
		t.Errorf("tos:account cross-nav = %+v", cross)
	}
	kb = HelpTosPaymentKeyboard().(models.InlineKeyboardMarkup)
	cross = kb.InlineKeyboard[0][0]
	if cross.Text != "Ketentuan Akun" || cross.CallbackData != CallbackHelpTosAccount {
		t.Errorf("tos:payment cross-nav = %+v", cross)
	}
}

func TestHelpPageKeyboards_GivenPages_ThenBackTargetPerPage(t *testing.T) {
	// FR-15 AC parity: each page's back button targets its parent page.
	cases := map[string]struct {
		keyboard  models.ReplyMarkup
		backLabel string
		backCb    string
	}{
		"order":       {HelpOrderKeyboard(), "⬅️ Kembali", CallbackHelp},
		"topup":       {HelpTopupKeyboard(), "⬅️ Kembali", CallbackHelp},
		"disclaimer":  {HelpDisclaimerKeyboard(), "⬅️ Kembali", CallbackHelp},
		"tos:account": {HelpTosAccountKeyboard(), "⬅️ Kembali", CallbackHelpDisclaimer},
		"tos:payment": {HelpTosPaymentKeyboard(), "⬅️ Kembali", CallbackHelpDisclaimer},
		"info":        {HelpInfoKeyboard(), "⬅️ Kembali", CallbackHelp},
	}
	for name, tc := range cases {
		kb := tc.keyboard.(models.InlineKeyboardMarkup)
		rows := kb.InlineKeyboard
		back := rows[len(rows)-1][0]
		if back.Text != tc.backLabel || back.CallbackData != tc.backCb {
			t.Errorf("%s: back row = %+v, want %q → %q", name, back, tc.backLabel, tc.backCb)
		}
		home := rows[len(rows)-1][1]
		if home.CallbackData != CallbackHome {
			t.Errorf("%s: home button = %+v", name, home)
		}
	}
}
