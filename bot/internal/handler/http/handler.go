// Package httphandler serves the /api/v1 HTTP surface (REST convention §26).
//
// @file      internal/handler/http/handler.go
// @for       Route registration for /api/v1/*, request logging, webhooks.
// @uses      internal/config, net/http, log/slog, crypto/subtle
// @reason    Owns the HTTP boundary so cmd/bot stays a pure composition root (AGENTS.md §1.5).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package httphandler

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/repository/redis"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

// apiBase is the versioned REST API prefix (PRD §26).
const apiBase = "/api/v1"

// Pinger reports connectivity of an external dependency (DB, Redis).
type Pinger interface {
	Ping(ctx context.Context) error
}

// UpdateEnqueuer hands a decoded Telegram update to the worker pool.
type UpdateEnqueuer interface {
	Enqueue(ctx context.Context, upd *models.Update) bool
}

// DedupStore marks seen update IDs (SETNX, TTL auto-expire).
type DedupStore interface {
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
}

// Options wires the HTTP handler's dependencies (composition root).
type Options struct {
	Logger        *slog.Logger
	Version       string
	WebhookPath   string
	WebhookSecret string
	DB            Pinger
	Redis         Pinger
	Worker        UpdateEnqueuer
	Dedup         DedupStore
	// Topup settles pg.charge webhooks (Phase 4); PaymentWebhookSecret is the
	// merchant secretKey used to verify X-Webhook-Signature (013 §2.2).
	Topup                TopupSettler
	PaymentWebhookSecret string

	// Admin REST API (PRD §26.5). RESTAPIKey empty disables the surface; the
	// concrete seams are wired from buildShop (composition root, §1.5).
	RESTAPIKey string
	Servers    ServerAdmin
	Orders     OrderAdmin
	Clients    ClientReader
	Users      UserResolver
	Topups     TopupTrigger
	Location   *time.Location
}

// New builds the /api/v1 router wrapped in the request logger.
// It panics on a malformed WebhookPath because net/http patterns require a
// leading '/' and an invalid path would only fail at the first request.
func New(opts Options) http.Handler {
	if !strings.HasPrefix(opts.WebhookPath, "/") {
		panic(fmt.Sprintf("httphandler: WebhookPath %q must start with '/'", opts.WebhookPath))
	}

	mux := http.NewServeMux()

	// GET /health — infra alias for Docker/Nginx probes, outside the business API.
	mux.HandleFunc("GET /health", opts.health)
	// GET /api/v1/health — versioned health API (PRD §26).
	mux.HandleFunc("GET "+apiBase+"/health", opts.health)

	// POST /api/v1/webhooks/telegram — ingestion update Telegram (WEBHOOK_PATH).
	// PRD §14.2: verify secret → parse update → dedup update_id → enqueue worker.
	mux.HandleFunc("POST "+opts.WebhookPath, opts.telegramWebhook)

	// POST /api/v1/webhooks/payments — pg.charge settlement (013 §2, Phase 4):
	// verifikasi X-Webhook-Signature, dedup X-Webhook-Id, kredit net atomik.
	mux.HandleFunc("POST "+apiBase+"/webhooks/payments", opts.paymentsWebhook)

	// Admin REST API (PRD §26.5) — no-op unless RESTAPIKey is set.
	opts.registerAdminAPI(mux)

	return requestLogger(opts.Logger, mux)
}

// requestLogger logs method, path, status and duration for every request.
func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote", r.RemoteAddr,
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// telegramWebhook ingests Telegram updates (PRD §14.2).
func (o Options) telegramWebhook(w http.ResponseWriter, r *http.Request) {
	// 1. Secret token — constant-time compare; mismatch → 403, no processing.
	got := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
	if subtle.ConstantTimeCompare([]byte(got), []byte(o.WebhookSecret)) != 1 {
		o.Logger.Warn("telegram webhook rejected: invalid secret token", "remote", r.RemoteAddr)
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "invalid secret token"})
		return
	}

	// 2. Decode the update (bounded body, PRD §14.2.2).
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		o.Logger.Error("reading webhook body", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	var upd models.Update
	if err := json.Unmarshal(body, &upd); err != nil {
		o.Logger.Warn("telegram webhook: malformed update", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "malformed update"})
		return
	}

	// 3. Dedup update_id (PRD §14.3) — duplicates answered 200 without work.
	if o.Dedup != nil {
		created, err := o.Dedup.SetNX(r.Context(), redis.UpdateDedupKey(upd.ID), "1", telegramservice.UpdateDedupTTL)
		if err != nil {
			o.Logger.Error("update dedup failed", "update_id", upd.ID, "error", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "dedup unavailable"})
			return
		}
		if !created {
			o.Logger.Debug("duplicate update dropped", "update_id", upd.ID)
			writeJSON(w, http.StatusOK, map[string]string{"ok": "true", "dedup": "true"})
			return
		}
	}

	// 4. Enqueue and answer 200 fast (Telegram retries on timeout).
	if o.Worker == nil {
		o.Logger.Error("telegram webhook: worker not configured")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "worker not ready"})
		return
	}
	o.Worker.Enqueue(r.Context(), &upd)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
