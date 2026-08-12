// Package serversvc_test covers the share-link generators (M7 detail/export).
//
// @file      internal/service/server/linkgen_test.go
// @for       ShareLink output against REAL panel fixtures (id2 staging) so a
// panel config change that breaks imports fails here first.
// @uses      testing, encoding/base64, encoding/json, strings, internal/repository/xui
// @reason    The bot generates the same share URIs the panel's /sub/ endpoint
// returns — the tests lock that contract (AGENTS.md §2.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package serversvc

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kentangtech/bot-order/internal/repository/xui"
)

const stagingHost = "id2.kentangtechstore.net"

// Fixtures are verbatim copies of the staging panel's inbound settings.
var (
	fixtureVlessReality = xui.Inbound{
		ID: 1, Protocol: "vless", Port: 20001, Remark: "VLESS-REALITY",
		Settings: `{"clients":[],"decryption":"none","encryption":"none","fallbacks":[]}`,
		StreamSettings: `{"network":"tcp","security":"reality","externalProxy":[],"realitySettings":{
			"serverNames":["microsoft.com","www.microsoft.com"],
			"shortIds":["a1b2c3d4"],
			"settings":{"publicKey":"VPLO7Mn-Q4nRACF9ppMujys3YH1EUozGgaV4-WbqEzE","fingerprint":"chrome","serverName":"microsoft.com"}},
			"tcpSettings":{"header":{"type":"none"}}}`,
	}
	fixtureVmessWS = xui.Inbound{
		ID: 2, Protocol: "vmess", Port: 20002, Remark: "VMess-WS",
		Settings:       `{"clients":[]}`,
		StreamSettings: `{"network":"ws","security":"none","externalProxy":[],"wsSettings":{"path":"/vmessws","host":"id2.kentangtechstore.net","headers":{"Host":"id2.kentangtechstore.net"}}}`,
	}
	fixtureTrojanGRPC = xui.Inbound{
		ID: 3, Protocol: "trojan", Port: 20003, Remark: "Trojan-gRPC",
		Settings:       `{"clients":[],"fallbacks":[]}`,
		StreamSettings: `{"network":"grpc","security":"none","externalProxy":[],"grpcSettings":{"serviceName":"trojan-grpc","multiMode":false}}`,
	}
	fixtureSS2022 = xui.Inbound{
		ID: 4, Protocol: "shadowsocks", Port: 20004, Remark: "Shadowsocks-2022",
		Settings:       `{"method":"2022-blake3-aes-256-gcm","password":"RKGCce+E1Eo7xA+Zs55Vns82s+i7lwQp7DHmCLm+xqk=","network":"tcp,udp","clients":[]}`,
		StreamSettings: `{"network":"tcp","security":"none","externalProxy":[],"tcpSettings":{"header":{"type":"none"}}}`,
	}
	fixtureHysteria2 = xui.Inbound{
		ID: 5, Protocol: "hysteria", Port: 20005, Remark: "Hysteria2-UDP",
		Settings: `{"version":2,"clients":[]}`,
		StreamSettings: `{"network":"hysteria","security":"tls","externalProxy":[],
			"tlsSettings":{"serverName":"id2.kentangtechstore.net","alpn":["h3"],
				"settings":{"allowInsecure":false,"fingerprint":"chrome","pinnedPeerCertSha256":[]}}}`,
	}
)

func TestShareLink_GivenVlessReality_ThenRealityParams(t *testing.T) {
	link := ShareLink("vless", fixtureVlessReality, stagingHost, ClientCred{UUID: "u1", Flow: "xtls-rprx-vision"})
	for _, want := range []string{
		"vless://u1@" + stagingHost + ":20001",
		"security=reality", "sni=microsoft.com", "pbk=VPLO7Mn-Q4nRACF9ppMujys3YH1EUozGgaV4-WbqEzE",
		"sid=a1b2c3d4", "fp=chrome", "flow=xtls-rprx-vision", "#VLESS-REALITY",
	} {
		if !strings.Contains(link, want) {
			t.Errorf("vless link %q missing %q", link, want)
		}
	}
}

func TestShareLink_GivenVmessWS_ThenBase64JSON(t *testing.T) {
	link := ShareLink("vmess", fixtureVmessWS, stagingHost, ClientCred{UUID: "u2"})
	if !strings.HasPrefix(link, "vmess://") {
		t.Fatalf("link = %q, want vmess:// prefix", link)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(link, "vmess://"))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for k, want := range map[string]any{
		"add": stagingHost, "port": float64(20002), "net": "ws", "path": "/vmessws",
		"host": stagingHost, "tls": "none", "id": "u2", "ps": "VMess-WS",
	} {
		if obj[k] != want {
			t.Errorf("vmess obj[%s] = %v, want %v", k, obj[k], want)
		}
	}
}

func TestShareLink_GivenTrojanGRPC_ThenGRPCParams(t *testing.T) {
	link := ShareLink("trojan", fixtureTrojanGRPC, stagingHost, ClientCred{Password: "pw3"})
	for _, want := range []string{
		"trojan://pw3@" + stagingHost + ":20003",
		"type=grpc", "serviceName=trojan-grpc", "security=none", "#Trojan-gRPC",
	} {
		if !strings.Contains(link, want) {
			t.Errorf("trojan link %q missing %q", link, want)
		}
	}
}

func TestShareLink_GivenSS2022_ThenThreePartSecret(t *testing.T) {
	link := ShareLink("shadowsocks", fixtureSS2022, stagingHost, ClientCred{Password: "client-secret"})
	if !strings.HasPrefix(link, "ss://") || !strings.Contains(link, "@"+stagingHost+":20004") {
		t.Fatalf("ss link = %q", link)
	}
	if !strings.Contains(link, "#Shadowsocks-2022") {
		t.Errorf("ss link missing remark fragment: %q", link)
	}
	userinfo := link[len("ss://"):strings.Index(link, "@")]
	raw, err := base64.StdEncoding.DecodeString(userinfo)
	if err != nil {
		t.Fatalf("decode userinfo: %v", err)
	}
	got := string(raw)
	want := "2022-blake3-aes-256-gcm:RKGCce+E1Eo7xA+Zs55Vns82s+i7lwQp7DHmCLm+xqk=:client-secret"
	if got != want {
		t.Errorf("encPart = %q, want %q", got, want)
	}
}

func TestShareLink_GivenHysteria2_ThenTLSParams(t *testing.T) {
	link := ShareLink("hysteria", fixtureHysteria2, stagingHost, ClientCred{Auth: "auth5"})
	for _, want := range []string{
		"hysteria2://auth5@" + stagingHost + ":20005",
		"security=tls", "sni=" + stagingHost, "alpn=h3", "fp=chrome", "#Hysteria2-UDP",
	} {
		if !strings.Contains(link, want) {
			t.Errorf("hysteria link %q missing %q", link, want)
		}
	}
	if strings.Contains(link, "hysteria://") {
		t.Errorf("version 2 must use hysteria2 scheme: %q", link)
	}
}

func TestShareLink_GivenUnknownProtocol_ThenEmpty(t *testing.T) {
	if got := ShareLink("wireguard", fixtureVlessReality, stagingHost, ClientCred{}); got != "" {
		t.Errorf("unknown protocol link = %q, want empty", got)
	}
	if got := ShareLink("vless", xui.Inbound{}, stagingHost, ClientCred{UUID: "u"}); got != "" {
		t.Errorf("wrong-protocol inbound link = %q, want empty", got)
	}
}
