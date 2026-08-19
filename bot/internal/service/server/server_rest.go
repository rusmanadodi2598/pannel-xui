// Package serversvc also hosts the REST admin server ops (PRD §26.5).
//
// @file      internal/service/server/server_rest.go
// @for       REST admin: get one server (no secrets), patch fields, guarded
// delete, and a single-panel health probe.
// @uses      context, errors, fmt, strings, time, internal/repository/postgres
// @reason    The /api/v1/servers CRUD shares the SAME sealed-password path as
// env seeding and chat add-server; the delete guard and live health probe keep
// the REST surface from becoming a footgun (AGENTS.md §1.6/§1.7).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-18
package serversvc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
	"github.com/kentangtech/bot-order/internal/repository/xui"
)

// Health status literals shared with the /servers/{id}/health endpoint.
const (
	HealthOK   = "ok"
	HealthDown = "down"
)

// UpdateServerInput is the PATCH /servers/{id} form (pointer = change field).
// Password, when present and non-empty, is re-sealed before the store update.
type UpdateServerInput struct {
	ID          int64
	Name        *string
	Host        *string
	Port        *int
	Username    *string
	Password    *string
	APIPath     *string
	UseSSL      *bool
	CountryCode *string
	FlagEmoji   *string
	Location    *string
	IsActive    *bool
	IsOpen      *bool
}

// GetAdminByID returns one panel as the credential-free admin view.
func (s *Service) GetAdminByID(ctx context.Context, id int64) (postgres.ServerAdminView, error) {
	return s.store.GetAdminByID(ctx, id)
}

// UpdateServer patches one server. Provided fields are trimmed/validated the
// same way AddServer does; a password is re-encrypted with the shared box.
func (s *Service) UpdateServer(ctx context.Context, in UpdateServerInput) error {
	if in.Port != nil && (*in.Port <= 0 || *in.Port > 65535) {
		return fmt.Errorf("update server: invalid port %d", *in.Port)
	}
	if in.CountryCode != nil && strings.TrimSpace(*in.CountryCode) == "" {
		return fmt.Errorf("update server: country code is required")
	}

	up := postgres.ServerUpdate{
		Name:        trimPtr(in.Name),
		Host:        trimPtr(in.Host),
		Port:        in.Port,
		Username:    trimPtr(in.Username),
		APIPath:     trimPtr(in.APIPath),
		UseSSL:      in.UseSSL,
		CountryCode: upperPtr(in.CountryCode),
		FlagEmoji:   trimPtr(in.FlagEmoji),
		Location:    trimPtr(in.Location),
		IsActive:    in.IsActive,
		IsOpen:      in.IsOpen,
	}
	if in.Password != nil && *in.Password != "" {
		enc, err := s.box.Encrypt(*in.Password)
		if err != nil {
			return fmt.Errorf("encrypting new panel password: %w", err)
		}
		up.PasswordEnc = &enc
	}
	if err := s.store.UpdateServer(ctx, in.ID, up); err != nil {
		return fmt.Errorf("updating server %d: %w", in.ID, err)
	}
	return nil
}

// DeleteServer removes a panel only when it has no clients (the repo guard).
// A served panel must be deactivated instead — return the guard error so the
// handler can map it to 409 CONFLICT.
func (s *Service) DeleteServer(ctx context.Context, id int64) error {
	if err := s.store.DeleteServer(ctx, id); err != nil {
		if errors.Is(err, postgres.ErrServerHasClients) {
			return err
		}
		return fmt.Errorf("deleting server %d: %w", id, err)
	}
	return nil
}

// statusProber is the panel health surface (xui.Client implements it).
type statusProber interface {
	GetServerStatus(ctx context.Context) (xui.Status, error)
}

// CheckHealth pings one panel live and returns "ok" or "down". It does not
// persist the result (the worker owns health_status); it is a point-in-time
// probe for the operator. A per-call budget bounds the outbound panel call.
func (s *Service) CheckHealth(ctx context.Context, id int64) (string, error) {
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	panel, err := s.statusFactory(probeCtx, id)
	if err != nil {
		return HealthDown, nil // unreachable = down, not an API error
	}
	if _, err := panel.GetServerStatus(probeCtx); err != nil {
		return HealthDown, nil
	}
	return HealthOK, nil
}

func trimPtr(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	return &v
}

func upperPtr(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.ToUpper(strings.TrimSpace(*s))
	return &v
}
