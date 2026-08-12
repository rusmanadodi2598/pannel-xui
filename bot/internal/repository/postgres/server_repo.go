// Package postgres also hosts the vpn_servers repository.
//
// @file      internal/repository/postgres/server_repo.go
// @for       Seed X-UI panel instances + query buyable/selected servers (PRD §13.2, FR-10).
// @uses      context, errors, fmt, gorm.io/gorm, encoding/json
// @reason    Multi-panel support: servers live in DB, seeded idempotently at boot (M4).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// ErrServerNotFound is returned when a server id does not exist.
var ErrServerNotFound = errors.New("server not found")

// ServerRepo persists X-UI panel instances.
type ServerRepo struct{ db *gorm.DB }

// NewServerRepo builds the repository on the shared GORM handle.
func NewServerRepo(db *gorm.DB) *ServerRepo { return &ServerRepo{db: db} }

// UpsertSeed creates or updates one panel seeded from env (matched on host+username).
func (r *ServerRepo) UpsertSeed(ctx context.Context, s VPNServer) error {
	protocols := s.Protocols
	if protocols == "" {
		protocols = "[]"
	}
	var existing VPNServer
	err := r.db.WithContext(ctx).
		Where("host = ? AND port = ? AND username = ?", s.Host, s.Port, s.Username).
		First(&existing).Error
	switch {
	case err == nil:
		s.ID = existing.ID
		s.CreatedAt = existing.CreatedAt
		return r.db.WithContext(ctx).Model(&existing).
			Updates(map[string]any{
				"name": s.Name, "password_enc": s.PasswordEnc, "api_path": s.APIPath,
				"use_ssl": s.UseSSL, "insecure_tls": s.InsecureTLS,
				"country_code": s.CountryCode, "flag_emoji": s.FlagEmoji,
				"location": s.Location, "priority": s.Priority, "protocols": protocols,
				"is_active": s.IsActive, "is_open": s.IsOpen, "updated_at": s.UpdatedAt,
			}).Error
	case errors.Is(err, gorm.ErrRecordNotFound):
		s.Protocols = protocols
		return r.db.WithContext(ctx).Create(&s).Error
	default:
		return fmt.Errorf("reading server seed: %w", err)
	}
}

// ListBuyable returns active & open panels for the buy menu (FR-03, FR-10),
// ordered by country then priority — never exposes credentials. Panels marked
// "down" by the health-check worker are excluded: server mati tidak dijual
// (PRD §17). Any other health (NULL, default 'unknown', 'ok') stays sellable
// so a fresh boot before the first health sweep is not hidden.
func (r *ServerRepo) ListBuyable(ctx context.Context) ([]ServerView, error) {
	var rows []ServerView
	err := r.db.WithContext(ctx).Table("vpn_servers").
		Select("id, name, flag_emoji, country_code, location, protocols").
		Where("is_active = true AND is_open = true" +
			" AND health_status IS DISTINCT FROM 'down'").
		Order("country_code, priority DESC, id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("listing buyable servers: %w", err)
	}
	return rows, nil
}

// GetByID loads one panel including password_enc (gateway use only).
func (r *ServerRepo) GetByID(ctx context.Context, id int64) (*VPNServer, error) {
	var s VPNServer
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrServerNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting server %d: %w", id, err)
	}
	return &s, nil
}

// ProtocolsList decodes the JSONB protocols column into a slice.
func (s *VPNServer) ProtocolsList() []string {
	var out []string
	_ = json.Unmarshal([]byte(s.Protocols), &out)
	return out
}
