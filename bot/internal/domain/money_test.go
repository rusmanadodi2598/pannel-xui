// Package domain_test covers value objects & the order state machine (AGENTS.md §2.1).
//
// @file      internal/domain/money_test.go
// @for       Unit tests: Money formatting, Order transitions, VpnPlan naming, random ids.
// @uses      testing, strings
// @reason    Guards the DDD invariants every service depends on.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     domain
// @stability stable
// @since     2026-08-11
package domain

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestMoneyFormatIDR(t *testing.T) {
	cases := []struct {
		rupiah int64
		want   string
	}{
		{0, "Rp 0"},
		{4000, "Rp 4.000"},
		{7000, "Rp 7.000"},
		{20000, "Rp 20.000"},
		{23000, "Rp 23.000"},
		{5000000, "Rp 5.000.000"},
	}
	for _, tc := range cases {
		if got := Money(tc.rupiah).FormatIDR(); got != tc.want {
			t.Errorf("Money(%d).FormatIDR() = %q, want %q", tc.rupiah, got, tc.want)
		}
	}
}

func TestNewMoney_GivenNegative_ThenError(t *testing.T) {
	if _, err := NewMoney(-1); err == nil {
		t.Fatal("NewMoney(-1): expected error")
	}
}

func TestMoneyAdd_GivenSum_ThenAdded(t *testing.T) {
	got, err := Money(4000).Add(Money(3000))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got != Money(7000) {
		t.Errorf("Add = %d, want 7000", got)
	}
}

func TestMoneyAdd_GivenOverflow_ThenError(t *testing.T) {
	if _, err := Money(math.MaxInt64).Add(Money(1)); err != ErrMoneyOverflow {
		t.Errorf("Add overflow: err = %v, want ErrMoneyOverflow", err)
	}
}

func TestMoneySub_GivenSufficient_ThenDifference(t *testing.T) {
	got, err := Money(7000).Sub(Money(2000))
	if err != nil {
		t.Fatalf("Sub: %v", err)
	}
	if got != Money(5000) {
		t.Errorf("Sub = %d, want 5000", got)
	}
}

func TestMoneySub_GivenNegativeResult_ThenError(t *testing.T) {
	if _, err := Money(1000).Sub(Money(2000)); err != ErrNegativeMoney {
		t.Errorf("Sub negative: err = %v, want ErrNegativeMoney", err)
	}
}

func TestOrderTransition_GivenValidSequence_ThenCompleted(t *testing.T) {
	o := NewOrder("KTS-TEST1234-VPN", OrderTypePurchase, 1, 2, "vless", 30, Money(7000))
	if o.Status != OrderPending {
		t.Fatalf("status = %s, want pending", o.Status)
	}
	if err := o.Transition(OrderProcessing); err != nil {
		t.Fatalf("pending→processing: %v", err)
	}
	if err := o.Complete(Money(5000)); err != nil {
		t.Fatalf("processing→completed: %v", err)
	}
	if o.BalanceAfter != Money(5000) || o.CompletedAt == nil {
		t.Errorf("complete side effects missing: %+v", o)
	}
	if !o.IsTerminal() {
		t.Error("completed must be terminal")
	}
}

func TestOrderTransition_GivenInvalidMove_ThenRejected(t *testing.T) {
	o := NewOrder("KTS-TEST1234-VPN", OrderTypePurchase, 1, 2, "vless", 30, Money(7000))
	if err := o.Transition(OrderCompleted); err == nil {
		t.Fatal("pending→completed must be rejected (must go through processing)")
	}
	if err := o.MarkFailed("panel down"); err != nil {
		t.Fatalf("pending→failed must be valid: %v", err)
	}
	if o.Status != OrderFailed || o.ErrorMessage != "panel down" {
		t.Errorf("failed state = %+v", o)
	}
	if err := o.Transition(OrderProcessing); err == nil {
		t.Fatal("terminal state must reject further moves")
	}
}

func TestParseOrderType(t *testing.T) {
	if got, _ := ParseOrderType("renewal"); got != OrderTypeRenewal {
		t.Errorf("ParseOrderType(renewal) = %s", got)
	}
	if got, _ := ParseOrderType("deletion"); got != OrderTypeDeletion {
		t.Errorf("ParseOrderType(deletion) = %s", got)
	}
	if _, err := ParseOrderType("bogus"); err == nil {
		t.Error("ParseOrderType(bogus): expected error")
	}
}

func TestNewDeletionRecord_GivenAccount_ThenCompletedZeroAmount(t *testing.T) {
	o := NewDeletionRecord("KTS-TEST1234-VPN", 1, 2, "vless", "del@vpn.kt")
	if o.Type != OrderTypeDeletion || o.Status != OrderCompleted {
		t.Errorf("deletion record = type %s status %s, want deletion/completed", o.Type, o.Status)
	}
	if !o.FinalAmount.IsZero() || o.CompletedAt == nil {
		t.Errorf("deletion record must be zero-amount and completed: %+v", o)
	}
	if o.AccountEmail != "del@vpn.kt" || o.Protocol != "vless" || o.ServerID != 2 || o.UserID != 1 {
		t.Errorf("deletion record fields = %+v", o)
	}
	if !o.IsTerminal() {
		t.Error("deletion record starts completed — must be terminal")
	}
}

func TestVpnPlan_GivenID30_ThenNamesMatchSeed(t *testing.T) {
	p, err := NewVpnPlan("id", 30, Money(7000), true)
	if err != nil {
		t.Fatalf("NewVpnPlan: %v", err)
	}
	if p.Name() != "VPN Indonesia 30 Hari" {
		t.Errorf("Name() = %q", p.Name())
	}
	if p.Code() != "ID30" {
		t.Errorf("Code() = %q", p.Code())
	}
	if p.ValidityLabel() != "30 Hari" {
		t.Errorf("ValidityLabel() = %q", p.ValidityLabel())
	}
}

func TestNewVPNClient_GivenValid_ThenExpiryAndQuota(t *testing.T) {
	c, err := NewVPNClient(1, 2, 3, "ktsx@vpn.kt", "vless", "uuid", "", 30, 100, 1)
	if err != nil {
		t.Fatalf("NewVPNClient: %v", err)
	}
	if c.ExpiresAt == nil {
		t.Fatal("expiry must be set")
	}
	if c.TrafficLimit != 100*1024*1024*1024 {
		t.Errorf("TrafficLimit = %d, want 100GB in bytes", c.TrafficLimit)
	}
	if c.Expired(c.ExpiresAt.Add(-time.Second)) {
		t.Error("must not be expired before expiry")
	}
	if !c.Expired(c.ExpiresAt.Add(time.Second)) {
		t.Error("must be expired after expiry")
	}
}

func TestNewOrderID_GivenCall_ThenKTSFormatAndUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		id := NewOrderID()
		if !strings.HasPrefix(id, "KTS-") || !strings.HasSuffix(id, "-VPN") || len(id) != 16 {
			t.Fatalf("bad order id %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate order id %q", id)
		}
		seen[id] = true
	}
}

func TestNewUUID_GivenCall_ThenVersion4(t *testing.T) {
	id := NewUUID()
	if len(id) != 36 || id[14] != '4' {
		t.Fatalf("bad uuid %q", id)
	}
}
