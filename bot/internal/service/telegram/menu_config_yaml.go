// Package telegram also hosts the Clash/Meta YAML convert view (FR-08 AC-2).
//
// @file      internal/service/telegram/menu_config_yaml.go
// @for       Convert Config YAML: Clash/Meta proxy blocks (TLS 443 / NTLS 80).
// @uses      fmt, strconv, strings, github.com/go-telegram/bot/models,
// internal/repository/postgres, internal/service/server
// @reason    AC-2 parity (reference build_yaml_configs) ported to the Go bot
// with the REAL per-inbound transport (dynamic ws path / grpc serviceName,
// v1.27) and the real trojan password (reference quirk pakai uuid diperbaiki).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	serversvc "github.com/kentangtech/bot-order/internal/service/server"
)

// clashYAMLBlock builds one Clash/Meta proxy block for the account. The tag
// uses the same remark rule as the dual config links, so the YAML proxy name
// matches the v2ray URL remark. Transport follows the REAL inbound network
// (ws path / grpc service name — v1.27), with the ws /{protocol} fallback for
// legacy rows. Trojan carries its real password, not the UUID (fix atas quirk
// reference build_yaml_configs yang salah pakai uuid).
func clashYAMLBlock(protocol, host, uuid, password, network, path, tag string, port int, tlsEnabled bool) string {
	proto := strings.ToLower(protocol)
	net := strings.ToLower(strings.TrimSpace(network))
	if net == "" {
		net = "ws" // legacy row (pre-v1.27): link tetap ws /{protocol}
	}
	lines := []string{
		"proxies:",
		"- name: " + tag,
		"  server: " + host,
		"  port: " + strconv.Itoa(port),
		"  type: " + proto,
	}
	if proto == "trojan" {
		lines = append(lines, "  password: "+password)
	} else {
		lines = append(lines, "  uuid: "+uuid)
	}
	if proto == "vmess" {
		lines = append(lines, "  alterId: 0", "  cipher: auto")
	}
	if tlsEnabled {
		lines = append(lines, "  tls: true", "  servername: "+host)
	} else {
		lines = append(lines, "  tls: false")
	}
	lines = append(lines, "  skip-cert-verify: true")
	if net == "grpc" {
		lines = append(lines, "  network: grpc", "  grpc-opts:", "    grpc-service-name: "+grpcLabel(path, protocol))
	} else {
		lines = append(lines, "  network: ws", "  ws-opts:", "    path: "+wsLabel(path, protocol), "    headers:", "      Host: "+host)
	}
	lines = append(lines, "  udp: true")
	return strings.Join(lines, "\n")
}

// AccountConvertText renders the Clash/Meta YAML convert page (FR-08 AC-2):
// two proxy blocks (TLS 443 / Non-TLS 80) for ws/grpc vless/vmess/trojan.
// Protocols without a ws/grpc variant (reality tcp, shadowsocks, hysteria)
// fall back to the native ConfigLink with a note — no fake ws YAML (sama
// dengan aturan dual link v1.27).
func AccountConvertText(c postgres.ClientView) string {
	var b strings.Builder
	b.WriteString("Convert Config YAML\n")
	b.WriteString("━━━━━━━━━━━━━━\n")
	b.WriteString(fmt.Sprintf("Akun: %s\n", c.Email))
	b.WriteString(fmt.Sprintf("Protokol: %s\n", strings.ToUpper(c.Protocol)))
	b.WriteString(fmt.Sprintf("Server: %s\n", c.ServerHost))
	pair := AccountConfigLinks(c)
	if !pair.Has() {
		b.WriteString("━━━━━━━━━━━━━━\n")
		b.WriteString(fmt.Sprintf("Config YAML Clash/Meta tidak tersedia untuk %s.\n", strings.ToUpper(c.Protocol)))
		// v1.36: URL tidak ditampilkan di menu — cukup di Ekspor .txt.
		b.WriteString("URL import lengkap: gunakan tombol Ekspor .txt.\n")
		return b.String()
	}
	remark := serversvc.RemarkEmail(c.Email)
	b.WriteString("━━━━━━━━━━━━━━\n")
	b.WriteString("YAML Config TLS (Port 443):\n")
	b.WriteString(clashYAMLBlock(c.Protocol, c.ServerHost, c.UUID, c.Password, c.InboundNetwork, c.InboundPath, remark+"-TLS", 443, true))
	b.WriteString("\n\n━━━━━━━━━━━━━━\n")
	b.WriteString("YAML Config Non-TLS (Port 80):\n")
	b.WriteString(clashYAMLBlock(c.Protocol, c.ServerHost, c.UUID, c.Password, c.InboundNetwork, c.InboundPath, remark+"-NTLS", 80, false))
	b.WriteString("\n\n━━━━━━━━━━━━━━\n")
	b.WriteString("Cara pakai:\n")
	b.WriteString("1. Copy salah satu config YAML di atas\n")
	b.WriteString("2. Buka Clash / Clash Meta / Stash\n")
	b.WriteString("3. Paste ke bagian proxies di config\n")
	b.WriteString("\nTips:\n")
	b.WriteString("• Format ini untuk Clash, Clash Meta, Stash\n")
	b.WriteString("• TLS (443) — lebih aman, disarankan\n")
	b.WriteString("• Non-TLS (80) — jika TLS diblokir ISP\n")
	return b.String()
}

// AccountConvertKeyboard offers the config view + back/home navigation
// (FR-08 AC-2; button relabeled "Config V2Ray" — view no longer shows URLs,
// v1.36).
func AccountConvertKeyboard(clientID int64) models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(
		models.InlineKeyboardButton{Text: "Config V2Ray", CallbackData: PrefixAccountConfig + fmt.Sprintf("%d", clientID)},
		backBtn(PrefixAccountView+fmt.Sprintf("%d", clientID), "⬅️ Kembali"),
		backBtn(CallbackHome, "🏠 Menu Utama"),
	)}
}
