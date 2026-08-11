// Package redis also hosts the admin input state machine store.
//
// @file      internal/repository/redis/admin_fsm.go
// @for       FR-11 admin free-text inputs: one pending state per admin (FSM).
// @uses      context, time, internal/repository/redis (ops.go primitives)
// @reason    Price/broadcast/ban/unban steps wait for a typed reply; the marker
//
//	tells the dispatcher which admin is mid-input and what to expect.
//
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability experimental
// @since     2026-08-11
package redis

import (
	"context"
	"time"
)

// AdminFSM marks an admin as mid free-text input with the expected next state
// (e.g. "price:ID:15", "broadcast", "ban", "unban"). The marker auto-expires
// (TTL) so an abandoned flow never wedges the admin's chat.
type AdminFSM struct {
	client *Client
	ttl    time.Duration
}

// NewAdminFSM builds the store; ttl bounds how long a pending input stays valid.
func NewAdminFSM(client *Client, ttl time.Duration) *AdminFSM {
	return &AdminFSM{client: client, ttl: ttl}
}

// Set arms the admin input state (idempotent overwrite).
func (f *AdminFSM) Set(ctx context.Context, userID int64, state string) error {
	return f.client.SetString(ctx, AdminFSMKey(userID), state, f.ttl)
}

// Get returns the armed state; ok is false when no input is pending.
func (f *AdminFSM) Get(ctx context.Context, userID int64) (string, bool, error) {
	return f.client.GetString(ctx, AdminFSMKey(userID))
}

// Clear removes the pending state (on submit, cancel or back).
func (f *AdminFSM) Clear(ctx context.Context, userID int64) error {
	return f.client.Delete(ctx, AdminFSMKey(userID))
}
