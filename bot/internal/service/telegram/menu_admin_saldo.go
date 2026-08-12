// Package telegram also hosts the admin adjust-saldo views (FR-11, v1.39).
//
// @file      internal/service/telegram/menu_admin_saldo.go
// @for       FR-11: admin credit/debit — menu, prompts, confirm & done copy/keyboards.
// @uses      fmt, github.com/go-telegram/bot/models, internal/domain, internal/repository/postgres
// @reason    Pure presentation per UI copy policy (emoji-free body); handler stays
// network-free and testable (split from menu_admin.go for §1.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-12
package telegram

import (
	"fmt"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/domain"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// Admin adjust-saldo callback data contract (FR-11 v1.39).
const (
	CallbackAdminSaldo      = "admin:saldo"
	PrefixAdminSaldoKredit  = "admin:saldo:kredit"
	PrefixAdminSaldoDebit   = "admin:saldo:debit"
	PrefixAdminSaldoConfirm = "admin:saldo:confirm:"
)

// AdminSaldoMenuText introduces the adjust-saldo action picker.
func AdminSaldoMenuText() string {
	return "Adjust Saldo\n━━━━━━━━━━━━━━\n\n" +
		"Kredit menambah saldo user (topup manual, kompensasi);\n" +
		"Debit mengurangi saldo (refund manual, koreksi).\n\n" +
		"Setiap perubahan tercatat di ledger balance_transactions."
}

// AdminSaldoMenuKeyboard offers credit or debit (FR-11 v1.39, 2-1-2-1 zigzag).
func AdminSaldoMenuKeyboard() models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(
		models.InlineKeyboardButton{Text: "+ Kredit Saldo", CallbackData: PrefixAdminSaldoKredit},
		models.InlineKeyboardButton{Text: "- Debit Saldo", CallbackData: PrefixAdminSaldoDebit},
		backBtn(CallbackAdminMenu, "⬅️ Kembali"),
	)}
}

// AdminSaldoIDPrompt asks for the target Telegram id (FSM input, FR-11).
func AdminSaldoIDPrompt(credit bool) string {
	verb := "di-debit"
	if credit {
		verb = "di-kredit"
	}
	return fmt.Sprintf("Ketik Telegram ID user yang saldonya mau %s (angka).\n\n"+
		"Ketik /cancel untuk membatalkan.", verb)
}

// AdminSaldoAmountPrompt asks for the nominal once the user is resolved.
func AdminSaldoAmountPrompt(credit bool, u *postgres.User) string {
	verb := "potong"
	if credit {
		verb = "tambahkan"
	}
	who := adminUserLabel(u)
	if u == nil {
		who = "user"
	}
	return fmt.Sprintf("Saldo %s.\n\nKetik nominal yang mau %s (rupiah, contoh: 25000).\n\n"+
		"Ketik /cancel untuk membatalkan.", who, verb)
}

// AdminSaldoConfirmText summarizes the adjustment before execution.
func AdminSaldoConfirmText(credit bool, u *postgres.User, tgID int64, amount domain.Money) string {
	who := adminUserLabel(u)
	if u == nil {
		who = fmt.Sprintf("User %d", tgID)
	}
	action := "Debit"
	verb := "dipotong"
	if credit {
		action = "Kredit"
		verb = "ditambahkan"
	}
	return fmt.Sprintf("Konfirmasi %s Saldo\n━━━━━━━━━━━━━━\n"+
		"User: %s (ID %d)\n"+
		"Nominal: %s\n"+
		"Saldo user akan %s. Lanjutkan?", action, who, tgID, amount.FormatIDR(), verb)
}

// AdminSaldoConfirmKeyboard asks explicit confirmation (FR-11 v1.39).
func AdminSaldoConfirmKeyboard(credit bool, tgID int64, amount domain.Money) models.ReplyMarkup {
	kind := "debit"
	if credit {
		kind = "kredit"
	}
	data := PrefixAdminSaldoConfirm + kind + ":" +
		fmt.Sprintf("%d", tgID) + ":" + fmt.Sprintf("%d", amount.Rupiah())
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(
		models.InlineKeyboardButton{Text: "Konfirmasi", CallbackData: data},
		backBtn(CallbackAdminCancel, "Batal ✕"),
	)}
}

// AdminSaldoDoneText confirms the executed adjustment with the new balance.
func AdminSaldoDoneText(credit bool, tgID int64, amount, newBalance domain.Money) string {
	action := "di-debit"
	if credit {
		action = "di-kredit"
	}
	return fmt.Sprintf("Saldo user %d %s %s.\n"+
		"Saldo sekarang: %s\n"+
		"Tercatat di ledger (ADJ-...).", tgID, action, amount.FormatIDR(), newBalance.FormatIDR())
}
