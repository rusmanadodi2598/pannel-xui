// Package postgres_test also covers the FR-09 expiry-reminder repository.
//
// @file      internal/repository/postgres/repo_expiry_test.go
// @for       Integration tests: candidate windows, notified guard, renewal reset, trial excluded.
// @uses      testing, context, time, github.com/kentangtech/bot-order/internal/repository/postgres
// @reason    Verifies the FR-09 persistence contract on the staging DB.
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

// seedExpiryServer creates one open server and returns its id.
func seedExpiryServer(t *testing.T, r *postgres.Repository, name string) int64 {
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

// addExpiryClient inserts a client expiring in `hours` (trial flag configurable).
func addExpiryClient(t *testing.T, r *postgres.Repository, userID, serverID int64, email string, hours float64, trial bool) postgres.VPNClient {
	t.Helper()
	exp := time.Now().Add(time.Duration(hours * float64(time.Hour)))
	c := postgres.VPNClient{
		UserID: userID, ServerID: serverID, InboundID: 1, Email: email,
		Protocol: "vless", ExpiresAt: &exp, IsTrial: trial,
	}
	if err := r.Clients().Create(context.Background(), &c); err != nil {
		t.Fatalf("Client Create %s: %v", email, err)
	}
	return c
}

func TestClientRepo_ExpiryCandidates_GivenWindows_ThenOnlyMatchingRows(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	ctx := context.Background()

	u, err := r.User().FindOrCreate(ctx, 889001, "gina", "Gina")
	if err != nil {
		t.Fatalf("FindOrCreate: %v", err)
	}
	serverID := seedExpiryServer(t, r, "ID-01")

	// 8 hari → di luar jendela H-7; 5 hari → jendela (3,7]; 2 hari → (1,3];
	// 0.5 hari → (0,1]; trial 2 hari → wajib dikecualikan.
	far := addExpiryClient(t, r, u.ID, serverID, "far@vpn.kt", 8*24, false)
	w7 := addExpiryClient(t, r, u.ID, serverID, "w7@vpn.kt", 5*24, false)
	w3 := addExpiryClient(t, r, u.ID, serverID, "w3@vpn.kt", 2*24, false)
	w1 := addExpiryClient(t, r, u.ID, serverID, "w1@vpn.kt", 12, false)
	addExpiryClient(t, r, u.ID, serverID, "trial@vpn.kt", 2*24, true)
	notified := addExpiryClient(t, r, u.ID, serverID, "notified@vpn.kt", 5*24, false)
	if err := r.Clients().MarkNotified(ctx, notified.ID, 7); err != nil {
		t.Fatalf("MarkNotified: %v", err)
	}

	if got := ids2(mustCandidates(t, r, 7, 3)); !assertOnly(got, w7.ID) {
		t.Errorf("window (3,7] = %v, want only client %d", got, w7.ID)
	}
	if got := ids2(mustCandidates(t, r, 3, 1)); !assertOnly(got, w3.ID) {
		t.Errorf("window (1,3] = %v, want only client %d", got, w3.ID)
	}
	if got := ids2(mustCandidates(t, r, 1, 0)); !assertOnly(got, w1.ID) {
		t.Errorf("window (0,1] = %v, want only client %d", got, w1.ID)
	}
	_ = far // 8-day client asserted absent by the assertOnly checks above
}

func TestClientRepo_ExpiryCandidates_GivenMarked_ThenExcludedUntilRenewalResets(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	ctx := context.Background()

	u, _ := r.User().FindOrCreate(ctx, 889002, "hendra", "Hendra")
	serverID := seedExpiryServer(t, r, "SG-01")
	c := addExpiryClient(t, r, u.ID, serverID, "ren@vpn.kt", 5*24, false)

	// Belum dinotifikasi → masuk jendela H-7.
	if got := ids2(mustCandidates(t, r, 7, 3)); !assertOnly(got, c.ID) {
		t.Fatalf("pre-mark window = %v, want client %d", got, c.ID)
	}

	// Setelah ditandai H-7 → tidak muncul lagi (FR-09 AC: sekali per ambang).
	if err := r.Clients().MarkNotified(ctx, c.ID, 7); err != nil {
		t.Fatalf("MarkNotified: %v", err)
	}
	if got := mustCandidates(t, r, 7, 3); len(got) != 0 {
		t.Fatalf("post-mark window = %v, want empty", got)
	}

	// Renewal (UpdateExpiry) reset notified_expiry → muncul lagi di jendela H-7.
	newExp := time.Now().Add(5 * 24 * time.Hour)
	if err := r.Clients().UpdateExpiry(ctx, c.ID, newExp, nil); err != nil {
		t.Fatalf("UpdateExpiry: %v", err)
	}
	if got := ids2(mustCandidates(t, r, 7, 3)); !assertOnly(got, c.ID) {
		t.Fatalf("post-renewal window = %v, want client %d again", got, c.ID)
	}
}

func TestClientRepo_ExpiryCandidates_GivenLimit_ThenBounded(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	ctx := context.Background()

	u, _ := r.User().FindOrCreate(ctx, 889003, "ira", "Ira")
	serverID := seedExpiryServer(t, r, "JP-01")
	for i := 0; i < 5; i++ {
		addExpiryClient(t, r, u.ID, serverID, "bulk"+string(rune('a'+i))+"@vpn.kt", 5*24, false)
	}
	rows, err := r.Clients().ListExpiryCandidates(ctx, 7, 3, 2)
	if err != nil {
		t.Fatalf("ListExpiryCandidates: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("bounded rows = %d, want 2", len(rows))
	}
}

func mustCandidates(t *testing.T, r *postgres.Repository, upper, lower int) []postgres.ExpiryCandidate {
	t.Helper()
	rows, err := r.Clients().ListExpiryCandidates(context.Background(), upper, lower, 50)
	if err != nil {
		t.Fatalf("ListExpiryCandidates(%d,%d): %v", upper, lower, err)
	}
	return rows
}

func ids2(rows []postgres.ExpiryCandidate) map[int64]bool {
	m := map[int64]bool{}
	for _, c := range rows {
		m[c.ClientID] = true
	}
	return m
}

func assertOnly(got map[int64]bool, want int64) bool {
	return len(got) == 1 && got[want]
}
