// Package telegram hosts webhook registration, gate/ban/rate-limit policies and menu views.
//
// @file      internal/service/telegram/role.go
// @for       Precise membership role lookup: AUTHOR (owner) vs admin vs member (authorization).
// @uses      context, github.com/go-telegram/bot/models
// @reason    Telegram distinguishes "creator" (pemilik grup) from "administrator"; admin
// commands (FR-11/M6) must grant AUTHOR the same rights as ADMIN.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"context"

	"github.com/go-telegram/bot/models"
)

// MemberRole is the user's level inside the required group.
type MemberRole int

const (
	// RoleUnknown means the gate is disabled or the lookup failed.
	RoleUnknown MemberRole = iota
	// RoleOwner is the group AUTHOR (Telegram status "creator").
	RoleOwner
	// RoleAdmin is a regular group administrator.
	RoleAdmin
	// RoleMember is a regular member.
	RoleMember
	// RoleRestricted is a restricted member (can still use the bot).
	RoleRestricted
	// RoleLeft means the user left / never joined.
	RoleLeft
	// RoleBanned means the user was kicked from the group.
	RoleBanned
)

// String renders the role for structured logs.
func (r MemberRole) String() string {
	switch r {
	case RoleOwner:
		return "owner"
	case RoleAdmin:
		return "admin"
	case RoleMember:
		return "member"
	case RoleRestricted:
		return "restricted"
	case RoleLeft:
		return "left"
	case RoleBanned:
		return "banned"
	default:
		return "unknown"
	}
}

// IsStaff reports whether the role carries staff privileges (AUTHOR or admin).
func (r MemberRole) IsStaff() bool { return r == RoleOwner || r == RoleAdmin }

// Role resolves the user's precise membership level in the required group.
// It performs a fresh lookup (no cache) because authorization decisions must
// reflect the current state, and reuses the same Telegram API as the gate.
func (s *GateService) Role(ctx context.Context, userID int64) MemberRole {
	if !s.Enabled() {
		return RoleUnknown
	}
	mtype, err := s.api.GetChatMember(ctx, s.groupID, userID)
	if err != nil {
		return RoleUnknown
	}
	return roleFromType(mtype)
}

// roleFromType maps the Telegram ChatMemberType to our MemberRole.
func roleFromType(m models.ChatMemberType) MemberRole {
	switch m {
	case models.ChatMemberTypeOwner:
		return RoleOwner
	case models.ChatMemberTypeAdministrator:
		return RoleAdmin
	case models.ChatMemberTypeMember:
		return RoleMember
	case models.ChatMemberTypeRestricted:
		return RoleRestricted
	case models.ChatMemberTypeLeft:
		return RoleLeft
	case models.ChatMemberTypeBanned:
		return RoleBanned
	default:
		return RoleUnknown
	}
}
