// Package telegramhandler also hosts the admin server-management flow (FR-11, v1.40).
//
// @file      internal/handler/telegram/admin_servers.go
// @for       FR-11: server list/detail + toggle open/active.
// @uses      context, github.com/go-telegram/bot/models, internal/service/server,
// internal/service/telegram
// @reason    Panel management via chat follows the ban/saldo FSM pattern; the
// add-server FSM and draft live in sibling files (split for §1.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability experimental
// @since     2026-08-12
package telegramhandler

import (
	"context"

	"github.com/go-telegram/bot/models"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

// adminServers lists every panel for the admin (active + inactive).
func (d *Dispatcher) adminServers(ctx context.Context, cb *models.CallbackQuery) {
	d.adminClearFSM(ctx, cb.From.ID)
	servers, err := d.admin.Ops.ListServers(ctx)
	if err != nil {
		d.logger.Error("admin list servers failed", "error", err)
		d.editCB(ctx, cb, "Gagal memuat daftar server. Coba lagi ya.", nil)
		return
	}
	d.editCB(ctx, cb, telegramservice.AdminServersText(), telegramservice.AdminServersKeyboard(servers))
}

// adminServerDetail shows one server with open/active toggles.
func (d *Dispatcher) adminServerDetail(ctx context.Context, cb *models.CallbackQuery, raw string) {
	id, ok := parsePositiveID(raw)
	if !ok {
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
		return
	}
	servers, err := d.admin.Ops.ListServers(ctx)
	if err != nil {
		d.logger.Error("admin list servers failed", "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat server. Coba lagi ya.")
		return
	}
	for _, s := range servers {
		if s.ID == id {
			d.editCB(ctx, cb, telegramservice.AdminServerDetailText(s), telegramservice.AdminServerDetailKeyboard(s))
			return
		}
	}
	d.answer(ctx, cb.ID, "Server tidak ditemukan.")
}

// adminServerToggle flips open (isOpen=true) or active (isOpen=false) and
// re-renders the detail with the fresh state.
func (d *Dispatcher) adminServerToggle(ctx context.Context, cb *models.CallbackQuery, raw string, isOpen bool) {
	id, ok := parsePositiveID(raw)
	if !ok {
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
		return
	}
	// Read the current state so the toggle flips it correctly.
	servers, err := d.admin.Ops.ListServers(ctx)
	if err != nil {
		d.logger.Error("admin list servers failed", "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat server. Coba lagi ya.")
		return
	}
	var current *telegramServerState
	for i := range servers {
		if servers[i].ID == id {
			current = &telegramServerState{open: servers[i].IsOpen, active: servers[i].IsActive}
			break
		}
	}
	if current == nil {
		d.answer(ctx, cb.ID, "Server tidak ditemukan.")
		return
	}
	if isOpen {
		err = d.admin.Ops.ToggleServerOpen(ctx, cb.From.ID, id, !current.open)
	} else {
		err = d.admin.Ops.ToggleServerActive(ctx, cb.From.ID, id, !current.active)
	}
	if err != nil {
		d.logger.Error("admin toggle server failed", "server_id", id, "error", err)
		d.answer(ctx, cb.ID, "Gagal mengubah server. Coba lagi ya.")
		return
	}
	d.answer(ctx, cb.ID, telegramservice.AdminServerToggledText("Server"))
	// Re-render detail with fresh state.
	servers, _ = d.admin.Ops.ListServers(ctx)
	for _, s := range servers {
		if s.ID == id {
			d.editCB(ctx, cb, telegramservice.AdminServerDetailText(s), telegramservice.AdminServerDetailKeyboard(s))
			return
		}
	}
	d.editCB(ctx, cb, "Server diperbarui.", telegramservice.AdminMenuKeyboard())
}

// telegramServerState is the current sellable/active flags read before a toggle.
type telegramServerState struct {
	open   bool
	active bool
}
