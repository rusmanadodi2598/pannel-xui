// Package telegramhandler also hosts the renewal flow.
//
// @file      internal/handler/telegram/renew.go
// @for       FR-05 renewal: account picker, plan picker, confirmation, execution.
// @uses      context, sort, github.com/go-telegram/bot/models, internal/domain, internal/service/order
// @reason    Renewal extends from remaining time — never double-counted (FR-05 AC).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"errors"
	"sort"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	ordersvc "github.com/kentangtech/bot-order/internal/service/order"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

// renewMenu lists the user's active accounts (FR-05 step 1). Trial accounts
// are excluded (v1.37): renewal is paid-only.
func (d *Dispatcher) renewMenu(ctx context.Context, cb *models.CallbackQuery) {
	user, err := d.shop.Users.EnsureUser(ctx, cb.From.ID, cb.From.Username, cb.From.FirstName)
	if err != nil {
		d.logger.Error("ensuring user", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat akun, coba lagi ya.")
		return
	}
	clients, err := d.shop.Clients.ListByUser(ctx, user.ID, 10)
	if err != nil {
		d.logger.Error("listing clients", "user_id", user.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat daftar akun, coba lagi ya.")
		return
	}
	clients = renewableClients(clients)
	// Only accounts that can still be renewed (not expired forever) are shown;
	// expired-but-owner accounts are still renewable in FR-05.
	if len(clients) == 0 {
		d.editCB(ctx, cb, telegramservice.RenewNoClientsText(), backHomeKeyboard())
		return
	}
	d.editCB(ctx, cb, telegramservice.RenewClientListText(), telegramservice.RenewClientsKeyboard(clients))
}

// renewPlans shows the plan picker for one account (FR-05 step 2).
func (d *Dispatcher) renewPlans(ctx context.Context, cb *models.CallbackQuery, clientID int64) {
	if clientID <= 0 {
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
		return
	}
	user, err := d.shop.Users.EnsureUser(ctx, cb.From.ID, cb.From.Username, cb.From.FirstName)
	if err != nil {
		d.logger.Error("ensuring user", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat akun, coba lagi ya.")
		return
	}
	clients, err := d.shop.Clients.ListByUser(ctx, user.ID, 10)
	if err != nil {
		d.answer(ctx, cb.ID, "Akun tidak ditemukan.")
		return
	}
	// v1.37: trial accounts are not renewable — a crafted callback for one is
	// treated exactly like a foreign/missing account.
	if !containsClient(renewableClients(clients), clientID) {
		d.answer(ctx, cb.ID, "Akun tidak ditemukan.")
		return
	}
	plans, err := d.shop.Plans.ListEnabled(ctx)
	if err != nil {
		d.logger.Error("listing plans", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat daftar harga, coba lagi ya.")
		return
	}
	country := clientCountry(clients, clientID)
	var countryPlans []domain.VpnPlan
	var name string
	for _, p := range plans {
		if p.CountryCode == country {
			countryPlans = append(countryPlans, p)
			name = p.CountryName
		}
	}
	if len(countryPlans) == 0 {
		d.answer(ctx, cb.ID, "Paket untuk negara ini belum tersedia.")
		return
	}
	sort.Slice(countryPlans, func(i, j int) bool { return countryPlans[i].Days < countryPlans[j].Days })
	d.editCB(ctx, cb, telegramservice.BuyPlanListText(name), telegramservice.RenewPlanKeyboard(clientID, countryPlans))
}

// renewConfirm shows the summary with live price + balance (FR-05 step 3).
func (d *Dispatcher) renewConfirm(ctx context.Context, cb *models.CallbackQuery, clientID int64, country string, days int) {
	client, err := d.clientForConfirm(ctx, cb, clientID)
	if err != nil {
		d.answer(ctx, cb.ID, "Akun tidak ditemukan.")
		return
	}
	plan, err := d.shop.Plans.GetPlan(ctx, country, days)
	if err != nil {
		d.answer(ctx, cb.ID, "Paket sudah tidak tersedia.")
		return
	}
	balance, err := d.shop.Users.Balance(ctx, cb.From.ID)
	if err != nil {
		balance = 0
	}
	d.editCB(ctx, cb, telegramservice.RenewConfirmText(client, *plan, balance),
		telegramservice.RenewConfirmKeyboard(clientID, country, days))
}

// renewExecute runs the renewal order (FR-05) and reports the outcome.
func (d *Dispatcher) renewExecute(ctx context.Context, cb *models.CallbackQuery, clientID int64, country string, days int) {
	user, err := d.shop.Users.EnsureUser(ctx, cb.From.ID, cb.From.Username, cb.From.FirstName)
	if err != nil {
		d.logger.Error("ensuring user", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat akun, coba lagi ya.")
		return
	}

	if !d.editCB(ctx, cb, "⏳ Memproses perpanjangan...", nil) {
		return
	}

	res, err := d.shop.Orders.Renew(ctx, user, clientID, country, days)
	switch {
	case err == nil:
		d.send(ctx, cb.Message.Message.Chat.ID,
			telegramservice.RenewSuccessText(res.OrderID, res.AccountEmail, days, res.NewExpiry, res.BalanceAfter), nil)
	case err == ordersvc.ErrInsufficientBalance:
		d.send(ctx, cb.Message.Message.Chat.ID, insufficientText(), topupHintKeyboard())
	case err == ordersvc.ErrClientNotFound:
		d.send(ctx, cb.Message.Message.Chat.ID, "Akun tidak ditemukan atau bukan milik kamu.", nil)
	case err == ordersvc.ErrTrialNotRenewable:
		d.send(ctx, cb.Message.Message.Chat.ID, "Akun trial tidak bisa diperpanjang. Silakan buat akun baru ya.", nil)
	case err == ordersvc.ErrOrderInFlight:
		d.send(ctx, cb.Message.Message.Chat.ID, "Perpanjangan sebelumnya masih diproses. Tunggu sebentar ya.", nil)
	default:
		d.logger.Error("renew failed", "user_id", cb.From.ID, "client_id", clientID, "error", err)
		// res may be nil for pre-order failures (DB errors) — never dereference.
		orderID := ""
		if res != nil {
			orderID = res.OrderID
		}
		d.send(ctx, cb.Message.Message.Chat.ID,
			telegramservice.BuyFailedText(orderID, "Terjadi kendala saat memperpanjang akun di server."), nil)
	}
}

// clientForConfirm loads the client row for the confirmation view (ownership already
// enforced later by the order service; here we only need display data). Trial
// accounts are excluded — renewal is paid-only (v1.37).
func (d *Dispatcher) clientForConfirm(ctx context.Context, cb *models.CallbackQuery, clientID int64) (postgres.ClientView, error) {
	user, err := d.shop.Users.EnsureUser(ctx, cb.From.ID, cb.From.Username, cb.From.FirstName)
	if err != nil {
		return postgres.ClientView{}, err
	}
	clients, err := d.shop.Clients.ListByUser(ctx, user.ID, 10)
	if err != nil {
		return postgres.ClientView{}, err
	}
	for _, c := range renewableClients(clients) {
		if c.ID == clientID {
			return c, nil
		}
	}
	return postgres.ClientView{}, errors.New("client not found for confirm view")
}

// renewableClients filters to paid accounts only (FR-05, v1.37): trial accounts
// are never renewable and must not appear in the picker or accept callbacks.
func renewableClients(clients []postgres.ClientView) []postgres.ClientView {
	paid := make([]postgres.ClientView, 0, len(clients))
	for _, c := range clients {
		if !c.IsTrial {
			paid = append(paid, c)
		}
	}
	return paid
}

func containsClient(clients []postgres.ClientView, id int64) bool {
	for _, c := range clients {
		if c.ID == id {
			return true
		}
	}
	return false
}

func clientCountry(clients []postgres.ClientView, id int64) string {
	for _, c := range clients {
		if c.ID == id {
			return c.CountryCode
		}
	}
	return ""
}

func backHomeKeyboard() models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "🏠 Menu Utama", CallbackData: telegramservice.CallbackHome}},
	}}
}
