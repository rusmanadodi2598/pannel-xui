// Package serversvc_test covers the buy-flow inbound picker (M7 fix).
//
// @file      internal/service/server/server_inbound_test.go
// @for       Unit tests: ListInbounds filtering, pinned-inbound provisioning.
// @uses      testing, context, github.com/kentangtech/bot-order/internal/crypto,
// github.com/kentangtech/bot-order/internal/repository/postgres, internal/repository/xui
// @reason    The FR-03 protocol picker must show only enabled inbounds and
// provision on the exact inbound the user chose (M7 gap fix).
//
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package serversvc

import (
	"context"
	"strings"
	"testing"

	"github.com/kentangtech/bot-order/internal/crypto"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	"github.com/kentangtech/bot-order/internal/repository/xui"
)

func TestListInbounds_GivenEnabledAndDisabled_ThenOnlyEnabledReturned(t *testing.T) {
	box := testBox(t)
	svc := New(&fakeServerStore{byID: &postgres.VPNServer{
		ID: 9, Name: "ID-01", CountryCode: "ID", Host: "h", Port: 1, Username: "u",
		PasswordEnc: mustEncrypt(t, box, "p"), APIPath: "/",
	}}, box, nil)
	svc.panelFactory = func(context.Context, int64) (inboundLister, error) {
		return &fakePanel{inbounds: []xui.Inbound{
			{ID: 1, Protocol: "vless", Enable: true, Port: 443, Remark: "reality"},
			{ID: 2, Protocol: "vless", Enable: false, Port: 443}, // disabled
			{ID: 3, Protocol: "trojan", Enable: true, Port: 0},   // port 0 → tidak valid
			{ID: 4, Protocol: "vmess", Enable: true, Port: 8443, Remark: "ws"},
		}}, nil
	}

	opts, err := svc.ListInbounds(context.Background(), 9)
	if err != nil {
		t.Fatalf("ListInbounds: %v", err)
	}
	if len(opts) != 2 {
		t.Fatalf("opts = %d, want 2 (inbounds 1 & 4)", len(opts))
	}
	if opts[0].InboundID != 1 || opts[0].Protocol != "vless" || opts[0].ServerName != "ID-01" || opts[0].Country != "ID" {
		t.Errorf("opts[0] = %+v, want inbound 1 vless on ID-01", opts[0])
	}
	if opts[1].InboundID != 4 || opts[1].Protocol != "vmess" {
		t.Errorf("opts[1] = %+v, want inbound 4 vmess", opts[1])
	}
}

func TestProvisionClient_GivenHysteria_ThenAuthCredentialReturned(t *testing.T) {
	box := testBox(t)
	svc := New(&fakeServerStore{byID: &postgres.VPNServer{
		ID: 9, Name: "ID-01", CountryCode: "ID", Host: "h", Port: 1, Username: "u",
		PasswordEnc: mustEncrypt(t, box, "p"), APIPath: "/",
	}}, box, nil)
	panel := &fakePanel{inbounds: []xui.Inbound{
		{ID: 5, Protocol: "hysteria", Enable: true, Port: 20005},
	}}
	svc.panelFactory = func(context.Context, int64) (inboundLister, error) { return panel, nil }

	pc, err := svc.CreateClient(context.Background(), 9, 5, "h@vpn.kt", "hysteria", 30, 1, 1)
	if err != nil {
		t.Fatalf("CreateClient(hysteria): %v", err)
	}
	if pc.InboundID != 5 {
		t.Errorf("inbound = %d, want 5 (pinned)", pc.InboundID)
	}
	if len(pc.Password) < 8 {
		t.Errorf("hysteria credential = %q, want auth secret", pc.Password)
	}
	if panel.added == nil || panel.added.InboundID != 5 || panel.added.Client.Auth == "" {
		t.Errorf("addClient payload = %+v, want inbound 5 with auth credential", panel.added)
	}
	// M7: the share link must be built from the server host (empty settings
	// still yields a valid hysteria2:// link with just the credential + port).
	if pc.ConfigLink == "" || !strings.HasPrefix(pc.ConfigLink, "hysteria2://") {
		t.Errorf("ConfigLink = %q, want hysteria2://... share link", pc.ConfigLink)
	}
}

