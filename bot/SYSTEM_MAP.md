# SYSTEM_MAP — Bot Auto-Order (`/bot`)

> SSoT ringkas topologi & batas domain modul `/bot`. Diperbarui dalam PR yang
> sama bila struktur berubah signifikan (AGENTS.md §1.9 / PRD §10).
> Versi terakhir disinkronkan pada **M7 partial — hardening: coverage gap tertutup + load test + UAT checklist + Riwayat FR-14 + Bantuan FR-15 + pagination Akun FR-08 AC-1 + hapus akun FR-08 AC-4 + traffic & refresh manual FR-08 AC-3 + convert YAML Clash/Meta FR-08 AC-2 + detail akun AC-1 lengkap (Limit IP + traffic terpakai) + hapus tercatat di Riwayat AC-4 + revisi minor UI akun (kredensial protocol-aware UUID/Password; URL build hanya di ekspor .txt; pesan sukses Beli/Trial tanpa URL) + renew paid-only + idempotence (FindInFlight) + alur uang debit-first + auto-refund (v1.37) + fix panel renew "empty client ID" — spec penuh client dipertahankan, PanelClientKey per-protocol vless→UUID/trojan→password/ss→email (v1.38) + adjust saldo admin dengan idempotence confirm arm-state (v1.39) + FR-11 lengkap: manajemen server (list/detail/toggle open+active, add-server FSM 6 langkah srvadd:confirm arm-state), statistik (OrderStats revenue/status/users/client aktif + recent orders), audit log admin_audit_log (migrasi 000005, index created_at DESC) tercatat di semua aksi admin (v1.40) + AC FR-04 terakhir: notifikasi order sukses (beli/renew) ke NOTIFICATION_GROUP_ID — seam OrderNotifier variadic, best-effort, trial dikecualikan, gate != 0 (v1.41) + UX keyboard zigzag 2-1-2-1 semua sub-menu — helper packRows/backBtn di service/telegram/menu_rows.go, dipakai semua picker/aksi/konfirmasi, urutan tombol dipertahankan (v1.42) + brand pada template notifikasi: BrandName KENTANG TECH (bukan KENTANG TECH STORE) + BrandHeader 🏪 di menu_brand.go, diterapkan ke 7 template pesan transaksi (notice admin, expiry, sukses beli/renew/trial, topup summary & QR) — banner brand = satu-satunya pengecualian icon policy (v1.43) + konsistensi brand ditutup: ringkasan konfirmasi & pesan gagal ber-brand, ejaan diseragamkan (header ekspor .txt / HomeText / HelpInfoText pakai BrandName — tanpa ejaan legacy) (v1.44) + worker health check panel: healthsvc RunOnce ping tiap panel aktif (GET /xui/API/server/status) → health_status ok/down + last_health_check (server_health.go), ListBuyable mengecualikan health_status=down (IS DISTINCT FROM 'down'; default 'unknown' tetap dijual — server mati tidak dijual PRD §17) (v1.45) + worker trial cleanup: trialcleanupsvc disable akun trial expired di panel (serversvc.DisableClients — satu GetInbounds per server, spec raw di-patch enable=false, kunci per-protocol dari spec) lalu MarkTrialExpired (is_active=false/is_expired=true); panel gagal → row tidak ditandai (v1.45) + FR-13 subscription URL: sub_id dipersist (migrasi 000006: sub_id + subscription_json_url), ordersvc.SubLinks membangun URL dari config SUB_BASE_URL/SUB_PATH/SUB_JSON_PATH (Opsi 2 — domain sama panel port beda), persist saat Purchase & CreateTrial, URL tampil HANYA di Ekspor .txt (akun lama tanpa backfill = legacy gap terdokumentasi) (v1.46) + purchase debit-first + auto-refund (parity renew v1.37): prepare client (read-only) → insert row vpn_clients → debit atomik → commit panel; gagal commit → refund + hapus row — akun aktif tanpa bayar mustahil (v1.47, domain.PreparedClient + service/server/provision.go PrepareClient/CommitClient) + M5 selesai: PG Aggregate charge (internal/repository/kts: S2S HMAC signer + Create/Confirm/GetCharge, amount = NET) + webhook pg.charge (X-Webhook-Signature raw body, X-Webhook-Event, dedup X-Webhook-Id) + settlement idempoten (PaymentRepo.MarkSettledTx conditional, migrasi 000007 payments.telegram_id) — StubGateway dihapus (v1.48)** (v1.48, 2026-08-18).

