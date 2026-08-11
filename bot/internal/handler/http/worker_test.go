// Package httphandler test covers the bounded update worker pool.
//
// @file      internal/handler/http/worker_test.go
// @for       Drain on close, per-user lock skip and error paths, full-queue drop (PRD §14.2).
// @uses      testing, context, io, log/slog, sync, time, errors, github.com/go-telegram/bot/models
// @reason    Guards serialization and back-pressure behavior of the ingestion pipeline.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package httphandler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
)

var errBoom = errors.New("boom")

type fakeProcessor struct {
	mu      sync.Mutex
	handled []int64
}

func (f *fakeProcessor) Handle(_ context.Context, upd *models.Update) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handled = append(f.handled, upd.ID)
}

func (f *fakeProcessor) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.handled)
}

type openLock struct{}

func (openLock) AcquireLock(context.Context, string, time.Duration) (bool, error) { return true, nil }
func (openLock) ReleaseLock(context.Context, string) error                        { return nil }

type denyLock struct{}

func (denyLock) AcquireLock(context.Context, string, time.Duration) (bool, error) { return false, nil }
func (denyLock) ReleaseLock(context.Context, string) error                        { return nil }

type errLock struct{}

func (errLock) AcquireLock(context.Context, string, time.Duration) (bool, error) {
	return false, errBoom
}
func (errLock) ReleaseLock(context.Context, string) error { return nil }

func newWorker(proc UpdateProcessor, locks LockStore) *Worker {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewWorker(2, 8, proc, locks, logger)
}

func TestWorker_GivenEnqueuedUpdates_ThenProcessedOnClose(t *testing.T) {
	proc := &fakeProcessor{}
	w := newWorker(proc, openLock{})

	w.Enqueue(context.Background(), &models.Update{ID: 1, Message: &models.Message{}})
	w.Enqueue(context.Background(), &models.Update{ID: 2, Message: &models.Message{}})
	w.Close()

	if got := proc.count(); got != 2 {
		t.Fatalf("processed = %d, want 2", got)
	}
}

func userUpdate(id int64) *models.Update {
	return &models.Update{ID: id, Message: &models.Message{From: &models.User{ID: 1}}}
}

func TestWorker_GivenLockBusy_ThenUpdateSkipped(t *testing.T) {
	proc := &fakeProcessor{}
	w := newWorker(proc, denyLock{})

	w.Enqueue(context.Background(), userUpdate(1))
	w.Close()

	if got := proc.count(); got != 0 {
		t.Fatalf("processed = %d, want 0 (lock busy)", got)
	}
}

func TestWorker_GivenLockError_ThenUpdateSkipped(t *testing.T) {
	proc := &fakeProcessor{}
	w := newWorker(proc, errLock{})

	w.Enqueue(context.Background(), userUpdate(1))
	w.Close()

	if got := proc.count(); got != 0 {
		t.Fatalf("processed = %d, want 0 (lock error)", got)
	}
}

// trackLock records lock acquisitions/releases to assert release-after-handle.
type trackLock struct {
	mu      sync.Mutex
	locked  int
	release int
}

func (t *trackLock) AcquireLock(context.Context, string, time.Duration) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.locked++
	return true, nil
}

func (t *trackLock) ReleaseLock(context.Context, string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.release++
	return nil
}

func TestWorker_GivenUserUpdate_ThenLockReleasedAfterHandle(t *testing.T) {
	proc := &fakeProcessor{}
	locks := &trackLock{}
	w := newWorker(proc, locks)

	w.Enqueue(context.Background(), userUpdate(1))
	w.Close()

	locks.mu.Lock()
	defer locks.mu.Unlock()
	if locks.locked != 1 || locks.release != 1 {
		t.Fatalf("lock acquired=%d released=%d, want 1/1", locks.locked, locks.release)
	}
}

func TestWorker_GivenFullQueue_ThenEnqueueReturnsFalse(t *testing.T) {
	proc := &fakeProcessor{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewWorker(0, 1, proc, openLock{}, logger) // no workers: queue never drains
	defer w.Close()

	first := w.Enqueue(context.Background(), &models.Update{ID: 1, Message: &models.Message{}})
	second := w.Enqueue(context.Background(), &models.Update{ID: 2, Message: &models.Message{}})
	if !first || second {
		t.Fatalf("Enqueue = %v, %v; want true, false", first, second)
	}
}
