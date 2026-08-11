// Package httphandler test covers the real Telegram webhook ingestion path.
//
// @file      internal/handler/http/webhook_test.go
// @for       Secret check, update parsing, update_id dedup, worker enqueue (PRD §14.2/14.3).
// @uses      testing, net/http/httptest, context, io, log/slog, strings, time, github.com/go-telegram/bot/models
// @reason    Guards the ingestion contract Telegram depends on for retries.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package httphandler

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
)

type fakeEnqueuer struct {
	updates []*models.Update
}

func (f *fakeEnqueuer) Enqueue(_ context.Context, upd *models.Update) bool {
	f.updates = append(f.updates, upd)
	return true
}

type fakeDedup struct {
	seen   map[string]bool
	fail   bool
	called int
}

func (f *fakeDedup) SetNX(_ context.Context, key, _ string, _ time.Duration) (bool, error) {
	f.called++
	if f.fail {
		return false, errBoom
	}
	if f.seen[key] {
		return false, nil
	}
	f.seen[key] = true
	return true, nil
}

func webhookHandler(t *testing.T, enq UpdateEnqueuer, dedup DedupStore) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(Options{
		Logger:        logger,
		Version:       "test",
		WebhookPath:   "/api/v1/webhooks/telegram",
		WebhookSecret: testSecret,
		DB:            fakePinger{},
		Redis:         fakePinger{},
		Worker:        enq,
		Dedup:         dedup,
	})
}

func doBody(t *testing.T, h http.Handler, method, path, body string, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

const validUpdate = `{"update_id":123,"message":{"message_id":1,"date":0,"chat":{"id":7,"type":"private"},"from":{"id":7,"is_bot":false,"first_name":"T"},"text":"/start"}}`

func TestWebhook_GivenValidUpdate_Then200AndEnqueued(t *testing.T) {
	enq := &fakeEnqueuer{}
	h := webhookHandler(t, enq, &fakeDedup{seen: map[string]bool{}})

	rec := doBody(t, h, http.MethodPost, "/api/v1/webhooks/telegram", validUpdate,
		map[string]string{"X-Telegram-Bot-Api-Secret-Token": testSecret})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(enq.updates) != 1 || enq.updates[0].ID != 123 {
		t.Fatalf("enqueued = %+v", enq.updates)
	}
}

func TestWebhook_GivenDuplicateUpdate_Then200DedupNotEnqueued(t *testing.T) {
	enq := &fakeEnqueuer{}
	h := webhookHandler(t, enq, &fakeDedup{seen: map[string]bool{}})
	header := map[string]string{"X-Telegram-Bot-Api-Secret-Token": testSecret}

	doBody(t, h, http.MethodPost, "/api/v1/webhooks/telegram", validUpdate, header)
	rec := doBody(t, h, http.MethodPost, "/api/v1/webhooks/telegram", validUpdate, header)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); !strings.Contains(got, `"dedup":"true"`) {
		t.Errorf("body = %s, want dedup marker", got)
	}
	if len(enq.updates) != 1 {
		t.Fatalf("enqueued = %d, want 1 (duplicate dropped)", len(enq.updates))
	}
}

func TestWebhook_GivenDedupError_Then503(t *testing.T) {
	enq := &fakeEnqueuer{}
	h := webhookHandler(t, enq, &fakeDedup{seen: map[string]bool{}, fail: true})
	rec := doBody(t, h, http.MethodPost, "/api/v1/webhooks/telegram", validUpdate,
		map[string]string{"X-Telegram-Bot-Api-Secret-Token": testSecret})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestWebhook_GivenNoWorker_Then503(t *testing.T) {
	h := webhookHandler(t, nil, &fakeDedup{seen: map[string]bool{}})
	rec := doBody(t, h, http.MethodPost, "/api/v1/webhooks/telegram", validUpdate,
		map[string]string{"X-Telegram-Bot-Api-Secret-Token": testSecret})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestWebhook_GivenInvalidSecret_Then403WithoutProcessing(t *testing.T) {
	enq := &fakeEnqueuer{}
	h := webhookHandler(t, enq, &fakeDedup{seen: map[string]bool{}})
	rec := doBody(t, h, http.MethodPost, "/api/v1/webhooks/telegram", validUpdate,
		map[string]string{"X-Telegram-Bot-Api-Secret-Token": "wrong"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if len(enq.updates) != 0 {
		t.Fatalf("updates processed despite bad secret: %+v", enq.updates)
	}
}
