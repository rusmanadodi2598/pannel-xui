// Package postgres also hosts the paged client listing (FR-08 AC-1).
//
// @file      internal/repository/postgres/client_repo_page.go
// @for       CountByUser + ListByUserPage for the 5/page "Akun Saya" list.
// @uses      context, fmt, gorm.io/gorm
// @reason    Split from client_repo.go to respect the 250-line limit (AGENTS.md
// §1.1); explicit offset pagination keeps the request path bounded (§1.7).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package postgres

import (
	"context"
	"fmt"
)

// CountByUser counts the user's clients (FR-08 AC-1 pagination sizing).
func (r *ClientRepo) CountByUser(ctx context.Context, userID int64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&VPNClient{}).Where("user_id = ?", userID).Count(&n).Error
	if err != nil {
		return 0, fmt.Errorf("counting user clients: %w", err)
	}
	return n, nil
}

// ListByUserPage returns one page of the user's clients, newest first
// (FR-08 list, 5/page), joined with server display fields. Limit is bounded
// and offset explicit — no unbounded fetch in a request-serving path (§1.7).
func (r *ClientRepo) ListByUserPage(ctx context.Context, userID int64, limit, offset int) ([]ClientView, error) {
	if limit <= 0 || limit > 50 {
		limit = 5
	}
	if offset < 0 {
		offset = 0
	}
	var rows []ClientView
	err := r.db.WithContext(ctx).
		Table("vpn_clients AS c").
		Select("c.*, s.name AS server_name, s.host AS server_host, s.flag_emoji, s.country_code").
		Joins("LEFT JOIN vpn_servers AS s ON s.id = c.server_id").
		Where("c.user_id = ?", userID).
		Order("c.created_at DESC").
		Limit(limit).
		Offset(offset).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("listing user clients: %w", err)
	}
	return rows, nil
}
