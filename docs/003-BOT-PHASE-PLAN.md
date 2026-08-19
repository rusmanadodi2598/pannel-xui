# Plan — Bot Order Hardening (Phased)

| Field         | Value                                                              |
|---------------|--------------------------------------------------------------------|
| **Dokumen**   | 003-BOT-PHASE-PLAN                                                  |
| **Status**    | Draft — Phase 1 ✅ · Phase 2 ✅ · Phase 3 ✅ · Phase 4 ✅ (PG Aggregate + sync v1.48) · Phase 5 ✅ (admin REST API §26.5 + sync v1.49) |
| **Tanggal**   | 2026-08-17                                                          |
| **Penulis**   | Dodi Rusmana `<rusmanadodi@kentangtechstore.com>`                   |
| **Scope**     | Hanya `/bot` — panel x-ui **tidak boleh berubah**                   |
| **Governance**| Patuh `AGENTS.md` (header §1.2, line limit §1.1, type safety §1.4, resilience §1.6, DDD §2.2) |
| **Referensi** | `docs/001-PRD-BOT-ORDER.md` (v1.46), `docs/002-UAT-BOT-ORDER.md`; backend: `kentangtech/docs/public/001-PRD-SPEC-FIRTS-API-V1.md` (S2S HMAC), `013-PUBLIC-MERCHANT-OUTBOX-WEBHOOKS.md` (webhook transport), `015-SPEC-PG-AGGREGATE.md` (PG charge) |

---

## Ringkasan (dari analisis 2026-08-17)

Kode `/bot` sehat secara umum: build/vet/test-race hijau, layer §1.5 rapi,
header §1.2 lengkap, panic-recovery §1.6 lengkap, test coverage tinggi
(102 file test vs 137 file non-test). Temuan yang perlu dikerjakan dipecah
menjadi 4 phase berurutan, diurutkan berdasarkan **risiko CI gate → kebenaran
domain → gap fungsional**.

