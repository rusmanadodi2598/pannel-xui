// Package domain test covers the trial client entity builder.
//
// @file      internal/domain/client_trial_test.go
// @for       NewTrialClient: is_trial flag, hour-based expiry, quota/limit mapping.
// @uses      testing, time
// @reason    Trial rows must never be mistaken for paid accounts (FR-07 AC-2)
// and expire in hours — the builder locks that contract.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     domain
// @stability experimental
// @since     2026-08-11
package domain

import (
	"testing"
	"time"
)

func TestNewTrialClient_GivenValid_ThenTrialFlagsAndHourExpiry(t *testing.T) {
	before := time.Now()
	c, err := NewTrialClient(9, 3, 5, "trial@vpn.kt", "vless", "uuid-1", "", 1, 1, 1)
	if err != nil {
		t.Fatalf("NewTrialClient: %v", err)
	}
	if !c.IsTrial {
		t.Error("IsTrial = false, want true (FR-07 AC-2)")
	}
	if !c.IsActive {
		t.Error("IsActive = false, want true")
	}
	if c.ExpiresAt == nil || c.ExpiresAt.Sub(before) < 50*time.Minute || c.ExpiresAt.Sub(before) > 70*time.Minute {
		t.Errorf("ExpiresAt = %v, want ~1 hour from now", c.ExpiresAt)
	}
	if c.TrafficLimit != 1024*1024*1024 {
		t.Errorf("TrafficLimit = %d, want 1 GB in bytes", c.TrafficLimit)
	}
	if c.IPLimit != 1 {
		t.Errorf("IPLimit = %d, want 1", c.IPLimit)
	}
	if c.UserID != 9 || c.ServerID != 3 || c.InboundID != 5 || c.Email != "trial@vpn.kt" {
		t.Errorf("row fields wrong: %+v", c)
	}
}

func TestNewTrialClient_GivenEmptyEmail_ThenError(t *testing.T) {
	if _, err := NewTrialClient(9, 3, 5, "", "vless", "uuid", "", 1, 1, 1); err == nil {
		t.Fatal("expected error for empty email")
	}
}

func TestNewTrialClient_GivenEmptyProtocol_ThenError(t *testing.T) {
	if _, err := NewTrialClient(9, 3, 5, "t@vpn.kt", "", "uuid", "", 1, 1, 1); err == nil {
		t.Fatal("expected error for empty protocol")
	}
}

func TestNewTrialClient_GivenHours_ThenExpiryScales(t *testing.T) {
	before := time.Now()
	c, _ := NewTrialClient(9, 3, 5, "t@vpn.kt", "trojan", "", "pw", 2, 1, 1)
	if c.ExpiresAt.Sub(before) < 110*time.Minute {
		t.Errorf("2-hour trial expiry too short: %v", c.ExpiresAt.Sub(before))
	}
}
