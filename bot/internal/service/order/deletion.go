// Package ordersvc also hosts the account-deletion history record (FR-08 AC-4).
//
// @file      internal/service/order/deletion.go
// @for       RecordDeletion: persist an account deletion as a completed order row.
// @uses      context, internal/domain
// @reason    FR-08 AC-4 requires the action to be recorded in log & riwayat —
// a zero-amount 'deletion' order keeps ONE source of truth (FR-14) without a
// separate audit table. Split from order.go for the §1.1 line limit.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-12
package ordersvc

import (
	"context"

	"github.com/kentangtech/bot-order/internal/domain"
)

// RecordDeletion persists a completed 'deletion' order so the FR-08 AC-4
// action shows up in the user's Riwayat (FR-14). No balance movement — the
// row carries the account email + protocol only. The caller decides what to
// surface on failure: the deletion itself already succeeded on the panel and
// in the DB, so a record failure is logged, never blocks the user.
func (s *Service) RecordDeletion(ctx context.Context, userID, serverID int64, protocol, email string) error {
	order := domain.NewDeletionRecord(s.newID(), userID, serverID, protocol, email)
	if err := s.orders.Create(ctx, toOrderRow(order)); err != nil {
		return err
	}
	return nil
}
