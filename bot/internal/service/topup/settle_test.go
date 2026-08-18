// Package topupsvc tests also cover the pg.charge settlement (Phase 4).
//
// @file      internal/service/topup/settle_test.go
// @for       ApplySettlement: succeeded credits once; failed/expired never
// credit; duplicate delivery is a no-op (idempotency, AGENTS.md §2.1).
// @uses      testing, context, internal/repository/postgres
// @reason    The webhook + poll fallback must never double-credit; these tests
// pin the conditional pending→terminal transition that makes it safe.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-18
package topupsvc

import (
	"context"
	"testing"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// seedPending inserts a pending payment row for the given net amount.
func seedPending(d *testDeps, orderID string, net int64) *postgres.Payment {
	p := &postgres.Payment{
		OrderID: orderID, UserID: 1, TelegramID: 7,
		AmountNet: domain.Money(net), AmountGross: domain.Money(net + 300),
		Status: postgres.PaymentStatusPending,
	}
	d.store.rows[orderID] = p
	return p
}

func TestApplySettlement_GivenSucceeded_ThenCreditsNetAndNotifies(t *testing.T) {
	d := newTestDeps()
	seedPending(d, "tp_1", 10000)
	s := d.build()

	res, err := s.ApplySettlement(context.Background(), "tp_1", "succeeded")
	if err != nil {
		t.Fatalf("ApplySettlement: %v", err)
	}
	if res.Status != postgres.PaymentStatusSuccess {
		t.Errorf("status = %s, want success", res.Status)
	}
	if res.AlreadyTerminal {
		t.Error("must not be already terminal")
	}
	if len(d.ledger.credits) != 1 || d.ledger.credits[0] != "tp_1" {
		t.Errorf("credits = %v, want [tp_1]", d.ledger.credits)
	}
	if d.ledger.balance != 10000 {
		t.Errorf("balance = %d, want 10000", d.ledger.balance)
	}
	if len(d.notices) != 1 || d.notices[0].OrderID != "tp_1" || d.notices[0].Amount != 10000 {
		t.Errorf("notices = %+v, want one tp_1 notice of 10000", d.notices)
	}
}

func TestApplySettlement_GivenDuplicateSucceeded_ThenNoDoubleCredit(t *testing.T) {
	d := newTestDeps()
	seedPending(d, "tp_1", 10000)
	s := d.build()

	if _, err := s.ApplySettlement(context.Background(), "tp_1", "succeeded"); err != nil {
		t.Fatalf("first settle: %v", err)
	}
	// The webhook retries / the poll fallback fires — must be a no-op.
	res, err := s.ApplySettlement(context.Background(), "tp_1", "succeeded")
	if err != nil {
		t.Fatalf("duplicate settle: %v", err)
	}
	if !res.AlreadyTerminal {
		t.Error("duplicate must report already terminal")
	}
	if len(d.ledger.credits) != 1 {
		t.Errorf("credits = %d, want exactly 1 (no double credit)", len(d.ledger.credits))
	}
	if d.ledger.balance != 10000 {
		t.Errorf("balance = %d, want 10000", d.ledger.balance)
	}
}

func TestApplySettlement_GivenFailed_ThenNoCredit(t *testing.T) {
	d := newTestDeps()
	seedPending(d, "tp_2", 25000)
	s := d.build()

	res, err := s.ApplySettlement(context.Background(), "tp_2", "failed")
	if err != nil {
		t.Fatalf("ApplySettlement: %v", err)
	}
	if res.Status != postgres.PaymentStatusFailed {
		t.Errorf("status = %s, want failed", res.Status)
	}
	if len(d.ledger.credits) != 0 {
		t.Errorf("credits = %v, want none", d.ledger.credits)
	}
}

func TestApplySettlement_GivenExpired_ThenNoCredit(t *testing.T) {
	d := newTestDeps()
	seedPending(d, "tp_3", 50000)
	s := d.build()

	res, err := s.ApplySettlement(context.Background(), "tp_3", "expired")
	if err != nil {
		t.Fatalf("ApplySettlement: %v", err)
	}
	if res.Status != postgres.PaymentStatusExpired {
		t.Errorf("status = %s, want expired", res.Status)
	}
	if len(d.ledger.credits) != 0 {
		t.Errorf("credits = %v, want none", d.ledger.credits)
	}
}

func TestApplySettlement_GivenUnknownStatus_ThenError(t *testing.T) {
	d := newTestDeps()
	seedPending(d, "tp_4", 10000)
	s := d.build()

	if _, err := s.ApplySettlement(context.Background(), "tp_4", "mystery"); err == nil {
		t.Fatal("expected error for unknown status")
	}
	if len(d.ledger.credits) != 0 {
		t.Errorf("credits = %v, want none", d.ledger.credits)
	}
}

func TestApplySettlement_GivenUnknownOrder_ThenError(t *testing.T) {
	d := newTestDeps()
	s := d.build()

	if _, err := s.ApplySettlement(context.Background(), "tp_nope", "succeeded"); err == nil {
		t.Fatal("expected error for unknown order")
	}
}
