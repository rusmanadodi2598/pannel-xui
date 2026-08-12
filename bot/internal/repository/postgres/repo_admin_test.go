// Package postgres_test covers the FR-11 admin persistence (v1.40).
//
// @file      internal/repository/postgres/repo_admin_test.go
// @for       Integration: audit log append/recent, server admin ops, order stats.
// @uses      testing, context, time, internal/domain, internal/repository/postgres
// @reason    The admin trail + server flags + dashboard aggregates must behave
// exactly against PostgreSQL (AGENTS.md §2.1, §1.7).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-12
package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

func TestAuditRepo_GivenRecords_ThenRecentNewestFirst(t *testing.T) {
	repo := openRepo(t)
	ctx := context.Background()
	if err := repo.DB().Exec(`TRUNCATE admin_audit_log RESTART IDENTITY CASCADE`).Error; err != nil {
		t.Fatalf("truncate: %v", err)
	}
	r := repo.Audit()

	for i := 0; i < 3; i++ {
		if err := r.Record(ctx, 7, "price:set", "ID:30", "7000"); err != nil {
			t.Fatalf("Record %d: %v", i, err)
		}
	}
	rows, err := r.Recent(ctx, 10)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(rows) != 3 || rows[0].Action != "price:set" || rows[0].AdminID != 7 {
		t.Errorf("Recent = %+v, want 3 rows newest first", rows)
	}
}

func TestServerRepo_GivenAdminOps_ThenFlagsAndCreateWork(t *testing.T) {
	repo := openRepo(t)
	r := repo.Servers()
	ctx := context.Background()

	seed := postgres.VPNServer{
		Name: "ADM-01", Host: "adm.example.com", Port: 2083, Username: "admin",
		PasswordEnc: "enc", APIPath: "/panel", UseSSL: true,
		CountryCode: "ID", FlagEmoji: "🇮🇩", Protocols: `["vless"]`,
		IsActive: true, IsOpen: true,
	}
	if err := r.UpsertSeed(ctx, seed); err != nil {
		t.Fatalf("UpsertSeed: %v", err)
	}
	all, err := r.ListAll(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("ListAll = %+v, err %v", all, err)
	}
	id := all[0].ID

	if err := r.SetOpen(ctx, id, false); err != nil {
		t.Fatalf("SetOpen: %v", err)
	}
	if err := r.SetActive(ctx, id, false); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	after, err := r.ListAll(ctx)
	if err != nil || after[0].IsOpen || after[0].IsActive {
		t.Errorf("after toggles = %+v, want both false", after)
	}
	// Toggling an unknown id → ErrServerNotFound.
	if err := r.SetOpen(ctx, 99999, true); err != postgres.ErrServerNotFound {
		t.Errorf("SetOpen unknown = %v, want ErrServerNotFound", err)
	}

	// Create a brand-new row; password stays encrypted as given.
	created := &postgres.VPNServer{
		Name: "ADM-02", Host: "adm2.example.com", Port: 8443, Username: "root",
		PasswordEnc: "sealed", APIPath: "/", UseSSL: true,
		CountryCode: "SG", IsActive: true, IsOpen: true,
	}
	if err := r.Create(ctx, created); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 {
		t.Error("created id = 0")
	}
	dup, err := r.FindByHostPort(ctx, "adm2.example.com", 8443, "root")
	if err != nil || dup == nil || dup.ID != created.ID {
		t.Errorf("FindByHostPort = %+v, err %v", dup, err)
	}
}

func TestOrderRepo_GivenOrders_ThenStatsAggregate(t *testing.T) {
	repo := openRepo(t)
	ctx := context.Background()
	userRepo := repo.User()
	u, err := userRepo.FindOrCreate(ctx, 424242, "stats-user", "Stats")
	if err != nil {
		t.Fatalf("FindOrCreate: %v", err)
	}
	orderRepo := repo.Orders()
	mk := func(id, status string) postgres.Order {
		return postgres.Order{
			OrderID: id, OrderType: string(domain.OrderTypePurchase),
			UserID: u.ID, Status: status, FinalAmount: 7000,
		}
	}
	o1 := mk("KTS-STATS001-VPN", string(domain.OrderCompleted))
	o2 := mk("KTS-STATS002-VPN", string(domain.OrderFailed))
	if err := orderRepo.Create(ctx, &o1); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := orderRepo.Create(ctx, &o2); err != nil {
		t.Fatalf("Create 2: %v", err)
	}

	stats, err := orderRepo.Stats(ctx, time.Local)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalOrders < 2 || stats.Completed < 1 || stats.Failed < 1 {
		t.Errorf("stats = %+v, want >=2 orders with >=1 completed + >=1 failed", stats)
	}
	if stats.TotalRevenue < 7000 {
		t.Errorf("revenue = %d, want >= 7000", stats.TotalRevenue)
	}
	recent, err := orderRepo.RecentOrders(ctx, 5)
	if err != nil || len(recent) == 0 {
		t.Fatalf("RecentOrders = %+v, err %v", recent, err)
	}
}
