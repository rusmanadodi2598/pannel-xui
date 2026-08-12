// Package postgres also hosts the server health-check queries (PRD §17).
//
// @file      internal/repository/postgres/server_health.go
// @for       Health-check sweep: list active panels, record health_status + last_health_check.
// @uses      context, fmt, time, gorm.io/gorm
// @reason    The buy menu only lists healthy panels (server mati tidak dijual,
// PRD §17); the worker needs a bounded list of active panels and a single
// UPDATE to persist each check. Split from server_repo.go for the §1.1 limit.
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

// HealthTarget is an active panel eligible for the health-check sweep (PRD §17).
type HealthTarget struct {
	ID   int64
	Name string
}

// ListHealthTargets returns active panels for the health-check sweep. Inactive
// servers are excluded (they are already hidden from the buy menu) so the
// worker only pings panels that can actually be sold.
func (r *ServerRepo) ListHealthTargets(ctx context.Context) ([]HealthTarget, error) {
	var rows []HealthTarget
	err := r.db.WithContext(ctx).Table("vpn_servers").
		Select("id, name").
		Where("is_active = true").
		Order("id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("listing health targets: %w", err)
	}
	return rows, nil
}

// UpdateHealth records the last health-check result (PRD §17). status is one
// of the healthsvc constants ("ok"/"down") — the buy menu reads it to stop
// selling unreachable panels.
func (r *ServerRepo) UpdateHealth(ctx context.Context, serverID int64, status string, checkedAt time.Time) error {
	res := r.db.WithContext(ctx).Model(&VPNServer{}).
		Where("id = ?", serverID).
		Updates(map[string]any{
			"health_status":     status,
			"last_health_check": checkedAt,
			"updated_at":        time.Now(),
		})
	if res.Error != nil {
		return fmt.Errorf("updating health for server %d: %w", serverID, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrServerNotFound
	}
	return nil
}
