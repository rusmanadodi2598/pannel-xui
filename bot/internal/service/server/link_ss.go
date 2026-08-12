// Package serversvc also hosts the Shadowsocks share-link generator.
//
// @file      internal/service/server/link_ss.go
// @for       ss:// URI builder (method+password; 2022 method includes the
// inbound password) — port of sub/subService.go genShadowsocksLink.
// @uses      encoding/base64, fmt, internal/repository/xui
// @reason    Shadowsocks-2022 links embed three secrets; older methods embed
// method:password — both shapes match the panel exactly.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package serversvc

import (
	"encoding/base64"
	"fmt"

	"github.com/kentangtech/bot-order/internal/repository/xui"
)

func genShadowsocksLink(inbound xui.Inbound, address string, cred ClientCred) string {
	if inbound.Protocol != "shadowsocks" {
		return ""
	}
	settings := decodeMap(inbound.Settings)
	method := str(settings, "method")
	inboundPassword := str(settings, "password")

	encPart := fmt.Sprintf("%s:%s", method, cred.Password)
	if len(method) > 0 && method[0] == '2' {
		encPart = fmt.Sprintf("%s:%s:%s", method, inboundPassword, cred.Password)
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

	if security := str(stream, "security"); security == "tls" {
		params["security"] = "tls"
		tlsSetting, _ := stream["tlsSettings"].(map[string]any)
		if a := alpnCSV(strSlice(tlsSetting, "alpn")); a != "" {
			params["alpn"] = a
		}
		if sni, ok := searchKey(tlsSetting, "serverName"); ok {
			params["sni"], _ = sni.(string)
		}
		applyTLSFingerprint(tlsSetting, params)
	}

	userinfo := base64.StdEncoding.EncodeToString([]byte(encPart))
	return buildURI("ss", userinfo, address, inbound.Port, params, remark(inbound, ""))
}
