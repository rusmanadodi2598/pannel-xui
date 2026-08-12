// Package healthsvc checks panel reachability (PRD §17).
//
// @file      internal/service/health/health.go
// @for       Periodic server health check: GET /xui/API/server/status → health_status.
// @uses      context, fmt, log/slog, time, internal/repository/postgres, internal/repository/xui
// @reason    "Server mati tidak dijual" (PRD §17): the buy menu only lists
// healthy panels, so an unreachable panel must be marked "down" promptly. One
// failing panel never aborts the sweep (same isolation as traffic sync).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-12
package healthsvc

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
	"github.com/kentangtech/bot-order/internal/repository/xui"
)

// Store is the persistence seam (postgres.ServerRepo implements it).
type Store interface {
	ListHealthTargets(ctx context.Context) ([]postgres.HealthTarget, error)
	UpdateHealth(ctx context.Context, serverID int64, status string, checkedAt time.Time) error
}

// Panel is the X-UI health surface (xui.Client implements it).
type Panel interface {
	GetServerStatus(ctx context.Context) (xui.Status, error)
}

// PanelFactory builds an authenticated panel client for one server id.
type PanelFactory func(ctx context.Context, serverID int64) (Panel, error)

// Health statuses written to vpn_servers.health_status.
const (
	StatusOK   = "ok"
	StatusDown = "down"
)

// healthWriteTimeout bounds the health-status DB write. It is deliberately
// separate from the per-server XUI budget: a dead panel can exhaust that
// budget (connect timeout), and the result must STILL be persisted — otherwise
// a slow panel stays 'unknown' and keeps being sold (staging E2E v1.45).
const healthWriteTimeout = 10 * time.Second

// Service checks every active panel's reachability once per sweep.
type Service struct {
	store   Store
	panels  PanelFactory
	timeout time.Duration // per-server call budget
	logger  *slog.Logger
}

// New builds the health-check service. panels is typically
// serversvc.Service.PanelClient.
func New(store Store, panels PanelFactory, timeout time.Duration, logger *slog.Logger) *Service {
	return &Service{store: store, panels: panels, timeout: timeout, logger: logger}
}

// RunOnce pings every active server. Unreachable panels are marked "down"
// (hidden from the buy menu); the rest are marked "ok". One failure never
// aborts the sweep — the error only reports how many panels were down.
func (s *Service) RunOnce(ctx context.Context) error {
	targets, err := s.store.ListHealthTargets(ctx)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return nil
	}
	var failed int
	for _, t := range targets {
		if err := s.check(ctx, t); err != nil {
			failed++
			s.logger.Error("health check failed",
				"server_id", t.ID, "server", t.Name, "error", err)
		}
	}
	if failed > 0 {
		return fmt.Errorf("health check: %d/%d panel(s) down", failed, len(targets))
	}
	return nil
}

// check pings one panel and records its status. A panel whose status call
// fails is marked down so the buy menu stops selling it (PRD §17). The DB
// write runs under its own short timeout off the PARENT context — the
// per-server budget may already be exhausted by a dead panel's connect
// timeout, and the 'down' result must still persist (staging E2E v1.45).
func (s *Service) check(ctx context.Context, t postgres.HealthTarget) error {
	sctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	status := StatusOK
	panel, err := s.panels(sctx, t.ID)
	if err == nil {
		_, err = panel.GetServerStatus(sctx)
	}
	if err != nil {
		status = StatusDown
	}
	wctx, wcancel := context.WithTimeout(ctx, healthWriteTimeout)
	defer wcancel()
	if err := s.store.UpdateHealth(wctx, t.ID, status, time.Now()); err != nil {
		return err // repo error already carries server id
	}
	if status == StatusDown {
		return fmt.Errorf("server %d unreachable: %w", t.ID, err)
	}
	return nil
}
