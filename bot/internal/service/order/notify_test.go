// Package ordersvc test covers the FR-04 AC admin-group notice (v1.41).
//
// @file      internal/service/order/notify_test.go
// @for       OrderNotifier seam: fired once on completed paid orders, never on
// failure, never for trial, nil-safe.
// @uses      testing, context, errors, time, internal/domain, internal/repository/postgres
// @reason    The notice is money-adjacent ops telemetry: guards that a completed
// paid order always reports to NOTIFICATION_GROUP_ID exactly once, and that no
// failure path (panel/debit/refund) ever leaks a notice.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-12
package ordersvc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// fakeNotifier records OrderNotice calls.
type fakeNotifier struct {
	notices []OrderNotice
	err     error
}

func (f *fakeNotifier) NotifyOrderCompleted(_ context.Context, n OrderNotice) error {
	if f.err != nil {
		return f.err
	}
	f.notices = append(f.notices, n)
	return nil
}

func (f *fakeNotifier) count() int { return len(f.notices) }

func TestPurchase_GivenCompleted_ThenAdminNotifiedOnce(t *testing.T) {
	plan := &domain.VpnPlan{CountryCode: "ID", CountryName: "Indonesia", Days: 30, Price: 7000}
	user := &postgres.User{ID: 1, FirstName: "Budi", Balance: 50000}
	store := newFakeStores()
	store.plans.plan = plan
	store.servers.serverID = 5
	store.panels.created = domain.PanelClient{InboundID: 9, Email: "ktsx@vpn.kt", UUID: "u1", Protocol: "vless"}
	notify := &fakeNotifier{}
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels, notify)

	res, err := svc.Purchase(context.Background(), user, "ID", 30, 0, 0, "vless")
	if err != nil {
		t.Fatalf("Purchase: %v", err)
	}
	if res.Status != domain.OrderCompleted {
		t.Fatalf("status = %s, want completed", res.Status)
	}
	if notify.count() != 1 {
		t.Fatalf("notified %d times, want exactly 1", notify.count())
	}
	n := notify.notices[0]
	if n.OrderType != domain.OrderTypePurchase {
		t.Errorf("order type = %s, want purchase", n.OrderType)
	}
	// The account email is derived from the order ID by the flow, so the notice
	// must carry the same email the result reports back.
	if n.OrderID != res.OrderID || n.AccountEmail != res.AccountEmail {
		t.Errorf("notice = %+v, want order %s + email %s", n, res.OrderID, res.AccountEmail)
	}
	if n.UserLabel != "Budi" {
		t.Errorf("user label = %q, want Budi", n.UserLabel)
	}
	if n.PlanLabel != "ID 30 Hari" || n.Amount != 7000 || n.BalanceAfter != 43000 {
		t.Errorf("notice payload = %+v", n)
	}
	if n.NewExpiry.IsZero() {
		t.Error("new expiry must be set on a completed purchase")
	}
}

func TestPurchase_GivenPanelFailure_ThenNotNotified(t *testing.T) {
	plan := &domain.VpnPlan{CountryCode: "ID", Days: 30, Price: 7000}
	store := newFakeStores()
	store.plans.plan = plan
	store.servers.serverID = 5
	store.panels.createErr = errors.New("panel unreachable")
	notify := &fakeNotifier{}
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels, notify)

	if _, err := svc.Purchase(context.Background(), &postgres.User{ID: 1, Balance: 50000}, "ID", 30, 0, 0, "vless"); !errors.Is(err, ErrFulfillFailed) {
		t.Fatalf("err = %v, want ErrFulfillFailed", err)
	}
	if notify.count() != 0 {
		t.Fatalf("notified %d times on failure, want 0", notify.count())
	}
}

func TestPurchase_GivenDebitFailure_ThenNotNotified(t *testing.T) {
	plan := &domain.VpnPlan{CountryCode: "ID", Days: 30, Price: 7000}
	store := newFakeStores()
	store.plans.plan = plan
	store.servers.serverID = 5
	store.panels.created = domain.PanelClient{InboundID: 9, Email: "k@vpn.kt", UUID: "u", Protocol: "vless"}
	store.users.debitErr = errors.New("db down")
	notify := &fakeNotifier{}
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels, notify)

	if _, err := svc.Purchase(context.Background(), &postgres.User{ID: 1, Balance: 50000}, "ID", 30, 0, 0, "vless"); !errors.Is(err, ErrFulfillFailed) {
		t.Fatalf("err = %v, want ErrFulfillFailed", err)
	}
	if notify.count() != 0 {
		t.Fatalf("notified %d times on debit failure, want 0", notify.count())
	}
}

