// Package telegramhandler also hosts tiny callback-data parsers.
//
// @file      internal/handler/telegram/parse.go
// @for       atoi / parseID64 for shop callback payloads.
// @uses      strconv
// @reason    Splits numeric parsing out of the flow files (§1.1 line limit).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import "strconv"

// atoi parses a non-negative integer from a callback payload.
func atoi(raw string) (int, error) { return strconv.Atoi(raw) }

// parseID64 parses a signed 64-bit id (client ids are positive).
func parseID64(raw string) (int64, error) { return strconv.ParseInt(raw, 10, 64) }
