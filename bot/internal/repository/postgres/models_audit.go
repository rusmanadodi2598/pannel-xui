// Package postgres also hosts the admin audit log model + repository (FR-11, v1.40).
//
// @file      internal/repository/postgres/models_audit.go
// @for       GORM model + repo of `admin_audit_log` — immutable admin action trail.
// @uses      context, fmt, time, gorm.io/gorm
// @reason    Every admin mutation (price, toggle, reload, ban/unban, adjust
// saldo, broadcast, server mgmt) is recorded here so FR-11 AC is auditable.
// Split from models_order.go for the §1.1 line limit.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-12
package postgres

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// AdminAuditLog is one immutable admin action row (FR-11 AC, migration 000005).
type AdminAuditLog struct {
	ID        int64     `gorm:"primaryKey"`
	AdminID   int64     `gorm:"index:idx_admin_audit_admin;not null"`
	Action    string    `gorm:"type:text;not null"` // price:set | plan:toggle | ... (see adminsvc)
	Target    string    `gorm:"type:text;not null;default:''"`
	Detail    string    `gorm:"type:text;not null;default:''"`
	CreatedAt time.Time `gorm:"index:idx_admin_audit_created;type:timestamptz;not null;default:now()"`
}

// TableName keeps GORM on the exact PRD table name.
func (AdminAuditLog) TableName() string { return "admin_audit_log" }

// AuditRepo persists the admin action trail.
type AuditRepo struct{ db *gorm.DB }

// NewAuditRepo builds the repository on the shared GORM handle.
func NewAuditRepo(db *gorm.DB) *AuditRepo { return &AuditRepo{db: db} }

// Record appends one immutable audit row (append-only; failures are logged by
// the caller, never fatal to the admin action itself).
func (r *AuditRepo) Record(ctx context.Context, adminID int64, action, target, detail string) error {
	row := AdminAuditLog{AdminID: adminID, Action: action, Target: target, Detail: detail}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("recording audit %s: %w", action, err)
	}
	return nil
}

// Recent lists the newest audit rows, newest first, bounded (FR-11 audit view).
func (r *AuditRepo) Recent(ctx context.Context, limit int) ([]AdminAuditLog, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	var rows []AdminAuditLog
	err := r.db.WithContext(ctx).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("listing audit log: %w", err)
	}
	return rows, nil
}
