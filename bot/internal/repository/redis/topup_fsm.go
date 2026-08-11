// Package redis also hosts the topup custom-input state machine store.
//
// @file      internal/repository/redis/topup_fsm.go
// @for       FR-06 custom nominal input: one pending marker per user (FSM).
// @uses      context, time, internal/repository/redis (ops.go primitives)
// @reason    The custom-nominal step waits for a free-text reply; the marker
// tells the dispatcher which user is mid-input (PRD §9.2 keys, M5).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability experimental
// @since     2026-08-11
package redis

import (
	"context"
	"time"
)

// TopupFSM marks a user as mid custom-nominal input. The marker auto-expires
// (TTL) so an abandoned flow never wedges the user's chat.
type TopupFSM struct {
	client *Client
	ttl    time.Duration
}

// NewTopupFSM builds the store; ttl bounds how long a pending input stays valid.
func NewTopupFSM(client *Client, ttl time.Duration) *TopupFSM {
	return &TopupFSM{client: client, ttl: ttl}
}

// SetPending marks the user as awaiting a custom nominal (idempotent).
func (f *TopupFSM) SetPending(ctx context.Context, userID int64) error {
	return f.client.SetString(ctx, TopupFSMKey(userID), "1", f.ttl)
}

// Pending reports whether the user is mid custom-nominal input.
func (f *TopupFSM) Pending(ctx context.Context, userID int64) (bool, error) {
	return f.client.Exists(ctx, TopupFSMKey(userID))
}

// Clear removes the pending marker (on submit, cancel or back).
func (f *TopupFSM) Clear(ctx context.Context, userID int64) error {
	return f.client.Delete(ctx, TopupFSMKey(userID))
}
