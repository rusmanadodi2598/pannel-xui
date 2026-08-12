// Package serversvc also hosts the Hysteria share-link generator.
//
// @file      internal/service/server/link_hysteria.go
// @for       hysteria:// + hysteria2:// URI builder (version from settings;
// tls; finalmask quic/obfs params) — port of sub/subService.go genHysteriaLink.
// @uses      fmt, strconv, internal/repository/xui
// @reason    The panel stores protocol "hysteria" for both; settings.version
// picks the link scheme. QUIC/obfs params stay optional (empty → omitted).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package serversvc

import (
	"fmt"
	"strconv"

	"github.com/kentangtech/bot-order/internal/repository/xui"
)

func genHysteriaLink(inbound xui.Inbound, address string, cred ClientCred) string {
	if inbound.Protocol != "hysteria" {
		return ""
	}
	settings := decodeMap(inbound.Settings)
	scheme := "hysteria2"
	if v, ok := settings["version"].(float64); ok && int(v) == 1 {
		scheme = "hysteria"
	}

	stream := decodeMap(inbound.StreamSettings)
	params := map[string]string{"security": "tls"}
	tlsSetting, _ := stream["tlsSettings"].(map[string]any)
	if a := alpnCSV(strSlice(tlsSetting, "alpn")); a != "" {
		params["alpn"] = a
	}
	if sni, ok := searchKey(tlsSetting, "serverName"); ok {
		params["sni"], _ = sni.(string)
	}
	inner, _ := searchKey(tlsSetting, "settings")
	if tlsSetting != nil {
		if fp, ok := searchKey(inner, "fingerprint"); ok {
			params["fp"], _ = fp.(string)
		}
		if insecure, ok := searchKey(inner, "allowInsecure"); ok {
			if b, ok := insecure.(bool); ok && b {
				params["insecure"] = "1"
			}
		}
	}

	// finalmask: QUIC tuning + UDP obfuscation (optional, omitted when unset).
	if fm, ok := stream["finalmask"].(map[string]any); ok {
		if qp, ok := fm["quicParams"].(map[string]any); ok {
			applyQUICParams(qp, params)
		}
		if udpMasks, ok := fm["udp"].([]any); ok {
			for _, m := range udpMasks {
				mask, _ := m.(map[string]any)
				msettings, _ := mask["settings"].(map[string]any)
				password := str(msettings, "password")
				maskType := str(mask, "type")
				if password != "" && maskType != "" {
					params["obfs"] = maskType
					params["obfs-password"] = password
					break
				}
			}
		}
	}

	return buildURI(scheme, cred.Auth, address, inbound.Port, params, remark(inbound, ""))
}

// applyQUICParams copies the tuned QUIC values into the link params.
func applyQUICParams(qp map[string]any, params map[string]string) {
	if v := str(qp, "congestion"); v != "" {
		params["congestion"] = v
	}
	if v := str(qp, "brutalUp"); v != "" {
		params["up"] = v
	}
	if v := str(qp, "brutalDown"); v != "" {
		params["down"] = v
	}
	if udpHop, ok := qp["udpHop"].(map[string]any); ok {
		if v := str(udpHop, "ports"); v != "" {
			params["mport"] = v
		}
		switch iv := udpHop["interval"].(type) {
		case string:
			if iv != "" {
				params["udphopInterval"] = iv
			}
		case float64:
			params["udphopInterval"] = strconv.Itoa(int(iv))
		}
	}
	// QUIC window tuning only when explicitly set (values are ms or bytes).
	for jsonKey, paramKey := range map[string]string{
		"initStreamReceiveWindow":     "initStreamReceiveWindow",
		"maxStreamReceiveWindow":      "maxStreamReceiveWindow",
		"initConnectionReceiveWindow": "initConnectionReceiveWindow",
		"maxConnectionReceiveWindow":  "maxConnectionReceiveWindow",
		"maxIdleTimeout":              "maxIdleTimeout",
		"keepAlivePeriod":             "keepAlivePeriod",
	} {
		if v, ok := qp[jsonKey].(float64); ok && v != 0 {
			params[paramKey] = fmt.Sprintf("%d", int(v))
		}
	}
	if v, ok := qp["disablePathMTUDiscovery"].(bool); ok && v {
		params["disablePathMTUDiscovery"] = "true"
	}
}
