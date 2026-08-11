# AGENT.md — Mandatory Rules for AI Coding Agents (Go)

> Applies to all Go services under `app-*/**` (the `cmd/` and `internal/` tree of every Go microservice in the Kentang Tech stack). Any AI coding agent (Claude Code, Cursor, Copilot, etc.) operating on a Go file in this repository MUST read this file before generating, editing, or reviewing code. A change that cannot satisfy these rules must be split or redesigned — do not bypass them.

**Stack:** Go (Golang) · Redis · PostgreSQL

**Version:** 1.1 · **Last Updated:** 2026-08-11 · **Status:** Active

---

## Before You Write Any Code

1. Check `WORKSPACE` (Graphify) for the target service's dependency graph.
2. Read `SYSTEM_MAP.md` — SSoT for topology, domain boundaries, data flows, async queues.
3. Read this file (`AGENT.md`) — coding standards for this codebase.

If the change alters domain boundaries, data structures, or service interactions, `SYSTEM_MAP.md` **must** be updated in the same PR. "I'll update the map later" is not acceptable.

---

## Enterprise Requirements

### 1.1 Source File Line Limit

Every new or modified `.go` source file under `app-*/internal/**` and `app-*/cmd/**` must stay under **250 lines**.

- [ ] Check line count before editing a file.
- [ ] Stop adding logic at ~200 lines — split before you're forced to.
- [ ] CI gate fails if any modified source file exceeds 250 lines.
- [ ] **Exception:** generated code (`*_gen.go`, `*.pb.go`, and any tool-generated output) is exempt — it isn't hand-authored and splitting it isn't meaningful. Never hand-edit generated files to dodge this rule.

```bash
find app-*/internal app-*/cmd -name '*.go' \
  -not -name '*_gen.go' -not -name '*.pb.go' -not -path '*/vendor/*' \
  -print0 | xargs -0 wc -l | sort -nr | head -50
```

**Rationale:** a file creeping toward 250 lines signals mixed concerns — schema, business logic, and I/O tangled together — not just "long code." Splitting before the limit is hit prevents last-minute refactors under deadline pressure.

---

### 1.2 Required Header Format

Every new/modified source file must carry this header (values illustrative):

```go
// Package settingsservice provides workspace settings backed by PostgreSQL.
//
// @file      internal/console/settings/service.go
// @for       Console workspace settings service backed by database rows.
// @uses      PostgreSQL connection pool, internal/console/settings/repository
// @reason    Provides scoped settings data to app-console via app-gateway.
// @author    Dodi Rusmana <rusmanadodi@kentangtechstore.com>
// @layer     service
// @stability stable
// @since     2026-08-09
package settingsservice
```

This is a per-file header rather than Go's typical package-level `doc.go` convention — a deliberate project convention (not idiomatic Go by default) that keeps authorship, purpose, and layer explicit and grep-able across every service.

- [ ] `@file` matches the actual relative path under `app-*/`.
- [ ] `@for` — one sentence, responsibility only, no implementation detail.
- [ ] `@uses` — major dependencies (external services, shared internal packages, infra primitives), not every import.
- [ ] `@reason` — why the file exists, not what it does.
- [ ] `@author` — stays `Dodi Rusmana <rusmanadodi@kentangtechstore.com>` across all files.
- [ ] `@layer` — one of: `schema | domain | repository | service | handler | router | worker | job | util | config`.
- [ ] `@stability` — `experimental | stable | deprecated`. `deprecated` requires `@deprecated-reason` + `@deprecated-since`.
- [ ] `@since` — ISO date first introduced; don't touch on later edits, git blame owns that.
- [ ] Header continuation lines use `// text` (single space after `//`) — NOT space-aligned continuation (`//            text`). gofmt reflows the 12-space style into `//\n//\ttext`, which makes the header non-gofmt-stable and blocks pre-commit (see Git Hooks section).
- [ ] CI gate fails on any modified/new file missing a required tag.
- [ ] Exempt: generated code (same exception as 1.1).

---

### 1.3 Error Message Language

All error messages returned to clients MUST be in **English**.

- [ ] No hardcoded Indonesian (or any non-English) in client-facing error payloads, HTTP status text, or validation messages.
- [ ] Internal structured logs may stay in Indonesian for team communication — never leak into the client JSON.
- [ ] Errors returned over the wire must be structured, not free text:

