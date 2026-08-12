// Package trafficsvc also hosts the on-demand per-client refresh (FR-08 AC-3).
//
// @file      internal/service/traffic/refresh.go
// @for       RefreshClient: one client's live usage fetched on demand.
// @uses      context, fmt, time, internal/repository/postgres
// @reason    The account traffic page needs live numbers without waiting for
// the sweep worker; the panel's getClientTraffics/:email matches the traffic
// table by email directly, so it works for every protocol (AGENTS.md §1.6).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package trafficsvc

import (
	"context"
	"fmt"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// RefreshClient syncs one client's usage from its panel on demand (FR-08
// AC-3). Uses getClientTraffics/:email — verified protocol-agnostic from the
// panel source (WHERE email = ?). A missing client (zero-value traffic) is an
// error: the caller keeps rendering the last known values. last_online is not
// touched — only the sweep sets it from the online set.
func (s *Service) RefreshClient(ctx context.Context, clientID, serverID int64, email string) error {
	sctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	panel, err := s.panels(sctx, serverID)
	if err != nil {
		return fmt.Errorf("panel client: %w", err)
	}
	ct, err := panel.GetClientTrafficByEmail(sctx, email)
	if err != nil {
		return fmt.Errorf("fetching client traffic: %w", err)
	}
	if ct.Email == "" {
		return fmt.Errorf("client %q not found on server %d", email, serverID)
	}
	return s.store.SyncTrafficBatch(ctx, time.Now(), []postgres.TrafficUpdate{{
		ClientID: clientID,
		Up:       ct.Up,
		Down:     ct.Down,
	}})
}
