// Package telegram also hosts the shop (buy/renew/accounts) views.
//
// @file      internal/service/telegram/menu_shop.go
// @for       FR-03/FR-05/FR-08 keyboards + copy for the auto-order flows (M4).
// @uses      fmt, strings, github.com/go-telegram/bot/models, internal/domain, internal/repository/postgres
// @reason    Pure presentation; keeps handler code network-free and testable.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// RenewClientsKeyboard lists the user's active accounts for renewal (FR-05,
// 2-1-2-1 zigzag).
func RenewClientsKeyboard(clients []postgres.ClientView) models.ReplyMarkup {
	buttons := make([]models.InlineKeyboardButton, 0, len(clients)+1)
	for _, c := range clients {
		buttons = append(buttons, models.InlineKeyboardButton{
			Text:         fmt.Sprintf("%s %s — %s", flagOrGlobe(c.FlagEmoji), c.ServerName, shortEmail(c.Email)),
			CallbackData: PrefixRenewClient + fmt.Sprintf("%d", c.ID),
		})
	}
	buttons = append(buttons, backBtn(CallbackRenewBack, "🏠 Menu Utama"))
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(buttons...)}
}

// RenewPlanKeyboard reuses the plan list, encoded with the client id prefix.
func RenewPlanKeyboard(clientID int64, plans []domain.VpnPlan) models.ReplyMarkup {
	buttons := make([]models.InlineKeyboardButton, 0, len(plans)+1)
	for _, p := range plans {
		buttons = append(buttons, models.InlineKeyboardButton{
			Text: fmt.Sprintf("%d Hari — %s", p.Days, p.Price.FormatIDR()),
			CallbackData: PrefixRenewPlan + fmt.Sprintf("%d", clientID) + ":" +
				p.CountryCode + ":" + fmt.Sprintf("%d", p.Days),
		})
	}
	buttons = append(buttons, backBtn(CallbackRenewBack, "⬅️ Kembali"))
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(buttons...)}
}

// RenewConfirmKeyboard asks explicit confirmation for a renewal.
func RenewConfirmKeyboard(clientID int64, country string, days int) models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(
		models.InlineKeyboardButton{Text: "Konfirmasi Perpanjang", CallbackData: PrefixRenewConfirm +
			fmt.Sprintf("%d", clientID) + ":" + country + ":" + fmt.Sprintf("%d", days)},
		backBtn(CallbackRenewBack, "⬅️ Kembali"),
	)}
}

// backRow is a single centered back/home button (slice shape for fixed-row
// keyboards; backBtn is the single-button shape for the zigzag packer).
func backRow(callback, label string) []models.InlineKeyboardButton {
	return []models.InlineKeyboardButton{backBtn(callback, label)}
}

// BuySuccessText reports a completed purchase (FR-04 AC-2, branded banner
// v1.43). The import URL is intentionally NOT rendered here (v1.36) — it
// lives only in the .txt export, which the keyboard below offers right after
// this message.
func BuySuccessText(orderID, email string, days int, balance domain.Money, countryName string) string {
	return fmt.Sprintf(BrandHeader()+"\n\nOrder Berhasil\n━━━━━━━━━━━━━━\n"+
		"Order ID: %s\n"+
		"Server: %s\n"+
		"Masa aktif: %d Hari\n"+
		"Email akun: %s\n"+
		"Sisa saldo: %s\n━━━━━━━━━━━━━━\n\n%s",
		orderID, countryName, days, email, balance.FormatIDR(), exportHintText)
}

// BuySuccessKeyboard offers the v2Ray config view, export + home after a
// purchase (M7 + v1.26, 2-1-2-1 zigzag).
func BuySuccessKeyboard(clientID int64) models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(
		models.InlineKeyboardButton{Text: "Config V2Ray", CallbackData: PrefixAccountConfig + fmt.Sprintf("%d", clientID)},
		models.InlineKeyboardButton{Text: "Ekspor .txt", CallbackData: PrefixAccountExport + fmt.Sprintf("%d", clientID)},
		backBtn(CallbackHome, "🏠 Menu Utama"),
	)}
}

// BuyFailedText reports a failed purchase with a friendly reason (branded
// banner v1.44).
func BuyFailedText(orderID, reason string) string {
	return fmt.Sprintf(BrandHeader()+"\n\nOrder Gagal\n━━━━━━━━━━━━━━\nOrder ID: %s\n\n%s\n\n"+
		"Saldo kamu tidak dipotong. Silakan coba lagi.", orderID, reason)
}

// RenewNoClientsText is shown when the user has no active accounts.
func RenewNoClientsText() string {
	return "Kamu belum punya akun VPN aktif.\n\nSilakan beli dulu di menu Beli VPN."
}

// RenewClientListText introduces the account picker.
func RenewClientListText() string {
	return "Pilih akun yang mau diperpanjang:\n\nMasa aktif akan ditambahkan dari sisa waktu akun."
}

