// Package telegram also hosts the admin-group order notice (FR-04 AC, v1.41).
//
// @file      internal/service/telegram/menu_order_notice.go
// @for       Renders the completed-order notice sent to NOTIFICATION_GROUP_ID.
// @uses      fmt, internal/domain
// @reason    Closes FR-04 AC "notifikasi ke grup admin": pure presentation,
// takes primitives (no ordersvc import — keeps telegramservice free of a
// service-to-service edge), emoji-free body per the UI copy policy.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-12
package telegram

import (
	"fmt"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
)

// AdminOrderNoticeText renders the FR-04 AC completed-order notice for the
// admin group. Body is text-only except the brand banner (the single icon
// exception, v1.43). newExpiry is the account's new expiry (useful for
// renewals — parity with the user-facing RenewSuccessText).
func AdminOrderNoticeText(orderID string, orderType domain.OrderType, userLabel, planLabel, email string, amount, balanceAfter domain.Money, newExpiry time.Time) string {
	kind := orderTypeLabel(string(orderType))
	return fmt.Sprintf(BrandHeader()+"\n\n%s\n━━━━━━━━━━━━━━\n"+
		"Order: %s\n"+
		"User: %s\n"+
		"Paket: %s\n"+
		"Nominal: %s\n"+
		"Akun: %s\n"+
		"Aktif sampai: %s\n"+
		"Sisa saldo: %s\n━━━━━━━━━━━━━━",
		kind, orderID, userLabel, planLabel, amount.FormatIDR(), email,
		newExpiry.Format("02 Jan 2006"), balanceAfter.FormatIDR())
}
