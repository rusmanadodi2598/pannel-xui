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
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	serversvc "github.com/kentangtech/bot-order/internal/service/server"
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

// buyCountry shows the panel's real inbounds (server + protocol) for the
// country — FR-03 step 2: the user picks the exact inbound before the plan.
func (d *Dispatcher) buyCountry(ctx context.Context, cb *models.CallbackQuery, country string) {
	plans, err := d.shop.Plans.ListEnabled(ctx)
	if err != nil {
		d.logger.Error("listing plans", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat daftar harga, coba lagi ya.")
		return
	}
	var countryName string
	for _, p := range plans {
		if p.CountryCode == country {
			countryName = p.CountryName
			break
		}
	}
	servers, err := d.shop.Servers.ListBuyable(ctx)
	if err != nil {
		d.logger.Error("listing servers", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Gagal memuat server, coba lagi ya.")
		return
	}
	var opts []serversvc.InboundOption
	for _, s := range servers {
		if s.CountryCode != country {
			continue
		}
		inbounds, err := d.shop.Servers.ListInbounds(ctx, s.ID)
		if err != nil {
			d.logger.Error("listing inbounds", "user_id", cb.From.ID, "server_id", s.ID, "error", err)
			continue
		}
		opts = append(opts, inbounds...)
	}
	if len(opts) == 0 {
		d.editCB(ctx, cb, "Belum ada protocol tersedia untuk negara ini. Coba lagi nanti ya.", nil)
		return
	}
	d.editCB(ctx, cb, telegramservice.BuyInboundListText(countryName),
		telegramservice.BuyInboundsKeyboard(opts))
}

// buyInbound shows the plans after the user picked server + protocol.
func (d *Dispatcher) buyInbound(ctx context.Context, cb *models.CallbackQuery, serverID, inboundID int, country string) {
	protocol, ok := d.inboundProtocol(ctx, serverID, inboundID)
	if !ok {
		d.answer(ctx, cb.ID, "Protocol tidak tersedia lagi. Silakan pilih ulang.")
		return
	}
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
	d.editCB(ctx, cb, telegramservice.BuyPlanListText(name),
		telegramservice.BuyPlansKeyboard(countryPlans, serverID, inboundID, protocol))
}

// inboundProtocol resolves the protocol of the chosen inbound (FR-03). It
// re-reads live panel state so a stale callback never provisions on a dead
// inbound.
func (d *Dispatcher) inboundProtocol(ctx context.Context, serverID, inboundID int) (string, bool) {
	opts, err := d.shop.Servers.ListInbounds(ctx, int64(serverID))
	if err != nil {
		d.logger.Error("resolving inbound protocol", "user_id", -1, "server_id", serverID, "inbound_id", inboundID, "error", err)
		return "", false
	}
	for _, o := range opts {
		if o.InboundID == inboundID {
			return o.Protocol, true
		}
	}
	return "", false
}

// buyConfirm shows the summary + live balance before explicit confirmation.
// The protocol is re-read from the panel so the summary always matches the
// real inbound the user picked (FR-03).
func (d *Dispatcher) buyConfirm(ctx context.Context, cb *models.CallbackQuery, country string, days, serverID, inboundID int, protocol string) {
	if protocol == "" {
		protocol, _ = d.inboundProtocol(ctx, serverID, inboundID)
	}
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
	d.editCB(ctx, cb, telegramservice.BuyConfirmText(*plan, balance, protocol),
		telegramservice.BuyConfirmKeyboard(country, days, serverID, inboundID, protocol))
}

// buyableServer resolves a server id to its buyable view for display (shared
// by the buy and trial flows).
func (d *Dispatcher) buyableServer(ctx context.Context, serverID int64) (postgres.ServerView, bool) {
	servers, err := d.shop.Servers.ListBuyable(ctx)
	if err != nil {
		return postgres.ServerView{}, false
	}
	for _, s := range servers {
		if s.ID == serverID {
			return s, true
		}
	}
	return postgres.ServerView{}, false
}
