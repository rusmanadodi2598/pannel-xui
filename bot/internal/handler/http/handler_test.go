// Package httphandler_test covers the /api/v1 HTTP surface (AGENTS.md §2.1).
//
// @file      internal/handler/http/handler_test.go
// @for       httptest: health with DB/Redis states, telegram/payment webhook stubs.
// @uses      testing, net/http/httptest, io, log/slog, context
// @reason    Guards the versioned REST contract and secret-token check (PRD §26).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package httphandler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testSecret = "0123456789abcdef0123456789abcdef"

// fakePinger simulates a dependency (DB/Redis) health state.
type fakePinger struct {
	err error
}

func (f fakePinger) Ping(ctx context.Context) error { return f.err }

func newTestHandler(dbErr, redisErr error) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(Options{
		Logger:        logger,
		Version:       "test",
		WebhookPath:   "/api/v1/webhooks/telegram",
		WebhookSecret: testSecret,
		DB:            fakePinger{err: dbErr},
		Redis:         fakePinger{err: redisErr},
	})
}

func do(t *testing.T, h http.Handler, method, path string, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealth_GivenAllDependenciesUp_Then200OK(t *testing.T) {
	rec := do(t, newTestHandler(nil, nil), http.MethodGet, "/api/v1/health", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != `{"db":"ok","redis":"ok","status":"ok","version":"test","webhook":"registered"}`+"\n" {
		t.Errorf("body = %s", got)
	}
}

func TestHealth_GivenDBDown_Then503Degraded(t *testing.T) {
	rec := do(t, newTestHandler(errors.New("db down"), nil), http.MethodGet, "/api/v1/health", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if got := rec.Body.String(); got != `{"db":"error","redis":"ok","status":"degraded","version":"test","webhook":"registered"}`+"\n" {
		t.Errorf("body = %s", got)
	}
}

func TestHealth_GivenRedisDown_Then503Degraded(t *testing.T) {
	rec := do(t, newTestHandler(nil, errors.New("redis down")), http.MethodGet, "/health", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestHealth_GivenNoDepsConfigured_ThenStillAnswers(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := New(Options{
		Logger:        logger,
		Version:       "test",
		WebhookPath:   "/api/v1/webhooks/telegram",
		WebhookSecret: testSecret,
	})
	rec := do(t, h, http.MethodGet, "/api/v1/health", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestWebhookTelegram_GivenMalformedBody_Then400(t *testing.T) {
	rec := do(t, newTestHandler(nil, nil), http.MethodPost, "/api/v1/webhooks/telegram",
		map[string]string{"X-Telegram-Bot-Api-Secret-Token": testSecret})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for empty/malformed body", rec.Code)
	}
}

func TestWebhookTelegram_GivenInvalidSecret_Then403(t *testing.T) {
	rec := do(t, newTestHandler(nil, nil), http.MethodPost, "/api/v1/webhooks/telegram",
		map[string]string{"X-Telegram-Bot-Api-Secret-Token": "wrong"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestWebhookTelegram_GivenMissingSecret_Then403(t *testing.T) {
	rec := do(t, newTestHandler(nil, nil), http.MethodPost, "/api/v1/webhooks/telegram", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestWebhookPayments_GivenPost_Then501Stub(t *testing.T) {
	rec := do(t, newTestHandler(nil, nil), http.MethodPost, "/api/v1/webhooks/payments", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
}

func TestUnknownPath_Then404(t *testing.T) {
	rec := do(t, newTestHandler(nil, nil), http.MethodGet, "/api/v1/nope", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
