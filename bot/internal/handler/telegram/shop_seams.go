// Package telegramhandler also hosts the shop service seams.
//
// @file      internal/handler/telegram/shop_seams.go
// @for       Narrow interfaces the shop flows depend on (FR-03/05/07/08/14).
// @uses      context, internal/domain, internal/repository/postgres,
// internal/service/order, internal/service/server
// @reason    Split from shop.go to respect the 250-line limit (§1.1) — the
// seams stay co-located with the Shop struct they serve (AGENTS.md §1.5).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import (
	"context"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	ordersvc "github.com/kentangtech/bot-order/internal/service/order"
	serversvc "github.com/kentangtech/bot-order/internal/service/server"
)

// PlanReader reads enabled plans (pricingsvc.Service).
type PlanReader interface {
	ListEnabled(ctx context.Context) ([]domain.VpnPlan, error)
	GetPlan(ctx context.Context, country string, days int) (*domain.VpnPlan, error)
}

// ServerReader lists buyable panels + their real inbounds (serversvc.Service).
type ServerReader interface {
	ListBuyable(ctx context.Context) ([]postgres.ServerView, error)
	ListInbounds(ctx context.Context, serverID int64) ([]serversvc.InboundOption, error)
}

// UserReader ensures/reads users (usersvc.Service).
type UserReader interface {
	EnsureUser(ctx context.Context, tgID int64, username, firstName string) (*postgres.User, error)
	Balance(ctx context.Context, tgID int64) (domain.Money, error)
}

// OrderRunner executes purchases & renewals (ordersvc.Service). The buy flow
// pins serverID + inboundID + protocol picked by the user (FR-03).
type OrderRunner interface {
	Purchase(ctx context.Context, user *postgres.User, country string, days, serverID, inboundID int, protocol string) (*ordersvc.PurchaseResult, error)
	Renew(ctx context.Context, user *postgres.User, clientID int64, country string, days int) (*ordersvc.PurchaseResult, error)
	// RecordDeletion logs an account deletion as a completed order row so it
	// appears in the user's Riwayat (FR-08 AC-4, FR-14).
	RecordDeletion(ctx context.Context, userID, serverID int64, protocol, email string) error
}

// ClientReader lists a user's clients (paged, FR-08 AC-1), reads single owned
// views, and deletes owned rows (FR-08 AC-4) — postgres.ClientRepo.
type ClientReader interface {
	ListByUser(ctx context.Context, userID int64, limit int) ([]postgres.ClientView, error)
	ListByUserPage(ctx context.Context, userID int64, limit, offset int) ([]postgres.ClientView, error)
	CountByUser(ctx context.Context, userID int64) (int64, error)
	GetViewOwned(ctx context.Context, clientID, userID int64) (postgres.ClientView, error)
	DeleteOwned(ctx context.Context, clientID, userID int64) error
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

// HistoryReader reads a user's orders for the FR-14 history view — paged list
// (newest first) + count for pagination + ownership-guarded detail
// (postgres.OrderRepo implements it).
type HistoryReader interface {
	CountByUser(ctx context.Context, userID int64) (int64, error)
	ListByUserPage(ctx context.Context, userID int64, limit, offset int) ([]postgres.Order, error)
	GetOwned(ctx context.Context, orderID, userID int64) (*postgres.Order, error)
}

// ClientDeleter removes a client from the panel (FR-08 AC-4). serversvc.Service
// implements it — the handler orchestrates panel-first, DB-after (the DB row
// is a mirror, never the source of truth).
type ClientDeleter interface {
	DeleteClient(ctx context.Context, serverID int64, inboundID int, clientID string) error
}

// TrafficRefresher syncs one client's live usage from its panel on demand
// (FR-08 AC-3). trafficsvc.Service implements it; the handler renders the
// last known values when the refresh fails (best effort).
type TrafficRefresher interface {
	RefreshClient(ctx context.Context, clientID, serverID int64, email string) error
}
