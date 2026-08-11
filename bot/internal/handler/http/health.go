// Package httphandler also hosts the health endpoint.
//
// @file      internal/handler/http/health.go
// @for       GET /api/v1/health and infra alias GET /health with DB/Redis checks.
// @uses      net/http, context, time
// @reason    Split from handler.go for the 250-line limit (AGENTS.md §1.1).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     handler
// @stability stable
// @since     2026-08-11
package httphandler

import (
	"context"
	"net/http"
	"time"
)

// health answers with dependency status: overall ok/degraded and per-store state.
// webhook is "registered" because main.go refuses to boot when setWebhook fails.
func (o Options) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	dbState := "ok"
	if o.DB != nil {
		if err := o.DB.Ping(ctx); err != nil {
			dbState = "error"
		}
	} else {
		dbState = "unconfigured"
	}

	redisState := "ok"
	if o.Redis != nil {
		if err := o.Redis.Ping(ctx); err != nil {
			redisState = "error"
		}
	} else {
		redisState = "unconfigured"
	}

	status := "ok"
	code := http.StatusOK
	if dbState == "error" || redisState == "error" {
		status = "degraded"
		code = http.StatusServiceUnavailable
	}

	writeJSON(w, code, map[string]string{
		"status":  status,
		"db":      dbState,
		"redis":   redisState,
		"webhook": "registered",
		"version": o.Version,
	})
}
