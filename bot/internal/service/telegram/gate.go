// Package telegram hosts webhook registration, gate/ban/rate-limit policies and menu views.
//
// @file      internal/service/telegram/gate.go
// @for       Required-group membership gate with 6-hour cache (FR-01).
// @uses      context, time, github.com/go-telegram/bot/models, internal/repository/redis (keys only)
// @reason    Encapsulates the join-gate decision so dispatchers stay thin.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"context"
	"time"

	"github.com/go-telegram/bot/models"
)

// GateResult is the outcome of a membership check.
type GateResult int

const (
	// GateAllowed means the user is a member (or the gate is disabled).
	GateAllowed GateResult = iota
	// GateDenied means the user is not (yet) a member.
	GateDenied
	// GateUnknown means the check failed; callers fail closed (PRD §1.6).
	GateUnknown
)

// MembershipAPI resolves a user's membership type in a chat.
type MembershipAPI interface {
	GetChatMember(ctx context.Context, chatID, userID int64) (models.ChatMemberType, error)
}

// KVStore is the cache seam (implemented by repository/redis).
type KVStore interface {
	GetString(ctx context.Context, key string) (string, bool, error)
	SetString(ctx context.Context, key, value string, ttl time.Duration) error
}

// GateService enforces the required-group membership gate (FR-01).
type GateService struct {
	api     MembershipAPI
	store   KVStore
	groupID int64
}

// NewGateService wires the gate; groupID 0 disables it.
func NewGateService(api MembershipAPI, store KVStore, groupID int64) *GateService {
	return &GateService{api: api, store: store, groupID: groupID}
}

// Enabled reports whether the group gate is active.
func (s *GateService) Enabled() bool { return s.groupID != 0 }

// Check evaluates membership, serving the 6-hour cache when present.
func (s *GateService) Check(ctx context.Context, userID int64) GateResult {
	if !s.Enabled() {
		return GateAllowed
	}
	if _, found, err := s.store.GetString(ctx, GateCacheKey(userID)); err == nil && found {
		return GateAllowed
	}
	return s.checkFresh(ctx, userID)
}

// CheckFresh re-evaluates membership against Telegram, bypassing the cache
// (used by the "✅ Sudah Join" button).
func (s *GateService) CheckFresh(ctx context.Context, userID int64) GateResult {
	if !s.Enabled() {
		return GateAllowed
	}
	return s.checkFresh(ctx, userID)
}

func (s *GateService) checkFresh(ctx context.Context, userID int64) GateResult {
	mtype, err := s.api.GetChatMember(ctx, s.groupID, userID)
	if err != nil {
		return GateUnknown
	}
	switch mtype {
	case models.ChatMemberTypeOwner,
		models.ChatMemberTypeAdministrator,
		models.ChatMemberTypeMember,
		models.ChatMemberTypeRestricted:
		// Cache is best-effort: a Redis hiccup must not deny a real member.
		_ = s.store.SetString(ctx, GateCacheKey(userID), "ok", GateCacheTTL)
		return GateAllowed
	default:
		return GateDenied
	}
}
