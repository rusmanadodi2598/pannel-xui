// Package postgres also hosts the schema migration runner.
//
// @file      internal/repository/postgres/migrate.go
// @for       Apply embedded SQL migrations (golang-migrate, iofs source).
// @uses      github.com/golang-migrate/migrate/v4, github.com/golang-migrate/migrate/v4/source/iofs, database/postgres driver, bot/migrations
// @reason    Applies versioned schema at boot so the DB matches the code (AGENTS.md §1.7).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package postgres

import (
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // postgres driver
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/kentangtech/bot-order/migrations"
)

// MigrateUp applies all pending embedded migrations, failing fast on errors.
func MigrateUp(dsn string) error {
	return runMigration(dsn, func(m *migrate.Migrate) error {
		return m.Up()
	})
}

// MigrateDown rolls back all applied migrations (used by tests and recovery).
func MigrateDown(dsn string) error {
	return runMigration(dsn, func(m *migrate.Migrate) error {
		return m.Down()
	})
}

// runMigration executes fn against an embedded-source migrate handle.
// Runs synchronously (boot-time, DB already pinged by Open) with panic
// recovery per AGENTS.md §1.6 — no goroutine, so Close can never race.
// ErrNoChange (nothing pending) is treated as success.
func runMigration(dsn string, fn func(*migrate.Migrate) error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic during migration: %v", r)
		}
	}()

	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("loading embedded migrations: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, dsn)
	if err != nil {
		return fmt.Errorf("initializing migrate: %w", err)
	}
	defer m.Close()

	if err := fn(m); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("running migrations: %w", err)
	}
	return nil
}
