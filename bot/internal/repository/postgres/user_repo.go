// Package postgres also hosts the user + ledger repository.
//
// @file      internal/repository/postgres/user_repo.go
// @for       Find-or-create users, atomic balance moves + immutable ledger (PRD §13.1/§13.5).
// @uses      context, errors, gorm.io/gorm, internal/domain
// @reason    Balance debit is atomic (guard in SQL) so double-charge is impossible (FR-04 AC-1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
	"gorm.io/gorm"
)

// ErrInsufficientBalance is returned when an atomic debit would go negative.
var ErrInsufficientBalance = errors.New("insufficient balance")

// UserRepo persists users and appends immutable ledger rows.
type UserRepo struct{ db *gorm.DB }

// NewUserRepo builds the repository on the shared GORM handle.
func NewUserRepo(db *gorm.DB) *UserRepo { return &UserRepo{db: db} }

// WithTx runs fn inside one DB transaction (atomic order fulfillment, FR-04 AC-1).
func (r *UserRepo) WithTx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}

// FindOrCreate returns the user row, creating it on first contact (FR-02 onboarding).
// A unique-violation race on telegram_id falls back to a read (idempotent).
func (r *UserRepo) FindOrCreate(ctx context.Context, tgID int64, username, firstName string) (*User, error) {
	var u User
	err := r.db.WithContext(ctx).Where("telegram_id = ?", tgID).First(&u).Error
	if err == nil {
		return &u, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	u = User{TelegramID: tgID, Username: username, FirstName: firstName, Language: "id"}
	if err := r.db.WithContext(ctx).Create(&u).Error; err != nil {
		// Race: another request created the row first — reread.
		var dup User
		if derr := r.db.WithContext(ctx).Where("telegram_id = ?", tgID).First(&dup).Error; derr == nil {
			return &dup, nil
		}
		return nil, fmt.Errorf("creating user: %w", err)
	}
	return &u, nil
}

// GetByTelegramID loads a user by their Telegram id.
func (r *UserRepo) GetByTelegramID(ctx context.Context, tgID int64) (*User, error) {
	var u User
	if err := r.db.WithContext(ctx).Where("telegram_id = ?", tgID).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// SetBanned flips the persistent ban flag (FR-11). It is the source of truth
// for the debit guard; the gate-level marker lives in Redis (service/telegram).
func (r *UserRepo) SetBanned(ctx context.Context, tgID int64, banned bool) error {
	res := r.db.WithContext(ctx).Model(&User{}).
		Where("telegram_id = ?", tgID).
		Updates(map[string]any{"is_banned": banned, "updated_at": time.Now()})
	if res.Error != nil {
		return fmt.Errorf("setting user %d banned=%v: %w", tgID, banned, res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ListTelegramIDs pages the registered user IDs for the admin broadcast (FR-11).
// Ordered by id so paging is stable across chunks (AGENTS.md §1.7: bounded).
func (r *UserRepo) ListTelegramIDs(ctx context.Context, limit, offset int) ([]int64, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var ids []int64
	err := r.db.WithContext(ctx).Model(&User{}).
		Order("id ASC").Limit(limit).Offset(offset).
		Pluck("telegram_id", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("listing user ids: %w", err)
	}
	return ids, nil
}

// CountUsers returns the number of registered users (broadcast sizing, FR-11).
func (r *UserRepo) CountUsers(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&User{}).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("counting users: %w", err)
	}
	return n, nil
}

// Debit atomically deducts amount and appends a ledger row in one self-contained
// transaction (order fulfillment, FR-04 AC-1).
func (r *UserRepo) Debit(ctx context.Context, userID int64, amount domain.Money, orderID string) (domain.Money, error) {
	var balance domain.Money
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		balance, err = r.DebitTx(ctx, tx, userID, amount, orderID)
		return err
	})
	return balance, err
}

// Credit atomically adds amount and appends a ledger row (topup, FR-06/M5).
func (r *UserRepo) Credit(ctx context.Context, userID int64, amount domain.Money, orderID string) (domain.Money, error) {
	var balance domain.Money
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		balance, err = r.CreditTx(ctx, tx, userID, amount, orderID)
		return err
	})
	return balance, err
}

// DebitTx atomically deducts amount and appends a ledger row inside tx.
// The SQL guard (balance >= amount AND NOT is_banned) makes double-charge
// impossible; ErrInsufficientBalance is returned when the guard fails.
// GORM Raw().Scan does not populate RowsAffected for UPDATE...RETURNING, so
// the guard runs through Exec (accurate RowsAffected) and the new balance is
// read in the same transaction — still atomic.
func (r *UserRepo) DebitTx(ctx context.Context, tx *gorm.DB, userID int64, amount domain.Money, orderID string) (domain.Money, error) {
	if amount <= 0 {
		return 0, fmt.Errorf("debit amount must be positive: %d", amount)
	}
	res := tx.WithContext(ctx).Exec(`
		UPDATE users SET balance = balance - ?, total_spent = total_spent + ?,
		                 updated_at = now()
		WHERE id = ? AND balance >= ? AND is_banned = false`,
		amount.Rupiah(), amount.Rupiah(), userID, amount.Rupiah())
	if res.Error != nil {
		return 0, fmt.Errorf("debiting balance: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return 0, ErrInsufficientBalance
	}
	newBalance, err := r.balanceInTx(ctx, tx, userID)
	if err != nil {
		return 0, err
	}
	if err := r.appendLedgerTx(ctx, tx, userID, orderID, "debit", amount, newBalance); err != nil {
		return 0, err
	}
	return newBalance, nil
}

// CreditTx atomically adds amount and appends a ledger row (topup, FR-06/M5).
func (r *UserRepo) CreditTx(ctx context.Context, tx *gorm.DB, userID int64, amount domain.Money, orderID string) (domain.Money, error) {
	if amount <= 0 {
		return 0, fmt.Errorf("credit amount must be positive: %d", amount)
	}
	res := tx.WithContext(ctx).Exec(`
		UPDATE users SET balance = balance + ?, updated_at = now()
		WHERE id = ?`, amount.Rupiah(), userID)
	if res.Error != nil {
		return 0, fmt.Errorf("crediting balance: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return 0, gorm.ErrRecordNotFound
	}
	newBalance, err := r.balanceInTx(ctx, tx, userID)
	if err != nil {
		return 0, err
	}
	if err := r.appendLedgerTx(ctx, tx, userID, orderID, "credit", amount, newBalance); err != nil {
		return 0, err
	}
	return newBalance, nil
}

// balanceInTx reads the user's balance inside the active transaction.
func (r *UserRepo) balanceInTx(ctx context.Context, tx *gorm.DB, userID int64) (domain.Money, error) {
	var newBalance domain.Money
	if err := tx.WithContext(ctx).Raw(`SELECT balance FROM users WHERE id = ?`, userID).Scan(&newBalance).Error; err != nil {
		return 0, fmt.Errorf("reading balance: %w", err)
	}
	return newBalance, nil
}

// appendLedgerTx writes one immutable balance_transactions row (PRD §13.5).
func (r *UserRepo) appendLedgerTx(ctx context.Context, tx *gorm.DB, userID int64, orderID, typ string, amount, balanceAfter domain.Money) error {
	row := BalanceTransaction{
		UserID:       userID,
		OrderID:      orderID,
		Type:         typ,
		Amount:       amount,
		BalanceAfter: balanceAfter,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("appending ledger: %w", err)
	}
	return nil
}
