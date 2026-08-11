# SYSTEM_MAP — Bot Auto-Order (`/bot`)

> SSoT ringkas topologi & batas domain modul `/bot`. Diperbarui dalam PR yang
> sama bila struktur berubah signifikan (AGENTS.md §1.9 / PRD §10).
> Versi terakhir disinkronkan pada **M7 partial — hardening: coverage gap tertutup + load test + UAT checklist** (v1.22, 2026-08-11).

## Topologi

```
Telegram Bot API ──HTTPS──► Nginx (TLS) ──► bot :8443 (Go, /api/v1)
                                              │
                    ┌─────────────────────────┼──────────────────────┐
                    ▼                         ▼                      ▼
              PostgreSQL (GORM)          Redis (go-redis)     X-UI Panel (REST API, M2+)
              orders/users/clients/      session xui,          login + cookie, addClient,
              servers/payments/pricing   dedup update_id,      updateClient, delClient,
              (7 tabel, PRD §13)         gate cache, rate      traffic (M2+)
                                         limit, per-user lock (M3)
                                         fsm topup {id} (M5)
                                         trial counter {id} (M6)
                                                                      │
                                                                      ▼
                                                              KentangTech Payment API (QRIS, M5+)
                                                              — di-defer: seam PaymentGateway → StubGateway
```

## Alur data (satu arah, layer AGENTS.md §1.5)

```
Telegram webhook (POST /api/v1/webhooks/telegram)
        │ secret → parse → dedup update_id (Redis)
        ▼
internal/handler/http ──worker pool (bounded)──► internal/handler/telegram (dispatcher)
        │                                              │ chain: ban → gate → rate-limit → route
        │ (health, payments stub M5)                   ▼
        │                                   internal/service/telegram (webhook, gate, ban,
        │                                   rate limit, menu — M3)
        │                                   internal/repository/telegram (go-telegram/bot)
        ▼
internal/repository/{postgres,redis} ──► migrations/ (embed, boot-time)
        │
        └──► internal/repository/xui (M2: login, CRUD client, traffic) ──► X-UI Panel

Alur topup (M5 partial, FR-06):

Telegram callback / teks FSM ──► dispatcher (routeTopup, /cancel)
        │  topup:menu → topup:amount:N → topup:confirm:N
        ▼
internal/service/topup (Quote fee §15.7, min/max) ──► PaymentGateway seam
        │                                                 StubGateway (sementara) → teks unavailable
        ├── internal/repository/redis (TopupFSM: bot:fsm:topup:{id}, TTL 10 mnt)
        └── internal/service/user (Balance — saldo saat ini)

Alur trial (M6 partial, FR-07):

Telegram callback / perintah /trial ──► dispatcher (routeTrial)
        │  trial:menu → trial:server:{id} → trial:confirm:{id}
        ▼
internal/service/trial (daily limit 2x/hari, claim anti-race + rollback)
        │   TTL s.d. tengah malam (TIME_LOCATION)
        ├── internal/repository/redis (TrialCounter: bot:trial:{userID})
        └── internal/service/order (CreateTrial — TANPA debit) ──► serversvc.CreateTrialClient (1 jam)
        └── domain.NewTrialClient (is_trial=true) ──► postgres vpn_clients

Alur notifikasi kadaluarsa (M6 partial, FR-09):

internal/job/interval.go ──IntervalWorker (stdlib time.Ticker — sweep langsung + tiap interval)
        │   ctx cancel + WaitGroup drain; panic-recover + timeout 2 mnt (AGENTS.md §1.6)
        ▼
internal/service/expiry (RunOnce: jendela eksklusif (lower, upper] × EXPIRY_NOTIFY_DAYS)
        │   sekali per ambang H-7/H-3/H-1; send gagal → tidak ditandai → retry
        ├── internal/repository/postgres (ListExpiryCandidates JOIN users, MarkNotified)
        │       notified_expiry INT (0/7/3/1 — migrasi 000003); renewal reset → siklus ulang
        └── internal/service/telegram (ExpiryNotifyText) ──► repository/telegram SendMessage
        └── tanggal diformat TIME_LOCATION (FR-09 AC)

Alur admin (M6 partial, FR-11):

Perintah /admin atau callback admin:* ──► dispatcher (routeAdmin; ADMIN_IDS)
        │   harga: admin:price → admin:plan:CODE:DAYS → setprice/toggle/reload
        │   broadcast: input teks → pratinjau → kirim (chunked 100/6 dtk, async)
        │   ban/unban: input ID → konfirmasi
        ▼
internal/service/admin (price ops, BanUser/UnbanUser dua layer, Broadcast)
        ├── internal/service/pricing (ListAll/Get/SetPrice/SetEnabled/Reload) ──► postgres pricing
        ├── internal/service/user (SetBanned, ListTelegramIDs, CountUsers) ──► postgres users
        ├── internal/service/telegram BanService (marker bot:ban:{id}, TTL 1 thn)
        ├── internal/repository/redis (AdminFSM bot:fsm:admin:{id}, broadcast lock bot:admin:broadcast)
        └── internal/repository/telegram SendMessage (broadcast + laporan ke admin)

Alur sync traffic (M6, PRD §16.2):

internal/job/interval.go ──IntervalWorker (sweep langsung + tiap TRAFFIC_SYNC_INTERVAL_MIN)
        │   ctx cancel + WaitGroup drain; panic-recover + timeout sweep; per-server timeout XUI_API_TIMEOUT
        ▼
internal/service/traffic (RunOnce: kandidat aktif group per server)
        │   satu panel gagal → log + lanjut (PRD §16.2)
        ├── internal/repository/postgres (ListTrafficCandidates bounded last_sync NULLS FIRST)
        ├── internal/repository/xui (GetInbounds → clientStats; GetOnlineClients → last_online)
        └── SyncTrafficBatch (UPDATE ... FROM VALUES — satu statement, last_online COALESCE)

Catatan: saat API KentangTech Go final, ganti StubGateway dengan client HTTP
nyata (internal/repository/kts) — menu/flow/FSM tidak berubah (PRD §15.7).
```

