// Package telegramhandler also hosts the account list/detail/export views.
//
// @file      internal/handler/telegram/accounts.go
// @for       FR-08: list accounts, show detail, export .txt document (M7).
// @uses      context, time, github.com/go-telegram/bot/models, internal/service/telegram
// @reason    The config link gap is closed here: the .txt export carries the
// share URI (URL hanya di ekspor sejak v1.36); detail renders clean info.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"time"

	"github.com/go-telegram/bot/models"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

// accountPageSize is the FR-08 list pagination size (5 per page, PRD AC).
const accountPageSize = 5

// handleAccount renders the first page of the user's accounts (FR-08 AC-1).
func (d *Dispatcher) handleAccount(ctx context.Context, cb *models.CallbackQuery) {
	d.accountList(ctx, cb, 1)
}

// accountList renders one page of the user's accounts, newest first, with
// pagination navigation (FR-08 AC-1). Out-of-range pages clamp to the nearest
// valid page — same behaviour as the FR-14 history list.
func (d *Dispatcher) accountList(ctx context.Context, cb *models.CallbackQuery, page int) {
	user, err := d.shop.Users.EnsureUser(ctx, cb.From.ID, cb.From.Username, cb.From.FirstName)
	if err != nil {
		d.logger.Error("ensuring user", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat akun, coba lagi ya.")
		return
	}
	total, err := d.shop.Clients.CountByUser(ctx, user.ID)
	if err != nil {
		d.logger.Error("counting clients", "user_id", user.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat daftar akun, coba lagi ya.")
		return
	}
	totalPages := int((total + accountPageSize - 1) / accountPageSize)
	if totalPages == 0 {
		totalPages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}
	clients, err := d.shop.Clients.ListByUserPage(ctx, user.ID, accountPageSize, (page-1)*accountPageSize)
	if err != nil {
		d.logger.Error("listing clients", "user_id", user.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat daftar akun, coba lagi ya.")
		return
	}
	if len(clients) == 0 {
		d.editCB(ctx, cb, telegramservice.AccountsText(nil, 1, 1, time.Now()),
			telegramservice.AccountEmptyKeyboard())
		return
	}
	d.editCB(ctx, cb, telegramservice.AccountsText(clients, page, totalPages, time.Now()),
		telegramservice.AccountListKeyboard(clients, page, totalPages))
}

// accountView shows one account's full details (FR-08). The import URL is
// intentionally not in the view — it lives only in the .txt export (v1.36).
func (d *Dispatcher) accountView(ctx context.Context, cb *models.CallbackQuery, clientID int64) {
	user, err := d.shop.Users.EnsureUser(ctx, cb.From.ID, cb.From.Username, cb.From.FirstName)
	if err != nil {
		d.logger.Error("ensuring user", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat akun, coba lagi ya.")
		return
	}
	client, err := d.shop.Clients.GetViewOwned(ctx, clientID, user.ID)
	if err != nil {
		d.logger.Error("getting client detail", "user_id", user.ID, "client_id", clientID, "error", err)
		d.answer(ctx, cb.ID, "Akun tidak ditemukan.")
		return
	}
	d.editCB(ctx, cb, telegramservice.AccountDetailText(client, time.Now()),
		telegramservice.AccountDetailKeyboard(clientID))
}

// accountTraffic shows one account's live usage (FR-08 AC-3). It syncs from
// the panel first (manual refresh) and re-reads the row so the page always
// renders fresh numbers. A failed sync still renders the last known values
// (best effort — parity with the reference client-vpn traffic page).
func (d *Dispatcher) accountTraffic(ctx context.Context, cb *models.CallbackQuery, clientID int64) {
	user, err := d.shop.Users.EnsureUser(ctx, cb.From.ID, cb.From.Username, cb.From.FirstName)
	if err != nil {
		d.logger.Error("ensuring user", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat akun, coba lagi ya.")
		return
	}
	client, err := d.shop.Clients.GetViewOwned(ctx, clientID, user.ID)
	if err != nil {
		d.logger.Error("getting client traffic", "user_id", user.ID, "client_id", clientID, "error", err)
		d.answer(ctx, cb.ID, "Akun tidak ditemukan.")
		return
	}
	if d.shop.Traffic != nil {
		if err := d.shop.Traffic.RefreshClient(ctx, clientID, client.ServerID, client.Email); err != nil {
			d.logger.Error("refreshing client traffic", "user_id", user.ID, "client_id", clientID, "error", err)
			d.answer(ctx, cb.ID, "Gagal sync, menampilkan data terakhir.")
		}
	}
	// Re-read after the sync so the page shows the panel numbers, not the
	// pre-refresh row.
	fresh, err := d.shop.Clients.GetViewOwned(ctx, clientID, user.ID)
	if err != nil {
		d.logger.Error("re-reading client traffic", "user_id", user.ID, "client_id", clientID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat data traffic, coba lagi ya.")
		return
	}
	d.editCB(ctx, cb, telegramservice.AccountTrafficText(fresh, time.Now()),
		telegramservice.AccountTrafficKeyboard(clientID))
}

// accountConvert shows the Clash/Meta YAML config for one account (FR-08
// AC-2) — ownership-guarded like the config view.
func (d *Dispatcher) accountConvert(ctx context.Context, cb *models.CallbackQuery, clientID int64) {
	user, err := d.shop.Users.EnsureUser(ctx, cb.From.ID, cb.From.Username, cb.From.FirstName)
	if err != nil {
		d.logger.Error("ensuring user", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat akun, coba lagi ya.")
		return
	}
	client, err := d.shop.Clients.GetViewOwned(ctx, clientID, user.ID)
	if err != nil {
		d.logger.Error("getting client yaml", "user_id", user.ID, "client_id", clientID, "error", err)
		d.answer(ctx, cb.ID, "Akun tidak ditemukan.")
		return
	}
	d.editCB(ctx, cb, telegramservice.AccountConvertText(client),
		telegramservice.AccountConvertKeyboard(clientID))
}

// accountConfig shows the v2Ray import parameters (TLS + non-TLS) without
// the URLs — those live only in the .txt export (v1.26 + v1.36 cleanup).
func (d *Dispatcher) accountConfig(ctx context.Context, cb *models.CallbackQuery, clientID int64) {
	user, err := d.shop.Users.EnsureUser(ctx, cb.From.ID, cb.From.Username, cb.From.FirstName)
	if err != nil {
		d.logger.Error("ensuring user", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat akun, coba lagi ya.")
		return
	}
	client, err := d.shop.Clients.GetViewOwned(ctx, clientID, user.ID)
	if err != nil {
		d.logger.Error("getting client config", "user_id", user.ID, "client_id", clientID, "error", err)
		d.answer(ctx, cb.ID, "Akun tidak ditemukan.")
		return
	}
	d.editCB(ctx, cb, telegramservice.AccountConfigText(client),
		telegramservice.AccountConfigKeyboard(clientID))
}

// accountExport sends the account details as a .txt document (M7 feature).
func (d *Dispatcher) accountExport(ctx context.Context, cb *models.CallbackQuery, clientID int64) {
	user, err := d.shop.Users.EnsureUser(ctx, cb.From.ID, cb.From.Username, cb.From.FirstName)
	if err != nil {
		d.logger.Error("ensuring user", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat akun, coba lagi ya.")
		return
	}
	client, err := d.shop.Clients.GetViewOwned(ctx, clientID, user.ID)
	if err != nil {
		d.logger.Error("getting client for export", "user_id", user.ID, "client_id", clientID, "error", err)
		d.answer(ctx, cb.ID, "Akun tidak ditemukan.")
		return
	}
	msg := cb.Message.Message
	if msg == nil {
		// Telegram may deliver a callback whose message is inaccessible;
		// answer (never panic) and let the user retry from Akun Saya.
		d.answer(ctx, cb.ID, "Ulangi dari menu Akun Saya ya.")
		return
	}
	content := telegramservice.AccountTXTContent(client, time.Now())
	if err := d.api.SendDocument(ctx, msg.Chat.ID,
		telegramservice.AccountTXTName(client.Email), []byte(content), "Akun VPN kamu — simpan baik-baik ya."); err != nil {
		d.logger.Error("export document failed", "user_id", user.ID, "client_id", clientID, "error", err)
		d.answer(ctx, cb.ID, "Gagal mengekspor akun, coba lagi ya.")
		return
	}
	d.answer(ctx, cb.ID, "File .txt terkirim.")
}

// parseAccountID validates an account callback payload (numeric client id).
func parseAccountID(raw string) (int64, bool) {
	return parsePositiveID(raw)
}
