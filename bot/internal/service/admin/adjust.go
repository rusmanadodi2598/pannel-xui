// Package adminsvc also hosts the balance-adjustment operation (FR-11, v1.39).
//
// @file      internal/service/admin/adjust.go
// @for       AdjustBalance: admin credit/debit manual — atomic, ledgered, traceable.
// @uses      context, fmt, internal/domain, gorm.io/gorm
// @reason    Manual corrections (compensation, topup manual, refund manual) must
// go through the SAME atomic Credit/Debit + immutable ledger path as orders so
// every mutation is recorded (split for §1.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-12
package adminsvc

import (
	"context"
	"errors"
	"fmt"

	"github.com/kentangtech/bot-order/internal/domain"
	"gorm.io/gorm"
)

// ErrUserNotFound wraps gorm.ErrRecordNotFound so the handler can distinguish an
// unregistered target from a DB failure.
var ErrUserNotFound = errors.New("user not found")

// AdjustBalance credits (credit=true) or debits (credit=false) a user's balance
// atomically with a ledger row (FR-11 admin correction). tgID is the Telegram
// id the admin typed; the row's primary key is resolved first. The ledger ref
// is ADJ-<random> so corrections are distinguishable from order IDs. adminID is
// recorded in the audit trail (FR-11 AC).
func (s *Service) AdjustBalance(ctx context.Context, adminID, tgID int64, amount domain.Money, credit bool) (domain.Money, error) {
	if amount <= 0 {
		return 0, fmt.Errorf("adjust amount must be positive: %d", amount.Rupiah())
	}
	u, err := s.users.Get(ctx, tgID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, ErrUserNotFound
		}
		return 0, fmt.Errorf("looking up user %d: %w", tgID, err)
	}
	ref := "ADJ-" + domain.NewSecret(4)
	var newBalance domain.Money
	if credit {
		newBalance, err = s.users.Credit(ctx, u.ID, amount, ref)
	} else {
		newBalance, err = s.users.Debit(ctx, u.ID, amount, ref)
	}
	if err != nil {
		return 0, err
	}
	kind := "debit"
	if credit {
		kind = "kredit"
	}
	s.auditRecord(ctx, adminID, AuditBalanceAdjust, fmt.Sprintf("%d", tgID), fmt.Sprintf("%s %s ref=%s", kind, amount.FormatIDR(), ref))
	return newBalance, nil
}