## Topologi

```
Telegram Bot API ──HTTPS──► Nginx (TLS) ──► bot :8443 (Go, /api/v1)
                                              │
                    ┌─────────────────────────┼──────────────────────┐
                    ▼                         ▼                      ▼
              PostgreSQL (GORM)          Redis (go-redis)     X-UI Panel (REST API, M2+)
              orders/users/clients/      session xui,          login + cookie, addClient,
              servers/payments/pricing   dedup update_id,      updateClient, delClient,
              (8 tabel, PRD §13)         gate cache, rate      traffic (M2+)
                                         limit, per-user lock (M3)
                                         fsm topup {id} (M5)
                                         trial counter {id} (M6)
                                         dedup webhook {id} (M5)
                                                                      │
                                                                      ▼
                                                              KentangTech PG Aggregate (QRIS, M5)
                                                              S2S HMAC: POST /api/v1/pg/charges
                                                              create/confirm/verify → webhook pg.charge
                                                              (kontrak 013/015, v1.48)
```

## Alur data (satu arah, layer AGENTS.md §1.5)

```
Telegram webhook (POST /api/v1/webhooks/telegram)
        │ secret → parse → dedup update_id (Redis)
        ▼
internal/handler/http ──worker pool (bounded)──► internal/handler/telegram (dispatcher)
        │                                              │ chain: ban → gate → rate-limit → route
        │ (health, payments webhook pg.charge M5)      ▼
        │                                   internal/service/telegram (webhook, gate, ban,
        │                                   rate limit, menu — M3)
        │                                   internal/repository/telegram (go-telegram/bot)
        ▼
internal/repository/{postgres,redis} ──► migrations/ (embed, boot-time)
        │
        └──► internal/repository/xui (M2: login, CRUD client, traffic) ──► X-UI Panel

Alur topup (M5, FR-06 — v1.48):

Telegram callback / teks FSM ──► dispatcher (routeTopup, /cancel)
        │  topup:menu → topup:amount:N → topup:confirm:N
        ▼
internal/service/topup (Quote fee §15.7, min/max; CreatePayment =
        persist row → create charge NET → confirm → checkoutUrl)
        │
        ├── internal/repository/kts (S2S HMAC signer + PG charge client:
        │       POST /api/v1/pg/charges → confirm → checkout QRIS)
        ├── internal/repository/postgres (PaymentRepo: payments/orders,
        │       MarkSettledTx conditional pending→terminal)
        ├── internal/repository/redis (TopupFSM: bot:fsm:topup:{id}, TTL 10 mnt;
        │       dedup webhook bot:webhook:{X-Webhook-Id})
        └── internal/service/user (Balance — saldo saat ini)

Alur settlement webhook (M5, v1.48) — POST /api/v1/webhooks/payments:

gateway ──pg.charge (X-Webhook-Signature: HMAC raw body, X-Webhook-Event,
        │  X-Webhook-Id)──► handler/http (payments_webhook.go)
        ▼  verify → branch event → dedup SETNX → ApplySettlement
internal/service/topup (kredit NET lokal + mark, SATU transaksi; 2xx hanya
        setelah durable — non-2xx → retry gateway, 013 §2)
        ├── internal/service/user (Credit atomik + ledger)
        └── cmd/bot/topup_notify.go (notif user + grup admin, best-effort)

Alur trial (M6 partial, FR-07):

Telegram callback / perintah /trial ──► dispatcher (routeTrial)
        │  trial:menu → trial:server:{id} → trial:inbound:{server}:{inbound}
        │    → trial:confirm:{server}:{inbound} (protocol dari panel, v1.24)
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

Alur riwayat (M7 partial, FR-14):

Telegram callback history:* ──► dispatcher (routeShop → handleHistory)
        │  history:menu → history:page:{n} → history:detail:{id}
        │  history:noop (indikator halaman — answer tanpa edit, FR-02 AC)
        ▼
internal/handler/telegram/history.go (list 5/halaman, clamp halaman di luar range)
        ├── internal/service/telegram (menu_history.go: teks list/detail + keyboard)
        └── internal/repository/postgres OrderRepo (CountByUser, ListByUserPage
                newest-first bounded, GetOwned — ownership guard: foreign =
                ErrOrderNotFound → "Transaksi tidak ditemukan")

Alur akun (M7 partial, FR-08 — pagination v1.30, hapus v1.31, traffic v1.32):

Telegram callback account:* ──► dispatcher (routeShop → handleAccount/accountList)
        │  account:menu → account:page:{n} (5/halaman, newest first)
        │  account:noop (indikator halaman — answer tanpa edit)
        │  item list: status teks Aktif/Hampir Habis/Expired + badge Trial +
        │    sisa waktu smart (v1.34, FR-08 AC-1)
        │  account:view:{id} → detail lengkap (kredensial protocol-aware: UUID
        │    vless/vmess, Password trojan/ss — v1.36; Limit IP + traffic
        │    terpakai/kuota AC-1 penuh v1.35; URL build TIDAK di detail ATAU
        │    view config ATAU pesan sukses Beli/Trial — cukup di Ekspor .txt
        │    v1.36) → account:config:{id} /
        │    account:convert:{id} (YAML Clash/Meta, v1.33 FR-08 AC-2) / account:export:{id}
        │  account:delete:{id} (konfirmasi) → account:delete_confirm:{id} (eksekusi;
        │    sukses → ordersvc.RecordDeletion: order type deletion → Riwayat FR-14,
        │    label "Hapus Akun" nominal "—" — v1.35 AC-4)
        │  account:traffic:{id} (v1.32): sync live dari panel (trafficsvc.RefreshClient
        │    — getClientTraffics/:email) → re-read DB → halaman usage (progress bar +
        │    status 🟢🟡🔴); tombol Refresh = callback yang sama; sync gagal → render
        │    data terakhir (best effort)
        ▼
internal/handler/telegram/accounts.go (clamp halaman, ownership guard di detail)
        ├── internal/service/telegram (AccountsText + AccountListKeyboard + pagerRow;
        │       menu_account_delete.go — konfirmasi/sukses hapus)
        ├── internal/service/server/delete.go (DeleteClient → panel delClient —
        │       PALING DULU; panel gagal → DB tidak dihapus)
        └── internal/repository/postgres ClientRepo (CountByUser, ListByUserPage,
                DeleteOwned — split client_repo_page.go §1.1; ListByUser = delegate page 1)

Alur bantuan (M7 partial, FR-15):

Telegram callback help:* ──► dispatcher (handleCallback → handleHelp)
        │  help:menu → help:order | help:topup | help:disclaimer | help:info
        │  help:disclaimer → help:tos:account | help:tos:payment
        ▼
internal/service/telegram (menu_help.go + menu_help_tos.go: konten statis
        id-ID + keyboard kembali/home — murni presentasi, tanpa seam)
        └── edit-in-place (editCB) — parity help_handler reference (FR-15 AC)

Alur subscription (M7 partial, FR-13 — v1.46):

ordersvc.Purchase / CreateTrial ──► serversvc.provisionClient
        │  (spec.SubID = UUID dikirim ke panel sejak awal; kini dikembalikan
        │   via PanelClient.SubID)
        ▼
ordersvc.SubLinks (value type, join kanonik — BaseURL/LinkPath/JSONPath dari
        config SUB_*; SetSubLinks di cmd/bot/shop.go; JSONPath dikosongkan bila
        SUB_JSON_ENABLED=false)
        ▼ persist migrasi 000006: vpn_clients.sub_id + subscription_url +
          subscription_json_url (akun lama kosong = legacy gap, tanpa backfill)
        ▼
account:export:{id} ──► AccountTXTContent (menu_account.go): blok
        "Subscription URL (auto-update)" + "Subscription JSON (Clash/Meta)"
        — URL HANYA di ekspor .txt (konsisten v1.36; chat/detail bersih)
        └── prasyarat ops: sub server panel aktif (subEnable/subPath/subJsonPath
            match config; Opsi 2 = domain panel + port beda, default 2096)

Alur admin (M6 partial → FR-11 lengkap v1.40):

Perintah /admin atau callback admin:* ──► dispatcher (routeAdmin; ADMIN_IDS)
        │   harga: admin:price → admin:plan:CODE:DAYS → setprice/toggle/reload
        │   broadcast: input teks → pratinjau → kirim (chunked 100/6 dtk, async)
        │   ban/unban: input ID → konfirmasi
        │   saldo (v1.39): admin:saldo → kredit/debit → FSM 2 langkah →
        │     admin:saldo:confirm:{kind}:{tgid}:{amount} (arm state, tap ganda aman)
        │   server (v1.40): admin:server → detail → toggle open/active
        │     (admin:server:open:{id} / admin:server:active:{id}); tambah =
        │     FSM 6 langkah srvadd:* → srvadd:confirm:* → AddServer (password
        │     disegel AES-256-GCM, tidak di-echo)
        │   stats (v1.40): admin:stats → OrderStats agregasi SQL + order terbaru
        │   audit (v1.40): admin:audit → AdminAuditLog 15 baris terbaru
        ▼
internal/service/admin (price ops, BanUser/UnbanUser dua layer, Broadcast,
        AdjustBalance, ServerOps/Stats/AuditStore seams, auditRecord di semua aksi)
        ├── internal/service/pricing (ListAll/Get/SetPrice/SetEnabled/Reload) ──► postgres pricing
        ├── internal/service/user (SetBanned, ListTelegramIDs, CountUsers, Credit/Debit) ──► postgres users
        ├── internal/service/server (ListAll/SetOpen/SetActive/AddServer — enkripsi kredensial)
        ├── internal/service/telegram BanService (marker bot:ban:{id}, TTL 1 thn)
        ├── internal/repository/redis (AdminFSM bot:fsm:admin:{id}, broadcast lock bot:admin:broadcast)
        ├── internal/repository/postgres (OrderRepo.Stats/RecentOrders, AuditRepo.Record/Recent,
        │        ServerRepo admin ops, ClientRepo.CountActive)
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

Catatan: M5 selesai v1.48 — kts client nyata menggantikan StubGateway;
menu/flow/FSM tidak berubah saat swap (PRD §15.7). Poll fallback dilimpahkan
ke gateway (reconciler 015 Phase 6.1 re-enqueue pg.notify); kts.GetCharge
siap utk poll manual/ops.
```

