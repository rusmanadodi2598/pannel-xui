// Package config also contains env parsing helpers (split for §1.1 line limit).
//
// @file      internal/config/parse.go
// @for       Low-level env parsing helpers used by Load.
// @uses      os, strconv, strings, time, encoding/base64
// @reason    Keeps config.go under 250 lines while centralizing parsing logic.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     config
// @stability stable
// @since     2026-08-11
package config

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// getEnv returns the env value or a fallback when unset/empty.
func getEnv(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// parseIntEnv parses an integer env var and fails fast on malformed input.
func parseIntEnv(name string, fallback int) (int, error) {
	raw := getEnv(name, "")
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer, got %q", name, raw)
	}
	return n, nil
}

// parseInt parses an integer env value and fails fast with a clean error.
func parseInt(name, raw string) (int, error) {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer, got %q", name, raw)
	}
	return n, nil
}

// parseFloat parses a float env value and fails fast with a clean error.
func parseFloat(name, raw string) (float64, error) {
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number, got %q", name, raw)
	}
	return f, nil
}

// parseBoolEnv parses a boolean env var (true/false/1/0), defaulting on empty.
func parseBoolEnv(name string, fallback bool) (bool, error) {
	raw := getEnv(name, "")
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean, got %q", name, raw)
	}
	return v, nil
}

// parseID parses a Telegram chat/user ID, which may be negative (supergroups).
func parseID(name, raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer, got %q", name, raw)
	}
	return id, nil
}

// parseIDList parses a comma-separated list of Telegram user IDs.
func parseIDList(name, raw string) ([]int64, error) {
	if raw == "" {
		return []int64{}, nil
	}
	parts := strings.Split(raw, ",")
	ids := make([]int64, 0, len(parts))
	for _, p := range parts {
		id, err := parseID(name, strings.TrimSpace(p))
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// parseDayList parses "7,3,1" into a day list, failing fast on bad entries.
func parseDayList(name, raw string) ([]int, error) {
	var days []int
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		d, err := parseInt(name, p)
		if err != nil {
			return nil, err
		}
		days = append(days, d)
	}
	return days, nil
}

// decodeEncryptionKey decodes a base64-encoded 32-byte AES-256 key.
func decodeEncryptionKey(raw string) ([]byte, error) {
	if raw == "" {
		return nil, fmt.Errorf("ENCRYPTION_KEY is required")
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("ENCRYPTION_KEY must be base64 encoded: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("ENCRYPTION_KEY must decode to 32 bytes for AES-256-GCM, got %d", len(key))
	}
	return key, nil
}

// parseLogLevel maps a string level to slog.Level.
func parseLogLevel(raw string) slog.Level {
	switch strings.ToLower(raw) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// loadLocation resolves the configured timezone, failing fast on invalid names.
func loadLocation(name string) (*time.Location, error) {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("invalid TIME_LOCATION %q: %w", name, err)
	}
	return loc, nil
}
