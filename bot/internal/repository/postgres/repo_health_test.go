// Package postgres_test also covers the health-check & trial-cleanup queries.
//
// @file      internal/repository/postgres/repo_health_test.go
// @for       Integration tests: health target list/update, buyable filter, trial cleanup candidates.
// @uses      testing, context, time, github.com/kentangtech/bot-order/internal/repository/postgres
// @reason    Locks the PRD §17 "server mati tidak dijual" filter and the trial
// cleanup contract against the real schema (health_status default 'unknown').
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-12
package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

// seedServer inserts one active+open panel row for the given country. GORM
// omits zero-value fields tagged with a default (default:true), so is_active
// = false is forced via SetActive — the real mutation path (admin toggles).
func seedServer(t *testing.T, r *postgres.Repository, name, country string, active bool) int64 {
	t.Helper()
	ctx := context.Background()
	s := postgres.VPNServer{
		Name: name, Host: name + ".example.com", Port: 443, Username: "admin",
		PasswordEnc: "enc", APIPath: "/panel", UseSSL: true,
		CountryCode: country, FlagEmoji: "🇮🇩", Protocols: `["vless"]`,
		IsActive: true, IsOpen: true, UpdatedAt: time.Now(),
	}
	if err := r.Servers().UpsertSeed(ctx, s); err != nil {
		t.Fatalf("UpsertSeed: %v", err)
	}
	// UpsertSeed takes VPNServer by value, so the caller's s.ID is never set —
	// look the row back up by its unique host+port+username (same as the
	// admin add-server dedup check).
	row, err := r.Servers().FindByHostPort(ctx, name+".example.com", 443, "admin")
	if err != nil || row == nil {
		t.Fatalf("FindByHostPort: row=%v err=%v", row, err)
	}
	if !active {
		if err := r.Servers().SetActive(ctx, row.ID, false); err != nil {
			t.Fatalf("SetActive: %v", err)
		}
	}
	return row.ID
}

func TestHealthRepo_GivenServers_ThenTargetsOnlyActiveAndStatusPersists(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	ctx := context.Background()

	idActive := seedServer(t, r, "ID-01", "ID", true)
	seedServer(t, r, "ID-02", "ID", false) // inactive — never a health target

	targets, err := r.Servers().ListHealthTargets(ctx)
	if err != nil {
		t.Fatalf("ListHealthTargets: %v", err)
	}
	if len(targets) != 1 || targets[0].ID != idActive {
		t.Fatalf("targets = %+v, want only the active server", targets)
	}

	checkedAt := time.Now()
	if err := r.Servers().UpdateHealth(ctx, idActive, "down", checkedAt); err != nil {
		t.Fatalf("UpdateHealth: %v", err)
	}
	row, err := r.Servers().GetByID(ctx, idActive)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if row.HealthStatus != "down" {
		t.Errorf("health_status = %q, want down", row.HealthStatus)
	}
	if row.LastHealthCheck == nil || row.LastHealthCheck.Before(checkedAt.Add(-time.Minute)) {
		t.Errorf("last_health_check = %v, want ~now", row.LastHealthCheck)
	}
}

func TestListBuyable_GivenDownServer_ThenExcludedFromBuyMenu(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	ctx := context.Background()

	// Fresh row: health_status defaults to 'unknown' → sellable.
	idUnknown := seedServer(t, r, "ID-01", "ID", true)
	// Healthy row → sellable.
	idOK := seedServer(t, r, "SG-01", "SG", true)
	// Dead row → NOT sellable (PRD §17).
	idDown := seedServer(t, r, "JP-01", "JP", true)
	if err := r.Servers().UpdateHealth(ctx, idDown, "down", time.Now()); err != nil {
		t.Fatalf("UpdateHealth: %v", err)
	}
	// Inactive row → not sellable regardless.
	seedServer(t, r, "ID-02", "ID", false)

	buyable, err := r.Servers().ListBuyable(ctx)
	if err != nil {
		t.Fatalf("ListBuyable: %v", err)
	}
	got := map[int64]bool{}
	for _, b := range buyable {
		got[b.ID] = true
	}
	if !got[idUnknown] || !got[idOK] {
		t.Errorf("buyable missing healthy/unknown server: %v", got)
	}
	if got[idDown] {
		t.Error("buyable includes the 'down' server — server mati masih dijual!")
	}
}

func TestTrialCleanupRepo_GivenExpiredTrial_ThenCandidateAndMark(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	ctx := context.Background()

	u, err := r.User().FindOrCreate(ctx, 778001, "trialuser", "Trial User")
	if err != nil {
		t.Fatalf("FindOrCreate: %v", err)
	}
	serverID := seedServer(t, r, "ID-01", "ID", true)

	expired := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)
	clients := []postgres.VPNClient{
		{UserID: u.ID, ServerID: serverID, InboundID: 1, Email: "trial-expired@vpn.kt", UUID: "u1", Protocol: "vless", IsTrial: true, ExpiresAt: &expired, IsActive: true},
		{UserID: u.ID, ServerID: serverID, InboundID: 1, Email: "trial-future@vpn.kt", UUID: "u2", Protocol: "vless", IsTrial: true, ExpiresAt: &future, IsActive: true},
		{UserID: u.ID, ServerID: serverID, InboundID: 1, Email: "paid-expired@vpn.kt", UUID: "u3", Protocol: "vless", IsTrial: false, ExpiresAt: &expired, IsActive: true},
	}
	for _, c := range clients {
		if err := r.Clients().Create(ctx, &c); err != nil {
			t.Fatalf("Client Create: %v", err)
		}
	}

	cands, err := r.Clients().ListExpiredTrialCandidates(ctx, 10)
	if err != nil {
		t.Fatalf("ListExpiredTrialCandidates: %v", err)
	}
	if len(cands) != 1 || cands[0].Email != "trial-expired@vpn.kt" {
		t.Fatalf("candidates = %+v, want only the expired trial (not future, not paid)", cands)
	}

	if err := r.Clients().MarkTrialExpired(ctx, cands[0].ClientID); err != nil {
		t.Fatalf("MarkTrialExpired: %v", err)
	}
	row, err := r.Clients().GetOwned(ctx, cands[0].ClientID, u.ID)
	if err != nil {
		t.Fatalf("GetOwned: %v", err)
	}
	if row.IsActive || !row.IsExpired {
		t.Errorf("after mark = active:%v expired:%v, want active:false expired:true", row.IsActive, row.IsExpired)
	}
}
