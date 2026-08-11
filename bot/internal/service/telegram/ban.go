// Package telegram hosts webhook registration, gate/ban/rate-limit policies and menu views.
//
// @file      internal/service/telegram/ban.go
// @for       Banned-user check via Redis marker (FR-01; admin ban lands in FR-11).
// @uses      context
// @reason    Separates the ban policy so dispatcher middleware stays declarative.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import "context"

// ExistsStore is the marker-key seam (implemented by repository/redis).
type ExistsStore interface {
	Exists(ctx context.Context, key string) (bool, error)
}

// BanService reports whether a user is banned (Redis marker bot:ban:{id}).
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
