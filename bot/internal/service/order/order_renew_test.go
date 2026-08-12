// Package ordersvc_test covers the v1.37 renewal invariants (AGENTS.md §2.1).
//
// @file      internal/service/order/order_renew_test.go
// @for       Paid-only renew, in-flight dedup, debit-first + auto-refund balance tests.
// @uses      testing, context, errors, time, internal/domain, internal/repository/postgres
// @reason    The user's money must stay precise across renew: never negative
// (atomic guard), never lost (refund on failure), never double-moved
// (idempotence) — every condition locked with a Given-When-Then test.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
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

func renewPlan() *domain.VpnPlan {
	return &domain.VpnPlan{CountryCode: "ID", CountryName: "Indonesia", Days: 30, Price: 7000}
}

func renewUser() *postgres.User {
	return &postgres.User{ID: 1, Balance: 50000}
}

func renewClient() *postgres.VPNClient {
	future := time.Now().Add(10 * 24 * time.Hour)
	return &postgres.VPNClient{
		ID: 3, UserID: 1, ServerID: 5, Email: "a@vpn.kt", Protocol: "vless",
		UUID: "client-uuid", ExpiresAt: &future,
	}
}

func TestRenew_GivenTrialClient_ThenErrTrialNotRenewableAndNothingMoves(t *testing.T) {
	store := newFakeStores()
	store.plans.plan = renewPlan()
	client := renewClient()
	client.IsTrial = true
	store.clients.owned = client
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels)

	_, err := svc.Renew(context.Background(), renewUser(), 3, "ID", 30)
	if !errors.Is(err, ErrTrialNotRenewable) {
		t.Fatalf("err = %v, want ErrTrialNotRenewable", err)
	}
	if len(store.orders.created) != 0 || store.debited != 0 || store.panels.renewCalled {
		t.Error("trial renew must not create an order, debit or touch the panel (v1.37)")
	}
}

func TestRenew_GivenInFlightOrder_ThenErrOrderInFlightAndNothingMoves(t *testing.T) {
	store := newFakeStores()
	store.plans.plan = renewPlan()
	store.clients.owned = renewClient()
	store.orders.inFlight = &postgres.Order{ID: 9, OrderType: string(domain.OrderTypeRenewal), Status: string(domain.OrderProcessing)}
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels)

	_, err := svc.Renew(context.Background(), renewUser(), 3, "ID", 30)
	if !errors.Is(err, ErrOrderInFlight) {
		t.Fatalf("err = %v, want ErrOrderInFlight", err)
	}
	if len(store.orders.created) != 0 || store.debited != 0 || store.panels.renewCalled {
		t.Error("duplicate execution must not create an order, debit or touch the panel (v1.37)")
	}
}

func TestPurchase_GivenInFlightOrder_ThenErrOrderInFlight(t *testing.T) {
	store := newFakeStores()
	store.plans.plan = renewPlan()
	store.servers.serverID = 5
	store.orders.inFlight = &postgres.Order{ID: 9, OrderType: string(domain.OrderTypePurchase), Status: string(domain.OrderProcessing)}
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels)

	_, err := svc.Purchase(context.Background(), renewUser(), "ID", 30, 0, 0, "vless")
	if !errors.Is(err, ErrOrderInFlight) {
		t.Fatalf("err = %v, want ErrOrderInFlight", err)
	}
	if len(store.orders.created) != 0 || store.debited != 0 {
		t.Error("duplicate purchase must not create an order or debit (v1.37)")
	}
}

func TestRenew_GivenPanelError_ThenRefundedAndFailed(t *testing.T) {
	store := newFakeStores()
	store.plans.plan = renewPlan()
	store.clients.owned = renewClient()
	store.panels.renewErr = errors.New("panel unreachable")
	store.users.balanceAfter = 43000
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels)

	_, err := svc.Renew(context.Background(), renewUser(), 3, "ID", 30)
	if !errors.Is(err, ErrFulfillFailed) {
		t.Fatalf("err = %v, want ErrFulfillFailed", err)
	}
	if store.debited != 1 {
		t.Errorf("debited = %d, want 1 (money moved once before the panel call)", store.debited)
	}
	if len(store.users.credited) != 1 {
		t.Fatalf("refunds = %d, want 1", len(store.users.credited))
	}
	if store.orders.created[0].OrderID != store.users.credited[0] {
		t.Errorf("refund orderID = %s, want %s (same order as the debit)", store.users.credited[0], store.orders.created[0].OrderID)
	}
	if last := store.saved[len(store.saved)-1]; last.Status != string(domain.OrderFailed) {
		t.Errorf("final order save status = %s, want failed", last.Status)
	}
	if store.clients.expiryUpdated != nil {
		t.Error("client expiry must NOT be updated in DB when the panel fails")
	}
}

func TestRenew_GivenExpiryUpdateError_ThenRefundedAndFailed(t *testing.T) {
	store := newFakeStores()
	store.plans.plan = renewPlan()
	store.clients.owned = renewClient()
	store.clients.expiryErr = errors.New("db down")
	store.users.balanceAfter = 43000
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels)

	_, err := svc.Renew(context.Background(), renewUser(), 3, "ID", 30)
	if !errors.Is(err, ErrFulfillFailed) {
		t.Fatalf("err = %v, want ErrFulfillFailed", err)
	}
	if len(store.users.credited) != 1 {
		t.Errorf("refunds = %d, want 1 (DB expiry failure must refund)", len(store.users.credited))
	}
	if last := store.saved[len(store.saved)-1]; last.Status != string(domain.OrderFailed) {
		t.Errorf("final order save status = %s, want failed", last.Status)
	}
}

