// Package adminsvc test also hosts the FR-11 v1.40 seam fakes.
//
// @file      internal/service/admin/admin_fakes_ops_test.go
// @for       In-memory ServerOps/StatsStore/AuditStore fakes (FR-11 v1.40).
// @uses      context, time, internal/repository/postgres, internal/service/server
// @reason    Split from admin_fakes_test.go for the §1.1 line limit; the v1.40
// seams (server ops, stats, audit) are only needed by server_stats_test.go.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-12
package adminsvc

import (
	"context"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
	serversvc "github.com/kentangtech/bot-order/internal/service/server"
)

type fakeServerOps struct {
	all      []postgres.ServerAdminView
	err      error
	added    *serverInput
	addedID  int64
	openID   int64
	activeID int64
}

type serverInput struct {
	name     string
	host     string
	port     int
	username string
	password string
}

func (f *fakeServerOps) ListAll(context.Context) ([]postgres.ServerAdminView, error) {
	return f.all, f.err
}
func (f *fakeServerOps) SetOpen(_ context.Context, id int64, _ bool) error {
	f.openID = id
	return f.err
}
func (f *fakeServerOps) SetActive(_ context.Context, id int64, _ bool) error {
	f.activeID = id
	return f.err
}
func (f *fakeServerOps) AddServer(_ context.Context, in serversvc.NewServerInput) (int64, error) {
	f.added = &serverInput{name: in.Name, host: in.Host, port: in.Port, username: in.Username, password: in.Password}
	return f.addedID, f.err
}

type fakeStats struct {
	stats  postgres.OrderStats
	recent []postgres.Order
	err    error
}

func (f *fakeStats) Stats(context.Context, *time.Location) (postgres.OrderStats, error) {
	return f.stats, f.err
}
func (f *fakeStats) RecentOrders(context.Context, int) ([]postgres.Order, error) {
	return f.recent, f.err
}

type fakeAudit struct {
	rows []postgres.AdminAuditLog
	err  error
}

func (f *fakeAudit) Record(_ context.Context, adminID int64, action, target, detail string) error {
	if f.err != nil {
		return f.err
	}
	f.rows = append(f.rows, postgres.AdminAuditLog{AdminID: adminID, Action: action, Target: target, Detail: detail})
	return nil
}
func (f *fakeAudit) Recent(context.Context, int) ([]postgres.AdminAuditLog, error) {
	return f.rows, f.err
}
