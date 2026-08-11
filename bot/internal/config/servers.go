// Package config also hosts the multi-server seed parsing (FR-10, M4).
//
// @file      internal/config/servers.go
// @for       Parse PANEL_N_* env pairs into typed ServerSeed values + PRICING_SEED_FILE path.
// @uses      os, fmt, strconv, strings
// @reason    Panel credentials are dynamic (many X-UI instances) and arrive via
//
//	numbered env blocks; business code never reads raw env vars (AGENTS.md §1.4).
//
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     config
// @stability stable
// @since     2026-08-11
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// DefaultPricingSeedFile is where the initial pricing JSON lives (PRD §13.7).
const DefaultPricingSeedFile = "seed/pricing.json"

// ServerSeed is one X-UI panel instance read from the PANEL_N_* env block.
// The password is plaintext at config time only; it is AES-256-GCM encrypted
// before reaching the database (service/server, PRD §15.1).
type ServerSeed struct {
	Index       int
	Name        string
	Host        string
	Port        int
	Username    string
	Password    string
	APIPath     string
	UseSSL      bool
	InsecureTLS bool // skip TLS verify for self-signed staging panels (opt-in)
	CountryCode string
	FlagEmoji   string
	Location    string
	Priority    int
	Protocols   []string
}

// ParseServerSeeds reads every consecutive PANEL_1_* .. PANEL_N_* env block.
// A missing key ends the sequence; a malformed value fails fast (AGENTS.md §1.4).
func ParseServerSeeds() ([]ServerSeed, error) {
	var seeds []ServerSeed
	for i := 1; ; i++ {
		prefix := fmt.Sprintf("PANEL_%d_", i)
		if os.Getenv(prefix+"HOST") == "" && os.Getenv(prefix+"NAME") == "" {
			break // sequence ended
		}
		seed, err := parseOneSeed(i, prefix)
		if err != nil {
			return nil, err
		}
		seeds = append(seeds, seed)
	}
	return seeds, nil
}

func parseOneSeed(index int, prefix string) (ServerSeed, error) {
	seed := ServerSeed{
		Index:       index,
		Name:        getEnv(prefix+"NAME", fmt.Sprintf("Panel %d", index)),
		Host:        os.Getenv(prefix + "HOST"),
		Port:        443,
		Username:    os.Getenv(prefix + "USERNAME"),
		Password:    os.Getenv(prefix + "PASSWORD"),
		APIPath:     getEnv(prefix+"API_PATH", "/panel"),
		UseSSL:      getEnv(prefix+"USE_SSL", "true") == "true",
		InsecureTLS: getEnv(prefix+"INSECURE", "false") == "true",
		CountryCode: strings.ToUpper(getEnv(prefix+"COUNTRY_CODE", "")),
		FlagEmoji:   os.Getenv(prefix + "FLAG_EMOJI"),
		Location:    os.Getenv(prefix + "LOCATION"),
		Priority:    0,
	}

	if seed.Host == "" {
		return ServerSeed{}, fmt.Errorf("%sHOST is required", prefix)
	}
	if p := os.Getenv(prefix + "PORT"); p != "" {
		port, err := strconv.Atoi(p)
		if err != nil || port <= 0 || port > 65535 {
			return ServerSeed{}, fmt.Errorf("%sPORT must be a valid port, got %q", prefix, p)
		}
		seed.Port = port
	}
	if pr := os.Getenv(prefix + "PRIORITY"); pr != "" {
		priority, err := strconv.Atoi(pr)
		if err != nil {
			return ServerSeed{}, fmt.Errorf("%sPRIORITY must be an integer, got %q", prefix, pr)
		}
		seed.Priority = priority
	}
	if seed.Username == "" || seed.Password == "" {
		return ServerSeed{}, fmt.Errorf("%sUSERNAME and %sPASSWORD are required", prefix, prefix)
	}
	if seed.CountryCode == "" {
		return ServerSeed{}, fmt.Errorf("%sCOUNTRY_CODE is required (e.g. ID, SG, JP)", prefix)
	}
	if pro := os.Getenv(prefix + "PROTOCOLS"); pro != "" {
		for _, p := range strings.Split(pro, ",") {
			if p = strings.TrimSpace(p); p != "" {
				seed.Protocols = append(seed.Protocols, p)
			}
		}
	}
	return seed, nil
}
