// Package serversvc also hosts trial provisioning (FR-07).
//
// @file      internal/service/server/trial.go
// @for       FR-07 AC-2: create trial account (expiry in hours).
// @uses      context, time, internal/domain
// @reason    Trial uses the same addClient endpoint with an hour-based expiry
// (addTrialClient does not exist in this fork — PRD §3.2/§15.2); it reuses the
// prepare/commit split in provision.go instead of duplicating spec build.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-11
package serversvc

import (
	"context"
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
)

// CreateTrialClient provisions a short-lived trial account (FR-07 AC-2):
// expiry = now + hours, quota = trafficGB, limit = ipLimit (defaults 1 jam/1 GB/1 IP).
func (s *Service) CreateTrialClient(ctx context.Context, serverID int64, inboundID int, email, protocol string, hours int, trafficGB, ipLimit int64) (domain.PanelClient, error) {
	expiry := time.Now().Add(time.Duration(hours) * time.Hour).UnixMilli()
	p, err := s.prepareClient(ctx, serverID, inboundID, email, protocol, trafficGB, ipLimit, expiry)
	if err != nil {
		return domain.PanelClient{}, err
	}
	if err := s.CommitClient(ctx, serverID, p); err != nil {
		return domain.PanelClient{}, err
	}
	return p.Panel, nil
}
