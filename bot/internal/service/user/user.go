// Package usersvc provides user & balance operations for order flows.
//
// @file      internal/service/user/user.go
// @for       Ensure users exist, read balance, atomic debit/credit + ledger (PRD §13.1/§13.5).
// @uses      context, internal/domain, internal/repository/postgres
// @reason    Order flows debit/credit atomically; ledger is immutable (FR-04 AC-1, M4).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package usersvc

import (
	"context"
	"fmt"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// Store is the user persistence seam (postgres.UserRepo implements it).
type Store interface {
	FindOrCreate(ctx context.Context, tgID int64, username, firstName string) (*postgres.User, error)
	GetByTelegramID(ctx context.Context, tgID int64) (*postgres.User, error)
	Debit(ctx context.Context, userID int64, amount domain.Money, orderID string) (domain.Money, error)
	Credit(ctx context.Context, userID int64, amount domain.Money, orderID string) (domain.Money, error)
	// FR-11 admin operations.
	SetBanned(ctx context.Context, tgID int64, banned bool) error
	ListTelegramIDs(ctx context.Context, limit, offset int) ([]int64, error)
	CountUsers(ctx context.Context) (int64, error)
}

// Service orchestrates user identity and balance moves.
type Service struct{ store Store }

// New builds the user service.
func New(store Store) *Service { return &Service{store: store} }

// EnsureUser returns the user row, creating it on first contact (FR-02).
func (s *Service) EnsureUser(ctx context.Context, tgID int64, username, firstName string) (*postgres.User, error) {
	return s.store.FindOrCreate(ctx, tgID, username, firstName)
}

// Get loads a user by Telegram id; ErrRecordNotFound when absent.
func (s *Service) Get(ctx context.Context, tgID int64) (*postgres.User, error) {
	return s.store.GetByTelegramID(ctx, tgID)
}

// Balance returns the user's current balance (whole rupiah).
func (s *Service) Balance(ctx context.Context, tgID int64) (domain.Money, error) {
	u, err := s.store.GetByTelegramID(ctx, tgID)
	if err != nil {
		return 0, fmt.Errorf("getting user balance: %w", err)
	}
	return u.Balance, nil
}

// Debit atomically deducts amount, appends a ledger row and returns the new
// balance. Insufficient balance surfaces as postgres.ErrInsufficientBalance.
func (s *Service) Debit(ctx context.Context, userID int64, amount domain.Money, orderID string) (domain.Money, error) {
	return s.store.Debit(ctx, userID, amount, orderID)
}

// Credit atomically adds amount (topup) and appends a ledger row.
func (s *Service) Credit(ctx context.Context, userID int64, amount domain.Money, orderID string) (domain.Money, error) {
	return s.store.Credit(ctx, userID, amount, orderID)
}

// SetBanned flips the persistent ban flag (FR-11; gate marker is separate).
func (s *Service) SetBanned(ctx context.Context, tgID int64, banned bool) error {
	return s.store.SetBanned(ctx, tgID, banned)
}

// ListTelegramIDs pages registered user ids for the admin broadcast (FR-11).
func (s *Service) ListTelegramIDs(ctx context.Context, limit, offset int) ([]int64, error) {
	return s.store.ListTelegramIDs(ctx, limit, offset)
}

// CountUsers returns the number of registered users (broadcast sizing, FR-11).
func (s *Service) CountUsers(ctx context.Context) (int64, error) {
	return s.store.CountUsers(ctx)
}
