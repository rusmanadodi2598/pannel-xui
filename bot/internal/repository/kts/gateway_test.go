// Package kts tests the PG charge HTTP client (spec 015).
//
// @file      internal/repository/kts/gateway_test.go
// @for       httptest: create/confirm/verify parsing + S2S headers + errors.
// @uses      testing, context, errors, net/http, net/http/httptest, time
// @reason    The bot's money path rides on this client; the tests pin the
// envelope parsing, the signed headers and the error mapping (AGENTS.md §2.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability experimental
// @since     2026-08-18
package kts

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCreateCharge_Given201_ThenParsesChargeAndSigns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "k1" {
			t.Errorf("X-API-Key = %q", r.Header.Get("X-API-Key"))
		}
		if r.Header.Get("X-Signature") == "" {
			t.Error("X-Signature missing")
		}
		if r.Header.Get("X-Timestamp") == "" || r.Header.Get("X-Nonce") == "" {
			t.Error("timestamp/nonce missing")
		}
		if r.Header.Get("Idempotency-Key") != "tp_1" {
			t.Errorf("Idempotency-Key = %q, want tp_1", r.Header.Get("Idempotency-Key"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"orderId":"tp_1","status":"created","amount":{"amount":10000,"currency":"IDR"}},"meta":{}}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL, "k1", "secret", 5*time.Second)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ch, err := c.CreateCharge(context.Background(), CreateChargeRequest{
		OrderID: "tp_1",
		Amount:  Money{Amount: 10000, Currency: "IDR"},
	})
	if err != nil {
		t.Fatalf("CreateCharge: %v", err)
	}
	if ch.OrderID != "tp_1" || ch.Status != "created" || ch.Amount.Amount != 10000 {
		t.Errorf("charge = %+v", ch)
	}
}

func TestConfirmCharge_Given202_ThenCheckoutURLParsed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/pg/charges/tp_1/confirm" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"data":{"orderId":"tp_1","status":"pending","checkoutUrl":"https://api.midtrans.com/v2/qris/abc"},"meta":{}}`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "k1", "secret", 5*time.Second)
	ch, err := c.ConfirmCharge(context.Background(), "tp_1")
	if err != nil {
		t.Fatalf("ConfirmCharge: %v", err)
	}
	if ch.Status != "pending" || ch.CheckoutURL != "https://api.midtrans.com/v2/qris/abc" {
		t.Errorf("charge = %+v", ch)
	}
}

func TestGetCharge_Given404_ThenErrChargeNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"PAYMENT_NOT_FOUND","message":"payment not found"},"meta":{}}`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "k1", "secret", 5*time.Second)
	_, err := c.GetCharge(context.Background(), "tp_missing")
	if !errors.Is(err, ErrChargeNotFound) {
		t.Fatalf("err = %v, want ErrChargeNotFound", err)
	}
}

func TestCreateCharge_Given409Duplicate_ThenErrDuplicateOrder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"code":"DUPLICATE_ORDER","message":"order id already owned"},"meta":{}}`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "k1", "secret", 5*time.Second)
	_, err := c.CreateCharge(context.Background(), CreateChargeRequest{OrderID: "tp_dup", Amount: Money{Amount: 1000, Currency: "IDR"}})
	if !errors.Is(err, ErrDuplicateOrder) {
		t.Fatalf("err = %v, want ErrDuplicateOrder", err)
	}
}

func TestNew_GivenBadBaseURL_ThenError(t *testing.T) {
	if _, err := New("not-a-url", "k1", "secret", time.Second); err == nil {
		t.Fatal("expected error for invalid base url")
	}
}
