// Package telegram also hosts the buy-menu views (FR-03, M7 fix).
//
// @file      internal/service/telegram/menu_buy.go
// @for       Buy flow keyboards + copy: country → inbound → plan → confirm.
// @uses      fmt, strings, github.com/go-telegram/bot/models, internal/domain,
// internal/service/server
// @reason    Inbound (server + protocol) picker renders real panel state; kept
// out of menu_shop.go to respect the 250-line limit (§1.1).
//
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"fmt"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/domain"
	serversvc "github.com/kentangtech/bot-order/internal/service/server"
)

// Shop callback data contract (prefix + payload, M4/M7).
const (
	CallbackBuyBack   = "buy:back"
	CallbackRenewBack = "renew:back"

	// Prefixes for parameterised callbacks (exported for the handler layer).
	PrefixBuyCountry     = "buy:country:"
	PrefixBuyInbound     = "buy:inbound:"
	PrefixBuyPlan        = "buy:plan:"
	PrefixBuyConfirm     = "buy:confirm:"
	PrefixRenewClient    = "renew:client:"
	PrefixRenewPlan      = "renew:plan:"
	PrefixRenewConfirm   = "renew:confirm:"
	PrefixAccountView    = "account:view:"
	PrefixAccountExport  = "account:export:"
	PrefixAccountConfig  = "account:config:"
	PrefixAccountPage    = "account:page:"
	PrefixAccountDelete  = "account:delete:"
	PrefixAccountTraffic = "account:traffic:" // FR-08 AC-3: live usage + refresh
	PrefixAccountConvert = "account:convert:" // FR-08 AC-2: Clash/Meta YAML

	// PrefixAccountDeleteConfirm executes the deletion (FR-08 AC-4).
	PrefixAccountDeleteConfirm = "account:delete_confirm:"

	// CallbackAccountNoop is the non-action page indicator (answer, never edit).
	CallbackAccountNoop = "account:noop"
)

// CountryOption is one buyable country button.
type CountryOption struct {
	Code string
	Flag string
	Name string
}

// BuyCountriesKeyboard renders the FR-03 step-1 country picker (2-1-2-1 zigzag).
func BuyCountriesKeyboard(countries []CountryOption) models.ReplyMarkup {
	buttons := make([]models.InlineKeyboardButton, 0, len(countries)+1)
	for _, c := range countries {
		label := c.Flag + " " + c.Name
		if c.Flag == "" {
			label = "🌐 " + c.Name
		}
		buttons = append(buttons, models.InlineKeyboardButton{Text: label, CallbackData: PrefixBuyCountry + c.Code})
	}
	buttons = append(buttons, backBtn(CallbackBuyBack, "🏠 Menu Utama"))
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(buttons...)}
} // BuyInboundsKeyboard renders the FR-03 step-2 picker: real inbounds of the
// chosen country's panel (server + protocol + remark).
func BuyInboundsKeyboard(opts []serversvc.InboundOption) models.ReplyMarkup {
	return InboundsKeyboard(opts, func(o serversvc.InboundOption) string {
		return PrefixBuyInbound + fmt.Sprintf("%d:%d:%s", o.ServerID, o.InboundID, o.Country)
	}, CallbackBuyBack)
}

// InboundsKeyboard renders inbound options; cb builds each button's callback
// data (buy carries the country, trial does not). Shared by the FR-03 and
// FR-07 pickers so the same live panel data is rendered identically.
func InboundsKeyboard(opts []serversvc.InboundOption, cb func(serversvc.InboundOption) string, backCb string) models.ReplyMarkup {
	buttons := make([]models.InlineKeyboardButton, 0, len(opts)+1)
	for _, o := range opts {
		label := fmt.Sprintf("%s · %s", o.ServerName, strings.ToUpper(o.Protocol))
		if o.Remark != "" {
			label += " · " + o.Remark
		}
		buttons = append(buttons, models.InlineKeyboardButton{Text: label, CallbackData: cb(o)})
	}
	buttons = append(buttons, backBtn(backCb, "⬅️ Kembali"))
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(buttons...)}
}

// BuyPlansKeyboard renders the FR-03 step-3 plan picker for one country,
// carrying the chosen server + inbound (FR-03: pick protocol before plan).
func BuyPlansKeyboard(plans []domain.VpnPlan, serverID, inboundID int, protocol string) models.ReplyMarkup {
	buttons := make([]models.InlineKeyboardButton, 0, len(plans)+1)
	for _, p := range plans {
		buttons = append(buttons, models.InlineKeyboardButton{
			Text:         fmt.Sprintf("%d Hari — %s", p.Days, p.Price.FormatIDR()),
			CallbackData: PrefixBuyPlan + fmt.Sprintf("%d:%d:%s:%d", serverID, inboundID, p.CountryCode, p.Days),
		})
	}
	buttons = append(buttons, backBtn(CallbackBuyBack, "⬅️ Kembali"))
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(buttons...)}
}

// BuyConfirmKeyboard offers explicit confirmation (FR-03 AC: order only after confirm).
func BuyConfirmKeyboard(country string, days, serverID, inboundID int, protocol string) models.ReplyMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: packRows(
		models.InlineKeyboardButton{Text: "Konfirmasi Beli", CallbackData: PrefixBuyConfirm +
			fmt.Sprintf("%d:%d:%s:%d", serverID, inboundID, country, days)},
		backBtn(CallbackBuyBack, "⬅️ Kembali"),
	)}
}

// BuyCountryText introduces the country step (FR-03).
func BuyCountryText() string {
	return "Pilih negara server:\n\nPilih lokasi server VPN yang kamu mau."
}

// BuyInboundListText introduces the protocol picker (FR-03 step 2).
func BuyInboundListText(countryName string) string {
	return fmt.Sprintf("Pilih server & protocol untuk %s:\n\n"+
		"Daftar ini diambil langsung dari panel — pilih yang kamu mau.", countryName)
}

// BuyPlanListText introduces the plan step.
func BuyPlanListText(countryName string) string {
	return fmt.Sprintf("Pilih paket untuk %s:\n\nHarga selalu update dari daftar harga terbaru.", countryName)
}

// BuyConfirmText summarizes the order before confirmation (FR-03 AC, branded
// banner v1.44 — parity with the branded topup summary).
func BuyConfirmText(plan domain.VpnPlan, balance domain.Money, protocol string) string {
	var warn string
	if balance < plan.Price {
		warn = fmt.Sprintf("\n\nSaldo kamu %s — tidak cukup untuk paket ini.\nSilakan top up dulu ya.", balance.FormatIDR())
	} else {
		warn = fmt.Sprintf("\n\nSaldo kamu: %s (cukup)", balance.FormatIDR())
	}
	if protocol == "" {
		protocol = "VLESS"
	}
	return fmt.Sprintf(BrandHeader()+"\n\nRingkasan Pesanan\n━━━━━━━━━━━━━━\n"+
		"Negara: %s\n"+
		"Protocol: %s\n"+
		"Paket: VPN %s %d Hari\n"+
		"Harga: %s\n"+
		"Kuota: 100 GB / 1 IP\n━━━━━━━━━━━━━━%s\n\n"+
		"Tekan Konfirmasi untuk memproses order.",
		plan.CountryName, strings.ToUpper(protocol), plan.CountryName, plan.Days, plan.Price.FormatIDR(), warn)
}
