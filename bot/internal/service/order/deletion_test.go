// Package ordersvc_test covers the account-deletion history record (FR-08 AC-4).
//
// @file      internal/service/order/deletion_test.go
// @for       Unit test: RecordDeletion persists a completed zero-amount order.
// @uses      testing, context, internal/domain
// @reason    The Riwayat entry for a deletion is product contract (FR-08
// AC-4) — locked here without DB (AGENTS.md §2.1). Split for the §1.1 limit.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-12
package ordersvc

import (
	"context"
	"testing"

	"github.com/kentangtech/bot-order/internal/domain"
)

func TestRecordDeletion_GivenAccount_ThenCompletedDeletionOrderCreated(t *testing.T) {
	store := newFakeStores()
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels)

	if err := svc.RecordDeletion(context.Background(), 9, 5, "vless", "del@vpn.kt"); err != nil {
		t.Fatalf("RecordDeletion: %v", err)
	}
	if len(store.orders.created) != 1 {
		t.Fatalf("orders created = %d, want 1", len(store.orders.created))
	}
	row := store.orders.created[0]
	if row.OrderType != string(domain.OrderTypeDeletion) {
		t.Errorf("order type = %s, want deletion", row.OrderType)
	}
	if row.Status != string(domain.OrderCompleted) {
		t.Errorf("status = %s, want completed", row.Status)
	}
	if !row.FinalAmount.IsZero() {
		t.Errorf("amount must be zero, got %s", row.FinalAmount.FormatIDR())
	}
	if row.AccountEmail != "del@vpn.kt" || row.Protocol != "vless" ||
		row.ServerID == nil || *row.ServerID != 5 {
		t.Errorf("row fields = %+v", row)
	}
	if row.CompletedAt == nil {
		t.Error("completed_at must be set")
	}
}
