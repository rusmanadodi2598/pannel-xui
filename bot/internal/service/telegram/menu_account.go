// Package telegram also hosts the account detail + export views (M7).
//
// @file      internal/service/telegram/menu_account.go
// @for       FR-08 detail: clean account view + Ekspor .txt document builder.
// @uses      fmt, strings, time, github.com/go-telegram/bot/models, internal/repository/postgres
// @reason    The config link (share URI) is now delivered ONLY through the
// .txt export — detail/chat views stay clean (M7 feature + v1.36 cleanup).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// AccountListKeyboard renders one "Lihat Detail" button per account (2-1-2-1
// zigzag) plus the pagination row (prev/next + non-action page indicator,
// FR-08 AC-1).
func AccountListKeyboard(clients []postgres.ClientView, page, totalPages int) models.ReplyMarkup {
	buttons := make([]models.InlineKeyboardButton, 0, len(clients))
	for _, c := range clients {
		buttons = append(buttons, models.InlineKeyboardButton{
			Text:         "Lihat Detail — " + shortEmail(c.Email),
			CallbackData: PrefixAccountView + fmt.Sprintf("%d", c.ID),
		})
	}
	rows := packRows(buttons...)
	rows = append(rows, pagerRow(PrefixAccountPage, CallbackAccountNoop, page, totalPages))
	rows = append(rows, backRow(CallbackHome, "🏠 Menu Utama"))
	return models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// AccountEmptyKeyboard offers a buy shortcut when the user has no accounts
// (parity with HistoryEmptyKeyboard — the empty prompt is not a dead end).
func AccountEmptyKeyboard() models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(
		backBtn(CallbackBuy, "Beli VPN"),
		backBtn(CallbackHome, "🏠 Menu Utama"),
	)}
}

// accountCredential returns the protocol-appropriate credential label and
// value (v1.36): UUID untuk vless/vmess, Password untuk trojan/shadowsocks
// (alireza0/x-ui client fields: id vs password; hysteria memakai auth secret).
func accountCredential(c postgres.ClientView) (label, value string) {
	switch strings.ToLower(strings.TrimSpace(c.Protocol)) {
	case "trojan", "shadowsocks", "hysteria", "hysteria2":
		return "Password", c.Password
	default:
		// Unknown protocol: prefer UUID, fall back to Password so a non-empty
		// credential is never hidden (review fix v1.36).
		if c.UUID != "" {
			return "UUID", c.UUID
		}
		return "Password", c.Password
	}
}

// exportHintText is the single source of the "URL only in the .txt export"
// hint copy (v1.36): account detail + Buy/Trial success messages share it so
// the wording cannot drift (code review fix).
const exportHintText = "Config lengkap (URL import): gunakan tombol Ekspor .txt di bawah."

// AccountDetailText renders the full account details. The credential line is
// protocol-aware (UUID vless/vmess, Password trojan/shadowsocks) and the built
// config URL is intentionally NOT shown here — full config lives in the
// Ekspor .txt document (v1.36 UI cleanup).
func AccountDetailText(c postgres.ClientView, now time.Time) string {
	status := "Aktif"
	if c.IsExpired || (c.ExpiresAt != nil && !c.ExpiresAt.After(now)) {
		status = "Expired"
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Detail Akun\n━━━━━━━━━━━━━━\nServer: %s %s\n",
		flagOrGlobe(c.FlagEmoji), c.ServerName))
	b.WriteString(fmt.Sprintf("Protocol: %s\n", strings.ToUpper(c.Protocol)))
	b.WriteString(fmt.Sprintf("Email: %s\n", c.Email))
	if label, cred := accountCredential(c); cred != "" {
		b.WriteString(fmt.Sprintf("%s: %s\n", label, cred))
	}
	b.WriteString(fmt.Sprintf("Limit IP: %d\n", c.IPLimit))
	b.WriteString(fmt.Sprintf("Traffic Terpakai: %s\n", trafficBytes(c.TrafficUsed)))
	if c.TrafficLimit > 0 {
		b.WriteString(fmt.Sprintf("Kuota: %d GB\n", c.TrafficLimit/1024/1024/1024))
	}
	if c.ExpiresAt != nil {
		b.WriteString(fmt.Sprintf("Masa aktif: %s (%s)\n", c.ExpiresAt.Format("02 Jan 2006"), status))
	}
	b.WriteString("━━━━━━━━━━━━━━\n")
	b.WriteString(exportHintText + "\n")
	return b.String()
}

