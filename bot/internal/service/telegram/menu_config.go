// Package telegram also hosts the account v2Ray config view (v1.26).
//
// @file      internal/service/telegram/menu_config.go
// @for       Account config view: manual-config reference (server/domain/ports/
// credential/transport) — import URLs only in the .txt export (v1.36).
// @uses      fmt, strings, github.com/go-telegram/bot/models,
// internal/repository/postgres, internal/service/server
// @reason    The per-account config view renders the parameters needed for
// manual v2rayNG entry; the ready-to-import URLs live only in Ekspor .txt.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"fmt"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
	serversvc "github.com/kentangtech/bot-order/internal/service/server"
)

// AccountConfigLinks returns the dual config pair built from the inbound's
// real transport (network + path stored at provisioning — dynamic path per
// inbound, v1.27); an empty pair otherwise (callers fall back to native link).
func AccountConfigLinks(c postgres.ClientView) serversvc.ConfigPair {
	return serversvc.DualConfigLinks(c.Protocol, c.ServerHost, c.UUID, c.Password, c.Email, c.InboundNetwork, c.InboundPath)
}

// AccountConfigText renders the account's manual-config reference: the real
// inbound parameters (domain, ports, credential, transport). The ready-to-
// import URLs are intentionally NOT shown here — URL hanya di Ekspor .txt
// (v1.36 cleanup, same rule as the detail view).
func AccountConfigText(c postgres.ClientView) string {
	var b strings.Builder
	b.WriteString("Detail Konfigurasi VPN\n")
	b.WriteString("━━━━━━━━━━━━━━\n")
	b.WriteString(fmt.Sprintf("Server   : %s %s\n", flagOrGlobe(c.FlagEmoji), c.ServerName))
	b.WriteString(fmt.Sprintf("Protocol : %s\n", strings.ToUpper(c.Protocol)))
	if pair := AccountConfigLinks(c); pair.Has() {
		b.WriteString(fmt.Sprintf("Domain   : %s\n", c.ServerHost))
		b.WriteString("Port TLS : 443\n")
		b.WriteString("Port Non-TLS: 80\n")
		// Protocol-aware credential label (v1.36): UUID vless/vmess,
		// Password trojan/shadowsocks (alireza0/x-ui client fields).
		if label, cred := accountCredential(c); cred != "" {
			b.WriteString(fmt.Sprintf("%s : %s\n", label, cred))
		}
		net := strings.ToLower(strings.TrimSpace(c.InboundNetwork))
		if net == "" {
			net = "ws" // legacy row (v1.26)
		}
		b.WriteString(fmt.Sprintf("Network  : %s\n", net))
		if net == "grpc" {
			b.WriteString(fmt.Sprintf("Service Name: %s\n", grpcLabel(c.InboundPath, c.Protocol)))
		} else {
			b.WriteString(fmt.Sprintf("Path     : %s\n", wsLabel(c.InboundPath, c.Protocol)))
		}
	}
	b.WriteString("━━━━━━━━━━━━━━\n")
	b.WriteString("URL import (TLS & Non-TLS): gunakan tombol Ekspor .txt.\n")
	b.WriteString("\nTips:\n")
	b.WriteString("• TLS (443) — lebih aman, disarankan\n")
	b.WriteString("• Non-TLS (80) — jika TLS diblokir ISP\n")
	return b.String()
}

// wsLabel renders the ws path label (leading slash, fallback /{protocol}).
func wsLabel(path, protocol string) string {
	p := strings.TrimSpace(path)
	if p == "" {
		return "/" + strings.ToLower(protocol)
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

// grpcLabel renders the gRPC service-name label (slash-free, fallback protocol).
func grpcLabel(path, protocol string) string {
	p := strings.TrimSpace(path)
	if p == "" {
		return strings.ToLower(protocol)
	}
	return strings.TrimPrefix(p, "/")
}

// AccountConfigKeyboard offers back-to-detail + home navigation.
func AccountConfigKeyboard(clientID int64) models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(
		backBtn(PrefixAccountView+fmt.Sprintf("%d", clientID), "⬅️ Kembali"),
		backBtn(CallbackHome, "🏠 Menu Utama"),
	)}
}
