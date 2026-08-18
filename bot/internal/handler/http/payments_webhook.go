// Package httphandler also hosts the pg.charge settlement webhook (Phase 4).
//
// @file      internal/handler/http/payments_webhook.go
// @for       POST /api/v1/webhooks/payments: verify X-Webhook-Signature, dedup
// X-Webhook-Id, settle the topup (credit only on succeeded).
// @uses      crypto/subtle, encoding/json, io, net/http, time, internal/repository/kts,
// internal/repository/redis, internal/service/topup
// @reason    The gateway retries non-2xx (013 §2), so the handler must answer
// 2xx ONLY after the settlement is durable; signature/event/dedup checks keep
// forged or duplicate deliveries from ever crediting (AGENTS.md §1.6).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability experimental
// @since     2026-08-18
package httphandler

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/kts"
	"github.com/kentangtech/bot-order/internal/repository/redis"
	topupsvc "github.com/kentangtech/bot-order/internal/service/topup"
)

// paymentWebhookDedupTTL bounds the processed-webhook marker (013 §2.3: one id
// per terminal outcome — a 7-day window covers the gateway's retry horizon).
const paymentWebhookDedupTTL = 7 * 24 * time.Hour

// TopupSettler applies a pg.charge terminal state (topupsvc.Service implements).
type TopupSettler interface {
	ApplySettlement(ctx context.Context, orderID, status string) (*topupsvc.SettlementResult, error)
}

// paymentsWebhook handles the pg.charge webhook from the gateway (013 §2):
//  1. verify X-Webhook-Signature over the RAW body (constant-time)
//  2. branch X-Webhook-Event == pg.charge
//  3. dedup X-Webhook-Id (Redis SETNX)
//  4. settle — 2xx only after the settlement is durable
func (o Options) paymentsWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		o.Logger.Error("payment webhook: reading body", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}

	// 1. Signature — HMAC-SHA256 over the raw body with the merchant secret
	// (the same secretKey used for outbound S2S signing, 013 §2.2).
	expected := kts.WebhookSignature(o.PaymentWebhookSecret, body)
	got := r.Header.Get("X-Webhook-Signature")
	if subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
		o.Logger.Warn("payment webhook rejected: invalid signature", "remote", r.RemoteAddr)
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "invalid signature"})
		return
	}

	// 2. Event branch — only pg.charge is accepted here.
	if r.Header.Get("X-Webhook-Event") != kts.EventPGCharge {
		o.Logger.Warn("payment webhook rejected: unexpected event", "event", r.Header.Get("X-Webhook-Event"))
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported event"})
		return
	}

	var n kts.Notification
	if err := json.Unmarshal(body, &n); err != nil || n.OrderID == "" || n.Status == "" {
		o.Logger.Warn("payment webhook: malformed payload", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "malformed payload"})
		return
	}

	// 3. Dedup — a re-delivered X-Webhook-Id is acknowledged without work.
	if o.Dedup != nil {
		webhookID := r.Header.Get("X-Webhook-Id")
		created, err := o.Dedup.SetNX(r.Context(), redis.PaymentWebhookKey(webhookID), "1", paymentWebhookDedupTTL)
		if err != nil {
			o.Logger.Error("payment webhook dedup failed", "order_id", n.OrderID, "error", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "dedup unavailable"})
			return
		}
		if !created {
			o.Logger.Debug("duplicate payment webhook dropped", "order_id", n.OrderID, "webhook_id", webhookID)
			writeJSON(w, http.StatusOK, map[string]string{"ok": "true", "dedup": "true"})
			return
		}
	}

	// 4. Settle — only a durable credit is acknowledged; any failure returns
	// 5xx so the gateway retries (013 §2: non-2xx → retry → dead-letter).
	if o.Topup == nil {
		o.Logger.Error("payment webhook: topup settler not configured")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "settler not ready"})
		return
	}
	if _, err := o.Topup.ApplySettlement(r.Context(), n.OrderID, n.Status); err != nil {
		o.Logger.Error("payment webhook settlement failed", "order_id", n.OrderID, "status", n.Status, "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "settlement failed"})
		return
	}
	o.Logger.Info("payment webhook settled", "order_id", n.OrderID, "status", n.Status)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}
