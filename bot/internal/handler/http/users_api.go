// Package httphandler also hosts the /api/v1/users read handlers (PRD §26.5).
//
// @file      internal/handler/http/users_api.go
// @for       User order history + client list by Telegram id (paged).
// @uses      errors, net/http, strconv, time, gorm.io/gorm,
// internal/repository/postgres
// @reason    Read-only user projections — credential-free clients, bounded
// pagination (AGENTS.md §1.7). Split from orders_api.go for the §1.1 limit.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability experimental
// @since     2026-08-18
package httphandler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
	"gorm.io/gorm"
)

// clientDTO is the client response shape — deliberately credential-free (no
// uuid/password/subId/configLink/subscription_url ever leaves the API).
type clientDTO struct {
	ID           int64      `json:"id"`
	UserID       int64      `json:"userId"`
	ServerID     int64      `json:"serverId"`
	ServerName   string     `json:"serverName"`
	CountryCode  string     `json:"countryCode"`
	Email        string     `json:"email"`
	Protocol     string     `json:"protocol"`
	IsActive     bool       `json:"isActive"`
	IsExpired    bool       `json:"isExpired"`
	IsTrial      bool       `json:"isTrial"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"`
	TrafficUsed  int64      `json:"trafficUsed"`
	TrafficLimit int64      `json:"trafficLimit"`
}

func toClientDTO(v postgres.ClientView) clientDTO {
	return clientDTO{
		ID:           v.ID,
		UserID:       v.UserID,
		ServerID:     v.ServerID,
		ServerName:   v.ServerName,
		CountryCode:  v.CountryCode,
		Email:        v.Email,
		Protocol:     v.Protocol,
		IsActive:     v.IsActive,
		IsExpired:    v.IsExpired,
		IsTrial:      v.IsTrial,
		ExpiresAt:    v.ExpiresAt,
		TrafficUsed:  v.TrafficUsed,
		TrafficLimit: v.TrafficLimit,
	}
}

func (o Options) userOrders(w http.ResponseWriter, r *http.Request) {
	userID, ok := o.resolveUser(w, r)
	if !ok {
		return
	}
	page, limit, offset := pagination(r)
	rows, err := o.Orders.ListByUserPage(r.Context(), userID, limit, offset)
	if err != nil {
		o.Logger.Error("api: user orders", "user", userID, "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Failed to list orders")
		return
	}
	total, err := o.Orders.CountByUser(r.Context(), userID)
	if err != nil {
		o.Logger.Error("api: counting user orders", "user", userID, "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Failed to count orders")
		return
	}
	out := make([]orderDTO, 0, len(rows))
	for _, v := range rows {
		out = append(out, toOrderDTO(v))
	}
	writeList(w, http.StatusOK, out, page, limit, total)
}

func (o Options) userClients(w http.ResponseWriter, r *http.Request) {
	userID, ok := o.resolveUser(w, r)
	if !ok {
		return
	}
	page, limit, offset := pagination(r)
	rows, err := o.Clients.ListByUserPage(r.Context(), userID, limit, offset)
	if err != nil {
		o.Logger.Error("api: user clients", "user", userID, "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Failed to list clients")
		return
	}
	total, err := o.Clients.CountByUser(r.Context(), userID)
	if err != nil {
		o.Logger.Error("api: counting user clients", "user", userID, "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Failed to count clients")
		return
	}
	out := make([]clientDTO, 0, len(rows))
	for _, v := range rows {
		out = append(out, toClientDTO(v))
	}
	writeList(w, http.StatusOK, out, page, limit, total)
}

// resolveUser maps the {telegramID} path segment to a user primary key, or
// writes 404/400 and returns ok=false.
func (o Options) resolveUser(w http.ResponseWriter, r *http.Request) (int64, bool) {
	tgID, err := strconv.ParseInt(r.PathValue("telegramID"), 10, 64)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid telegram id")
		return 0, false
	}
	u, err := o.Users.GetByTelegramID(r.Context(), tgID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeAPIError(w, http.StatusNotFound, "USER_NOT_FOUND", "User not found")
		return 0, false
	}
	if err != nil {
		o.Logger.Error("api: resolving user", "telegramId", tgID, "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL", "Failed to resolve user")
		return 0, false
	}
	return u.ID, true
}

// pagination parses ?page & ?limit with sane bounds (AGENTS.md §1.7: bounded).
func pagination(r *http.Request) (page, limit, offset int) {
	page = 1
	limit = 10
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
		page = p
	}
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 50 {
		limit = l
	}
	return page, limit, (page - 1) * limit
}
