# Bot Auto-Order Telegram (Go)

Bot Telegram auto-order untuk X-UI Panel, dibangun sebagai **modul Go mandiri**
di `/bot` — tidak menyentuh source, database, maupun proses panel x-ui.
Komunikasi ke panel hanya lewat REST API. Mode Telegram **WAJIB Webhook**
(tanpa polling). Reverse proxy: **Nginx** (bukan Caddy).

- **PRD**: [`docs/001-PRD-BOT-ORDER.md`](../docs/001-PRD-BOT-ORDER.md)
- **Governance**: [`AGENTS.md`](../AGENTS.md) — semua kode di sini wajib patuh.
- **Referensi**: bot Python `kentangtech-xui/client-vpn`.
- **Staging (2026-08-11)**: bot `@kentangtechidcloudhost_bot` · domain
  `bot-xui.kentangtechstore.com` · VPS `46.250.232.48` (`vmi3491075`).
  Kredensial di `bot/.env` (**gitignored** — jangan commit).

## Struktur

```
bot/
├── cmd/bot/main.go              # Composition root: config → DB/Redis → HTTP
├── internal/config/             # Typed env config, fail-fast (PRD §19.2)
├── internal/handler/http/       # /api/v1/* (health, webhook telegram real M3, stub payments)
├── internal/handler/telegram/   # Dispatcher: ban→gate→rate-limit→route (FR-01/FR-02)
├── internal/crypto/             # AES-256-GCM secret box (kredensial panel)
├── internal/service/telegram/   # Webhook register, gate grup, ban, rate limit, menu (M3)
├── internal/repository/
│   ├── postgres/                # GORM + pool limits + migration runner (embed)
│   ├── redis/                   # go-redis client + ops (dedup, lock, rate limit, cache)
│   ├── telegram/                # Wrapper go-telegram/bot (M3): webhook, messaging, membership
│   └── xui/                     # X-UI panel REST client (M2): login, session cache, CRUD client, traffic
├── migrations/                  # SQL up/down (golang-migrate, embedded)
├── .env.example                 # Template environment
├── Dockerfile                   # Multi-stage (alpine + CA certs)
├── nginx.conf                   # Reverse proxy TLS (template envsubst)
├── docker-compose.yml           # bot + nginx + postgres + redis
└── README.md
```

## Prasyarat

- Go 1.26+ (`go 1.26.5` sesuai `AGENTS.md`)
- PostgreSQL & Redis (atau `docker compose`)

## Menjalankan lokal (dev)

```bash
cd bot
cp .env.example .env          # isi BOT_TOKEN, WEBHOOK_SECRET, dsb.
go build ./... && go vet ./...
go test -race ./...           # butuh PG (bot_test) + Redis local, lihat bawah
go run ./cmd/bot              # konek DB/Redis + migrate + listen :8443
curl http://127.0.0.1:8443/api/v1/health
```

Saat boot bot **wajib** konek PostgreSQL & Redis (fail-fast bila mati),
menjalankan migration embedded (golang-migrate), **mendaftarkan webhook
Telegram** (`setWebhook` + verifikasi `getWebhookInfo`; gagal = boot gagal),
lalu melayani `GET /api/v1/health` (status `db`/`redis`/`webhook` — 200 `ok`
/ 503 `degraded`). `POST /api/v1/webhooks/telegram` aktif sejak **M3**
(dedup + worker pool); `POST /api/v1/webhooks/payments` masih stub sampai
**M5**.

### Test integration (PostgreSQL + Redis lokal)

Test repository (`internal/repository/postgres`, `internal/repository/redis`)
adalah **integration test** terhadap service lokal (bukan mock):

- PostgreSQL: butuh DB `bot_test` (user `bot`) di `127.0.0.1:5432`;
  override via env `TEST_DATABASE_URL`.
- Redis: butuh Redis di `127.0.0.1:6379`, DB `15` khusus test;
  override via env `TEST_REDIS_URL`.

Setup satu kali di host (Ubuntu/Debian):

