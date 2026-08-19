// Package httphandler also hosts the admin REST API (PRD §26.5).
//
// @file      internal/handler/http/api.go
// @for       X-API-Key auth + §26.4 envelope helpers + admin route registration.
// @uses      context, crypto/subtle, net/http, time, internal/domain,
// internal/repository/postgres, internal/service/server, internal/service/topup
// @reason    The deferred /api/v1 admin surface (servers CRUD, orders/users
// read, topup trigger) shares one constant-time key check and one response
// envelope — no ad-hoc auth or error shapes (AGENTS.md §1.3/§1.4).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability experimental
// @since     2026-08-18
package httphandler

import (
	"context"
	"crypto/subtle"
	"net/http"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	serversvc "github.com/kentangtech/bot-order/internal/service/server"
	topupsvc "github.com/kentangtech/bot-order/internal/service/topup"
)

// ServerAdmin is the admin server surface (serversvc.Service implements it).
type ServerAdmin interface {
	ListAll(ctx context.Context) ([]postgres.ServerAdminView, error)
	GetAdminByID(ctx context.Context, id int64) (postgres.ServerAdminView, error)
	AddServer(ctx context.Context, in serversvc.NewServerInput) (int64, error)
	UpdateServer(ctx context.Context, in serversvc.UpdateServerInput) error
	DeleteServer(ctx context.Context, id int64) error
	CheckHealth(ctx context.Context, id int64) (string, error)
}

// OrderAdmin reads order stats and lookups (postgres.OrderRepo implements it).
type OrderAdmin interface {
	Stats(ctx context.Context, loc *time.Location) (postgres.OrderStats, error)
	RecentOrders(ctx context.Context, limit int) ([]postgres.Order, error)
	GetByOrderID(ctx context.Context, orderID string) (*postgres.Order, error)
	ListByUserPage(ctx context.Context, userID int64, limit, offset int) ([]postgres.Order, error)
	CountByUser(ctx context.Context, userID int64) (int64, error)
}

// ClientReader lists a user's clients (postgres.ClientRepo implements it).
type ClientReader interface {
	ListByUserPage(ctx context.Context, userID int64, limit, offset int) ([]postgres.ClientView, error)
	CountByUser(ctx context.Context, userID int64) (int64, error)
}

// UserResolver resolves a Telegram id (postgres.UserRepo implements it).
type UserResolver interface {
	GetByTelegramID(ctx context.Context, tgID int64) (*postgres.User, error)
}

// TopupTrigger starts a topup charge (topupsvc.Service implements it).
type TopupTrigger interface {
	Quote(net domain.Money) (topupsvc.Quote, error)
	CreatePayment(ctx context.Context, req topupsvc.CreatePaymentRequest) (*topupsvc.PaymentResult, error)
}

// withAPIKey guards an admin route with the shared X-API-Key (constant-time).
func withAPIKey(key string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if key == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-API-Key")), []byte(key)) != 1 {
			writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or invalid API key")
			return
		}
		next(w, r)
	}
}

// writeData wraps a single resource in the §26.4 success envelope.
func writeData(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, map[string]any{"data": data})
}

// writeList wraps a paginated list in the §26.4 envelope with page metadata.
func writeList(w http.ResponseWriter, status int, data any, page, limit int, total int64) {
	writeJSON(w, status, map[string]any{
		"data": data,
		"meta": map[string]any{"page": page, "limit": limit, "total": total},
	})
}

// writeAPIError writes the §26.4 error envelope (code + message, English).
func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"code": code, "message": message})
}

// registerAdminAPI wires the deferred §26.5 admin surface. It is a no-op when
// RESTAPIKey is empty — the surface stays disabled by default.
func (o Options) registerAdminAPI(mux *http.ServeMux) {
	if o.RESTAPIKey == "" {
		return
	}
	key := o.RESTAPIKey

	mux.HandleFunc("GET "+apiBase+"/servers", withAPIKey(key, o.listServers))
	mux.HandleFunc("POST "+apiBase+"/servers", withAPIKey(key, o.createServer))
	mux.HandleFunc("GET "+apiBase+"/servers/{id}/health", withAPIKey(key, o.serverHealth))
	mux.HandleFunc("GET "+apiBase+"/servers/{id}", withAPIKey(key, o.getServer))
	mux.HandleFunc("PATCH "+apiBase+"/servers/{id}", withAPIKey(key, o.updateServer))
	mux.HandleFunc("DELETE "+apiBase+"/servers/{id}", withAPIKey(key, o.deleteServer))

	mux.HandleFunc("GET "+apiBase+"/orders", withAPIKey(key, o.listOrders))
	mux.HandleFunc("GET "+apiBase+"/orders/{orderId}", withAPIKey(key, o.getOrder))

	mux.HandleFunc("GET "+apiBase+"/users/{telegramID}/orders", withAPIKey(key, o.userOrders))
	mux.HandleFunc("GET "+apiBase+"/users/{telegramID}/clients", withAPIKey(key, o.userClients))

	mux.HandleFunc("POST "+apiBase+"/payments/topups", withAPIKey(key, o.createTopup))
}