- `cmd/bot` = wiring saja, tanpa logika bisnis.
- `internal/handler/http` = boundary HTTP: webhook telegram (M3 real), health, payments (stub M5).
- `internal/handler/telegram` = dispatcher + middleware chain (M3) + flow shop (M4) + flow topup (M5).
- `internal/service/telegram` = webhook register, gate grup (cache 6 jam), ban, rate limit, menu (M3).
- `internal/service/topup` (M5 partial) = Quote fee §15.7 + seam `PaymentGateway` (StubGateway) — API KentangTech di-defer.
- `internal/service/trial` (M6) = FR-07 policy: daily limit + claim anti-race + TTL s.d. tengah malam.
- `internal/service/expiry` (M6) = FR-09 policy: jendela ambang H-7/H-3/H-1, kirim + tandai sekali per ambang.
- `internal/service/admin` (M6 partial) = FR-11 ops: price/toggle/reload, ban/unban dua layer, broadcast chunked (lock + timeout + recover).
- `internal/service/traffic` (M6) = PRD §16.2 policy: kandidat aktif group per server → GetInbounds + GetOnlineClients → batch update; server gagal → lanjut.
- `internal/job` (M6) = **`IntervalWorker` generik** (loop stdlib ticker, panic-recover, terminasi ctx) — dipakai notifikasi & sync traffic.
- `internal/service/order` (M4/M6) = Purchase/Renew/CreateTrial: state machine, panel provisioning, debit atomik + ledger (trial tanpa debit).
- `internal/service/pricing` (M4) = seed JSON → pricing DB; harga live untuk order.
- `internal/service/server` (M4) = seed panel terenkripsi, PickForCountry, gateway XUI.
- `internal/service/user` (M4) = ensure user, balance, debit/kredit atomik + ledger (dipakai topup utk cek saldo).
- `internal/repository/telegram` = wrapper typed go-telegram/bot (M3).
- `internal/repository/postgres` = GORM + pool limits + runner migration; repos user/pricing/server/client/order (M4).
- `internal/repository/redis` = go-redis + ops (dedup, gate cache, rate limit, per-user lock, `TopupFSM` key, `AdminFSM` key, broadcast lock).
- `internal/repository/xui` = REST client panel (login + session cache Redis + CRUD client + traffic).
- `internal/crypto` = AES-256-GCM secret box (kredensial panel server).
- `internal/domain` (M4) = Money (int64 + Scanner/Valuer), Order state machine, VpnPlan, VPNClient, random ID.
- `migrations/` = SQL up/down golang-migrate (000001 init, 000002 insecure_tls, 000003 expiry_notify_day), di-embed & diterapkan saat boot.
- Service/domain/schema mengikuti aturan layer yang sama (AGENTS.md §1.5).

