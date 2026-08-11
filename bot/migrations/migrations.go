// Package migrations embeds the SQL schema migrations (golang-migrate).
//
// @file      migrations/migrations.go
// @for       Embed SQL up/down migrations for the bot database (PRD §13).
// @uses      embed (stdlib only)
// @reason    Keeps schema versioned and applied at boot via golang-migrate iofs source (AGENTS.md §1.7).
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     schema
// @stability stable
// @since     2026-08-11
package migrations

import "embed"

// FS holds the *.sql migration files, applied with golang-migrate (iofs).
//
//go:embed *.sql
var FS embed.FS
