// Package crypto provides the AES-256-GCM secret box for panel credentials.
//
// @file      internal/crypto/crypto.go
// @for       Encrypt/decrypt panel server credentials at rest (PRD §15.1, FR-10).
// @uses      crypto/aes, crypto/cipher, crypto/rand, encoding/base64, fmt, errors
// @reason    Replaces Python Fernet with Go stdlib AES-256-GCM (PRD §3.2), no extra deps.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     util
// @stability stable
// @since     2026-08-11
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// nonceSize is the GCM standard nonce length (12 bytes).
const nonceSize = 12

// SecretBox seals and opens values with AES-256-GCM using a 32-byte key.
type SecretBox struct {
	aead cipher.AEAD
}

// NewSecretBox builds a SecretBox from a 32-byte key (AES-256).
func NewSecretBox(key []byte) (*SecretBox, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("crypto: key must be 32 bytes for AES-256-GCM, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: creating cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: creating GCM: %w", err)
	}
	return &SecretBox{aead: aead}, nil
}

// Encrypt seals plaintext with a fresh random nonce and returns base64
// (nonce || ciphertext) — safe for TEXT columns like vpn_servers.password_enc.
func (s *SecretBox) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: generating nonce: %w", err)
	}
	sealed := s.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt opens a value produced by Encrypt. It fails on any tampering or
// wrong key (GCM authentication covers both ciphertext and nonce).
func (s *SecretBox) Decrypt(encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("crypto: decoding ciphertext: %w", err)
	}
	if len(raw) < nonceSize {
		return "", errors.New("crypto: ciphertext too short")
	}
	nonce, sealed := raw[:nonceSize], raw[nonceSize:]
	plain, err := s.aead.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("crypto: decrypting (wrong key or tampered): %w", err)
	}
	return string(plain), nil
}