func TestRenew_GivenDebitError_ThenFailedWithoutPanelOrRefund(t *testing.T) {
	store := newFakeStores()
	store.plans.plan = renewPlan()
	store.clients.owned = renewClient()
	store.users.debitErr = errors.New("db down")
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels)

	_, err := svc.Renew(context.Background(), renewUser(), 3, "ID", 30)
	if !errors.Is(err, ErrFulfillFailed) {
		t.Fatalf("err = %v, want ErrFulfillFailed", err)
	}
	if store.debited != 0 || len(store.users.credited) != 0 {
		t.Error("money must not move when the debit itself fails")
	}
	if store.panels.renewCalled {
		t.Error("panel must not be touched when the debit fails (debit-first)")
	}
	if last := store.saved[len(store.saved)-1]; last.Status != string(domain.OrderFailed) {
		t.Errorf("final order save status = %s, want failed", last.Status)
	}
}

func TestRenew_GivenPanelErrorAndRefundFailure_ThenOrderStillFailed(t *testing.T) {
	store := newFakeStores()
	store.plans.plan = renewPlan()
	store.clients.owned = renewClient()
	store.panels.renewErr = errors.New("panel unreachable")
	store.users.creditErr = errors.New("refund db down")
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels)

	_, err := svc.Renew(context.Background(), renewUser(), 3, "ID", 30)
	if !errors.Is(err, ErrFulfillFailed) {
		t.Fatalf("err = %v, want ErrFulfillFailed", err)
	}
	// The order still lands in failed even when the refund cannot be written —
	// the unmatched debit ledger row is the reconciliation trail (v1.37).
	if last := store.saved[len(store.saved)-1]; last.Status != string(domain.OrderFailed) {
		t.Errorf("final order save status = %s, want failed", last.Status)
	}
	if store.debited != 1 {
		t.Errorf("debited = %d, want 1", store.debited)
	}
}

func TestRenew_GivenVlessClient_ThenPanelKeyIsUUID(t *testing.T) {
	// v1.38: the panel client key must follow the protocol — vless/vmess → UUID.
	store := newFakeStores()
	store.plans.plan = renewPlan()
	client := renewClient()
	client.UUID = "client-uuid"
	client.Password = "ignored"
	store.clients.owned = client
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels)

	if _, err := svc.Renew(context.Background(), renewUser(), 3, "ID", 30); err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if store.panels.renewClientID != "client-uuid" {
		t.Errorf("panel key = %q, want client-uuid (vless→UUID)", store.panels.renewClientID)
	}
}

func TestRenew_GivenShadowsocksClient_ThenPanelKeyIsEmail(t *testing.T) {
	// v1.38: x-ui keys ss clients by EMAIL, not password — a wrong key would
	// fail "empty client ID" on the panel and burn a refund cycle.
	store := newFakeStores()
	store.plans.plan = renewPlan()
	client := renewClient()
	client.Protocol = "shadowsocks"
	client.UUID = ""
	client.Password = "ss-secret"
	store.clients.owned = client
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels)

	if _, err := svc.Renew(context.Background(), renewUser(), 3, "ID", 30); err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if store.panels.renewClientID != "a@vpn.kt" {
		t.Errorf("panel key = %q, want a@vpn.kt (shadowsocks→email)", store.panels.renewClientID)
	}
}

func TestRenew_GivenTrojanClient_ThenPanelKeyIsPassword(t *testing.T) {
	store := newFakeStores()
	store.plans.plan = renewPlan()
	client := renewClient()
	client.Protocol = "trojan"
	client.UUID = ""
	client.Password = "trojan-secret"
	store.clients.owned = client
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels)

	if _, err := svc.Renew(context.Background(), renewUser(), 3, "ID", 30); err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if store.panels.renewClientID != "trojan-secret" {
		t.Errorf("panel key = %q, want trojan-secret (trojan→password)", store.panels.renewClientID)
	}
}

func TestRenew_GivenClientWithoutCredential_ThenRejectedBeforeAnySideEffect(t *testing.T) {
	// v1.38: a row with no panel credential (legacy/corrupt) can never be
	// renewed on the panel — reject before order/debit/panel (guard parity
	// with the delete flow).
	store := newFakeStores()
	store.plans.plan = renewPlan()
	client := renewClient()
	client.Protocol = "vless"
	client.UUID = ""
	client.Password = ""
	store.clients.owned = client
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels)

	_, err := svc.Renew(context.Background(), renewUser(), 3, "ID", 30)
	if !errors.Is(err, ErrClientNotFound) {
		t.Fatalf("err = %v, want ErrClientNotFound", err)
	}
	if len(store.orders.created) != 0 || store.debited != 0 || store.panels.renewCalled {
		t.Error("renewal without credential must not create an order, debit or touch the panel")
	}
}

func TestRenew_GivenDebitInsufficient_ThenErrInsufficientBalance(t *testing.T) {
	store := newFakeStores()
	store.plans.plan = renewPlan()
	store.clients.owned = renewClient()
	store.users.debitErr = postgres.ErrInsufficientBalance
	svc := New(store.orders, store.clients, store.users, store.plans, store.servers, store.panels)

	_, err := svc.Renew(context.Background(), renewUser(), 3, "ID", 30)
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("err = %v, want ErrInsufficientBalance", err)
	}
	if store.panels.renewCalled || len(store.users.credited) != 0 {
		t.Error("no panel call and no refund when the atomic guard rejects the debit")
	}
}
