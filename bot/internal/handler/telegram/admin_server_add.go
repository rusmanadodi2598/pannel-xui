// Package telegramhandler also hosts the admin add-server FSM (FR-11, v1.40).
//
// @file      internal/handler/telegram/admin_server_add.go
// @for       FR-11: 6-step add-server flow (arm → 6 inputs → confirm).
// @uses      context, strings, github.com/go-telegram/bot/models,
// internal/service/server, internal/service/telegram
// @reason    Split from admin_servers.go for the §1.1 line limit; the FSM
// accumulates a pipe-separated draft (pattern ban/saldo) and the final input
// step arms a distinct srvadd:confirm state so a double-tapped confirm never
// calls AddServer twice (idempotence parity with saldo v1.39).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability experimental
// @since     2026-08-12
package telegramhandler

import (
	"context"
	"strings"

	"github.com/go-telegram/bot/models"
	serversvc "github.com/kentangtech/bot-order/internal/service/server"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

// State: srvadd:<langkah> saat mengisi, srvadd:confirm:<encoded> setelah lengkap.

// adminServerAddArm starts the add-server flow with an empty draft (step 1: nama).
func (d *Dispatcher) adminServerAddArm(ctx context.Context, cb *models.CallbackQuery) {
	if err := d.admin.FSM.Set(ctx, cb.From.ID, "srvadd:"); err != nil {
		d.logger.Error("admin fsm set failed", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Terjadi kendala, coba lagi ya.")
		return
	}
	d.editCB(ctx, cb, telegramservice.AdminServerAddNamePrompt(), nil)
}

// adminServerAddInput consumes one free-text step and advances the FSM.
func (d *Dispatcher) adminServerAddInput(ctx context.Context, msg *models.Message, state string) {
	// Load the draft accumulated so far from the FSM state (an empty value
	// re-prompts the CURRENT step, not step 1 — fix review v1.40).
	draft, err := d.adminServerDraft(state)
	if err != nil {
		d.logger.Error("admin server draft failed", "user_id", msg.From.ID, "error", err)
		_ = d.admin.FSM.Clear(ctx, msg.From.ID)
		d.send(ctx, msg.Chat.ID, "Terjadi kendala, coba lagi ya.", nil)
		return
	}
	value := strings.TrimSpace(msg.Text)
	if value == "" {
		d.send(ctx, msg.Chat.ID, d.adminServerPrompt(draft), nil)
		return
	}

	step, ok := draft.fillNext(value)
	if !ok {
		// Invalid value for the current step — re-prompt the same step.
		d.send(ctx, msg.Chat.ID, d.adminServerPrompt(draft), nil)
		return
	}
	if step == "" {
		// All six fields filled — arm the confirm-pending state and show the
		// confirmation. The confirm callback verifies this exact draft before
		// executing (idempotence — double tap runs AddServer once).
		pending := "srvadd:confirm:" + draft.encode()
		if err := d.admin.FSM.Set(ctx, msg.From.ID, pending); err != nil {
			d.logger.Error("admin fsm set failed", "user_id", msg.From.ID, "error", err)
			_ = d.admin.FSM.Clear(ctx, msg.From.ID)
			d.send(ctx, msg.Chat.ID, "Terjadi kendala, coba lagi ya.", nil)
			return
		}
		d.send(ctx, msg.Chat.ID, telegramservice.AdminServerAddConfirmText(
			draft.name, draft.host, draft.port, draft.username, draft.country, draft.flag),
			telegramservice.AdminServerAddConfirmKeyboard())
		return
	}
	if err := d.admin.FSM.Set(ctx, msg.From.ID, "srvadd:"+draft.encode()); err != nil {
		d.logger.Error("admin fsm set failed", "user_id", msg.From.ID, "error", err)
		_ = d.admin.FSM.Clear(ctx, msg.From.ID)
		d.send(ctx, msg.Chat.ID, "Terjadi kendala, coba lagi ya.", nil)
		return
	}
	d.send(ctx, msg.Chat.ID, d.adminServerPrompt(draft), nil)
}

// adminServerDraft parses an FSM state back into an in-progress draft. Both the
// in-progress (srvadd:<encoded>) and confirm-pending (srvadd:confirm:<encoded>)
// forms are accepted.
func (d *Dispatcher) adminServerDraft(state string) (*serverDraft, error) {
	raw := strings.TrimPrefix(state, "srvadd:")
	raw = strings.TrimPrefix(raw, "confirm:")
	draft := &serverDraft{}
	if raw != "" {
		if err := draft.decode(raw); err != nil {
			return nil, err
		}
	}
	return draft, nil
}

// adminServerPrompt returns the next prompt for the current draft.
func (d *Dispatcher) adminServerPrompt(draft *serverDraft) string {
	if draft == nil {
		draft = &serverDraft{}
	}
	switch {
	case draft.name == "":
		return telegramservice.AdminServerAddNamePrompt()
	case draft.host == "":
		return telegramservice.AdminServerAddHostPrompt()
	case draft.port == 0:
		return telegramservice.AdminServerAddPortPrompt()
	case draft.username == "":
		return telegramservice.AdminServerAddUsernamePrompt()
	case draft.password == "":
		return telegramservice.AdminServerAddPasswordPrompt()
	default:
		return telegramservice.AdminServerAddCountryPrompt()
	}
}

// adminServerAddConfirm executes the staged server creation (idempotence: only
// runs when the FSM still holds the exact confirm-pending draft, so a stale or
// double-tapped confirm is answered without creating a second server).
func (d *Dispatcher) adminServerAddConfirm(ctx context.Context, cb *models.CallbackQuery) {
	state, ok, err := d.admin.FSM.Get(ctx, cb.From.ID)
	if err != nil {
		d.logger.Error("admin fsm read failed", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Terjadi kendala, coba lagi ya.")
		return
	}
	if !ok || !strings.HasPrefix(state, "srvadd:confirm:") {
		d.answer(ctx, cb.ID, "Form tambah server sudah kedaluwarsa. Ulangi dari awal ya.")
		return
	}
	draft, derr := d.adminServerDraft(state)
	if derr != nil || !draft.complete() {
		d.answer(ctx, cb.ID, "Data server belum lengkap. Ulangi dari awal ya.")
		return
	}
	d.adminClearFSM(ctx, cb.From.ID)

	id, aerr := d.admin.Ops.AddServer(ctx, cb.From.ID, serversvc.NewServerInput{
		Name:        draft.name,
		Host:        draft.host,
		Port:        draft.port,
		Username:    draft.username,
		Password:    draft.password,
		APIPath:     "/panel",
		UseSSL:      true,
		CountryCode: draft.country,
		FlagEmoji:   draft.flag,
		Protocols:   nil,
	})
	if aerr != nil {
		d.logger.Error("admin add server failed", "error", aerr)
		d.editCB(ctx, cb, "Gagal menambahkan server. Coba lagi ya.", telegramservice.AdminMenuKeyboard())
		return
	}
	// Re-render the fresh list so the new server is visible immediately.
	servers, lerr := d.admin.Ops.ListServers(ctx)
	if lerr != nil {
		d.logger.Error("admin list servers failed", "error", lerr)
		d.editCB(ctx, cb, telegramservice.AdminServerAddedText(id), telegramservice.AdminMenuKeyboard())
		return
	}
	d.editCB(ctx, cb, telegramservice.AdminServerAddedText(id), telegramservice.AdminServersKeyboard(servers))
}
