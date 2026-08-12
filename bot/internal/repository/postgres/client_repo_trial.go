// Package postgres also hosts the trial-cleanup queries (PRD worker list).
//
// @file      internal/repository/postgres/client_repo_trial.go
// @for       Trial cleanup: list expired trial clients still enabled, mark them expired.
// @uses      context, fmt, time, gorm.io/gorm
// @reason    Trial accounts are 1-hour by policy (FR-07); without a cleanup
// sweep they stay enabled on the panel past their window. The worker disables
// them on the panel first, then marks the DB row — this file holds the two
// bounded queries. Split from client_repo.go for the §1.1 line limit.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-12
package postgres

import (
	"context"
	"fmt"
	"time"
)

// ExpiredTrialCandidate is a trial client whose 1-hour window has passed but
// that is still enabled on its panel (is_active=true, is_expired=false).
type ExpiredTrialCandidate struct {
	ClientID int64
	ServerID int64
	Email    string
	Protocol string
}

// ListExpiredTrialCandidates returns enabled trial clients past their expiry,
// soonest-expired first, bounded to limit (AGENTS.md §1.7).
func (r *ClientRepo) ListExpiredTrialCandidates(ctx context.Context, limit int) ([]ExpiredTrialCandidate, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	var rows []ExpiredTrialCandidate
	err := r.db.WithContext(ctx).
		Table("vpn_clients").
		Select("id AS client_id, server_id, email, protocol").
		Where("is_trial = true AND is_active = true AND is_expired = false AND expires_at <= now()").
		Order("expires_at ASC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("listing expired trial candidates: %w", err)
	}
	return rows, nil
}

// MarkTrialExpired flags a trial client expired: is_active=false + is_expired=true
// so it drops out of the active-account views while its row stays for the
// account list (badge Trial + status Expired). Called only after the panel
// disable succeeded — a panel failure never marks the row.
func (r *ClientRepo) MarkTrialExpired(ctx context.Context, clientID int64) error {
	res := r.db.WithContext(ctx).Model(&VPNClient{}).
		Where("id = ? AND is_trial = true", clientID).
		Updates(map[string]any{"is_active": false, "is_expired": true, "updated_at": time.Now()})
	if res.Error != nil {
		return fmt.Errorf("marking trial client %d expired: %w", clientID, res.Error)
	}
	return nil
}
