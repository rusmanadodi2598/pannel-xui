// Package postgres_test also covers the traffic-sync repository (PRD §16.2).
//
// @file      internal/repository/postgres/repo_traffic_test.go
// @for       Integration tests: candidate filters, batch update writes usage + last_online COALESCE.
// @uses      testing, context, time, github.com/kentangtech/bot-order/internal/repository/postgres
// @reason    Verifies the traffic-sync persistence contract on the staging DB.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// seedTrafficServer creates one active panel and returns its id.
func seedTrafficServer(t *testing.T, r *postgres.Repository, name string) int64 {
	t.Helper()
	s := postgres.VPNServer{
		Name: name, Host: name + ".example.com", Port: 443, Username: "admin",
		PasswordEnc: "enc", APIPath: "/panel", UseSSL: true,
		CountryCode: "ID", FlagEmoji: "🇮🇩", Protocols: `["vless"]`,
		IsActive: true, IsOpen: true, UpdatedAt: time.Now(),
	}
	if err := r.Servers().UpsertSeed(context.Background(), s); err != nil {
		t.Fatalf("UpsertSeed: %v", err)
	}
	buyable, err := r.Servers().ListBuyable(context.Background())
	if err != nil || len(buyable) != 1 {
		t.Fatalf("ListBuyable = %v, err %v", buyable, err)
	}
	return buyable[0].ID
}

// addTrafficClient inserts a client row with the given usage and activity flags.
// GORM omits zero-value fields tagged with a default (default:true), so false
// flags are forced via UPDATE — the real mutation path (same as admin toggles).
func addTrafficClient(t *testing.T, r *postgres.Repository, userID, serverID int64, email string, active, expired bool) postgres.VPNClient {
	t.Helper()
	exp := time.Now().Add(30 * 24 * time.Hour)
	c := postgres.VPNClient{
		UserID: userID, ServerID: serverID, InboundID: 1, Email: email,
		Protocol: "vless", ExpiresAt: &exp,
		IsActive: true,
	}
	if err := r.Clients().Create(context.Background(), &c); err != nil {
		t.Fatalf("Client Create %s: %v", email, err)
	}
	if !active {
		if err := r.DB().Model(&postgres.VPNClient{}).Where("id = ?", c.ID).Update("is_active", false).Error; err != nil {
			t.Fatalf("set %s is_active=false: %v", email, err)
		}
	}
	if expired {
		if err := r.DB().Model(&postgres.VPNClient{}).Where("id = ?", c.ID).Update("is_expired", true).Error; err != nil {
			t.Fatalf("set %s is_expired=true: %v", email, err)
		}
	}
	return c
}

func TestClientRepo_TrafficCandidates_GivenFilters_ThenOnlyLiveClients(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	ctx := context.Background()

	u, err := r.User().FindOrCreate(ctx, 889101, "trafik", "Trafik")
	if err != nil {
		t.Fatalf("FindOrCreate: %v", err)
	}
	activeServer := seedTrafficServer(t, r, "ID-02")

	// Server nonaktif → client-nya tidak boleh muncul.
	// Server nonaktif (IsActive=false) → GORM omits false saat INSERT (default:true),
	// jadi set lewat UPDATE — jalur nyata (server health worker, M6).
	inactive := postgres.VPNServer{
		Name: "DOWN", Host: "down.example.com", Port: 443, Username: "admin",
		PasswordEnc: "enc", APIPath: "/panel", UseSSL: true,
		CountryCode: "SG", FlagEmoji: "🇸🇬", Protocols: `["vless"]`,
		IsActive: true, IsOpen: true, UpdatedAt: time.Now(),
	}
	if err := r.Servers().UpsertSeed(ctx, inactive); err != nil {
		t.Fatalf("UpsertSeed inactive: %v", err)
	}
	var inactiveID int64
	{
		var rows []postgres.ServerView
		if err := r.DB().Table("vpn_servers").Select("id").Where("name = ?", "DOWN").Scan(&rows).Error; err != nil {
			t.Fatalf("find inactive server: %v", err)
		}
		inactiveID = rows[0].ID
	}
	if err := r.DB().Model(&postgres.VPNServer{}).Where("id = ?", inactiveID).Update("is_active", false).Error; err != nil {
		t.Fatalf("deactivate server: %v", err)
	}

	live := addTrafficClient(t, r, u.ID, activeServer, "live@vpn.kt", true, false)
	addTrafficClient(t, r, u.ID, activeServer, "off@vpn.kt", false, false) // is_active=false
	addTrafficClient(t, r, u.ID, activeServer, "gone@vpn.kt", true, true)  // is_expired=true
	addTrafficClient(t, r, u.ID, inactiveID, "dark@vpn.kt", true, false)   // server nonaktif

	cands, err := r.Clients().ListTrafficCandidates(ctx, 50)
	if err != nil {
		t.Fatalf("ListTrafficCandidates: %v", err)
	}
	if len(cands) != 1 || cands[0].ClientID != live.ID || cands[0].Email != "live@vpn.kt" {
		t.Fatalf("candidates = %+v, want only live@vpn.kt", cands)
	}
}

