// Package postgres also hosts the admin server-management queries (FR-11, v1.40).
//
// @file      internal/repository/postgres/server_admin.go
// @for       Admin ops: list all servers, toggle open/active, create a panel row.
// @uses      context, errors, fmt, gorm.io/gorm
// @reason    FR-11 server management — the buy menu only reads active+open;
// the admin panel must see every server and flip sellable/active flags.
// Split from server_repo.go for the §1.1 line limit.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-12
package postgres

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// ServerAdminView is the admin read model: every server with its sellable state
// and health, never exposing credentials.
type ServerAdminView struct {
	ID             int64
	Name           string
	Host           string
	Port           int
	CountryCode    string
	FlagEmoji      string
	Location       string
	IsActive       bool
	IsOpen         bool
	HealthStatus   string
	Protocols      string
	CurrentClients int
}

// ListAll returns every server for the admin panel (active and inactive),
// ordered by country then name — the buy menu stays on ListBuyable.
func (r *ServerRepo) ListAll(ctx context.Context) ([]ServerAdminView, error) {
	var rows []ServerAdminView
	err := r.db.WithContext(ctx).Table("vpn_servers").
		Select("id, name, host, port, country_code, flag_emoji, location, " +
			"is_active, is_open, health_status, protocols, current_clients").
		Order("country_code, name, id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("listing all servers: %w", err)
	}
	return rows, nil
}

// SetOpen flips the sellable flag (is_open) — FR-11 buka/tutup penjualan.
// A server that is open but inactive is NOT buyable (ListBuyable ANDs both).
func (r *ServerRepo) SetOpen(ctx context.Context, id int64, open bool) error {
	res := r.db.WithContext(ctx).Model(&VPNServer{}).
		Where("id = ?", id).
		Update("is_open", open)
	if res.Error != nil {
		return fmt.Errorf("setting server %d is_open: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrServerNotFound
	}
	return nil
}

// SetActive flips the server's active flag (is_active) — nonaktifkan/aktifkan
// server. Inactive servers disappear from buy + traffic sweep, but their
// existing clients stay intact in the DB.
func (r *ServerRepo) SetActive(ctx context.Context, id int64, active bool) error {
	res := r.db.WithContext(ctx).Model(&VPNServer{}).
		Where("id = ?", id).
		Update("is_active", active)
	if res.Error != nil {
		return fmt.Errorf("setting server %d is_active: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrServerNotFound
	}
	return nil
}

// Create inserts a brand-new panel row (admin add-server, FR-11). The password
// must already be encrypted by the caller (serversvc seals it with the box).
func (r *ServerRepo) Create(ctx context.Context, s *VPNServer) error {
	if s.Protocols == "" {
		s.Protocols = "[]"
	}
	if err := r.db.WithContext(ctx).Create(s).Error; err != nil {
		return fmt.Errorf("creating server %s: %w", s.Name, err)
	}
	return nil
}

// FindByHostPort returns an existing server matching host+port+username, used
// by admin add-server to avoid duplicate panels (mirrors UpsertSeed matching).
func (r *ServerRepo) FindByHostPort(ctx context.Context, host string, port int, username string) (*VPNServer, error) {
	var s VPNServer
	err := r.db.WithContext(ctx).
		Where("host = ? AND port = ? AND username = ?", host, port, username).
		First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finding server by host: %w", err)
	}
	return &s, nil
}