- `cmd/bot` = wiring saja, tanpa logika bisnis.
- `internal/handler/http` = boundary HTTP: webhook telegram (M3 real), health, webhook payments `pg.charge` (M5, v1.48).
- `internal/handler/telegram` = dispatcher + middleware chain (M3) + flow shop (M4) + flow topup (M5) + riwayat (M7, FR-14) + bantuan statis (M7, FR-15) + akun pagination & hapus (M7, FR-08).
- `internal/service/telegram` = webhook register, gate grup (cache 6 jam), ban, rate limit, menu (M3) + view bantuan statis (M7, FR-15) + **layout keyboard zigzag 2-1-2-1** (v1.42, `menu_rows.go`: `packRows` + `backBtn` — dipakai semua keyboard sub-menu) + **brand banner** (v1.43, `menu_brand.go`: `BrandName`/`BrandHeader` `🏪 KENTANG TECH` — dipakai 7 template notifikasi/pesan transaksi).
- `internal/service/topup` (M5, v1.48) = Quote fee §15.7 + `CreatePayment` (persist row → create charge NET → confirm → checkoutUrl) + `ApplySettlement` (kredit NET lokal + mark pending→terminal dalam satu transaksi conditional); seam `PaymentGateway` → `kts.Client` (`StubGateway` dihapus).
- `internal/service/trial` (M6) = FR-07 policy: daily limit + claim anti-race + TTL s.d. tengah malam.
- `internal/service/expiry` (M6) = FR-09 policy: jendela ambang H-7/H-3/H-1, kirim + tandai sekali per ambang.
- `internal/service/admin` (M6 partial, v1.39) = FR-11 ops: price/toggle/reload, ban/unban dua layer, broadcast chunked (lock + timeout + recover) + **AdjustBalance** (kredit/debit manual — resolve tgID→PK, jalur atomik + ledger `ADJ-<random>` yang SAMA dengan order).
- `internal/service/traffic` (M6) = PRD §16.2 policy: kandidat aktif group per server → GetInbounds + GetOnlineClients → batch update; server gagal → lanjut.
- `internal/job` (M6/M7) = **`IntervalWorker` generik** (loop stdlib ticker, panic-recover, terminasi ctx) — dipakai notifikasi, sync traffic, health check & trial cleanup (v1.45).
- `internal/service/order` (M4/M6/M7) = Purchase/Renew/CreateTrial: state machine, panel provisioning, debit atomik + ledger (trial tanpa debit) + `RecordDeletion` (FR-08 AC-4: order type `deletion` → Riwayat FR-14, v1.35) + **idempotence `OrderRepo.FindInFlight`** (`ErrOrderInFlight` — duplikat pending/processing ditolak, v1.37) + **Renew paid-only** (`ErrTrialNotRenewable`; handler `renewableClients`) + **alur uang debit-first + auto-refund** (renew `renew.go` v1.37; **purchase `purchase.go` v1.47** — prepare client → insert row → debit atomik → commit panel, gagal commit → refund + hapus row; akun aktif tanpa bayar mustahil) + `SubLinks` (FR-13 v1.46: persist `sub_id` + subscription URL/JSON saat order).
- `internal/service/pricing` (M4) = seed JSON → pricing DB; harga live untuk order.
- `internal/service/server` (M4/M7) = seed panel terenkripsi, PickForCountry, gateway XUI + DeleteClient (FR-08 AC-4) + RenewClient (FR-05, v1.38: spec penuh client dari settings panel dipertahankan — kuota/ipLimit/flow; hanya enable+expiryTime diubah; cari client by email di settings JSON) + DisableClients (v1.45: batch disable by email — satu GetInbounds per panel, patch enable=false, kunci updateClient per-protocol dari spec) + provisionClient mengembalikan `SubID` (FR-13 v1.46).
- `internal/service/traffic` (M6/M7) = sweep batch worker (PRD §16.2) + RefreshClient per akun (FR-08 AC-3) — SATU instance dipakai keduanya (buildShop), getClientTraffics/:email verified protocol-agnostic.
- `internal/service/health` (M7, v1.45) = PRD §17 health check: RunOnce ping tiap panel aktif (GET /xui/API/server/status, per-server timeout XUI_API_TIMEOUT) → UpdateHealth (ok/down); satu panel gagal → lanjut; ListBuyable mengecualikan health_status=down.
- `internal/service/trialcleanup` (M7, v1.45) = PRD worker: disable akun trial expired di panel (group per server → serversvc.DisableClients) lalu tandai is_expired; panel gagal → tidak ditandai.
- `internal/service/user` (M4) = ensure user, balance, debit/kredit atomik + ledger (dipakai topup utk cek saldo).
- `internal/repository/telegram` = wrapper typed go-telegram/bot (M3).
- `internal/repository/postgres` = GORM + pool limits + runner migration; repos user/pricing/server/client/order/payment (M4; order juga: CountByUser/ListByUserPage/GetOwned FR-14; client juga: CountByUser/ListByUserPage FR-08 AC-1; payment: `PaymentRepo` M5 v1.48 — create/get/save-provider-ref/mark-failed/`MarkSettledTx` conditional).
- `internal/repository/redis` = go-redis + ops (dedup, gate cache, rate limit, per-user lock, `TopupFSM` key, `AdminFSM` key, broadcast lock, payment webhook dedup `bot:webhook:{X-Webhook-Id}`).
- `internal/repository/kts` (M5, v1.48) = client PG Aggregate: S2S signer (canonical `v1\n...\nhex_sha256(body)` + `X-Signature`) + `CreateCharge`/`ConfirmCharge`/`GetCharge` + `WebhookSignature` (HMAC raw body) — kontrak 013/015.
- `internal/repository/xui` = REST client panel (login + session cache Redis + CRUD client + traffic).
- `internal/crypto` = AES-256-GCM secret box (kredensial panel server).
- `internal/domain` (M4) = Money (int64 + Scanner/Valuer), Order state machine, VpnPlan, VPNClient, random ID + `PanelClientKey` (v1.38: kunci panel per-protocol — vless/vmess→UUID, trojan/hysteria→password, ss→email; dipakai renew & delete) + `PreparedClient` (v1.47: record bot-side + param commit panel — prepare/commit split).
- `migrations/` = SQL up/down golang-migrate (000001 init, 000002 insecure_tls, 000003 expiry_notify_day, 000004 inbound stream/path, 000005 admin_audit_log, 000006 sub_id/subscription_url, 000007 payments.telegram_id), di-embed & diterapkan saat boot.
- Service/domain/schema mengikuti aturan layer yang sama (AGENTS.md §1.5).

