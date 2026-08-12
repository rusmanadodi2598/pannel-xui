// Package telegramhandler hosts the admin add-server draft state (FR-11, v1.40).
//
// @file      internal/handler/telegram/admin_server_draft.go
// @for       serverDraft value type: validation, completion and the pipe-
// separated FSM encoding shared by the add-server flow.
// @uses      fmt, strconv, strings
// @reason    Split from the add-server handlers for the §1.1 line limit; the
// draft is pure state (no I/O) so it stays trivially testable.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability experimental
// @since     2026-08-12
package telegramhandler

import (
	"fmt"
	"strconv"
	"strings"
)

// serverDraft is the in-progress add-server form.
type serverDraft struct {
	name     string
	host     string
	port     int
	username string
	password string
	country  string
	flag     string
}

// fillNext validates and stores the typed value at the current step. It returns
// the NEXT step key ("" when the form is complete) and false when the value was
// rejected (re-prompt the same step).
func (d *serverDraft) fillNext(value string) (string, bool) {
	switch {
	case d.name == "":
		d.name = value
		return "host", true
	case d.host == "":
		d.host = value
		return "port", true
	case d.port == 0:
		port, err := strconv.Atoi(value)
		if err != nil || port <= 0 || port > 65535 {
			return "", false
		}
		d.port = port
		return "username", true
	case d.username == "":
		d.username = value
		return "password", true
	case d.password == "":
		d.password = value
		return "country", true
	default:
		// country — optionally "CODE,FLAG" so the flag is preserved.
		parts := strings.SplitN(value, ",", 2)
		d.country = strings.ToUpper(strings.TrimSpace(parts[0]))
		if len(parts) == 2 {
			d.flag = strings.TrimSpace(parts[1])
		}
		if d.country == "" {
			return "", false
		}
		return "", true
	}
}

// complete reports whether all required fields are filled.
func (d *serverDraft) complete() bool {
	return d.name != "" && d.host != "" && d.port > 0 && d.username != "" &&
		d.password != "" && d.country != ""
}

// encode serializes the draft into the FSM state (pipe-separated).
func (d *serverDraft) encode() string {
	return fmt.Sprintf("%s|%s|%d|%s|%s|%s|%s",
		d.name, d.host, d.port, d.username, d.password, d.country, d.flag)
}

// decode restores the draft from the FSM state (empty raw = fresh form).
func (d *serverDraft) decode(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, "|")
	if len(parts) < 6 {
		return fmt.Errorf("malformed server draft")
	}
	d.name = parts[0]
	d.host = parts[1]
	port, err := strconv.Atoi(parts[2])
	if err != nil {
		return err
	}
	d.port = port
	d.username = parts[3]
	d.password = parts[4]
	d.country = parts[5]
	if len(parts) >= 7 {
		d.flag = parts[6]
	}
	return nil
}
