// Package healthsvc test covers the health-check sweep (AGENTS.md §2.1).
//
// @file      internal/service/health/health_test.go
// @for       Unit tests: all-ok sweep, down-panel isolation, no-target short-circuit.
// @uses      testing, context, errors, io, log/slog, time, internal/repository/postgres, internal/repository/xui
// @reason    Guards the PRD §17 contract: reachable panels are marked ok, one
// down panel never aborts the sweep, and the buy menu filter depends on it.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-12
package healthsvc

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
	"github.com/kentangtech/bot-order/internal/repository/xui"
)

var errUnreachable = errors.New("panel unreachable")

type fakeHealthStore struct {
	targets []postgres.HealthTarget
	status  map[int64]string
}

func (f *fakeHealthStore) ListHealthTargets(context.Context) ([]postgres.HealthTarget, error) {
	return f.targets, nil
}

func (f *fakeHealthStore) UpdateHealth(_ context.Context, serverID int64, status string, _ time.Time) error {
	f.status[serverID] = status
	return nil
}

type fakeHealthPanel struct{ err error }

func (f *fakeHealthPanel) GetServerStatus(context.Context) (xui.Status, error) {
	return xui.Status{}, f.err
}

// hangingPanel blocks until its context expires — simulating a dead panel
// whose connect timeout exhausts the per-server budget.
type hangingPanel struct{}

func (hangingPanel) GetServerStatus(ctx context.Context) (xui.Status, error) {
	<-ctx.Done()
	return xui.Status{}, ctx.Err()
}

func testHealthLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRunOnce_GivenHealthyPanels_ThenAllMarkedOK(t *testing.T) {
	store := &fakeHealthStore{
		targets: []postgres.HealthTarget{{ID: 1, Name: "ID-01"}, {ID: 2, Name: "SG-01"}},
		status:  map[int64]string{},
	}
	svc := New(store, func(_ context.Context, _ int64) (Panel, error) {
		return &fakeHealthPanel{}, nil
	}, 30*time.Second, testHealthLogger())

	if err := svc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if store.status[1] != StatusOK || store.status[2] != StatusOK {
		t.Errorf("status = %v, want both ok", store.status)
	}
}

func TestRunOnce_GivenDownPanel_ThenMarkedDownAndOthersStillChecked(t *testing.T) {
	store := &fakeHealthStore{
		targets: []postgres.HealthTarget{{ID: 1, Name: "ID-01"}, {ID: 2, Name: "SG-01"}},
		status:  map[int64]string{},
	}
	svc := New(store, func(_ context.Context, serverID int64) (Panel, error) {
		if serverID == 1 {
			return &fakeHealthPanel{err: errUnreachable}, nil
		}
		return &fakeHealthPanel{}, nil
	}, 30*time.Second, testHealthLogger())

	err := svc.RunOnce(context.Background())
	if err == nil {
		t.Fatal("RunOnce = nil, want error reporting the down panel")
	}
	if store.status[1] != StatusDown {
		t.Errorf("server 1 status = %q, want down", store.status[1])
	}
	if store.status[2] != StatusOK {
		t.Errorf("server 2 status = %q, want ok (sweep continues)", store.status[2])
	}
}

func TestRunOnce_GivenPanelClientBuildFails_ThenMarkedDown(t *testing.T) {
	store := &fakeHealthStore{
		targets: []postgres.HealthTarget{{ID: 7, Name: "JP-01"}},
		status:  map[int64]string{},
	}
	svc := New(store, func(_ context.Context, _ int64) (Panel, error) {
		return nil, errUnreachable // decrypt/cache failure = unreachable
	}, 30*time.Second, testHealthLogger())

	if err := svc.RunOnce(context.Background()); err == nil {
		t.Fatal("RunOnce = nil, want error")
	}
	if store.status[7] != StatusDown {
		t.Errorf("server 7 status = %q, want down", store.status[7])
	}
}

// TestRunOnce_GivenPanelExhaustsBudget_ThenStatusStillPersisted locks the
// v1.45 staging fix: a dead panel consumes the whole per-server budget, yet
// the 'down' status must still be written (the DB write uses its own context).
func TestRunOnce_GivenPanelExhaustsBudget_ThenStatusStillPersisted(t *testing.T) {
	store := &fakeHealthStore{
		targets: []postgres.HealthTarget{{ID: 1, Name: "ID-01"}},
		status:  map[int64]string{},
	}
	svc := New(store, func(_ context.Context, _ int64) (Panel, error) {
		return hangingPanel{}, nil
	}, 50*time.Millisecond, testHealthLogger())

	if err := svc.RunOnce(context.Background()); err == nil {
		t.Fatal("RunOnce = nil, want error reporting the down panel")
	}
	if store.status[1] != StatusDown {
		t.Errorf("server 1 status = %q, want down (persisted despite exhausted budget)", store.status[1])
	}
}

func TestRunOnce_GivenNoTargets_ThenNoCalls(t *testing.T) {
	store := &fakeHealthStore{status: map[int64]string{}}
	svc := New(store, func(_ context.Context, _ int64) (Panel, error) {
		t.Fatal("panel must not be built with no targets")
		return nil, nil
	}, 30*time.Second, testHealthLogger())

	if err := svc.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(store.status) != 0 {
		t.Fatalf("status writes = %d, want 0", len(store.status))
	}
}
