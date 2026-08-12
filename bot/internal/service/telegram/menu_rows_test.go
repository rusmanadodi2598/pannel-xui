// Package telegram test covers the shared keyboard-layout helper (v1.42).
//
// @file      internal/service/telegram/menu_rows_test.go
// @for       packRows zigzag 2-1-2-1 row packing contract.
// @uses      testing, github.com/go-telegram/bot/models
// @reason    Every sub-menu keyboard depends on this helper — a regression
// here silently re-stacks every menu back to one button per row.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-12
package telegram

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestPackRows_GivenCounts_ThenZigzagPattern(t *testing.T) {
	for _, tt := range []struct {
		n    int
		want []int // per-row widths
	}{
		{0, nil},
		{1, []int{1}},
		{2, []int{2}},
		{3, []int{2, 1}},
		{4, []int{2, 1, 1}},
		{5, []int{2, 1, 2}},
		{6, []int{2, 1, 2, 1}},
		{7, []int{2, 1, 2, 1, 1}},
		{8, []int{2, 1, 2, 1, 2}},
		{9, []int{2, 1, 2, 1, 2, 1}},
	} {
		buttons := make([]models.InlineKeyboardButton, tt.n)
		for i := range buttons {
			buttons[i] = models.InlineKeyboardButton{CallbackData: "cb" + string(rune('a'+i))}
		}
		rows := packRows(buttons...)
		if len(rows) != len(tt.want) {
			t.Errorf("packRows(%d) rows = %d, want %d (%v)", tt.n, len(rows), len(tt.want), tt.want)
			continue
		}
		// Row widths follow the zigzag, and the relative order is preserved.
		pos := 0
		for i, row := range rows {
			if len(row) != tt.want[i] {
				t.Errorf("packRows(%d) row %d width = %d, want %d", tt.n, i, len(row), tt.want[i])
			}
			for _, btn := range row {
				if btn.CallbackData != "cb"+string(rune('a'+pos)) {
					t.Errorf("packRows(%d) order broken at %d: %q", tt.n, pos, btn.CallbackData)
				}
				pos++
			}
		}
		if pos != tt.n {
			t.Errorf("packRows(%d) emitted %d buttons, want %d", tt.n, pos, tt.n)
		}
	}
}
