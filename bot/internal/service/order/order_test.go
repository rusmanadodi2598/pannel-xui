// Package ordersvc_test covers the auto-order flows with fakes (AGENTS.md §2.1).
//
// @file      internal/service/order/order_test.go
// @for       Unit tests: purchase happy/fail paths, renewal, atomic debit ordering.
// @uses      testing, context, errors, time, internal/domain, internal/repository/postgres
// @reason    Locks the FR-04 invariants: debit only after panel success, no double order.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package ordersvc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

func TestPurchase_GivenSufficientBalance_ThenCompletedAndDebited(t *testing.T) {
	plan := &domain.VpnPlan{CountryCode: "ID", CountryName: "Indonesia", Days: 30, Price: 7000}
	user := &postgres.User{ID: 1, Balance: 50000}
	store := newFakeStores()
	store.plans.plan = plan
	store.servers.serverID = 5
	store.panels.created = domain.PanelClient{InboundID: 9, Email: "ktsx@vpn.kt", UUID: "u1", Protocol: "vless"}
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels)

	res, err := svc.Purchase(context.Background(), user, "ID", 30, 0, 0, "vless")
	if err != nil {
		t.Fatalf("Purchase: %v", err)
	}
	if res.Status != domain.OrderCompleted {
		t.Fatalf("status = %s, want completed", res.Status)
	}
	if res.BalanceAfter != 43000 {
		t.Errorf("balance after = %d, want 43000", res.BalanceAfter)
	}
	if store.debited == 0 {
		t.Error("balance must be debited on success")
	}
	if len(store.clients.created) != 1 {
		t.Errorf("clients created = %d, want 1", len(store.clients.created))
	}
	if !store.panels.called || !store.debitAfterPanel {
		t.Error("panel must be called BEFORE debit (FR-04 AC-1)")
	}
	if last := store.saved[len(store.saved)-1]; last.Status != string(domain.OrderCompleted) {
		t.Errorf("final order save status = %s, want completed", last.Status)
	}
}

func TestPurchase_GivenPanelError_ThenFailedWithoutDebit(t *testing.T) {
	plan := &domain.VpnPlan{CountryCode: "ID", CountryName: "Indonesia", Days: 30, Price: 7000}
	user := &postgres.User{ID: 1, Balance: 50000}
	store := newFakeStores()
	store.plans.plan = plan
	store.servers.serverID = 5
	store.panels.createErr = errors.New("panel unreachable")
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels)

	_, err := svc.Purchase(context.Background(), user, "ID", 30, 0, 0, "vless")
	if !errors.Is(err, ErrFulfillFailed) {
		t.Fatalf("err = %v, want ErrFulfillFailed", err)
	}
	if store.debited != 0 {
		t.Error("balance must NOT be debited when panel fails")
	}
	if len(store.clients.created) != 0 {
		t.Error("client must not be recorded when panel fails")
	}
	if last := store.saved[len(store.saved)-1]; last.Status != string(domain.OrderFailed) {
		t.Errorf("final order save status = %s, want failed", last.Status)
	}
}

func TestPurchase_GivenInsufficientBalance_ThenRejectedBeforeOrder(t *testing.T) {
	plan := &domain.VpnPlan{CountryCode: "ID", CountryName: "Indonesia", Days: 30, Price: 7000}
	user := &postgres.User{ID: 1, Balance: 1000}
	store := newFakeStores()
	store.plans.plan = plan
	store.servers.serverID = 5
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels)

	_, err := svc.Purchase(context.Background(), user, "ID", 30, 0, 0, "vless")
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("err = %v, want ErrInsufficientBalance", err)
	}
	if store.debited != 0 || len(store.orders.created) != 0 {
		t.Error("no order or debit allowed with insufficient balance")
	}
}

func TestPurchase_GivenPlanMissing_ThenErrPlanNotFound(t *testing.T) {
	user := &postgres.User{ID: 1, Balance: 50000}
	store := newFakeStores()
	store.plans.err = ErrPlanNotFound
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels)

	if _, err := svc.Purchase(context.Background(), user, "ID", 30, 0, 0, "vless"); !errors.Is(err, ErrPlanNotFound) {
		t.Fatalf("err = %v, want ErrPlanNotFound", err)
	}
}

