// Package serversvc also hosts the VLESS share-link generator.
//
// @file      internal/service/server/link_vless.go
// @for       vless:// URI builder (tcp/http, kcp, ws, grpc, httpupgrade, xhttp;
// tls + reality) — port of sub/subService.go genVlessLink.
// @uses      encoding/json, internal/repository/xui
// @reason    Faithful to the panel output so client apps accept the link; the
// externalProxy branch is dropped (not configured on these panels).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package serversvc

import (
	"encoding/json"

	"github.com/kentangtech/bot-order/internal/repository/xui"
)

func genVlessLink(inbound xui.Inbound, address string, cred ClientCred) string {
	if inbound.Protocol != "vless" {
		return ""
	}
	stream := decodeMap(inbound.StreamSettings)
	network := str(stream, "network")
	params := map[string]string{}

	if enc, ok := decodeMap(inbound.Settings)["encryption"].(string); ok && enc != "" {
		params["encryption"] = enc
	}
	params["type"] = network

	switch network {
	case "tcp":
		if typeStr := tcpHeaderType(stream); typeStr == "http" {
			req := httpRequest(stream)
			params["path"] = firstPath(req)
			params["host"] = searchHost(req["headers"])
			params["headerType"] = "http"
		}
	case "kcp":
		kcp, _ := stream["kcpSettings"].(map[string]any)
		hdr, _ := kcp["header"].(map[string]any)
		params["headerType"], _ = hdr["type"].(string)
		params["seed"], _ = kcp["seed"].(string)
	case "ws":
		ws, _ := stream["wsSettings"].(map[string]any)
		params["path"] = str(ws, "path")
		params["host"] = wsHost(ws)
	case "grpc":
		grpc, _ := stream["grpcSettings"].(map[string]any)
		params["serviceName"] = str(grpc, "serviceName")
		params["authority"] = str(grpc, "authority")
		if mm, ok := grpc["multiMode"].(bool); ok && mm {
			params["mode"] = "multi"
		}
	case "httpupgrade":
		hu, _ := stream["httpupgradeSettings"].(map[string]any)
		params["path"] = str(hu, "path")
		params["host"] = streamHost(hu)
	case "xhttp":
		xh, _ := stream["xhttpSettings"].(map[string]any)
		params["path"] = str(xh, "path")
		params["host"] = streamHost(xh)
		params["mode"] = str(xh, "mode")
		if extra := buildXhttpExtra(xh); extra != nil {
			if b, err := json.Marshal(extra); err == nil {
				params["extra"] = string(b)
			}
		}
	}

	security := str(stream, "security")
	switch security {
	case "tls":
		params["security"] = "tls"
		tlsSetting, _ := stream["tlsSettings"].(map[string]any)
		if a := alpnCSV(strSlice(tlsSetting, "alpn")); a != "" {
			params["alpn"] = a
		}
		if sni, ok := searchKey(tlsSetting, "serverName"); ok {
			params["sni"], _ = sni.(string)
		}
		applyTLSFingerprint(tlsSetting, params)
		if network == "tcp" && cred.Flow != "" {
			params["flow"] = cred.Flow
		}
	case "reality":
		params["security"] = "reality"
		reality, _ := stream["realitySettings"].(map[string]any)
		realitySettings, _ := searchKey(reality, "settings")
		if sniList := strSlice(reality, "serverNames"); len(sniList) > 0 {
			params["sni"], _ = sniList[0].(string)
		}
		if pbk, ok := searchKey(realitySettings, "publicKey"); ok {
			params["pbk"], _ = pbk.(string)
		}
		if sidList := strSlice(reality, "shortIds"); len(sidList) > 0 {
			params["sid"], _ = sidList[0].(string)
		}
		if fp, ok := searchKey(realitySettings, "fingerprint"); ok {
			if s, ok := fp.(string); ok && s != "" {
				params["fp"] = s
			}
		}
		if pqv, ok := searchKey(realitySettings, "mldsa65Verify"); ok {
			if s, ok := pqv.(string); ok && s != "" {
				params["pqv"] = s
			}
		}
		params["spx"] = "/spx" // deterministic (panel uses random seq; any works)
		if network == "tcp" && cred.Flow != "" {
			params["flow"] = cred.Flow
		}
	default:
		params["security"] = "none"
	}

	return buildURI("vless", cred.UUID, address, inbound.Port, params, remark(inbound, ""))
}

// tcpHeaderType returns the tcp header type (http → http disguised transport).
func tcpHeaderType(stream map[string]any) string {
	tcp, _ := stream["tcpSettings"].(map[string]any)
	hdr, _ := tcp["header"].(map[string]any)
	typ, _ := hdr["type"].(string)
	return typ
}

// httpRequest returns the header.request map of the tcp http disguise.
func httpRequest(stream map[string]any) map[string]any {
	tcp, _ := stream["tcpSettings"].(map[string]any)
	hdr, _ := tcp["header"].(map[string]any)
	req, _ := hdr["request"].(map[string]any)
	return req
}

// firstPath returns the first request path entry (panel uses request.path[0]).
func firstPath(req map[string]any) string {
	if p := strSlice(req, "path"); len(p) > 0 {
		if s, ok := p[0].(string); ok {
			return s
		}
	}
	return ""
}

// wsHost returns the ws host (direct field, falling back to headers).
func wsHost(ws map[string]any) string {
	if h := str(ws, "host"); h != "" {
		return h
	}
	headers, _ := ws["headers"].(map[string]any)
	return searchHost(headers)
}

// streamHost is the httpupgrade/xhttp variant of wsHost.
func streamHost(m map[string]any) string {
	if h := str(m, "host"); h != "" {
		return h
	}
	headers, _ := m["headers"].(map[string]any)
	return searchHost(headers)
}

// applyTLSFingerprint copies fp / allowInsecure / pcs / vcn from the nested
// tls settings into the params (same shape as the panel's share links).
func applyTLSFingerprint(tlsSetting map[string]any, params map[string]string) {
	inner, _ := searchKey(tlsSetting, "settings")
	if tlsSetting == nil {
		return
	}
	if fp, ok := searchKey(inner, "fingerprint"); ok {
		params["fp"], _ = fp.(string)
	}
	if insecure, ok := searchKey(inner, "allowInsecure"); ok {
		if b, ok := insecure.(bool); ok && b {
			params["allowInsecure"] = "1"
		}
	}
	if pcs := pinnedPeerCertSha256ToString(inner); pcs != "" {
		params["pcs"] = pcs
	}
	if vcn, ok := searchKey(inner, "verifyPeerCertByName"); ok {
		if s, ok := vcn.(string); ok && s != "" {
			params["vcn"] = s
		}
	}
}

// buildXhttpExtra cleans the xhttp extra object for the share link.
func buildXhttpExtra(xhttp map[string]any) map[string]any {
	if xhttp == nil {
		return nil
	}
	extra, _ := xhttp["extra"].(map[string]any)
	if len(extra) == 0 {
		return nil
	}
	cleaned := map[string]any{}
	for k, v := range extra {
		switch t := v.(type) {
		case nil:
			continue
		case string:
			if t == "" {
				continue
			}
		case bool:
			if !t {
				continue
			}
		case float64:
			if t == 0 {
				continue
			}
		case []any:
			if len(t) == 0 {
				continue
			}
		case map[string]any:
			if len(t) == 0 {
				continue
			}
		}
		cleaned[k] = v
	}
	if len(cleaned) == 0 {
		return nil
	}
	return cleaned
}
