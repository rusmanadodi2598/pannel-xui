// Package adminsvc test covers the adjust-saldo operation (FR-11, v1.39).
//
// @file      internal/service/admin/adjust_test.go
// @for       AdjustBalance: resolves tgID → PK, atomic credit/debit + ADJ- ledger ref.
// @uses      testing, context, errors, internal/domain, internal/repository/postgres, gorm.io/gorm
// @reason    Admin corrections must use the same atomic + ledgered path as orders
// (AGENTS.md §1.3/§2.2) and never move money for an unknown user.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-12
package adminsvc

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	"gorm.io/gorm"
)

func TestAdjustBalance_GivenCredit_ThenCreditsUserPKWithADJRef(t *testing.T) {
	users := &fakeUsers{user: &postgres.User{ID: 9, TelegramID: 42, Balance: 100000}}
	audit := &fakeAudit{}
	s := New(&fakePlans{}, users, &fakeBanner{}, &fakeSender{}, &fakeLocker{}, &fakeServerOps{}, &fakeStats{}, audit, testLogger())

	bal, err := s.AdjustBalance(context.Background(), 7, 42, domain.Money(25000), true)
	if err != nil {
		t.Fatalf("AdjustBalance: %v", err)
	}
	if bal != 125000 {
		t.Errorf("balance = %d, want 125000", bal)
	}
	if len(users.credited) != 1 {
		t.Fatalf("credits = %d, want 1", len(users.credited))
	}
	move := users.credited[0]
	if move.userID != 9 || move.amount != 25000 {
		t.Errorf("credit = %+v, want userID 9 amount 25000", move)
	}
	if !strings.HasPrefix(move.orderID, "ADJ-") {
		t.Errorf("ledger ref = %q, want ADJ- prefix", move.orderID)
	}
	if len(users.debited) != 0 {
		t.Error("credit must not debit")
	}
	if len(audit.rows) != 1 || audit.rows[0].Action != AuditBalanceAdjust {
		t.Errorf("audit = %+v, want balance:adjust row", audit.rows)
	}
}

func TestAdjustBalance_GivenDebit_ThenDebitsUserPK(t *testing.T) {
	users := &fakeUsers{user: &postgres.User{ID: 9, TelegramID: 42, Balance: 100000}}
	s := New(&fakePlans{}, users, &fakeBanner{}, &fakeSender{}, &fakeLocker{}, &fakeServerOps{}, &fakeStats{}, &fakeAudit{}, testLogger())

	bal, err := s.AdjustBalance(context.Background(), 7, 42, domain.Money(30000), false)
	if err != nil {
		t.Fatalf("AdjustBalance: %v", err)
	}
	if bal != 70000 {
		t.Errorf("balance = %d, want 70000", bal)
	}
	if len(users.debited) != 1 || users.debited[0].userID != 9 || users.debited[0].amount != 30000 {
		t.Errorf("debit = %+v, want userID 9 amount 30000", users.debited)
	}
	if len(users.credited) != 0 {
		t.Error("debit must not credit")
	}
}

func TestAdjustBalance_GivenUnknownUser_ThenErrUserNotFound(t *testing.T) {
	users := &fakeUsers{err: gorm.ErrRecordNotFound}
	s := New(&fakePlans{}, users, &fakeBanner{}, &fakeSender{}, &fakeLocker{}, &fakeServerOps{}, &fakeStats{}, &fakeAudit{}, testLogger())

	_, err := s.AdjustBalance(context.Background(), 7, 999, domain.Money(1000), true)
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("err = %v, want ErrUserNotFound", err)
	}
	if len(users.credited) != 0 || len(users.debited) != 0 {
		t.Error("money must not move for an unknown user")
	}
}

func TestAdjustBalance_GivenInsufficientDebit_ThenPostgresErr(t *testing.T) {
	users := &fakeUsers{
		user:     &postgres.User{ID: 9, TelegramID: 42, Balance: 10000},
		debitErr: postgres.ErrInsufficientBalance,
	}
	s := New(&fakePlans{}, users, &fakeBanner{}, &fakeSender{}, &fakeLocker{}, &fakeServerOps{}, &fakeStats{}, &fakeAudit{}, testLogger())

	_, err := s.AdjustBalance(context.Background(), 7, 42, domain.Money(50000), false)
	if !errors.Is(err, postgres.ErrInsufficientBalance) {
		t.Fatalf("err = %v, want ErrInsufficientBalance", err)
	}
}

func TestAdjustBalance_GivenZeroAmount_ThenErrorWithoutMove(t *testing.T) {
	users := &fakeUsers{user: &postgres.User{ID: 9, TelegramID: 42, Balance: 10000}}
	s := New(&fakePlans{}, users, &fakeBanner{}, &fakeSender{}, &fakeLocker{}, &fakeServerOps{}, &fakeStats{}, &fakeAudit{}, testLogger())

	if _, err := s.AdjustBalance(context.Background(), 7, 42, domain.Money(0), true); err == nil {
		t.Fatal("expected error for zero amount")
	}
	if len(users.credited) != 0 {
		t.Error("zero amount must not move money")
	}
}