```go
// apperror.Error is the only shape a handler may write to a client response body.
type Error struct {
	Code    string `json:"code"`    // machine-parseable, e.g. "WALLET_INSUFFICIENT_BALANCE"
	Message string `json:"message"` // English, human-readable
}

func (e *Error) Error() string { return e.Message }
```

Internally, wrap with `fmt.Errorf("charging wallet %s: %w", walletID, err)` for stack context — the wrapped chain never reaches the client directly; the `handler` layer maps it to an `apperror.Error` before writing the response.

---

### 1.4 Type Safety & Validation (Zero Tolerance)

- [ ] `any` / `interface{}` are **forbidden** in application code, except narrow I/O boundaries (raw `json.Unmarshal` target before validation, an untyped third-party response). Even there, decode into a concrete struct and validate immediately — don't let it propagate past the boundary function.
- [ ] Every external input — HTTP body, query params, path params, headers, queue payloads, webhook bodies — decodes into a typed struct (`schema` layer) and is validated before use. No `if v, ok := x.(string); ok` type-assertion chains standing in for real schema validation.
- [ ] `go vet` and `staticcheck` must pass clean in CI, alongside an aggregated linter suite that flags unchecked errors, missing `context` timeouts on outbound calls, unclosed HTTP response bodies, and unclosed DB rows/statements. No unexplained suppression comment; every one needs `// reason: <why> (TICKET-123)`.
- [ ] Env vars are parsed into a typed, validated `Config` struct at process boot — fail fast on missing/malformed config. No raw `os.Getenv()` scattered through business logic.

```go
type Req struct {
	PhoneNumber string `json:"phone_number" validate:"required,e164"`
	Amount      int64  `json:"amount"        validate:"required,gt=0"`
}

func decode(r *http.Request) (Req, error) {
	var req Req
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return Req{}, fmt.Errorf("decoding request: %w", err)
	}
	if err := validate.Struct(req); err != nil {
		return Req{}, fmt.Errorf("validating request: %w", err)
	}
	return req, nil
}
```

---

### 1.5 Layer Separation (Architectural Boundaries)

- [ ] Strict unidirectional flow: `schema → domain → repository → service → handler → router`. `repository` never imports `service` or `handler`; `handler` never contains raw SQL or direct Redis calls — that's `repository`'s job.
- [ ] `service` owns business logic/orchestration and must not import `net/http` (`*http.Request`/`http.ResponseWriter`) — keeps it reusable from workers/queue consumers, not just HTTP handlers.
- [ ] The compiler already does part of this job for you: Go rejects circular package imports at build time, and `internal/` packages are compiler-blocked from being imported by anything outside the module rooted at their parent. Layer boundaries here carry a compile-time guarantee, not just review discipline.
- [ ] What the compiler *can't* catch: cross-*service* reach-ins (one service importing another's `internal/...` via a monorepo replace directive). Still forbidden — run a Graphify check (`/graphify update`) before merging any change that adds a new cross-module import. A detected cross-service cycle or reach-in blocks the PR.
- [ ] Shared logic used by 2+ services lives in a designated shared Go module (e.g. `pkg/kentang-shared`), published via Go modules — not vendored or symlinked across service boundaries.

Canonical layout:
```
app-<service>/
  cmd/<service>/main.go   # composition root — wiring only, no logic
  internal/
    domain/                # entities, value objects, domain events
    repository/            # PostgreSQL / Redis data access
    service/                # business logic, orchestration
    handler/                # HTTP handlers
    schema/                  # request/response DTOs + validation tags
    config/                  # typed, validated env config
  migrations/               # schema migrations (up/down pairs)
```

---

### 1.6 Resilience & Fault Tolerance (Production Readiness)

