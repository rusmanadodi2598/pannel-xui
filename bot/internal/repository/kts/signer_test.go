// Package kts tests the S2S signer (contract 001 §2.3).
//
// @file      internal/repository/kts/signer_test.go
// @for       Canonical payload shape + HMAC-SHA256 signature + webhook verify.
// @uses      testing, crypto/hmac, crypto/sha256, encoding/hex, strings
// @reason    A byte-exact signing bug would make every outbound call 401; these
// tests pin the canonical construction and the header format (AGENTS.md §2.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability experimental
// @since     2026-08-18
package kts

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestCanonical_ThenExactShape(t *testing.T) {
	body := []byte(`{}`)
	got := canonical("k1", "123", "n1", "POST", "/api/v1/pg/charges", body)
	sum := sha256.Sum256(body)
	want := "v1\nk1\n123\nn1\nPOST\n/api/v1/pg/charges\n" + hex.EncodeToString(sum[:])
	if got != want {
		t.Errorf("canonical = %q, want %q", got, want)
	}
	if strings.HasSuffix(got, "\n") {
		t.Error("canonical must not have a trailing newline")
	}
}

func TestSign_GivenSecret_ThenHMACSHA256Prefixed(t *testing.T) {
	canon := "v1\nk1\n123\nn1\nPOST\n/p\nabc"
	got := sign("secret", canon)
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write([]byte(canon))
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if got != want {
		t.Errorf("sign = %q, want %q", got, want)
	}
}

func TestWebhookSignature_GivenRawBody_ThenMatchesManualHMAC(t *testing.T) {
	body := []byte(`{"eventType":"pg.charge","status":"succeeded"}`)
	got := WebhookSignature("secret", body)
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if got != want {
		t.Errorf("webhook signature = %q, want %q", got, want)
	}
}
