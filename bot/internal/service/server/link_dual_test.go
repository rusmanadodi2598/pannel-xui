// Package serversvc test covers the dual config pair generator (v1.26/v1.27).
//
// @file      internal/service/server/link_dual_test.go
// @for       DualConfigLinks: dynamic per-inbound path (ws/grpc), TLS/NTLS
// shape, legacy fallback + InboundStream extraction from streamSettings.
// @uses      testing, encoding/base64, encoding/json, strings
// @reason    The config pair is user-facing contract (reference client-vpn
// format + v1.27 dynamic path) — regressions break v2rayNG imports.
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
)

const dualHost = "id2.kentangtechstore.net"

func TestDualConfigLinks_GivenWSRealPath_ThenDynamicPathInPair(t *testing.T) {
	p := DualConfigLinks("vless", dualHost, "uuid-1", "", "kts-abcd1234", "ws", "/vlessws")
	if !p.Has() {
		t.Fatal("want a pair")
	}
	if !strings.Contains(p.TLS, "vless://uuid-1@"+dualHost+":443?") {
		t.Errorf("TLS host:port wrong: %s", p.TLS)
	}
	for _, want := range []string{
		"security=tls", "encryption=none", "type=ws",
		"host=" + dualHost, "path=%2Fvlessws",
		"sni=" + dualHost, "kts-abcd1234-TLS",
	} {
		if !strings.Contains(p.TLS, want) {
			t.Errorf("TLS missing %q in %s", want, p.TLS)
		}
	}
	if !strings.Contains(p.NTLS, dualHost+":80?") || !strings.Contains(p.NTLS, "security=none") ||
		strings.Contains(p.NTLS, "sni=") {
		t.Errorf("NTLS must be port 80 security=none without sni: %s", p.NTLS)
	}
}

func TestDualConfigLinks_GivenWSWithoutSlash_ThenNormalized(t *testing.T) {
	p := DualConfigLinks("trojan", dualHost, "", "pw-1", "kts-abcd5678", "ws", "trojanws")
	if !strings.Contains(p.TLS, "path=%2Ftrojanws") {
		t.Errorf("path not slash-normalized: %s", p.TLS)
	}
}

func TestDualConfigLinks_GivenLegacyNoNetwork_ThenWSProtocolPathFallback(t *testing.T) {
	// Pre-v1.27 rows have empty network/path — keep the ws /{protocol} links.
	p := DualConfigLinks("vless", dualHost, "uuid-1", "", "kts-abcd1234", "", "")
	if !strings.Contains(p.TLS, "path=%2Fvless") || !strings.Contains(p.TLS, "type=ws") {
		t.Errorf("legacy fallback wrong: %s", p.TLS)
	}
	if !p.Has() {
		t.Error("legacy row must keep dual links")
	}
}

func TestDualConfigLinks_GivenGRPC_ThenServiceNamePair(t *testing.T) {
	p := DualConfigLinks("trojan", dualHost, "", "pw-123", "kts-abcd9999", "grpc", "trojan-grpc")
	if !strings.Contains(p.TLS, "trojan://pw-123@"+dualHost+":443?") ||
		!strings.Contains(p.TLS, "type=grpc") || !strings.Contains(p.TLS, "serviceName=trojan-grpc") ||
		!strings.Contains(p.TLS, "sni="+dualHost) {
		t.Errorf("grpc TLS wrong: %s", p.TLS)
	}
	if !strings.Contains(p.NTLS, "serviceName=trojan-grpc") || strings.Contains(p.NTLS, "sni=") {
		t.Errorf("grpc NTLS wrong: %s", p.NTLS)
	}
}

func TestDualConfigLinks_GivenGRPCVmess_ThenBase64JSONPair(t *testing.T) {
	p := DualConfigLinks("vmess", dualHost, "uuid-2", "", "kts-abcd5678", "grpc", "vmess-grpc")
	decode := func(link string) map[string]any {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(link, "vmess://"))
		if err != nil {
			t.Fatalf("decode %q: %v", link, err)
		}
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err != nil {
			t.Fatalf("json: %v", err)
		}
		return obj
	}
	tls := decode(p.TLS)
	if tls["net"] != "grpc" || tls["path"] != "vmess-grpc" || tls["port"] != "443" || tls["tls"] != "tls" {
		t.Errorf("grpc TLS vmess obj = %v", tls)
	}
	ntls := decode(p.NTLS)
	if ntls["port"] != "80" || ntls["tls"] != "none" || ntls["sni"] != "" {
		t.Errorf("grpc NTLS vmess obj = %v", ntls)
	}
}