> **Catatan §1.3 (bukan gap):** error berbahasa Indonesia di
> `service/order/order.go` (`ErrInsufficientBalance = "saldo tidak cukup"`, dst.)
> adalah **copy Telegram**, bukan error payload HTTP JSON. Sudah didokumentasikan
> sebagai keputusan produk di PRD §10 + N7 ("copy Telegram = konten i18n id-ID —
> bukan error payload"). Tidak perlu diubah.

---

## Phase 1 — Structural hygiene: split file di ambang line-limit §1.1

**Risiko:** 4 file tepat di 248–249 baris. Tambahan 1 baris = CI `wc -l` gagal.
Ini prasyarat sebelum phase berikutnya (yang pasti menambah kode).

| File | Baris | Arah split |
|------|-------|------------|
| `internal/handler/telegram/buy.go` | 249 | pisah routing callback vs render teks/keyboard |
| `internal/repository/postgres/client_repo.go` | 248 | pisah query traffic/expiry ke file baru (pola `client_repo_page.go`/`client_repo_trial.go` yang sudah ada) |
| `internal/handler/telegram/dispatcher.go` | 248 | pisah middleware chain vs route table |
| `cmd/bot/main.go` | 248 | pindah wiring worker (expiry/traffic/health/cleanup starter) ke file baru (pola `shop.go`/`notify.go`) |

**AC Phase 1:**
- [x] Semua file `.go` non-generated `< 250` baris (target `< 200`).
- [x] **Tanpa perubahan perilaku** — murni refactor, header §1.2 tetap di tiap file baru.
- [x] `gofmt -l .` kosong; `go build ./...`; `go vet ./...` hijau.
- [x] `go test ./...` hijau (unit; integration PG/Redis gagal hanya karena service lokal mati — pre-existing).

**Follow-up split (keputusan user, 2026-08-17):**
- `trial.go` 244 → `trial.go` 115 + `trial_execute.go` 149
- `admin_user.go` 243 → `admin_user.go` 189 + `admin_broadcast.go` 76

---

## Phase 2 — Domain hardening: invariant value object `Money` (DDD §2.2)

**Gap:** `Money.Sub`/`Add` tidak memvalidasi hasil (bisa negatif/overflow),
bertentangan dengan value object immutable yang mengeksekusi aturannya sendiri.

**Scope `internal/domain/money.go`:**
- `Sub(other) (Money, error)` — tolak hasil negatif (`ErrNegativeMoney`).
- `Add(other) (Money, error)` — guard overflow `int64`.
- Perbarui semua caller + test.

**AC Phase 2:**
- [x] Test: `Sub` negatif → error; `Add` overflow → error; happy path tetap.
- [x] Caller ter-update — **nihil**: `Money.Add`/`Money.Sub` tidak dipakai di production (aritmatika saldo via SQL `Debit`/`Credit`), jadi tidak ada caller yang perlu diubah.
- [x] `go test -race ./internal/domain/...` hijau.

---

## Phase 3 — FR-04 atomicity: debit-first + auto-refund (bukan worker reconciliation)

**Keputusan user (2026-08-17):** alih-alih worker reconciliation, purchase
**disamakan dengan renewal v1.37** — debit-first + auto-refund — dan sisa
orphan "panel client tanpa row DB" ditutup dengan **insert row sebelum panel**
(opsi (a)). Ini menghilangkan akar masalah, bukan sekadar membersihkan gejala.

**Alur purchase baru (v1.47):**
```
1. create order (pending → processing)
2. prepare client (read-only: inbound + creds + share link)  ← tidak sentuh panel
3. insert row vpn_clients                                     ← kredensial sudah ada
4. debit saldo atomik (guard balance >= harga)               ← uang pindah dulu
5. panel commit (addClient)                                  ← langkah terakhir yang bisa gagal
   └─ gagal → refund (Credit + ledger) + hapus row, order failed
```
Gagal di step 2–4 = order `failed` bersih (tanpa akun panel, tanpa uang
terpotong). Gagal di step 5 = refund eksak + hapus row. **Akun aktif tanpa
bayar mustahil terjadi.**

**Perubahan teknis:**
- `domain.PreparedClient` (baru) — record bot-side + param commit panel
  (subId/expiry/quota) dibangun **tanpa mutasi panel**.
- `service/server/provision.go` (baru) — split `provisionClient` →
  `PrepareClient` (read-only) + `CommitClient` (addClient); `CreateClient`/
  `CreateTrialClient` rebuild dari prepare+commit.
- `service/order/order.go` — interface `PanelGateway` + `ClientStore`
  (`PrepareClient`/`CommitClient`; `Create`/`DeleteOwned`).
- `service/order/purchase.go` — rewrite ke urutan prepare → row → debit → commit.
- Fix: `prepareClient` sempat tidak mengisi `SubID` di `PanelClient` (FR-13)
  — ditambahkan agar sub URL tidak kosong.

**AC Phase 3:**
- [x] Debit gagal → row dihapus, order `failed`, panel tidak pernah disentuh.
- [x] Panel commit gagal → refund eksak + row dihapus, order `failed`.
- [x] Row DB selalu ada sebelum panel → tidak ada "akun gratis tanpa catatan".
- [x] Test: commit-failure → refund + hapus row + `failed` (order_test);
      FR-13 subId tetap dikembalikan (provision_subid_test).
- [x] `go test -race` order/server/domain/telegram hijau; build/vet/gofmt bersih.
- [ ] (sisa, non-blokir) `SYSTEM_MAP.md` + README: update deskripsi alur
      purchase debit-first — §1.9 (dikerjakan bersama Phase 4).

---

## Phase 4 — FR-06 completion: PG Aggregate charge + webhook HMAC (M5 selesai)

**Gap fungsional terbesar:** `StubGateway` return `ErrPaymentAPIUnavailable`;
`POST /api/v1/webhooks/payments` masih 501. Kontrak backend **sudah final &
implemented** (PG Aggregate, spec 015 Phases 1–6.1) — draft plan lama memakai
endpoint `autobuy-saldo` yang **tidak ada**; Phase 4 ini memakai kontrak nyata.

### 4.0 Kontrak backend (analisis 2026-08-18)

**Endpoints (semua di chain S2S HMAC merchant):**

| Method | Path | Hasil |
|---|---|---|
| POST | `/api/v1/pg/charges` | 201 `created` (tanpa panggil provider); `orderId` sama → 200 replay; dipakai merchant lain → 409 `DUPLICATE_ORDER` |
| POST | `/api/v1/pg/charges/{orderId}/confirm` | 202 `pending` + `checkoutUrl` (state-first: pending dipersist sebelum panggil Midtrans) |
| GET | `/api/v1/pg/charges/{orderId}` | 200 verify / 404 |

**S2S outbound (001 §2.3)** — header per request:
`X-API-Key`, `X-Timestamp` (epoch detik, ±300s), `X-Nonce` (UUIDv4,
replay guard), `X-Signature: sha256=hex(hmac_sha256(secret, canonical))`;
`Idempotency-Key` wajib utk mutasi (16–128 chars). Canonical string
(dipisah LF, tanpa trailing newline):

```
v1\n<apiKey>\n<timestamp>\n<nonce>\n<METHOD>\n<raw-path-with-query>\n<hex-sha256(rawBody)>
```

**Webhook masuk (013 §2)** — bot sebagai merchant menerima:
`X-Webhook-Signature: sha256=hex(hmac_sha256(secretKey, raw body))`
(secret sama dgn inbound S2S), `X-Webhook-Event: pg.charge`,
`X-Webhook-Id: pg.charge.{orderId}.{status}` (dedup, tanpa `:`). Status:
`succeeded` | `failed` | `expired`. Body `amount` = **GROSS** (2.5% MDR +
11% PPN, gross-up `ceil(net/0.97225)`); `refId` = `orderId`. Non-2xx →
gateway retry (max 5, backoff 30s×attempt) → dead-letter.

**Kredensial:** onboarding backend (`create-merchant`) → API key + secret
(sekali tampil) + daftarkan `webhook_url` bot. Env baru: `KTS_BASE_URL`,
`KTS_API_KEY`, `KTS_SECRET`, `KTS_CHARGE_TTL` (default 24h).

### 4.1 Alur topup baru (PG charge + webhook)

```
1. user pilih nominal → Quote (net→gross utk display, §15.7)
2. bot generate orderId (mis. tp_<random>, 4–50, charset [A-Za-z0-9._-])
   → persist row orders (order_type=topup, status pending, net)  ← SEBELUM panggil gateway (pola Phase 3: persist dulu)
3. POST /api/v1/pg/charges {orderId, amount: net} (S2S + Idempotency-Key)
   → 201 created; replay 200 → lanjut
4. POST /api/v1/pg/charges/{orderId}/confirm → 202 {checkoutUrl}
   → kirim link/QRIS ke user Telegram
5. settlement:
   a. webhook POST /api/v1/webhooks/payments (X-Webhook-Event: pg.charge)
      → verifikasi signature (constant-time, raw body) → 403 kalau salah
      → dedup X-Webhook-Id (Redis SETNX TTL 7d)
      → cari order by orderId → kredit NET atomik + ledger + notif user & grup
      → 200 HANYA setelah durable; 5xx kalau gagal (gateway retry)
   b. fallback poll: GET /api/v1/pg/charges/{orderId} (jeda ~30s s/d TTL)
      → paid → kredit idempoten (guard status order)
```

Catatan: body webhook mengirim **gross** — kredit pakai **NET dari order
lokal**, bukan dari body. `succeeded` → kredit; `failed`/`expired` → tandai
order, tanpa kredit. Webhook & poll saling idempoten (kredit sekali).

### 4.2 Scope teknis

- `internal/repository/kts` (baru): HTTP client PG charge + S2S signer
  (canonical 001 §2.3; timeout 30s §1.6; error mapping 401/404/409 → typed;
  X-Nonce UUIDv4 fresh per request).
- `internal/service/topup`: ganti `StubGateway` → `kts.Gateway`; seam
  `PaymentGateway` menyesuaikan (`CreateCharge`/`ConfirmCharge`/`GetCharge`
  atau satu `CreatePayment` yg mengembalikan orderId + checkoutUrl).
- Repo order topup (`internal/repository/postgres/topup_order_repo.go`):
  create-by-orderId, get-by-orderId, mark-paid/mark-failed (row `orders`,
  migration kecil utk kolom status charge/checkout_url bila perlu).
- Webhook `POST /api/v1/webhooks/payments` (route sudah ada, ganti stub 501):
  verifikasi `X-Webhook-Signature` (HMAC-SHA256 raw body,
  `subtle.ConstantTimeCompare`) → branch `X-Webhook-Event` → dedup
  `X-Webhook-Id` → kredit net + ledger + notif → 2xx setelah durable.
- Config: `KTS_*` di-parse + divalidasi di boot (fail fast §1.4).

### 4.3 AC Phase 4

- [x] FR-06 AC-1/2/3 + UAT §4 items `[ ]`: QRIS link tampil (`TopupPaymentText`
      + checkoutUrl), status, signature, idempotensi.
- [x] Signature salah → 403 tanpa diproses; webhook ganda → kredit sekali
      (duplicate-request test; dedup `X-Webhook-Id` + conditional transition).
- [x] `expired`/`failed` → tanpa kredit, order ditandai (`ApplySettlement`),
      notif user & grup (adapter `cmd/bot/topup_notify.go`).
- [x] Kredit NET dari order lokal (bukan gross body webhook) — 015 §4.4.
- [x] Poll fallback: **dilimpahkan ke gateway** — PG reconciler backend
      (015 Phase 6.1) sudah re-enqueue `pg.notify` utk charge pending yang
      webhook-nya hilang, jadi webhook tetap jalur primer; `kts.GetCharge`
      sudah tersedia utk poll manual/ops bila diperlukan.
- [x] `README.md` + `SYSTEM_MAP.md` + PRD milestone M5 di-update (termasuk
      sisa §1.9 Phase 3: alur purchase debit-first).
- [x] Build/vet/gofmt + `go test -race` hijau; header §1.2; file < 250 baris.

### 4.4 Status implementasi (2026-08-18) — wire selesai

| Komponen | Status |
|---|---|
| `internal/repository/kts` (S2S signer + PG charge client) | ✅ `signer.go`/`gateway.go`/`types.go` + test (`signer_test`, `gateway_test` — httptest 201/202/404/409) |
| Migration `000007_payments_telegram` + `PaymentRepo` | ✅ create/get/save-provider-ref/mark-failed/`MarkSettledTx` (conditional) + model `telegram_id` |
| `service/topup` rewrite | ✅ `CreatePayment` (persist → create → confirm), `ApplySettlement` (credit+mark satu transaksi), seam `PaymentGateway` → `kts.Client`; `StubGateway` dihapus |
| Webhook `POST /api/v1/webhooks/payments` | ✅ verifikasi `X-Webhook-Signature` (raw body, constant-time), branch event, dedup Redis, settle, 5xx → gateway retry |
| Config `KTS_*` | ✅ `KTS_BASE_URL`/`KTS_API_KEY`/`KTS_SECRET` (required, fail-fast), `KTS_CHARGE_TTL_MIN` (default 24h) |
| Wiring | ✅ `buildShop` (kts client + paymentRepo + notifier), `main.go` (Options Topup + secret), adapter `topup_notify.go` |
| Teks Telegram | ✅ `TopupPaymentText` (checkout URL), `TopupSettledText`, `AdminTopupNoticeText` |

**Catatan:** `KTS_SECRET` = secretKey merchant tunggal — dipakai utk signing
outbound S2S DAN verifikasi inbound `X-Webhook-Signature` (013 §2.2).

---

## Phase 5 — Admin REST API §26.5 (servers CRUD + orders/users read + topup trigger)

**Gap:** endpoint di `§26.5` katalog tertunda "nanti" — servers CRUD, orders/users read,
dan payments/topups trigger belum terwired; admin hanya bisa via Telegram.

### 5.1 Scope teknis

- Config: `REST_API_KEY` (opsional — kosong = surface tidak terdaftar).
- Auth: `X-API-Key` header constant-time → `withAPIKey` middleware.
- Repo `server_rest.go`: `GetAdminByID` (safe tanpa password), `UpdateServer`
  (password di-seal ulang), `DeleteServer` (guard: 409 bila ada client —
  mencegah cascade `ON DELETE CASCADE`)
- Service `server_rest.go`: `GetAdminByID`, `UpdateServer`, `DeleteServer`,
  `CheckHealth` (seam `statusFactory`)
- Handler: `api.go` (auth + envelope + `registerAdminAPI`), `servers_api.go`,
  `orders_api.go`, `users_api.go`, `topup_api.go`
- Wiring: expose `OrderRepo`/`ClientRepo`/`UserRepo` dari `shopBundle`

### 5.2 AC Phase 5

- [x] `REST_API_KEY` kosong → surface tidak terdaftar (404)
- [x] `X-API-Key` salah/missing → 401 tanpa proses
- [x] Server read **tanpa** `password_enc`/`username`; client read tanpa kredensial
- [x] `DELETE /servers/{id}` → 409 bila ada client (guard cascade)
- [x] `POST /payments/topups` → Quote (validasi min/max) → CreatePayment (PG)
- [x] Envelope §26.4 konsisten (data/meta/error)
- [x] Build/vet/gofmt + `go test -race` hijau; file < 250 baris
- [x] Doc sync §1.9 (PRD §26.5, SYSTEM_MAP, README, bot/README, .env.example)

---

## Definition of Done (umum, tiap phase)

- [ ] Build/vet/gofmt/lint sesuai gate `.githooks` + CI (tidak bypass).
- [ ] Test baru untuk logika service/repository (§2.1, tanpa `t.Skip`).
- [ ] Header §1.2 lengkap di tiap file baru/diedit; file < 250 baris (§1.1).
- [ ] Commit Conventional Commits (`feat|fix|refactor|test|docs|chore`).
- [ ] `SYSTEM_MAP.md`/PRD di-update bila batas domain/topologi berubah (§1.9).
