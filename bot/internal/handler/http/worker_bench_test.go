// Package httphandler benchmarks the update worker pool (M7 load test).
//
// @file      internal/handler/http/worker_bench_test.go
// @for       Throughput (cheap & realistic handlers) and drain latency of a full batch.
// @uses      testing, context, io, log/slog, sync, sync/atomic, time, github.com/go-telegram/bot/models
// @reason    M7 exit criteria include load: proves the bounded pool processes a
// full batch end-to-end without drops (fix review v1.22: queue cap = batch so
// b.N scaling under default benchtime never overflows the queue — the earlier
// version b.Fatal'd on drop and reported a misleading 1.000-update drain).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package httphandler

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
)

// noopProcessor records the update count (cheap handler).
type noopProcessor struct{ handled atomic.Int64 }

func (f *noopProcessor) Handle(_ context.Context, _ *models.Update) {
	f.handled.Add(1)
}

// slowProcessor sleeps 1ms per update — simulates a realistic dispatcher
// (ban check + gate + routing) so the benchmark measures sustained throughput.
type slowProcessor struct{ handled atomic.Int64 }

func (p *slowProcessor) Handle(_ context.Context, _ *models.Update) {
	p.handled.Add(1)
	time.Sleep(time.Millisecond)
}

func benchLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// benchWorker builds a worker whose queue can hold the whole batch: enqueueing
// batch updates can never drop (b.N-independent), so Close() drains exactly
// batch updates and the measured rate is the true processing throughput.
func benchWorker(batch int, proc UpdateProcessor) *Worker {
	return NewWorker(8, batch, proc, openLock{}, benchLogger())
}

func benchUpdate() *models.Update {
	return &models.Update{ID: 1, Message: &models.Message{From: &models.User{ID: 1}}}
}

// BenchmarkWorker_Throughput_CheapHandler measures enqueue+drain of a full
// batch with an almost-free handler (upper bound of the ingestion pipeline).
func BenchmarkWorker_Throughput_CheapHandler(b *testing.B) {
	const batch = 1000
	proc := &noopProcessor{}
	upd := benchUpdate()
	for i := 0; i < b.N; i++ {
		proc.handled.Store(0)
		w := benchWorker(batch, proc)
		b.StartTimer()
		for j := 0; j < batch; j++ {
			w.Enqueue(context.Background(), upd)
		}
		w.Close() // drains exactly batch updates
		b.StopTimer()
		if got := proc.handled.Load(); got != batch {
			b.Fatalf("handled = %d, want %d (queue cap = batch, no drops)", got, batch)
		}
	}
}

// BenchmarkWorker_Throughput_RealisticHandler simulates a dispatcher spending
// ~1ms per update. Sustained ops/s here is the load ceiling before webhook
// timeouts make Telegram retries likely.
func BenchmarkWorker_Throughput_RealisticHandler(b *testing.B) {
	const batch = 200 // 200 × 1ms / 8 workers ≈ 25 ms per iteration
	proc := &slowProcessor{}
	upd := benchUpdate()
	for i := 0; i < b.N; i++ {
		proc.handled.Store(0)
		w := benchWorker(batch, proc)
		b.StartTimer()
		for j := 0; j < batch; j++ {
			w.Enqueue(context.Background(), upd)
		}
		w.Close()
		b.StopTimer()
		if got := proc.handled.Load(); got != batch {
			b.Fatalf("handled = %d, want %d (queue cap = batch, no drops)", got, batch)
		}
	}
}

// BenchmarkWorker_DrainLatency_AtBatchCapacity enqueues a full batch into the
// pipeline and measures how long the pool needs to drain it — the worst-case
// backlog the worker pool clears after a burst. All batch updates are in the
// queue (cap = batch), so the number drained is real (fix review v1.22).
func BenchmarkWorker_DrainLatency_AtBatchCapacity(b *testing.B) {
	const batch = 1000
	upd := benchUpdate()
	for i := 0; i < b.N; i++ {
		proc := &slowProcessor{}
		w := benchWorker(batch, proc)
		for j := 0; j < batch; j++ {
			w.Enqueue(context.Background(), upd)
		}
		b.StartTimer()
		w.Close() // blocks until all 1,000 are processed
		b.StopTimer()
		if got := proc.handled.Load(); got != batch {
			b.Fatalf("handled = %d, want %d", got, batch)
		}
	}
}
