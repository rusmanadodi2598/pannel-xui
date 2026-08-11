// Package telegramhandler also hosts the shop (buy/renew/accounts) flows.
//
// @file      internal/handler/telegram/shop.go
// @for       FR-03/FR-05/FR-08 callback routing + service seams for the auto-order flow.
// @uses      context, strings, github.com/go-telegram/bot/models, internal/domain,
// internal/repository/postgres, internal/service/order
// @reason    Keeps the dispatcher thin: shop flows live here with narrow seams
// so they are unit-testable without DB/network (AGENTS.md §1.5).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	ordersvc "github.com/kentangtech/bot-order/internal/service/order"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

// Shop holds the auto-order service seams (implemented by service packages).
type Shop struct {
	Plans   PlanReader
	Servers ServerReader
	Users   UserReader
	Orders  OrderRunner
	Clients ClientReader
	Trials  TrialRunner  // FR-07: create trial accounts
	TrialLm TrialLimiter // FR-07: daily limit + enabled flag
}

// PlanReader reads enabled plans (pricingsvc.Service).
type PlanReader interface {
	ListEnabled(ctx context.Context) ([]domain.VpnPlan, error)
	GetPlan(ctx context.Context, country string, days int) (*domain.VpnPlan, error)
}

// ServerReader lists buyable panels (serversvc.Service).
type ServerReader interface {
	ListBuyable(ctx context.Context) ([]postgres.ServerView, error)
}

// UserReader ensures/reads users (usersvc.Service).
type UserReader interface {
	EnsureUser(ctx context.Context, tgID int64, username, firstName string) (*postgres.User, error)
	Balance(ctx context.Context, tgID int64) (domain.Money, error)
}

// OrderRunner executes purchases & renewals (ordersvc.Service).
type OrderRunner interface {
	Purchase(ctx context.Context, user *postgres.User, country string, days int) (*ordersvc.PurchaseResult, error)
	Renew(ctx context.Context, user *postgres.User, clientID int64, country string, days int) (*ordersvc.PurchaseResult, error)
}

// ClientReader lists a user's clients (postgres.ClientRepo).
type ClientReader interface {
	ListByUser(ctx context.Context, userID int64, limit int) ([]postgres.ClientView, error)
}

// TrialRunner creates free trial accounts (ordersvc.Service implements it).
type TrialRunner interface {
	CreateTrial(ctx context.Context, user *postgres.User, serverID int64, spec ordersvc.TrialSpec) (*ordersvc.PurchaseResult, error)
}

// TrialLimiter enforces the daily trial policy (trialsvc.Service implements it).
type TrialLimiter interface {
	Enabled() bool
	Limit() int
	Hours() int
	TrafficGB() int
	IPLimit() int
	Remaining(ctx context.Context, userID int64) (int, error)
	Claim(ctx context.Context, userID int64) (int, error)
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
	default:
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
	}
}

// handleBuy routes the buy flow steps (FR-03).
func (d *Dispatcher) handleBuy(ctx context.Context, cb *models.CallbackQuery, data string) {
	switch {
	case data == telegramservice.CallbackBuy:
		d.buyMenu(ctx, cb)
	case strings.HasPrefix(data, telegramservice.PrefixBuyCountry):
		d.buyCountry(ctx, cb, strings.TrimPrefix(data, telegramservice.PrefixBuyCountry))
	case strings.HasPrefix(data, telegramservice.PrefixBuyPlan):
		country, days, ok := parsePlanData(strings.TrimPrefix(data, telegramservice.PrefixBuyPlan))
		if !ok {
			d.answer(ctx, cb.ID, telegramservice.UnavailableText())
			return
		}
		d.buyConfirm(ctx, cb, country, days)
	case strings.HasPrefix(data, telegramservice.PrefixBuyConfirm):
		country, days, ok := parsePlanData(strings.TrimPrefix(data, telegramservice.PrefixBuyConfirm))
		if !ok {
			d.answer(ctx, cb.ID, telegramservice.UnavailableText())
			return
		}
		d.buyExecute(ctx, cb, country, days)
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

// parsePlanData splits "<CODE>:<DAYS>" into its parts.
func parsePlanData(raw string) (country string, days int, ok bool) {
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return "", 0, false
	}
	days, err := atoi(parts[1])
	if err != nil || days <= 0 {
		return "", 0, false
	}
	return strings.ToUpper(parts[0]), days, true
}

// parseRenewData splits "<CLIENTID>:<CODE>:<DAYS>".
func parseRenewData(raw string) (clientID int64, country string, days int, ok bool) {
	parts := strings.Split(raw, ":")
	if len(parts) != 3 {
		return 0, "", 0, false
	}
	clientID, err := parseID64(parts[0])
	if err != nil {
		return 0, "", 0, false
	}
	days, err = atoi(parts[2])
	if err != nil || days <= 0 {
		return 0, "", 0, false
	}
	return clientID, strings.ToUpper(parts[1]), days, true
}

func parseID(raw string) int64 {
	id, _ := parseID64(raw)
	return id
}
