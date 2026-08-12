// Package telegram test covers the shared brand banner (v1.43).
//
// @file      internal/service/telegram/menu_brand_test.go
// @for       Brand contract: every transactional notification carries the
// KENTANG TECH banner (and never the legacy KENTANG TECH STORE).
// @uses      testing, strings, time, internal/domain, internal/repository/postgres
// @reason    The brand is a product decision (v1.43) — a regression would drop
// the banner from notifications silently (parity legacy reference).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-12
package telegram

import (
	"strings"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	topupsvc "github.com/kentangtech/bot-order/internal/service/topup"
)

func TestBrandHeader_ThenIconNameAndSeparator(t *testing.T) {
	header := BrandHeader()
	for _, want := range []string{"🏪", "KENTANG TECH", "━━━━━━━━━━━━━━"} {
		if !strings.Contains(header, want) {
			t.Errorf("BrandHeader missing %q in: %q", want, header)
		}
	}
	// The new brand is explicitly NOT the legacy "KENTANG TECH STORE".
	if strings.Contains(header, "STORE") {
		t.Errorf("BrandHeader must not carry the legacy 'STORE' suffix: %q", header)
	}
	if BrandName != "KENTANG TECH" {
		t.Errorf("BrandName = %q, want KENTANG TECH", BrandName)
	}
}

func serverViewFixture() postgres.ServerView {
	return postgres.ServerView{Name: "ID-01", FlagEmoji: "🇮🇩"}
}

// TestTransactionalTemplates_ThenBrandBannerPresent locks the v1.43/v1.44
// contract (extended with confirm summaries + failure texts in v1.44):
// every transactional message opens with the brand banner, and never with the
// legacy "KENTANG TECH STORE" name.
func TestTransactionalTemplates_ThenBrandBannerPresent(t *testing.T) {
	now := time.Now()
	client := postgres.ClientView{}
	client.Email = "r@vpn.kt"
	cases := []struct {
		name string
		text string
	}{
		{"BuySuccessText", BuySuccessText("KTS-1-VPN", "u@vpn.kt", 30, domain.Money(2500), "Indonesia")},
		{"RenewSuccessText", RenewSuccessText("KTS-2-VPN", "r@vpn.kt", 30, now.Add(30*24*time.Hour), domain.Money(20000))},
		{"TrialSuccessText", TrialSuccessText("KTS-T1", "t@vpn.kt", "SG-01", 1)},
		{"TopupSummaryText", TopupSummaryText(topupsvc.Quote{Net: domain.Money(10000), Gross: domain.Money(11400), TotalFee: domain.Money(1400), FeePercent: 0.14}, domain.Money(5000))},
		{"ExpiryNotifyText", ExpiryNotifyText(3, "ID-01", "a@vpn.kt", "14 Agu 2026")},
		{"AdminOrderNoticeText", AdminOrderNoticeText("KTS-3-VPN", domain.OrderTypePurchase, "Budi", "ID 30 Hari",
			"u@vpn.kt", domain.Money(7000), domain.Money(43000), now)},
		// v1.44: ringkasan konfirmasi + pesan gagal juga ber-brand (konsistensi).
		{"BuyConfirmText", BuyConfirmText(plan("ID", "Indonesia", 30, 7500, true), domain.Money(5000), "vless")},
		{"RenewConfirmText", RenewConfirmText(client, plan("SG", "Singapore", 30, 12000, true), domain.Money(20000))},
		{"TrialConfirmText", TrialConfirmText(serverViewFixture(), 1, 1, 1, "vless")},
		{"BuyFailedText", BuyFailedText("KTS-F1", "Panel timeout")},
		{"TrialFailedText", TrialFailedText()},
	}
	for _, tc := range cases {
		if !strings.HasPrefix(tc.text, "🏪 KENTANG TECH") {
			t.Errorf("%s must open with the brand banner:\n%s", tc.name, tc.text)
		}
		if !strings.Contains(tc.text, "━━━━━━━━━━━━━━") {
			t.Errorf("%s missing separator:\n%s", tc.name, tc.text)
		}
		if strings.Contains(tc.text, "KENTANG TECH STORE") {
			t.Errorf("%s must not use the legacy brand:\n%s", tc.name, tc.text)
		}
	}
}

// TestBrandSpelling_ThenSingleBrandEverywhere locks v1.44 consistency: the
// .txt export header, the /start greeting and the help:info page use the
// BrandName spelling (never the old "KentangTech"/"KENTANGTECH" shapes).
func TestBrandSpelling_ThenSingleBrandEverywhere(t *testing.T) {
	expiry := time.Now().Add(24 * time.Hour)
	c := postgres.ClientView{
		VPNClient:  postgres.VPNClient{Email: "a@vpn.kt", Protocol: "vless", UUID: "u1", ExpiresAt: &expiry},
		ServerName: "ID-01",
	}
	bodies := map[string]string{
		"AccountTXTContent": AccountTXTContent(c, time.Now()),
		"HomeText":          HomeText("Dodi"),
		"HelpInfoText":      HelpInfoText(),
	}
	for name, body := range bodies {
		if !strings.Contains(body, BrandName) {
			t.Errorf("%s must carry the brand spelling %q:\n%s", name, BrandName, body)
		}
		for _, old := range []string{"KentangTech", "KENTANGTECH", "Kentang Tech Store", "KENTANG TECH STORE"} {
			if strings.Contains(body, old) {
				t.Errorf("%s must not use legacy spelling %q:\n%s", name, old, body)
			}
		}
	}
}
