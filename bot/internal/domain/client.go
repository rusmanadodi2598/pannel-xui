// Package domain holds entities and value objects (DDD, AGENTS.md §2.2).
//
// @file      internal/domain/client.go
// @for       VPNClient entity persisted in vpn_clients (PRD §13.3).
// @uses      time
// @reason    Rich domain type for client lifecycle used by order & account flows (M4).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     domain
// @stability stable
// @since     2026-08-11
package domain

import (
	"fmt"
	"time"
)

// VPNClient is a client provisioned on an X-UI panel (PRD §13.3).
type VPNClient struct {
	ID              int64
	UserID          int64
	ServerID        int64
	InboundID       int
	Email           string
	UUID            string
	Password        string
	Protocol        string
	Flow            string
	TrafficLimit    int64
	TrafficUsed     int64
	TrafficUp       int64
	TrafficDown     int64
	IPLimit         int
	IsBanned        bool
	IsActive        bool
	IsExpired       bool
	IsTrial         bool
	ExpiresAt       *time.Time
	ConfigLink      string
	SubscriptionURL string
	LastOnline      *time.Time
	CreatedAt       time.Time
	ServerName      string
	FlagEmoji       string
	CountryCode     string
}

// NewVPNClient builds the client record after a successful panel addClient.
func NewVPNClient(userID, serverID int64, inboundID int, email, protocol, uuid, password string, days int, trafficGB, ipLimit int64) (*VPNClient, error) {
	if email == "" {
		return nil, fmt.Errorf("client email is required")
	}
	if protocol == "" {
		return nil, fmt.Errorf("client protocol is required")
	}
	expiry := time.Now().AddDate(0, 0, days)
	return &VPNClient{
		UserID:       userID,
		ServerID:     serverID,
		InboundID:    inboundID,
		Email:        email,
		UUID:         uuid,
		Password:     password,
		Protocol:     protocol,
		TrafficLimit: trafficGB * 1024 * 1024 * 1024, // GB → bytes
		IPLimit:      int(ipLimit),
		IsActive:     true,
		IsTrial:      false,
		ExpiresAt:    &expiry,
	}, nil
}

// PanelClient is the result of a successful panel addClient (FR-04).
// Shared by server & order services so no cross-service type coupling exists.
type PanelClient struct {
	InboundID int
	Email     string
	UUID      string
	Password  string
	Protocol  string
}

// Expired reports whether the client is past its expiry.
func (c *VPNClient) Expired(now time.Time) bool {
	return c.ExpiresAt != nil && !c.ExpiresAt.After(now)
}

// TimeRemaining returns the duration until expiry (negative when expired).
func (c *VPNClient) TimeRemaining(now time.Time) time.Duration {
	if c.ExpiresAt == nil {
		return 0
	}
	return c.ExpiresAt.Sub(now)
}
