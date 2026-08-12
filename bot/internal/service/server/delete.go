// Package serversvc also hosts client deletion (FR-08 AC-4).
//
// @file      internal/service/server/delete.go
// @for       DeleteClient: remove a client from the panel (delClient endpoint).
// @uses      context, fmt, internal/repository/xui
// @reason    The account-delete flow must remove the client from the panel
// (delClient) before the DB row — parity reference delClient (FR-08 AC-4).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package serversvc

import (
	"context"
	"fmt"
)

// DeleteClient removes a client from the panel (FR-08 AC-4). The caller
// already resolved serverID + inboundID + panel client id (UUID/password) from
// the owned DB row; the panel is the source of truth, so a panel failure
// aborts the whole deletion (the DB row must not vanish while the panel
// client still exists).
func (s *Service) DeleteClient(ctx context.Context, serverID int64, inboundID int, clientID string) error {
	client, err := s.panelFactory(ctx, serverID)
	if err != nil {
		return fmt.Errorf("panel client for server %d: %w", serverID, err)
	}
	if err := client.DeleteClient(ctx, inboundID, clientID); err != nil {
		return fmt.Errorf("panel delClient: %w", err)
	}
	return nil
}
