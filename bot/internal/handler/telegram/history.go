// Package telegramhandler also hosts the order history flow (FR-14).
//
// @file      internal/handler/telegram/history.go
// @for       FR-14 Riwayat: paged order list + owned order detail (5/page).
// @uses      context, github.com/go-telegram/bot/models, internal/service/telegram
// @reason    Parity with the reference history_handler: only the user's own
// orders are ever rendered; pagination is bounded (AGENTS.md §1.7).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"strings"

	"github.com/go-telegram/bot/models"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

// historyPageSize is the FR-14 list pagination size (5 per page, PRD AC).
const historyPageSize = 5

// handleHistory routes the FR-14 flow: list → page navigation → detail.
func (d *Dispatcher) handleHistory(ctx context.Context, cb *models.CallbackQuery, data string) {
	switch {
	case data == telegramservice.CallbackHistory:
		d.historyList(ctx, cb, 1)
	case strings.HasPrefix(data, telegramservice.PrefixHistoryPage):
		page, ok := parsePage(strings.TrimPrefix(data, telegramservice.PrefixHistoryPage))
		if !ok {
			d.answer(ctx, cb.ID, telegramservice.UnavailableText())
			return
		}
		d.historyList(ctx, cb, page)
	case strings.HasPrefix(data, telegramservice.PrefixHistoryDetail):
		id, ok := parsePositiveID(strings.TrimPrefix(data, telegramservice.PrefixHistoryDetail))
		if !ok {
			d.answer(ctx, cb.ID, telegramservice.UnavailableText())
			return
		}
		d.historyDetail(ctx, cb, id)
	case data == telegramservice.CallbackHistoryNoop:
		// Non-action page indicator: answer without editing (FR-02 AC).
		d.answer(ctx, cb.ID, "")
	default:
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
	}
}

// historyList renders the user's orders, newest first, paginated (FR-14 AC).
// Out-of-range pages clamp to the nearest valid page instead of erroring.
func (d *Dispatcher) historyList(ctx context.Context, cb *models.CallbackQuery, page int) {
	user, err := d.shop.Users.EnsureUser(ctx, cb.From.ID, cb.From.Username, cb.From.FirstName)
	if err != nil {
		d.logger.Error("ensuring user", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat riwayat, coba lagi ya.")
		return
	}
	total, err := d.shop.History.CountByUser(ctx, user.ID)
	if err != nil {
		d.logger.Error("counting orders", "user_id", user.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat riwayat, coba lagi ya.")
		return
	}
	totalPages := int((total + historyPageSize - 1) / historyPageSize)
	if totalPages == 0 {
		totalPages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}
	orders, err := d.shop.History.ListByUserPage(ctx, user.ID, historyPageSize, (page-1)*historyPageSize)
	if err != nil {
		d.logger.Error("listing orders", "user_id", user.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat riwayat, coba lagi ya.")
		return
	}
	if len(orders) == 0 {
		d.editCB(ctx, cb, telegramservice.HistoryEmptyText(), telegramservice.HistoryEmptyKeyboard())
		return
	}
	d.editCB(ctx, cb, telegramservice.HistoryListText(orders, page, totalPages),
		telegramservice.HistoryListKeyboard(page, totalPages))
}

// historyDetail shows one order, only when it belongs to the user (FR-14 AC —
// foreign or missing orders are indistinguishable to the caller).
func (d *Dispatcher) historyDetail(ctx context.Context, cb *models.CallbackQuery, orderID int64) {
	user, err := d.shop.Users.EnsureUser(ctx, cb.From.ID, cb.From.Username, cb.From.FirstName)
	if err != nil {
		d.logger.Error("ensuring user", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat transaksi, coba lagi ya.")
		return
	}
	order, err := d.shop.History.GetOwned(ctx, orderID, user.ID)
	if err != nil {
		d.logger.Error("getting order detail", "user_id", user.ID, "order_id", orderID, "error", err)
		d.answer(ctx, cb.ID, "Transaksi tidak ditemukan.")
		return
	}
	d.editCB(ctx, cb, telegramservice.HistoryDetailText(*order), telegramservice.HistoryDetailKeyboard())
}
