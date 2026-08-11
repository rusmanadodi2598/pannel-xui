// Package telegram hosts webhook registration, gate/ban/rate-limit policies and menu views.
//
// @file      internal/service/telegram/ban.go
// @for       Banned-user check via Redis marker + admin ban/unban (FR-01/FR-11).
// @uses      context, time
// @reason    Separates the ban policy so dispatcher middleware stays declarative;
//
//	the admin flow reuses the same marker (FR-11 ban/unban).
//
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"context"
	"time"
)

// ExistsStore is the marker-key seam (implemented by repository/redis).
type ExistsStore interface {
	Exists(ctx context.Context, key string) (bool, error)
	SetString(ctx context.Context, key, value string, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

// BanService reports whether a user is banned and applies admin ban/unban
// (Redis marker bot:ban:{id}; the DB flag is owned by service/user, FR-11).
type BanService struct {
	store ExistsStore
}

// NewBanService wires the ban store.
func NewBanService(store ExistsStore) *BanService {
	return &BanService{store: store}
}

// IsBanned checks the ban marker for a user.
func (s *BanService) IsBanned(ctx context.Context, userID int64) (bool, error) {
	return s.store.Exists(ctx, BanKey(userID))
}

// Ban sets the gate-level ban marker (FR-11). The TTL acts as a crash guard;
// the persistent flag lives in the DB, so a Redis flush never unbans silently.
func (s *BanService) Ban(ctx context.Context, userID int64) error {
	return s.store.SetString(ctx, BanKey(userID), "1", BanTTL)
}

// Unban removes the gate-level ban marker (FR-11).
func (s *BanService) Unban(ctx context.Context, userID int64) error {
	return s.store.Delete(ctx, BanKey(userID))
}
