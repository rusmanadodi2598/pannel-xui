// Package telegram test covers the shop/topup/trial views (M7 hardening).
//
// @file      internal/service/telegram/menu_shop_test.go
// @for       Format & callback-data contract of buy/renew/accounts, topup and trial menus.
// @uses      testing, strings, time, github.com/go-telegram/bot/models, internal/domain,
// internal/repository/postgres, internal/service/topup
// @reason    UI copy & keyboard callbacks are product contract — format bugs
// (harga, tanggal) show up here first (M7 coverage gap).
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
	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	topupsvc "github.com/kentangtech/bot-order/internal/service/topup"
)

func plan(countryCode, countryName string, days int, price int64, enabled bool) domain.VpnPlan {
	p, err := domain.NewVpnPlan(countryCode, days, domain.Money(price), enabled)
	if err != nil {
		panic(err)
	}
	p.CountryName = countryName
	return *p
}

func TestBuyPlansKeyboard_GivenPlans_ThenPriceAndCallback(t *testing.T) {
	kb := BuyPlansKeyboard([]domain.VpnPlan{plan("ID", "Indonesia", 30, 7500, true)})
	markup, ok := kb.(models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("keyboard type = %T, want InlineKeyboardMarkup", kb)
	}
	btn := markup.InlineKeyboard[0][0]
	if !strings.Contains(btn.Text, "30 Hari") || !strings.Contains(btn.Text, "Rp 7.500") {
		t.Errorf("plan button text = %q, want price formatted", btn.Text)
	}
	if btn.CallbackData != "buy:plan:ID:30" {
		t.Errorf("callback = %q, want buy:plan:ID:30", btn.CallbackData)
	}
}

func TestBuyConfirmText_GivenInsufficientBalance_ThenWarnsWithoutBlocking(t *testing.T) {
	text := BuyConfirmText(plan("ID", "Indonesia", 30, 7500, true), domain.Money(5000))
	if !strings.Contains(text, "tidak cukup") || !strings.Contains(text, "Rp 5.000") {
		t.Errorf("text = %q, want insufficient-balance warning with formatted balance", text)
	}
}

func TestBuySuccessText_GivenOrder_ThenOrderIDAndBalance(t *testing.T) {
	text := BuySuccessText("KTS-1-VPN", "u@vpn.kt", 30, domain.Money(2500), "Indonesia")
	if !strings.Contains(text, "KTS-1-VPN") || !strings.Contains(text, "u@vpn.kt") ||
		!strings.Contains(text, "Rp 2.500") {
		t.Errorf("text = %q, want order details + balance", text)
	}
}

func TestRenewConfirmText_GivenClientAndPlan_ThenSummary(t *testing.T) {
	client := postgres.ClientView{}
	client.Email = "r@vpn.kt"
	text := RenewConfirmText(client, plan("SG", "Singapore", 30, 12000, true), domain.Money(20000))
	if !strings.Contains(text, "r@vpn.kt") || !strings.Contains(text, "Rp 12.000") {
		t.Errorf("text = %q, want client + plan price", text)
	}
}

func TestAccountsText_GivenExpiredAndActive_ThenStatuses(t *testing.T) {
	now := time.Now()
	expired := now.Add(-time.Hour)
	active := now.Add(5 * 24 * time.Hour) // deterministic: exactly 5 days → "sisa 6 hari" (int trunc +1)
	clients := []postgres.ClientView{
		{VPNClient: postgres.VPNClient{Email: "a@vpn.kt", Protocol: "vless", IsExpired: true}, ServerName: "ID-01"},
		{VPNClient: postgres.VPNClient{Email: "b@vpn.kt", Protocol: "trojan", ExpiresAt: &expired}, ServerName: "SG-01"},
		{VPNClient: postgres.VPNClient{Email: "c@vpn.kt", Protocol: "vmess", ExpiresAt: &active}, ServerName: "JP-01"},
	}
	text := AccountsText(clients, now)
	for _, want := range []string{"Akun Saya", "ID-01", "Expired", "sisa 6 hari", "VLESS", "TROJAN"} {
		if !strings.Contains(text, want) {
			t.Errorf("AccountsText missing %q in:\n%s", want, text)
		}
	}
}

