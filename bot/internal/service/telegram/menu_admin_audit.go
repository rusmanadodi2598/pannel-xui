// Package telegram also hosts the admin audit-log view (FR-11, v1.40).
//
// @file      internal/service/telegram/menu_admin_audit.go
// @for       FR-11 AC: render the immutable admin action trail.
// @uses      fmt, github.com/go-telegram/bot/models, internal/repository/postgres
// @reason    Pure presentation per UI copy policy (split for §1.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-12
package telegram

import (
	"fmt"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// CallbackAdminAudit opens the audit log view.
const CallbackAdminAudit = "admin:audit"

// AdminAuditText renders the newest audit rows (FR-11 AC).
func AdminAuditText(rows []postgres.AdminAuditLog) string {
	if len(rows) == 0 {
		return "Audit Log\n━━━━━━━━━━━━━━\nBelum ada aksi admin tercatat."
	}
	out := "Audit Log\n━━━━━━━━━━━━━━\n"
	for _, r := range rows {
		detail := r.Detail
		if r.Target != "" {
			detail = r.Target
			if r.Detail != "" {
				detail += " | " + r.Detail
			}
		}
		out += fmt.Sprintf("%s | admin %d | %s | %s\n",
			r.CreatedAt.Format("02/01 15:04"), r.AdminID, auditActionLabel(r.Action), detail)
	}
	return out
}

// AdminAuditKeyboard goes back to the admin menu.
func AdminAuditKeyboard() models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		backRow(CallbackAdminMenu, "⬅️ Kembali"),
	}}
}

// auditActionLabel maps the audit action constant to an Indonesian label.
func auditActionLabel(action string) string {
	switch action {
	case "price:set":
		return "Ubah Harga"
	case "plan:toggle":
		return "Toggle Paket"
	case "pricing:reload":
		return "Reload Pricing"
	case "user:ban":
		return "Ban User"
	case "user:unban":
		return "Unban User"
	case "balance:adjust":
		return "Adjust Saldo"
	case "broadcast:start":
		return "Broadcast"
	case "server:add":
		return "Tambah Server"
	case "server:open":
		return "Toggle Penjualan"
	case "server:active":
		return "Toggle Server"
	default:
		return action
	}
}
