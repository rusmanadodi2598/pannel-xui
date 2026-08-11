// Package telegram test covers the precise role lookup (AUTHOR vs admin).
//
// @file      internal/service/telegram/role_test.go
// @for       GateService.Role mapping: owner/admin/member/restricted/left/banned/unknown + IsStaff.
// @uses      testing, context, github.com/go-telegram/bot/models
// @reason    Guards the authorization distinction between AUTHOR (creator) and regular admins.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"context"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestRole_GivenOwner_ThenRoleOwner(t *testing.T) {
	g := NewGateService(&fakeMembership{mtype: models.ChatMemberTypeOwner}, newFakeKV(), -100123)
	if got := g.Role(context.Background(), 1); got != RoleOwner {
		t.Fatalf("Role = %v (%s), want owner", got, got)
	}
}

func TestRole_GivenAdministrator_ThenRoleAdmin(t *testing.T) {
	g := NewGateService(&fakeMembership{mtype: models.ChatMemberTypeAdministrator}, newFakeKV(), -100123)
	if got := g.Role(context.Background(), 1); got != RoleAdmin {
		t.Fatalf("Role = %v, want admin", got)
	}
}

func TestRole_GivenMember_ThenRoleMember(t *testing.T) {
	g := NewGateService(&fakeMembership{mtype: models.ChatMemberTypeMember}, newFakeKV(), -100123)
	if got := g.Role(context.Background(), 1); got != RoleMember {
		t.Fatalf("Role = %v, want member", got)
	}
}

func TestRole_GivenRestricted_ThenRoleRestricted(t *testing.T) {
	g := NewGateService(&fakeMembership{mtype: models.ChatMemberTypeRestricted}, newFakeKV(), -100123)
	if got := g.Role(context.Background(), 1); got != RoleRestricted {
		t.Fatalf("Role = %v, want restricted", got)
	}
}

func TestRole_GivenLeft_ThenRoleLeft(t *testing.T) {
	g := NewGateService(&fakeMembership{mtype: models.ChatMemberTypeLeft}, newFakeKV(), -100123)
	if got := g.Role(context.Background(), 1); got != RoleLeft {
		t.Fatalf("Role = %v, want left", got)
	}
}

func TestRole_GivenBanned_ThenRoleBanned(t *testing.T) {
	g := NewGateService(&fakeMembership{mtype: models.ChatMemberTypeBanned}, newFakeKV(), -100123)
	if got := g.Role(context.Background(), 1); got != RoleBanned {
		t.Fatalf("Role = %v, want banned", got)
	}
}

func TestRole_GivenAPIError_ThenRoleUnknown(t *testing.T) {
	g := NewGateService(&fakeMembership{err: errBoom}, newFakeKV(), -100123)
	if got := g.Role(context.Background(), 1); got != RoleUnknown {
		t.Fatalf("Role = %v, want unknown", got)
	}
}

func TestRole_GivenDisabledGate_ThenRoleUnknown(t *testing.T) {
	g := NewGateService(&fakeMembership{mtype: models.ChatMemberTypeOwner}, newFakeKV(), 0)
	if got := g.Role(context.Background(), 1); got != RoleUnknown {
		t.Fatalf("Role = %v, want unknown for disabled gate", got)
	}
}

func TestRoleIsStaff_GivenOwnerOrAdmin_ThenTrue(t *testing.T) {
	if !RoleOwner.IsStaff() || !RoleAdmin.IsStaff() {
		t.Error("owner/admin must be staff")
	}
	for _, r := range []MemberRole{RoleMember, RoleRestricted, RoleLeft, RoleBanned, RoleUnknown} {
		if r.IsStaff() {
			t.Errorf("role %s must not be staff", r)
		}
	}
}

func TestRole_GivenOwner_ThenGateCheckAllowsWithoutAPICache(t *testing.T) {
	// AUTHOR adalah member level tertinggi — gate harus allow, dan cache terisi.
	api := &fakeMembership{mtype: models.ChatMemberTypeOwner}
	store := newFakeKV()
	g := NewGateService(api, store, -100123)

	if res := g.Check(context.Background(), 1); res != GateAllowed {
		t.Fatalf("gate result = %v, want allowed for owner", res)
	}
	if _, ok := store.vals[GateCacheKey(1)]; !ok {
		t.Fatal("owner check must be cached like any member")
	}
}
