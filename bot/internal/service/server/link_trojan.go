// Package serversvc also hosts the Trojan share-link generator.
//
// @file      internal/service/server/link_trojan.go
// @for       trojan:// URI builder (tcp/http, kcp, ws, grpc, httpupgrade, xhttp;
// tls + reality) — port of sub/subService.go genTrojanLink.
// @uses      internal/repository/xui
// @reason    Faithful to the panel output; externalProxy branch dropped.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package serversvc

import (
	"github.com/kentangtech/bot-order/internal/repository/xui"
)

func genTrojanLink(inbound xui.Inbound, address string, cred ClientCred) string {
	if inbound.Protocol != "trojan" {
		return ""
	}
	stream := decodeMap(inbound.StreamSettings)
	network := str(stream, "network")
	params := map[string]string{"type": network}

	switch network {
	case "tcp":
		if tcpHeaderType(stream) == "http" {
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
		params["spx"] = "/spx"
	default:
		params["security"] = "none"
	}

	return buildURI("trojan", cred.Password, address, inbound.Port, params, remark(inbound, ""))
}
