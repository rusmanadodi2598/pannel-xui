// Package telegram test covers the Clash/Meta YAML convert view (FR-08 AC-2).
//
// @file      internal/service/telegram/menu_config_yaml_test.go
// @for       YAML proxy blocks per protocol/transport + fallback for non-ws.
// @uses      testing, strings, github.com/go-telegram/bot/models, internal/repository/postgres
// @reason    The YAML is the Clash/Meta import contract — wrong field names
// (uuid vs password, ws-opts vs grpc-opts) break user imports (v1.33).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package telegram

import (
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestAccountConvertText_GivenVlessWSRealPath_ThenBothYAMLBlocks(t *testing.T) {
	c := wsViewClient("vless", "uuid-1", "", "kts-abcd1234", "vless://native", "ws", "/vlessws")
	text := AccountConvertText(c)
	for _, want := range []string{
		"Convert Config YAML", "kts-abcd1234", "VLESS", cfgHost,
		"YAML Config TLS (Port 443):", "YAML Config Non-TLS (Port 80):",
		"- name: kts-abcd1234-TLS", "  server: " + cfgHost, "  port: 443", "  type: vless",
		"  uuid: uuid-1", "  tls: true", "  servername: " + cfgHost,
		"  network: ws", "    path: /vlessws", "      Host: " + cfgHost, "  udp: true",
		"- name: kts-abcd1234-NTLS", "  port: 80", "  tls: false",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("AccountConvertText missing %q in:\n%s", want, text)
		}
	}
	// Native link must not leak into the YAML page.
	if strings.Contains(text, "vless://native") {
		t.Errorf("native link leaked into YAML view:\n%s", text)
	}
}

func TestAccountConvertText_GivenTrojan_ThenRealPasswordNotUUID(t *testing.T) {
	c := wsViewClient("trojan", "uuid-x", "pw-9", "kts-abcd9999", "trojan://native", "ws", "/trojanws")
	text := AccountConvertText(c)
	if !strings.Contains(text, "  password: pw-9") {
		t.Errorf("trojan YAML must carry the real password:\n%s", text)
	}
	if strings.Contains(text, "  password: uuid-x") || strings.Contains(text, "  uuid:") {
		t.Errorf("trojan YAML must NOT use uuid (reference quirk fixed):\n%s", text)
	}
}

func TestAccountConvertText_GivenVMess_ThenAlterIDAndCipher(t *testing.T) {
	c := wsViewClient("vmess", "uuid-m", "", "kts-vmess", "vmess://native", "ws", "/vmessws")
	text := AccountConvertText(c)
	if !strings.Contains(text, "  type: vmess") || !strings.Contains(text, "  alterId: 0") ||
		!strings.Contains(text, "  cipher: auto") {
		t.Errorf("vmess YAML missing alterId/cipher:\n%s", text)
	}
}

func TestAccountConvertText_GivenGRPC_ThenGRPCOpts(t *testing.T) {
	c := wsViewClient("trojan", "", "pw-g", "kts-grpc", "trojan://native", "grpc", "trojan-grpc")
	text := AccountConvertText(c)
	for _, want := range []string{"  network: grpc", "  grpc-opts:", "    grpc-service-name: trojan-grpc"} {
		if !strings.Contains(text, want) {
			t.Errorf("grpc YAML missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "ws-opts") {
		t.Errorf("grpc YAML must not carry ws-opts:\n%s", text)
	}
}

func TestAccountConvertText_GivenLegacyRow_ThenWSPathFallback(t *testing.T) {
	// Pre-v1.27 rows: network/path kosong → tetap ws /{protocol} (backward compat).
	c := wsViewClient("vless", "uuid-1", "", "kts-legacy", "vless://native", "", "")
	text := AccountConvertText(c)
	if !strings.Contains(text, "    path: /vless") {
		t.Errorf("legacy fallback wrong in:\n%s", text)
	}
}

func TestAccountConvertText_GivenReality_ThenFallbackNoteWithoutURL(t *testing.T) {
	c := wsViewClient("vless", "uuid-r", "", "kts-reality", "vless://native-reality", "tcp", "")
	text := AccountConvertText(c)
	if !strings.Contains(text, "tidak tersedia") || !strings.Contains(text, "Ekspor .txt") {
		t.Errorf("reality must show fallback note + export hint:\n%s", text)
	}
	// v1.36: URL tidak dirender di menu — cukup di ekspor .txt.
	if strings.Contains(text, "vless://") || strings.Contains(text, "proxies:") {
		t.Errorf("reality fallback must not render URL or fake YAML (v1.36):\n%s", text)
	}
}

func TestAccountConvertText_GivenSSAndHysteria_ThenFallbackNoteWithoutURL(t *testing.T) {
	for _, proto := range []struct {
		name   string
		native string
	}{
		{"shadowsocks", "ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNQ==@host:8388#ss"},
		{"hysteria", "hysteria2://auth@host:8443?insecure=1#hy"},
	} {
		c := wsViewClient(proto.name, "", "pw-"+proto.name, "kts-"+proto.name, proto.native, "tcp", "")
		text := AccountConvertText(c)
		if !strings.Contains(text, "tidak tersedia") || !strings.Contains(text, "Ekspor .txt") {
			t.Errorf("%s must show fallback note + export hint:\n%s", proto.name, text)
		}
		// v1.36: URL tidak dirender di menu — cukup di ekspor .txt.
		if strings.Contains(text, proto.native) || strings.Contains(text, "proxies:") {
			t.Errorf("%s must not render URL or fake YAML (v1.36):\n%s", proto.name, text)
		}
	}
}

func TestAccountConvertText_GivenTrialEmail_ThenShortRemarkTag(t *testing.T) {
	c := wsViewClient("vless", "uuid-1", "", "trial_829710_abcd1234@trial.kentangtech.com", "vless://native", "ws", "/vlessws")
	text := AccountConvertText(c)
	if !strings.Contains(text, "- name: trial-abcd1234-TLS") || !strings.Contains(text, "- name: trial-abcd1234-NTLS") {
		t.Errorf("trial email must shorten the proxy tag:\n%s", text)
	}
}

func TestAccountConvertKeyboard_GivenID_ThenConfigBackAndHome(t *testing.T) {
	kb := AccountConvertKeyboard(3)
	markup, ok := kb.(models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("keyboard type = %T", kb)
	}
	if markup.InlineKeyboard[0][0].CallbackData != "account:config:3" {
		t.Errorf("config button = %q, want account:config:3", markup.InlineKeyboard[0][0].CallbackData)
	}
	// v1.36: tombol relabeled (view tidak lagi berisi URL).
	if markup.InlineKeyboard[0][0].Text != "Config V2Ray" {
		t.Errorf("config button text = %q, want Config V2Ray", markup.InlineKeyboard[0][0].Text)
	}
	// v1.42: [Config, Kembali] satu baris, [Menu Utama] di bawahnya.
	if markup.InlineKeyboard[0][1].CallbackData != "account:view:3" {
		t.Errorf("back button = %q, want account:view:3", markup.InlineKeyboard[0][1].CallbackData)
	}
	// Icon policy: action button (Config V2Ray) icon-free.
	if strings.ContainsAny(markup.InlineKeyboard[0][0].Text, "📄🔒🔓🗑️") {
		t.Errorf("config button must be icon-free: %q", markup.InlineKeyboard[0][0].Text)
	}
}
