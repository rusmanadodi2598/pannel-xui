// Package adminsvc test covers the FR-11 server management, stats & audit ops (v1.40).
//
// @file      internal/service/admin/server_stats_test.go
// @for       Given seams, then ListServers/Toggle/AddServer/Stats/AuditLog delegate + record.
// @uses      testing, context, time, internal/repository/postgres, internal/service/server
// @reason    Server mgmt & statistik are FR-11 gap-fills (v1.40); every mutation
// must produce an audit row while read-only views never write.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-12
package adminsvc

import (
	"context"
	"testing"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
	serversvc "github.com/kentangtech/bot-order/internal/service/server"
)

func TestListServers_GivenSeam_ThenDelegates(t *testing.T) {
	srv := &fakeServerOps{all: []postgres.ServerAdminView{{ID: 1, Name: "ID-01"}}}
	s := New(&fakePlans{}, &fakeUsers{}, &fakeBanner{}, &fakeSender{}, &fakeLocker{}, srv, &fakeStats{}, &fakeAudit{}, testLogger())

	rows, err := s.ListServers(context.Background())
	if err != nil || len(rows) != 1 {
		t.Fatalf("ListServers = %+v, err %v", rows, err)
	}
}

func TestToggleServerOpen_GivenID_ThenFlipsAndAudits(t *testing.T) {
	srv := &fakeServerOps{}
	audit := &fakeAudit{}
	s := New(&fakePlans{}, &fakeUsers{}, &fakeBanner{}, &fakeSender{}, &fakeLocker{}, srv, &fakeStats{}, audit, testLogger())

	if err := s.ToggleServerOpen(context.Background(), 7, 3, false); err != nil {
		t.Fatalf("ToggleServerOpen: %v", err)
	}
	if srv.openID != 3 {
		t.Errorf("open id = %d, want 3", srv.openID)
	}
	if len(audit.rows) != 1 || audit.rows[0].Action != AuditServerOpen || audit.rows[0].AdminID != 7 {
		t.Errorf("audit = %+v, want server:open by admin 7", audit.rows)
	}
}

func TestToggleServerActive_GivenID_ThenFlipsAndAudits(t *testing.T) {
	srv := &fakeServerOps{}
	audit := &fakeAudit{}
	s := New(&fakePlans{}, &fakeUsers{}, &fakeBanner{}, &fakeSender{}, &fakeLocker{}, srv, &fakeStats{}, audit, testLogger())

	if err := s.ToggleServerActive(context.Background(), 7, 3, false); err != nil {
		t.Fatalf("ToggleServerActive: %v", err)
	}
	if srv.activeID != 3 {
		t.Errorf("active id = %d, want 3", srv.activeID)
	}
	if len(audit.rows) != 1 || audit.rows[0].Action != AuditServerActive {
		t.Errorf("audit = %+v, want server:active row", audit.rows)
	}
}

func TestAddServer_GivenInput_ThenDelegatesAndAudits(t *testing.T) {
	srv := &fakeServerOps{addedID: 55}
	audit := &fakeAudit{}
	s := New(&fakePlans{}, &fakeUsers{}, &fakeBanner{}, &fakeSender{}, &fakeLocker{}, srv, &fakeStats{}, audit, testLogger())

	id, err := s.AddServer(context.Background(), 7, serversvc.NewServerInput{
		Name: "ID-09", Host: "id9.example.com", Port: 2083, Username: "admin", Password: "p", CountryCode: "ID",
	})
	if err != nil || id != 55 {
		t.Fatalf("AddServer = %d, err %v", id, err)
	}
	if srv.added == nil || srv.added.name != "ID-09" {
		t.Errorf("added = %+v", srv.added)
	}
	if len(audit.rows) != 1 || audit.rows[0].Action != AuditServerAdd || audit.rows[0].Target != "55" {
		t.Errorf("audit = %+v, want server:add target 55", audit.rows)
	}
}

func TestStats_GivenSeam_ThenDelegates(t *testing.T) {
	want := postgres.OrderStats{TotalOrders: 12, TotalRevenue: 90000}
	st := &fakeStats{stats: want}
	s := New(&fakePlans{}, &fakeUsers{}, &fakeBanner{}, &fakeSender{}, &fakeLocker{}, &fakeServerOps{}, st, &fakeAudit{}, testLogger())

	got, err := s.Stats(context.Background(), time.Local)
	if err != nil || got.TotalOrders != 12 {
		t.Fatalf("Stats = %+v, err %v", got, err)
	}
}

func TestAuditLog_GivenRows_ThenDelegates(t *testing.T) {
	audit := &fakeAudit{rows: []postgres.AdminAuditLog{{Action: AuditPriceSet}}}
	s := New(&fakePlans{}, &fakeUsers{}, &fakeBanner{}, &fakeSender{}, &fakeLocker{}, &fakeServerOps{}, &fakeStats{}, audit, testLogger())

	rows, err := s.AuditLog(context.Background(), 10)
	if err != nil || len(rows) != 1 || rows[0].Action != AuditPriceSet {
		t.Fatalf("AuditLog = %+v, err %v", rows, err)
	}
}

func TestAuditFailure_GivenRecordError_ThenActionStillSucceeds(t *testing.T) {
	// A failing audit store must never fail the admin action (best-effort trail).
	audit := &fakeAudit{err: errAuditDown}
	s := New(&fakePlans{}, &fakeUsers{}, &fakeBanner{}, &fakeSender{}, &fakeLocker{}, &fakeServerOps{}, &fakeStats{}, audit, testLogger())
	if err := s.ToggleServerOpen(context.Background(), 7, 1, true); err != nil {
		t.Fatalf("ToggleServerOpen with audit down = %v, want success", err)
	}
}

var errAuditDown = &auditDownError{}

type auditDownError struct{}

func (e *auditDownError) Error() string { return "audit store down" }
