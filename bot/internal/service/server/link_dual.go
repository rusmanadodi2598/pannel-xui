// Package serversvc also hosts the TLS/non-TLS WebSocket config pair builder.
//
// @file      internal/service/server/link_dual.go
// @for       DualConfigLinks: ws/grpc config pair (TLS 443 / non-TLS 80) with
// the REAL per-inbound path from streamSettings (v1.27 dynamic path).
// @uses      encoding/base64, encoding/json, strconv, strings
// @reason    The reference client-vpn hardcoded /{protocol}; the panel's real
// ws/grpc paths differ (/vlessws, trojan-grpc, …) — the actual API
// (streamSettings) carries them, so the dual link uses them per inbound.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package serversvc

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
)

// ConfigPair is the TLS/non-TLS WebSocket config pair of one account.
type ConfigPair struct {
	TLS  string
	NTLS string
}

// Has reports whether at least one link is present.
func (p ConfigPair) Has() bool { return p.TLS != "" || p.NTLS != "" }

// InboundStream extracts the transport network + path of an inbound from its
// streamSettings JSON (the actual API source — dynamic path per inbound).
// Path is wsSettings.path for ws, grpcSettings.serviceName for grpc, and the
// path for httpupgrade/xhttp (kept for completeness, no dual variant).
func InboundStream(streamSettings string) (network, path string) {
	m := decodeMap(streamSettings)
	if m == nil {
		return "", ""
	}
	network = str(m, "network")
	switch network {
	case "ws":
		ws, _ := m["wsSettings"].(map[string]any)
		path = str(ws, "path")
	case "grpc":
		grpc, _ := m["grpcSettings"].(map[string]any)
		path = str(grpc, "serviceName")
	case "httpupgrade":
		hu, _ := m["httpupgradeSettings"].(map[string]any)
		path = str(hu, "path")
	case "xhttp":
		xh, _ := m["xhttpSettings"].(map[string]any)
		path = str(xh, "path")
	}
	return network, path
}

// DualConfigLinks builds the ws/grpc config pair for an account (reference
// format): host is the panel's public domain, network + path mirror the real
// inbound transport (dynamic path per inbound — v1.27). Legacy rows without a
// stored network fall back to ws /{protocol} (v1.26 links keep working).
// Only vless/vmess/trojan over ws or grpc get a pair; reality (tcp),
// shadowsocks and hysteria return an empty pair → native ConfigLink fallback.
func DualConfigLinks(protocol, host, uuid, password, email, network, path string) ConfigPair {
	switch strings.ToLower(protocol) {
	case "vless", "vmess", "trojan":
	default:
		return ConfigPair{}
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return ConfigPair{}
	}
	net := strings.ToLower(strings.TrimSpace(network))
	if net == "" {
		net = "ws" // legacy row (pre-v1.27): keep the ws /{protocol} links
	}
	switch net {
	case "ws", "grpc":
	default:
		return ConfigPair{} // tcp (reality), hysteria, ss: no ws/grpc variant
	}
	remark := RemarkEmail(email)
	cred := uuid
	if strings.EqualFold(protocol, "trojan") {
		cred = password
	}
	return ConfigPair{
		TLS:  dualLink(protocol, cred, host, 443, "tls", remark, net, path),
		NTLS: dualLink(protocol, cred, host, 80, "none", remark, net, path),
	}
}

// dualLink builds one variant: vmess is base64 JSON, the rest use the standard
// scheme://cred@host:port?params#tag URI (buildURI keeps +,= raw, like the
// panel generators do). ws links carry path + host; grpc links carry serviceName.
func dualLink(protocol, cred, host string, port int, security, remark, network, path string) string {
	tag := remark + "-TLS"
	if security == "none" {
		tag = remark + "-NTLS"
	}
	if strings.EqualFold(protocol, "vmess") {
		obj := map[string]any{
			"v": "2", "ps": tag, "add": host, "port": strconv.Itoa(port),
			"id": cred, "aid": "0", "scy": "auto", "net": network,
			"host": host, "path": vmessPath(network, path, protocol),
			"tls": security, "alpn": "", "fp": "",
		}
		if network == "grpc" {
			obj["type"] = ""
		} else {
			obj["type"] = "none"
		}
		if security == "tls" {
			obj["sni"] = host
		} else {
			obj["sni"] = ""
		}
		b, err := json.Marshal(obj)
		if err != nil {
			return ""
		}
		return "vmess://" + base64.StdEncoding.EncodeToString(b)
	}
	params := map[string]string{
		"security": security,
		"type":     network,
	}
	if network == "grpc" {
		params["serviceName"] = grpcServiceName(path, protocol)
	} else {
		params["path"] = wsPath(path, protocol)
		params["host"] = host
	}
	if security == "tls" {
		params["sni"] = host
	}
	if strings.EqualFold(protocol, "vless") {
		params["encryption"] = "none"
	}
	return buildURI(strings.ToLower(protocol), cred, host, port, params, tag)
}

// wsPath normalizes a ws path to a leading-slash form (fallback /{protocol}).
func wsPath(path, protocol string) string {
	p := strings.TrimSpace(path)
	if p == "" {
		return "/" + strings.ToLower(protocol)
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

// grpcServiceName normalizes a gRPC service name to a slash-free form
// (fallback = protocol).
func grpcServiceName(path, protocol string) string {
	p := strings.TrimSpace(path)
	if p == "" {
		return strings.ToLower(protocol)
	}
	return strings.TrimPrefix(p, "/")
}

// vmessPath picks the right path form for the vmess JSON (grpc has no slash).
func vmessPath(network, path, protocol string) string {
	if network == "grpc" {
		return grpcServiceName(path, protocol)
	}
	return wsPath(path, protocol)
}

// RemarkEmail shortens the email into the link/YAML remark (reference rule):
// "trial_829710_abcd1234@…" → "trial-abcd1234", otherwise the local part.
// Exported so the Clash YAML convert view (telegram package) names its proxies
// exactly like the dual config links (FR-08 AC-2).
func RemarkEmail(email string) string {
	local := email
	if i := strings.IndexByte(email, '@'); i >= 0 {
		local = email[:i]
	}
	if strings.HasPrefix(local, "trial_") {
		parts := strings.Split(local, "_")
		return "trial-" + parts[len(parts)-1] // reference always keeps the last part
	}
	return local
}
