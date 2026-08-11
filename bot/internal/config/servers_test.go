// Package config_test covers the PANEL_N_* multi-server parsing (M4, FR-10).
//
// @file      internal/config/servers_test.go
// @for       Unit tests for ParseServerSeeds: happy path, sequence end, fail-fast.
// @uses      testing, os
// @reason    Guards the multi-panel config contract (AGENTS.md §2.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     config
// @stability stable
// @since     2026-08-11
package config

import (
	"os"
	"strconv"
	"testing"
)

func TestParseServerSeeds_GivenTwoPanels_ThenParsed(t *testing.T) {
	setPanelEnv(t, 1, "Staging ID", "id.kentangtech.id", "443", "admin", "s3cret", "ID", "🇮🇩", "vless,vmess")
	setPanelEnv(t, 2, "SG Edge", "sg.kentangtech.id", "2083", "root", "pw2", "SG", "🇸🇬", "trojan")
	t.Setenv("PANEL_3_HOST", "") // sequence ends here

	seeds, err := ParseServerSeeds()
	if err != nil {
		t.Fatalf("ParseServerSeeds: %v", err)
	}
	if len(seeds) != 2 {
		t.Fatalf("got %d seeds, want 2", len(seeds))
	}

	s1 := seeds[0]
	if s1.Name != "Staging ID" || s1.Host != "id.kentangtech.id" || s1.Port != 443 {
		t.Errorf("seed1 = %+v", s1)
	}
	if s1.Username != "admin" || s1.Password != "s3cret" {
		t.Errorf("seed1 credentials mismatch: %+v", s1)
	}
	if s1.CountryCode != "ID" || !s1.UseSSL || s1.APIPath != "/panel" {
		t.Errorf("seed1 panel fields = %+v", s1)
	}
	if s1.InsecureTLS {
		t.Errorf("seed1 insecure must default false: %+v", s1)
	}
	if len(s1.Protocols) != 2 || s1.Protocols[0] != "vless" {
		t.Errorf("seed1 protocols = %v", s1.Protocols)
	}
	if seeds[1].CountryCode != "SG" || seeds[1].Port != 2083 {
		t.Errorf("seed2 = %+v", seeds[1])
	}
}

func TestParseServerSeeds_GivenInsecureFlag_ThenParsed(t *testing.T) {
	setPanelEnv(t, 1, "SelfSigned", "x.example.com", "443", "a", "b", "ID", "", "")
	t.Setenv("PANEL_1_INSECURE", "true")
	clearPanelEnv(t, 2)
	seeds, err := ParseServerSeeds()
	if err != nil {
		t.Fatalf("ParseServerSeeds: %v", err)
	}
	if len(seeds) != 1 || !seeds[0].InsecureTLS {
		t.Errorf("seeds = %+v, want insecure=true", seeds)
	}
}

func TestParseServerSeeds_GivenNoPanels_ThenEmpty(t *testing.T) {
	clearPanelEnv(t, 1)
	seeds, err := ParseServerSeeds()
	if err != nil {
		t.Fatalf("ParseServerSeeds: %v", err)
	}
	if len(seeds) != 0 {
		t.Fatalf("got %d seeds, want 0", len(seeds))
	}
}

func TestParseServerSeeds_GivenMissingHost_ThenError(t *testing.T) {
	clearPanelEnv(t, 1)
	t.Setenv("PANEL_1_NAME", "Broken")
	// No PANEL_1_HOST → host required.
	if _, err := ParseServerSeeds(); err == nil {
		t.Fatal("expected error for missing PANEL_1_HOST")
	}
}

func TestParseServerSeeds_GivenInvalidPort_ThenError(t *testing.T) {
	setPanelEnv(t, 1, "Bad", "x.example.com", "notaport", "a", "b", "ID", "", "")
	if _, err := ParseServerSeeds(); err == nil {
		t.Fatal("expected error for invalid PANEL_1_PORT")
	}
}

func TestParseServerSeeds_GivenMissingCredentials_ThenError(t *testing.T) {
	setPanelEnv(t, 1, "NoCred", "x.example.com", "443", "", "", "ID", "", "")
	if _, err := ParseServerSeeds(); err == nil {
		t.Fatal("expected error for missing credentials")
	}
}

func TestParseServerSeeds_GivenMissingCountry_ThenError(t *testing.T) {
	setPanelEnv(t, 1, "NoCountry", "x.example.com", "443", "a", "b", "", "", "")
	if _, err := ParseServerSeeds(); err == nil {
		t.Fatal("expected error for missing COUNTRY_CODE")
	}
}

func TestLoad_GivenPanels_ThenConfigCarriesSeeds(t *testing.T) {
	env := validEnv()
	for k, v := range env {
		t.Setenv(k, v)
	}
	setPanelEnv(t, 1, "ID1", "id.example.com", "443", "admin", "pw", "ID", "🇮🇩", "vless")
	clearPanelEnv(t, 2)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Panels) != 1 {
		t.Fatalf("Panels = %d, want 1", len(cfg.Panels))
	}
	if cfg.Panels[0].CountryCode != "ID" {
		t.Errorf("panel country = %q", cfg.Panels[0].CountryCode)
	}
	if cfg.PricingSeedFile != DefaultPricingSeedFile {
		t.Errorf("PricingSeedFile = %q, want %q", cfg.PricingSeedFile, DefaultPricingSeedFile)
	}
}

// setPanelEnv writes one PANEL_N_* block.
func setPanelEnv(t *testing.T, n int, name, host, port, user, pass, country, flag, protocols string) {
	t.Helper()
	p := "PANEL_" + itoa(n) + "_"
	t.Setenv(p+"NAME", name)
	t.Setenv(p+"HOST", host)
	t.Setenv(p+"PORT", port)
	t.Setenv(p+"USERNAME", user)
	t.Setenv(p+"PASSWORD", pass)
	t.Setenv(p+"COUNTRY_CODE", country)
	t.Setenv(p+"FLAG_EMOJI", flag)
	t.Setenv(p+"PROTOCOLS", protocols)
}

// clearPanelEnv removes every key of one PANEL_N_* block.
func clearPanelEnv(t *testing.T, n int) {
	t.Helper()
	p := "PANEL_" + itoa(n) + "_"
	for _, k := range []string{"NAME", "HOST", "PORT", "USERNAME", "PASSWORD", "COUNTRY_CODE", "FLAG_EMOJI", "LOCATION", "PROTOCOLS", "INSECURE"} {
		_ = os.Unsetenv(p + k)
	}
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