## Payment Gateway (M5 selesai, v1.48 — PG Aggregate)

- Bot bergantung pada interface `topupsvc.PaymentGateway` (`CreatePayment`),
  diimplementasikan `kts.Client` (PG Aggregate, kontrak 015) — menu/flow/FSM
  tidak berubah sejak v1.13 (StubGateway → kts.Client tanpa rewrite).
- S2S outbound: `X-API-Key`/`X-Timestamp` (±300 s)/`X-Nonce`/`Idempotency-Key`
  + `X-Signature: sha256=hex(hmac_sha256(secret, canonical))` (001 §2.3).
  Webhook inbound: `X-Webhook-Signature` (HMAC raw body) + `X-Webhook-Event:
  pg.charge` + dedup `X-Webhook-Id` (013 §2) — secretKey merchant SAMA untuk
  keduanya (`KTS_SECRET`).
- Amount = **NET**; gross-up (2,5% MDR + 11% PPN) di-handle gateway; kredit
  selalu NET dari order lokal. Settlement = kredit + mark `pending→terminal`
  dalam satu transaksi conditional (anti double-credit webhook/poll race).
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
| stdlib `time.Ticker`           | Scheduler worker interval — notifikasi (FR-09), sync traffic (§16.2), health check (§17) & trial cleanup; robfig/cron tidak dipakai: preferensi stdlib AGENTS.md |
| stdlib `crypto/aes`            | Enkripsi AES-256-GCM kredensial server (M2) |
| stdlib `crypto/hmac` + `crypto/sha256` | S2S signer + verifikasi webhook PG (M5, v1.48) — kontrak 013/015 |

