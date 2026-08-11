// Package postgres also hosts the vpn_servers GORM model.
//
// @file      internal/repository/postgres/models_server.go
// @for       GORM model of `vpn_servers` (PRD §13.2) + ServerView read model.
// @uses      time, gorm.io/gorm
// @reason    Typed persistence of X-UI panel instances (FR-10, M4).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package postgres

import (
	"time"
)

// VPNServer mirrors the `vpn_servers` table (PRD §13.2).
// PasswordEnc holds the AES-256-GCM sealed password (never plaintext).
type VPNServer struct {
	ID                 int64      `gorm:"primaryKey"`
	Name               string     `gorm:"type:text;not null"`
	Host               string     `gorm:"type:text;not null"`
	Port               int        `gorm:"not null"`
	Username           string     `gorm:"type:text;not null"`
	PasswordEnc        string     `gorm:"type:text;not null"`
	APIPath            string     `gorm:"type:text;not null;default:/panel"`
	UseSSL             bool       `gorm:"not null;default:false"`
	InsecureTLS        bool       `gorm:"not null;default:false"` // skip TLS verify (opt-in only)
	CountryCode        string     `gorm:"type:text;not null"`
	FlagEmoji          string     `gorm:"type:text;not null;default:''"`
	Location           string     `gorm:"type:text"`
	MaxClients         *int       `gorm:"type:int"`
	CurrentClients     int        `gorm:"not null;default:0"`
	IsActive           bool       `gorm:"not null;default:true"`
	IsPremium          bool       `gorm:"not null;default:false"`
	IsOpen             bool       `gorm:"not null;default:true"`
	Priority           int        `gorm:"not null;default:0"`
	MaintenanceMessage string     `gorm:"type:text"`
	Protocols          string     `gorm:"type:jsonb;not null;default:'[]'"`
	LastSync           *time.Time `gorm:"type:timestamptz"`
	LastHealthCheck    *time.Time `gorm:"type:timestamptz"`
	HealthStatus       string     `gorm:"type:text;not null;default:unknown"`
	CreatedAt          time.Time  `gorm:"type:timestamptz;not null;default:now()"`
	UpdatedAt          time.Time  `gorm:"type:timestamptz;not null;default:now()"`
}

// TableName keeps GORM on the exact PRD table name.
func (VPNServer) TableName() string { return "vpn_servers" }

// ServerView is the buy-menu read model: server visible to users, no secrets.
type ServerView struct {
	ID          int64
	Name        string
	FlagEmoji   string
	CountryCode string
	Location    string
	Protocols   string
}
