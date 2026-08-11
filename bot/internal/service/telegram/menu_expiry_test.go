// Package telegram test covers the FR-09 expiry reminder copy.
//
// @file      internal/service/telegram/menu_expiry_test.go
// @for       Given a day/server/email/date, then message is clear & emoji-free.
// @uses      testing, strings
// @reason    Guards the user-facing reminder wording (UI copy policy).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"strings"
	"testing"
)

func TestExpiryNotifyText_GivenDay3_ThenMentionsDayAndDate(t *testing.T) {
	got := ExpiryNotifyText(3, "ID-01", "a@vpn.kt", "14 Agu 2026")

	for _, want := range []string{"3 hari", "a@vpn.kt", "ID-01", "14 Agu 2026", "Perpanjang"} {
		if !strings.Contains(got, want) {
			t.Errorf("text missing %q:\n%s", want, got)
		}
	}
}

func TestExpiryNotifyText_GivenDay1_ThenMentions1Day(t *testing.T) {
	got := ExpiryNotifyText(1, "SG-01", "b@vpn.kt", "12 Agu 2026")
	if !strings.Contains(got, "dalam 1 hari") {
		t.Errorf("day-1 wording missing:\n%s", got)
	}
}
