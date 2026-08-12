// Package serversvc_test covers FR-13 subId propagation (AGENTS.md §2.1).
//
// @file      internal/service/server/provision_subid_test.go
// @for       Verify provisionClient exposes the subId sent to the panel.
// @uses      testing, context, internal/repository/postgres, internal/repository/xui
// @reason    The order flow builds the subscription URL from this value — a
// missing SubID would silently ship empty sub URLs (FR-13 AC).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-12
package serversvc

import (
	"context"
	"testing"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
	"github.com/kentangtech/bot-order/internal/repository/xui"
)

func TestProvisionClient_GivenPurchase_ThenSubIDSentAndReturned(t *testing.T) {
	panel := &fakePanel{inbounds: []xui.Inbound{{
		ID: 1, Protocol: "vless", Enable: true, Port: 443,
		Settings: `{"clients":[]}`,
	}}}
	box := testBox(t)
	svc := New(&fakeServerStore{byID: &postgres.VPNServer{
		ID: 9, Name: "ID-01", Host: "h", Port: 1, Username: "u",
		PasswordEnc: mustEncrypt(t, box, "p"), APIPath: "/",
	}}, box, nil)
	svc.panelFactory = func(context.Context, int64) (inboundLister, error) { return panel, nil }

	pc, err := svc.CreateClient(context.Background(), 9, 1, "a@vpn.kt", "vless", 30, 10, 1)
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	if pc.SubID == "" {
		t.Fatal("SubID must be returned for the subscription URL (FR-13)")
	}
	if panel.added == nil || panel.added.Client.SubID != pc.SubID {
		got := ""
		if panel.added != nil {
			got = panel.added.Client.SubID
		}
		t.Errorf("panel subId = %q, want returned SubID %q", got, pc.SubID)
	}
}
