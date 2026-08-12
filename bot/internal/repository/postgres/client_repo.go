// Package postgres also hosts the vpn_clients repository.
//
// @file      internal/repository/postgres/client_repo.go
// @for       Create/list/update VPN clients, server display join (FR-08), expiry-reminder (FR-09) & traffic-sync (PRD §16.2) queries.
// @uses      context, errors, fmt, strings, time, gorm.io/gorm
// @reason    Client records mirror panel state; queries are bounded & indexed (AGENTS.md §1.7).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ErrClientNotFound is returned for missing or foreign-owned clients.
var ErrClientNotFound = errors.New("client not found")

// ClientRepo persists VPN clients.
type ClientRepo struct{ db *gorm.DB }

// NewClientRepo builds the repository on the shared GORM handle.
func NewClientRepo(db *gorm.DB) *ClientRepo { return &ClientRepo{db: db} }

// Create inserts one client row.
func (r *ClientRepo) Create(ctx context.Context, c *VPNClient) error {
	if err := r.db.WithContext(ctx).Create(c).Error; err != nil {
		return fmt.Errorf("creating client: %w", err)
	}
	return nil
}

// ListByUser returns the user's first page of clients, newest first
// (FR-08 list, bounded to limit) — delegate to the paged query so the
// renew/account views share one paginated path (FR-08 AC-1).
func (r *ClientRepo) ListByUser(ctx context.Context, userID int64, limit int) ([]ClientView, error) {
	return r.ListByUserPage(ctx, userID, limit, 0)
}

// GetViewOwned returns one client joined with its server display fields, only
// when it belongs to the user (FR-08 detail/export, M7).
func (r *ClientRepo) GetViewOwned(ctx context.Context, clientID, userID int64) (ClientView, error) {
	var rows []ClientView
	err := r.db.WithContext(ctx).
		Table("vpn_clients AS c").
		Select("c.*, s.name AS server_name, s.host AS server_host, s.flag_emoji, s.country_code").
		Joins("LEFT JOIN vpn_servers AS s ON s.id = c.server_id").
		Where("c.id = ? AND c.user_id = ?", clientID, userID).
		Limit(1).
		Scan(&rows).Error
	if err != nil {
		return ClientView{}, fmt.Errorf("getting client view %d: %w", clientID, err)
	}
	if len(rows) == 0 {
		return ClientView{}, ErrClientNotFound
	}
	return rows[0], nil
}

// GetOwned returns one client only if it belongs to the user.
func (r *ClientRepo) GetOwned(ctx context.Context, clientID, userID int64) (*VPNClient, error) {
	var c VPNClient
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", clientID, userID).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrClientNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting client %d: %w", clientID, err)
	}
	return &c, nil
}

// DeleteOwned removes one client row only when it belongs to the user
// (FR-08 AC-4). The caller must already have deleted the panel client — the
// DB row is the local mirror, never the source of truth.
func (r *ClientRepo) DeleteOwned(ctx context.Context, clientID, userID int64) error {
	res := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", clientID, userID).
		Delete(&VPNClient{})
	if res.Error != nil {
		return fmt.Errorf("deleting client %d: %w", clientID, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrClientNotFound
	}
	return nil
}

// UpdateExpiry extends the client validity and optional traffic limit (FR-05).
// The expiry-notification flag is reset so renewal restarts the H-7/H-3/H-1 cycle.
func (r *ClientRepo) UpdateExpiry(ctx context.Context, clientID int64, expiresAt time.Time, trafficLimit *int64) error {
	updates := map[string]any{"expires_at": expiresAt, "is_expired": false, "notified_expiry": 0, "updated_at": time.Now()}
	if trafficLimit != nil {
		updates["traffic_limit"] = *trafficLimit
	}
	if err := r.db.WithContext(ctx).Model(&VPNClient{}).
		Where("id = ?", clientID).Updates(updates).Error; err != nil {
		return fmt.Errorf("updating client expiry: %w", err)
	}
	return nil
}

// CountActive counts all live clients across servers (FR-11 admin dashboard).
func (r *ClientRepo) CountActive(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&VPNClient{}).
		Where("is_active = true AND (is_expired = false OR expires_at > now())").
		Count(&n).Error
	if err != nil {
		return 0, fmt.Errorf("counting active clients: %w", err)
	}
	return n, nil
}

// CountActiveByUser counts non-expired, non-trial clients (renew menu sizing).
func (r *ClientRepo) CountActiveByUser(ctx context.Context, userID int64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&VPNClient{}).
		Where("user_id = ? AND is_active = true AND is_trial = false AND (is_expired = false OR expires_at > now())", userID).
		Count(&n).Error
	if err != nil {
		return 0, fmt.Errorf("counting active clients: %w", err)
	}
	return n, nil
}

