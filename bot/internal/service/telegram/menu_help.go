// Package telegram also hosts the static help & ToS views (FR-15).
//
// @file      internal/service/telegram/menu_help.go
// @for       FR-15 Bantuan: static id-ID copy for help hub, cara order, cara topup.
// @uses      github.com/go-telegram/bot/models
// @reason    Parity with the reference help_handler: static content with
// back-to-help and home navigation on every page (FR-15 AC).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"github.com/go-telegram/bot/models"
)

// Help callback data contract (FR-15, parity help_handler reference).
const (
	PrefixHelp             = "help:"
	CallbackHelpOrder      = "help:order"
	CallbackHelpTopup      = "help:topup"
	CallbackHelpDisclaimer = "help:disclaimer"
	CallbackHelpTosAccount = "help:tos:account"
	CallbackHelpTosPayment = "help:tos:payment"
	CallbackHelpInfo       = "help:info"
)

// HelpMenuText is the FR-15 help hub page (category list).
func HelpMenuText() string {
	return `Pusat Bantuan
━━━━━━━━━━━━━━

Pilih kategori bantuan yang kamu butuhkan:

Cara Order — panduan langkah demi langkah membeli akun VPN.
Cara Top Up — panduan mengisi saldo untuk pembelian.
Disclaimer & ToS — syarat dan ketentuan penggunaan layanan.
Info — informasi bot, grup, dan kontak admin.

━━━━━━━━━━━━━━`
}

// HelpMenuKeyboard offers the help categories (2-column, FR-15 AC).
func HelpMenuKeyboard() models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{
			{Text: "Cara Order", CallbackData: CallbackHelpOrder},
			{Text: "Cara Top Up", CallbackData: CallbackHelpTopup},
		},
		{
			{Text: "Disclaimer & ToS", CallbackData: CallbackHelpDisclaimer},
			{Text: "Info", CallbackData: CallbackHelpInfo},
		},
		backRow(CallbackHome, "🏠 Menu Utama"),
	}}
}

// HelpOrderText is the step-by-step purchase guide (FR-15 help:order).
func HelpOrderText() string {
	return `Cara Order VPN
━━━━━━━━━━━━━━

Langkah-langkah membeli akun VPN:

1. Top Up Saldo
   Pastikan saldo kamu cukup. Isi saldo lewat menu Top Up.

2. Buka Menu Order
   Klik tombol Beli VPN pada menu utama.

3. Pilih Negara & Protocol
   Pilih negara, lalu pilih server dan protocol yang tersedia.

4. Pilih Durasi
   Pilih paket durasi (15, 30, atau 90 hari).

5. Konfirmasi & Bayar
   Review pesanan lalu klik Konfirmasi. Saldo dipotong otomatis dan akun langsung aktif.

6. Selesai
   Akun VPN siap dipakai. Salin config dari menu Akun Saya.

Tips:
• Coba Trial Gratis dulu sebelum membeli (maks 2x per hari).
• Gunakan aplikasi v2rayNG (Android) atau Streisand (iOS).

━━━━━━━━━━━━━━`
}

// HelpOrderKeyboard jumps straight to the buy flow (FR-15 AC shortcut).
func HelpOrderKeyboard() models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		backRow(CallbackBuy, "Beli VPN"),
		helpNavRow(CallbackHelp),
	}}
}

// HelpTopupText is the step-by-step top-up guide (FR-15 help:topup).
func HelpTopupText() string {
	return `Cara Top Up Saldo
━━━━━━━━━━━━━━

Langkah-langkah mengisi saldo:

1. Buka Menu Top Up
   Klik tombol Top Up pada menu utama.

2. Pilih Nominal
   Pilih nominal yang tersedia, atau masukkan jumlah manual.

3. Scan QR Code
   Sistem menampilkan QRIS. Scan pakai e-wallet atau mobile banking.

4. Saldo Bertambah Otomatis
   Setelah pembayaran berhasil, saldo bertambah otomatis dalam beberapa detik.

5. Selesai
   Saldo siap dipakai untuk membeli akun VPN.

Catatan Penting:
• Pembayaran diproses via QRIS — GoPay, OVO, Dana, ShopeePay, atau mobile banking.
• Jika saldo tidak bertambah setelah 5 menit, hubungi admin.
• Jangan tutup halaman pembayaran sebelum scan QR selesai.

━━━━━━━━━━━━━━`
}

// HelpTopupKeyboard jumps straight to the top-up flow (FR-15 AC shortcut).
func HelpTopupKeyboard() models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		backRow(CallbackTopup, "Top Up"),
		helpNavRow(CallbackHelp),
	}}
}

// helpNavRow is the standard back-to-help + home navigation row (FR-15 AC).
func helpNavRow(backCb string) []models.InlineKeyboardButton {
	return []models.InlineKeyboardButton{
		{Text: "⬅️ Kembali", CallbackData: backCb},
		{Text: "🏠 Menu Utama", CallbackData: CallbackHome},
	}
}
