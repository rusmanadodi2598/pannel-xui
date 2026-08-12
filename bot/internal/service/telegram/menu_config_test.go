// Package telegram test covers the account v2Ray config view (v1.26).
//
// @file      internal/service/telegram/menu_config_test.go
// @for       AccountConfigText dual TLS/NTLS view, keyboard contract, .txt export.
// @uses      testing, strings, time, github.com/go-telegram/bot/models,
// internal/repository/postgres
// @reason    The config view is the v2Ray import contract — regressions break
// user imports (v1.26, reference client-vpn format).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

const cfgHost = "id2.kentangtechstore.net"

func wsViewClient(protocol, uuid, password, email, nativeLink, network, path string) postgres.ClientView {
	return postgres.ClientView{
		VPNClient:  postgres.VPNClient{Email: email, UUID: uuid, Password: password, Protocol: protocol, ConfigLink: nativeLink, InboundNetwork: network, InboundPath: path},
		ServerName: "ID-01",
		ServerHost: cfgHost,
		FlagEmoji:  "🇮🇩",
	}
}

func TestAccountConfigText_GivenWSRealPath_ThenParametersShownWithoutURLs(t *testing.T) {
	c := wsViewClient("vless", "uuid-1", "", "kts-abcd1234", "vless://native", "ws", "/vlessws")
	text := AccountConfigText(c)
	for _, want := range []string{
		"Detail Konfigurasi VPN", "Server   : 🇮🇩 ID-01", "Protocol : VLESS",
		"Domain   : " + cfgHost, "Port TLS : 443", "Port Non-TLS: 80",
		"UUID : uuid-1", "Network  : ws", "Path     : /vlessws",
		"Ekspor .txt", "TLS (443) — lebih aman", "Non-TLS (80)",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("AccountConfigText missing %q in:\n%s", want, text)
		}
	}
	// v1.36: URL build TIDAK tampil di view Config V2Ray — hanya di ekspor .txt.
	for _, banned := range []string{"URL Config", "vless://", "path=%2F", "Cara pakai"} {
		if strings.Contains(text, banned) {
			t.Errorf("config view must not contain %q (URL hanya di ekspor):\n%s", banned, text)
		}
	}
}

func TestAccountConfigText_GivenGRPC_ThenServiceNameShown(t *testing.T) {
	c := wsViewClient("trojan", "", "pw-9", "kts-abcd9999", "trojan://native", "grpc", "trojan-grpc")
	text := AccountConfigText(c)
	if !strings.Contains(text, "Service Name: trojan-grpc") {
		t.Errorf("grpc config must show real serviceName in:\n%s", text)
	}
	// v1.36: parameter saja — URL (serviceName=...) tidak dirender.
	if strings.Contains(text, "serviceName=") || strings.Contains(text, "type=grpc") {
		t.Errorf("grpc config must not render URL params (v1.36):\n%s", text)
	}
}

func TestAccountConfigText_GivenLegacyRow_ThenProtocolPathFallback(t *testing.T) {
	// Pre-v1.27 rows: network/path kosong → tetap ws /{protocol} (v1.26 links).
	c := wsViewClient("vless", "uuid-1", "", "kts-abcd1234", "vless://native", "", "")
	text := AccountConfigText(c)
	if !strings.Contains(text, "Path     : /vless") {
		t.Errorf("legacy fallback wrong in:\n%s", text)
	}
	if strings.Contains(text, "path=%2Fvless") {
		t.Errorf("legacy row must not render URL (v1.36):\n%s", text)
	}
}

func TestAccountConfigText_GivenTrojan_ThenPasswordLabel(t *testing.T) {
	c := wsViewClient("trojan", "", "pw-9", "kts-abcd9999", "trojan://native", "ws", "/trojanws")
	text := AccountConfigText(c)
	if !strings.Contains(text, "Password : pw-9") {
		t.Errorf("trojan must show Password label:\n%s", text)
	}
	// v1.36: parameter saja — URL trojan tidak dirender.
	if strings.Contains(text, "trojan://") {
		t.Errorf("trojan config must not render URL (v1.36):\n%s", text)
	}
}

func TestAccountConfigText_GivenNonWSProtocol_ThenNoURLAndExportHint(t *testing.T) {
	c := wsViewClient("hysteria", "", "auth-1", "kts-abcd5678", "hysteria2://native", "tcp", "")
	text := AccountConfigText(c)
	for _, banned := range []string{"Config Link", "hysteria2://", "Port TLS", "URL Config"} {
		if strings.Contains(text, banned) {
			t.Errorf("non-ws config must not contain %q (v1.36):\n%s", banned, text)
		}
	}
	if !strings.Contains(text, "Ekspor .txt") {
		t.Errorf("non-ws config missing export hint:\n%s", text)
	}
}

func TestAccountConfigText_GivenReality_ThenNoURLAndExportHint(t *testing.T) {
	// VLESS-REALITY (network tcp) — tidak boleh link ws palsu dan tanpa URL.
	c := wsViewClient("vless", "uuid-r", "", "kts-reality", "vless://native-reality", "tcp", "")
	text := AccountConfigText(c)
	for _, banned := range []string{"vless://", "URL Config", "Port TLS"} {
		if strings.Contains(text, banned) {
			t.Errorf("reality config must not contain %q (v1.36):\n%s", banned, text)
		}
	}
	if !strings.Contains(text, "Ekspor .txt") {
		t.Errorf("reality config missing export hint:\n%s", text)
	}
}

