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

// Shop callback data contract (prefix + payload, M4).
const (
	CallbackBuyBack   = "buy:back"
	CallbackRenewBack = "renew:back"

	// Prefixes for parameterised callbacks (exported for the handler layer).
	PrefixBuyCountry   = "buy:country:"
	PrefixBuyPlan      = "buy:plan:"
	PrefixBuyConfirm   = "buy:confirm:"
	PrefixRenewClient  = "renew:client:"
	PrefixRenewPlan    = "renew:plan:"
	PrefixRenewConfirm = "renew:confirm:"
	PrefixAccountView  = "account:view:"
)

// CountryOption is one buyable country button.
type CountryOption struct {
	Code string
	Flag string
	Name string
}

// BuyCountriesKeyboard renders the FR-03 step-1 country picker.
func BuyCountriesKeyboard(countries []CountryOption) models.ReplyMarkup {
	rows := make([][]models.InlineKeyboardButton, 0, len(countries)+1)
	for _, c := range countries {
		label := c.Flag + " " + c.Name
		if c.Flag == "" {
			label = "🌐 " + c.Name
		}
		rows = append(rows, []models.InlineKeyboardButton{{
			Text: label, CallbackData: PrefixBuyCountry + c.Code,
		}})
	}
	rows = append(rows, backRow(CallbackBuyBack, "🏠 Menu Utama"))
	return models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// BuyPlansKeyboard renders the FR-03 step-2 plan picker for one country.
func BuyPlansKeyboard(plans []domain.VpnPlan) models.ReplyMarkup {
	rows := make([][]models.InlineKeyboardButton, 0, len(plans)+1)
	for _, p := range plans {
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         fmt.Sprintf("%d Hari — %s", p.Days, p.Price.FormatIDR()),
			CallbackData: PrefixBuyPlan + p.CountryCode + ":" + fmt.Sprintf("%d", p.Days),
		}})
	}
	rows = append(rows, backRow(CallbackBuyBack, "⬅️ Kembali"))
	return models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// BuyConfirmKeyboard offers explicit confirmation (FR-03 AC: order only after confirm).
func BuyConfirmKeyboard(country string, days int) models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "✅ Konfirmasi Beli", CallbackData: PrefixBuyConfirm + country + ":" + fmt.Sprintf("%d", days)}},
		backRow(CallbackBuyBack, "⬅️ Kembali"),
	}}
}

// RenewClientsKeyboard lists the user's active accounts for renewal (FR-05).
func RenewClientsKeyboard(clients []postgres.ClientView) models.ReplyMarkup {
	rows := make([][]models.InlineKeyboardButton, 0, len(clients)+1)
	for _, c := range clients {
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         fmt.Sprintf("%s %s — %s", flagOrGlobe(c.FlagEmoji), c.ServerName, shortEmail(c.Email)),
			CallbackData: PrefixRenewClient + fmt.Sprintf("%d", c.ID),
		}})
	}
	rows = append(rows, backRow(CallbackRenewBack, "🏠 Menu Utama"))
	return models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// RenewPlanKeyboard reuses the plan list, encoded with the client id prefix.
func RenewPlanKeyboard(clientID int64, plans []domain.VpnPlan) models.ReplyMarkup {
	rows := make([][]models.InlineKeyboardButton, 0, len(plans)+1)
	for _, p := range plans {
		rows = append(rows, []models.InlineKeyboardButton{{
			Text: fmt.Sprintf("%d Hari — %s", p.Days, p.Price.FormatIDR()),
			CallbackData: PrefixRenewPlan + fmt.Sprintf("%d", clientID) + ":" +
				p.CountryCode + ":" + fmt.Sprintf("%d", p.Days),
		}})
	}
	rows = append(rows, backRow(CallbackRenewBack, "⬅️ Kembali"))
	return models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// RenewConfirmKeyboard asks explicit confirmation for a renewal.
func RenewConfirmKeyboard(clientID int64, country string, days int) models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "✅ Konfirmasi Perpanjang", CallbackData: PrefixRenewConfirm +
			fmt.Sprintf("%d", clientID) + ":" + country + ":" + fmt.Sprintf("%d", days)}},
		backRow(CallbackRenewBack, "⬅️ Kembali"),
	}}
}

// backRow is a single centered back/home button.
func backRow(callback, label string) []models.InlineKeyboardButton {
	return []models.InlineKeyboardButton{{Text: label, CallbackData: callback}}
}

// BuyCountryText introduces the country step (FR-03).
func BuyCountryText() string {
	return "Pilih negara server:\n\nPilih lokasi server VPN yang kamu mau."
}

// BuyPlanListText introduces the plan step.
func BuyPlanListText(countryName string) string {
	return fmt.Sprintf("Pilih paket untuk %s:\n\nHarga selalu update dari daftar harga terbaru.", countryName)
}