## Seam PaymentGateway (keputusan produk, M5)

- Bot hanya bergantung pada interface `topupsvc.PaymentGateway`
  (`CreatePayment`) — **bukan** pada kontrak HTTP tertentu.
- Saat ini `StubGateway` → `ErrPaymentAPIUnavailable`; teks ramah ditampilkan
  ke user, flow tetap utuh. Implementasi nyata (client Go API KentangTech)
  di-swap tanpa mengubah menu/flow/FSM (PRD §15.7, v1.13).
- FSM input nominal custom: `bot:fsm:topup:{id}` (Redis, TTL 10 mnt) —
  auto-expiry = crash guard; `/start`, `topup:menu`, tombol batal & `/cancel`
  membersihkannya.

## Batas domain (dilarang)

1. **Tidak ada import ke source panel** (`github.com/alireza0/x-ui/...`) —
   komunikasi ke panel hanya REST API (M2+). (AGENTS.md §1.5.)
2. **Tidak ada akses ke `x-ui.db` / proses xray panel.**
3. **Telegram wajib webhook** — polling dilarang (PRD §14.4); grep CI
   `getUpdates|DeleteWebhook` harus 0.
4. `service` tidak meng-import `net/http` (AGENTS.md §1.5); hanya
   `repository/telegram` & `repository/xui` yang menyentuh HTTP client.
5. `handler/http` worker tidak memproses update bersamaan untuk user yang
   sama: per-user lock Redis `bot:lock:user:{id}` (TTL 30 s, di-release
   setelah handle).

## Dependensi eksternal

| Paket                          | Digunakan untuk                       |
|--------------------------------|---------------------------------------|
| `gorm.io/gorm` + driver postgres | ORM (M1)                            |
| `github.com/redis/go-redis/v9` | Client Redis (M1)                     |
| `github.com/golang-migrate/migrate/v4` | Migration embedded (M1)       |
| `github.com/go-telegram/bot`   | Telegram Bot API webhook (M3)         |
| `github.com/alicebob/miniredis/v2` | Test Redis ops in-memory (M3)     |
| stdlib `time.Ticker`           | Scheduler worker interval — notifikasi (FR-09) & sync traffic (§16.2); robfig/cron tidak dipakai: preferensi stdlib AGENTS.md |
| `github.com/skip2/go-qrcode`   | Render QR QRIS (M5)                   |
| stdlib `crypto/aes`            | Enkripsi AES-256-GCM kredensial server (M2) |

## Layanan infra (docker-compose)

`bot` (build ./) · `nginx` (TLS) · `postgres:16-alpine` · `redis:7-alpine` —
network `botnet` internal; env dari `.env` (PRD §19.2).

## Status milestone

| M   | Isi                                   | Status |
|-----|---------------------------------------|--------|
| M0  | Scaffolding + `/api/v1` convention    | ✅     |
| M1  | Config, PG+Redis, migration, /health  | ✅     |
| M1+ | Integration test migration & Redis (PG 16 + Redis 7 di host staging) | ✅     |
| M2  | X-UI client (login, CRUD client)      | ✅     |
| M3  | Webhook go-telegram/bot + dispatcher  | ✅     |
| M4  | Order flow + ledger                   | ✅     |
| M5  | Topup menu/flow + QRIS + webhook HMAC | 🔶 (flow ✅, API 🔜) |
| M6  | Trial, notifikasi, sync traffic, admin | ✅ (v1.21) |
| M7  | Hardening, test, UAT                  | 🔶 (v1.22: coverage gap ✅, load test ✅, UAT checklist ✅) |
