// Package kts also hosts the S2S HMAC signer (contract 001 §2.3).
//
// @file      internal/repository/kts/signer.go
// @for       Canonical signing payload + HMAC-SHA256 X-Signature header.
// @uses      crypto/hmac, crypto/sha256, encoding/hex, fmt
// @reason    Every outbound call to /api/v1/pg/* must be signed; keeping the
// canonical construction here guarantees byte-exact signing in one place.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability experimental
// @since     2026-08-18
package kts

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// canonical joins the S2S signing payload with LF and no trailing newline
// (001 §2.3): v1, apiKey, timestamp, nonce, method, raw path, body sha256.
// The gateway does not decode or normalize the path before verification, so
// callers must pass the exact path sent on the wire.
func canonical(apiKey, timestamp, nonce, method, path string, body []byte) string {
	bodyHash := sha256.Sum256(body)
	return fmt.Sprintf("v1\n%s\n%s\n%s\n%s\n%s\n%s",
		apiKey, timestamp, nonce, method, path, hex.EncodeToString(bodyHash[:]))
}

// sign returns the X-Signature header value: "sha256=" + hex(HMAC-SHA256)
// of the canonical payload with the merchant secret.
func sign(secret, canonical string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// WebhookSignature returns the X-Webhook-Signature a merchant must verify on
// inbound pg.charge webhooks (013 §2.2): "sha256=" + hex(HMAC-SHA256) over the
// RAW body bytes with the same merchant secret used for outbound S2S signing.
func WebhookSignature(secret string, rawBody []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(rawBody)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
