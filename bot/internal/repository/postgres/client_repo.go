// Package postgres also hosts the vpn_clients repository.
//
// @file      internal/repository/postgres/client_repo.go
// @for       Create/list/update VPN clients with server display join (PRD §13.3, FR-08).
// @uses      context, errors, fmt, time, gorm.io/gorm
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

// ListByUser returns the user's clients joined with server display fields
// (FR-08 list), newest first, bounded to limit.
func (r *ClientRepo) ListByUser(ctx context.Context, userID int64, limit int) ([]ClientView, error) {
	if limit <= 0 || limit > 50 {
		limit = 5
	}
	var rows []ClientView
	err := r.db.WithContext(ctx).
		Table("vpn_clients AS c").
		Select("c.*, s.name AS server_name, s.flag_emoji, s.country_code").
		Joins("LEFT JOIN vpn_servers AS s ON s.id = c.server_id").
		Where("c.user_id = ?", userID).
		Order("c.created_at DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("listing user clients: %w", err)
	}
	return rows, nil
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

// UpdateExpiry extends the client validity and optional traffic limit (FR-05).
func (r *ClientRepo) UpdateExpiry(ctx context.Context, clientID int64, expiresAt time.Time, trafficLimit *int64) error {
	updates := map[string]any{"expires_at": expiresAt, "is_expired": false, "updated_at": time.Now()}
	if trafficLimit != nil {
		updates["traffic_limit"] = *trafficLimit
	}
	if err := r.db.WithContext(ctx).Model(&VPNClient{}).
		Where("id = ?", clientID).Updates(updates).Error; err != nil {
		return fmt.Errorf("updating client expiry: %w", err)
	}
	return nil
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
