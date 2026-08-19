// Package serversvc tests cover the REST admin server ops (PRD §26.5).
//
// @file      internal/service/server/server_rest_test.go
// @for       UpdateServer (password re-seal), DeleteServer guard passthrough,
// CheckHealth ok/down via the statusFactory seam.
// @uses      testing, context, errors, internal/repository/postgres,
// internal/repository/xui
// @reason    Guards the /api/v1/servers service logic without a live panel
// (AGENTS.md §2.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-18
package serversvc

import (
	"context"
	"errors"
	"testing"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
	"github.com/kentangtech/bot-order/internal/repository/xui"
)

func TestUpdateServer_GivenPassword_ThenReEncryptedAndStored(t *testing.T) {
	store := &fakeServerStore{}
	svc := New(store, testBox(t), nil)
	name := "jp"
	pw := "secret"
	err := svc.UpdateServer(context.Background(), UpdateServerInput{ID: 5, Name: &name, Password: &pw})
	if err != nil {
		t.Fatalf("UpdateServer = %v, want nil", err)
	}
	if store.updatedID != 5 {
		t.Fatalf("updatedID = %d, want 5", store.updatedID)
	}
	if store.updated.PasswordEnc == nil || *store.updated.PasswordEnc == "secret" {
		t.Fatalf("password not sealed: %v", store.updated.PasswordEnc)
	}
	if store.updated.Name == nil || *store.updated.Name != "jp" {
		t.Fatalf("name not updated: %v", store.updated.Name)
	}
}

func TestUpdateServer_GivenInvalidPort_ThenErrorBeforeStore(t *testing.T) {
	store := &fakeServerStore{}
	svc := New(store, testBox(t), nil)
	port := 70000
	if err := svc.UpdateServer(context.Background(), UpdateServerInput{ID: 5, Port: &port}); err == nil {
		t.Fatal("UpdateServer = nil, want invalid-port error")
	}
	if store.updatedID != 0 {
		t.Fatalf("store touched despite invalid port (id=%d)", store.updatedID)
	}
}

func TestDeleteServer_GivenHasClients_ThenGuardError(t *testing.T) {
	store := &fakeServerStore{deleteErr: postgres.ErrServerHasClients}
	svc := New(store, testBox(t), nil)
	if err := svc.DeleteServer(context.Background(), 7); !errors.Is(err, postgres.ErrServerHasClients) {
		t.Fatalf("DeleteServer = %v, want ErrServerHasClients", err)
	}
}

type fakeStatusProber struct{ err error }

func (f fakeStatusProber) GetServerStatus(context.Context) (xui.Status, error) {
	return xui.Status{}, f.err
}

func TestCheckHealth_GivenPanelOK_ThenOK(t *testing.T) {
	svc := New(&fakeServerStore{}, testBox(t), nil)
	svc.statusFactory = func(context.Context, int64) (statusProber, error) {
		return fakeStatusProber{}, nil
	}
	got, err := svc.CheckHealth(context.Background(), 1)
	if err != nil || got != HealthOK {
		t.Fatalf("CheckHealth = (%q, %v), want (ok, nil)", got, err)
	}
}

func TestCheckHealth_GivenPanelDown_ThenDown(t *testing.T) {
	svc := New(&fakeServerStore{}, testBox(t), nil)
	svc.statusFactory = func(context.Context, int64) (statusProber, error) {
		return fakeStatusProber{err: errors.New("boom")}, nil
	}
	got, err := svc.CheckHealth(context.Background(), 1)
	if err != nil || got != HealthDown {
		t.Fatalf("CheckHealth = (%q, %v), want (down, nil)", got, err)
	}
}

func TestCheckHealth_GivenMissingServer_ThenDownNotError(t *testing.T) {
	svc := New(&fakeServerStore{}, testBox(t), nil)
	svc.statusFactory = func(context.Context, int64) (statusProber, error) {
		return nil, postgres.ErrServerNotFound
	}
	got, err := svc.CheckHealth(context.Background(), 999)
	if err != nil || got != HealthDown {
		t.Fatalf("CheckHealth = (%q, %v), want (down, nil)", got, err)
	}
}
