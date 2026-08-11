// Package postgres_test covers the embedded migration against real PostgreSQL.
//
// @file      internal/repository/postgres/migrate_test.go
// @for       Integration tests: up (tables/columns/indexes per PRD §13), down, idempotent rerun.
// @uses      testing, github.com/jackc/pgx/v5/stdlib (test driver)
// @reason    Verifies the schema contract (AGENTS.md §1.7/§2.1) against the staging DB.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package postgres_test

import (
	"testing"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

func TestMigrateUp_GivenFreshDB_ThenCreatesSevenTables(t *testing.T) {
	db := openSQL(t)
	defer db.Close()

	if err := postgres.MigrateUp(testDSN()); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	t.Cleanup(func() { _ = postgres.MigrateDown(testDSN()) })

	for table := range expectedTables {
		if !tableExists(t, db, table) {
			t.Errorf("table %q missing after MigrateUp", table)
		}
	}
}

func TestMigrateUp_GivenFreshDB_ThenColumnsMatchPRD13(t *testing.T) {
	db := openSQL(t)
	defer db.Close()

	if err := postgres.MigrateUp(testDSN()); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	t.Cleanup(func() { _ = postgres.MigrateDown(testDSN()) })

	for table, want := range expectedTables {
		if missing := missingColumns(t, db, table, want); len(missing) > 0 {
			t.Errorf("table %q missing columns %v (PRD §13)", table, missing)
		}
	}
}

func TestMigrateUp_GivenFreshDB_ThenIndexesCreated(t *testing.T) {
	db := openSQL(t)
	defer db.Close()

	if err := postgres.MigrateUp(testDSN()); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	t.Cleanup(func() { _ = postgres.MigrateDown(testDSN()) })

	if missing := missingIndexes(t, db, expectedIndexes); len(missing) > 0 {
		t.Errorf("missing indexes %v (PRD §13)", missing)
	}
}

func TestMigrateUp_GivenFreshDB_ThenUniqueConstraintsExist(t *testing.T) {
	db := openSQL(t)
	defer db.Close()

	if err := postgres.MigrateUp(testDSN()); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	t.Cleanup(func() { _ = postgres.MigrateDown(testDSN()) })

	for table, column := range expectedUniques {
		if !uniqueConstraint(t, db, table, column) {
			t.Errorf("UNIQUE constraint missing on %s.%s (anti double-order/credit, FR-04/FR-06)", table, column)
		}
	}
}

func TestMigrateUp_GivenAppliedSchema_ThenIdempotent(t *testing.T) {
	db := openSQL(t)
	defer db.Close()

	if err := postgres.MigrateUp(testDSN()); err != nil {
		t.Fatalf("first MigrateUp: %v", err)
	}
	t.Cleanup(func() { _ = postgres.MigrateDown(testDSN()) })

	// Rerun on the applied schema must succeed (ErrNoChange is swallowed).
	if err := postgres.MigrateUp(testDSN()); err != nil {
		t.Fatalf("second MigrateUp must be idempotent, got: %v", err)
	}
}

func TestMigrate_GivenFreshDB_ThenUpDownUpLeavesSchemaConsistent(t *testing.T) {
	db := openSQL(t)
	defer db.Close()
	// Self-contained: whatever happened before, we start from a clean schema
	// and leave a clean schema, so the test never depends on sibling ordering.
	_ = postgres.MigrateDown(testDSN())
	t.Cleanup(func() { _ = postgres.MigrateDown(testDSN()) })

	if err := postgres.MigrateUp(testDSN()); err != nil {
		t.Fatalf("up: %v", err)
	}
	if !tableExists(t, db, "users") {
		t.Fatal("users table expected after up")
	}

	if err := postgres.MigrateDown(testDSN()); err != nil {
		t.Fatalf("down: %v", err)
	}
	if tableExists(t, db, "users") {
		t.Fatal("users table must be dropped after down")
	}

	if err := postgres.MigrateUp(testDSN()); err != nil {
		t.Fatalf("second up: %v", err)
	}
	if !tableExists(t, db, "orders") {
		t.Fatal("orders table expected after second up")
	}
}

func TestMigrateDown_GivenAppliedSchema_ThenDropsAllTables(t *testing.T) {
	db := openSQL(t)
	defer db.Close()

	if err := postgres.MigrateUp(testDSN()); err != nil {
		t.Fatalf("up: %v", err)
	}
	if err := postgres.MigrateDown(testDSN()); err != nil {
		t.Fatalf("down: %v", err)
	}

	for table := range expectedTables {
		if tableExists(t, db, table) {
			t.Errorf("table %q still exists after down", table)
		}
	}
}

func TestMigrate_GivenUnreachableDB_ThenCleanError(t *testing.T) {
	// Points at a closed port; must return a clean error, never panic.
	badDSN := "postgres://bot:bot@127.0.0.1:59999/bot_test?sslmode=disable&connect_timeout=2"
	err := postgres.MigrateUp(badDSN)
	if err == nil {
		t.Fatal("expected error for unreachable DB, got nil")
	}
	t.Logf("got expected error: %v", err)
}