```bash
# PostgreSQL
sudo pg_createcluster 16 main --start   # bila belum ada cluster
sudo -u postgres psql -c "CREATE USER bot WITH PASSWORD 'bot';"
sudo -u postgres psql -c 'CREATE DATABASE bot OWNER bot;'
sudo -u postgres psql -c 'CREATE DATABASE bot_test OWNER bot;'

# Redis
sudo apt-get install -y redis-server
sudo systemctl enable --now redis-server
```

Migration test memverifikasi: up → 7 tabel (PRD §13) + kolom + index +
UNIQUE constraint, down → semua tabel ter-drop, rerun → idempoten.

### REST API Convention
Seluruh endpoint bot memakai **`/api/v1`** dengan penamaan resource jamak &
readable (PRD §26):

| Method | Path                                        | Status | Milestone |
|--------|---------------------------------------------|--------|-----------|
| GET    | `/health` (alias infra)                     | ✅     | M0        |
| GET    | `/api/v1/health` (cek DB+Redis)             | ✅     | M1        |
| POST   | `/api/v1/webhooks/telegram`                 | ✅     | M3        |
| POST   | `/api/v1/webhooks/payments`                 | stub   | M5        |

## X-UI Client (M2)

`internal/repository/xui` — REST client panel yang kontraknya **diverifikasi dari
source panel ini** (`web/controller/api.go`, `web/service/inbound.go`), bukan
dari reference Python. Fakta kunci yang berbeda dari reference:

- Cookie session: **`x-ui`** (gin-contrib sessions) — bukan `session`.
- Envelope: `{success, msg, obj}`; HTTP 200 utk bisnis, **401** saat session
  invalid (auto-relogin sekali + retry).
- `getClientTrafficsById` → **array**; `addTrialClient` **tidak ada** di fork ini.
- Base path panel dinamis (`GetBasePath()`) — didukung via `APIPath`.
- Session cache Redis `xui:session:{serverID}` (TTL 1 jam).

Kredensial server dienkripsi AES-256-GCM (`internal/crypto`, `ENCRYPTION_KEY`).

## Webhook Telegram (M3)

Core webhook `github.com/go-telegram/bot` v1.23.0 — **tanpa polling**:

- `service/telegram/webhook.go` — `setWebhook` saat boot (secret token,
  `allowed_updates=[message, callback_query]`, `drop_pending_updates=true`,
  `max_connections=40`) + verifikasi `getWebhookInfo`; boot **fail-fast** bila
  Telegram tidak bisa dijangkau.
- `handler/http` — `POST /api/v1/webhooks/telegram`: secret constant-time →
  parse update → **dedup `update_id`** (Redis SETNX 24 jam) → enqueue worker
  pool (bounded) → balas 200 secepatnya.
- `handler/telegram` — dispatcher chain **ban → gate grup (cache 6 jam) →
  rate limit (30/menit sliding window) → route**; `/start` → menu FR-02;
  callback `menu:*` → **edit in-place**; `gate:check` → re-check membership;
  tombol non-aksi → answer noop.
- Per-user serialization: lock Redis `bot:lock:user:{id}` (TTL 30 s) diambil
  di worker dan **di-release setelah handle** (TTL = crash guard).

Env baru: `WEBHOOK_MAX_CONNECTIONS`, `WEBHOOK_DROP_PENDING`,
`WEBHOOK_WORKERS`, `WEBHOOK_QUEUE_BUFFER`.

## Topup Flow (M5 partial)

- `topup:menu` → quick-pick 6 nominal (`10k`–`500k`) + "Nominal Lain"
- `topup:amount:N` → ringkasan fee §15.7 → `topup:confirm:N`
- `topup:custom` → FSM input teks (Redis `bot:fsm:topup:{id}`, TTL 10 mnt) — `/cancel` atau tombol batal
- `topup:confirm:N` → `PaymentGateway.CreatePayment` — sekarang `StubGateway` (API KentangTech rewrite); teks unavailable ditampilkan, flow tetap utuh
- Quote selalu dihitung server-side (callback payload tidak dipercaya)

## Order Flow (M4)

