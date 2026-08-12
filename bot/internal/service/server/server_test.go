// Package serversvc_test covers panel seeding and the gateway (AGENTS.md §2.1).
//
// @file      internal/service/server/server_test.go
// @for       Unit tests: EnsureSeeded encryption, PickForCountry, PanelClient creds.
// @uses      testing, context, github.com/kentangtech/bot-order/internal/crypto,
// github.com/kentangtech/bot-order/internal/config, internal/repository/postgres
// @reason    Panel secrets must never reach the DB in plaintext (PRD §15.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package serversvc

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/config"
	"github.com/kentangtech/bot-order/internal/crypto"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	"github.com/kentangtech/bot-order/internal/repository/xui"
)

func TestEnsureSeeded_GivenPanels_ThenPasswordsEncryptedAtRest(t *testing.T) {
	box := testBox(t)
	store := &fakeServerStore{}
	svc := New(store, box, nil)

	seeds := []config.ServerSeed{
		{Name: "ID-01", Host: "id.example.com", Port: 443, Username: "admin",
			Password: "plaintext-pw", APIPath: "/panel", UseSSL: true, CountryCode: "ID",
			FlagEmoji: "🇮🇩", Protocols: []string{"vless"}},
	}
	if err := svc.EnsureSeeded(context.Background(), seeds); err != nil {
		t.Fatalf("EnsureSeeded: %v", err)
	}
	if len(store.upserted) != 1 {
		t.Fatalf("upserted = %d, want 1", len(store.upserted))
	}
	row := store.upserted[0]
	if row.PasswordEnc == "plaintext-pw" || row.PasswordEnc == "" {
		t.Errorf("password must be encrypted at rest, got %q", row.PasswordEnc)
	}
	dec, err := box.Decrypt(row.PasswordEnc)
	if err != nil || dec != "plaintext-pw" {
		t.Errorf("round-trip = %q, err = %v", dec, err)
	}
}

func TestPickForCountry_GivenBuyableServers_ThenFirstMatch(t *testing.T) {
	store := &fakeServerStore{buyable: []postgres.ServerView{
		{ID: 1, CountryCode: "SG"},
		{ID: 2, CountryCode: "ID"},
		{ID: 3, CountryCode: "ID"},
	}}
	svc := New(store, testBox(t), nil)

	id, err := svc.PickForCountry(context.Background(), "ID")
	if err != nil {
		t.Fatalf("PickForCountry: %v", err)
	}
	if id != 2 {
		t.Errorf("id = %d, want 2 (first ID server)", id)
	}
	if _, err := svc.PickForCountry(context.Background(), "JP"); err == nil {
		t.Error("expected ErrNoServer for JP")
	}
}

func TestPanelClient_GivenStoredServer_ThenDecryptedCredentials(t *testing.T) {
	box := testBox(t)
	enc, _ := box.Encrypt("panel-pw")
	store := &fakeServerStore{byID: &postgres.VPNServer{
		ID: 9, Name: "SG-01", Host: "sg.example.com", Port: 2083,
		Username: "root", PasswordEnc: enc, APIPath: "/panel", UseSSL: true,
	}}
	svc := New(store, box, nil)

	client, err := svc.PanelClient(context.Background(), 9)
	if err != nil {
		t.Fatalf("PanelClient: %v", err)
	}
	if client == nil {
		t.Fatal("client is nil")
	}
}

func TestMatchInbound_GivenProtocol_ThenSelectsEnabledWithPort(t *testing.T) {
	inbounds := []xui.Inbound{
		{ID: 1, Protocol: "vmess", Enable: true, Port: 443},
		{ID: 2, Protocol: "vless", Enable: true, Port: 0},    // port 0 → tidak valid
		{ID: 3, Protocol: "vless", Enable: false, Port: 443}, // disabled → dilewati
		{ID: 4, Protocol: "vless", Enable: true, Port: 8443},
	}
	got, ok := matchInbound(inbounds, "vless")
	if !ok || got.ID != 4 {
		t.Errorf("matchInbound(vless) = %+v, %v; want inbound 4", got, ok)
	}
	// Case-insensitive (VLESS vs vless).
	got, ok = matchInbound(inbounds, "VLESS")
	if !ok || got.ID != 4 {
		t.Errorf("matchInbound(VLESS) = %+v, %v; want inbound 4", got, ok)
	}
	// Protocol yang tidak ada → tidak match.
	if _, ok := matchInbound(inbounds, "trojan"); ok {
		t.Error("matchInbound(trojan) = ok, want false")
	}
	// Inbounds kosong → tidak match.
	if _, ok := matchInbound(nil, "vless"); ok {
		t.Error("matchInbound(nil) = ok, want false")
	}
}

