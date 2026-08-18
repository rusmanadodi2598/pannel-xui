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
	"strings"
	"time"
)

// VPNClient is a client provisioned on an X-UI panel (PRD §13.3).
type VPNClient struct {
	ID                  int64
	UserID              int64
	ServerID            int64
	InboundID           int
	Email               string
	UUID                string
	Password            string
	Protocol            string
	Flow                string
	TrafficLimit        int64
	TrafficUsed         int64
	TrafficUp           int64
	TrafficDown         int64
	IPLimit             int
	IsBanned            bool
	IsActive            bool
	IsExpired           bool
	IsTrial             bool
	ExpiresAt           *time.Time
	ConfigLink          string
	SubscriptionURL     string
	SubscriptionJSONURL string // FR-13: JSON/Clash sub (kosong bila disabled)
	SubID               string // FR-13: subId yang dikirim ke panel (basis sub URL)
	InboundNetwork      string // transport asli inbound (ws/grpc/…) — path dinamis v1.27
	InboundPath         string // path asli (wsSettings.path / grpcSettings.serviceName)
	LastOnline          *time.Time
	CreatedAt           time.Time
	ServerName          string
	FlagEmoji           string
	CountryCode         string
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
// ConfigLink is the share URI (vless:// etc.) the bot builds itself because the
// panel's sub server may be disabled (M7 detail/export feature). InboundNetwork
// + InboundPath mirror the inbound's real transport (ws/grpc + path) so the
// dual TLS/non-TLS config links use the actual path per inbound (v1.27).
// SubID mirrors the subId the bot sent to the panel (FR-13): the order flow
// persists it and builds the subscription URL from it.
type PanelClient struct {
	InboundID      int
	Email          string
	UUID           string
	Password       string
	Protocol       string
	ConfigLink     string
	InboundNetwork string
	InboundPath    string
	SubID          string
}

// PreparedClient is a client fully prepared for panel provisioning: the
// bot-side record (for the vpn_clients row) plus the panel commit parameters.
// It is built BEFORE any panel mutation so the order flow can persist the row
// and debit balance first, then commit to the panel — a panel failure then only
// needs a refund + row delete, never an orphaned active account (debit-first,
// parity renewal v1.37).
type PreparedClient struct {
	Panel     PanelClient // bot-side record (creds, config link, subId, network/path)
	ExpiryMs  int64       // exact panel expiry (ms epoch) — commit reuses it
	TrafficGB int64       // quota bytes for the panel spec
	IPLimit   int64       // per-client IP limit for the panel spec
}

// NewTrialClient builds a short-lived trial client record (FR-07 AC-2):
// is_trial=true, quota 1 GB / 1 IP (default), expiry = now + hours.
func NewTrialClient(userID, serverID int64, inboundID int, email, protocol, uuid, password string, hours int, trafficGB, ipLimit int64) (*VPNClient, error) {
	if email == "" {
		return nil, fmt.Errorf("client email is required")
	}
	if protocol == "" {
		return nil, fmt.Errorf("client protocol is required")
	}
	expiry := time.Now().Add(time.Duration(hours) * time.Hour)
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
		IsTrial:      true,
		ExpiresAt:    &expiry,
	}, nil
}

// PanelClientKey returns the credential x-ui's API uses to identify a client
// (verified from web/service/inbound.go UpdateInboundClient + DelInboundClient):
//
//	vless/vmess → the client id (UUID), trojan → password, hysteria → auth,
//	shadowsocks → email (the panel keys ss clients by email, not password).
//
// It is shared by renew, delete and any future panel-keyed operation so the
// per-protocol mapping stays in one place (v1.38).
func PanelClientKey(protocol, uuid, password, email string) string {
	switch strings.ToLower(protocol) {
	case "vless", "vmess":
		return uuid
	case "shadowsocks":
		return email
	default: // trojan, hysteria, hysteria2, unknown
		return password
	}
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
