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
	"strings"

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

// route dispatches by update kind: /start, /cancel, FSM-aware text or callback.
func (d *Dispatcher) route(ctx context.Context, upd *models.Update) {
	switch {
	case upd.Message != nil && upd.Message.Text == "/start":
		d.handleStart(ctx, upd.Message)
	case upd.Message != nil && upd.Message.Text == "/cancel":
		d.handleCancel(ctx, upd.Message)
	case upd.Message != nil && upd.Message.Text == "/trial":
		// FR-07: /trial opens the trial menu directly.
		if d.shop != nil && d.shop.Trials != nil && d.shop.TrialLm != nil && d.shop.TrialLm.Enabled() {
			d.trialMenuSend(ctx, upd.Message)
			return
		}
		d.send(ctx, upd.Message.Chat.ID, telegramservice.TrialDisabledText(), nil)
	case upd.Message != nil && upd.Message.Text == "/admin":
		// FR-11: /admin opens the admin panel (ADMIN_IDS only).
		d.handleAdmin(ctx, upd.Message)
	case upd.Message != nil && upd.Message.Text != "":
		// FSM-aware: pending topup custom-input or admin input consumes the text.
		if d.topupHandleText(ctx, upd.Message) {
			return
		}
		if d.adminHandleText(ctx, upd.Message) {
			return
		}
		// Unrecognized text only answers in private chats — never in a group,
		// where the bot would pollute shared conversation with hints.
		if upd.Message.Chat.Type == "private" {
			d.send(ctx, upd.Message.Chat.ID, telegramservice.HelpHintText(), nil)
		}
	case upd.CallbackQuery != nil:
		d.handleCallback(ctx, upd.CallbackQuery)
	default:
		d.logger.Debug("unhandled update", "update_id", upd.ID)
	}
}

// handleCancel aborts any pending flow (FR-06 custom / FR-11 admin input) and shows home.
func (d *Dispatcher) handleCancel(ctx context.Context, msg *models.Message) {
	d.topupClearFSM(ctx, msg.From.ID)
	d.adminClearFSM(ctx, msg.From.ID)
	d.send(ctx, msg.Chat.ID, telegramservice.TopupCancelledText(), telegramservice.HomeKeyboard())
}

// handleStart answers FR-01 onboarding with the FR-02 main menu.
// Plain text, no parse mode: usernames may contain markdown-special characters.
// Any pending flow (topup custom input) is aborted — /start always restarts clean.
func (d *Dispatcher) handleStart(ctx context.Context, msg *models.Message) {
	d.topupClearFSM(ctx, msg.From.ID)
	d.adminClearFSM(ctx, msg.From.ID)
	d.send(ctx, msg.Chat.ID, telegramservice.HomeText(firstName(msg.From)), telegramservice.HomeKeyboard())
}

// handleCallback routes inline button taps.
func (d *Dispatcher) handleCallback(ctx context.Context, cb *models.CallbackQuery) {
	switch {
	case cb.Data == telegramservice.CallbackGateCheck:
		d.handleGateCheck(ctx, cb)
	case cb.Data == telegramservice.CallbackHome:
		// Main menu re-renders in place (FR-02 AC).
		d.editHome(ctx, cb)
	case strings.HasPrefix(cb.Data, "topup:"):
		if d.topup != nil {
			d.routeTopup(ctx, cb)
			return
		}
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
	case strings.HasPrefix(cb.Data, "buy:") || strings.HasPrefix(cb.Data, "renew:") ||
		cb.Data == telegramservice.CallbackAccount || strings.HasPrefix(cb.Data, "account:") ||
		strings.HasPrefix(cb.Data, "history:"):
		if d.shop != nil {
			d.routeShop(ctx, cb)
			return
		}
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
	case strings.HasPrefix(cb.Data, "trial:"):
		// FR-07: trial flow (nil-safe — lands when the trial service is wired).
		if d.shop != nil && d.shop.Trials != nil && d.shop.TrialLm != nil {
			d.routeTrial(ctx, cb)
			return
		}
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
	case strings.HasPrefix(cb.Data, telegramservice.PrefixHelp):
		// FR-15: static help/ToS content — no service seam (edit-in-place).
		d.handleHelp(ctx, cb, cb.Data)
	case strings.HasPrefix(cb.Data, "admin:"):
		// FR-11: admin panel (nil-safe; non-admins are denied inside routeAdmin).
		if d.admin != nil {
			d.routeAdmin(ctx, cb)
			return
		}
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
	default:
		// Known menu buttons whose feature lands in later milestones answer noop.
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
	}
}

// handleGateCheck re-verifies group membership without the cache (FR-01).
func (d *Dispatcher) handleGateCheck(ctx context.Context, cb *models.CallbackQuery) {
	if d.gate.Enabled() {
		switch d.gate.CheckFresh(ctx, cb.From.ID) {
		case telegramservice.GateAllowed:
			d.editHome(ctx, cb)
			return
		case telegramservice.GateDenied:
			d.answer(ctx, cb.ID, "Kamu belum join grup.")
			return
		default:
			d.answer(ctx, cb.ID, "Gagal verifikasi, coba lagi ya.")
			return
		}
	}
	d.editHome(ctx, cb)
}

// editHome re-renders the main menu in place (FR-02 AC: edit, not resend).
func (d *Dispatcher) editHome(ctx context.Context, cb *models.CallbackQuery) {
	msg := cb.Message.Message
	if msg == nil {
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
		return
	}
	d.edit(ctx, msg.Chat.ID, msg.ID, telegramservice.HomeText(firstName(&cb.From)), telegramservice.HomeKeyboard())
	d.answer(ctx, cb.ID, "")
}
