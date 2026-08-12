// Package telegramhandler also hosts the shop (buy/renew/accounts) flows.
//
// @file      internal/handler/telegram/shop.go
// @for       FR-03/FR-05/FR-08 callback routing for the auto-order flow.
// @uses      context, strings, github.com/go-telegram/bot/models, internal/service/telegram
// @reason    Keeps the dispatcher thin: shop flows live here with narrow seams
// (see shop_seams.go) so they are unit-testable without DB/network (§1.5).
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

// Shop holds the auto-order service seams (implemented by service packages).
type Shop struct {
	Plans   PlanReader
	Servers ServerReader
	Users   UserReader
	Orders  OrderRunner
	Clients ClientReader
	Trials  TrialRunner      // FR-07: create trial accounts
	TrialLm TrialLimiter     // FR-07: daily limit + enabled flag
	History HistoryReader    // FR-14: user's order history (paged + owned)
	Deleter ClientDeleter    // FR-08 AC-4: remove client from panel + DB
	Traffic TrafficRefresher // FR-08 AC-3: on-demand usage refresh
}

// routeShop dispatches buy/renew/account callbacks to their handlers.
func (d *Dispatcher) routeShop(ctx context.Context, cb *models.CallbackQuery) {
	data := cb.Data
	switch {
	case data == telegramservice.CallbackBuy || strings.HasPrefix(data, "buy:"):
		d.handleBuy(ctx, cb, data)
	case data == telegramservice.CallbackRenew || strings.HasPrefix(data, "renew:"):
		d.handleRenew(ctx, cb, data)
	case data == telegramservice.CallbackAccount:
		d.handleAccount(ctx, cb)
	case data == telegramservice.CallbackAccountNoop:
		// Non-action page indicator: answer without editing (FR-02 AC).
		d.answer(ctx, cb.ID, "")
	case strings.HasPrefix(data, telegramservice.PrefixAccountPage):
		page, ok := parsePage(strings.TrimPrefix(data, telegramservice.PrefixAccountPage))
		if !ok {
			d.answer(ctx, cb.ID, telegramservice.UnavailableText())
			return
		}
		d.accountList(ctx, cb, page)
	case strings.HasPrefix(data, telegramservice.PrefixAccountView):
		id, ok := parseAccountID(strings.TrimPrefix(data, telegramservice.PrefixAccountView))
		if !ok {
			d.answer(ctx, cb.ID, telegramservice.UnavailableText())
			return
		}
		d.accountView(ctx, cb, id)
	case strings.HasPrefix(data, telegramservice.PrefixAccountTraffic):
		id, ok := parseAccountID(strings.TrimPrefix(data, telegramservice.PrefixAccountTraffic))
		if !ok {
			d.answer(ctx, cb.ID, telegramservice.UnavailableText())
			return
		}
		d.accountTraffic(ctx, cb, id)
	case strings.HasPrefix(data, telegramservice.PrefixAccountConfig):
		id, ok := parseAccountID(strings.TrimPrefix(data, telegramservice.PrefixAccountConfig))
		if !ok {
			d.answer(ctx, cb.ID, telegramservice.UnavailableText())
			return
		}
		d.accountConfig(ctx, cb, id)
	case strings.HasPrefix(data, telegramservice.PrefixAccountConvert):
		id, ok := parseAccountID(strings.TrimPrefix(data, telegramservice.PrefixAccountConvert))
		if !ok {
			d.answer(ctx, cb.ID, telegramservice.UnavailableText())
			return
		}
		d.accountConvert(ctx, cb, id)
	case strings.HasPrefix(data, telegramservice.PrefixAccountExport):
		id, ok := parseAccountID(strings.TrimPrefix(data, telegramservice.PrefixAccountExport))
		if !ok {
			d.answer(ctx, cb.ID, telegramservice.UnavailableText())
			return
		}
		d.accountExport(ctx, cb, id)
	case strings.HasPrefix(data, telegramservice.PrefixAccountDeleteConfirm):
		id, ok := parseAccountID(strings.TrimPrefix(data, telegramservice.PrefixAccountDeleteConfirm))
		if !ok {
			d.answer(ctx, cb.ID, telegramservice.UnavailableText())
			return
		}
		if d.shop.Deleter == nil {
			d.answer(ctx, cb.ID, telegramservice.UnavailableText())
			return
		}
		d.accountDeleteConfirm(ctx, cb, id)
	case strings.HasPrefix(data, telegramservice.PrefixAccountDelete):
		id, ok := parseAccountID(strings.TrimPrefix(data, telegramservice.PrefixAccountDelete))
		if !ok {
			d.answer(ctx, cb.ID, telegramservice.UnavailableText())
			return
		}
		d.accountDelete(ctx, cb, id)
	case strings.HasPrefix(data, telegramservice.PrefixHistory):
		d.handleHistory(ctx, cb, data)
	default:
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
	}
}