Auto-order end-to-end: **pricing DB + seed → beli → fulfillment → ledger → renewal**.

> **UI copy policy**: semua **teks pesan tanpa emoji** (clean & modern — separator
> `━━━` dan bullet `•` dipakai sebagai elemen non-emoji); **icon hanya pada
> tombol navigasi** (menu utama, join grup, konfirmasi, kembali, flag negara).

- **Pricing** — 12 paket (ID/SG/JP/CN × 15/30/90 hari) di-seed idempotent dari
  `seed/pricing.json` (`PRICING_SEED_FILE`) ke tabel `pricing` saat boot;
  harga beli **selalu live dari DB** (FR-03 AC).
- **Multi-panel dinamis (FR-10)** — blok env `PANEL_N_*`
  (`HOST/PORT/USERNAME/PASSWORD/API_PATH/USE_SSL/INSECURE/COUNTRY_CODE/FLAG_EMOJI/LOCATION/PROTOCOLS`),
  password dienkripsi AES-256-GCM saat seed (`vpn_servers.password_enc`);
  `insecure_tls` per-panel (default `false` — hanya aktifkan untuk panel
  staging self-signed).
- **Beli (FR-03/FR-04)** — `buy:menu` → pilih negara (hanya negara yang punya
  server open) → pilih paket → ringkasan + cek saldo → konfirmasi →
  `ordersvc.Purchase`: order `pending→processing` → `addClient` di panel →
  **client row dibuat → debit saldo atomik + ledger** → `completed`; gagal di
  panel → `failed`, saldo **tidak** dipotong; order ID `KTS-XXXXXXXX-VPN`.
- **Renewal (FR-05)** — `renew:menu` → pilih akun → pilih paket → konfirmasi →
  `updateClient` (expiry diperpanjang dari **sisa waktu**, tidak double-count)
  → debit → `completed`; pesan sukses menampilkan expiry asli.
- **Akun (FR-08 subset)** — `account:menu` list akun (status, sisa hari,
  server flag); detail/config link menyusul di M6.
- **Ledger** — `balance_transactions` immutable; debit/kredit atomik via
  `UPDATE users SET balance = balance ± x WHERE balance >= x` dalam satu tx
  (`ErrInsufficientBalance` untuk saldo kurang/banned).

Env baru M4: `PRICING_SEED_FILE`, `PANEL_1_*` … `PANEL_N_*`
(`PANEL_N_INSECURE=true` untuk panel self-signed).

## Trial Flow (M6 partial, FR-07)

- `trial:menu` (tombol 🎁 Trial di menu utama atau perintah `/trial`) →
  **daily limit di-re-check** → pilih server (hanya server buyable) →
  `trial:confirm:{id}` → **claim atomik** (anti-race, maks 2x/hari via Redis
  counter `bot:trial:{userID}` TTL s.d. tengah malam) → `CreateTrial`
  (order type `trial`, **tanpa debit**) → akun 1 jam / 1 GB / 1 IP
  (`is_trial=true`) → pesan sukses + tombol "Beli VPN Premium".
- Limit di-re-check di **menu, pilih server, DAN confirm** (PRD FR-07 AC-1);
  claim yang melebihi limit di-rollback otomatis.
- Env: `TRIAL_ENABLED` (false = fitur nonaktif), `TRIAL_DAILY_LIMIT` (2),
  `TRIAL_DURATION_HOURS` (1), `TRIAL_TRAFFIC_GB` (1), `TRIAL_IP_LIMIT` (1).

## Notifikasi Kadaluarsa (M6 partial, FR-09)

Worker interval memindai akun yang hampir habis dan mengirim pengingat
**H-7 / H-3 / H-1** (dapat dikonfigurasi via `EXPIRY_NOTIFY_DAYS`):

- **Jendela eksklusif** `(lower, upper]` + guard `notified_expiry != ambang` →
  **sekali per ambang** (AC FR-09); `notified_expiry` kini integer
  (migrasi `000003`: 0 = belum, N = ambang terakhir terkirim).
