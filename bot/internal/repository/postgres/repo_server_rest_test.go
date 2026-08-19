// Package postgres_test covers the REST admin server repo (PRD §26.5).
//
// @file      internal/repository/postgres/repo_server_rest_test.go
// @for       Integration: GetAdminByID (no secrets), UpdateServer patch,
// DeleteServer guard against ON DELETE CASCADE.
// @uses      testing, context, errors, internal/repository/postgres
// @reason    The delete guard protects user accounts — it must hold against
// real PostgreSQL FK semantics (AGENTS.md §1.7).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability experimental
// @since     2026-08-18
package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

func seedRestServer(t *testing.T, r *postgres.Repository, name string) int64 {
	t.Helper()
	s := &postgres.VPNServer{
		Name:        name,
		Host:        "h-" + name,
		Port:        2053,
		Username:    "u-" + name,
		PasswordEnc: "enc",
		CountryCode: "SG",
		IsActive:    true,
		IsOpen:      true,
		Protocols:   `["vless"]`,
	}
	if err := r.Servers().Create(context.Background(), s); err != nil {
		t.Fatalf("seed server: %v", err)
	}
	return s.ID
}

func TestServerRepo_GetAdminByID_ThenNoCredentials(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	id := seedRestServer(t, r, "sg")

	v, err := r.Servers().GetAdminByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetAdminByID: %v", err)
	}
	if v.ID != id || v.Name != "sg" || v.Host != "h-sg" {
		t.Fatalf("view = %+v", v)
	}
	// ServerAdminView carries no password/username fields by construction.
}

func TestServerRepo_UpdateServer_ThenOnlyPatchedFieldsChange(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	id := seedRestServer(t, r, "sg")

	name := "sg2"
	port := 3000
	if err := r.Servers().UpdateServer(context.Background(), id, postgres.ServerUpdate{
		Name: &name, Port: &port, IsOpen: restPtr(true),
	}); err != nil {
		t.Fatalf("UpdateServer: %v", err)
	}
	v, err := r.Servers().GetAdminByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetAdminByID: %v", err)
	}
	if v.Name != "sg2" || v.Port != 3000 || !v.IsOpen {
		t.Fatalf("view = %+v", v)
	}
	if v.Host != "h-sg" || v.CountryCode != "SG" {
		t.Fatalf("unpatched fields changed: %+v", v)
	}
}

func TestServerRepo_UpdateServer_GivenMissing_ThenNotFound(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	name := "x"
	if err := r.Servers().UpdateServer(context.Background(), 99999, postgres.ServerUpdate{Name: &name}); !errors.Is(err, postgres.ErrServerNotFound) {
		t.Fatalf("UpdateServer = %v, want ErrServerNotFound", err)
	}
}

func TestServerRepo_DeleteServer_GivenNoClients_ThenDeleted(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	id := seedRestServer(t, r, "sg")
	if err := r.Servers().DeleteServer(context.Background(), id); err != nil {
		t.Fatalf("DeleteServer: %v", err)
	}
	if _, err := r.Servers().GetAdminByID(context.Background(), id); !errors.Is(err, postgres.ErrServerNotFound) {
		t.Fatalf("GetAdminByID after delete = %v, want ErrServerNotFound", err)
	}
}

func TestServerRepo_DeleteServer_GivenClients_ThenGuardError(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	id := seedRestServer(t, r, "sg")

	u, err := r.User().FindOrCreate(context.Background(), 999001, "c", "C")
	if err != nil {
		t.Fatalf("FindOrCreate: %v", err)
	}
	client := &postgres.VPNClient{
		UserID:   u.ID,
		ServerID: id,
		Email:    "c1@test",
		Protocol: "vless",
	}
	if err := r.Clients().Create(context.Background(), client); err != nil {
		t.Fatalf("create client: %v", err)
	}

	if err := r.Servers().DeleteServer(context.Background(), id); !errors.Is(err, postgres.ErrServerHasClients) {
		t.Fatalf("DeleteServer = %v, want ErrServerHasClients", err)
	}
}

func restPtr[T any](v T) *T { return &v }