- [ ] Every outbound call to a third party (DigiFlazz, OkeConnect, Midtrans, Tripay, Xendit) runs under a `context` with an explicit timeout — never `context.Background()` passed straight into an HTTP client, never a client left with no timeout configured.
- [ ] Payment/webhook handlers are idempotent by design — Redis replay-guard, HMAC-SHA256 S2S authentication on every inbound service-to-service call.
- [ ] Every new background worker/queue consumer states retry policy (max attempts, backoff strategy — fixed, exponential, jittered) and dead-letter handling explicitly. "Just retry forever" or "no retry" are not acceptable defaults without a stated reason.
- [ ] **Non-negotiable: every goroutine boundary recovers from panic.** A panic in an unrecovered goroutine kills the entire process — not just that request, not just that job, the whole binary — regardless of how deep in a worker pool it originated.
  ```go
  go func() {
      defer func() {
          if r := recover(); r != nil {
              logger.Error("worker panic recovered", "panic", r, "stack", debug.Stack())
          }
      }()
      // ...
  }()
  ```
- [ ] Goroutines have an explicit termination condition — no fire-and-forget spawns. Propagate `context` cancellation and manage lifecycle with a wait-group/error-group pattern; a leaked goroutine is a slow memory leak that won't show up until load.
- [ ] Rate-limiting at the gateway layer for any new public-facing endpoint; document the limit and rationale in the PR description.
- [ ] Structured logging for every error path, with enough context (request ID, service name, error code) to trace an incident without reproducing it locally. Never bare `print`-style logging in production paths.

---

### 1.7 Database & Query Discipline

- [ ] No unbounded `SELECT *` / fetch-all in request-serving paths — pagination or explicit `LIMIT`.
- [ ] N+1 patterns (loop + query-per-item) are blocked in review; batch/JOIN alternatives required, or the tradeoff documented for a deliberately small, bounded N.
- [ ] New indexed lookup columns ship with a migration adding the index.
- [ ] Destructive migrations (drops, data-lossy `ALTER`) require explicit sign-off in the PR and a working rollback (`down`) migration before merge.
- [ ] PostgreSQL connection-pool limits (max open connections, max idle connections, connection lifetime) are set explicitly per service, never left at library defaults. Concurrent goroutines make it easy to fan out far more simultaneous DB calls than a single-threaded request handler would — an unbounded pool exhausts Postgres `max_connections` fast under real concurrency.

---

## Core Development Paradigms

Untuk mendukung stabilitas microservices dan mempercepat delivery, codebase ini mengadopsi paradigma pengembangan berikut secara bersamaan — melengkapi TDD dan DDD di bawah.

### 2.1 Testing Discipline (TDD-First, No Green-Skip)

- [ ] New `service`/`repository` logic has tests written before or alongside implementation — no PR merges net-new untested logic in these layers.
- [ ] No `t.Skip()`, no isolated `-run` filters, no commented-out tests left in a merged diff.
- [ ] Every new HTTP route has: one happy-path test, one validation-failure test (bad payload against the `schema` layer), one auth-failure test if protected — via `net/http/httptest`.
- [ ] Idempotency-critical paths (payment callbacks, H2H webhooks) get an explicit duplicate-request test, not just happy path.
- [ ] `go test -race ./...` is a required CI gate for any package touching goroutines, channels, or shared state. Treat a race-detector failure with the same severity as a failing unit test — data races are silent until they aren't.

### 2.2 Domain-Driven Design (DDD) Enforcement

