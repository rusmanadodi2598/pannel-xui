// Package postgres also hosts the traffic-sync queries (PRD §16.2).
//
// @file      internal/repository/postgres/client_repo_traffic.go
// @for       Traffic sync: list live candidates + bulk write one UPDATE ... FROM (VALUES ...).
// @uses      context, fmt, strings, time
// @reason    The traffic sweep reads candidate clients and writes results in a
// single statement (anti N+1 §1.7). Split from client_repo.go for the §1.1 line limit.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     repository
// @stability stable
// @since     2026-08-17
package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// TrafficCandidate is a live client eligible for the traffic-sync sweep
// (PRD §16.2): active, not expired, on an active panel.
type TrafficCandidate struct {
	ClientID int64
	ServerID int64
	Email    string
}

// ListTrafficCandidates returns active clients of active panels for the
// traffic-sync sweep, oldest-synced first (round-robin fairness across sweeps
// when the batch is smaller than the fleet), bounded to limit (AGENTS.md §1.7).
func (r *ClientRepo) ListTrafficCandidates(ctx context.Context, limit int) ([]TrafficCandidate, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	var rows []TrafficCandidate
	err := r.db.WithContext(ctx).
		Table("vpn_clients AS c").
		Select("c.id AS client_id, c.server_id, c.email").
		Joins("JOIN vpn_servers AS s ON s.id = c.server_id").
		Where("c.is_active = true AND c.is_expired = false AND s.is_active = true").
		Order("c.last_sync ASC NULLS FIRST, c.id").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("listing traffic candidates: %w", err)
	}
	return rows, nil
}

// TrafficUpdate carries one client's panel traffic for the batch write.
// LastOnline is set only when the client is online right now; nil keeps the
// previous value (COALESCE in the single bulk UPDATE).
type TrafficUpdate struct {
	ClientID   int64
	Up         int64
	Down       int64
	LastOnline *time.Time
}

// SyncTrafficBatch writes the sweep result in ONE statement (anti N+1 §1.7):
// UPDATE ... FROM (VALUES ...) matching each row by id. last_online uses
// COALESCE so offline clients keep their previous online timestamp.
func (r *ClientRepo) SyncTrafficBatch(ctx context.Context, syncedAt time.Time, updates []TrafficUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	var sb strings.Builder
	sb.WriteString(`UPDATE vpn_clients AS c SET
		traffic_up = v.up, traffic_down = v.down, traffic_used = v.up + v.down,
		last_online = COALESCE(v.online, c.last_online), last_sync = ?, updated_at = ?
	FROM (VALUES `)
	args := []any{syncedAt, syncedAt}
	for i, u := range updates {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("(?::bigint, ?::bigint, ?::bigint, ?::timestamptz)")
		args = append(args, u.ClientID, u.Up, u.Down, u.LastOnline)
	}
	sb.WriteString(`) AS v(id, up, down, online)
	WHERE c.id = v.id`)
	if err := r.db.WithContext(ctx).Exec(sb.String(), args...).Error; err != nil {
		return fmt.Errorf("syncing traffic batch: %w", err)
	}
	return nil
}
