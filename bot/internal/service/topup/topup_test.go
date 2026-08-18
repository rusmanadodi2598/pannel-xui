// Package topupsvc tests the FR-06 fee math and PG charge flow.
//
// @file      internal/service/topup/topup_test.go
// @for       Quote formula §15.7, min/max range, CreatePayment orchestration.
// @uses      testing, context, errors, math, internal/domain, internal/repository/kts,
// internal/repository/postgres
// @reason    Guards the fee contract the QRIS amount shown to users depends on
// and the persist-then-charge ordering (Phase 4).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-11
package topupsvc

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/kts"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

func TestQuote_GivenNet10K_ThenGrossRoundsUpToHundred(t *testing.T) {
	d := newTestDeps()
	s := d.build()
	q, err := s.Quote(10000)
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	// effective = 0.025 * 1.11 = 0.02775 → gross = ceil(10000/0.97225) = 10286 → 10300
	if q.Gross != 10300 {
		t.Errorf("gross = %d, want 10300", q.Gross)
	}
	if q.TotalFee != 300 {
		t.Errorf("total fee = %d, want 300", q.TotalFee)
	}
	if q.Net != 10000 {
		t.Errorf("net = %d, want 10000", q.Net)
	}
	// Effective rate must be fee * (1 + ppn) (float compare within 1e-9).
	wantRate := 0.025 * 1.11
	if math.Abs(q.FeePercent-wantRate) > 1e-9 {
		t.Errorf("effective rate = %v, want %v", q.FeePercent, wantRate)
	}
}

func TestQuote_GivenMultipleNominals_ThenGrossAlwaysCoversNet(t *testing.T) {
	d := newTestDeps()
	s := d.build()
	for _, net := range []domain.Money{5000, 25000, 50000, 100000, 200000, 500000, 5000000} {
		q, err := s.Quote(net)
		if err != nil {
			t.Fatalf("quote %d: %v", net, err)
		}
		if q.Gross <= q.Net {
			t.Errorf("net %d: gross %d must exceed net", net, q.Gross)
		}
		if q.Gross%100 != 0 {
			t.Errorf("net %d: gross %d not rounded to ×100", net, q.Gross)
		}
		if q.TotalFee != q.Gross-q.Net {
			t.Errorf("net %d: fee %d != gross-net %d", net, q.TotalFee, q.Gross-q.Net)
		}
	}
}

func TestQuote_GivenNetBelowMin_ThenInvalidNominal(t *testing.T) {
	d := newTestDeps()
	s := d.build()
	_, err := s.Quote(1000)
	if !errors.Is(err, ErrInvalidNominal) {
		t.Fatalf("err = %v, want ErrInvalidNominal", err)
	}
}

func TestQuote_GivenNetAboveMax_ThenInvalidNominal(t *testing.T) {
	d := newTestDeps()
	s := d.build()
	_, err := s.Quote(6000000)
	if !errors.Is(err, ErrInvalidNominal) {
		t.Fatalf("err = %v, want ErrInvalidNominal", err)
	}
}

func TestMinMax_ThenBoundsExposed(t *testing.T) {
	d := newTestDeps()
	s := d.build()
	if s.MinNet() != 5000 || s.MaxNet() != 5000000 {
		t.Errorf("bounds = %d..%d, want 5000..5000000", s.MinNet(), s.MaxNet())
	}
}

func TestCreatePayment_GivenGatewayOK_ThenRowPersistedAndConfirmed(t *testing.T) {
	d := newTestDeps()
	d.gw.createResp = &kts.Charge{OrderID: "tp_x", Status: "created"}
	d.gw.confirmResp = &kts.Charge{
		OrderID: "tp_x", Status: "pending",
		CheckoutURL: "https://api.midtrans.com/v2/qris/abc",
		ExpiresAt:   time.Now().Add(24 * time.Hour),
	}
	s := d.build()

	res, err := s.CreatePayment(context.Background(), CreatePaymentRequest{
		TelegramUserID: 7, FirstName: "Budi",
		NetAmount: 10000, GrossAmount: 10300,
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if res.CheckoutURL != "https://api.midtrans.com/v2/qris/abc" {
		t.Errorf("checkout url = %q", res.CheckoutURL)
	}
	if res.Amount != 10300 {
		t.Errorf("amount = %d, want gross 10300", res.Amount)
	}
	// Amount sent to the gateway is the NET (gross-up handled gateway-side).
	if d.gw.createdReq == nil || d.gw.createdReq.Amount.Amount != 10000 {
		t.Errorf("gateway amount = %+v, want net 10000", d.gw.createdReq)
	}
	if d.gw.confirmedID == "" {
		t.Error("confirm not called")
	}
	p, err := d.store.GetByOrderID(context.Background(), res.OrderID)
	if err != nil {
		t.Fatalf("payment row: %v", err)
	}
	if p.Status != postgres.PaymentStatusPending || p.AmountNet != 10000 || p.TelegramID != 7 {
		t.Errorf("payment row = %+v, want pending net 10000 tg 7", p)
	}
}

func TestCreatePayment_GivenChargeFails_ThenRowMarkedFailed(t *testing.T) {
	d := newTestDeps()
	d.gw.createErr = kts.ErrUnauthorized
	s := d.build()

	_, err := s.CreatePayment(context.Background(), CreatePaymentRequest{
		TelegramUserID: 7, NetAmount: 10000, GrossAmount: 10300,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	// The pending row is marked failed — never a zombie pending.
	var found *postgres.Payment
	for _, p := range d.store.rows {
		found = p
	}
	if found == nil || found.Status != postgres.PaymentStatusFailed {
		t.Errorf("payment row = %+v, want failed", found)
	}
}

func TestCreatePayment_GivenPersistFails_ThenNoGatewayCall(t *testing.T) {
	d := newTestDeps()
	d.store.createErr = errors.New("db down")
	s := d.build()

	_, err := s.CreatePayment(context.Background(), CreatePaymentRequest{
		TelegramUserID: 7, NetAmount: 10000, GrossAmount: 10300,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if d.gw.createdReq != nil {
		t.Error("gateway must not be called when the row cannot persist")
	}
}
