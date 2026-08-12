// Package ordersvc_test covers the FR-07 trial order path.
//
// @file      internal/service/order/order_trial_test.go
// @for       CreateTrial: order type trial, no debit, is_trial client, failure paths.
// @uses      testing, context, errors, time, internal/domain, internal/repository/postgres
// @reason    Trial must never debit balance and must never be charged twice —
// the order path is locked separately from the purchase path (AGENTS.md §2.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
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

var errTrialPanel = errors.New("panel unreachable")

func newTrialStore() *fakeStores {
	f := newFakeStores()
	f.users.balanceAfter = 50000
	f.panels.created = domain.PanelClient{
		InboundID: 5, Email: "x@vpn.kt", UUID: "uuid-1", Protocol: "vless",
	}
	return f
}

func TestCreateTrial_GivenValid_ThenTrialOrderNoDebit(t *testing.T) {
	f := newTrialStore()
	svc := New(f.orders, f.clients, f.users, f.plans, f.servers, f.panels)

	user := &postgres.User{ID: 9, Balance: 50000}
	res, err := svc.CreateTrial(context.Background(), user, 3, TrialSpec{Hours: 1, TrafficGB: 1, IPLimit: 1})
	if err != nil {
		t.Fatalf("CreateTrial: %v", err)
	}
	if res.Status != domain.OrderCompleted {
		t.Errorf("Status = %s, want completed", res.Status)
	}
	if f.debited != 0 {
		t.Errorf("debited = %d, want 0 (trial is free)", f.debited)
	}
	if res.BalanceAfter != 50000 {
		t.Errorf("BalanceAfter = %d, want unchanged 50000", res.BalanceAfter)
	}
	if len(f.orders.created) != 1 || f.orders.created[0].OrderType != string(domain.OrderTypeTrial) {
		t.Errorf("order type = %+v, want trial", f.orders.created)
	}
	if len(f.clients.created) != 1 || !f.clients.created[0].IsTrial {
		t.Errorf("client row = %+v, want is_trial=true", f.clients.created)
	}
}

func TestCreateTrial_GivenPanelFailure_ThenFailedNoClient(t *testing.T) {
	f := newTrialStore()
	f.panels.createErr = errTrialPanel
	svc := New(f.orders, f.clients, f.users, f.plans, f.servers, f.panels)

	user := &postgres.User{ID: 9, Balance: 50000}
	res, err := svc.CreateTrial(context.Background(), user, 3, TrialSpec{Hours: 1, TrafficGB: 1, IPLimit: 1})
	if err != ErrFulfillFailed {
		t.Fatalf("err = %v, want ErrFulfillFailed", err)
	}
	if res.Status != domain.OrderFailed {
		t.Errorf("Status = %s, want failed", res.Status)
	}
	if len(f.clients.created) != 0 {
		t.Errorf("client created on panel failure: %+v", f.clients.created)
	}
	if f.debited != 0 {
		t.Errorf("debited = %d, want 0", f.debited)
	}
}

func TestCreateTrial_GivenPinnedInbound_ThenProvisionedOnIt(t *testing.T) {
	f := newTrialStore()
	svc := New(f.orders, f.clients, f.users, f.plans, f.servers, f.panels)

	user := &postgres.User{ID: 9, Balance: 50000}
	_, err := svc.CreateTrial(context.Background(), user, 3,
		TrialSpec{Hours: 1, TrafficGB: 1, IPLimit: 1, InboundID: 7, Protocol: "trojan"})
	if err != nil {
		t.Fatalf("CreateTrial: %v", err)
	}
	if f.panels.trialInboundID != 7 {
		t.Errorf("trial inbound = %d, want 7 (pinned)", f.panels.trialInboundID)
	}
	if got := f.orders.created[0].Protocol; got != "trojan" {
		t.Errorf("order protocol = %q, want trojan", got)
	}
}

func TestCreateTrial_GivenCompleted_ThenNewExpiryAboutOneHour(t *testing.T) {
	f := newTrialStore()
	svc := New(f.orders, f.clients, f.users, f.plans, f.servers, f.panels)

	before := time.Now()
	res, err := svc.CreateTrial(context.Background(), &postgres.User{ID: 9, Balance: 50000}, 3,
		TrialSpec{Hours: 1, TrafficGB: 1, IPLimit: 1})
	if err != nil {
		t.Fatalf("CreateTrial: %v", err)
	}
	if res.NewExpiry.Before(before) || res.NewExpiry.After(before.Add(70*time.Minute)) {
		t.Errorf("NewExpiry = %v, want ~1 hour from now", res.NewExpiry)
	}
}