// RenewConfirmText summarizes a renewal before confirmation (FR-05 AC,
// branded banner v1.44).
func RenewConfirmText(client postgres.ClientView, plan domain.VpnPlan, balance domain.Money) string {
	var warn string
	if balance < plan.Price {
		warn = fmt.Sprintf("\n\nSaldo kamu %s — tidak cukup. Silakan top up dulu ya.", balance.FormatIDR())
	} else {
		warn = fmt.Sprintf("\n\nSaldo kamu: %s (cukup)", balance.FormatIDR())
	}
	return fmt.Sprintf(BrandHeader()+"\n\nRingkasan Perpanjangan\n━━━━━━━━━━━━━━\n"+
		"Akun: %s\n"+
		"Paket: %d Hari\n"+
		"Harga: %s\n━━━━━━━━━━━━━━%s\n\n"+
		"Tekan Konfirmasi Perpanjang untuk melanjutkan.",
		client.Email, plan.Days, plan.Price.FormatIDR(), warn)
}

// RenewSuccessText reports a completed renewal with the new expiry (branded
// banner v1.43).
func RenewSuccessText(orderID, email string, days int, newExpiry time.Time, balance domain.Money) string {
	return fmt.Sprintf(BrandHeader()+"\n\nPerpanjangan Berhasil\n━━━━━━━━━━━━━━\n"+
		"Order ID: %s\n"+
		"Akun: %s\n"+
		"Masa aktif bertambah %d Hari\n"+
		"Aktif sampai: %s\n"+
		"Sisa saldo: %s\n━━━━━━━━━━━━━━",
		orderID, email, days, newExpiry.Format("02 Jan 2006"), balance.FormatIDR())
}

// AccountsText renders one page of the FR-08 account list (5/page, newest
// first). Empty state prompts a purchase; the header shows the current page.
func AccountsText(clients []postgres.ClientView, page, totalPages int, now time.Time) string {
	if len(clients) == 0 {
		return "Kamu belum punya akun VPN.\n\nSilakan beli dulu di menu Beli VPN."
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Akun Saya\n━━━━━━━━━━━━━━\nHalaman %d dari %d\n", page, totalPages))
	for i, c := range clients {
		b.WriteString(fmt.Sprintf("\n%d. %s\n", i+1, c.ServerName))
		b.WriteString(fmt.Sprintf("   %s\n", c.Email))
		b.WriteString(fmt.Sprintf("   %s\n", strings.ToUpper(c.Protocol)))
		badge := ""
		if c.IsTrial {
			badge = "Trial · " // badge teks polos (icon policy) — parity AC-1 tanpa emoji
		}
		b.WriteString(fmt.Sprintf("   %s%s\n", badge, accountListStatus(c, now)))
		if c.ExpiresAt != nil {
			b.WriteString("   " + accountRemaining(c, now) + "\n")
		}
	}
	b.WriteString("━━━━━━━━━━━━━━\n\nPilih Detail untuk info akun & ekspor config (.txt).")
	return b.String()
}

// accountListStatus derives the FR-08 AC-1 status label for one list item
// (parity reference ✅/⚠️/❌ sebagai teks polos — icon policy): Expired →
// "Expired"; non-aktif (mis. dinonaktifkan / kuota habis) → "Hampir Habis";
// aktif dengan kuota ≥90% (threshold AC-3) → "Hampir Habis"; else "Aktif".
func accountListStatus(c postgres.ClientView, now time.Time) string {
	if c.IsExpired || (c.ExpiresAt != nil && !c.ExpiresAt.After(now)) {
		return "Expired"
	}
	if !c.IsActive {
		return "Hampir Habis"
	}
	// Kuota ≥90% (parity AC-3): bandingkan persen, bukan label — perubahan
	// copy halaman traffic tidak boleh mengubah perilaku list (anti-coupling).
	if _, _, pct := trafficStatus(c.TrafficUsed, c.TrafficLimit); pct >= 90 {
		return "Hampir Habis"
	}
	return "Aktif"
}

// accountRemaining renders the smart remaining time (FR-08 AC-1
// time_remaining_display parity): jam untuk sisa < 24 jam (trial 1 jam),
// hari untuk sisa lebih panjang — sama dengan format "sisa N hari" lama.
func accountRemaining(c postgres.ClientView, now time.Time) string {
	if c.ExpiresAt == nil {
		return "" // defensive: caller sudah guard, helper tetap aman berdiri sendiri
	}
	remain := c.ExpiresAt.Sub(now)
	if remain <= 0 {
		return "sudah habis"
	}
	if remain < 24*time.Hour {
		h := int(remain.Hours())
		if h < 1 {
			h = 1
		}
		return fmt.Sprintf("sisa %d jam", h)
	}
	return fmt.Sprintf("sisa %d hari", int(remain.Hours()/24)+1)
}

func flagOrGlobe(flag string) string {
	if strings.TrimSpace(flag) != "" {
		return flag
	}
	return "🌐"
}

func shortEmail(email string) string {
	if len(email) <= 24 {
		return email
	}
	return email[:10] + "…"
}
