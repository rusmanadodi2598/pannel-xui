// Package telegram also hosts the admin panel views (FR-11, M6).
//
// @file      internal/service/telegram/menu_admin.go
// @for       FR-11 admin menu: price management, broadcast, ban/unban keyboards & copy.
// @uses      fmt, strings, github.com/go-telegram/bot/models, internal/domain,
// internal/repository/postgres
// @reason    Pure presentation per UI copy policy (emoji-free body); the admin
//
//	handler stays network-free and testable.
//
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-11
package telegram

import (
	"fmt"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/domain"
)

// Admin callback data contract (FR-11, pola admin:*).
const (
	CallbackAdminMenu        = "admin:menu"
	CallbackAdminBack        = "admin:back"
	CallbackAdminPrice       = "admin:price"
	CallbackAdminReload      = "admin:reload"
	CallbackAdminBroadcast   = "admin:broadcast"
	CallbackAdminBcastSend   = "admin:bcast:send"
	CallbackAdminBcastCancel = "admin:bcast:cancel"
	CallbackAdminBan         = "admin:ban"
	CallbackAdminUnban       = "admin:unban"
	CallbackAdminCancel      = "admin:cancel"

	PrefixAdminPlan         = "admin:plan:"
	PrefixAdminSetPrice     = "admin:setprice:"
	PrefixAdminToggle       = "admin:toggle:"
	PrefixAdminBanConfirm   = "admin:ban:confirm:"
	PrefixAdminUnbanConfirm = "admin:unban:confirm:"
)

// AdminDeniedText answers non-admin taps on admin surfaces.
func AdminDeniedText() string {
	return "Akses ditolak. Panel ini khusus admin."
}

// AdminMenuText introduces the admin panel (FR-11).
func AdminMenuText() string {
	return "Panel Admin\n━━━━━━━━━━━━━━\n\n" +
		"Kelola harga paket, broadcast pengumuman, dan status user.\n\n" +
		"Menu ini hanya dapat diakses ADMIN_IDS."
}

// AdminMenuKeyboard renders the FR-11 admin actions (icon policy: action
// buttons text-only, navigation buttons keep icons; 2-1-2-1 zigzag).
func AdminMenuKeyboard() models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(
		models.InlineKeyboardButton{Text: "Harga", CallbackData: CallbackAdminPrice},
		models.InlineKeyboardButton{Text: "Server", CallbackData: CallbackAdminServers},
		models.InlineKeyboardButton{Text: "Broadcast", CallbackData: CallbackAdminBroadcast},
		models.InlineKeyboardButton{Text: "Ban User", CallbackData: CallbackAdminBan},
		models.InlineKeyboardButton{Text: "Unban User", CallbackData: CallbackAdminUnban},
		models.InlineKeyboardButton{Text: "Adjust Saldo", CallbackData: CallbackAdminSaldo},
		models.InlineKeyboardButton{Text: "Statistik", CallbackData: CallbackAdminStats},
		models.InlineKeyboardButton{Text: "Audit Log", CallbackData: CallbackAdminAudit},
		backBtn(CallbackHome, "🏠 Menu Utama"),
	)}
}

// AdminPriceText introduces the plan list.
func AdminPriceText() string {
	return "Daftar Paket\n━━━━━━━━━━━━━━\n\nPilih paket untuk ubah harga atau toggle status."
}

// AdminPriceKeyboard lists every plan (enabled and disabled); disabled plans
// are prefixed with a marker so the admin sees the sellable state at a glance
// (2-1-2-1 zigzag).
func AdminPriceKeyboard(plans []domain.VpnPlan) models.ReplyMarkup {
	buttons := make([]models.InlineKeyboardButton, 0, len(plans)+2)
	for _, p := range plans {
		label := fmt.Sprintf("%s %d Hari — %s", p.CountryCode, p.Days, p.Price.FormatIDR())
		if !p.Enabled {
			label = "🚫 " + label
		}
		buttons = append(buttons, models.InlineKeyboardButton{
			Text:         label,
			CallbackData: PrefixAdminPlan + p.CountryCode + ":" + fmt.Sprintf("%d", p.Days),
		})
	}
	buttons = append(buttons,
		models.InlineKeyboardButton{Text: "Reload Seed", CallbackData: CallbackAdminReload},
		backBtn(CallbackAdminMenu, "⬅️ Kembali"),
	)
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(buttons...)}
}

// AdminPlanDetailText shows one plan with its sellable state.
func AdminPlanDetailText(p domain.VpnPlan) string {
	status := "Aktif"
	if !p.Enabled {
		status = "Nonaktif"
	}
	return fmt.Sprintf("Detail Paket\n━━━━━━━━━━━━━━\n"+
		"Negara: %s (%s)\n"+
		"Durasi: %d Hari\n"+
		"Harga: %s\n"+
		"Status: %s\n━━━━━━━━━━━━━━",
		p.CountryName, p.CountryCode, p.Days, p.Price.FormatIDR(), status)
}

// AdminPlanDetailKeyboard offers set-price and toggle actions (2-1-2-1 zigzag).
func AdminPlanDetailKeyboard(country string, days int) models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(
		models.InlineKeyboardButton{Text: "Ubah Harga", CallbackData: PrefixAdminSetPrice + country + ":" + fmt.Sprintf("%d", days)},
		models.InlineKeyboardButton{Text: "Toggle Status", CallbackData: PrefixAdminToggle + country + ":" + fmt.Sprintf("%d", days)},
		backBtn(CallbackAdminPrice, "⬅️ Daftar Paket"),
	)}
}

// AdminSetPricePrompt asks for the new price (FSM input, FR-11).
func AdminSetPricePrompt(country string, days int) string {
	return fmt.Sprintf("Ketik harga baru untuk paket %s %d Hari (angka rupiah, contoh: 7500).\n\n"+
		"Ketik /cancel untuk membatalkan.", country, days)
}

// AdminPriceSavedText confirms a price update.
func AdminPriceSavedText(country string, days int, price domain.Money) string {
	return fmt.Sprintf("Harga paket %s %d Hari diubah menjadi %s.",
		country, days, price.FormatIDR())
}

// AdminPlanToggledText confirms an enabled/disabled toggle.
func AdminPlanToggledText(country string, days int, enabled bool) string {
	state := "diaktifkan"
	if !enabled {
		state = "dinonaktifkan"
	}
	return fmt.Sprintf("Paket %s %d Hari %s. Harga live langsung terupdate.", country, days, state)
}

// AdminReloadText confirms a pricing reseed.
func AdminReloadText() string {
	return "Pricing di-reload dari file seed. Daftar harga live sudah diperbarui."
}
