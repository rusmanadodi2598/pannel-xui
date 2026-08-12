// Package telegram test covers the FR-02 menu keyboard and copy builders.
//
// @file      internal/service/telegram/menu_test.go
// @for       Golden assertions: 7 menu buttons with exact callback data, join keyboard, texts.
// @uses      testing, strings, github.com/go-telegram/bot/models
// @reason    Locks the callback-data contract bot handlers and future features rely on.
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

func TestHomeKeyboard_GivenLayout_ThenSevenButtonsInOrder(t *testing.T) {
	kb, ok := HomeKeyboard().(models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("HomeKeyboard must return InlineKeyboardMarkup")
	}
	var got []string
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			got = append(got, btn.Text+"="+btn.CallbackData)
		}
	}
	want := []string{
		"Beli VPN=buy:menu",
		"Perpanjang=renew:menu",
		"Akun Saya=account:menu",
		"Top Up=topup:menu",
		"Trial=trial:menu",
		"Riwayat=history:menu",
		"Bantuan=help:menu",
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

func TestJoinKeyboard_GivenLink_ThenLinkAndRecheckButtons(t *testing.T) {
	kb, ok := JoinKeyboard("https://t.me/kentangtech").(models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("JoinKeyboard must return InlineKeyboardMarkup")
	}
	link := kb.InlineKeyboard[0][0]
	if link.Text != "Join Grup ↗" || link.URL != "https://t.me/kentangtech" {
		t.Errorf("link button = %+v", link)
	}
	recheck := kb.InlineKeyboard[1][0]
	if recheck.Text != "Sudah Join ✓" || recheck.CallbackData != CallbackGateCheck {
		t.Errorf("recheck button = %+v", recheck)
	}
}

func TestJoinKeyboard_GivenEmptyLink_ThenOnlyRecheckButton(t *testing.T) {
	kb, ok := JoinKeyboard("").(models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("JoinKeyboard must return InlineKeyboardMarkup")
	}
	if len(kb.InlineKeyboard) != 1 {
		t.Fatalf("rows = %d, want 1 (no URL button without a link)", len(kb.InlineKeyboard))
	}
	btn := kb.InlineKeyboard[0][0]
	if btn.CallbackData != CallbackGateCheck || btn.URL != "" {
		t.Errorf("button = %+v, want only the re-check button", btn)
	}
}

func TestHomeText_GivenName_ThenGreets(t *testing.T) {
	text := HomeText("Dodi")
	if !strings.Contains(text, "Dodi") || !strings.Contains(text, BrandName) {
		t.Errorf("HomeText missing greeting/brand: %s", text)
	}
}

func TestJoinText_GivenLink_ThenMentionsLink(t *testing.T) {
	text := JoinText("https://t.me/kentangtech")
	if !strings.Contains(text, "https://t.me/kentangtech") {
		t.Errorf("JoinText missing link: %s", text)
	}
}

func TestTexts_GivenBuilders_ThenNonEmpty(t *testing.T) {
	for name, text := range map[string]string{
		"banned":    BannedText(),
		"ratelimit": RateLimitText(),
		"unavail":   UnavailableText(),
		"help_hint": HelpHintText(),
	} {
		if strings.TrimSpace(text) == "" {
			t.Errorf("%s text is empty", name)
		}
	}
}
