// Package postgres_test also hosts the schema expectations and query helpers.
//
// @file      internal/repository/postgres/schema_test.go
// @for       Shared test helpers (DSN, table/column/index checks) + PRD §13 expectations.
// @uses      testing, database/sql, os, github.com/jackc/pgx/v5/stdlib
// @reason    Keeps migrate_test.go under 250 lines (AGENTS.md §1.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package postgres_test

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver for verification queries
)

// testDSN is the migration test database. Override with TEST_DATABASE_URL.
// Default points to the staging host DB created during setup (bot_test).
func testDSN() string {
	if dsn := os.Getenv("TEST_DATABASE_URL"); dsn != "" {
		return dsn
	}
	return "postgres://bot:bot@127.0.0.1:5432/bot_test?sslmode=disable"
}

// openSQL opens a plain connection for schema verification queries.
func openSQL(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("postgres", testDSN())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("postgres unreachable (is bot_test created?): %v", err)
	}
	return db
}

// tableExists reports whether a table exists in the public schema.
func tableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var n int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public' AND table_name=$1",
		table).Scan(&n)
	if err != nil {
		t.Fatalf("querying table %s: %v", table, err)
	}
	return n > 0
}

// missingColumns returns expected columns absent from the given table.
func missingColumns(t *testing.T, db *sql.DB, table string, want []string) []string {
	t.Helper()
	rows, err := db.Query(
		"SELECT column_name FROM information_schema.columns WHERE table_schema='public' AND table_name=$1",
		table)
	if err != nil {
		t.Fatalf("querying columns of %s: %v", table, err)
	}
	defer rows.Close()

	got := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scanning column: %v", err)
		}
		got[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating columns: %v", err)
	}

	var missing []string
	for _, c := range want {
		if !got[c] {
			missing = append(missing, c)
		}
	}
	return missing
}

// missingIndexes returns expected index names absent from the database.
func missingIndexes(t *testing.T, db *sql.DB, want []string) []string {
	t.Helper()
	var missing []string
	for _, idx := range want {
		var n int
		err := db.QueryRow(
			"SELECT COUNT(*) FROM pg_indexes WHERE schemaname='public' AND indexname=$1",
			idx).Scan(&n)
		if err != nil {
			t.Fatalf("querying index %s: %v", idx, err)
		}
		if n == 0 {
			missing = append(missing, idx)
		}
	}
	return missing
}

// uniqueConstraint reports whether a UNIQUE constraint exists on the table+column
// (checks both table-level UNIQUE and UNIQUE constraint on a single column).
// Column may be "" for a multi-column table-level UNIQUE constraint.
func uniqueConstraint(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	var n int
	if column == "" {
		err := db.QueryRow(`
			SELECT COUNT(*) FROM information_schema.table_constraints tc
			WHERE tc.table_schema='public' AND tc.table_name=$1
			  AND tc.constraint_type='UNIQUE'`, table).Scan(&n)
		if err != nil {
			t.Fatalf("querying unique constraints on %s: %v", table, err)
		}
		return n > 0
	}
	err := db.QueryRow(`
		SELECT COUNT(*) FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON kcu.constraint_name = tc.constraint_name AND kcu.table_name = tc.table_name
		WHERE tc.table_schema='public' AND tc.table_name=$1
		  AND tc.constraint_type='UNIQUE' AND kcu.column_name=$2`, table, column).Scan(&n)
	if err != nil {
		t.Fatalf("querying unique constraint on %s.%s: %v", table, column, err)
	}
	return n > 0
}

// expectedUniques maps table → column for the UNIQUE constraints PRD §13 requires.
// Empty column means a table-level UNIQUE (multi-column) constraint.
var expectedUniques = map[string]string{
	"users":       "telegram_id",
	"vpn_clients": "email",
	"orders":      "order_id",
	"payments":    "order_id",
	"pricing":     "", // UNIQUE (country_code, plan_days)
}

// expectedTables maps each PRD §13 table to its required columns.
var expectedTables = map[string][]string{
	"users": {
		"id", "telegram_id", "username", "first_name", "last_name", "phone",
		"language", "is_active", "is_banned", "is_admin", "balance",
		"total_spent", "referral_code", "referred_by", "last_active",
		"created_at", "updated_at",
	},
	"vpn_servers": {
		"id", "name", "host", "port", "username", "password_enc", "api_path",
		"use_ssl", "insecure_tls", "country_code", "flag_emoji", "location", "max_clients",
		"current_clients", "is_active", "is_premium", "is_open", "priority",
		"maintenance_message", "protocols", "last_sync", "last_health_check",
		"health_status", "created_at", "updated_at",
	},
	"vpn_clients": {
		"id", "user_id", "server_id", "inbound_id", "email", "uuid", "password",
		"protocol", "flow", "traffic_limit", "traffic_used", "traffic_up",
		"traffic_down", "ip_limit", "is_banned", "is_active", "is_expired",
		"is_trial", "expires_at", "config_link", "subscription_url",
		"subscription_json_url", "sub_id",
		"inbound_network", "inbound_path",
		"notified_expiry", "last_sync", "last_online", "created_at", "updated_at",
	},
	"orders": {
		"id", "order_id", "order_type", "user_id", "server_id", "client_id",
		"protocol", "duration_days", "traffic_gb", "ip_limit", "amount",
		"discount", "final_amount", "currency", "status", "notes",
		"error_message", "account_email", "account_remark", "balance_before",
		"balance_after", "completed_at", "created_at", "updated_at",
	},
	"balance_transactions": {
		"id", "user_id", "order_id", "type", "amount", "balance_after", "created_at",
	},
	"payments": {
		"id", "order_id", "user_id", "telegram_id", "amount_gross", "amount_net",
		"fee_amount", "fee_pct", "provider_ref", "provider_status", "status",
		"paid_at", "created_at", "updated_at",
	},
	"pricing": {
		"id", "country_code", "plan_days", "price", "enabled", "updated_at",
	},
	"admin_audit_log": {
		"id", "admin_id", "action", "target", "detail", "created_at",
	},
}

// expectedIndexes lists the explicit indexes required by PRD §13.
var expectedIndexes = []string{
	"idx_vpn_clients_user_id",
	"idx_vpn_clients_expires_at",
	"idx_vpn_clients_is_trial",
	"idx_orders_user_id",
	"idx_orders_status",
	"idx_orders_created",
	"idx_balance_tx_user",
	"idx_balance_tx_created",
	"idx_payments_user",
	"idx_payments_status",
	"idx_payments_telegram",
	"idx_admin_audit_created",
}