func TestProvisionClient_GivenWSInbound_ThenStreamCaptured(t *testing.T) {
	// v1.27: network + path asli inbound (dari streamSettings API) ikut
	// tersimpan di PanelClient — dasar dual config link path dinamis.
	box := testBox(t)
	svc := New(&fakeServerStore{byID: &postgres.VPNServer{
		ID: 9, Name: "ID-01", CountryCode: "ID", Host: "h", Port: 1, Username: "u",
		PasswordEnc: mustEncrypt(t, box, "p"), APIPath: "/",
	}}, box, nil)
	svc.panelFactory = func(context.Context, int64) (inboundLister, error) {
		return &fakePanel{inbounds: []xui.Inbound{
			{ID: 6, Protocol: "vless", Enable: true, Port: 20006,
				StreamSettings: `{"network":"ws","security":"none","wsSettings":{"path":"/vlessws"}}`},
		}}, nil
	}

	pc, err := svc.CreateClient(context.Background(), 9, 6, "v@vpn.kt", "vless", 30, 1, 1)
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	if pc.InboundNetwork != "ws" || pc.InboundPath != "/vlessws" {
		t.Errorf("stream = (%q, %q), want (ws, /vlessws)", pc.InboundNetwork, pc.InboundPath)
	}
}

func TestMatchInboundByID_GivenPinnedID_ThenExactInbound(t *testing.T) {
	inbounds := []xui.Inbound{
		{ID: 1, Protocol: "vless", Enable: true, Port: 443},
		{ID: 2, Protocol: "vless", Enable: true, Port: 8443},
		{ID: 3, Protocol: "trojan", Enable: false, Port: 443},
	}
	got, ok := matchInboundByID(inbounds, 2)
	if !ok || got.ID != 2 {
		t.Errorf("matchInboundByID(2) = %+v, %v; want inbound 2", got, ok)
	}
	// Disabled inbound → tidak dipilih walau ID cocok.
	if _, ok := matchInboundByID(inbounds, 3); ok {
		t.Error("matchInboundByID(3) = ok, want false (disabled)")
	}
	// ID 0 / tidak ada → tidak match.
	if _, ok := matchInboundByID(inbounds, 0); ok {
		t.Error("matchInboundByID(0) = ok, want false")
	}
	if _, ok := matchInboundByID(inbounds, 99); ok {
		t.Error("matchInboundByID(99) = ok, want false")
	}
}

// fakePanel stubs the panel surface used by ListInbounds + provisionClient +
// renew (UpdateClient) + delete.
type fakePanel struct {
	inbounds   []xui.Inbound
	onlines    []xui.OnlineUser
	added      *xui.AddClientPayload
	updated    *xui.UpdateClientRawPayload
	deletedIn  int
	deletedCid string
}

func (f *fakePanel) GetInbounds(context.Context) ([]xui.Inbound, error) { return f.inbounds, nil }
func (f *fakePanel) GetOnlineClients(context.Context) ([]xui.OnlineUser, error) {
	return f.onlines, nil
}
func (f *fakePanel) AddClient(_ context.Context, p xui.AddClientPayload) error {
	f.added = &p
	return nil
}
func (f *fakePanel) UpdateClientRaw(_ context.Context, p xui.UpdateClientRawPayload) error {
	f.updated = &p
	return nil
}
func (f *fakePanel) DeleteClient(_ context.Context, inboundID int, clientID string) error {
	f.deletedIn = inboundID
	f.deletedCid = clientID
	return nil
}

var _ inboundLister = (*fakePanel)(nil)

func mustEncrypt(t *testing.T, box *crypto.SecretBox, s string) string {
	t.Helper()
	enc, err := box.Encrypt(s)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	return enc
}