## Layanan infra (docker-compose)

`bot` (build ./) · `nginx` (TLS) — **PostgreSQL & Redis = service native
HOST** (`postgresql@16-main` :5432, `redis-server` :6379; tidak ada container
DB). Karena PG/Redis host hanya bind loopback, kedua service memakai
`network_mode: host`; `DATABASE_URL`/`REDIS_URL` → `127.0.0.1`, proxy nginx →
`127.0.0.1:8443`. Env dari `.env` (PRD §19.2).

## Status milestone

| M   | Isi                                   | Status |
|-----|---------------------------------------|--------|
| M0  | Scaffolding + `/api/v1` convention    | ✅     |
| M1  | Config, PG+Redis, migration, /health  | ✅     |
| M1+ | Integration test migration & Redis (PG 16 + Redis 7 di host staging) | ✅     |
| M2  | X-UI client (login, CRUD client)      | ✅     |
| M3  | Webhook go-telegram/bot + dispatcher  | ✅     |
| M4  | Order flow + ledger                   | ✅     |
| M5  | Topup menu/flow + PG Aggregate charge + webhook pg.charge | ✅ (v1.48: flow v1.13 + API v1.48) |
| M6  | Trial, notifikasi, sync traffic, admin | ✅ (v1.21) |
| M7  | Hardening, test, UAT                  | 🔶 (v1.22: coverage gap ✅, load test ✅, UAT checklist ✅; v1.28: Riwayat FR-14 ✅; v1.29: Bantuan FR-15 ✅; v1.30: pagination Akun FR-08 ✅; v1.31: hapus akun FR-08 AC-4 ✅; v1.32: traffic + refresh manual FR-08 AC-3 ✅; v1.33: convert YAML Clash/Meta FR-08 AC-2 ✅; v1.34: status display list Akun FR-08 AC-1 ✅; v1.35: detail akun AC-1 lengkap ✅ + hapus tercatat di Riwayat AC-4 ✅; v1.36: revisi minor UI akun ✅; v1.37: renew paid-only + idempotence ✅; v1.38: fix renew panel spec client ✅; v1.39: adjust saldo + idempotence ✅; v1.40: FR-11 lengkap ✅; v1.41: notifikasi order ke grup admin ✅; v1.42: UX keyboard zigzag 2-1-2-1 semua sub-menu ✅; v1.43: brand KENTANG TECH ✅; v1.44: konsistensi brand ✅; v1.45: worker health check (PRD §17) + worker trial cleanup ✅; v1.46: FR-13 subscription URL ✅; v1.47: purchase debit-first + auto-refund ✅; v1.48: M5 PG Aggregate charge + webhook pg.charge ✅) |
