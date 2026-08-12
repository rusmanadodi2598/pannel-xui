// Package telegram also hosts the shared keyboard-layout helper (v1.42).
//
// @file      internal/service/telegram/menu_rows.go
// @for       Zigzag 2-1-2-1 row packing for the sub-menu keyboards.
// @uses      github.com/go-telegram/bot/models
// @reason    Sub-menus stop stacking one button per row; the 2-1-2-1 pattern
// (UX revision v1.42) is implemented once and reused by every picker so the
// layouts cannot drift apart.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-12
package telegram

import (
	"github.com/go-telegram/bot/models"
)

// packRows arranges buttons into the 2-1-2-1-… row pattern (v1.42 UX): the
// first row holds 2 buttons, the next 1, the next 2, and so on; the last row
// takes whatever remains (1 when only one button is left). Every button keeps
// its relative order. An empty input yields an empty layout.
func packRows(buttons ...models.InlineKeyboardButton) [][]models.InlineKeyboardButton {
	rows := make([][]models.InlineKeyboardButton, 0, len(buttons)/2+1)
	left := len(buttons)
	for row := 0; left > 0; row++ {
		want := 2
		if row%2 == 1 {
			want = 1
		}
		if want > left {
			want = left
		}
		rows = append(rows, buttons[:want])
		buttons = buttons[want:]
		left -= want
	}
	return rows
}

// backBtn is a single back/home navigation button ready to be packed into a
// zigzag layout (the existing backRow keeps its slice shape for fixed rows).
func backBtn(callback, label string) models.InlineKeyboardButton {
	return models.InlineKeyboardButton{Text: label, CallbackData: callback}
}
