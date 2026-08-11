// Package postgres also hosts GORM models mirroring PRD §13.
//
// @file      internal/repository/postgres/models_user.go
// @for       GORM model of `users` + `vpn_clients` (PRD §13.1/§13.3).
// @uses      time, internal/domain
// @reason    Typed persistence of user & VPN client entities (AGENTS.md §1.4/§1.5).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package postgres

import (
	"time"

	"github.com/kentangtech/bot-order/internal/domain"
)

// User mirrors the `users` table (PRD §13.1). Money fields stay int64 rupiah.
type User struct {
	ID           int64  `gorm:"primaryKey"`
	TelegramID   int64  `gorm:"uniqueIndex;not null"`
	Username     string `gorm:"type:text"`
	FirstName    string `gorm:"type:text"`
	LastName     string `gorm:"type:text"`
	Phone        string `gorm:"type:text"`
	Language     string `gorm:"type:text;not null;default:id"`
	IsActive     bool   `gorm:"not null;default:true"`
	IsBanned     bool   `gorm:"not null;default:false"`
	IsAdmin      bool   `gorm:"not null;default:false"`
	Balance      domain.Money
	TotalSpent   domain.Money
	ReferralCode *string    `gorm:"uniqueIndex"` // NULL (not "") so empty is unique-safe
	ReferredBy   *int64     `gorm:"index"`
	LastActive   *time.Time `gorm:"type:timestamptz"`
	CreatedAt    time.Time  `gorm:"type:timestamptz;not null;default:now()"`
	UpdatedAt    time.Time  `gorm:"type:timestamptz;not null;default:now()"`
}

// TableName keeps GORM on the exact PRD table name.
func (User) TableName() string { return "users" }

// VPNClient mirrors the `vpn_clients` table (PRD §13.3).
type VPNClient struct {
	ID              int64      `gorm:"primaryKey"`
	UserID          int64      `gorm:"index:idx_vpn_clients_user_id;not null"`
	ServerID        int64      `gorm:"not null"`
	InboundID       int        `gorm:"not null"`
	Email           string     `gorm:"uniqueIndex;not null"`
	UUID            string     `gorm:"type:text"`
	Password        string     `gorm:"type:text"`
	Protocol        string     `gorm:"type:text;not null"`
	Flow            string     `gorm:"type:text"`
	TrafficLimit    int64      `gorm:"not null;default:0"`
	TrafficUsed     int64      `gorm:"not null;default:0"`
	TrafficUp       int64      `gorm:"not null;default:0"`
	TrafficDown     int64      `gorm:"not null;default:0"`
	IPLimit         int        `gorm:"not null;default:1"`
	IsBanned        bool       `gorm:"not null;default:false"`
	IsActive        bool       `gorm:"not null;default:true"`
	IsExpired       bool       `gorm:"not null;default:false"`
	IsTrial         bool       `gorm:"not null;default:false"`
	ExpiresAt       *time.Time `gorm:"type:timestamptz"`
	ConfigLink      string     `gorm:"type:text"`
	SubscriptionURL string     `gorm:"type:text"`
	NotifiedExpiry  int        `gorm:"not null;default:0"` // FR-09: ambang hari terakhir dikirim (0/7/3/1)
	LastSync        *time.Time `gorm:"type:timestamptz"`
	LastOnline      *time.Time `gorm:"type:timestamptz"`
	CreatedAt       time.Time  `gorm:"type:timestamptz;not null;default:now()"`
	UpdatedAt       time.Time  `gorm:"type:timestamptz;not null;default:now()"`
}

// TableName keeps GORM on the exact PRD table name.
func (VPNClient) TableName() string { return "vpn_clients" }

// ClientView is the read model for the "Akun Saya" list (FR-08 subset, M4):
// a vpn_clients row joined with its server display fields.
type ClientView struct {
	VPNClient
	ServerName  string
	FlagEmoji   string
	CountryCode string
}
