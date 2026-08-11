// Package domain also hosts random identifier generators.
//
// @file      internal/domain/random.go
// @for       Crypto-random order IDs (KTS-XXXXXXXX-VPN) and client secrets.
// @uses      crypto/rand, fmt, strings
// @reason    Order IDs must be unpredictable & unique (FR-03 AC); client secrets
// need 128+ bits of entropy (PRD §11).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     domain
// @stability stable
// @since     2026-08-11
package domain

import (
	"crypto/rand"
	"fmt"
	"strings"
)

// orderAlphabet avoids ambiguous characters (0/O, 1/I/L) in public order IDs.
const orderAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// NewOrderID builds a public order id: KTS-XXXXXXXX-VPN (FR-03 AC).
func NewOrderID() string {
	return "KTS-" + randomFrom(orderAlphabet, 8) + "-VPN"
}

// NewUUID generates a random UUID v4 (for VLESS/VMess client ids).
func NewUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err)) // /dev/urandom unavailable
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// NewSecret returns n bytes of crypto-random hex (passwords, tokens).
func NewSecret(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return fmt.Sprintf("%x", b)
}

func randomFrom(alphabet string, n int) string {
	var sb strings.Builder
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	for _, b := range buf {
		sb.WriteByte(alphabet[int(b)%len(alphabet)])
	}
	return sb.String()
}