- Gagal kirim → **tidak** ditandai → retry sweep berikutnya; renewal
  (`UpdateExpiry`) mereset flag → siklus notifikasi dimulai ulang.
- Akun trial (1 jam) dan akun nonaktif/expired **tidak** di-notifikasi.
- `internal/job/interval.go` — **`IntervalWorker` generik** (dipakai notifikasi
  & traffic sync, bukan duplikasi loop): stdlib `time.Ticker` (bukan
  robfig/cron; AGENTS.md prefer stdlib), sweep pertama langsung saat boot, lalu
  tiap interval. Setiap sweep ber-timeout dan panic-recovered (AGENTS.md §1.6);
  loop berhenti saat shutdown (ctx cancel + WaitGroup drain). Tanggal di pesan
  diformat sesuai `TIME_LOCATION`.

Env: `EXPIRY_NOTIFY_DAYS` (7,3,1), `EXPIRY_NOTIFY_ENABLED` (true),
`EXPIRY_NOTIFY_INTERVAL_MIN` (360), `EXPIRY_NOTIFY_BATCH` (50 — maks akun per
ambang per sweep).

## Sync Traffic (M6, PRD §16.2)

Worker interval menyinkronkan pemakaian kuota dari panel X-UI ke `vpn_clients`
(`traffic_used/up/down`, `last_online`, `last_sync`) — data tampilan "Akun
Saya" selalu segar tanpa N+1:

- `service/traffic` mengambil kandidat aktif (client `is_active` & tidak
  expired di server `is_active`) lalu **group per server**: `GetInbounds`
  sekali per panel (field `clientStats` membawa traffic semua client — sumber
  yang sama dengan `getClientTrafficsById`) + `GetOnlineClients` untuk
  `last_online`.
- `SyncTrafficBatch` menulis **satu statement** `UPDATE ... FROM (VALUES ...)`
  (anti N+1 §1.7); `last_online = COALESCE(...)` — client offline
  mempertahankan timestamp lama. Client yang sudah dihapus dari panel di-skip.
- **Satu panel gagal tidak menggagalkan sweep** — log + lanjut server lain
  (PRD §16.2); per-server timeout `XUI_API_TIMEOUT`.
- Kandidat diurutkan `last_sync ASC NULLS FIRST` (fair round-robin bila batch
  lebih kecil dari jumlah client).

Env: `TRAFFIC_SYNC_ENABLED` (true), `TRAFFIC_SYNC_INTERVAL_MIN` (5 — 1-60),
`TRAFFIC_SYNC_BATCH` (200).

## Panel Admin (M6 partial, FR-11)

`/admin` + callback `admin:*` — **hanya `ADMIN_IDS`** (di-re-check di setiap
surface, AC FR-11). Input bebas (harga, broadcast, ID user) memakai FSM admin
(`bot:fsm:admin:{id}`, TTL 10 mnt) — pola sama dengan nominal custom topup;
`/cancel`, `/start`, dan tombol batal membersihkan FSM.

- **Harga** — `admin:price` → daftar semua paket (termasuk nonaktif, bertanda
  🚫) → detail paket → **Ubah Harga** (input angka rupiah) / **Toggle Status**
  (aktif/nonaktif, harga live langsung berubah) / **Reload Seed** (re-import
  `PRICING_SEED_FILE`).
- **Broadcast** — input pesan → pratinjau → konfirmasi → pengiriman chunked
  **100 pesan / 6 detik** di goroutine terbound (timeout 15 mnt, panic-recover,
  lock Redis `bot:admin:broadcast` anti-double) → laporan hasil ke admin.
- **Ban / Unban** — input Telegram ID → konfirmasi → **dua layer**: marker
  gate Redis `bot:ban:{id}` (efek seketika) + flag persisten
  `users.is_banned` (tahan Redis flush; guard debit SQL juga memblokir).
- FSM di-clear setelah selesai/batal; input invalid → re-prompt.

Belum di M6 (FR-11): manajemen server, statistik/audit log, adjust saldo.

## Menjalankan dengan Docker Compose (produksi)

