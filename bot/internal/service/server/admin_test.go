// Package serversvc_test covers the admin server-management ops (FR-11, v1.40).
//
// @file      internal/service/server/admin_test.go
// @for       Unit tests: ListAll, SetOpen/SetActive, AddServer validation + encryption.
// @uses      testing, context, strings, internal/crypto, internal/repository/postgres
// @reason    Admin add-server must seal the password like env seeding and reject
// duplicates before touching the store (FR-11 AC, PRD §15.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-12
package serversvc

import (
	"context"
	"strings"
	"testing"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

func TestListAll_GivenMixedServers_ThenAllReturned(t *testing.T) {
	store := &fakeServerStore{all: []postgres.ServerAdminView{
		{ID: 1, Name: "ID-01", IsActive: true, IsOpen: true},
		{ID: 2, Name: "SG-01", IsActive: false, IsOpen: false},
	}}
	svc := New(store, testBox(t), nil)

	rows, err := svc.ListAll(context.Background())
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(rows) != 2 || rows[1].ID != 2 {
		t.Errorf("ListAll = %+v, want 2 rows including inactive", rows)
	}
}

func TestSetOpen_GivenID_ThenFlagFlipped(t *testing.T) {
	store := &fakeServerStore{}
	svc := New(store, testBox(t), nil)
	if err := svc.SetOpen(context.Background(), 7, false); err != nil {
		t.Fatalf("SetOpen: %v", err)
	}
	if store.toggledID != 7 || store.setOpen == nil || *store.setOpen {
		t.Errorf("SetOpen = (id %d, open %v), want (7, false)", store.toggledID, store.setOpen)
	}
}

func TestSetActive_GivenID_ThenFlagFlipped(t *testing.T) {
	store := &fakeServerStore{}
	svc := New(store, testBox(t), nil)
	if err := svc.SetActive(context.Background(), 3, true); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	if store.toggledID != 3 || store.setActive == nil || !*store.setActive {
		t.Errorf("SetActive = (id %d, active %v), want (3, true)", store.toggledID, store.setActive)
	}
}

func TestAddServer_GivenValidInput_ThenEncryptedAndCreated(t *testing.T) {
	box := testBox(t)
	store := &fakeServerStore{}
	svc := New(store, box, nil)

	id, err := svc.AddServer(context.Background(), NewServerInput{
		Name: "ID-02", Host: "id2.example.com", Port: 2083,
		Username: "admin", Password: "secret-pw", APIPath: "/panel", UseSSL: true,
		CountryCode: "id", FlagEmoji: "🇮🇩", Location: "Jakarta",
		Protocols: []string{"vless", "vmess"},
	})
	if err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	if store.created == nil {
		t.Fatal("server not created")
	}
	if id != 99 || store.created.Name != "ID-02" {
		t.Errorf("id = %d, created = %+v", id, store.created)
	}
	if store.created.CountryCode != "ID" {
		t.Errorf("country = %q, want uppercased ID", store.created.CountryCode)
	}
	if store.created.PasswordEnc == "secret-pw" || store.created.PasswordEnc == "" {
		t.Errorf("password must be encrypted, got %q", store.created.PasswordEnc)
	}
	if dec, err := box.Decrypt(store.created.PasswordEnc); err != nil || dec != "secret-pw" {
		t.Errorf("round-trip = %q, err = %v", dec, err)
	}
	if !store.created.IsActive || !store.created.IsOpen {
		t.Error("new server must start active + open")
	}
}

func TestAddServer_GivenMissingField_ThenRejectedBeforeStore(t *testing.T) {
	svc := New(&fakeServerStore{}, testBox(t), nil)
	cases := []NewServerInput{
		{Name: "", Host: "h", Port: 443, Username: "u", Password: "p", CountryCode: "ID"},
		{Name: "x", Host: "", Port: 443, Username: "u", Password: "p", CountryCode: "ID"},
		{Name: "x", Host: "h", Port: 443, Username: "", Password: "p", CountryCode: "ID"},
		{Name: "x", Host: "h", Port: 443, Username: "u", Password: "", CountryCode: "ID"},
		{Name: "x", Host: "h", Port: 0, Username: "u", Password: "p", CountryCode: "ID"},
		{Name: "x", Host: "h", Port: 443, Username: "u", Password: "p", CountryCode: ""},
	}
	for _, in := range cases {
		if _, err := svc.AddServer(context.Background(), in); err == nil {
			t.Errorf("AddServer(%+v) = nil, want validation error", in)
		}
	}
}

func TestAddServer_GivenDuplicateHost_ThenRejected(t *testing.T) {
	svc := New(&fakeServerStore{dup: &postgres.VPNServer{ID: 5}}, testBox(t), nil)
	_, err := svc.AddServer(context.Background(), NewServerInput{
		Name: "dup", Host: "h", Port: 443, Username: "u", Password: "p", CountryCode: "ID",
	})
	if err == nil || !strings.Contains(err.Error(), "sudah terdaftar") {
		t.Errorf("AddServer dup = %v, want duplicate error", err)
	}
}