- [ ] **Ubiquitous Language:** Penamaan *type*, *method*, dan API HARUS merefleksikan terminologi bisnis secara ketat (contoh: gunakan `WalletDebited` bukan `UpdateBalance`). Istilah yang dipakai tim bisnis dipakai apa adanya, jangan diterjemahkan ke istilah teknis generik.
- [ ] **Bounded Contexts:** Akses data lintas domain (misal servis A butuh data dari servis B) dilarang keras lewat *direct query*, `JOIN`, atau meng-*import* package `repository` milik domain lain. Integrasi wajib lewat panggilan API internal eksplisit (*Anti-Corruption Layer*) atau *Domain Events* asinkron (Redis Pub/Sub atau *message queue*). `internal/` memberi *backstop* level compiler untuk hal ini *di dalam* satu module — tapi lintas servis yang deploy terpisah, ini tetap disiplin review, bukan jaminan compiler.
- [ ] **Rich Domain Models (No Anemic Models):** *Entity*/*Aggregate* wajib mengenkapsulasi aturan bisnis dan transisi *state*-nya sendiri lewat *method*, dengan *field* unexported — bukan *struct* ter-*export* yang dimodifikasi prosedural dari `service`. Lapisan `service` mengorkestrasi; *domain type* yang mengeksekusi logika.
  ```go
  type Wallet struct {
      id      WalletID
      balance Money // unexported — tidak bisa dimutasi langsung dari luar package
  }

  func (w *Wallet) Debit(amount Money) (WalletDebited, error) {
      if amount.GreaterThan(w.balance) {
          return WalletDebited{}, ErrInsufficientBalance
      }
      w.balance = w.balance.Sub(amount)
      return WalletDebited{WalletID: w.id, Amount: amount}, nil
  }
  ```
- [ ] **Value Objects over Primitives:** `Money`, `Email`, `PhoneNumber`, `TransactionID` harus berupa tipe *immutable* — *struct* kecil dengan *field* unexported plus *constructor* yang memvalidasi (`NewMoney(cents int64, currency string) (Money, error)`) — bukan `int64`/`string` mentah yang lewat begitu saja di seluruh aplikasi.
- [ ] **Aggregate Roots as Mutation Boundary:** `repository` hanya boleh menyimpan/memuat *Aggregate Root*. Jangan buat *repository* mandiri untuk *child entity*. Semua mutasi *child entity* wajib lewat *method* di *Aggregate Root*.

#### 2.3 EDD (Event-Driven Development)
- [ ] Komunikasi *stateful* lintas domain wajib pakai *event* asinkron (Redis Pub/Sub atau *queue*), bukan panggilan sinkron — mencegah *cascading timeout* antar servis.
- [ ] Setiap mutasi penting pada *Aggregate Root* wajib memancarkan *Domain Event* (`PaymentSucceeded`, `UserRegistered`) agar servis lain bisa bereaksi mandiri.

#### 2.4 CDD (Contract-Driven Development)
- [ ] **Schema-First:** Definisi *request*/*response* ditulis sebagai *typed struct* dengan aturan validasi di package `schema` **sebelum** logika handler ditulis. Kalau servis punya API publik, dukung dengan spesifikasi kontrak eksplisit (OpenAPI, atau `.proto` untuk gRPC) yang divalidasi terhadap struct tersebut.
- [ ] **Contract Immutability:** Kontrak yang sudah dikonsumsi servis lain tidak boleh *breaking change* (ubah tipe, hapus *field*) tanpa *versioning* (`/v2/...`).

#### 2.5 BDD (Behavior-Driven Development)
- [ ] *Integration test* ditulis dengan pola pikir Given-When-Then, nama test bisa dipahami tim bisnis tanpa baca isinya.
- [ ] Contoh: `TestPayBill_GivenSufficientBalance_ThenBalanceDecreasesAndMutationIsRecorded`.
- [ ] **Catatan solo-dev:** jangan tambah *framework* BDD terpisah cuma demi sintaks Given-When-Then literal — `testing` bawaan Go dengan penamaan disiplin dan *table-driven test* sudah cukup jelas tanpa nambah DSL testing kedua yang harus dirawat.

#### 2.6 FDD (Feature-Driven Development)
- [ ] Task di tracker adalah fitur independen yang bernilai langsung untuk bisnis ("Kalkulasi Diskon"), bukan task infrastruktur murni ("Bikin Tabel").
- [ ] Tiap fitur cukup kecil untuk diimplementasi, ditest, dan di-*merge* dalam beberapa hari.

---

### 1.9 Documentation Sync Obligation

- [ ] Any change altering domain boundaries, data structures, service interactions, or async topology requires a `SYSTEM_MAP.md` update **in the same PR**.
- [ ] `/graphify update` runs, and its output is reviewed, on any task touching module dependencies.
- [ ] PR description states explicitly whether `SYSTEM_MAP.md` / this `AGENT.md` were reviewed and needed updates — "N/A, no structural change" is acceptable but required.

---

## Stack

- **Go (Golang)** — latest stable release (currently 1.26.5), pinned via the `toolchain` directive in `go.mod`.
- **PostgreSQL** — primary datastore. Connection-pool limits set explicitly per service (see §1.7).
- **Redis** — replay guard, rate limiter, cache, idempotence store.
- **HMAC-SHA256** — service-to-service (S2S) authentication (see §1.6).