func TestRenew_GivenCompleted_ThenAdminNotifiedOnce(t *testing.T) {
	plan := &domain.VpnPlan{CountryCode: "ID", CountryName: "Indonesia", Days: 30, Price: 7000}
	user := &postgres.User{ID: 1, FirstName: "Sari", Balance: 50000}
	store := newFakeStores()
	store.plans.plan = plan
	future := time.Now().Add(10 * 24 * time.Hour)
	store.clients.owned = &postgres.VPNClient{
		ID: 3, UserID: 1, ServerID: 5, Email: "ktsx@vpn.kt",
		Protocol: "vless", UUID: "u1", IsTrial: false, ExpiresAt: &future,
	}
	store.users.balanceAfter = 43000
	notify := &fakeNotifier{}
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels, notify)

	res, err := svc.Renew(context.Background(), user, 3, "ID", 30)
	if err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if res.Status != domain.OrderCompleted {
		t.Fatalf("status = %s, want completed", res.Status)
	}
	if notify.count() != 1 {
		t.Fatalf("notified %d times, want exactly 1", notify.count())
	}
	n := notify.notices[0]
	if n.OrderType != domain.OrderTypeRenewal {
		t.Errorf("order type = %s, want renewal", n.OrderType)
	}
	if n.UserLabel != "Sari" || n.PlanLabel != "ID 30 Hari" {
		t.Errorf("notice = %+v", n)
	}
}

func TestRenew_GivenPanelFailure_ThenNotNotified(t *testing.T) {
	plan := &domain.VpnPlan{CountryCode: "ID", Days: 30, Price: 7000}
	store := newFakeStores()
	store.plans.plan = plan
	store.clients.owned = &postgres.VPNClient{
		ID: 3, UserID: 1, ServerID: 5, Email: "k@vpn.kt", Protocol: "vless", UUID: "u1", IsTrial: false,
	}
	store.users.balanceAfter = 43000
	store.panels.renewErr = errors.New("panel timeout")
	notify := &fakeNotifier{}
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels, notify)

	if _, err := svc.Renew(context.Background(), &postgres.User{ID: 1, Balance: 50000}, 3, "ID", 30); !errors.Is(err, ErrFulfillFailed) {
		t.Fatalf("err = %v, want ErrFulfillFailed", err)
	}
	if notify.count() != 0 {
		t.Fatalf("notified %d times on panel failure, want 0", notify.count())
	}
}

func TestCreateTrial_GivenCompleted_ThenNotNotified(t *testing.T) {
	store := newTrialStore()
	store.panels.created = domain.PanelClient{InboundID: 9, Email: "trial@vpn.kt", UUID: "u1", Protocol: "vless"}
	notify := &fakeNotifier{}
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels, notify)

	res, err := svc.CreateTrial(context.Background(), &postgres.User{ID: 9, Balance: 50000}, 3,
		TrialSpec{Hours: 1, TrafficGB: 1, IPLimit: 1})
	if err != nil {
		t.Fatalf("CreateTrial: %v", err)
	}
	if res.Status != domain.OrderCompleted {
		t.Fatalf("status = %s, want completed", res.Status)
	}
	if notify.count() != 0 {
		t.Fatalf("trial notified %d times, want 0 (free account, admin notice is paid-only)", notify.count())
	}
}

func TestPurchase_GivenNilNotifier_ThenNoPanic(t *testing.T) {
	plan := &domain.VpnPlan{CountryCode: "ID", Days: 30, Price: 7000}
	store := newFakeStores()
	store.plans.plan = plan
	store.servers.serverID = 5
	store.panels.created = domain.PanelClient{InboundID: 9, Email: "k@vpn.kt", UUID: "u", Protocol: "vless"}
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels)

	if _, err := svc.Purchase(context.Background(), &postgres.User{ID: 1, Balance: 50000}, "ID", 30, 0, 0, "vless"); err != nil {
		t.Fatalf("Purchase: %v", err)
	}
}

func TestPurchase_GivenNotifierError_ThenOrderStillCompleted(t *testing.T) {
	plan := &domain.VpnPlan{CountryCode: "ID", Days: 30, Price: 7000}
	store := newFakeStores()
	store.plans.plan = plan
	store.servers.serverID = 5
	store.panels.created = domain.PanelClient{InboundID: 9, Email: "k@vpn.kt", UUID: "u", Protocol: "vless"}
	notify := &fakeNotifier{err: errors.New("telegram down")}
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels, notify)

	res, err := svc.Purchase(context.Background(), &postgres.User{ID: 1, Balance: 50000}, "ID", 30, 0, 0, "vless")
	if err != nil {
		t.Fatalf("Purchase: %v", err)
	}
	if res.Status != domain.OrderCompleted {
		t.Fatalf("status = %s — a failed notice must never fail the order", res.Status)
	}
}

// Compile-time check: fakes satisfy the seam.
var _ OrderNotifier = (*fakeNotifier)(nil)
