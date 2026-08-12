// Package serversvc also hosts the admin server-management ops (FR-11, v1.40).
//
// @file      internal/service/server/admin.go
// @for       Admin: list all servers, toggle open/active, add a panel (encrypted).
// @uses      context, fmt, time, internal/repository/postgres
// @reason    FR-11 server management reuses the SAME encrypted-password path
// as env seeding (box.Encrypt) so panels added via chat are just as safe.
// Split from server.go for the §1.1 line limit.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-12
package serversvc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// NewServerInput is the admin add-server form (FR-11): plaintext fields the
// admin typed in chat — the password is sealed before reaching the store.
type NewServerInput struct {
	Name        string
	Host        string
	Port        int
	Username    string
	Password    string
	APIPath     string
	UseSSL      bool
	CountryCode string
	FlagEmoji   string
	Location    string
	Protocols   []string
}

// ListAll returns every panel for the admin view (active + inactive).
func (s *Service) ListAll(ctx context.Context) ([]postgres.ServerAdminView, error) {
	return s.store.ListAll(ctx)
}

// SetOpen toggles the sellable flag (buka/tutup penjualan, FR-11).
func (s *Service) SetOpen(ctx context.Context, id int64, open bool) error {
	if err := s.store.SetOpen(ctx, id, open); err != nil {
		return fmt.Errorf("toggling server %d open: %w", id, err)
	}
	return nil
}

// SetActive toggles the server's active flag (nonaktifkan/aktifkan, FR-11).
func (s *Service) SetActive(ctx context.Context, id int64, active bool) error {
	if err := s.store.SetActive(ctx, id, active); err != nil {
		return fmt.Errorf("toggling server %d active: %w", id, err)
	}
	return nil
}

// AddServer creates a new panel from the admin form: validates required
// fields, rejects an existing host+port+username, seals the password with the
// same AES-256-GCM box used at seed time, then inserts the row (FR-11).
func (s *Service) AddServer(ctx context.Context, in NewServerInput) (int64, error) {
	in.APIPath = strings.TrimSpace(in.APIPath)
	if in.APIPath == "" {
		in.APIPath = "/panel"
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Host = strings.TrimSpace(in.Host)
	in.Username = strings.TrimSpace(in.Username)
	if in.Name == "" || in.Host == "" || in.Username == "" || in.Password == "" {
		return 0, fmt.Errorf("add server: name, host, username and password are required")
	}
	if in.Port <= 0 || in.Port > 65535 {
		return 0, fmt.Errorf("add server: invalid port %d", in.Port)
	}
	if strings.TrimSpace(in.CountryCode) == "" {
		return 0, fmt.Errorf("add server: country code is required")
	}

	dup, err := s.store.FindByHostPort(ctx, in.Host, in.Port, in.Username)
	if err != nil {
		return 0, err
	}
	if dup != nil {
		return 0, fmt.Errorf("server %s:%d sudah terdaftar", in.Host, in.Port)
	}

	enc, err := s.box.Encrypt(in.Password)
	if err != nil {
		return 0, fmt.Errorf("encrypting new panel password: %w", err)
	}
	protocols, _ := json.Marshal(in.Protocols)
	row := &postgres.VPNServer{
		Name:        in.Name,
		Host:        in.Host,
		Port:        in.Port,
		Username:    in.Username,
		PasswordEnc: enc,
		APIPath:     in.APIPath,
		UseSSL:      in.UseSSL,
		CountryCode: strings.ToUpper(strings.TrimSpace(in.CountryCode)),
		FlagEmoji:   strings.TrimSpace(in.FlagEmoji),
		Location:    strings.TrimSpace(in.Location),
		IsActive:    true,
		IsOpen:      true,
		Protocols:   string(protocols),
		UpdatedAt:   time.Now(),
	}
	if err := s.store.Create(ctx, row); err != nil {
		return 0, err
	}
	return row.ID, nil
}
