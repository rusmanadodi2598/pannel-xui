// Package telegramhandler routes Telegram updates through the middleware chain.
//
// @file      internal/handler/telegram/dispatcher.go
// @for       Update dispatch: ban check → group gate → rate limit → route (PRD §14.2.5).
// @uses      context, strings, log/slog, github.com/go-telegram/bot/models, internal/service/telegram
// @reason    Owns the bot conversation flow; services stay network-free per AGENTS.md §1.5.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"log/slog"

	"github.com/go-telegram/bot/models"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

// API is the messaging seam (implemented by repository/telegram.Client).
type API interface {
	SendMessage(ctx context.Context, chatID int64, text string, parseMode models.ParseMode, markup models.ReplyMarkup) error
	EditMessageText(ctx context.Context, chatID int64, messageID int, text string, parseMode models.ParseMode, markup models.ReplyMarkup) error
	AnswerCallbackQuery(ctx context.Context, callbackID, text string) error
	SendDocument(ctx context.Context, chatID int64, filename string, content []byte, caption string) error
}

// GateChecker is the membership gate seam (telegramservice.GateService).
type GateChecker interface {
	Enabled() bool
	Check(ctx context.Context, userID int64) telegramservice.GateResult
	CheckFresh(ctx context.Context, userID int64) telegramservice.GateResult
}

// BanChecker is the ban seam (telegramservice.BanService).
type BanChecker interface {
	IsBanned(ctx context.Context, userID int64) (bool, error)
}

// RateLimiter is the throttling seam (telegramservice.RateLimiter).
type RateLimiter interface {
	Allow(ctx context.Context, userID int64) (bool, error)
}

// Dispatcher runs the middleware chain and routes each update (FR-01/FR-02).
type Dispatcher struct {
	api       API
	gate      GateChecker
	banned    BanChecker
	limiter   RateLimiter
	logger    *slog.Logger
	groupLink string
	adminIDs  map[int64]struct{}
	shop      *Shop  // M4 auto-order flows (FR-03/FR-05/FR-08)
	topup     *Topup // M5 topup flow, gateway deferred (FR-06)
	admin     *Admin // M6 admin panel (FR-11)
}

// NewDispatcher wires the dispatcher with its middleware dependencies.
// adminIDs (from ADMIN_IDS) bypass the group gate — owner/admins do not need to
// join the discussion group; the ban check still applies (PRD FR-01 deviation).
// shop, topup and admin may be nil in minimal tests that only exercise FR-01/FR-02.
func NewDispatcher(api API, gate GateChecker, banned BanChecker, limiter RateLimiter, logger *slog.Logger, groupLink string, adminIDs []int64, shop *Shop, topup *Topup, admin *Admin) *Dispatcher {
	admins := make(map[int64]struct{}, len(adminIDs))
	for _, id := range adminIDs {
		admins[id] = struct{}{}
	}
	return &Dispatcher{api: api, gate: gate, banned: banned, limiter: limiter, logger: logger, groupLink: groupLink, adminIDs: admins, shop: shop, topup: topup, admin: admin}
}

// Handle applies the middleware chain then routes the update. It is called
// serially per user by the HTTP worker (per-user lock, PRD §14.2.4).
func (d *Dispatcher) Handle(ctx context.Context, upd *models.Update) {
	uid := UserIDOf(upd)
	if uid == 0 {
		d.logger.Debug("ignoring update without user id", "update_id", upd.ID)
		return
	}

	// 1. Ban check (FR-01) — fail closed.
	banned, err := d.banned.IsBanned(ctx, uid)
	if err != nil {
		d.logger.Error("ban check failed, denying", "user_id", uid, "error", err)
		d.reject(ctx, upd, telegramservice.BannedText())
		return
	}
	if banned {
		d.logger.Info("banned user blocked", "user_id", uid)
		d.reject(ctx, upd, telegramservice.BannedText())
		return
	}

	// 2. Membership gate (FR-01) — fail closed, except ADMIN_IDS bypass
	//    (owner/admins are exempt from joining the discussion group; ban still applies).
	if !d.isAdmin(uid) && d.gate.Enabled() {
		switch d.gate.Check(ctx, uid) {
		case telegramservice.GateDenied:
			d.sendJoinPrompt(ctx, upd)
			return
		case telegramservice.GateUnknown:
			d.logger.Error("membership check failed, denying", "user_id", uid)
			d.reject(ctx, upd, "Terjadi kendala saat verifikasi. Coba lagi nanti ya.")
			return
		}
	}

	// 3. Rate limit — fail open (Redis blip must not kill the bot).
	ok, err := d.limiter.Allow(ctx, uid)
	if err != nil {
		d.logger.Error("rate limit check failed, allowing", "user_id", uid, "error", err)
	} else if !ok {
		d.logger.Debug("rate limit hit", "user_id", uid)
		d.reject(ctx, upd, telegramservice.RateLimitText())
		return
	}

	// 4. Route to the feature handler.
	d.route(ctx, upd)
}
