// Package telegram also hosts the admin server-management views (FR-11, v1.40).
//
// @file      internal/service/telegram/menu_admin_servers.go
// @for       FR-11: server list/detail, toggle open/active, add-server FSM copy.
// @uses      fmt, strings, github.com/go-telegram/bot/models, internal/repository/postgres
// @reason    Pure presentation per UI copy policy; add-server is a 6-step FSM
// that mirrors the ban/saldo input pattern (split for §1.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-12
package telegram

import (
	"fmt"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// Admin server-management callback data contract (FR-11 v1.40).
const (
	CallbackAdminServers          = "admin:servers"
	CallbackAdminServerAdd        = "admin:server:add"
	CallbackAdminServerAddConfirm = "admin:server:add:confirm"
	PrefixAdminServer             = "admin:server:"
	PrefixAdminServerOpen         = "admin:server:open:"
	PrefixAdminServerActive       = "admin:server:active:"
)

// AdminServersText introduces the server list.
func AdminServersText() string {
	return "Manajemen Server\n━━━━━━━━━━━━━━\n\n" +
		"Pilih server untuk toggle penjualan / status, atau tambah server baru."
}

// AdminServersKeyboard lists every server (active + inactive) with sellable
// state (2-1-2-1 zigzag).
func AdminServersKeyboard(servers []postgres.ServerAdminView) models.ReplyMarkup {
	buttons := make([]models.InlineKeyboardButton, 0, len(servers)+2)
	for _, s := range servers {
		state := "●"
		if !s.IsActive {
			state = "○"
		}
		open := "Terbuka"
		if !s.IsOpen {
			open = "Tutup"
		}
		label := fmt.Sprintf("%s %s — %s | %s", state, s.FlagEmoji, s.Name, open)
		if !s.IsActive {
			label += " (nonaktif)"
		}
		buttons = append(buttons, models.InlineKeyboardButton{
			Text:         label,
			CallbackData: PrefixAdminServer + fmt.Sprintf("%d", s.ID),
		})
	}
	buttons = append(buttons,
		models.InlineKeyboardButton{Text: "+ Tambah Server", CallbackData: CallbackAdminServerAdd},
		backBtn(CallbackAdminMenu, "⬅️ Kembali"),
	)
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(buttons...)}
}

// AdminServerDetailText shows one server with its state.
func AdminServerDetailText(s postgres.ServerAdminView) string {
	active := "Aktif"
	if !s.IsActive {
		active = "Nonaktif"
	}
	open := "Terbuka (bisa dibeli)"
	if !s.IsOpen {
		open = "Ditutup (tidak bisa dibeli)"
	}
	return fmt.Sprintf("Detail Server\n━━━━━━━━━━━━━━\n"+
		"Nama: %s %s\n"+
		"Host: %s:%d\n"+
		"Negara: %s (%s)\n"+
		"Status: %s\n"+
		"Penjualan: %s\n"+
		"Health: %s\n"+
		"Client aktif: %d",
		s.FlagEmoji, s.Name, s.Host, s.Port,
		strings.ToUpper(s.CountryCode), s.Location, active, open, s.HealthStatus, s.CurrentClients)
}

// AdminServerDetailKeyboard offers the open/active toggles (2-1-2-1 zigzag).
func AdminServerDetailKeyboard(s postgres.ServerAdminView) models.ReplyMarkup {
	openLabel := "Tutup Penjualan"
	if !s.IsOpen {
		openLabel = "Buka Penjualan"
	}
	activeLabel := "Nonaktifkan Server"
	if !s.IsActive {
		activeLabel = "Aktifkan Server"
	}
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(
		models.InlineKeyboardButton{Text: openLabel, CallbackData: PrefixAdminServerOpen + fmt.Sprintf("%d", s.ID)},
		models.InlineKeyboardButton{Text: activeLabel, CallbackData: PrefixAdminServerActive + fmt.Sprintf("%d", s.ID)},
		backBtn(CallbackAdminServers, "⬅️ Daftar Server"),
	)}
}

// AdminServerToggledText confirms an open/active toggle.
func AdminServerToggledText(label string) string {
	return label + " berhasil diperbarui."
}

// --- add-server FSM prompts (FR-11) ---

// AdminServerAddNamePrompt starts the add flow (step 1/6).
func AdminServerAddNamePrompt() string {
	return "Tambah Server (1/6)\n━━━━━━━━━━━━━━\nKetik nama server (contoh: ID-03).\n\nKetik /cancel untuk membatalkan."
}

// AdminServerAddHostPrompt asks for the panel host (step 2/6).
func AdminServerAddHostPrompt() string {
	return "Tambah Server (2/6)\n━━━━━━━━━━━━━━\nKetik host panel (domain/IP, tanpa port).\n\nKetik /cancel untuk membatalkan."
}

// AdminServerAddPortPrompt asks for the panel port (step 3/6).
func AdminServerAddPortPrompt() string {
	return "Tambah Server (3/6)\n━━━━━━━━━━━━━━\nKetik port panel (contoh: 2083).\n\nKetik /cancel untuk membatalkan."
}

// AdminServerAddUsernamePrompt asks for the panel login (step 4/6).
func AdminServerAddUsernamePrompt() string {
	return "Tambah Server (4/6)\n━━━━━━━━━━━━━━\nKetik username login panel.\n\nKetik /cancel untuk membatalkan."
}

// AdminServerAddPasswordPrompt asks for the panel password (step 5/6).
// The password is sealed with AES-256-GCM before storage (PRD §15.1).
func AdminServerAddPasswordPrompt() string {
	return "Tambah Server (5/6)\n━━━━━━━━━━━━━━\nKetik password login panel.\n\n" +
		"Password akan dienkripsi sebelum disimpan (AES-256-GCM).\n\nKetik /cancel untuk membatalkan."
}

// AdminServerAddCountryPrompt asks for the country code (step 6/6).
func AdminServerAddCountryPrompt() string {
	return "Tambah Server (6/6)\n━━━━━━━━━━━━━━\nKetik kode negara (contoh: ID, SG, JP).\n\nKetik /cancel untuk membatalkan."
}

// AdminServerAddConfirmText summarizes the new panel before creation.
func AdminServerAddConfirmText(name, host string, port int, username, country, flag string) string {
	return fmt.Sprintf("Konfirmasi Tambah Server\n━━━━━━━━━━━━━━\n"+
		"Nama: %s %s\n"+
		"Host: %s:%d\n"+
		"Username: %s\n"+
		"Negara: %s\n\n"+
		"Server akan langsung aktif dan terbuka. Lanjutkan?",
		flag, name, host, port, username, strings.ToUpper(country))
}

// AdminServerAddConfirmKeyboard asks explicit confirmation.
func AdminServerAddConfirmKeyboard() models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(
		models.InlineKeyboardButton{Text: "Simpan Server", CallbackData: CallbackAdminServerAddConfirm},
		backBtn(CallbackAdminCancel, "Batal ✕"),
	)}
}

// AdminServerAddedText confirms the new server with its id.
func AdminServerAddedText(id int64) string {
	return fmt.Sprintf("Server ditambahkan (ID %d). Sekarang aktif dan terbuka.", id)
}
