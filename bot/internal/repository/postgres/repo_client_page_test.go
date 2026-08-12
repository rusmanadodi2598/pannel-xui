// Package postgres_test also covers the FR-08 AC-1 client pagination.
//
// @file      internal/repository/postgres/repo_client_page_test.go
// @for       CountByUser + ListByUserPage: paged, bounded, ownership-scoped.
// @uses      testing, context, time, github.com/kentangtech/bot-order/internal/repository/postgres
// @reason    Verifies the 5/page account-list persistence contract on the
// staging DB (FR-08 AC-1; split file respects the 250-line limit §1.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-11
package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

func TestClientPage_GivenSixClients_ThenCountAndPages(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	ctx := context.Background()

	u, err := r.User().FindOrCreate(ctx, 778001, "pagi", "Pagi")
	if err != nil {
		t.Fatalf("FindOrCreate: %v", err)
	}
	serverID := seedTestServer(t, r, ctx, "ID-01")
	expiry := time.Now().AddDate(0, 0, 30)
	for i := 1; i <= 6; i++ {
		c := postgres.VPNClient{
			UserID: u.ID, ServerID: serverID, InboundID: 5,
			Email: "page" + string(rune('0'+i)) + "@vpn.kt", Protocol: "vless",
			ExpiresAt: &expiry,
		}
		if err := r.Clients().Create(ctx, &c); err != nil {
			t.Fatalf("Create client %d: %v", i, err)
		}
	}

	count, err := r.Clients().CountByUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("CountByUser: %v", err)
	}
	if count != 6 {
		t.Errorf("count = %d, want 6", count)
	}

	page1, err := r.Clients().ListByUserPage(ctx, u.ID, 5, 0)
	if err != nil {
		t.Fatalf("ListByUserPage(0): %v", err)
	}
	if len(page1) != 5 {
		t.Errorf("page1 len = %d, want 5", len(page1))
	}
	page2, err := r.Clients().ListByUserPage(ctx, u.ID, 5, 5)
	if err != nil {
		t.Fatalf("ListByUserPage(5): %v", err)
	}
	if len(page2) != 1 {
		t.Errorf("page2 len = %d, want 1", len(page2))
	}

	// Newest first: client 6 is on page 1.
	if page1[0].Email != "page6@vpn.kt" {
		t.Errorf("page1[0] = %s, want page6@vpn.kt (newest first)", page1[0].Email)
	}
	// Offsets beyond the data return empty — no error.
	beyond, err := r.Clients().ListByUserPage(ctx, u.ID, 5, 10)
	if err != nil || len(beyond) != 0 {
		t.Errorf("beyond = %v (len %d), want empty", err, len(beyond))
	}
}

func TestClientPage_GivenLimitOutOfRange_ThenDefaultFive(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	ctx := context.Background()

	u, err := r.User().FindOrCreate(ctx, 778002, "pagi2", "Pagi2")
	if err != nil {
		t.Fatalf("FindOrCreate: %v", err)
	}
	serverID := seedTestServer(t, r, ctx, "ID-02")
	expiry := time.Now().AddDate(0, 0, 30)
	for i := 1; i <= 3; i++ {
		c := postgres.VPNClient{
			UserID: u.ID, ServerID: serverID, InboundID: 5,
			Email: "lim" + string(rune('0'+i)) + "@vpn.kt", Protocol: "vmess",
			ExpiresAt: &expiry,
		}
		if err := r.Clients().Create(ctx, &c); err != nil {
			t.Fatalf("Create client %d: %v", i, err)
		}
	}
	// limit 0 clamps to 5; 3 rows exist → all 3 returned (bounded, not padded).
	rows, err := r.Clients().ListByUserPage(ctx, u.ID, 0, 0)
	if err != nil {
		t.Fatalf("ListByUserPage(0,0): %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("rows = %d, want 3 (clamped limit 5, only 3 exist)", len(rows))
	}
	// Negative offset clamps to 0.
	rows, err = r.Clients().ListByUserPage(ctx, u.ID, 5, -3)
	if err != nil {
		t.Fatalf("ListByUserPage(5,-3): %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("rows(neg offset) = %d, want 3", len(rows))
	}
}

func TestClientDelete_GivenOwnedClient_ThenRowRemoved(t *testing.T) {
	r := openRepo(t)
	cleanTables(t)
	ctx := context.Background()

	u, err := r.User().FindOrCreate(ctx, 778003, "del", "Del")
	if err != nil {
		t.Fatalf("FindOrCreate: %v", err)
	}
	serverID := seedTestServer(t, r, ctx, "ID-03")
	expiry := time.Now().AddDate(0, 0, 30)
	client := postgres.VPNClient{
		UserID: u.ID, ServerID: serverID, InboundID: 5,
		Email: "del@vpn.kt", UUID: "uuid-del", Protocol: "vless", ExpiresAt: &expiry,
	}
	if err := r.Clients().Create(ctx, &client); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Owner can delete; row disappears and a re-delete reports not-found.
	if err := r.Clients().DeleteOwned(ctx, client.ID, u.ID); err != nil {
		t.Fatalf("DeleteOwned: %v", err)
	}
	if _, err := r.Clients().GetOwned(ctx, client.ID, u.ID); !errors.Is(err, postgres.ErrClientNotFound) {
		t.Errorf("GetOwned after delete = %v, want ErrClientNotFound", err)
	}

	// Foreign user cannot delete (and the row stays for the owner).
	other, err := r.User().FindOrCreate(ctx, 778004, "del2", "Del2")
	if err != nil {
		t.Fatalf("FindOrCreate other: %v", err)
	}
	if err := r.Clients().DeleteOwned(ctx, client.ID, other.ID); !errors.Is(err, postgres.ErrClientNotFound) {
		t.Errorf("foreign DeleteOwned = %v, want ErrClientNotFound (row must stay)", err)
	}
}

// seedTestServer upserts one active buyable panel and returns its id.
func seedTestServer(t *testing.T, r *postgres.Repository, ctx context.Context, name string) int64 {
	t.Helper()
	srv := postgres.VPNServer{
		Name: name, Host: name + ".example.com", Port: 443, Username: "admin",
		PasswordEnc: "enc", APIPath: "/panel", UseSSL: true,
		CountryCode: "ID", FlagEmoji: "🇮🇩", Protocols: `["vless"]`,
		IsActive: true, IsOpen: true, UpdatedAt: time.Now(),
	}
	if err := r.Servers().UpsertSeed(ctx, srv); err != nil {
		t.Fatalf("UpsertSeed: %v", err)
	}
	buyable, err := r.Servers().ListBuyable(ctx)
	if err != nil || len(buyable) != 1 {
		t.Fatalf("ListBuyable: %v (len %d)", err, len(buyable))
	}
	return buyable[0].ID
}