Prefer the Go standard library over third-party frameworks by default (`net/http`, `database/sql`, `encoding/json`, `log/slog`, `context`, `crypto/hmac`, `crypto/sha256`). Reach for a third-party dependency only when the standard library genuinely can't do the job cleanly — struct-tag validation and a PostgreSQL driver being the two unavoidable exceptions — and document the choice in that service's `README.md`.

---

## Git Hooks & Secret Scanning (Enforced)

Repo ini memakai git hooks yang DI-COMMIT (`.githooks/`, diaktifkan sekali per
clone via `bash scripts/install-githooks.sh` → `core.hooksPath`). Setiap AI
agent WAJIB tahu dan mematuhi gate berikut — jangan menganggapnya opsional:

| Hook | Gate | Akibat bila gagal |
|------|------|-------------------|
| `pre-commit` | 1) **gitleaks** `protect --staged` (fail-closed bila gitleaks absen) · 2) **gofmt** semua `.go` yang di-stage · 3) **golangci-lint** baris baru di `bot/` (`--new-from-patch`, config `.golangci.yml`) · 4) `git diff --cached --check` (whitespace) | commit ditolak |
| `commit-msg` | **Conventional Commits**: `type(scope)!: subject` (`feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert`) | commit ditolak |
| `pre-push` | gitleaks scan commit range (validasi `git rev-list` dulu, fail-closed) + `go build`/`vet` (root & `bot/`) + golangci-lint penuh `bot/` | push ditolak |
| CI (`security.yml`) | gitleaks-action + go root build/vet + bot build/vet/test `-race` + golangci-lint (wajib, tidak bisa di-skip) | merge ditolak |

Aturan wajib untuk AI agent:

- [ ] **Jangan pernah bypass** (`git commit --no-verify` / `--push` bypass) untuk
      secret atau lint — CI tetap enforce, dan secret yang lolos = insiden keamanan.
- [ ] Setiap Go baru/diedit di `bot/` harus lolos **golangci-lint** sebelum
      dinyatakan selesai: `(cd bot && golangci-lint run ./...)` — linter set:
      `errcheck, gosimple, govet, ineffassign, staticcheck, unused, misspell, bodyclose`.
- [ ] File Go wajib **gofmt-clean** — cek `gofmt -l .` pada modul yang diubah.
- [ ] Pesan commit wajib Conventional Commits (dipaksa `commit-msg` hook).
- [ ] **Secret scan** (`gitleaks.toml`): kredensial hanya di `.env` (gitignored);
      allowlist gitleaks berisi HANYA false positive terverifikasi — menambah
      entri allowlist tanpa justifikasi tertulis = bypass keamanan.
- [ ] Header kontinuasi `// text` (1 spasi) — lihat §1.2.

Catatan teknis (jangan diubah tanpa alasan): `git diff --cached --check` tidak
boleh diberi `--quiet` (implisit `--exit-code`, selalu gagal saat ada staging);
golangci-lint versi **v1.64.x** (config format v1 tanpa field `version`) —
jangan upgrade ke v2 tanpa migrasi `.golangci.yml`.

---

## Compliance Checklist (per-PR)

- [ ] File(s) checked against 1.1 line limit (generated files exempt)
- [ ] Header(s) present and complete per 1.2
- [ ] No client-facing non-English error strings (1.3)
- [ ] No new `any`/`interface{}` outside declared I/O boundaries (1.4)
- [ ] Layer boundaries respected; no cross-service `internal/` reach-ins (1.5)
- [ ] Tests present for new service/repository logic; `-race` clean; no skipped tests (2.1)
- [ ] Every outbound call has an explicit `context` timeout; every goroutine recovers from panic (1.6)
- [ ] No unbounded queries; N+1 documented if accepted; indexes added; pool limits set (1.7)
- [ ] `SYSTEM_MAP.md` updated or explicitly marked N/A (1.9)
- [ ] Modified Go files gofmt-clean; bot/ golangci-lint-clean (enforced by pre-commit + CI)
- [ ] No secrets staged; gitleaks allowlist untouched without written justification (Git Hooks)
- [ ] Commit message follows Conventional Commits (commit-msg hook)

---

*Governance file for AI coding agents working in this Go codebase. Do not edit ad hoc — changes go through the same review process as other project documentation. || Created for Dodi Rusmana*
