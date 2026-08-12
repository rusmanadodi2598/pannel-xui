// Package serversvc also hosts the VMess share-link generator.
//
// @file      internal/service/server/link_vmess.go
// @for       vmess:// URI builder (base64 JSON) — port of sub/subService.go
// genVmessLink, minus externalProxy.
// @uses      encoding/base64, encoding/json, internal/repository/xui
// @reason    VMess share links are base64-encoded JSON; every field mirrors the
// panel's output so client apps import them unchanged.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package serversvc

import (
	"encoding/base64"
	"encoding/json"

	"github.com/kentangtech/bot-order/internal/repository/xui"
)

func genVmessLink(inbound xui.Inbound, address string, cred ClientCred) string {
	if inbound.Protocol != "vmess" {
		return ""
	}
	obj := map[string]any{
		"v":    "2",
		"add":  address,
		"port": inbound.Port,
		"type": "none",
	}
	stream := decodeMap(inbound.StreamSettings)
	network := str(stream, "network")
	obj["net"] = network

	switch network {
	case "tcp":
		if t := tcpHeaderType(stream); t != "" {
			obj["type"] = t
		}
		if t := tcpHeaderType(stream); t == "http" {
			req := httpRequest(stream)
			obj["path"] = firstPath(req)
			obj["host"] = searchHost(req["headers"])
		}
	case "kcp":
		kcp, _ := stream["kcpSettings"].(map[string]any)
		hdr, _ := kcp["header"].(map[string]any)
		obj["type"], _ = hdr["type"].(string)
		obj["path"], _ = kcp["seed"].(string)
	case "ws":
		ws, _ := stream["wsSettings"].(map[string]any)
		obj["path"] = str(ws, "path")
		if h := wsHost(ws); h != "" {
			obj["host"] = h
		}
	case "grpc":
		grpc, _ := stream["grpcSettings"].(map[string]any)
		obj["path"] = str(grpc, "serviceName")
		obj["authority"] = str(grpc, "authority")
		if mm, ok := grpc["multiMode"].(bool); ok && mm {
			obj["type"] = "multi"
		}
	case "httpupgrade":
		hu, _ := stream["httpupgradeSettings"].(map[string]any)
		obj["path"] = str(hu, "path")
		if h := streamHost(hu); h != "" {
			obj["host"] = h
		}
	case "xhttp":
		xh, _ := stream["xhttpSettings"].(map[string]any)
		obj["path"] = str(xh, "path")
		if h := streamHost(xh); h != "" {
			obj["host"] = h
		}
		obj["mode"] = str(xh, "mode")
		if extra := buildXhttpExtra(xh); extra != nil {
			obj["extra"] = extra
		}
	}

	security := str(stream, "security")
	obj["tls"] = security
	if security == "tls" {
		tlsSetting, _ := stream["tlsSettings"].(map[string]any)
		if a := alpnCSV(strSlice(tlsSetting, "alpn")); a != "" {
			obj["alpn"] = a
		}
		if sni, ok := searchKey(tlsSetting, "serverName"); ok {
			obj["sni"], _ = sni.(string)
		}
		inner, _ := searchKey(tlsSetting, "settings")
		if tlsSetting != nil {
			if fp, ok := searchKey(inner, "fingerprint"); ok {
				obj["fp"], _ = fp.(string)
			}
			if insecure, ok := searchKey(inner, "allowInsecure"); ok {
				obj["allowInsecure"], _ = insecure.(bool)
			}
			if pcs := pinnedPeerCertSha256ToString(inner); pcs != "" {
				obj["pcs"] = pcs
			}
			if vcn, ok := searchKey(inner, "verifyPeerCertByName"); ok {
				if s, ok := vcn.(string); ok && s != "" {
					obj["vcn"] = s
				}
			}
		}
	}

	obj["id"] = cred.UUID
	obj["ps"] = remark(inbound, "")
	b, err := json.Marshal(obj)
	if err != nil {
		return ""
	}
	return "vmess://" + base64.StdEncoding.EncodeToString(b)
}
