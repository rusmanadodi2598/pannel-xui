// Package telegramhandler also hosts tiny callback-data parsers.
//
// @file      internal/handler/telegram/parse.go
// @for       atoi / parseID64 + shop callback payload parsers.
// @uses      strconv, strings
// @reason    Splits parsing out of the flow files (§1.1 line limit).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import (
	"strconv"
	"strings"
)

// atoi parses a non-negative integer from a callback payload.
func atoi(raw string) (int, error) { return strconv.Atoi(raw) }

// parseID64 parses a signed 64-bit id (client ids are positive).
func parseID64(raw string) (int64, error) { return strconv.ParseInt(raw, 10, 64) }

// parseID returns the parsed id or 0 (lenient — callers validate ranges).
func parseID(raw string) int64 {
	id, _ := parseID64(raw)
	return id
}

// parsePositiveID parses a positive 64-bit id from a callback payload
// (client ids, order ids stay > 0).
func parsePositiveID(raw string) (int64, bool) {
	id, err := parseID64(strings.TrimSpace(raw))
	return id, err == nil && id > 0
}

// parsePage validates a 1-based page number from a paged-list callback
// payload (history:page:N / account:page:N, FR-14/FR-08).
func parsePage(raw string) (int, bool) {
	page, err := atoi(strings.TrimSpace(raw))
	return page, err == nil && page > 0
}

// parsePlanData splits "<CODE>:<DAYS>" (legacy format used by admin flows).
func parsePlanData(raw string) (country string, days int, ok bool) {
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return "", 0, false
	}
	days, err := atoi(parts[1])
	if err != nil || days <= 0 {
		return "", 0, false
	}
	return strings.ToUpper(parts[0]), days, true
}

// parseBuyInbound splits "<SERVERID>:<INBOUNDID>:<CODE>" from the protocol
// picker (FR-03 step 2).
func parseBuyInbound(raw string) (serverID, inboundID int, country string, ok bool) {
	parts := strings.Split(raw, ":")
	if len(parts) != 3 {
		return 0, 0, "", false
	}
	serverID, err := atoi(parts[0])
	if err != nil {
		return 0, 0, "", false
	}
	inboundID, err = atoi(parts[1])
	if err != nil || serverID <= 0 || inboundID <= 0 {
		return 0, 0, "", false
	}
	return serverID, inboundID, strings.ToUpper(parts[2]), true
}

// parseBuySelection splits "<SERVERID>:<INBOUNDID>:<CODE>:<DAYS>" from the
// plan & confirm steps (FR-03). The protocol is re-derived from the live
// inbound on the plan step, so the callback itself never carries it.
func parseBuySelection(raw string) (country string, days, serverID, inboundID int, protocol string, ok bool) {
	parts := strings.Split(raw, ":")
	if len(parts) != 4 {
		return "", 0, 0, 0, "", false
	}
	var err error
	serverID, err = atoi(parts[0])
	if err != nil {
		return "", 0, 0, 0, "", false
	}
	inboundID, err = atoi(parts[1])
	if err != nil || serverID <= 0 || inboundID <= 0 {
		return "", 0, 0, 0, "", false
	}
	days, err = atoi(parts[3])
	if err != nil || days <= 0 {
		return "", 0, 0, 0, "", false
	}
	return strings.ToUpper(parts[2]), days, serverID, inboundID, "", true
}

// parseRenewData splits "<CLIENTID>:<CODE>:<DAYS>".
func parseRenewData(raw string) (clientID int64, country string, days int, ok bool) {
	parts := strings.Split(raw, ":")
	if len(parts) != 3 {
		return 0, "", 0, false
	}
	clientID, err := parseID64(parts[0])
	if err != nil {
		return 0, "", 0, false
	}
	days, err = atoi(parts[2])
	if err != nil || days <= 0 {
		return 0, "", 0, false
	}
	return clientID, strings.ToUpper(parts[1]), days, true
}