// handleBuy routes the buy flow steps (FR-03): country → inbound (server +
// protocol) → plan → confirm → execute.
func (d *Dispatcher) handleBuy(ctx context.Context, cb *models.CallbackQuery, data string) {
	switch {
	case data == telegramservice.CallbackBuy:
		d.buyMenu(ctx, cb)
	case strings.HasPrefix(data, telegramservice.PrefixBuyCountry):
		d.buyCountry(ctx, cb, strings.TrimPrefix(data, telegramservice.PrefixBuyCountry))
	case strings.HasPrefix(data, telegramservice.PrefixBuyInbound):
		serverID, inboundID, country, ok := parseBuyInbound(strings.TrimPrefix(data, telegramservice.PrefixBuyInbound))
		if !ok {
			d.answer(ctx, cb.ID, telegramservice.UnavailableText())
			return
		}
		d.buyInbound(ctx, cb, serverID, inboundID, country)
	case strings.HasPrefix(data, telegramservice.PrefixBuyPlan):
		country, days, serverID, inboundID, protocol, ok := parseBuySelection(strings.TrimPrefix(data, telegramservice.PrefixBuyPlan))
		if !ok {
			d.answer(ctx, cb.ID, telegramservice.UnavailableText())
			return
		}
		d.buyConfirm(ctx, cb, country, days, serverID, inboundID, protocol)
	case strings.HasPrefix(data, telegramservice.PrefixBuyConfirm):
		country, days, serverID, inboundID, protocol, ok := parseBuySelection(strings.TrimPrefix(data, telegramservice.PrefixBuyConfirm))
		if !ok {
			d.answer(ctx, cb.ID, telegramservice.UnavailableText())
			return
		}
		d.buyExecute(ctx, cb, country, days, serverID, inboundID, protocol)
	case data == telegramservice.CallbackBuyBack:
		d.editHome(ctx, cb)
	default:
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
	}
}

// handleRenew routes the renew flow steps (FR-05).
func (d *Dispatcher) handleRenew(ctx context.Context, cb *models.CallbackQuery, data string) {
	switch {
	case data == telegramservice.CallbackRenew:
		d.renewMenu(ctx, cb)
	case strings.HasPrefix(data, telegramservice.PrefixRenewClient):
		d.renewPlans(ctx, cb, parseID(strings.TrimPrefix(data, telegramservice.PrefixRenewClient)))
	case strings.HasPrefix(data, telegramservice.PrefixRenewPlan):
		clientID, country, days, ok := parseRenewData(strings.TrimPrefix(data, telegramservice.PrefixRenewPlan))
		if !ok {
			d.answer(ctx, cb.ID, telegramservice.UnavailableText())
			return
		}
		d.renewConfirm(ctx, cb, clientID, country, days)
	case strings.HasPrefix(data, telegramservice.PrefixRenewConfirm):
		clientID, country, days, ok := parseRenewData(strings.TrimPrefix(data, telegramservice.PrefixRenewConfirm))
		if !ok {
			d.answer(ctx, cb.ID, telegramservice.UnavailableText())
			return
		}
		d.renewExecute(ctx, cb, clientID, country, days)
	case data == telegramservice.CallbackRenewBack:
		d.editHome(ctx, cb)
	default:
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
	}
}
