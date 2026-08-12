// Package serversvc also hosts the X-UI share-link generator.
//
// @file      internal/service/server/linkgen.go
// @for       ShareLink: builds vless/vmess/trojan/shadowsocks/hysteria URIs from
// the inbound's settings + the client credential (M7 detail/export feature).
// @uses      encoding/json, net/url, strings, internal/repository/xui
// @reason    The panel's sub server is disabled on staging, so the bot builds
// the same share links itself — a faithful port of sub/subService.go's
// generators, minus the externalProxy branch (not used on these panels).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package serversvc

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/kentangtech/bot-order/internal/repository/xui"
)

// ClientCred carries the credential fields the share link embeds — exactly one
// is set per protocol (vless/vmess → UUID, trojan/shadowsocks → Password,
// hysteria → Auth; Flow only for vless with xtls/reality).
type ClientCred struct {
	UUID     string
	Password string
	Auth     string
	Flow     string
}

// ShareLink builds the client URI for an inbound ("" when the protocol has no
// generator). The link is the exact same shape the panel's /sub/ endpoint
// returns, so every client app that imports a panel link accepts ours.
func ShareLink(protocol string, inbound xui.Inbound, address string, cred ClientCred) string {
	switch strings.ToLower(protocol) {
	case "vmess":
		return genVmessLink(inbound, address, cred)
	case "vless":
		return genVlessLink(inbound, address, cred)
	case "trojan":
		return genTrojanLink(inbound, address, cred)
	case "shadowsocks":
		return genShadowsocksLink(inbound, address, cred)
	case "hysteria", "hysteria2":
		return genHysteriaLink(inbound, address, cred)
	}
	return ""
}

// decodeMap parses the JSON blob into a map (nil when invalid/absent).
func decodeMap(raw string) map[string]any {
	m := map[string]any{}
	if raw == "" || json.Unmarshal([]byte(raw), &m) != nil {
		return nil
	}
	return m
}

// str returns the string value of a key (empty when absent/typed differently).
func str(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

// strSlice returns the []any value of a key (nil when absent).
func strSlice(m map[string]any, key string) []any {
	if m == nil {
		return nil
	}
	v, _ := m[key].([]any)
	return v
}

// searchKey finds a key recursively in a JSON tree (same contract as the
// panel's sub/subService.go helper — Reality settings nest under "settings").
func searchKey(data any, key string) (any, bool) {
	switch val := data.(type) {
	case map[string]any:
		for k, v := range val {
			if k == key {
				return v, true
			}
			if result, ok := searchKey(v, key); ok {
				return result, true
			}
		}
	case []any:
		for _, v := range val {
			if result, ok := searchKey(v, key); ok {
				return result, true
			}
		}
	}
	return nil, false
}

// searchHost extracts the first "host" header value (WebSocket/HTTP-upgrade).
func searchHost(headers any) string {
	data, _ := headers.(map[string]any)
	for k, v := range data {
		if !strings.EqualFold(k, "host") {
			continue
		}
		switch t := v.(type) {
		case []any:
			if len(t) > 0 {
				if h, ok := t[0].(string); ok {
					return h
				}
			}
		case string:
			return t
		}
	}
	return ""
}

// pinnedPeerCertSha256ToString flattens the TLS pinned-cert list to CSV.
func pinnedPeerCertSha256ToString(tlsSettings any) string {
	v, ok := searchKey(tlsSettings, "pinnedPeerCertSha256")
	if !ok {
		return ""
	}
	switch t := v.(type) {
	case []any:
		var out []string
		for _, h := range t {
			if hs, ok := h.(string); ok && len(hs) > 0 {
				out = append(out, hs)
			}
		}
		return strings.Join(out, ",")
	case string:
		return t
	}
	return ""
}

// alpnCSV joins the TLS alpn list into a comma string.
func alpnCSV(list []any) string {
	var alpn []string
	for _, a := range list {
		if s, ok := a.(string); ok {
			alpn = append(alpn, s)
		}
	}
	return strings.Join(alpn, ",")
}

// remark builds the link fragment: inbound remark then email, dash separated
// (mirrors the panel's default "-ieo" remark model without traffic info).
func remark(inbound xui.Inbound, email string) string {
	var parts []string
	if strings.TrimSpace(inbound.Remark) != "" {
		parts = append(parts, inbound.Remark)
	}
	if strings.TrimSpace(email) != "" {
		parts = append(parts, email)
	}
	return strings.Join(parts, "-")
}

// buildURI assembles scheme://cred@addr:port?query#remark.
func buildURI(scheme, cred, address string, port int, params map[string]string, fragment string) string {
	host := address
	if port > 0 {
		host += ":" + strconv.Itoa(port)
	}
	u, _ := url.Parse(scheme + "://" + cred + "@" + host)
	q := u.Query()
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	u.Fragment = fragment
	return u.String()
}
