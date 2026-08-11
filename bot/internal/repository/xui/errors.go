// Package xui also hosts the structured error taxonomy.
//
// @file      internal/repository/xui/errors.go
// @for       XUIError with machine-parseable codes (PRD §15.6).
// @uses      fmt
// @reason    Lets services classify panel failures (auth, network, duplicate email, etc.).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package xui

import (
	"fmt"
	"strings"
)

// ErrorCode categorizes panel failures (PRD §15.6).
type ErrorCode string

const (
	CodeAuth           ErrorCode = "AUTH"
	CodeNetwork        ErrorCode = "NETWORK"
	CodeDuplicateEmail ErrorCode = "DUPLICATE_EMAIL"
	CodeInboundFull    ErrorCode = "INBOUND_FULL"
	CodeTimeout        ErrorCode = "TIMEOUT"
	CodeNotFound       ErrorCode = "NOT_FOUND"
	CodeUnknown        ErrorCode = "UNKNOWN"
)

// XUIError is a structured panel API error (English message per AGENTS.md §1.3).
type XUIError struct {
	Code       ErrorCode
	Message    string
	StatusCode int
}

func (e *XUIError) Error() string {
	return fmt.Sprintf("xui %s: %s (http %d)", e.Code, e.Message, e.StatusCode)
}

// classify maps a panel message to an error code (English panel messages).
func classify(msg string) ErrorCode {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "duplicate email"):
		return CodeDuplicateEmail
	// "full" alone matches "successfully"; require the inbound-full phrasing.
	case strings.Contains(lower, "inbound is full") || strings.Contains(lower, "capacity"):
		return CodeInboundFull
	case strings.Contains(lower, "not found") || strings.Contains(lower, "doesn't exist") || strings.Contains(lower, "no such"):
		return CodeNotFound
	default:
		return CodeUnknown
	}
}
