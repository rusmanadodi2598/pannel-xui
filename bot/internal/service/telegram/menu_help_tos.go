// Package telegram also hosts the static ToS & info views (FR-15).
//
// @file      internal/service/telegram/menu_help_tos.go
// @for       FR-15 Bantuan: disclaimer, ToS akun, ToS pembayaran, info bot.
// @uses      github.com/go-telegram/bot/models
// @reason    Split from menu_help.go to respect the 250-line limit (AGENTS.md
// §1.1); static id-ID copy with back-to-help and home navigation (FR-15 AC).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"fmt"

	"github.com/go-telegram/bot/models"
)

// HelpDisclaimerText introduces the ToS sub-pages (FR-15 help:disclaimer).
func HelpDisclaimerText() string {
	return `Disclaimer & Ketentuan Layanan
━━━━━━━━━━━━━━

Dengan menggunakan layanan kami, kamu dianggap telah membaca, memahami, dan menyetujui seluruh ketentuan berikut.

Pilih kategori di bawah untuk membaca selengkapnya:

Ketentuan Penggunaan Akun — aturan dan larangan dalam penggunaan akun VPN.
Ketentuan Pembayaran — kebijakan saldo, pembelian, dan refund.

━━━━━━━━━━━━━━`
}

// HelpDisclaimerKeyboard offers the two ToS sub-pages (FR-15 AC).
func HelpDisclaimerKeyboard() models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{
			{Text: "Ketentuan Akun", CallbackData: CallbackHelpTosAccount},
			{Text: "Ketentuan Pembayaran", CallbackData: CallbackHelpTosPayment},
		},
		helpNavRow(CallbackHelp),
	}}
}

// HelpTosAccountText is the account-usage ToS (FR-15 help:tos:account).
func HelpTosAccountText() string {
	return `Ketentuan Penggunaan Akun VPN
━━━━━━━━━━━━━━

Dengan menggunakan akun VPN dari layanan kami, kamu wajib mematuhi seluruh ketentuan berikut:

DILARANG KERAS:
1. Konten Ilegal — dilarang mengakses, mengunduh, atau menyebarkan konten yang melanggar hukum.
2. Konten Pornografi — dilarang mengakses konten pornografi dalam bentuk apa pun.
3. Torrent & P2P Ilegal — dilarang torrent atau file sharing ilegal yang membebani bandwidth server.
4. Multi-Login Berlebihan — batas perangkat maks 2 device; pemakaian berlebih terdeteksi otomatis.
5. Spam & Serangan — dilarang spam, DDoS, brute force, phishing, atau serangan siber.
6. Penyalahgunaan Akun — dilarang membagikan IP address dan domain server ke publik.
7. Speedtest Berlebihan — dilarang speedtest terus-menerus; akun yang melanggar di-suspend langsung tanpa peringatan.

SANKSI PELANGGARAN:
• Peringatan 1-2: notifikasi peringatan otomatis.
• Peringatan 3: akun DIBLOKIR PERMANEN tanpa pemberitahuan lebih lanjut.
• Sanksi berlaku tanpa pengecualian. Sistem memantau penggunaan secara otomatis.

Kami berhak menonaktifkan akun yang melanggar ketentuan di atas tanpa pengembalian dana.

━━━━━━━━━━━━━━`
}

// HelpTosAccountKeyboard links to the payment ToS (FR-15 AC).
func HelpTosAccountKeyboard() models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		backRow(CallbackHelpTosPayment, "Ketentuan Pembayaran"),
		helpNavRow(CallbackHelpDisclaimer),
	}}
}

// HelpTosPaymentText is the payment & balance ToS (FR-15 help:tos:payment).
func HelpTosPaymentText() string {
	return `Ketentuan Pembayaran & Saldo
━━━━━━━━━━━━━━

1. Kebijakan Saldo
   • Saldo yang sudah diisi tidak dapat direfund atau ditarik kembali dalam kondisi apa pun.
   • Saldo hanya bisa dipakai untuk pembelian layanan VPN.

2. Kebijakan Pembelian
   • Dengan membeli, kamu dianggap telah memahami cara penggunaan layanan.
   • Harga yang tertera sudah termasuk biaya layanan.
   • Akun yang sudah dibuat tidak bisa dibatalkan atau ditukar.

3. Kegagalan Pembayaran
   • Jika saldo tidak bertambah setelah bayar, hubungi admin dengan bukti transfer.
   • Verifikasi manual diproses dalam waktu 1x24 jam.

4. Masa Aktif & Kuota
   • Akun yang expired dinonaktifkan otomatis oleh sistem.
   • Kuota yang tidak terpakai tidak bisa dialihkan atau diakumulasi.
   • Tidak ada perpanjangan otomatis — beli ulang setelah masa aktif habis.

PENTING:
Dengan menggunakan layanan ini, kamu menyetujui seluruh ketentuan di atas. Ketidaktahuan terhadap syarat dan ketentuan bukan alasan pengecualian.

━━━━━━━━━━━━━━`
}

// HelpTosPaymentKeyboard links back to the account ToS (FR-15 AC).
func HelpTosPaymentKeyboard() models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		backRow(CallbackHelpTosAccount, "Ketentuan Akun"),
		helpNavRow(CallbackHelpDisclaimer),
	}}
}

// HelpInfoText describes the bot, its features and contact (FR-15 help:info;
// brand spelling from BrandName, v1.44).
func HelpInfoText() string {
	return fmt.Sprintf(`Informasi Bot
━━━━━━━━━━━━━━

%s VPN Bot — bot resmi untuk pembelian dan manajemen akun VPN.

Fitur:
• Beli Akun VPN — order akun baru (VLESS, VMESS, TROJAN, dan lainnya)
• Trial Gratis — coba gratis sebelum membeli
• Top Up Saldo — isi saldo via QRIS (otomatis)
• Akun Saya — kelola akun aktif, lihat config & traffic
• Riwayat — histori transaksi pembelian & top up

Komunitas & Kontak:
• Gabung grup diskusi kami untuk info terbaru.
• Hubungi admin jika mengalami kendala.

━━━━━━━━━━━━━━`, BrandName)
}

// HelpInfoKeyboard returns to the help hub (FR-15 AC).
func HelpInfoKeyboard() models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		helpNavRow(CallbackHelp),
	}}
}
