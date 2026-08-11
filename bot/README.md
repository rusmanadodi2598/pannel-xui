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
| M6 | Trial, notifikasi, multi-server, admin    | ⬜     |
| M7 | Hardening, test, UAT                      | ⬜     |
