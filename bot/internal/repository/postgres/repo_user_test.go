// Package postgres_test covers the user/ledger repository against real PostgreSQL.
//
// @file      internal/repository/postgres/repo_user_test.go
// @for       Integration tests: FindOrCreate, atomic Debit/Credit, ledger rows.
// @uses      testing, context, github.com/kentangtech/bot-order/internal/domain,
// github.com/kentangtech/bot-order/internal/repository/postgres
// @reason    Verifies the atomic balance guard (FR-04 AC-1) at the SQL level.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

func TestUserRepo_FindOrCreate_GivenNewAndExisting_ThenSingleRow(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)

	ctx := context.Background()
	first, err := r.User().FindOrCreate(ctx, 555001, "budi", "Budi")
	if err != nil {
		t.Fatalf("FindOrCreate: %v", err)
	}
	second, err := r.User().FindOrCreate(ctx, 555001, "budi", "Budi")
	if err != nil {
		t.Fatalf("FindOrCreate (2nd): %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("same telegram_id must map to one row: %d vs %d", first.ID, second.ID)
	}
	if first.Balance != 0 {
		t.Errorf("new user balance = %d, want 0", first.Balance)
	}
}

func TestUserRepo_Debit_GivenSufficientBalance_ThenBalancedAndLedger(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	ctx := context.Background()

	u, _ := r.User().FindOrCreate(ctx, 555002, "sari", "Sari")
	if _, err := r.User().Credit(ctx, u.ID, domain.Money(10000), "KTS-CREDIT-1"); err != nil {
		t.Fatalf("Credit: %v", err)
	}

	after, err := r.User().Debit(ctx, u.ID, domain.Money(7000), "KTS-ORDER-1")
	if err != nil {
		t.Fatalf("Debit: %v", err)
	}
	if after != 3000 {
		t.Errorf("balance after debit = %d, want 3000", after)
	}

	fresh, _ := r.User().GetByTelegramID(ctx, 555002)
	if fresh.Balance != 3000 {
		t.Errorf("persisted balance = %d, want 3000", fresh.Balance)
	}
	if fresh.TotalSpent != 7000 {
		t.Errorf("total_spent = %d, want 7000", fresh.TotalSpent)
	}
	if got := ledgerCount(t, u.ID); got != 2 { // 1 credit + 1 debit
		t.Errorf("ledger rows = %d, want 2", got)
	}
}

func TestUserRepo_Debit_GivenInsufficient_ThenErrorAndNoLedger(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	ctx := context.Background()

	u, _ := r.User().FindOrCreate(ctx, 555003, "andi", "Andi")
	if _, err := r.User().Credit(ctx, u.ID, domain.Money(1000), "KTS-CREDIT-2"); err != nil {
		t.Fatalf("Credit: %v", err)
	}

	if _, err := r.User().Debit(ctx, u.ID, domain.Money(5000), "KTS-ORDER-2"); !errors.Is(err, postgres.ErrInsufficientBalance) {
		t.Fatalf("Debit = %v, want ErrInsufficientBalance", err)
	}
	fresh, _ := r.User().GetByTelegramID(ctx, 555003)
	if fresh.Balance != 1000 {
		t.Errorf("balance must be untouched: %d", fresh.Balance)
	}
	if got := ledgerCount(t, u.ID); got != 1 {
		t.Errorf("ledger rows = %d, want 1 (failed debit must not append)", got)
	}
}

func TestUserRepo_Debit_GivenBannedUser_ThenRejected(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	ctx := context.Background()

	u, _ := r.User().FindOrCreate(ctx, 555004, "joko", "Joko")
	if _, err := r.User().Credit(ctx, u.ID, domain.Money(50000), "KTS-CREDIT-3"); err != nil {
		t.Fatalf("Credit: %v", err)
	}
	if err := r.DB().Exec("UPDATE users SET is_banned = true WHERE id = ?", u.ID).Error; err != nil {
		t.Fatalf("banning user: %v", err)
	}

	if _, err := r.User().Debit(ctx, u.ID, domain.Money(1000), "KTS-ORDER-3"); !errors.Is(err, postgres.ErrInsufficientBalance) {
		t.Fatalf("Debit = %v, want ErrInsufficientBalance for banned user", err)
	}
}
