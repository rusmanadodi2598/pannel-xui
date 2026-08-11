// Package domain holds entities and value objects (DDD, AGENTS.md §2.2).
//
// @file      internal/domain/money.go
// @for       Money value object in IDR (int64 rupiah, anti-float) + SQL scan/valuers.
// @uses      strconv, fmt, strings, database/sql/driver
// @reason    Balance/prices are NUMERIC(15,2) in DB; Go-side we keep whole rupiah as int64
//
//	to avoid float drift (AGENTS.md §2.2 value objects, PRD §11 domain/money.go).
//	Scanner/Valuer bridge NUMERIC ⇄ int64 for GORM.
//
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     domain
// @stability stable
// @since     2026-08-11
package domain

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"
)

// Money is an immutable amount of Indonesian Rupiah (whole rupiah).
type Money int64

// NewMoney validates a non-negative amount and wraps it.
func NewMoney(rupiah int64) (Money, error) {
	if rupiah < 0 {
		return 0, fmt.Errorf("money cannot be negative: %d", rupiah)
	}
	return Money(rupiah), nil
}

// Add returns the sum (never negative for valid inputs).
func (m Money) Add(other Money) Money { return m + other }

// Sub returns the difference; caller must guard with LessThan to avoid negatives.
func (m Money) Sub(other Money) Money { return m - other }

// LessThan reports whether m is strictly smaller than other.
func (m Money) LessThan(other Money) bool { return m < other }

// IsZero reports whether the amount is zero.
func (m Money) IsZero() bool { return m == 0 }

// Rupiah returns the raw whole-rupiah value for DB storage.
func (m Money) Rupiah() int64 { return int64(m) }

// Scan implements sql.Scanner so NUMERIC(15,2) values ("7000.00") map to
// whole rupiah. Whole amounts up to 2^53 are exact in float64 — safe here.
func (m *Money) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*m = 0
	case int64:
		*m = Money(v)
	case float64:
		*m = Money(v)
	case []byte:
		return m.scanString(string(v))
	case string:
		return m.scanString(v)
	default:
		return fmt.Errorf("cannot scan %T into Money", src)
	}
	return nil
}

// scanString parses a numeric string that may carry decimal places.
func (m *Money) scanString(s string) error {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("parsing money %q: %w", s, err)
	}
	*m = Money(f)
	return nil
}

// Value implements driver.Valuer: whole rupiah written into NUMERIC columns.
func (m Money) Value() (driver.Value, error) {
	return int64(m), nil
}

// FormatIDR renders "Rp 25.000" with thousand separators (Asia/Jakarta locale style).
func (m Money) FormatIDR() string {
	digits := strconv.FormatInt(int64(m), 10)
	// Insert '.' every 3 digits from the right.
	var b strings.Builder
	for i, c := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	return "Rp " + b.String()
}