func TestRenewClient_GivenUnknownServer_ThenErrorBeforePanelCall(t *testing.T) {
	box := testBox(t)
	store := &fakeServerStore{byIDErr: errNotFound}
	svc := New(store, box, nil)

	err := svc.RenewClient(context.Background(), 999, "cid", "a@vpn.kt", "vless", time.Now())
	if err == nil {
		t.Fatal("RenewClient = nil, want error for unknown server")
	}
}

func TestRenewClient_GivenCorruptPassword_ThenError(t *testing.T) {
	box := testBox(t)
	store := &fakeServerStore{byID: &postgres.VPNServer{
		ID: 9, Name: "SG-01", Host: "sg.example.com", Port: 2083,
		Username: "root", PasswordEnc: "garbage-not-encrypted", APIPath: "/panel", UseSSL: true,
	}}
	svc := New(store, box, nil)

	err := svc.RenewClient(context.Background(), 9, "cid", "a@vpn.kt", "vless", time.Now())
	if err == nil {
		t.Fatal("RenewClient = nil, want decrypt error")
	}
}

func TestDeleteClient_GivenOwnedIDs_ThenPanelDelClientCalled(t *testing.T) {
	box := testBox(t)
	svc := New(&fakeServerStore{byID: &postgres.VPNServer{
		ID: 9, Name: "ID-01", CountryCode: "ID", Host: "h", Port: 1, Username: "u",
		PasswordEnc: mustEncrypt(t, box, "p"), APIPath: "/",
	}}, box, nil)
	panel := &fakePanel{}
	svc.panelFactory = func(context.Context, int64) (inboundLister, error) { return panel, nil }

	if err := svc.DeleteClient(context.Background(), 9, 5, "uuid-1"); err != nil {
		t.Fatalf("DeleteClient: %v", err)
	}
	if panel.deletedIn != 5 || panel.deletedCid != "uuid-1" {
		t.Errorf("delClient = (inbound %d, client %s), want (5, uuid-1)", panel.deletedIn, panel.deletedCid)
	}
}

func TestDeleteClient_GivenUnknownServer_ThenErrorBeforePanelCall(t *testing.T) {
	svc := New(&fakeServerStore{byIDErr: errNotFound}, testBox(t), nil)
	if err := svc.DeleteClient(context.Background(), 999, 5, "uuid-1"); err == nil {
		t.Fatal("DeleteClient = nil, want error for unknown server")
	}
}

// --- fakes ---

type fakeServerStore struct {
	upserted  []postgres.VPNServer
	buyable   []postgres.ServerView
	all       []postgres.ServerAdminView
	byID      *postgres.VPNServer
	byIDErr   error
	created   *postgres.VPNServer
	createErr error
	dup       *postgres.VPNServer
	setOpen   *bool
	setActive *bool
	toggledID int64
	toggleErr error
}

func (f *fakeServerStore) UpsertSeed(_ context.Context, s postgres.VPNServer) error {
	f.upserted = append(f.upserted, s)
	return nil
}
func (f *fakeServerStore) ListBuyable(context.Context) ([]postgres.ServerView, error) {
	return f.buyable, nil
}
func (f *fakeServerStore) GetByID(_ context.Context, _ int64) (*postgres.VPNServer, error) {
	return f.byID, f.byIDErr
}
func (f *fakeServerStore) ListAll(context.Context) ([]postgres.ServerAdminView, error) {
	return f.all, nil
}
func (f *fakeServerStore) SetOpen(_ context.Context, id int64, open bool) error {
	f.toggledID, f.setOpen = id, &open
	return f.toggleErr
}
func (f *fakeServerStore) SetActive(_ context.Context, id int64, active bool) error {
	f.toggledID, f.setActive = id, &active
	return f.toggleErr
}
func (f *fakeServerStore) Create(_ context.Context, s *postgres.VPNServer) error {
	s.ID = 99
	f.created = s
	return f.createErr
}
func (f *fakeServerStore) FindByHostPort(context.Context, string, int, string) (*postgres.VPNServer, error) {
	return f.dup, nil
}

var errNotFound = errors.New("server not found")

func testBox(t *testing.T) *crypto.SecretBox {
	t.Helper()
	box, err := crypto.NewSecretBox(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatalf("NewSecretBox: %v", err)
	}
	return box
}

var _ Store = (*fakeServerStore)(nil)
