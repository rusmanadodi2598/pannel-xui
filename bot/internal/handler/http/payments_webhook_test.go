// Package httphandler tests also cover the pg.charge webhook (Phase 4).
//
// @file      internal/handler/http/payments_webhook_test.go
// @for       httptest: signature verification, event branch, dedup, settlement.
// @uses      testing, context, errors, io, log/slog, net/http, net/http/httptest,
// time, internal/repository/kts, internal/repository/redis, internal/service/topup
// @reason    The webhook is the money path: forged signature → 403, duplicate
// delivery → 200 dedup, unknown event → 400 (AGENTS.md §2.1 + §1.6).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability experimental
// @since     2026-08-18
package httphandler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/kts"
	topupsvc "github.com/kentangtech/bot-order/internal/service/topup"
)

// testPaymentSecret mirrors a merchant secretKey (≥16 chars).
const testPaymentSecret = "0123456789abcdef0123456789abcdef"

type fakeSettler struct {
	applied []string
	err     error
}

func (f *fakeSettler) ApplySettlement(ctx context.Context, orderID, status string) (*topupsvc.SettlementResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.applied = append(f.applied, orderID)
	return &topupsvc.SettlementResult{OrderID: orderID, Status: status, BalanceAfter: 10000}, nil
}

type fakePaymentDedup struct {
	keys     map[string]bool
	setnxErr error
}

func (f *fakePaymentDedup) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	if f.setnxErr != nil {
		return false, f.setnxErr
	}
	if f.keys == nil {
		f.keys = map[string]bool{}
	}
	if f.keys[key] {
		return false, nil
	}
	f.keys[key] = true
	return true, nil
}

// newPaymentTestHandler wires a fake settler + dedup + payment secret.
func newPaymentTestHandler(settler *fakeSettler, dedup DedupStore) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(Options{
		Logger:               logger,
		Version:              "test",
		WebhookPath:          "/api/v1/webhooks/telegram",
		WebhookSecret:        testSecret,
		Topup:                settler,
		PaymentWebhookSecret: testPaymentSecret,
		Dedup:                dedup,
	})
}

// paymentBody returns a valid pg.charge notification JSON + its signature.
func paymentBody(orderID, status string) ([]byte, string) {
	body := []byte(`{"eventType":"pg.charge","orderId":"` + orderID + `","refId":"` + orderID + `","status":"` + status + `","amount":{"amount":10300,"currency":"IDR"},"occurredAt":"2026-08-18T00:00:00Z"}`)
	return body, kts.WebhookSignature(testPaymentSecret, body)
}

// doPaymentBody posts a raw body (the webhook signature covers the exact bytes).
func doPaymentBody(t *testing.T, h http.Handler, method, path string, header map[string]string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPaymentsWebhook_GivenValidSignature_ThenSettlesAnd200(t *testing.T) {
	settler := &fakeSettler{}
	h := newPaymentTestHandler(settler, &fakePaymentDedup{})
	body, sig := paymentBody("tp_1", "succeeded")
	rec := doPaymentBody(t, h, http.MethodPost, "/api/v1/webhooks/payments", map[string]string{
		"X-Webhook-Signature": sig,
		"X-Webhook-Event":     kts.EventPGCharge,
		"X-Webhook-Id":        "pg.charge.tp_1.succeeded",
	}, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(settler.applied) != 1 || settler.applied[0] != "tp_1" {
		t.Errorf("settled = %v, want [tp_1]", settler.applied)
	}
}

func TestPaymentsWebhook_GivenInvalidSignature_Then403NoSettle(t *testing.T) {
	settler := &fakeSettler{}
	h := newPaymentTestHandler(settler, &fakePaymentDedup{})
	body, _ := paymentBody("tp_1", "succeeded")
	rec := doPaymentBody(t, h, http.MethodPost, "/api/v1/webhooks/payments", map[string]string{
		"X-Webhook-Signature": "sha256=deadbeef",
		"X-Webhook-Event":     kts.EventPGCharge,
		"X-Webhook-Id":        "pg.charge.tp_1.succeeded",
	}, body)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if len(settler.applied) != 0 {
		t.Errorf("settled = %v, want none (forged signature)", settler.applied)
	}
}

func TestPaymentsWebhook_GivenUnexpectedEvent_Then400(t *testing.T) {
	settler := &fakeSettler{}
	h := newPaymentTestHandler(settler, &fakePaymentDedup{})
	body, sig := paymentBody("tp_1", "succeeded")
	rec := doPaymentBody(t, h, http.MethodPost, "/api/v1/webhooks/payments", map[string]string{
		"X-Webhook-Signature": sig,
		"X-Webhook-Event":     "h2h.purchase",
		"X-Webhook-Id":        "h2h.purchase.pay_x.succeeded",
	}, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if len(settler.applied) != 0 {
		t.Errorf("settled = %v, want none", settler.applied)
	}
}

func TestPaymentsWebhook_GivenDuplicateWebhookID_Then200Dedup(t *testing.T) {
	settler := &fakeSettler{}
	h := newPaymentTestHandler(settler, &fakePaymentDedup{})
	body, sig := paymentBody("tp_1", "succeeded")
	headers := map[string]string{
		"X-Webhook-Signature": sig,
		"X-Webhook-Event":     kts.EventPGCharge,
		"X-Webhook-Id":        "pg.charge.tp_1.succeeded",
	}
	if rec := doPaymentBody(t, h, http.MethodPost, "/api/v1/webhooks/payments", headers, body); rec.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", rec.Code)
	}
	rec := doPaymentBody(t, h, http.MethodPost, "/api/v1/webhooks/payments", headers, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("duplicate status = %d, want 200", rec.Code)
	}
	if len(settler.applied) != 1 {
		t.Errorf("settled = %v, want exactly one settlement (dedup)", settler.applied)
	}
}

func TestPaymentsWebhook_GivenSettleFailure_Then503SoGatewayRetries(t *testing.T) {
	settler := &fakeSettler{err: errors.New("db down")}
	h := newPaymentTestHandler(settler, &fakePaymentDedup{})
	body, sig := paymentBody("tp_1", "succeeded")
	rec := doPaymentBody(t, h, http.MethodPost, "/api/v1/webhooks/payments", map[string]string{
		"X-Webhook-Signature": sig,
		"X-Webhook-Event":     kts.EventPGCharge,
		"X-Webhook-Id":        "pg.charge.tp_1.succeeded",
	}, body)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 (non-2xx → gateway retry)", rec.Code)
	}
}
