// Package postgres_test covers the v1.37 idempotence guard (AGENTS.md §2.1).
//
// @file      internal/repository/postgres/repo_inflight_test.go
// @for       OrderRepo.FindInFlight: pending/processing matched, terminal/type/owner scoped.
// @uses      testing, context, time, github.com/kentangtech/bot-order/internal/repository/postgres
// @reason    The in-flight query is the idempotence source of truth for order &
// renewal — a duplicate execution must never slip through (v1.37).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-12
package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

func TestFindInFlight_GivenPendingAndProcessing_ThenNewestReturned(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	ctx := context.Background()
	u, _ := r.User().FindOrCreate(ctx, 555010, "inflight", "Inflight")
	now := time.Now()

	// No in-flight order yet → nil, nil (not an error).
	got, err := r.Orders().FindInFlight(ctx, u.ID, "renewal")
	if err != nil || got != nil {
		t.Fatalf("FindInFlight(empty) = %v, err = %v; want nil, nil", got, err)
	}

	// A completed order is NOT in flight.
	if err := r.Orders().Create(ctx, &postgres.Order{OrderID: "KTS-IF01-VPN", OrderType: "renewal", UserID: u.ID, Status: "completed", Currency: "IDR", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create completed: %v", err)
	}
	got, _ = r.Orders().FindInFlight(ctx, u.ID, "renewal")
	if got != nil {
		t.Fatalf("FindInFlight(completed) = %v, want nil", got)
	}

	// pending + processing are both in flight; the newest (processing) wins.
	for _, o := range []*postgres.Order{
		{OrderID: "KTS-IF02-VPN", OrderType: "renewal", UserID: u.ID, Status: "pending", Currency: "IDR", CreatedAt: now, UpdatedAt: now},
		{OrderID: "KTS-IF03-VPN", OrderType: "renewal", UserID: u.ID, Status: "processing", Currency: "IDR", CreatedAt: now, UpdatedAt: now},
	} {
		if err := r.Orders().Create(ctx, o); err != nil {
			t.Fatalf("create in-flight: %v", err)
		}
	}
	got, err = r.Orders().FindInFlight(ctx, u.ID, "renewal")
	if err != nil || got == nil || got.OrderID != "KTS-IF03-VPN" {
		t.Fatalf("FindInFlight = %+v, err = %v; want the newest processing order", got, err)
	}

	// A different order type is ignored.
	if err := r.Orders().Create(ctx, &postgres.Order{OrderID: "KTS-IF04-VPN", OrderType: "purchase", UserID: u.ID, Status: "processing", Currency: "IDR", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create purchase: %v", err)
	}
	got, _ = r.Orders().FindInFlight(ctx, u.ID, "renewal")
	if got == nil || got.OrderID != "KTS-IF03-VPN" {
		t.Fatalf("FindInFlight must ignore other order types: %+v", got)
	}

	// A foreign user's in-flight order is not matched.
	other, _ := r.User().FindOrCreate(ctx, 555011, "other", "Other")
	got, _ = r.Orders().FindInFlight(ctx, other.ID, "renewal")
	if got != nil {
		t.Fatalf("FindInFlight(other user) = %v, want nil", got)
	}
}
