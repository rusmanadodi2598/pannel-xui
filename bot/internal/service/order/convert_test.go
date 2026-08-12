// Package ordersvc_test covers the row converters (M7 fix regression).
//
// @file      internal/service/order/convert_test.go
// @for       Zero-value (0) ServerID/ClientID must map to NULL, not 0 — a 0
// foreign key violates orders_client_id_fkey (staging bug v1.24).
// @uses      testing, internal/domain
// @reason    Orders are created BEFORE the client row exists; inserting 0 into
// a nullable FK column is a Postgres FK violation (SQLSTATE 23503).
//
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package ordersvc

import (
	"testing"

	"github.com/kentangtech/bot-order/internal/domain"
)

func TestToOrderRow_GivenZeroClientID_ThenNilNotZero(t *testing.T) {
	order := domain.NewOrder("KTS-1-VPN", domain.OrderTypePurchase, 9, 1, "vless", 30, 7000)
	// ClientID 0: client row does not exist yet — must persist as NULL.
	row := toOrderRow(order)
	if row.ClientID != nil {
		t.Errorf("ClientID = %v, want nil (0 → NULL, FK-safe)", row.ClientID)
	}
}

func TestToOrderRow_GivenClientID_ThenPointer(t *testing.T) {
	order := domain.NewOrder("KTS-1-VPN", domain.OrderTypePurchase, 9, 1, "vless", 30, 7000)
	order.ClientID = 7 // set after the client row is created
	row := toOrderRow(order)
	if row.ClientID == nil || *row.ClientID != 7 {
		t.Errorf("ClientID = %v, want pointer to 7", row.ClientID)
	}
}

func TestToOrderRow_GivenServerID_ThenPointer(t *testing.T) {
	order := domain.NewOrder("KTS-1-VPN", domain.OrderTypePurchase, 9, 5, "vless", 30, 7000)
	row := toOrderRow(order)
	if row.ServerID == nil || *row.ServerID != 5 {
		t.Errorf("ServerID = %v, want pointer to 5", row.ServerID)
	}
}

func TestToClientRow_GivenConfigLink_ThenPersisted(t *testing.T) {
	// M7 regression: the share URI must reach vpn_clients.config_link so the
	// detail view / .txt export still work after a restart.
	client, err := domain.NewVPNClient(9, 1, 4, "u@vpn.kt", "vless", "uuid-1", "", 30, 1, 1)
	if err != nil {
		t.Fatalf("NewVPNClient: %v", err)
	}
	client.ConfigLink = "vless://uuid-1@h:443?security=reality"
	row := toClientRow(client)
	if row.ConfigLink != client.ConfigLink {
		t.Errorf("ConfigLink = %q, want %q", row.ConfigLink, client.ConfigLink)
	}
}

func TestToClientRow_GivenInboundStream_ThenPersisted(t *testing.T) {
	// v1.27: the real transport (ws/grpc + path) must survive restarts so the
	// dual config links use the dynamic per-inbound path, not /{protocol}.
	client, err := domain.NewVPNClient(9, 1, 6, "u@vpn.kt", "vless", "uuid-1", "", 30, 1, 1)
	if err != nil {
		t.Fatalf("NewVPNClient: %v", err)
	}
	client.InboundNetwork = "ws"
	client.InboundPath = "/vlessws"
	row := toClientRow(client)
	if row.InboundNetwork != "ws" || row.InboundPath != "/vlessws" {
		t.Errorf("inbound stream = (%q, %q), want (ws, /vlessws)", row.InboundNetwork, row.InboundPath)
	}
}
