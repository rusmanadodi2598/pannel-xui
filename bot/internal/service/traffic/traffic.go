// Package trafficsvc syncs X-UI panel traffic into vpn_clients (PRD §16.2, M6).
//
// @file      internal/service/traffic/traffic.go
// @for       Sweep active clients grouped per panel: fetch inbounds + online users, bulk-update usage.
// @uses      context, fmt, log/slog, time, internal/repository/postgres, internal/repository/xui
// @reason    Traffic/usage shown in "Akun Saya" must come from the panels; the
// worker keeps it fresh without N+1 calls (§1.7) and never lets one dead panel
// fail the whole sweep (PRD §16.2).
//
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package trafficsvc

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
	"github.com/kentangtech/bot-order/internal/repository/xui"
)

// Store is the persistence seam (postgres.ClientRepo implements it).
type Store interface {
	ListTrafficCandidates(ctx context.Context, limit int) ([]postgres.TrafficCandidate, error)
	SyncTrafficBatch(ctx context.Context, syncedAt time.Time, updates []postgres.TrafficUpdate) error
}

// Panel is the X-UI client surface used by the sweep (xui.Client implements it).
type Panel interface {
	GetInbounds(ctx context.Context) ([]xui.Inbound, error)
	GetOnlineClients(ctx context.Context) ([]xui.OnlineUser, error)
}

// PanelFactory builds an authenticated panel client for one server id.
type PanelFactory func(ctx context.Context, serverID int64) (Panel, error)

// Service runs one traffic-sync sweep: candidates → grouped per server →
// one GetInbounds + one GetOnlineClients per server → one batch write.
type Service struct {
	store   Store
	panels  PanelFactory
	batch   int
	timeout time.Duration // per-server call budget
	logger  *slog.Logger
}

// New builds the sync service. panels is typically serversvc.Service.PanelClient.
func New(store Store, panels PanelFactory, batch int, timeout time.Duration, logger *slog.Logger) *Service {
	return &Service{store: store, panels: panels, batch: batch, timeout: timeout, logger: logger}
}

// RunOnce performs one sweep. A failing server is logged and skipped so the
// rest of the fleet still syncs (PRD §16.2: log + metric, no restart).
func (s *Service) RunOnce(ctx context.Context) error {
	cands, err := s.store.ListTrafficCandidates(ctx, s.batch)
	if err != nil {
		return err
	}
	if len(cands) == 0 {
		return nil
	}

	// Group candidates by panel to fetch each panel exactly once.
	byServer := map[int64][]postgres.TrafficCandidate{}
	var serverOrder []int64
	for _, c := range cands {
		if _, ok := byServer[c.ServerID]; !ok {
			serverOrder = append(serverOrder, c.ServerID)
		}
		byServer[c.ServerID] = append(byServer[c.ServerID], c)
	}

	var failed int
	for _, serverID := range serverOrder {
		if err := s.syncServer(ctx, serverID, byServer[serverID]); err != nil {
			failed++
			s.logger.Error("traffic sync server failed",
				"server_id", serverID, "error", err)
		}
	}
	if failed > 0 {
		return fmt.Errorf("traffic sync: %d/%d panel(s) failed", failed, len(serverOrder))
	}
	return nil
}

// syncServer fetches one panel's traffic + online set and writes the batch.
func (s *Service) syncServer(ctx context.Context, serverID int64, cands []postgres.TrafficCandidate) error {
	sctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	panel, err := s.panels(sctx, serverID)
	if err != nil {
		return fmt.Errorf("panel client: %w", err)
	}
	inbounds, err := panel.GetInbounds(sctx)
	if err != nil {
		return fmt.Errorf("listing inbounds: %w", err)
	}

	// clientStats on inbounds carry per-client traffic (same source the panel's
	// own sub service uses) — one call instead of one per email (anti N+1).
	traffic := map[string]xui.ClientTraffic{}
	for _, in := range inbounds {
		for _, ct := range in.ClientStats {
			traffic[ct.Email] = ct
		}
	}

	online := map[string]bool{}
	onlines, err := panel.GetOnlineClients(sctx)
	if err != nil {
		// last_online is best-effort: sync usage even when the online check fails.
		s.logger.Warn("traffic sync online check failed",
			"server_id", serverID, "error", err)
	} else {
		for _, u := range onlines {
			online[u.Email] = true
		}
	}

	now := time.Now()
	updates := make([]postgres.TrafficUpdate, 0, len(cands))
	for _, c := range cands {
		ct, ok := traffic[c.Email]
		if !ok {
			// Client removed from the panel (or never provisioned) — skip it.
			s.logger.Warn("traffic sync client missing on panel",
				"server_id", serverID, "email", c.Email)
			continue
		}
		u := postgres.TrafficUpdate{ClientID: c.ClientID, Up: ct.Up, Down: ct.Down}
		if online[c.Email] {
			u.LastOnline = &now
		}
		updates = append(updates, u)
	}
	if len(updates) == 0 {
		return nil
	}
	return s.store.SyncTrafficBatch(ctx, now, updates)
}
