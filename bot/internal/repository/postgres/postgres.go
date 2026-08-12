// Package postgres implements the PostgreSQL repository layer via GORM.
//
// @file      internal/repository/postgres/postgres.go
// @for       GORM connection with explicit pool limits and health ping.
// @uses      gorm.io/gorm, gorm.io/driver/postgres, context, time
// @reason    Centralizes DB access so services never construct raw connections (AGENTS.md §1.5/§1.7).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package postgres

import (
	"context"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Repository wraps the GORM handle with explicit pool configuration.
type Repository struct {
	db *gorm.DB
}

// PoolOptions carries the connection-pool limits (AGENTS.md §1.7).
type PoolOptions struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// Open connects to PostgreSQL and applies the configured pool limits.
// It pings once so a bad DSN fails fast at boot.
func Open(ctx context.Context, dsn string, pool PoolOptions) (*Repository, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("opening postgres: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("getting sql handle: %w", err)
	}
	sqlDB.SetMaxOpenConns(pool.MaxOpenConns)
	sqlDB.SetMaxIdleConns(pool.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(pool.ConnMaxLifetime)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}
	return &Repository{db: db}, nil
}

// Ping checks database connectivity for the health endpoint.
func (r *Repository) Ping(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// Close releases the connection pool (called on graceful shutdown).
func (r *Repository) Close() error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// DB exposes the raw GORM handle for repositories and future milestones.
func (r *Repository) DB() *gorm.DB {
	return r.db
}

// User returns the user + ledger repository (M4).
func (r *Repository) User() *UserRepo { return NewUserRepo(r.db) }

// Pricing returns the pricing repository (M4).
func (r *Repository) Pricing() *PricingRepo { return NewPricingRepo(r.db) }

// Servers returns the vpn_servers repository (M4, FR-10).
func (r *Repository) Servers() *ServerRepo { return NewServerRepo(r.db) }

// Clients returns the vpn_clients repository (M4, FR-08).
func (r *Repository) Clients() *ClientRepo { return NewClientRepo(r.db) }

// Orders returns the orders repository (M4, FR-04).
func (r *Repository) Orders() *OrderRepo { return NewOrderRepo(r.db) }

// Audit returns the admin audit log repository (FR-11, v1.40).
func (r *Repository) Audit() *AuditRepo { return NewAuditRepo(r.db) }
