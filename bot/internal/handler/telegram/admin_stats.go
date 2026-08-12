// Package telegramhandler also hosts the admin dashboard views (FR-11, v1.40).
//
// @file      internal/handler/telegram/admin_stats.go
// @for       FR-11: statistik order/revenue + audit log view.
// @uses      context, time, github.com/go-telegram/bot/models, internal/service/telegram
// @reason    Read-only admin views behind AdminOps — thin handlers, no business
// logic (AGENTS.md §1.5). Split for the §1.1 line limit.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability experimental
// @since     2026-08-12
package telegramhandler

import (
	"context"
	"time"

	"github.com/go-telegram/bot/models"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

// adminStats renders the FR-11 dashboard aggregates.
func (d *Dispatcher) adminStats(ctx context.Context, cb *models.CallbackQuery) {
	d.adminClearFSM(ctx, cb.From.ID)
	stats, err := d.admin.Ops.Stats(ctx, time.Local)
	if err != nil {
		d.logger.Error("admin stats failed", "error", err)
		d.editCB(ctx, cb, "Gagal memuat statistik. Coba lagi ya.", nil)
		return
	}
	d.editCB(ctx, cb, telegramservice.AdminStatsText(stats, time.Now()), telegramservice.AdminStatsKeyboard())
}

// adminRecentOrders lists the newest orders for the admin dashboard.
func (d *Dispatcher) adminRecentOrders(ctx context.Context, cb *models.CallbackQuery) {
	orders, err := d.admin.Ops.RecentOrders(ctx, 10)
	if err != nil {
		d.logger.Error("admin recent orders failed", "error", err)
		d.editCB(ctx, cb, "Gagal memuat order terbaru. Coba lagi ya.", nil)
		return
	}
	d.editCB(ctx, cb, telegramservice.AdminRecentOrdersText(orders), telegramservice.AdminRecentOrdersKeyboard())
}

// adminAudit renders the newest admin actions (FR-11 AC).
func (d *Dispatcher) adminAudit(ctx context.Context, cb *models.CallbackQuery) {
	d.adminClearFSM(ctx, cb.From.ID)
	rows, err := d.admin.Ops.AuditLog(ctx, 10)
	if err != nil {
		d.logger.Error("admin audit failed", "error", err)
		d.editCB(ctx, cb, "Gagal memuat audit log. Coba lagi ya.", nil)
		return
	}
	d.editCB(ctx, cb, telegramservice.AdminAuditText(rows), telegramservice.AdminAuditKeyboard())
}
