// Package postgres also hosts the REST admin server queries (PRD §26.5).
//
// @file      internal/repository/postgres/server_rest.go
// @for       REST admin: get one server view (no secrets), patch fields, guarded delete.
// @uses      context, errors, fmt, time, gorm.io/gorm
// @reason    The /api/v1/servers CRUD reads a credential-free view and never
// exposes password_enc; delete is guarded so an ON DELETE CASCADE on
// vpn_clients.server_id cannot silently wipe user accounts (AGENTS.md §1.7).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability experimental
// @since     2026-08-18
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// ErrServerHasClients is returned when a delete would cascade into vpn_clients.
var ErrServerHasClients = errors.New("server has clients")

// ServerUpdate is the set of fields the PATCH /servers/{id} endpoint may
// change. Nil pointers leave the column untouched; PasswordEnc is the
// already-sealed password (the service encrypts it, never the caller).
type ServerUpdate struct {
	Name        *string
	Host        *string
	Port        *int
	Username    *string
	PasswordEnc *string
	APIPath     *string
	UseSSL      *bool
	CountryCode *string
	FlagEmoji   *string
	Location    *string
	IsActive    *bool
	IsOpen      *bool
}

// GetAdminByID returns one server as the credential-free admin view (PRD §26.5
// detail) — password_enc and username are never selected.
func (r *ServerRepo) GetAdminByID(ctx context.Context, id int64) (ServerAdminView, error) {
	var v ServerAdminView
	err := r.db.WithContext(ctx).Table("vpn_servers").
		Select("id, name, host, port, country_code, flag_emoji, location, "+
			"is_active, is_open, health_status, protocols, current_clients").
		Where("id = ?", id).
		Limit(1).
		Scan(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ServerAdminView{}, ErrServerNotFound
	}
	if err != nil {
		return ServerAdminView{}, fmt.Errorf("getting server view %d: %w", id, err)
	}
	if v.ID == 0 {
		return ServerAdminView{}, ErrServerNotFound
	}
	return v, nil
}

// UpdateServer patches only the provided (non-nil) fields of one server row.
// RowsAffected 0 means the id does not exist (ErrServerNotFound).
func (r *ServerRepo) UpdateServer(ctx context.Context, id int64, up ServerUpdate) error {
	cols := map[string]any{"updated_at": time.Now()}
	if up.Name != nil {
		cols["name"] = *up.Name
	}
	if up.Host != nil {
		cols["host"] = *up.Host
	}
	if up.Port != nil {
		cols["port"] = *up.Port
	}
	if up.Username != nil {
		cols["username"] = *up.Username
	}
	if up.PasswordEnc != nil {
		cols["password_enc"] = *up.PasswordEnc
	}
	if up.APIPath != nil {
		cols["api_path"] = *up.APIPath
	}
	if up.UseSSL != nil {
		cols["use_ssl"] = *up.UseSSL
	}
	if up.CountryCode != nil {
		cols["country_code"] = *up.CountryCode
	}
	if up.FlagEmoji != nil {
		cols["flag_emoji"] = *up.FlagEmoji
	}
	if up.Location != nil {
		cols["location"] = *up.Location
	}
	if up.IsActive != nil {
		cols["is_active"] = *up.IsActive
	}
	if up.IsOpen != nil {
		cols["is_open"] = *up.IsOpen
	}
	res := r.db.WithContext(ctx).Model(&VPNServer{}).Where("id = ?", id).Updates(cols)
	if res.Error != nil {
		return fmt.Errorf("updating server %d: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrServerNotFound
	}
	return nil
}

// DeleteServer hard-deletes a server only when no vpn_clients row references it
// (vpn_clients.server_id is ON DELETE CASCADE — deleting a served panel would
// wipe its accounts). A served panel must be deactivated via is_active=false
// instead (AGENTS.md §1.7: no accidental destructive cascade).
func (r *ServerRepo) DeleteServer(ctx context.Context, id int64) error {
	var n int64
	if err := r.db.WithContext(ctx).Table("vpn_clients").
		Where("server_id = ?", id).Count(&n).Error; err != nil {
		return fmt.Errorf("counting clients for server %d: %w", id, err)
	}
	if n > 0 {
		return ErrServerHasClients
	}
	res := r.db.WithContext(ctx).Delete(&VPNServer{}, id)
	if res.Error != nil {
		return fmt.Errorf("deleting server %d: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrServerNotFound
	}
	return nil
}
