// Package expirysvc drives the FR-09 expiry reminders (H-7/H-3/H-1, M6).
//
// @file      internal/service/expiry/expiry.go
// @for       FR-09 worker: sweep each notification window, send reminder, mark notified.
// @uses      context, log/slog, sort, time, github.com/go-telegram/bot/models,
// internal/repository/postgres, internal/service/telegram (copy)
// @reason    Enforces "send once per threshold" (FR-09 AC) via exclusive windows
//
//	and the notified_expiry guard; a send failure never marks, so the
//	next sweep retries it.
//
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package expirysvc

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	telegramservice "github.com/kentangtech/bot-order/internal/service/telegram"
)

// ClientLister lists expiry candidates and records what was notified
// (postgres.ClientRepo implements it).
type ClientLister interface {
	ListExpiryCandidates(ctx context.Context, upperDays, lowerDays, limit int) ([]postgres.ExpiryCandidate, error)
	MarkNotified(ctx context.Context, clientID int64, day int) error
}

// Messenger sends Telegram messages (repository/telegram.Client implements it).
type Messenger interface {
	SendMessage(ctx context.Context, chatID int64, text string, parseMode models.ParseMode, markup models.ReplyMarkup) error
}

// Service sweeps the configured threshold windows and notifies each candidate
// exactly once per threshold.
type Service struct {
	clients ClientLister
	send    Messenger
	days    []int // sorted descending, e.g. [7 3 1]
	batch   int
	loc     *time.Location // TIME_LOCATION for displayed expiry date (FR-09 AC)
	logger  *slog.Logger
}

// New builds the service. days is copied and sorted descending; windows are
// exclusive: (nextSmaller, day] so a client is notified once per sweep.
func New(clients ClientLister, send Messenger, days []int, batch int, loc *time.Location, logger *slog.Logger) *Service {
	sorted := append([]int(nil), days...)
	sort.Sort(sort.Reverse(sort.IntSlice(sorted)))
	if batch <= 0 {
		batch = 50
	}
	if loc == nil {
		loc = time.UTC
	}
	return &Service{clients: clients, send: send, days: sorted, batch: batch, loc: loc, logger: logger}
}

// Days exposes the sorted threshold windows (tests / logging).
func (s *Service) Days() []int { return append([]int(nil), s.days...) }

// RunOnce performs one sweep across every window. A window failure is logged
// and reported but does not stop the remaining windows.
func (s *Service) RunOnce(ctx context.Context) error {
	var firstErr error
	for i, upper := range s.days {
		lower := 0
		if i+1 < len(s.days) {
			lower = s.days[i+1]
		}
		if err := s.sweepWindow(ctx, upper, lower); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("window H-%d: %w", upper, err)
		}
	}
	return firstErr
}

// sweepWindow fetches the bounded candidate list for one window and notifies
// each client individually (send + mark are per-row by design).
func (s *Service) sweepWindow(ctx context.Context, upper, lower int) error {
	candidates, err := s.clients.ListExpiryCandidates(ctx, upper, lower, s.batch)
	if err != nil {
		return err
	}
	for _, c := range candidates {
		s.notifyOne(ctx, upper, c)
	}
	return nil
}

// notifyOne sends the reminder, then marks the threshold only on success —
// a failed send is retried by the next sweep, a failed mark may resend once
// (idempotent reminder, never duplicated charges).
func (s *Service) notifyOne(ctx context.Context, day int, c postgres.ExpiryCandidate) {
	expiryDate := c.ExpiresAt.In(s.loc).Format("02 Jan 2006")
	text := telegramservice.ExpiryNotifyText(day, c.ServerName, c.Email, expiryDate)
	if err := s.send.SendMessage(ctx, c.TelegramID, text, "", nil); err != nil {
		s.logger.Error("expiry reminder send failed (not marked, will retry)",
			"client_id", c.ClientID, "telegram_id", c.TelegramID, "day", day, "error", err)
		return
	}
	if err := s.clients.MarkNotified(ctx, c.ClientID, day); err != nil {
		s.logger.Error("expiry reminder mark failed (may resend)",
			"client_id", c.ClientID, "day", day, "error", err)
	}
}