func TestClientRepo_TrafficCandidates_GivenLimit_ThenBoundedByOldestSync(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	ctx := context.Background()

	u, _ := r.User().FindOrCreate(ctx, 889102, "rio", "Rio")
	serverID := seedTrafficServer(t, r, "MY-01")
	old := addTrafficClient(t, r, u.ID, serverID, "old@vpn.kt", true, false)
	_ = addTrafficClient(t, r, u.ID, serverID, "new@vpn.kt", true, false)

	// Tandai old.last_sync lama → harus diprioritaskan (NULLS FIRST mengalahkannya,
	// jadi set old.last_sync ke 1 jam lalu dan new tetap NULL → new menang).
	recent := time.Now().Add(-time.Hour)
	if err := r.DB().Table("vpn_clients").Where("id = ?", old.ID).Update("last_sync", recent).Error; err != nil {
		t.Fatalf("set old last_sync: %v", err)
	}

	cands, err := r.Clients().ListTrafficCandidates(ctx, 1)
	if err != nil {
		t.Fatalf("ListTrafficCandidates: %v", err)
	}
	if len(cands) != 1 || cands[0].Email != "new@vpn.kt" {
		t.Fatalf("bounded = %+v, want new@vpn.kt (NULL last_sync first)", cands)
	}
}

func TestClientRepo_SyncTrafficBatch_GivenUpdates_ThenWritesUsageAndCOALESCEOnline(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	ctx := context.Background()

	u, _ := r.User().FindOrCreate(ctx, 889103, "nova", "Nova")
	serverID := seedTrafficServer(t, r, "TH-01")
	c1 := addTrafficClient(t, r, u.ID, serverID, "n1@vpn.kt", true, false)
	c2 := addTrafficClient(t, r, u.ID, serverID, "n2@vpn.kt", true, false)
	c3 := addTrafficClient(t, r, u.ID, serverID, "n3@vpn.kt", true, false)

	// c3 sudah pernah online kemarin — tanpa update, nil harus mempertahankan.
	yesterday := time.Now().Add(-24 * time.Hour)
	if err := r.DB().Table("vpn_clients").Where("id = ?", c3.ID).Update("last_online", yesterday).Error; err != nil {
		t.Fatalf("set c3 last_online: %v", err)
	}

	now := time.Now()
	updates := []postgres.TrafficUpdate{
		{ClientID: c1.ID, Up: 1000, Down: 2000},                   // offline → last_online tetap
		{ClientID: c2.ID, Up: 3000, Down: 4000, LastOnline: &now}, // online → last_online = now
		{ClientID: c3.ID, Up: 5000, Down: 0},                      // offline → last_online lama dipertahankan
	}
	if err := r.Clients().SyncTrafficBatch(ctx, now, updates); err != nil {
		t.Fatalf("SyncTrafficBatch: %v", err)
	}

	var c1row, c2row, c3row postgres.VPNClient
	for _, id := range []int64{c1.ID, c2.ID, c3.ID} {
		var row postgres.VPNClient
		if err := r.DB().Where("id = ?", id).First(&row).Error; err != nil {
			t.Fatalf("load client %d: %v", id, err)
		}
		switch id {
		case c1.ID:
			c1row = row
		case c2.ID:
			c2row = row
		case c3.ID:
			c3row = row
		}
	}

	if c1row.TrafficUp != 1000 || c1row.TrafficDown != 2000 || c1row.TrafficUsed != 3000 {
		t.Errorf("c1 usage = up%d/down%d/used%d, want 1000/2000/3000",
			c1row.TrafficUp, c1row.TrafficDown, c1row.TrafficUsed)
	}
	if c1row.LastSync == nil {
		t.Error("c1 LastSync = nil, want syncedAt")
	}
	if c2row.LastOnline == nil {
		t.Error("c2 LastOnline = nil, want set (online)")
	}
	// DB timestamptz memotong ke mikrodetik — bandingkan dalam rentang, bukan Equal.
	if c3row.LastOnline == nil ||
		c3row.LastOnline.Before(now.Add(-25*time.Hour)) ||
		c3row.LastOnline.After(now.Add(-23*time.Hour)) {
		t.Errorf("c3 LastOnline = %v, want ~24h ago kept (COALESCE)", c3row.LastOnline)
	}
}