func TestAccountConfigKeyboard_GivenID_ThenBackToDetail(t *testing.T) {
	kb := AccountConfigKeyboard(3)
	markup, ok := kb.(models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("keyboard type = %T", kb)
	}
	if markup.InlineKeyboard[0][0].CallbackData != "account:view:3" {
		t.Errorf("back button = %q, want account:view:3", markup.InlineKeyboard[0][0].CallbackData)
	}
	// Back is a nav button (⬅️ allowed); action icons like 📄 must never appear.
	if strings.Contains(markup.InlineKeyboard[0][0].Text, "📄") {
		t.Errorf("back button must not carry action icons: %q", markup.InlineKeyboard[0][0].Text)
	}
}

func TestAccountDetailKeyboard_GivenID_ThenTrafficConfigConvertExportAndDeleteButtons(t *testing.T) {
	kb := AccountDetailKeyboard(3)
	markup, ok := kb.(models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("keyboard type = %T", kb)
	}
	// 6 buttons → 2-1-2-1 zigzag (v1.42): [Traffic,Config],[Convert],
	// [Export,Delete],[Kembali].
	// FR-08 AC-3 (v1.32): the first action is the live traffic page.
	if markup.InlineKeyboard[0][0].CallbackData != "account:traffic:3" {
		t.Errorf("traffic button = %q, want account:traffic:3", markup.InlineKeyboard[0][0].CallbackData)
	}
	if markup.InlineKeyboard[0][1].CallbackData != "account:config:3" {
		t.Errorf("config button = %q, want account:config:3", markup.InlineKeyboard[0][1].CallbackData)
	}
	// FR-08 AC-2 (v1.33): Convert YAML sits between config and export.
	if markup.InlineKeyboard[1][0].CallbackData != "account:convert:3" {
		t.Errorf("convert button = %q, want account:convert:3", markup.InlineKeyboard[1][0].CallbackData)
	}
	if markup.InlineKeyboard[2][0].CallbackData != "account:export:3" {
		t.Errorf("export button = %q, want account:export:3", markup.InlineKeyboard[2][0].CallbackData)
	}
	// FR-08 AC-4 (v1.31): the detail page offers Hapus Akun before back.
	if markup.InlineKeyboard[2][1].CallbackData != "account:delete:3" {
		t.Errorf("delete button = %q, want account:delete:3", markup.InlineKeyboard[2][1].CallbackData)
	}
	// Icon policy: action buttons (Traffic / Config V2Ray / Convert YAML /
	// Ekspor .txt / Hapus Akun) stay icon-free.
	for _, row := range markup.InlineKeyboard[:3] {
		for _, btn := range row {
			if strings.ContainsAny(btn.Text, "📄🔒🔓🗑️") {
				t.Errorf("action button must be icon-free: %q", btn.Text)
			}
		}
	}
}

func TestAccountTXTContent_GivenWSRealPath_ThenDualLinksIncluded(t *testing.T) {
	c := wsViewClient("trojan", "", "pw-9", "kts-abcd9999", "trojan://native", "ws", "/trojanws")
	content := AccountTXTContent(c, time.Now())
	for _, want := range []string{
		"Config TLS (443):", "trojan://pw-9@" + cfgHost + ":443?",
		"Config Non-TLS (80):", "trojan://pw-9@" + cfgHost + ":80?",
		"path=%2Ftrojanws",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("export missing %q in:\n%s", want, content)
		}
	}
	if strings.Contains(content, "trojan://native") {
		t.Errorf("export must use dual links, not native fallback:\n%s", content)
	}
}

func TestAccountTXTContent_GivenSubscriptionURLs_ThenIncluded(t *testing.T) {
	c := wsViewClient("vless", "uuid-1", "", "kts-abcd9999", "vless://native", "ws", "/vlessws")
	c.SubscriptionURL = "https://p.example.com:2096/sub/kts-abcd9999"
	c.SubscriptionJSONURL = "https://p.example.com:2096/json/kts-abcd9999"
	content := AccountTXTContent(c, time.Now())
	for _, want := range []string{
		"Subscription URL (auto-update):", "https://p.example.com:2096/sub/kts-abcd9999",
		"Subscription JSON (Clash/Meta):", "https://p.example.com:2096/json/kts-abcd9999",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("export missing %q in:\n%s", want, content)
		}
	}
}

func TestAccountTXTContent_GivenNoSubscriptionURL_ThenAbsent(t *testing.T) {
	c := wsViewClient("vless", "uuid-1", "", "kts-abcd9999", "vless://native", "ws", "/vlessws")
	content := AccountTXTContent(c, time.Now())
	// Legacy accounts (sebelum FR-13) punya kolom kosong — blok tidak muncul.
	if strings.Contains(content, "Subscription URL (auto-update):") {
		t.Errorf("legacy account must not show the subscription block:\n%s", content)
	}
}
