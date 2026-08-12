// Package postgres_test also covers the FR-14 order history queries.
//
// @file      internal/repository/postgres/repo_history_test.go
// @for       Integration tests: CountByUser, ListByUserPage, GetOwned.
// @uses      testing, context, time, github.com/kentangtech/bot-order/internal/domain,
// github.com/kentangtech/bot-order/internal/repository/postgres
// @reason    Verifies the FR-14 persistence contract on the staging DB: paged
// newest-first reads + ownership guard (AGENTS.md §2.1 Given-When-Then).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

func TestOrderHistory_GivenThreeOrders_ThenCountPageAndOwnership(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	ctx := context.Background()

	u, err := r.User().FindOrCreate(ctx, 778001, "budi", "Budi")
	if err != nil {
		t.Fatalf("FindOrCreate: %v", err)
	}
	other, err := r.User().FindOrCreate(ctx, 778002, "sari", "Sari")
	if err != nil {
		t.Fatalf("FindOrCreate other: %v", err)
	}

	// Three orders with distinct creation times to assert newest-first order.
	base := time.Now().Add(-3 * time.Hour)
	for i, id := range []string{"KTS-HIST01-VPN", "KTS-HIST02-VPN", "KTS-HIST03-VPN"} {
		o := postgres.Order{
			OrderID: id, OrderType: string(domain.OrderTypePurchase), UserID: u.ID,
			Amount: 7000, Discount: 0, FinalAmount: 7000, Currency: "IDR",
			Status:    string(domain.OrderCompleted),
			CreatedAt: base.Add(time.Duration(i) * time.Hour),
		}
		if err := r.Orders().Create(ctx, &o); err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
	}

	// Count sizes pagination.
	n, err := r.Orders().CountByUser(ctx, u.ID)
	if err != nil || n != 3 {
		t.Fatalf("CountByUser = %d, err = %v", n, err)
	}

	// Page 1 (limit 2) returns the two newest; page 2 returns the oldest.
	page1, err := r.Orders().ListByUserPage(ctx, u.ID, 2, 0)
	if err != nil || len(page1) != 2 {
		t.Fatalf("page1 = %d rows, err = %v", len(page1), err)
	}
	if page1[0].OrderID != "KTS-HIST03-VPN" || page1[1].OrderID != "KTS-HIST02-VPN" {
		t.Errorf("page1 order = %s, %s — want newest first", page1[0].OrderID, page1[1].OrderID)
	}
	page2, err := r.Orders().ListByUserPage(ctx, u.ID, 2, 2)
	if err != nil || len(page2) != 1 || page2[0].OrderID != "KTS-HIST01-VPN" {
		t.Fatalf("page2 = %+v, err = %v", page2, err)
	}

	// Limit is bounded by the repo (never an unbounded fetch, §1.7): a zero
	// limit clamps to the default 5, returning every available row (3 here).
	bounded, err := r.Orders().ListByUserPage(ctx, u.ID, 0, 0)
	if err != nil || len(bounded) != 3 {
		t.Errorf("bounded page = %d rows, err = %v (limit 0 clamps to default 5)", len(bounded), err)
	}

	// Ownership guard: owner reads it, a foreign user cannot.
	got, err := r.Orders().GetOwned(ctx, page1[0].ID, u.ID)
	if err != nil || got.OrderID != "KTS-HIST03-VPN" {
		t.Fatalf("owner GetOwned = %+v, err = %v", got, err)
	}
	if _, err := r.Orders().GetOwned(ctx, page1[0].ID, other.ID); !errors.Is(err, postgres.ErrOrderNotFound) {
		t.Errorf("foreign GetOwned err = %v, want ErrOrderNotFound", err)
	}
	if _, err := r.Orders().GetOwned(ctx, 999999, u.ID); !errors.Is(err, postgres.ErrOrderNotFound) {
		t.Errorf("missing GetOwned err = %v, want ErrOrderNotFound", err)
	}
}