func TestAccountsText_GivenNoClients_ThenEmptyPrompt(t *testing.T) {
	if text := AccountsText(nil, time.Now()); !strings.Contains(text, "belum punya akun") {
		t.Errorf("empty text = %q, want friendly prompt", text)
	}
}

func TestTopupKeyboard_GivenQuickPicks_ThenTwoPerRowAndCustom(t *testing.T) {
	kb := TopupKeyboard()
	markup, ok := kb.(models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("keyboard type = %T", kb)
	}
	rows := markup.InlineKeyboard
	if len(rows) != 5 { // 3 baris quick-pick (6 nilai) + custom + back
		t.Fatalf("rows = %d, want 5", len(rows))
	}
	if rows[0][0].CallbackData != "topup:amount:10000" || rows[0][1].CallbackData != "topup:amount:25000" {
		t.Errorf("first row callbacks = %q, %q", rows[0][0].CallbackData, rows[0][1].CallbackData)
	}
	if rows[3][0].CallbackData != "topup:custom" {
		t.Errorf("custom row = %q, want topup:custom", rows[3][0].CallbackData)
	}
}

func TestTopupSummaryText_GivenQuote_ThenNetGrossAndFee(t *testing.T) {
	q := topupsvc.Quote{Net: domain.Money(10000), Gross: domain.Money(11400), TotalFee: domain.Money(1400), FeePercent: 0.14}
	text := TopupSummaryText(q, domain.Money(5000))
	for _, want := range []string{"Rp 10.000", "Rp 11.400", "Rp 1.400"} {
		if !strings.Contains(text, want) {
			t.Errorf("summary missing %q in:\n%s", want, text)
		}
	}
}

func TestTrialMenuText_GivenQuota_ThenShowsRemaining(t *testing.T) {
	text := TrialMenuText(2, 1, 1)
	if !strings.Contains(text, "Sisa kesempatan trial hari ini: 2") || !strings.Contains(text, "1 jam") {
		t.Errorf("trial menu = %q, want remaining + duration", text)
	}
}

func TestTrialConfirmKeyboard_GivenServer_ThenConfirmCallback(t *testing.T) {
	kb := TrialConfirmKeyboard(7)
	markup, _ := kb.(models.InlineKeyboardMarkup)
	if markup.InlineKeyboard[0][0].CallbackData != "trial:confirm:7" {
		t.Errorf("callback = %q, want trial:confirm:7", markup.InlineKeyboard[0][0].CallbackData)
	}
}

func TestTrialSuccessText_GivenResult_ThenOrderAndRemaining(t *testing.T) {
	text := TrialSuccessText("KTS-T1", "t@vpn.kt", "SG-01", 1)
	if !strings.Contains(text, "KTS-T1") || !strings.Contains(text, "Sisa trial hari ini: 1") {
		t.Errorf("trial success = %q", text)
	}
}

func TestFlagOrGlobe_GivenEmptyFlag_ThenGlobe(t *testing.T) {
	if got := flagOrGlobe(""); got != "🌐" {
		t.Errorf("flagOrGlobe(\"\") = %q, want globe", got)
	}
	if got := flagOrGlobe("🇮🇩"); got != "🇮🇩" {
		t.Errorf("flagOrGlobe(flag) = %q, want passthrough", got)
	}
}

func TestShortEmail_GivenLongEmail_ThenTruncated(t *testing.T) {
	if got := shortEmail("very-long-email-address@vpn.kt"); got != "very-long-…" {
		t.Errorf("shortEmail = %q, want truncated with ellipsis", got)
	}
	if got := shortEmail("short@vpn.kt"); got != "short@vpn.kt" {
		t.Errorf("shortEmail(short) = %q, want passthrough", got)
	}
}