```bash
cd bot
cp .env.example .env          # set BOT_DOMAIN + BOT_TOKEN + secret
mkdir -p certs                # siapkan sertifikat TLS (lihat di bawah)
docker compose up -d --build
docker compose ps
curl https://<BOT_DOMAIN>/health
```

`nginx` menerima HTTPS di `443` dan meneruskan ke `bot:8443`:
`https://<BOT_DOMAIN>/api/v1/webhooks/telegram → http://bot:8443/api/v1/webhooks/telegram`.

### TLS (Nginx) — syarat webhook Telegram

1. Terbitkan sertifikat dengan **certbot** (Let's Encrypt) di host
   (pastikan port 80/443 bebas saat issuance):
   ```bash
   sudo certbot certonly --standalone -d bot-xui.kentangtechstore.com
   ```
2. Letakkan di folder `./certs`:
   ```bash
   mkdir -p certs
   sudo cp /etc/letsencrypt/live/bot-xui.kentangtechstore.com/{fullchain.pem,privkey.pem} certs/
   sudo chown -R "$USER" certs && chmod 600 certs/privkey.pem
   ```
3. Alternatif: mount langsung direktori Let's Encrypt
   (lihat komentar di `docker-compose.yml`) dan sesuaikan
   `ssl_certificate*` di `nginx.conf`.
4. Renewal otomatis: `sudo certbot renew` via cron/systemd timer,
   lalu `sudo docker compose exec nginx nginx -s reload`.

> Webhook Telegram hanya menerima HTTPS. Untuk pengujian lokal tanpa
> sertifikat, komentari blok `server 443` di `nginx.conf` dan ganti
> redirect `301` dengan blok `location` proxy (lihat komentar di file).

## Status DNS (webhook Telegram)

`bot-xui.kentangtechstore.com` masih menunjuk ke IP Cloudflare (proxied).
Sebelum webhook aktif (M3), ubah di Cloudflare Dashboard → DNS:

```
Type: A · Name: bot-xui · IPv4: 46.250.232.48 · Proxy: OFF (grey cloud)
```

## Security & Git Hooks

Repo ini menerapkan lapisan keamanan developer-standard (AGENTS.md §1.4, PRD §15):

| Layer | Alat | Lokasi | Fungsi |
|-------|------|--------|--------|
| Secret scan lokal | **gitleaks** | `gitleaks.toml` | `pre-commit` tolak commit ber-secret (`protect --staged`); `pre-push` scan commit baru |
| Format gate | gofmt | `.githooks/pre-commit` | tolak file `.go` yang tidak ter-format |
| Static analysis | **golangci-lint** (opsional lokal, wajib CI) | `.golangci.yml` | `pre-commit` lint baris baru di `bot/` (`--new-from-patch`); `pre-push` lint penuh `bot/`; CI enforce penuh |
| Commit convention | Conventional Commits | `.githooks/commit-msg` | `feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert` |
| Pre-push build | `go build`/`vet` | `.githooks/pre-push` | kedua modul (root x-ui + `bot/`) |
| CI | gitleaks-action + Go gates | `.github/workflows/security.yml` | push/PR: secret scan + build/vet/test -race (PG+Redis service) |

**Aktivasi sekali per clone:**

```bash
bash scripts/install-githooks.sh   # verifikasi gitleaks + golangci-lint + chmod +x + core.hooksPath
```

**golangci-lint** — linter set: `errcheck, gosimple, govet, ineffassign,
staticcheck, unused, misspell, bodyclose` (selaras AGENTS.md §1.4/§1.6).
Install opsional (PIN v1.64.x — config format v1):
`go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8`
(config `.golangci.yml` di root). Pre-commit hanya memeriksa **baris yang
diubah** di modul `bot/` (`--new-from-patch` + `--relative=bot`); pre-push
lint penuh `bot/`; CI (`job golangci`) enforce penuh. Modul root x-ui
(upstream legacy) tidak di-lint di gate mana pun — rule `misspell/gosimple`
pada dir legacy di `.golangci.yml` hanya untuk lint manual.

**Catatan gaya header**: kontinuasi header AGENTS.md memakai `// teks`
(1 spasi) — bukan 12 spasi — karena gofmt me-reflow style lama (`//\n//\t`).

**Kebijakan secret (wajib):**

- Kredensial asli HANYA di `bot/.env` — file ini gitignored (`git check-ignore` terverifikasi).
- `.env.example` hanya berisi placeholder — jangan pernah isi nilai asli di sana.
- Allowlist gitleaks (`gitleaks.toml`) berisi **hanya false positive terverifikasi**
  (nama ikon di source map vendor `antd.min.js.map` + identifier JS panel
  `getNewX25519.obj.privateKey` per-commit + fixture dummy file test Go
  `bot/*_test.go`: `testSecret` 32-hex & `trojan-pass-123` — per-regex, bukan
  per-path). Tambahan allowlist wajib disertai justifikasi — commit yang
  mem-bypass scan secret dianggap insiden keamanan.
- `git commit --no-verify` hanya untuk kasus non-secret (mis. bypass commit-msg
  sementara); secret tetap tertangkap di CI.
- Rotasi kredensial segera bila secret pernah ter-commit ke riwayat.

## Milestone

| M | Isi                                       | Status |
|---|-------------------------------------------|--------|
| M0 | Scaffolding (dokumen ini)                 | ✅     |
| M1 | Config+DB+Redis+migration+/health penuh   | ✅     |
| M2 | X-UI client (login, CRUD client, traffic) | ✅     |
| M3 | Core webhook go-telegram/bot              | ✅     |
| M4 | Order flow (beli, renewal, ledger)        | ✅     |
| M5 | Topup menu/flow + QRIS + webhook HMAC    | 🔶     |

> **M5 status (v1.13)**: menu & flow top up (FR-06) **aktif** — quick-pick,
> nominal custom (FSM), ringkasan fee §15.7, `/cancel`. Call ke KentangTech
> payment API **di-defer**: bot hanya bergantung pada seam `PaymentGateway`
> (`internal/service/topup`), sekarang `StubGateway` — saat API Go final tinggal
> swap client, tanpa rewrite ulang. Env baru: `MIN_TOPUP_AMOUNT`,
> `MAX_TOPUP_AMOUNT`, `QRIS_FEE_PERCENT`, `QRIS_PPN_PERCENT`,
> `QRIS_EXPIRY_MINUTES` (default ada di `config`/`.env.example`).
| M6 | Trial, notifikasi, sync traffic, admin     | ✅ (v1.21) |
| M7 | Hardening, test, UAT                      | 🔶 (v1.22: coverage gap ✅, load test ✅, UAT checklist ✅) |

> **M6 status (v1.21)**: **Trial (FR-07) ✅** — `service/trial` (daily limit
> 2x/hari via Redis counter TTL s.d. tengah malam, claim anti-race + rollback),
> flow `trial:menu` → `trial:server:{id}` → `trial:confirm:{id}` (+ `/trial`),
> akun trial 1 jam / 1 GB / 1 IP (`is_trial=true`, **tanpa debit**), tombol
> "Beli VPN Premium" setelah sukses. **Notifikasi kadaluarsa (FR-09) ✅** —
> worker H-7/H-3/H-1 (section di bawah). **Admin (FR-11) ✅** — harga,
> toggle plan, broadcast, ban/unban (section di bawah). **Sync traffic
> (PRD §16.2) ✅** — worker interval sinkron kuota dari panel (section di
> bawah). Env baru: `TRIAL_ENABLED`, `TRIAL_DAILY_LIMIT`, `TRIAL_DURATION_HOURS`,
> `TRIAL_TRAFFIC_GB`, `TRIAL_IP_LIMIT`, `EXPIRY_NOTIFY_ENABLED`,
> `EXPIRY_NOTIFY_INTERVAL_MIN`, `EXPIRY_NOTIFY_BATCH`, `TRAFFIC_SYNC_ENABLED`,
> `TRAFFIC_SYNC_INTERVAL_MIN`, `TRAFFIC_SYNC_BATCH`.
