// Package postgres_test also covers the shop repositories and shared helpers.
//
// @file      internal/repository/postgres/repo_shop_test.go
// @for       Integration tests: pricing upsert, server seed, client list, order round-trip.
// @uses      testing, context, time, github.com/kentangtech/bot-order/internal/domain,
// github.com/kentangtech/bot-order/internal/repository/postgres
// @reason    Verifies the M4 persistence contract end-to-end on the staging DB.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

func TestPricingRepo_UpsertAndList_GivenSeed_ThenIdempotent(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	ctx := context.Background()

	rows := []postgres.Pricing{
		{CountryCode: "ID", PlanDays: 15, Price: 4000, Enabled: true},
		{CountryCode: "ID", PlanDays: 30, Price: 7000, Enabled: true},
		{CountryCode: "SG", PlanDays: 15, Price: 5000, Enabled: true},
	}
	if err := r.Pricing().UpsertMany(ctx, rows); err != nil {
		t.Fatalf("UpsertMany: %v", err)
	}
	// Rerun with a price change — must update, not duplicate.
	rows[1].Price = 7500
	if err := r.Pricing().UpsertMany(ctx, rows); err != nil {
		t.Fatalf("UpsertMany (2nd): %v", err)
	}

	list, err := r.Pricing().ListEnabled(ctx)
	if err != nil {
		t.Fatalf("ListEnabled: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("rows = %d, want 3 (no duplicates)", len(list))
	}
	plan, err := r.Pricing().GetPlan(ctx, "ID", 30)
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if plan.Price != 7500 {
		t.Errorf("price after upsert = %d, want 7500", plan.Price)
	}
}

func TestServerRepo_UpsertSeed_GivenSameHost_ThenOneRow(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	ctx := context.Background()

	seed := postgres.VPNServer{
		Name: "ID-01", Host: "id.example.com", Port: 443, Username: "admin",
		PasswordEnc: "enc1", APIPath: "/panel", UseSSL: true,
		CountryCode: "ID", FlagEmoji: "🇮🇩", Protocols: `["vless"]`,
		IsActive: true, IsOpen: true, UpdatedAt: time.Now(),
	}
	if err := r.Servers().UpsertSeed(ctx, seed); err != nil {
		t.Fatalf("UpsertSeed: %v", err)
	}
	seed.PasswordEnc = "enc2"
	if err := r.Servers().UpsertSeed(ctx, seed); err != nil {
		t.Fatalf("UpsertSeed (2nd): %v", err)
	}

	buyable, err := r.Servers().ListBuyable(ctx)
	if err != nil {
		t.Fatalf("ListBuyable: %v", err)
	}
	if len(buyable) != 1 {
		t.Fatalf("buyable = %d, want 1", len(buyable))
	}
	srv, err := r.Servers().GetByID(ctx, buyable[0].ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if srv.PasswordEnc != "enc2" {
		t.Errorf("password_enc = %q, want enc2 (updated)", srv.PasswordEnc)
	}
	if got := srv.ProtocolsList(); len(got) != 1 || got[0] != "vless" {
		t.Errorf("protocols = %v", got)
	}
}

func TestOrderClientFlow_GivenPurchase_ThenOrderAndClientAndJoin(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	ctx := context.Background()

	u, err := r.User().FindOrCreate(ctx, 777001, "dewi", "Dewi")
	if err != nil {
		t.Fatalf("FindOrCreate dewi: %v", err)
	}
	server := postgres.VPNServer{
		Name: "SG-01", Host: "sg.example.com", Port: 443, Username: "admin",
		PasswordEnc: "enc", APIPath: "/panel", UseSSL: true,
		CountryCode: "SG", FlagEmoji: "🇸🇬", Protocols: `["vless"]`,
		IsActive: true, IsOpen: true, UpdatedAt: time.Now(),
	}
	if err := r.Servers().UpsertSeed(ctx, server); err != nil {
		t.Fatalf("UpsertSeed: %v", err)
	}
	buyable, _ := r.Servers().ListBuyable(ctx)

	order := postgres.Order{
		OrderID: "KTS-INTEG001-VPN", OrderType: string(domain.OrderTypePurchase),
		UserID: u.ID, ServerID: &buyable[0].ID, Protocol: "vless",
		DurationDays: 30, TrafficGB: 100, IPLimit: 1,
		Amount: 7000, Discount: 0, FinalAmount: 7000, Currency: "IDR",
		Status: string(domain.OrderPending),
	}
	if err := r.Orders().Create(ctx, &order); err != nil {
		t.Fatalf("Order Create: %v", err)
	}
	if order.ID == 0 {
		t.Fatal("order id not populated")
	}

	expiry := time.Now().AddDate(0, 0, 30)
	client := postgres.VPNClient{
		UserID: u.ID, ServerID: buyable[0].ID, InboundID: 5,
		Email: "kts-integ001@vpn.kt", UUID: "uuid-1", Protocol: "vless",
		TrafficLimit: 100 * 1024 * 1024 * 1024, IPLimit: 1, ExpiresAt: &expiry,
	}
	if err := r.Clients().Create(ctx, &client); err != nil {
		t.Fatalf("Client Create: %v", err)
	}

	// Order state transition via Save.
	order.Status = string(domain.OrderCompleted)
	if err := r.Orders().Save(ctx, &order); err != nil {
		t.Fatalf("Order Save: %v", err)
	}
	got, err := r.Orders().GetByOrderID(ctx, "KTS-INTEG001-VPN")
	if err != nil {
		t.Fatalf("GetByOrderID: %v", err)
	}
	if got.Status != string(domain.OrderCompleted) {
		t.Errorf("order status = %s", got.Status)
	}

	// Client list join shows server display fields.
	views, err := r.Clients().ListByUser(ctx, u.ID, 5)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(views) != 1 || views[0].ServerName != "SG-01" || views[0].CountryCode != "SG" {
		t.Errorf("client views = %+v", views)
	}

	// Ownership guard: other user must not see it.
	other, err := r.User().FindOrCreate(ctx, 777002, "eka", "Eka")
	if err != nil {
		t.Fatalf("FindOrCreate eka: %v", err)
	}
	if _, err := r.Clients().GetOwned(ctx, client.ID, other.ID); err == nil {
		t.Error("foreign client must not be readable")
	}
	if _, err := r.Clients().GetOwned(ctx, client.ID, u.ID); err != nil {
		t.Errorf("owner lookup failed: %v", err)
	}

	// Recent orders for user.
	orders, err := r.Orders().ListByUser(ctx, u.ID, 5)
	if err != nil || len(orders) != 1 {
		t.Errorf("orders = %v, err = %v", orders, err)
	}
}

// --- shared helpers ---

// openRepo opens a Repository on the test DSN and migrates up.
func openRepo(t *testing.T) *postgres.Repository {
	t.Helper()
	if err := postgres.MigrateUp(testDSN()); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	t.Cleanup(func() { _ = postgres.MigrateDown(testDSN()) })
	db, err := postgres.Open(context.Background(), testDSN(), postgres.PoolOptions{
		MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf("postgres.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// cleanTables truncates every M4 table (CASCADE resets FKs).
func cleanTables(t *testing.T) {
	t.Helper()
	db := openSQL(t)
	defer db.Close()
	if _, err := db.Exec(`TRUNCATE users, vpn_servers, vpn_clients, orders, balance_transactions, payments, pricing CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

// ledgerCount counts immutable ledger rows for a user.
func ledgerCount(t *testing.T, userID int64) int {
	t.Helper()
	db := openSQL(t)
	defer db.Close()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM balance_transactions WHERE user_id = $1", userID).Scan(&n); err != nil {
		t.Fatalf("ledger count: %v", err)
	}
	return n
}
