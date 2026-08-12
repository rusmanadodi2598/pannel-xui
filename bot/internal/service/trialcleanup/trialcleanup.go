// Package trialcleanupsvc auto-disables expired trial accounts (PRD worker).
//
// @file      internal/service/trialcleanup/trialcleanup.go
// @for       Sweep expired trial clients: disable on the panel, then mark expired in DB.
// @uses      context, fmt, log/slog, time, internal/repository/postgres
// @reason    Trial accounts are 1-hour by policy (FR-07); without a cleanup
// sweep they stay enabled on the panel past their window, wasting quota. The
// worker disables them on the panel first (source of truth) and only then
// marks the DB row — a panel failure never marks a row, so it is retried.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-12
package trialcleanupsvc

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// Store is the persistence seam (postgres.ClientRepo implements it).
type Store interface {
	ListExpiredTrialCandidates(ctx context.Context, limit int) ([]postgres.ExpiredTrialCandidate, error)
	MarkTrialExpired(ctx context.Context, clientID int64) error
}

// Disabler is the panel seam (serversvc.Service implements it). It returns the
// emails that could NOT be disabled so the sweep only marks successful ones.
type Disabler interface {
	DisableClients(ctx context.Context, serverID int64, emails []string) (failed []string, err error)
}

// Service runs one trial-cleanup sweep.
type Service struct {
	store   Store
	disable Disabler
	batch   int
	timeout time.Duration // per-server call budget
	logger  *slog.Logger
}

// New builds the cleanup service. disable is typically serversvc.Service.
func New(store Store, disable Disabler, batch int, timeout time.Duration, logger *slog.Logger) *Service {
	return &Service{store: store, disable: disable, batch: batch, timeout: timeout, logger: logger}
}

// RunOnce disables every expired trial client and marks its DB row expired.
// Candidates are grouped per panel and each panel is fetched once; one failing
// server never fails the sweep (same isolation as traffic sync).
func (s *Service) RunOnce(ctx context.Context) error {
	cands, err := s.store.ListExpiredTrialCandidates(ctx, s.batch)
	if err != nil {
		return err
	}
	if len(cands) == 0 {
		return nil
	}

	byServer := map[int64][]postgres.ExpiredTrialCandidate{}
	var serverOrder []int64
	for _, c := range cands {
		if _, ok := byServer[c.ServerID]; !ok {
			serverOrder = append(serverOrder, c.ServerID)
		}
		byServer[c.ServerID] = append(byServer[c.ServerID], c)
	}

	var failed int
	for _, serverID := range serverOrder {
		if err := s.cleanServer(ctx, serverID, byServer[serverID]); err != nil {
			failed++
			s.logger.Error("trial cleanup server failed",
				"server_id", serverID, "error", err)
		}
	}
	if failed > 0 {
		return fmt.Errorf("trial cleanup: %d/%d panel(s) failed", failed, len(serverOrder))
	}
	return nil
}

// cleanServer disables the server's expired trial clients and marks the ones
// that were actually disabled (panel failure → DB row untouched, retried on
// the next sweep).
func (s *Service) cleanServer(ctx context.Context, serverID int64, cands []postgres.ExpiredTrialCandidate) error {
	sctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	emails := make([]string, 0, len(cands))
	for _, c := range cands {
		emails = append(emails, c.Email)
	}
	failedEmails, err := s.disable.DisableClients(sctx, serverID, emails)
	failed := make(map[string]bool, len(failedEmails))
	for _, e := range failedEmails {
		failed[e] = true
	}
	var markFailed int
	for _, c := range cands {
		if failed[c.Email] {
			continue // panel error — keep the row active, retry next sweep
		}
		// Mark under the PARENT context, not sctx: a slow panel can exhaust the
		// per-server budget, and a confirmed disable must still be recorded
		// (same lesson as the health-check write, staging E2E v1.45).
		if err := s.store.MarkTrialExpired(ctx, c.ClientID); err != nil {
			markFailed++
			s.logger.Error("trial cleanup mark failed",
				"client_id", c.ClientID, "error", err)
		}
	}
	if err != nil {
		return err
	}
	if markFailed > 0 {
		return fmt.Errorf("trial cleanup: %d mark(s) failed on server %d", markFailed, serverID)
	}
	return nil
}
