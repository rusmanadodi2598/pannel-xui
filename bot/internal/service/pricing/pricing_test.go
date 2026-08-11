// Package pricingsvc_test covers seeding and live plan reads (AGENTS.md §2.1).
//
// @file      internal/service/pricing/pricing_test.go
// @for       Unit tests: FileSeeder parsing, EnsureSeeded, ListEnabled/GetPlan.
// @uses      testing, context, os, strings, internal/domain, internal/repository/postgres
// @reason    Guards the pricing contract (seed shape → DB rows) every order depends on.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-11
package pricingsvc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
)

const seedJSON = `{
  "version": "1.0.0",
  "currency": "IDR",
  "categories": {
    "ID": [
      {"id": 1, "code": "ID15", "price": 4000, "validity": "15 Hari", "is_active": true},
      {"id": 2, "code": "ID30", "price": 7000, "validity": "30 Hari", "is_active": true},
      {"id": 3, "code": "ID90", "price": 20000, "validity": "90 Hari", "is_active": false}
    ],
    "SG": [
      {"id": 4, "code": "SG15", "price": 5000, "validity": "15 Hari", "is_active": true}
    ]
  }
}`

func TestFileSeeder_GivenValidJSON_ThenRowsParsed(t *testing.T) {
	path := writeSeed(t, seedJSON)
	seeder := FileSeeder{Path: path}

	rows, err := seeder.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(rows) != 3 { // ID15, ID30 (ID90 disabled), SG15
		t.Fatalf("rows = %d, want 3 (inactive skipped)", len(rows))
	}

	got := map[string]postgres.Pricing{}
	for _, r := range rows {
		got[r.CountryCode+strconv.Itoa(r.PlanDays)] = r
	}
	if p := got["ID15"]; p.Price != 4000 || !p.Enabled {
		t.Errorf("ID15 = %+v", p)
	}
	if p := got["ID30"]; p.Price != 7000 {
		t.Errorf("ID30 = %+v", p)
	}
	if p := got["SG15"]; p.Price != 5000 {
		t.Errorf("SG15 = %+v", p)
	}
	if _, ok := got["ID90"]; ok {
		t.Error("inactive plan must be skipped")
	}
}

func TestFileSeeder_GivenEmptyFile_ThenError(t *testing.T) {
	path := writeSeed(t, `{"categories": {}}`)
	if _, err := (FileSeeder{Path: path}).Load(); err == nil {
		t.Fatal("expected error for seed with no active plans")
	}
}

func TestService_EnsureSeeded_ThenUpsertsAndLists(t *testing.T) {
	store := &fakePricingStore{}
	svc := New(store, FileSeeder{Path: writeSeed(t, seedJSON)})

	if err := svc.EnsureSeeded(context.Background()); err != nil {
		t.Fatalf("EnsureSeeded: %v", err)
	}
	if len(store.upserted) != 3 {
		t.Errorf("upserted = %d, want 3", len(store.upserted))
	}

	store.enabled = []postgres.Pricing{
		{CountryCode: "ID", PlanDays: 15, Price: 4000, Enabled: true},
		{CountryCode: "ID", PlanDays: 30, Price: 7000, Enabled: true},
		{CountryCode: "SG", PlanDays: 15, Price: 5000, Enabled: true},
	}
	plans, err := svc.ListEnabled(context.Background())
	if err != nil {
		t.Fatalf("ListEnabled: %v", err)
	}
	if len(plans) != 3 {
		t.Fatalf("plans = %d", len(plans))
	}
	if plans[0].CountryName != "Indonesia" || plans[0].Price != 4000 {
		t.Errorf("plan[0] = %+v", plans[0])
	}

	store.plan = &postgres.Pricing{CountryCode: "ID", PlanDays: 30, Price: 7000, Enabled: true}
	plan, err := svc.GetPlan(context.Background(), "ID", 30)
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if plan.Code() != "ID30" || plan.Price != 7000 {
		t.Errorf("plan = %+v", plan)
	}
}

func TestService_EnsureSeeded_GivenNilSeeder_ThenNoop(t *testing.T) {
	svc := New(&fakePricingStore{}, nil)
	if err := svc.EnsureSeeded(context.Background()); err != nil {
		t.Fatalf("EnsureSeeded with nil seeder: %v", err)
	}
}

func writeSeed(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "pricing.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing seed: %v", err)
	}
	return path
}

// fakePricingStore implements Store.
type fakePricingStore struct {
	upserted []postgres.Pricing
	enabled  []postgres.Pricing
	plan     *postgres.Pricing
	err      error
}

func (f *fakePricingStore) UpsertMany(_ context.Context, rows []postgres.Pricing) error {
	f.upserted = append(f.upserted, rows...)
	return f.err
}
func (f *fakePricingStore) ListEnabled(context.Context) ([]postgres.Pricing, error) {
	return f.enabled, f.err
}
func (f *fakePricingStore) GetPlan(_ context.Context, _ string, _ int) (*postgres.Pricing, error) {
	if f.plan == nil {
		return nil, errNoPlan
	}
	return f.plan, nil
}

var errNoPlan = errors.New("plan not found")
