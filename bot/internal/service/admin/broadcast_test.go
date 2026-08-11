// Package adminsvc test also covers the FR-11 broadcast loop.
//
// @file      internal/service/admin/broadcast_test.go
// @for       Given users, then broadcast is chunked, locked and reports; busy
//
//	and empty cases are handled.
//
// @uses      testing, context, errors, time
// @reason    Guards the broadcast side effects without Telegram/Redis.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-11
package adminsvc

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestService_Broadcast_GivenUsers_ThenChunkedLockedAndReports(t *testing.T) {
	adminID := int64(99999)
	users := &fakeUsers{ids: []int64{1, 2, 3, 4, 5}, total: 5}
	sender := &fakeSender{admin: adminID, done: make(chan struct{})}
	locker := &fakeLocker{}
	s := newTestService(users, sender, locker)

	n, err := s.Broadcast(context.Background(), adminID, "halo")
	if err != nil {
		t.Fatalf("Broadcast: %v", err)
	}
	if n != 5 {
		t.Fatalf("Broadcast total = %d, want 5", n)
	}

	select {
	case <-sender.done:
	case <-time.After(3 * time.Second):
		t.Fatal("broadcast did not finish (completion report missing)")
	}
	// 5 users + 1 admin report = 6 sends.
	if got := sender.count(); got != 6 {
		t.Fatalf("sends = %d, want 6", got)
	}
	if locker.releaseCount() != 1 {
		t.Fatalf("lock releases = %d, want 1", locker.releaseCount())
	}
}

func TestService_Broadcast_GivenBusy_ThenErrBroadcastRunning(t *testing.T) {
	locker := &fakeLocker{busy: true}
	s := newTestService(&fakeUsers{total: 3}, &fakeSender{}, locker)

	if _, err := s.Broadcast(context.Background(), 1, "x"); !errors.Is(err, ErrBroadcastRunning) {
		t.Fatalf("Broadcast err = %v, want ErrBroadcastRunning", err)
	}
}

func TestService_Broadcast_GivenNoUsers_ThenNoop(t *testing.T) {
	s := newTestService(&fakeUsers{total: 0}, &fakeSender{}, &fakeLocker{})
	n, err := s.Broadcast(context.Background(), 1, "x")
	if err != nil || n != 0 {
		t.Fatalf("Broadcast = %d, %v; want 0, nil", n, err)
	}
}
