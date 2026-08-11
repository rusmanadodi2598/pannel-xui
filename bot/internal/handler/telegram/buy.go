// Package telegramhandler also hosts the buy flow.
//
// @file      internal/handler/telegram/buy.go
// @for       FR-03 buy: country picker, plan picker, confirmation, execution.
// @uses      context, fmt, sort, github.com/go-telegram/bot/models, internal/domain, internal/service/order
// @reason    The auto-order core; every step re-reads live pricing (FR-03 AC).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"sort"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/domain"
	ordersvc "github.com/kentangtech/bot-order/internal/service/order"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

// buyMenu shows the countries that have both an enabled plan and an open server.
func (d *Dispatcher) buyMenu(ctx context.Context, cb *models.CallbackQuery) {
	plans, err := d.shop.Plans.ListEnabled(ctx)
	if err != nil {
		d.logger.Error("listing plans", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat daftar harga, coba lagi ya.")
		return
	}
	servers, err := d.shop.Servers.ListBuyable(ctx)
	if err != nil {
		d.logger.Error("listing servers", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat server, coba lagi ya.")
		return
	}

	available := map[string]bool{}
	for _, s := range servers {
		available[s.CountryCode] = true
	}
	seen := map[string]bool{}
	var countries []telegramservice.CountryOption
	for _, p := range plans {
		if !available[p.CountryCode] || seen[p.CountryCode] {
			continue
		}
		seen[p.CountryCode] = true
		countries = append(countries, telegramservice.CountryOption{
			Code: p.CountryCode,
			Name: p.CountryName,
		})
	}
	sort.Slice(countries, func(i, j int) bool { return countries[i].Code < countries[j].Code })

	if len(countries) == 0 {
		d.editCB(ctx, cb, "Belum ada server tersedia. Coba lagi nanti ya.", nil)
		return
	}
	d.editCB(ctx, cb, telegramservice.BuyCountryText(), telegramservice.BuyCountriesKeyboard(countries))
}

// buyCountry shows the plans for one country.
func (d *Dispatcher) buyCountry(ctx context.Context, cb *models.CallbackQuery, country string) {
	plans, err := d.shop.Plans.ListEnabled(ctx)
	if err != nil {
		d.logger.Error("listing plans", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat daftar harga, coba lagi ya.")
		return
	}
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
	d.editCB(ctx, cb, telegramservice.BuyPlanListText(name), telegramservice.BuyPlansKeyboard(countryPlans))
}

// buyConfirm shows the summary + live balance before explicit confirmation.
func (d *Dispatcher) buyConfirm(ctx context.Context, cb *models.CallbackQuery, country string, days int) {
	plan, err := d.shop.Plans.GetPlan(ctx, country, days)
	if err != nil {
		d.answer(ctx, cb.ID, "Paket sudah tidak tersedia.")
		return
	}
	balance, err := d.shop.Users.Balance(ctx, cb.From.ID)
	if err != nil {
		d.logger.Error("reading balance", "user_id", cb.From.ID, "error", err)
		balance = 0
	}
	d.editCB(ctx, cb, telegramservice.BuyConfirmText(*plan, balance),
		telegramservice.BuyConfirmKeyboard(country, days))
}

// buyExecute runs the order (FR-04 state machine) and reports the outcome.
func (d *Dispatcher) buyExecute(ctx context.Context, cb *models.CallbackQuery, country string, days int) {
	user, err := d.shop.Users.EnsureUser(ctx, cb.From.ID, cb.From.Username, cb.From.FirstName)
	if err != nil {
		d.logger.Error("ensuring user", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat akun, coba lagi ya.")
		return
	}

	if !d.editCB(ctx, cb, "⏳ Memproses order...", nil) {
		return
	}

	res, err := d.shop.Orders.Purchase(ctx, user, country, days)
	switch {
	case err == nil:
		d.send(ctx, cb.Message.Message.Chat.ID,
			telegramservice.BuySuccessText(res.OrderID, res.AccountEmail, days, res.BalanceAfter, res.Plan.CountryName), nil)
	case err == ordersvc.ErrInsufficientBalance:
		d.send(ctx, cb.Message.Message.Chat.ID, insufficientText(), topupHintKeyboard())
	case err == ordersvc.ErrNoServer:
		d.send(ctx, cb.Message.Message.Chat.ID, "Belum ada server tersedia untuk negara ini. Coba lagi nanti ya.", nil)
	case err == ordersvc.ErrPlanNotFound:
		d.send(ctx, cb.Message.Message.Chat.ID, "Paket sudah tidak tersedia.", nil)
	default:
		d.logger.Error("purchase failed", "user_id", cb.From.ID, "country", country, "days", days, "error", err)
		d.send(ctx, cb.Message.Message.Chat.ID,
			telegramservice.BuyFailedText(res.OrderID, "Terjadi kendala saat memproses order di server."), nil)
	}
}

func insufficientText() string {
	return "Saldo kamu tidak cukup untuk paket ini.\n\nSilakan top up saldo dulu ya."
}

func topupHintKeyboard() models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "💳 Top Up", CallbackData: telegramservice.CallbackTopup}},
		{{Text: "🏠 Menu Utama", CallbackData: telegramservice.CallbackHome}},
	}}
}
