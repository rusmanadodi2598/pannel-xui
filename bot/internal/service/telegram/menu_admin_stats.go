// Package telegram also hosts the admin dashboard views (FR-11, v1.40).
//
// @file      internal/service/telegram/menu_admin_stats.go
// @for       FR-11: order/revenue statistik + recent orders view.
// @uses      fmt, time, github.com/go-telegram/bot/models, internal/domain,
// internal/repository/postgres
// @reason    Pure presentation per UI copy policy (emoji-free body, `━━━`
// separators); handler stays network-free and testable (split for §1.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-12
package telegram

import (
	"fmt"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// CallbackAdminStats opens the admin dashboard.
const CallbackAdminStats = "admin:stats"

// AdminStatsText renders the FR-11 dashboard aggregates.
func AdminStatsText(s postgres.OrderStats, now time.Time) string {
	return fmt.Sprintf("Statistik\n━━━━━━━━━━━━━━\n"+
		"Order total: %d\n"+
		"Order hari ini: %d\n"+
		"Revenue total: %s\n"+
		"Revenue hari ini: %s\n"+
		"User terdaftar: %d\n"+
		"Client aktif: %d\n━━━━━━━━━━━━━━\n"+
		"Breakdown status:\n"+
		"  Selesai: %d\n"+
		"  Gagal: %d\n"+
		"  Pending: %d\n"+
		"  Diproses: %d\n"+
		"  Dibatalkan: %d\n"+
		"  Refund: %d",
		s.TotalOrders, s.TodayOrders,
		s.TotalRevenue.FormatIDR(), s.TodayRevenue.FormatIDR(),
		s.TotalUsers, s.ActiveClients,
		s.Completed, s.Failed, s.Pending, s.Processing, s.Cancelled, s.Refunded)
}

// AdminStatsKeyboard offers recent orders + back to admin menu.
func AdminStatsKeyboard() models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(
		models.InlineKeyboardButton{Text: "Order Terbaru", CallbackData: CallbackAdminRecentOrders},
		backBtn(CallbackAdminMenu, "⬅️ Kembali"),
	)}
}

// CallbackAdminRecentOrders lists the newest orders.
const CallbackAdminRecentOrders = "admin:orders:recent"

// AdminRecentOrdersText renders the newest orders (FR-11).
func AdminRecentOrdersText(orders []postgres.Order) string {
	if len(orders) == 0 {
		return "Belum ada order tercatat."
	}
	out := "Order Terbaru\n━━━━━━━━━━━━━━\n"
	for _, o := range orders {
		status := orderStatusLabel(o.Status)
		out += fmt.Sprintf("%s | %s | %s | %s\n",
			o.CreatedAt.Format("02/01 15:04"), orderTypeLabel(o.OrderType),
			status, o.FinalAmount.FormatIDR())
	}
	return out
}

// AdminRecentOrdersKeyboard goes back to the dashboard.
func AdminRecentOrdersKeyboard() models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		backRow(CallbackAdminStats, "⬅️ Statistik"),
	}}
}