func TestPurchase_GivenDebitError_ThenFailedWithClientRecordedButNoDebit(t *testing.T) {
	plan := &domain.VpnPlan{CountryCode: "ID", CountryName: "Indonesia", Days: 30, Price: 7000}
	user := &postgres.User{ID: 1, Balance: 50000}
	store := newFakeStores()
	store.plans.plan = plan
	store.servers.serverID = 5
	store.panels.created = domain.PanelClient{InboundID: 9, Email: "ktsx@vpn.kt", UUID: "u1", Protocol: "vless"}
	store.users.debitErr = errors.New("db down")
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels)

	_, err := svc.Purchase(context.Background(), user, "ID", 30, 0, 0, "vless")
	if !errors.Is(err, ErrFulfillFailed) {
		t.Fatalf("err = %v, want ErrFulfillFailed", err)
	}
	if store.debited != 0 {
		t.Error("money must NOT be taken when debit fails")
	}
	if len(store.clients.created) != 1 {
		t.Error("client record must exist before debit (recoverable, no money lost)")
	}
	if last := store.saved[len(store.saved)-1]; last.Status != string(domain.OrderFailed) {
		t.Errorf("final order save status = %s, want failed", last.Status)
	}
}

func TestPurchase_GivenNoServer_ThenErrNoServer(t *testing.T) {
	plan := &domain.VpnPlan{CountryCode: "SG", CountryName: "Singapore", Days: 15, Price: 5000}
	user := &postgres.User{ID: 1, Balance: 50000}
	store := newFakeStores()
	store.plans.plan = plan
	store.servers.err = ErrNoServer
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels)

	if _, err := svc.Purchase(context.Background(), user, "SG", 15, 0, 0, "vless"); !errors.Is(err, ErrNoServer) {
		t.Fatalf("err = %v, want ErrNoServer", err)
	}
}

func TestRenew_GivenOwnedClient_ThenExpiryExtendedFromRemaining(t *testing.T) {
	plan := &domain.VpnPlan{CountryCode: "ID", CountryName: "Indonesia", Days: 30, Price: 7000}
	user := &postgres.User{ID: 1, Balance: 50000}
	future := time.Now().Add(10 * 24 * time.Hour) // 10 hari tersisa
	client := &postgres.VPNClient{
		ID: 3, UserID: 1, ServerID: 5, Email: "a@vpn.kt", Protocol: "vless",
		UUID: "client-uuid", ExpiresAt: &future,
	}

	store := newFakeStores()
	store.plans.plan = plan
	store.clients.owned = client
	store.users.balanceAfter = 43000
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels)

	res, err := svc.Renew(context.Background(), user, 3, "ID", 30)
	if err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if res.Status != domain.OrderCompleted {
		t.Fatalf("status = %s", res.Status)
	}
	if res.OrderID == "" {
		t.Error("order id missing")
	}
	if !store.panels.renewCalled {
		t.Error("panel renew must be called")
	}
	// v1.37 debit-first: money moves before the panel call, so a failed panel
	// renewal can always be refunded exactly.
	if !store.debitBeforePanel {
		t.Error("debit must happen BEFORE the panel call (v1.37 renewal ordering)")
	}
	wantExpiry := future.AddDate(0, 0, 30)
	if store.clients.expiryUpdated == nil || !store.clients.expiryUpdated.Equal(wantExpiry) {
		t.Errorf("expiry = %v, want %v (base = remaining 10d)", store.clients.expiryUpdated, wantExpiry)
	}
	if store.saved[0].OrderType != string(domain.OrderTypeRenewal) {
		t.Errorf("order type = %s", store.saved[0].OrderType)
	}
}

func TestRenew_GivenForeignClient_ThenErrClientNotFound(t *testing.T) {
	user := &postgres.User{ID: 1, Balance: 50000}
	store := newFakeStores()
	store.clients.ownedErr = ErrClientNotFound
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels)

	if _, err := svc.Renew(context.Background(), user, 99, "ID", 30); !errors.Is(err, ErrClientNotFound) {
		t.Fatalf("err = %v, want ErrClientNotFound", err)
	}
}
