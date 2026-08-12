// Package telegramhandler also hosts the admin panel flows (FR-11, M6).
//
// @file      internal/handler/telegram/admin.go
// @for       FR-11 admin menu + price management (list, set price, toggle, reload).
// @uses      context, strings, github.com/go-telegram/bot/models, internal/domain,
// internal/repository/postgres, internal/service/telegram
// @reason    Keeps the dispatcher thin: admin callbacks route here behind an
//
//	AdminOps seam, unit-testable without DB/network (AGENTS.md §1.5).
//
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability experimental
// @since     2026-08-11
package telegramhandler

import (
	"context"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	"github.com/kentangtech/bot-order/internal/service/server"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

// Admin holds the admin service seams (adminsvc.Service + redis.AdminFSM).
type Admin struct {
	Ops AdminOps
	FSM AdminFSM
}

// AdminOps is the admin side-effect seam (adminsvc.Service implements it).
// Mutating methods carry adminID (the acting Telegram id) so every action is
// recorded in the audit trail (FR-11 AC, v1.40).
type AdminOps interface {
	ListPlans(ctx context.Context) ([]domain.VpnPlan, error)
	GetPlan(ctx context.Context, country string, days int) (*domain.VpnPlan, error)
	SetPrice(ctx context.Context, adminID int64, country string, days int, price domain.Money) error
	SetEnabled(ctx context.Context, adminID int64, country string, days int, enabled bool) error
	ReloadPricing(ctx context.Context, adminID int64) error
	LookupUser(ctx context.Context, tgID int64) (*postgres.User, error)
	BanUser(ctx context.Context, adminID, tgID int64) error
	UnbanUser(ctx context.Context, adminID, tgID int64) error
	Broadcast(ctx context.Context, adminChatID int64, text string) (int, error)
	// AdjustBalance credits (credit=true) or debits (credit=false) a user's
	// balance atomically + ledgered (FR-11, v1.39). Returns the new balance.
	AdjustBalance(ctx context.Context, adminID, tgID int64, amount domain.Money, credit bool) (domain.Money, error)
	// Server management (FR-11, v1.40).
	ListServers(ctx context.Context) ([]postgres.ServerAdminView, error)
	ToggleServerOpen(ctx context.Context, adminID, serverID int64, open bool) error
	ToggleServerActive(ctx context.Context, adminID, serverID int64, active bool) error
	AddServer(ctx context.Context, adminID int64, in serversvc.NewServerInput) (int64, error)
	// Statistik & audit (FR-11, v1.40).
	Stats(ctx context.Context, loc *time.Location) (postgres.OrderStats, error)
	RecentOrders(ctx context.Context, limit int) ([]postgres.Order, error)
	AuditLog(ctx context.Context, limit int) ([]postgres.AdminAuditLog, error)
}

// AdminFSM persists admin free-text input state (redis.AdminFSM).
type AdminFSM interface {
	Set(ctx context.Context, userID int64, state string) error
	Get(ctx context.Context, userID int64) (string, bool, error)
	Clear(ctx context.Context, userID int64) error
}

// handleAdmin answers the /admin command (FR-11). ADMIN_IDS only — a stale
// admin FSM marker is cleared so /admin always restarts clean. When the admin
// service is not wired, answer unavailable instead of a dead menu.
func (d *Dispatcher) handleAdmin(ctx context.Context, msg *models.Message) {
	if d.admin == nil || d.admin.Ops == nil {
		d.send(ctx, msg.Chat.ID, telegramservice.UnavailableText(), nil)
		return
	}
	if !d.isAdmin(msg.From.ID) {
		d.send(ctx, msg.Chat.ID, telegramservice.AdminDeniedText(), nil)
		return
	}
	d.adminClearFSM(ctx, msg.From.ID)
	d.send(ctx, msg.Chat.ID, telegramservice.AdminMenuText(), telegramservice.AdminMenuKeyboard())
}

// routeAdmin dispatches admin:* callbacks. Every surface re-checks ADMIN_IDS:
// the middleware only bypasses the gate, it never grants admin powers.
func (d *Dispatcher) routeAdmin(ctx context.Context, cb *models.CallbackQuery) {
	if !d.isAdmin(cb.From.ID) {
		d.answer(ctx, cb.ID, telegramservice.AdminDeniedText())
		return
	}
	data := cb.Data
	switch {
	case data == telegramservice.CallbackAdminMenu || data == telegramservice.CallbackAdminBack:
		d.adminMenu(ctx, cb)
	case data == telegramservice.CallbackAdminPrice:
		d.adminPrice(ctx, cb)
	case strings.HasPrefix(data, telegramservice.PrefixAdminPlan):
		d.adminPlanDetail(ctx, cb, strings.TrimPrefix(data, telegramservice.PrefixAdminPlan))
	case strings.HasPrefix(data, telegramservice.PrefixAdminSetPrice):
		d.adminSetPrice(ctx, cb, strings.TrimPrefix(data, telegramservice.PrefixAdminSetPrice))
	case strings.HasPrefix(data, telegramservice.PrefixAdminToggle):
		d.adminToggle(ctx, cb, strings.TrimPrefix(data, telegramservice.PrefixAdminToggle))
	case data == telegramservice.CallbackAdminReload:
		d.adminReload(ctx, cb)
	case data == telegramservice.CallbackAdminBroadcast:
		d.adminBroadcastPrompt(ctx, cb)
	case data == telegramservice.CallbackAdminBcastSend:
		d.adminBcastSend(ctx, cb)
	case data == telegramservice.CallbackAdminBcastCancel || data == telegramservice.CallbackAdminCancel:
		d.adminClearToMenu(ctx, cb)
	case data == telegramservice.CallbackAdminBan:
		d.adminBanPrompt(ctx, cb)
	case strings.HasPrefix(data, telegramservice.PrefixAdminBanConfirm):
		d.adminBanConfirm(ctx, cb, strings.TrimPrefix(data, telegramservice.PrefixAdminBanConfirm))
	case data == telegramservice.CallbackAdminUnban:
		d.adminUnbanPrompt(ctx, cb)
	case strings.HasPrefix(data, telegramservice.PrefixAdminUnbanConfirm):
		d.adminUnbanConfirm(ctx, cb, strings.TrimPrefix(data, telegramservice.PrefixAdminUnbanConfirm))
	case data == telegramservice.CallbackAdminSaldo:
		d.adminSaldoMenu(ctx, cb)
	case data == telegramservice.PrefixAdminSaldoKredit:
		d.adminSaldoArm(ctx, cb, true)
	case data == telegramservice.PrefixAdminSaldoDebit:
		d.adminSaldoArm(ctx, cb, false)
	case strings.HasPrefix(data, telegramservice.PrefixAdminSaldoConfirm):
		// Format: admin:saldo:confirm:{kredit|debit}:{tgid}:{amount}
		raw := strings.TrimPrefix(data, telegramservice.PrefixAdminSaldoConfirm)
		credit, payload, ok := strings.Cut(raw, ":")
		if !ok {
			d.answer(ctx, cb.ID, telegramservice.UnavailableText())
			return
		}
		d.adminSaldoConfirm(ctx, cb, credit == "kredit", payload)
	case data == telegramservice.CallbackAdminServers:
		d.adminServers(ctx, cb)
	case data == telegramservice.CallbackAdminServerAdd:
		d.adminServerAddArm(ctx, cb)
	case data == telegramservice.CallbackAdminServerAddConfirm:
		d.adminServerAddConfirm(ctx, cb)
	// Order matters: open/active prefixes are more specific than the generic
	// server prefix (admin:server:open:3 vs admin:server:3).
	case strings.HasPrefix(data, telegramservice.PrefixAdminServerOpen):
		d.adminServerToggle(ctx, cb, strings.TrimPrefix(data, telegramservice.PrefixAdminServerOpen), true)
	case strings.HasPrefix(data, telegramservice.PrefixAdminServerActive):
		d.adminServerToggle(ctx, cb, strings.TrimPrefix(data, telegramservice.PrefixAdminServerActive), false)
	case strings.HasPrefix(data, telegramservice.PrefixAdminServer):
		d.adminServerDetail(ctx, cb, strings.TrimPrefix(data, telegramservice.PrefixAdminServer))
	case data == telegramservice.CallbackAdminStats:
		d.adminStats(ctx, cb)
	case data == telegramservice.CallbackAdminRecentOrders:
		d.adminRecentOrders(ctx, cb)
	case data == telegramservice.CallbackAdminAudit:
		d.adminAudit(ctx, cb)
	default:
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
	}
}

// adminMenu re-renders the admin panel in place.
func (d *Dispatcher) adminMenu(ctx context.Context, cb *models.CallbackQuery) {
	d.adminClearFSM(ctx, cb.From.ID)
	d.editCB(ctx, cb, telegramservice.AdminMenuText(), telegramservice.AdminMenuKeyboard())
}

// adminPrice lists every plan (enabled and disabled).
func (d *Dispatcher) adminPrice(ctx context.Context, cb *models.CallbackQuery) {
	d.adminClearFSM(ctx, cb.From.ID)
	plans, err := d.admin.Ops.ListPlans(ctx)
	if err != nil {
		d.logger.Error("admin list plans failed", "error", err)
		d.editCB(ctx, cb, "Gagal memuat daftar paket. Coba lagi ya.", nil)
		return
	}
	d.editCB(ctx, cb, telegramservice.AdminPriceText(), telegramservice.AdminPriceKeyboard(plans))
}

// adminPlanDetail shows one plan with set-price/toggle actions.
func (d *Dispatcher) adminPlanDetail(ctx context.Context, cb *models.CallbackQuery, raw string) {
	country, days, ok := parsePlanData(raw)
	if !ok {
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
		return
	}
	d.adminClearFSM(ctx, cb.From.ID)
	plan, err := d.admin.Ops.GetPlan(ctx, country, days)
	if err != nil {
		d.logger.Warn("admin plan detail failed", "country", country, "days", days, "error", err)
		d.editCB(ctx, cb, "Paket tidak ditemukan.", telegramservice.AdminPriceKeyboard(nil))
		return
	}
	d.editCB(ctx, cb, telegramservice.AdminPlanDetailText(*plan), telegramservice.AdminPlanDetailKeyboard(country, days))
}

// adminSetPrice arms the FSM and asks for the new price.
func (d *Dispatcher) adminSetPrice(ctx context.Context, cb *models.CallbackQuery, raw string) {
	country, days, ok := parsePlanData(raw)
	if !ok {
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
		return
	}
	if err := d.admin.FSM.Set(ctx, cb.From.ID, "price:"+raw); err != nil {
		d.logger.Error("admin fsm set failed", "user_id", cb.From.ID, "error", err)
		d.answer(ctx, cb.ID, "Terjadi kendala, coba lagi ya.")
		return
	}
	d.editCB(ctx, cb, telegramservice.AdminSetPricePrompt(country, days), nil)
}

// adminToggle flips the plan's sellable state and re-renders the detail.
func (d *Dispatcher) adminToggle(ctx context.Context, cb *models.CallbackQuery, raw string) {
	country, days, ok := parsePlanData(raw)
	if !ok {
		d.answer(ctx, cb.ID, telegramservice.UnavailableText())
		return
	}
	plan, err := d.admin.Ops.GetPlan(ctx, country, days)
	if err != nil {
		d.answer(ctx, cb.ID, "Paket tidak ditemukan.")
		return
	}
	if err := d.admin.Ops.SetEnabled(ctx, cb.From.ID, country, days, !plan.Enabled); err != nil {
		d.logger.Error("admin toggle failed", "country", country, "days", days, "error", err)
		d.answer(ctx, cb.ID, "Gagal mengubah status paket.")
		return
	}
	d.answer(ctx, cb.ID, telegramservice.AdminPlanToggledText(country, days, !plan.Enabled))
	d.adminPlanDetail(ctx, cb, raw)
}

// adminReload reseeds pricing from the seed file.
func (d *Dispatcher) adminReload(ctx context.Context, cb *models.CallbackQuery) {
	if err := d.admin.Ops.ReloadPricing(ctx, cb.From.ID); err != nil {
		d.logger.Error("admin reload pricing failed", "error", err)
		d.answer(ctx, cb.ID, "Gagal reload pricing.")
		return
	}
	d.editCB(ctx, cb, telegramservice.AdminReloadText(), telegramservice.AdminMenuKeyboard())
}
