// Package telegram also hosts the shared brand banner (v1.43).
//
// @file      internal/service/telegram/menu_brand.go
// @for       Single source of the KENTANG TECH brand banner for notifications.
// @uses      (none — pure constant + string)
// @reason    User request v1.43: brand the notification templates like the
// legacy reference (client-vpn notification_service.py); the brand line is
// the ONE icon exception to the emoji-free body copy policy (documented).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-12
package telegram

// BrandName is the notification brand (v1.43 product decision): KENTANG TECH —
// explicitly NOT "KENTANG TECH STORE" (the legacy reference header).
const BrandName = "KENTANG TECH"

// BrandHeader is the brand banner placed at the top of every notification
// template: one icon (🏪 — the single icon exception to the emoji-free body
// policy, parity with the legacy reference) + brand name + separator.
func BrandHeader() string {
	return "🏪 " + BrandName + "\n━━━━━━━━━━━━━━"
}