// AccountDetailKeyboard offers the live traffic page, v2Ray config view,
// Clash YAML convert, export, delete + back navigation (v1.26 Config V2Ray;
// v1.31 Hapus Akun FR-08 AC-4; v1.32 Traffic FR-08 AC-3; v1.33 Convert YAML
// FR-08 AC-2).
func AccountDetailKeyboard(clientID int64) models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(
		models.InlineKeyboardButton{Text: "Traffic", CallbackData: PrefixAccountTraffic + fmt.Sprintf("%d", clientID)},
		models.InlineKeyboardButton{Text: "Config V2Ray", CallbackData: PrefixAccountConfig + fmt.Sprintf("%d", clientID)},
		models.InlineKeyboardButton{Text: "Convert YAML", CallbackData: PrefixAccountConvert + fmt.Sprintf("%d", clientID)},
		models.InlineKeyboardButton{Text: "Ekspor .txt", CallbackData: PrefixAccountExport + fmt.Sprintf("%d", clientID)},
		models.InlineKeyboardButton{Text: "Hapus Akun", CallbackData: PrefixAccountDelete + fmt.Sprintf("%d", clientID)},
		backBtn(CallbackAccount, "⬅️ Kembali"),
	)}
}

// AccountTXTContent builds the exported .txt document body (M7 feature): the
// account credentials + config link + short import guide.
func AccountTXTContent(c postgres.ClientView, now time.Time) string {
	var b strings.Builder
	b.WriteString("=== AKUN VPN " + BrandName + " ===\n")
	b.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	b.WriteString(fmt.Sprintf("Server      : %s %s\n", flagOrGlobe(c.FlagEmoji), c.ServerName))
	b.WriteString(fmt.Sprintf("Protocol    : %s\n", strings.ToUpper(c.Protocol)))
	b.WriteString(fmt.Sprintf("Email       : %s\n", c.Email))
	if label, cred := accountCredential(c); cred != "" {
		b.WriteString(fmt.Sprintf("%-12s: %s\n", label, cred))
	}
	b.WriteString(fmt.Sprintf("Limit IP    : %d\n", c.IPLimit))
	if c.TrafficLimit > 0 {
		b.WriteString(fmt.Sprintf("Traffic     : %s / %d GB\n", trafficBytes(c.TrafficUsed), c.TrafficLimit/1024/1024/1024))
	} else {
		b.WriteString(fmt.Sprintf("Traffic     : %s (Unlimited)\n", trafficBytes(c.TrafficUsed)))
	}
	if c.ExpiresAt != nil {
		status := "Aktif"
		if c.IsExpired || !c.ExpiresAt.After(now) {
			status = "Expired"
		}
		b.WriteString(fmt.Sprintf("Masa Aktif  : %s (%s)\n", c.ExpiresAt.Format("02 Jan 2006"), status))
	}
	b.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	if pair := AccountConfigLinks(c); pair.Has() {
		b.WriteString("Config TLS (443):\n" + pair.TLS + "\n")
		b.WriteString("Config Non-TLS (80):\n" + pair.NTLS + "\n")
	} else if strings.TrimSpace(c.ConfigLink) != "" {
		b.WriteString(fmt.Sprintf("Config Link :\n%s\n", c.ConfigLink))
	} else {
		b.WriteString("Config Link : belum tersedia\n")
	}
	b.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	b.WriteString("Cara pakai:\n")
	b.WriteString("1. Install aplikasi VPN (v2rayNG / NekoBox / Hiddify).\n")
	b.WriteString("2. Import Config Link dari file ini (paste / scan QR di aplikasi VPN).\n")
	b.WriteString("3. Aktifkan koneksi — selamat berselancar.\n")
	b.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	return b.String()
}

// AccountTXTName returns a safe document filename for the exported account.
func AccountTXTName(email string) string {
	slug := strings.ToLower(strings.ReplaceAll(email, "@", "-at-"))
	slug = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '.' || r == '_' {
			return r
		}
		return '-'
	}, slug)
	if len(slug) > 40 {
		slug = slug[:40]
	}
	return "akun-" + slug + ".txt"
}
