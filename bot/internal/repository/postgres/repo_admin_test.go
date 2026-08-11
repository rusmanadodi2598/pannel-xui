// Package postgres_test also covers the FR-11 admin repository operations.
//
// @file      internal/repository/postgres/repo_admin_test.go
// @for       Integration tests: pricing admin ops + user ban/list/count (FR-11).
// @uses      testing, context, time, github.com/kentangtech/bot-order/internal/domain,
// github.com/kentangtech/bot-order/internal/repository/postgres
// @reason    Verifies the FR-11 persistence contract on the staging DB.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package postgres_test

import (
	"context"
	"testing"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

func TestPricingRepo_AdminOps_GivenPlans_ThenListGetSetPriceSetEnabled(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	ctx := context.Background()

	rows := []postgres.Pricing{
		{CountryCode: "ID", PlanDays: 15, Price: 4000, Enabled: true},
		{CountryCode: "ID", PlanDays: 30, Price: 7000, Enabled: true},
	}
	if err := r.Pricing().UpsertMany(ctx, rows); err != nil {
		t.Fatalf("UpsertMany: %v", err)
	}

	// Disable via the admin path (GORM omits Enabled:false on INSERT because
	// of the default:true tag — the toggle uses UPDATE, which is correct).
	if err := r.Pricing().SetEnabled(ctx, "ID", 30, false); err != nil {
		t.Fatalf("SetEnabled(false): %v", err)
	}

	// ListAll includes disabled plans.
	all, err := r.Pricing().ListAll(ctx)
	if err != nil || len(all) != 2 {
		t.Fatalf("ListAll = %d, err %v", len(all), err)
	}

	// Get returns the disabled plan too (admin detail).
	p, err := r.Pricing().Get(ctx, "ID", 30)
	if err != nil || p.Enabled {
		t.Fatalf("Get(ID,30) = %+v, err %v (want disabled row)", p, err)
	}

	// SetPrice + SetEnabled persist.
	if err := r.Pricing().SetPrice(ctx, "ID", 30, 7500); err != nil {
		t.Fatalf("SetPrice: %v", err)
	}
	if err := r.Pricing().SetEnabled(ctx, "ID", 30, true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	p, err = r.Pricing().Get(ctx, "ID", 30)
	if err != nil || p.Price != 7500 || !p.Enabled {
		t.Fatalf("after update = %+v, err %v", p, err)
	}

	// Missing plan → ErrPlanNotFound.
	if err := r.Pricing().SetPrice(ctx, "XX", 99, 100); err != postgres.ErrPlanNotFound {
		t.Fatalf("SetPrice missing = %v, want ErrPlanNotFound", err)
	}
}

func TestUserRepo_AdminOps_GivenUsers_ThenSetBannedAndListAndCount(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	ctx := context.Background()

	u1, _ := r.User().FindOrCreate(ctx, 991001, "admin_a", "A")
	u2, _ := r.User().FindOrCreate(ctx, 991002, "admin_b", "B")
	_ = u2

	if n, err := r.User().CountUsers(ctx); err != nil || n != 2 {
		t.Fatalf("CountUsers = %d, err %v", n, err)
	}

	ids, err := r.User().ListTelegramIDs(ctx, 1, 0)
	if err != nil || len(ids) != 1 {
		t.Fatalf("ListTelegramIDs page = %v, err %v", ids, err)
	}
	ids2, err := r.User().ListTelegramIDs(ctx, 1, 1)
	if err != nil || len(ids2) != 1 || ids[0] == ids2[0] {
		t.Fatalf("paging unstable: %v then %v", ids, ids2)
	}

	if err := r.User().SetBanned(ctx, u1.TelegramID, true); err != nil {
		t.Fatalf("SetBanned: %v", err)
	}
	got, err := r.User().GetByTelegramID(ctx, u1.TelegramID)
	if err != nil || !got.IsBanned {
		t.Fatalf("banned flag = %+v, err %v", got, err)
	}
	if err := r.User().SetBanned(ctx, 999999, true); err == nil {
		t.Error("SetBanned on unknown user must error (RowsAffected 0)")
	}

	// Debit guard honors the flag: banned user cannot be charged.
	if _, err := r.User().Debit(ctx, u1.ID, domain.Money(100), "KTS-ADM001-VPN"); err == nil {
		t.Error("debit on banned user must fail (guard)")
	}
}
