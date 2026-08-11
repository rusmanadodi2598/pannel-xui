// Package job test covers the generic interval worker loop.
//
// @file      internal/job/interval_test.go
// @for       Given ctx cancel, then loop stops; immediate first sweep; panics recovered.
// @uses      testing, context, io, log/slog, errors, time
// @reason    Guards goroutine lifecycle and panic safety (AGENTS.md §1.6).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     job
// @stability stable
// @since     2026-08-11
package job

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

var errBoom = errors.New("boom")

type fakeSweeper struct {
	mu      sync.Mutex
	calls   int
	err     error
	panicOn bool
}

func (f *fakeSweeper) RunOnce(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.panicOn {
		panic("sweep boom")
	}
	return f.err
}

func (f *fakeSweeper) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestIntervalWorker_Run_GivenCancel_ThenStopsAfterImmediateSweep(t *testing.T) {
	svc := &fakeSweeper{}
	j := NewIntervalWorker(time.Hour, time.Minute, svc, testLogger()) // long interval: only the immediate sweep fires

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		j.Run(ctx)
		close(done)
	}()

	// The first sweep runs synchronously at start — wait until it happened.
	deadline := time.Now().Add(2 * time.Second)
	for svc.count() < 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := svc.count(); got != 1 {
		t.Fatalf("sweeps = %d, want exactly 1 (immediate)", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after context cancellation")
	}
}

func TestIntervalWorker_Sweep_GivenPanic_ThenRecovered(t *testing.T) {
	svc := &fakeSweeper{panicOn: true}
	j := NewIntervalWorker(time.Hour, time.Minute, svc, testLogger())

	// Must not panic even though the sweeper panics.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("sweep leaked a panic: %v", r)
			}
		}()
		j.sweep(context.Background())
	}()
	if got := svc.count(); got != 1 {
		t.Fatalf("sweeper calls = %d, want 1", got)
	}
}

func TestIntervalWorker_Sweep_GivenError_ThenLoggedNotFatal(t *testing.T) {
	svc := &fakeSweeper{err: errBoom}
	j := NewIntervalWorker(time.Hour, time.Minute, svc, testLogger())

	j.sweep(context.Background()) // must not panic or propagate
	if err := j.Sweep(context.Background()); err != errBoom {
		t.Fatalf("Sweep = %v, want the sweeper error", err)
	}
}
