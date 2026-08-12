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
	serversvc "github.com/kentangtech/bot-order/internal/service/server"
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
	kb := BuyPlansKeyboard([]domain.VpnPlan{plan("ID", "Indonesia", 30, 7500, true)}, 3, 9, "vless")
	markup, ok := kb.(models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("keyboard type = %T, want InlineKeyboardMarkup", kb)
	}
	btn := markup.InlineKeyboard[0][0]
	if !strings.Contains(btn.Text, "30 Hari") || !strings.Contains(btn.Text, "Rp 7.500") {
		t.Errorf("plan button text = %q, want price formatted", btn.Text)
	}
	if btn.CallbackData != "buy:plan:3:9:ID:30" {
		t.Errorf("callback = %q, want buy:plan:3:9:ID:30 (server+inbound pinned)", btn.CallbackData)
	}
}

func TestBuyInboundsKeyboard_GivenOptions_ThenProtocolAndCallback(t *testing.T) {
	kb := BuyInboundsKeyboard([]serversvc.InboundOption{
		{ServerID: 3, ServerName: "ID-01", Country: "ID", InboundID: 9, Protocol: "vless", Remark: "reality"},
	})
	markup, ok := kb.(models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("keyboard type = %T", kb)
	}
	btn := markup.InlineKeyboard[0][0]
	if !strings.Contains(btn.Text, "ID-01") || !strings.Contains(btn.Text, "VLESS") || !strings.Contains(btn.Text, "reality") {
		t.Errorf("inbound button text = %q, want server+protocol+remark", btn.Text)
	}
	if btn.CallbackData != "buy:inbound:3:9:ID" {
		t.Errorf("callback = %q, want buy:inbound:3:9:ID", btn.CallbackData)
	}
}

func TestBuyConfirmText_GivenProtocol_ThenShowsProtocol(t *testing.T) {
	text := BuyConfirmText(plan("ID", "Indonesia", 30, 7500, true), domain.Money(5000), "trojan")
	if !strings.Contains(text, "Protocol: TROJAN") {
		t.Errorf("text = %q, want protocol badge", text)
	}
}

