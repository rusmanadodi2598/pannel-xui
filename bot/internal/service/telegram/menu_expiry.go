// Package telegram also hosts the expiry-reminder copy (FR-09, M6).
//
// @file      internal/service/telegram/menu_expiry.go
// @for       FR-09 proactive expiry reminders (H-7/H-3/H-1) — message text only.
// @uses      fmt
// @reason    Pure presentation per UI copy policy (emoji-free body); the worker
//
//	service builds the recipient list, this owns the wording.
//
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import "fmt"

// ExpiryNotifyText renders the H-N reminder for a single account. expiryDate is
// pre-formatted by the caller in the configured TIME_LOCATION (FR-09 AC).
func ExpiryNotifyText(day int, serverName, email, expiryDate string) string {
	return fmt.Sprintf("Pengingat Kadaluarsa\n━━━━━━━━━━━━━━\n\n"+
		"Akun VPN kamu (%s) di server %s akan kadaluarsa dalam %d hari.\n"+
		"Tanggal kadaluarsa: %s\n\n"+
		"Perpanjang sebelum habis agar koneksi tidak terputus.\n"+
		"Menu: Beli VPN > Perpanjang > pilih akun.",
		email, serverName, day, expiryDate)
}
