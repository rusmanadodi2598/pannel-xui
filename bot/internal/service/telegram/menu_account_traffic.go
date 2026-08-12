// Package telegram also hosts the account traffic page (FR-08 AC-3).
//
// @file      internal/service/telegram/menu_account_traffic.go
// @for       Live usage page: progress bar + status colour (🟢🟡🔴) + refresh.
// @uses      fmt, strings, time, github.com/go-telegram/bot/models, internal/repository/postgres
// @reason    The sweep worker keeps usage fresh in the background; this page
// gives the user a manual refresh on demand (parity reference client-vpn).
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

// trafficBarLen is the FR-08 AC-3 progress bar width (10 blocks).
const trafficBarLen = 10

// trafficStatus maps usage percent to the AC-3 status colour + label (parity
// reference: 🔴 ≥90% "Hampir Habis", 🟡 ≥70% "Perhatian", 🟢 otherwise).
// An unlimited plan (limit ≤ 0) is always green with no percent.
func trafficStatus(used, limit int64) (emoji, label string, pct float64) {
	if limit <= 0 {
		return "🟢", "Normal", 0
	}
	pct = float64(used) / float64(limit) * 100
	if pct > 100 {
		pct = 100
	}
	switch {
	case pct >= 90:
		return "🔴", "Hampir Habis", pct
	case pct >= 70:
		return "🟡", "Perhatian", pct
	default:
		return "🟢", "Normal", pct
	}
}

// trafficBar renders the [█…░…] usage bar plus the percent (limit > 0 only).
// trafficStatus already clamps pct to 100, so filled can never exceed the bar.
func trafficBar(used, limit int64) string {
	_, _, pct := trafficStatus(used, limit)
	filled := int(pct / (100 / trafficBarLen))
	return strings.Repeat("█", filled) + strings.Repeat("░", trafficBarLen-filled) +
		fmt.Sprintf(" %.1f%%", pct)
}

// trafficBytes formats bytes as B/KB/MB/GB/TB (binary 1024, 2 decimals —
// parity with the reference format_bytes).
func trafficBytes(b int64) string {
	if b <= 0 {
		return "0 B"
	}
	f := float64(b)
	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	return fmt.Sprintf("%.2f %s", f, units[i])
}

// AccountTrafficText renders the live usage page (FR-08 AC-3): status colour,
// upload/download split, total vs quota, remaining, progress bar and the last
// sync time.
func AccountTrafficText(c postgres.ClientView, now time.Time) string {
	emoji, label, _ := trafficStatus(c.TrafficUsed, c.TrafficLimit)
	var b strings.Builder
	b.WriteString("Detail Traffic\n━━━━━━━━━━━━━━\n")
	b.WriteString(fmt.Sprintf("Akun: %s\n", c.Email))
	b.WriteString(fmt.Sprintf("Status: %s %s\n\n", emoji, label))
	b.WriteString(fmt.Sprintf("Upload: %s\n", trafficBytes(c.TrafficUp)))
	b.WriteString(fmt.Sprintf("Download: %s\n", trafficBytes(c.TrafficDown)))
	b.WriteString("━━━━━━━━━━━━━━\n")
	b.WriteString(fmt.Sprintf("Total: %s\n", trafficBytes(c.TrafficUsed)))
	if c.TrafficLimit > 0 {
		remaining := c.TrafficLimit - c.TrafficUsed
		if remaining < 0 {
			remaining = 0
		}
		b.WriteString(fmt.Sprintf("Kuota: %s\n", trafficBytes(c.TrafficLimit)))
		b.WriteString(fmt.Sprintf("Sisa: %s\n", trafficBytes(remaining)))
		b.WriteString(fmt.Sprintf("[%s]\n", trafficBar(c.TrafficUsed, c.TrafficLimit)))
	} else {
		b.WriteString("Kuota: Unlimited\n")
	}
	b.WriteString("━━━━━━━━━━━━━━\n")
	if c.LastSync != nil {
		b.WriteString(fmt.Sprintf("Terakhir Sync: %s\n", c.LastSync.In(now.Location()).Format("02 Jan 2006 15:04")))
	} else {
		b.WriteString("Terakhir Sync: Belum pernah\n")
	}
	return b.String()
}

// AccountTrafficKeyboard offers a manual refresh (re-syncs from the panel)
// and back to the account detail (FR-08 AC-3).
func AccountTrafficKeyboard(clientID int64) models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(
		models.InlineKeyboardButton{Text: "Refresh", CallbackData: PrefixAccountTraffic + fmt.Sprintf("%d", clientID)},
		backBtn(PrefixAccountView+fmt.Sprintf("%d", clientID), "⬅️ Kembali"),
	)}
}
