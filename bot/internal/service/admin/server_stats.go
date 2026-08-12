// Package adminsvc also hosts the server-management, stats & audit ops (FR-11, v1.40).
//
// @file      internal/service/admin/server_stats.go
// @for       Admin: server list/toggle/add (via serversvc), dashboard stats, audit trail.
// @uses      context, fmt, time, internal/repository/postgres, internal/service/server
// @reason    FR-11 server management & statistik: adminsvc delegates panel ops
// to serversvc (owns encryption) and reads dashboard aggregates from the repos;
// every mutation records an immutable audit row. Split for §1.1.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability experimental
// @since     2026-08-12
package adminsvc

import (
	"context"
	"fmt"
	"time"

	"github.com/kentangtech/bot-order/internal/repository/postgres"
	serversvc "github.com/kentangtech/bot-order/internal/service/server"
)

// Audit action vocabulary (FR-11 AC) — stored verbatim in admin_audit_log.
const (
	AuditPriceSet       = "price:set"
	AuditPlanToggle     = "plan:toggle"
	AuditPricingReload  = "pricing:reload"
	AuditUserBan        = "user:ban"
	AuditUserUnban      = "user:unban"
	AuditBalanceAdjust  = "balance:adjust"
	AuditBroadcastStart = "broadcast:start"
	AuditServerAdd      = "server:add"
	AuditServerOpen     = "server:open"
	AuditServerActive   = "server:active"
)

// --- server management (FR-11, v1.40) ---

// ListServers returns every panel (active + inactive) for the admin view.
func (s *Service) ListServers(ctx context.Context) ([]postgres.ServerAdminView, error) {
	if s.servers == nil {
		return nil, fmt.Errorf("server management not wired")
	}
	return s.servers.ListAll(ctx)
}

// ToggleServerOpen flips the sellable flag and records the audit row.
func (s *Service) ToggleServerOpen(ctx context.Context, adminID, serverID int64, open bool) error {
	if s.servers == nil {
		return fmt.Errorf("server management not wired")
	}
	if err := s.servers.SetOpen(ctx, serverID, open); err != nil {
		return err
	}
	s.auditRecord(ctx, adminID, AuditServerOpen, fmt.Sprintf("%d", serverID), fmt.Sprintf("open=%v", open))
	return nil
}

// ToggleServerActive flips the server's active flag and records the audit row.
func (s *Service) ToggleServerActive(ctx context.Context, adminID, serverID int64, active bool) error {
	if s.servers == nil {
		return fmt.Errorf("server management not wired")
	}
	if err := s.servers.SetActive(ctx, serverID, active); err != nil {
		return err
	}
	s.auditRecord(ctx, adminID, AuditServerActive, fmt.Sprintf("%d", serverID), fmt.Sprintf("active=%v", active))
	return nil
}

// AddServer delegates the panel creation (password sealed by serversvc) and
// records the audit row with the new panel id.
func (s *Service) AddServer(ctx context.Context, adminID int64, in serversvc.NewServerInput) (int64, error) {
	if s.servers == nil {
		return 0, fmt.Errorf("server management not wired")
	}
	id, err := s.servers.AddServer(ctx, in)
	if err != nil {
		return 0, err
	}
	s.auditRecord(ctx, adminID, AuditServerAdd, fmt.Sprintf("%d", id), fmt.Sprintf("%s (%s)", in.Name, in.Host))
	return id, nil
}

// --- statistik (FR-11, v1.40) ---

// Stats returns the dashboard aggregates (orders/revenue/users/clients).
func (s *Service) Stats(ctx context.Context, loc *time.Location) (postgres.OrderStats, error) {
	if s.stats == nil {
		return postgres.OrderStats{}, fmt.Errorf("stats not wired")
	}
	return s.stats.Stats(ctx, loc)
}

// RecentOrders returns the newest orders for the admin dashboard.
func (s *Service) RecentOrders(ctx context.Context, limit int) ([]postgres.Order, error) {
	if s.stats == nil {
		return nil, fmt.Errorf("stats not wired")
	}
	return s.stats.RecentOrders(ctx, limit)
}

// --- audit trail (FR-11 AC) ---

// auditRecord appends one immutable audit row, best-effort: a failed write is
// logged and never fails the admin action itself (the action already happened).
func (s *Service) auditRecord(ctx context.Context, adminID int64, action, target, detail string) {
	if s.audit == nil {
		return
	}
	if err := s.audit.Record(ctx, adminID, action, target, detail); err != nil {
		s.logger.Error("audit record failed", "admin_id", adminID, "action", action, "error", err)
	}
}

// AuditLog returns the newest audit rows for the admin view (FR-11).
func (s *Service) AuditLog(ctx context.Context, limit int) ([]postgres.AdminAuditLog, error) {
	if s.audit == nil {
		return nil, fmt.Errorf("audit not wired")
	}
	return s.audit.Recent(ctx, limit)
}
