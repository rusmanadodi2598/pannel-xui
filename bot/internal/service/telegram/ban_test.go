// Package telegram test also covers the ban policy (FR-01/FR-11).
//
// @file      internal/service/telegram/ban_test.go
// @for       IsBanned marker check, admin Ban/Unban key+TTL semantics.
// @uses      testing, context, errors, time
// @reason    Ban gates every request — key shape and TTL must be locked by
// tests before the dispatcher relies on them (M7 hardening).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeBanStore struct {
	vals    map[string]string
	setTTL  time.Duration
	setVal  string
	deleted []string
	err     error
}

func newFakeBanStore() *fakeBanStore { return &fakeBanStore{vals: map[string]string{}} }

func (f *fakeBanStore) Exists(_ context.Context, key string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	_, ok := f.vals[key]
	return ok, nil
}

func (f *fakeBanStore) SetString(_ context.Context, key, value string, ttl time.Duration) error {
	if f.err != nil {
		return f.err
	}
	f.vals[key] = value
	f.setVal, f.setTTL = value, ttl
	return nil
}

func (f *fakeBanStore) Delete(_ context.Context, key string) error {
	if f.err != nil {
		return f.err
	}
	delete(f.vals, key)
	f.deleted = append(f.deleted, key)
	return nil
}

func TestBan_GivenUser_ThenMarkerSetOnBanKeyWithLongTTL(t *testing.T) {
	store := newFakeBanStore()
	s := NewBanService(store)

	if err := s.Ban(context.Background(), 42); err != nil {
		t.Fatalf("Ban: %v", err)
	}
	if _, ok := store.vals[BanKey(42)]; !ok {
		t.Errorf("marker %q not set", BanKey(42))
	}
	if store.setVal != "1" {
		t.Errorf("marker value = %q, want %q", store.setVal, "1")
	}
	if store.setTTL != BanTTL {
		t.Errorf("marker TTL = %v, want BanTTL %v (1 year crash guard)", store.setTTL, BanTTL)
	}
}

func TestUnban_GivenUser_ThenMarkerRemoved(t *testing.T) {
	store := newFakeBanStore()
	s := NewBanService(store)
	_ = store.SetString(context.Background(), BanKey(7), "1", BanTTL)

	if err := s.Unban(context.Background(), 7); err != nil {
		t.Fatalf("Unban: %v", err)
	}
	if len(store.deleted) != 1 || store.deleted[0] != BanKey(7) {
		t.Errorf("deleted = %v, want [%s]", store.deleted, BanKey(7))
	}
	ok, _ := s.IsBanned(context.Background(), 7)
	if ok {
		t.Error("IsBanned after Unban = true, want false")
	}
}

func TestIsBanned_GivenMarker_ThenTrue(t *testing.T) {
	store := newFakeBanStore()
	s := NewBanService(store)
	_ = store.SetString(context.Background(), BanKey(9), "1", BanTTL)

	ok, err := s.IsBanned(context.Background(), 9)
	if err != nil || !ok {
		t.Fatalf("IsBanned = %v, %v; want true, nil", ok, err)
	}
}

func TestIsBanned_GivenNoMarker_ThenFalse(t *testing.T) {
	s := NewBanService(newFakeBanStore())
	ok, err := s.IsBanned(context.Background(), 1)
	if err != nil || ok {
		t.Fatalf("IsBanned = %v, %v; want false, nil", ok, err)
	}
}

func TestIsBanned_GivenStoreError_ThenFailClosed(t *testing.T) {
	store := newFakeBanStore()
	store.err = errors.New("redis down")
	s := NewBanService(store)

	if _, err := s.IsBanned(context.Background(), 1); err == nil {
		t.Fatal("IsBanned err = nil, want propagated (dispatcher treats as fail-closed)")
	}
}

var _ ExistsStore = (*fakeBanStore)(nil)
