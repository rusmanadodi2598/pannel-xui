// Package adminsvc test covers the FR-11 admin operations.
//
// @file      internal/service/admin/admin_test.go
// @for       Given plans/users, then price ops delegate; ban touches both layers.
// @uses      testing, context, internal/domain
// @reason    Guards the FR-11 side effects without DB/network (AGENTS.md §2.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-11
package adminsvc

import (
	"context"
	"testing"

	"github.com/kentangtech/bot-order/internal/domain"
)

func TestService_PriceOps_GivenPlan_ThenDelegates(t *testing.T) {
	plans := &fakePlans{plans: []domain.VpnPlan{{CountryCode: "ID", Days: 30, Price: 7000, Enabled: true}}}
	audit := &fakeAudit{}
	s := New(plans, &fakeUsers{}, &fakeBanner{}, &fakeSender{}, &fakeLocker{}, &fakeServerOps{}, &fakeStats{}, audit, testLogger())
	ctx := context.Background()

	if got, err := s.ListPlans(ctx); err != nil || len(got) != 1 {
		t.Fatalf("ListPlans = %v, err %v", got, err)
	}
	if err := s.SetPrice(ctx, 7, "ID", 30, 7500); err != nil || !plans.setPrice {
		t.Errorf("SetPrice not delegated: %v", err)
	}
	if err := s.SetEnabled(ctx, 7, "ID", 30, false); err != nil || plans.setEnable == nil {
		t.Errorf("SetEnabled not delegated: %v", err)
	}
	if len(audit.rows) != 2 {
		t.Errorf("audit rows = %d, want 2 (price + toggle)", len(audit.rows))
	}
}

func TestService_BanUser_GivenID_ThenMarkerAndFlagSet(t *testing.T) {
	users := &fakeUsers{}
	banner := &fakeBanner{}
	s := New(&fakePlans{}, users, banner, &fakeSender{}, &fakeLocker{}, &fakeServerOps{}, &fakeStats{}, &fakeAudit{}, testLogger())

	if err := s.BanUser(context.Background(), 7, 42); err != nil {
		t.Fatalf("BanUser: %v", err)
	}
	if len(banner.banned) != 1 || banner.banned[0] != 42 {
		t.Errorf("marker ban = %v, want [42]", banner.banned)
	}
	if len(users.banned) != 1 || users.banned[0] != 42 {
		t.Errorf("flag ban = %v, want [42]", users.banned)
	}

	if err := s.UnbanUser(context.Background(), 7, 42); err != nil {
		t.Fatalf("UnbanUser: %v", err)
	}
	if len(banner.unbanned) != 1 || len(users.unbanned) != 1 {
		t.Errorf("unban = marker %v, flag %v", banner.unbanned, users.unbanned)
	}
}