// BuyConfirmText summarizes the order before confirmation (FR-03 AC).
func BuyConfirmText(plan domain.VpnPlan, balance domain.Money) string {
	var warn string
	if balance < plan.Price {
		warn = fmt.Sprintf("\n\nSaldo kamu %s — tidak cukup untuk paket ini.\nSilakan top up dulu ya.", balance.FormatIDR())
	} else {
		warn = fmt.Sprintf("\n\nSaldo kamu: %s (cukup)", balance.FormatIDR())
	}
	return fmt.Sprintf("Ringkasan Pesanan\n━━━━━━━━━━━━━━\n"+
		"Negara: %s\n"+
		"Paket: VPN %s %d Hari\n"+
		"Harga: %s\n"+
		"Kuota: 100 GB / 1 IP\n━━━━━━━━━━━━━━%s\n\n"+
		"Tekan Konfirmasi untuk memproses order.",
		plan.CountryName, plan.CountryName, plan.Days, plan.Price.FormatIDR(), warn)
}

// BuySuccessText reports a completed purchase (FR-04 AC-2).
func BuySuccessText(orderID, email string, days int, balance domain.Money, countryName string) string {
	return fmt.Sprintf("Order Berhasil\n━━━━━━━━━━━━━━\n"+
		"Order ID: %s\n"+
		"Server: %s\n"+
		"Masa aktif: %d Hari\n"+
		"Email akun: %s\n"+
		"Sisa saldo: %s\n━━━━━━━━━━━━━━\n\n"+
		"Detail koneksi (config link) akan tersedia di menu Akun Saya.",
		orderID, countryName, days, email, balance.FormatIDR())
}

// BuyFailedText reports a failed purchase with a friendly reason.
func BuyFailedText(orderID, reason string) string {
	return fmt.Sprintf("Order Gagal\n━━━━━━━━━━━━━━\nOrder ID: %s\n\n%s\n\n"+
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

// RenewConfirmText summarizes a renewal before confirmation (FR-05 AC).
func RenewConfirmText(client postgres.ClientView, plan domain.VpnPlan, balance domain.Money) string {
	var warn string
	if balance < plan.Price {
		warn = fmt.Sprintf("\n\nSaldo kamu %s — tidak cukup. Silakan top up dulu ya.", balance.FormatIDR())
	} else {
		warn = fmt.Sprintf("\n\nSaldo kamu: %s (cukup)", balance.FormatIDR())
	}
	return fmt.Sprintf("Ringkasan Perpanjangan\n━━━━━━━━━━━━━━\n"+
		"Akun: %s\n"+
		"Paket: %d Hari\n"+
		"Harga: %s\n━━━━━━━━━━━━━━%s\n\n"+
		"Tekan Konfirmasi Perpanjang untuk melanjutkan.",
		client.Email, plan.Days, plan.Price.FormatIDR(), warn)
}

// RenewSuccessText reports a completed renewal with the new expiry.
func RenewSuccessText(orderID, email string, days int, newExpiry time.Time, balance domain.Money) string {
	return fmt.Sprintf("Perpanjangan Berhasil\n━━━━━━━━━━━━━━\n"+
		"Order ID: %s\n"+
		"Akun: %s\n"+
		"Masa aktif bertambah %d Hari\n"+
		"Aktif sampai: %s\n"+
		"Sisa saldo: %s\n━━━━━━━━━━━━━━",
		orderID, email, days, newExpiry.Format("02 Jan 2006"), balance.FormatIDR())
}

// AccountsText renders the FR-08 account list (M4 subset: no pagination yet).
func AccountsText(clients []postgres.ClientView, now time.Time) string {
	if len(clients) == 0 {
		return "Kamu belum punya akun VPN.\n\nSilakan beli dulu di menu Beli VPN."
	}
	var b strings.Builder
	b.WriteString("Akun Saya\n━━━━━━━━━━━━━━\n")
	for i, c := range clients {
		status := "Aktif"
		if c.IsExpired || (c.ExpiresAt != nil && !c.ExpiresAt.After(now)) {
			status = "Expired"
		}
		b.WriteString(fmt.Sprintf("\n%d. %s\n", i+1, c.ServerName))
		b.WriteString(fmt.Sprintf("   %s\n", c.Email))
		b.WriteString(fmt.Sprintf("   %s\n", strings.ToUpper(c.Protocol)))
		b.WriteString(fmt.Sprintf("   %s\n", status))
		if c.ExpiresAt != nil {
			remain := c.ExpiresAt.Sub(now)
			if remain > 0 {
				b.WriteString(fmt.Sprintf("   sisa %d hari\n", int(remain.Hours()/24)+1))
			} else {
				b.WriteString("   sudah habis\n")
			}
		}
	}
	b.WriteString("━━━━━━━━━━━━━━\n\nDetail & config link menyusul di milestone berikutnya.")
	return b.String()
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