func TestDualConfigLinks_GivenTCPReality_ThenEmptyPair(t *testing.T) {
	// VLESS-REALITY (network tcp) must NOT get a fake ws link — native only.
	if p := DualConfigLinks("vless", dualHost, "uuid-1", "", "kts-x", "tcp", ""); p.Has() {
		t.Errorf("tcp/reality: want empty pair, got %+v", p)
	}
	for _, proto := range []string{"shadowsocks", "hysteria", "hysteria2"} {
		if p := DualConfigLinks(proto, dualHost, "u", "", "kts-x", "tcp", ""); p.Has() {
			t.Errorf("%s: want empty pair, got %+v", proto, p)
		}
	}
}

func TestDualConfigLinks_GivenEmptyHost_ThenEmptyPair(t *testing.T) {
	if p := DualConfigLinks("vless", "", "u", "", "kts-x", "ws", "/vlessws"); p.Has() {
		t.Errorf("empty host: want empty pair, got %+v", p)
	}
}

func TestDualConfigLinks_GivenTrojan_ThenPasswordCredAndTrialRemark(t *testing.T) {
	p := DualConfigLinks("trojan", dualHost, "", "pw-123", "trial_829710_abcd1234@trial.kentangtech.com", "ws", "/trojanws")
	if !strings.Contains(p.TLS, "trojan://pw-123@"+dualHost+":443?") {
		t.Errorf("TLS must use password cred: %s", p.TLS)
	}
	if !strings.Contains(p.TLS, "trial-abcd1234-TLS") {
		t.Errorf("trial remark not shortened: %s", p.TLS)
	}
}

func TestInboundStream_GivenRealPanelStreamSettings_ThenNetworkAndPath(t *testing.T) {
	// Fixture = streamSettings asli panel id2 (VLESS-WS inbound 20006).
	const wsStream = `{"network":"ws","security":"none","wsSettings":{"path":"/vlessws","headers":{"Host":"id2.kentangtechstore.net"}}}`
	net, path := InboundStream(wsStream)
	if net != "ws" || path != "/vlessws" {
		t.Errorf("InboundStream(ws) = (%q, %q), want (ws, /vlessws)", net, path)
	}

	// Fixture = streamSettings asli panel id2 (Trojan-gRPC inbound 20003).
	const grpcStream = `{"network":"grpc","security":"tls","grpcSettings":{"serviceName":"trojan-grpc"}}`
	net, path = InboundStream(grpcStream)
	if net != "grpc" || path != "trojan-grpc" {
		t.Errorf("InboundStream(grpc) = (%q, %q), want (grpc, trojan-grpc)", net, path)
	}

	// Reality/tcp + empty settings → no path.
	net, path = InboundStream(`{"network":"tcp","security":"reality"}`)
	if net != "tcp" || path != "" {
		t.Errorf("InboundStream(tcp) = (%q, %q)", net, path)
	}
	net, path = InboundStream("")
	if net != "" || path != "" {
		t.Errorf("InboundStream(empty) = (%q, %q)", net, path)
	}
}

func TestRemarkEmail_GivenTrialAndPlain_ThenShortened(t *testing.T) {
	if got := RemarkEmail("trial_829710_abcd1234@trial.kentangtech.com"); got != "trial-abcd1234" {
		t.Errorf("trial remark = %q", got)
	}
	if got := RemarkEmail("kts-abcd5678@vpn.kt"); got != "kts-abcd5678" {
		t.Errorf("plain remark = %q", got)
	}
	if got := RemarkEmail("kts-nolocalpart"); got != "kts-nolocalpart" {
		t.Errorf("no-@ remark = %q", got)
	}
}
