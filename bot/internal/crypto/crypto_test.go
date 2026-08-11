// Package crypto_test covers the AES-256-GCM secret box (AGENTS.md §2.1).
//
// @file      internal/crypto/crypto_test.go
// @for       Unit tests: encrypt/decrypt roundtrip, wrong key, tamper detection, key length.
// @uses      testing, encoding/base64
// @reason    Guards credential encryption contract (PRD §17) that M2 depends on.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     util
// @stability stable
// @since     2026-08-11
package crypto

import (
	"encoding/base64"
	"strings"
	"testing"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	return key
}

func TestSecretBox_GivenValidKey_ThenEncryptDecryptRoundtrip(t *testing.T) {
	box, err := NewSecretBox(testKey(t))
	if err != nil {
		t.Fatalf("NewSecretBox: %v", err)
	}

	const secret = `{"username":"admin","password":"s3cret-integration-pass"}`
	enc, err := box.Encrypt(secret)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if enc == secret {
		t.Fatal("ciphertext must not equal plaintext")
	}
	// Output must be base64 (TEXT column safe) and unique per call (random nonce).
	if _, err := base64.StdEncoding.DecodeString(enc); err != nil {
		t.Errorf("Encrypt output is not valid base64: %v", err)
	}
	enc2, _ := box.Encrypt(secret)
	if enc == enc2 {
		t.Error("two encryptions of the same value must differ (random nonce)")
	}

	got, err := box.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != secret {
		t.Errorf("roundtrip = %q, want %q", got, secret)
	}
}

func TestSecretBox_GivenWrongKey_ThenDecryptFails(t *testing.T) {
	box, _ := NewSecretBox(testKey(t))
	wrongKey := make([]byte, 32)
	for i := range wrongKey {
		wrongKey[i] = byte(255 - i)
	}
	wrongBox, err := NewSecretBox(wrongKey)
	if err != nil {
		t.Fatalf("NewSecretBox(wrong): %v", err)
	}

	enc, _ := box.Encrypt("admin:secret")
	if _, err := wrongBox.Decrypt(enc); err == nil {
		t.Fatal("decrypt with wrong key must fail")
	}
}

func TestSecretBox_GivenTamperedCiphertext_ThenDecryptFails(t *testing.T) {
	box, _ := NewSecretBox(testKey(t))
	enc, _ := box.Encrypt("admin:secret")

	// Flip one byte in the payload portion (after the nonce).
	raw, _ := base64.StdEncoding.DecodeString(enc)
	raw[len(raw)-1] ^= 0x01
	tampered := base64.StdEncoding.EncodeToString(raw)

	if _, err := box.Decrypt(tampered); err == nil {
		t.Fatal("tampered ciphertext must fail GCM authentication")
	}
}

func TestSecretBox_GivenGarbageInput_ThenDecryptFails(t *testing.T) {
	box, _ := NewSecretBox(testKey(t))
	for _, in := range []string{"", "not-base64!!", base64.StdEncoding.EncodeToString([]byte("tiny"))} {
		if _, err := box.Decrypt(in); err == nil {
			t.Errorf("Decrypt(%q) must fail", in)
		}
	}
}

func TestNewSecretBox_GivenWrongKeyLength_ThenError(t *testing.T) {
	for _, n := range []int{16, 24, 31, 33} {
		if _, err := NewSecretBox(make([]byte, n)); err == nil {
			t.Errorf("NewSecretBox with %d-byte key must fail (AES-256)", n)
		} else if !strings.Contains(err.Error(), "32 bytes") {
			t.Errorf("error should mention 32 bytes, got: %v", err)
		}
	}
}
