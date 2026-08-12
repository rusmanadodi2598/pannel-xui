// Package telegramhandler also hosts the trial callback parsing (FR-07, M7 fix).
//
// @file      internal/handler/telegram/trial_parse.go
// @for       Parse trial callback payloads (server, server+inbound picker).
// @uses      strings
// @reason    Keeps trial.go under the 250-line limit (§1.1); parsing is pure
// and unit-testable without the dispatcher.
//
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package telegramhandler

import "strings"

// parseTrialInbound splits "<SERVERID>:<INBOUNDID>" (trial picker + confirm).
func parseTrialInbound(raw string) (serverID, inboundID int64, ok bool) {
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return 0, 0, false
	}
	serverID, err := parseID64(parts[0])
	if err != nil {
		return 0, 0, false
	}
	inboundID, err = parseID64(parts[1])
	if err != nil || serverID <= 0 || inboundID <= 0 {
		return 0, 0, false
	}
	return serverID, inboundID, true
}