// ExpiryCandidate is a vpn_clients row joined with the owner's Telegram chat ID,
// shaped for the FR-09 expiry-reminder worker (H-7/H-3/H-1).
type ExpiryCandidate struct {
	ClientID    int64
	TelegramID  int64
	Email       string
	ServerName  string
	ExpiresAt   time.Time
	NotifiedDay int
}

// ListExpiryCandidates returns active, unexpired, non-trial clients whose
// remaining time falls in the exclusive window (lower, upper] days and that
// have not been notified for the upper window yet (FR-09 AC: send once per
// threshold). Ordered by soonest expiry, bounded to limit (AGENTS.md §1.7).
func (r *ClientRepo) ListExpiryCandidates(ctx context.Context, upperDays, lowerDays, limit int) ([]ExpiryCandidate, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows []ExpiryCandidate
	err := r.db.WithContext(ctx).
		Table("vpn_clients AS c").
		Select("c.id AS client_id, u.telegram_id, c.email, COALESCE(s.name, '') AS server_name, c.expires_at, c.notified_expiry AS notified_day").
		Joins("JOIN users AS u ON u.id = c.user_id").
		Joins("LEFT JOIN vpn_servers AS s ON s.id = c.server_id").
		Where("c.is_active = true AND c.is_expired = false AND c.is_trial = false").
		Where("c.expires_at > now() + make_interval(days => ?)", lowerDays).
		Where("c.expires_at <= now() + make_interval(days => ?)", upperDays).
		Where("c.notified_expiry <> ?", upperDays).
		Order("c.expires_at ASC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("listing expiry candidates: %w", err)
	}
	return rows, nil
}

// MarkNotified records the notification-window day already sent for a client
// (FR-09 AC). Renewal resets it via UpdateExpiry; a send failure never marks.
func (r *ClientRepo) MarkNotified(ctx context.Context, clientID int64, day int) error {
	if err := r.db.WithContext(ctx).Model(&VPNClient{}).
		Where("id = ?", clientID).Update("notified_expiry", day).Error; err != nil {
		return fmt.Errorf("marking client %d notified: %w", clientID, err)
	}
	return nil
}

// TrafficCandidate is a live client eligible for the traffic-sync sweep
// (PRD §16.2): active, not expired, on an active panel.
type TrafficCandidate struct {
	ClientID int64
	ServerID int64
	Email    string
}

// ListTrafficCandidates returns active clients of active panels for the
// traffic-sync sweep, oldest-synced first (round-robin fairness across sweeps
// when the batch is smaller than the fleet), bounded to limit (AGENTS.md §1.7).
func (r *ClientRepo) ListTrafficCandidates(ctx context.Context, limit int) ([]TrafficCandidate, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	var rows []TrafficCandidate
	err := r.db.WithContext(ctx).
		Table("vpn_clients AS c").
		Select("c.id AS client_id, c.server_id, c.email").
		Joins("JOIN vpn_servers AS s ON s.id = c.server_id").
		Where("c.is_active = true AND c.is_expired = false AND s.is_active = true").
		Order("c.last_sync ASC NULLS FIRST, c.id").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("listing traffic candidates: %w", err)
	}
	return rows, nil
}

// TrafficUpdate carries one client's panel traffic for the batch write.
// LastOnline is set only when the client is online right now; nil keeps the
// previous value (COALESCE in the single bulk UPDATE).
type TrafficUpdate struct {
	ClientID   int64
	Up         int64
	Down       int64
	LastOnline *time.Time
}

// SyncTrafficBatch writes the sweep result in ONE statement (anti N+1 §1.7):
// UPDATE ... FROM (VALUES ...) matching each row by id. last_online uses
// COALESCE so offline clients keep their previous online timestamp.
func (r *ClientRepo) SyncTrafficBatch(ctx context.Context, syncedAt time.Time, updates []TrafficUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	var sb strings.Builder
	sb.WriteString(`UPDATE vpn_clients AS c SET
		traffic_up = v.up, traffic_down = v.down, traffic_used = v.up + v.down,
		last_online = COALESCE(v.online, c.last_online), last_sync = ?, updated_at = ?
	FROM (VALUES `)
	args := []any{syncedAt, syncedAt}
	for i, u := range updates {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("(?::bigint, ?::bigint, ?::bigint, ?::timestamptz)")
		args = append(args, u.ClientID, u.Up, u.Down, u.LastOnline)
	}
	sb.WriteString(`) AS v(id, up, down, online)
	WHERE c.id = v.id`)
	if err := r.db.WithContext(ctx).Exec(sb.String(), args...).Error; err != nil {
		return fmt.Errorf("syncing traffic batch: %w", err)
	}
	return nil
}