func TestBuyConfirmText_GivenInsufficientBalance_ThenWarnsWithoutBlocking(t *testing.T) {
	text := BuyConfirmText(plan("ID", "Indonesia", 30, 7500, true), domain.Money(5000), "vless")
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
	// v1.36: URL import TIDAK dirender di pesan sukses — hanya hint ekspor.
	if strings.Contains(text, "vless://") || strings.Contains(text, "Config Link:") {
		t.Errorf("success text must not render the import URL (v1.36):\n%s", text)
	}
	if !strings.Contains(text, "Ekspor .txt") {
		t.Errorf("success text missing export hint:\n%s", text)
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
	text := AccountsText(clients, 1, 1, now)
	for _, want := range []string{"Akun Saya", "Halaman 1 dari 1", "ID-01", "Expired", "sisa 6 hari", "VLESS", "TROJAN"} {
		if !strings.Contains(text, want) {
			t.Errorf("AccountsText missing %q in:\n%s", want, text)
		}
	}
}

func TestAccountsText_GivenNoClients_ThenEmptyPrompt(t *testing.T) {
	if text := AccountsText(nil, 1, 1, time.Now()); !strings.Contains(text, "belum punya akun") {
		t.Errorf("empty text = %q, want friendly prompt", text)
	}
}

func TestAccountDetailText_GivenIPLimitAndTraffic_ThenShown(t *testing.T) {
	expiry := time.Now().Add(5 * 24 * time.Hour)
	c := postgres.ClientView{
		VPNClient: postgres.VPNClient{
			Email: "d@vpn.kt", Protocol: "vless", UUID: "u1", IPLimit: 2,
			TrafficUsed: 1 << 30, TrafficLimit: 100 << 30, ExpiresAt: &expiry,
			ConfigLink: "vless://u1@h:443",
		},
		ServerName: "ID-01",
	}
	text := AccountDetailText(c, time.Now())
	for _, want := range []string{"Limit IP: 2", "Traffic Terpakai: 1.00 GB", "Kuota: 100 GB"} {
		if !strings.Contains(text, want) {
			t.Errorf("detail text missing %q in:\n%s", want, text)
		}
	}
	// v1.36: vless/vmess menampilkan UUID (bukan Password) dan URL config
	// hasil build TIDAK lagi tampil di detail — cukup via Ekspor .txt.
	if !strings.Contains(text, "UUID: u1") {
		t.Errorf("detail text missing UUID credential:\n%s", text)
	}
	if strings.Contains(text, "Password") || strings.Contains(text, "Config Link") {
		t.Errorf("detail must not show Password line or built Config Link (v1.36):\n%s", text)
	}
	if !strings.Contains(text, "Ekspor .txt") {
		t.Errorf("detail text missing export hint:\n%s", text)
	}
}

func TestAccountDetailText_GivenTrojan_ThenPasswordCredentialOnly(t *testing.T) {
	expiry := time.Now().Add(3 * 24 * time.Hour)
	c := postgres.ClientView{
		VPNClient: postgres.VPNClient{
			Email: "t@vpn.kt", Protocol: "trojan", UUID: "", Password: "pw-7",
			IPLimit: 1, ExpiresAt: &expiry,
		},
		ServerName: "SG-01",
	}
	text := AccountDetailText(c, time.Now())
	if !strings.Contains(text, "Password: pw-7") {
		t.Errorf("trojan detail must show Password credential:\n%s", text)
	}
	if strings.Contains(text, "UUID") || strings.Contains(text, "Config Link") {
		t.Errorf("trojan detail must not show UUID or built Config Link (v1.36):\n%s", text)
	}
}

func TestAccountTXTContent_GivenIPLimitAndTraffic_ThenShown(t *testing.T) {
	c := postgres.ClientView{
		VPNClient: postgres.VPNClient{
			Email: "d@vpn.kt", Protocol: "vless", UUID: "u1", IPLimit: 3,
			TrafficUsed: 2 << 30, TrafficLimit: 100 << 30,
		},
	}
	content := AccountTXTContent(c, time.Now())
	for _, want := range []string{"Limit IP    : 3", "Traffic     : 2.00 GB / 100 GB", "UUID        : u1"} {
		if !strings.Contains(content, want) {
			t.Errorf("txt content missing %q in:\n%s", want, content)
		}
	}
	// v1.36: ekspor memakai label protocol-aware — vless = UUID saja.
	if strings.Contains(content, "Password") {
		t.Errorf("vless export must not show Password line:\n%s", content)
	}
}

func TestAccountListKeyboard_GivenPage_ThenPagerAndDetail(t *testing.T) {
	clients := []postgres.ClientView{
		{VPNClient: postgres.VPNClient{ID: 1, Email: "a@vpn.kt"}},
		{VPNClient: postgres.VPNClient{ID: 2, Email: "b@vpn.kt"}},
	}
	kb, ok := AccountListKeyboard(clients, 2, 3).(models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("keyboard type = %T", kb)
	}
	rows := kb.InlineKeyboard
	// v1.42: 2 detail → 1 baris zigzag [view:1, view:2] + pager + home.
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 (zigzag detail + pager + home)", len(rows))
	}
	if rows[0][0].CallbackData != "account:view:1" || rows[0][1].CallbackData != "account:view:2" {
		t.Errorf("detail buttons = %+v", rows[0])
	}
	pager := rows[1]
	if pager[0].CallbackData != "account:page:1" || pager[1].CallbackData != CallbackAccountNoop ||
		pager[2].CallbackData != "account:page:3" {
		t.Errorf("pager row = %+v", pager)
	}
	if pager[1].Text != "2/3" {
		t.Errorf("page indicator = %q, want 2/3", pager[1].Text)
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

func TestTrialConfirmKeyboard_GivenServerAndInbound_ThenConfirmCallback(t *testing.T) {
	kb := TrialConfirmKeyboard(7, 4)
	markup, _ := kb.(models.InlineKeyboardMarkup)
	if markup.InlineKeyboard[0][0].CallbackData != "trial:confirm:7:4" {
		t.Errorf("callback = %q, want trial:confirm:7:4 (server+inbound pinned)", markup.InlineKeyboard[0][0].CallbackData)
	}
	if strings.Contains(markup.InlineKeyboard[0][0].Text, "✅") {
		t.Errorf("confirm button must be icon-free (icon policy), got %q", markup.InlineKeyboard[0][0].Text)
	}
}

func TestTrialSuccessText_GivenResult_ThenOrderAndRemaining(t *testing.T) {
	text := TrialSuccessText("KTS-T1", "t@vpn.kt", "SG-01", 1)
	if !strings.Contains(text, "KTS-T1") || !strings.Contains(text, "Sisa trial hari ini: 1") {
		t.Errorf("trial success = %q", text)
	}
	// v1.36: URL import TIDAK dirender di pesan sukses trial — hanya hint ekspor.
	if strings.Contains(text, "vless://") || strings.Contains(text, "Config Link:") {
		t.Errorf("trial success text must not render the import URL (v1.36):\n%s", text)
	}
	if !strings.Contains(text, "Ekspor .txt") {
		t.Errorf("trial success text missing export hint:\n%s", text)
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
