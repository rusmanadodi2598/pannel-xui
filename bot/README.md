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
├── internal/handler/http/       # /api/v1/* (health, webhook telegram M3, webhook payments pg.charge M5)
├── internal/handler/telegram/   # Dispatcher: ban→gate→rate-limit→route (FR-01/FR-02)
├── internal/crypto/             # AES-256-GCM secret box (kredensial panel)
├── internal/service/telegram/   # Webhook register, gate grup, ban, rate limit, menu (M3)
├── internal/repository/
│   ├── postgres/                # GORM + pool limits + migration runner (embed); PaymentRepo (M5)
│   ├── redis/                   # go-redis client + ops (dedup, lock, rate limit, cache)
│   ├── telegram/                # Wrapper go-telegram/bot (M3): webhook, messaging, membership
│   ├── kts/                     # KentangTech PG Aggregate client (M5): S2S signer + charge create/confirm/verify
│   └── xui/                     # X-UI panel REST client (M2): login, session cache, CRUD client, traffic
├── migrations/                  # SQL up/down (golang-migrate, embedded)
├── .env.example                 # Template environment
├── Dockerfile                   # Multi-stage (alpine + CA certs)
├── nginx.conf                   # Reverse proxy TLS (template envsubst)
├── docker-compose.yml           # bot + nginx (DB/Redis: service HOST)
└── README.md
```

## Prasyarat

- Go 1.26+ (`go 1.26.5` sesuai `AGENTS.md`)
- PostgreSQL & Redis **native di host** (systemd `postgresql@16-main` di
  `127.0.0.1:5432` + `redis-server` di `127.0.0.1:6379`) — compose TIDAK
  membundel DB/Redis (lihat section Docker Compose)

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
(dedup + worker pool); `POST /api/v1/webhooks/payments` aktif sejak **M5
(v1.48)** — verifikasi `X-Webhook-Signature` + settle charge `pg.charge`.

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

Migration test memverifikasi: up → 8 tabel (PRD §13 — users, orders,
vpn_clients, vpn_servers, pricing, balance_transactions, payments,
admin_audit_log) + kolom + index + UNIQUE constraint, down → semua tabel
ter-drop, rerun → idempoten.

### REST API Convention
Seluruh endpoint bot memakai **`/api/v1`** dengan penamaan resource jamak &
readable (PRD §26):

| Method | Path                                        | Status | Milestone |
|--------|---------------------------------------------|--------|-----------|
| GET    | `/health` (alias infra)                     | ✅     | M0        |
| GET    | `/api/v1/health` (cek DB+Redis)             | ✅     | M1        |
| POST   | `/api/v1/webhooks/telegram`                 | ✅     | M3        |
| POST   | `/api/v1/webhooks/payments`                 | ✅     | M5 (v1.48) |

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

## Topup Flow & Payment (M5, v1.48)

- `topup:menu` → quick-pick 6 nominal (`10k`–`500k`) + "Nominal Lain"
- `topup:amount:N` → ringkasan fee §15.7 → `topup:confirm:N`
- `topup:custom` → FSM input teks (Redis `bot:fsm:topup:{id}`, TTL 10 mnt) — `/cancel` atau tombol batal
- `topup:confirm:N` → `topupsvc.CreatePayment` → **PG Aggregate**
  (`internal/repository/kts`): persist order topup (orderId `tp_*`, row
  `orders` + `payments`) → `POST /api/v1/pg/charges` (amount = **NET**) →
  `POST /api/v1/pg/charges/{orderId}/confirm` → tampilkan **checkout QRIS**
  (link checkout + caption). Gagal → order ditandai `failed` bersih.
- **S2S outbound** — chain HMAC: `X-API-Key` + `X-Timestamp` (±300 s) +
  `X-Nonce` (UUIDv4) + `Idempotency-Key` + `X-Signature:
  sha256=hex(hmac_sha256(secret, canonical))` (canonical 001 §2.3).
- **Webhook settlement** — `POST /api/v1/webhooks/payments`: verifikasi
  `X-Webhook-Signature` (HMAC raw body, constant-time) → 403; branch
  `X-Webhook-Event: pg.charge`; dedup `X-Webhook-Id` (Redis SETNX, TTL 7 hari);
  `ApplySettlement` kredit **NET dari order lokal** + mark `pending→terminal`
  dalam satu transaksi conditional; **2xx hanya setelah durable** — non-2xx →
  retry gateway (013 §2). `succeeded`→kredit, `failed`/`expired`→tanpa
  kredit; notif user + grup admin (best-effort).
- Quote selalu dihitung server-side (callback payload tidak dipercaya).
- Env: `KTS_BASE_URL`, `KTS_API_KEY`, `KTS_SECRET` (required),
  `KTS_CHARGE_TTL_MIN` (default 24 jam), `MIN/MAX_TOPUP_AMOUNT`,
  `QRIS_FEE_PERCENT`, `QRIS_PPN_PERCENT`, `QRIS_EXPIRY_MINUTES`.
  Kredensial dari onboarding merchant (create-merchant) — `KTS_SECRET` =
  secretKey tunggal utk signing S2S DAN verifikasi webhook.

## Order Flow (M4)

Auto-order end-to-end: **pricing DB + seed → beli → fulfillment → ledger → renewal**.

> **UI copy policy**: semua **teks pesan tanpa emoji** (clean & modern — separator
> `━━━` dan bullet `•` dipakai sebagai elemen non-emoji); **icon hanya pada
> tombol navigasi** (menu utama, join grup, konfirmasi, kembali, flag negara).
> **Satu pengecualian (v1.43)**: banner brand `🏪 KENTANG TECH` di template
> notifikasi — keputusan user, parity legacy reference (lihat bullet Brand).

- **Pricing** — 12 paket (ID/SG/JP/CN × 15/30/90 hari) di-seed idempotent dari
  `seed/pricing.json` (`PRICING_SEED_FILE`) ke tabel `pricing` saat boot;
  harga beli **selalu live dari DB** (FR-03 AC).
- **Multi-panel dinamis (FR-10)** — blok env `PANEL_N_*`
  (`HOST/PORT/USERNAME/PASSWORD/API_PATH/USE_SSL/INSECURE/COUNTRY_CODE/FLAG_EMOJI/LOCATION/PROTOCOLS`),
  password dienkripsi AES-256-GCM saat seed (`vpn_servers.password_enc`);
  `insecure_tls` per-panel (default `false` — hanya aktifkan untuk panel
  staging self-signed).
- **Beli (FR-03/FR-04)** — `buy:menu` → pilih negara (hanya negara yang punya
  server open) → **pilih inbound (server + protocol) live dari panel**
  (`buy:inbound:{server}:{inbound}:{country}` — vless reality/ws, vmess,
  trojan, shadowsocks, hysteria, grpc, dll; enabled + port > 0 saja) →
  pilih paket → ringkasan + cek saldo → konfirmasi → `ordersvc.Purchase`
  (server+inbound+protocol terpin) — **debit-first + auto-refund (v1.47)**:
  order `pending→processing` → `PrepareClient` (read-only, tanpa sentuh
  panel) → **client row dibuat** (kredensial sudah digenerate) → **debit
  saldo atomik + ledger** → `CommitClient` (`addClient` di inbound terpilih)
  → `completed`; gagal sebelum debit → `failed` bersih (saldo tidak
  dipotong); gagal setelah debit → **refund atomik + ledger** + hapus row;
  order ID `KTS-XXXXXXXX-VPN`.
- **Renewal (FR-05)** — `renew:menu` → pilih akun **paid only** (trial
  disaring dari UI + ditolak service `ErrTrialNotRenewable`, v1.37) → pilih
  paket → konfirmasi → **idempotence** (`FindInFlight` — order duplikat yang
  masih pending/processing ditolak `ErrOrderInFlight`, v1.37) → **debit-first
  + auto-refund** (v1.37): debit atomik `balance >= harga` (tidak pernah
  minus) SEBELUM `updateClient`; panel/DB gagal setelah debit → **refund
  atomik + ledger**; expiry diperpanjang dari **sisa waktu**, tidak
  double-count → `completed`; pesan sukses menampilkan expiry asli.
  **Fix v1.38 (ditemukan E2E staging)**: `updateClient` x-ui mengganti
  seluruh objek client + validasi kredensial per-protocol — RenewClient kini
  memuat spec penuh client dari settings panel (kuota/ipLimit/flow tetap) dan
  hanya menaikkan `enable` + `expiryTime`; kunci panel per-protocol
  (`domain.PanelClientKey`): vless/vmess→UUID, trojan/hysteria→password,
  **shadowsocks→email** (dipakai renew & hapus akun).
- **Akun (FR-08)** — `account:menu` list akun **pagination 5/halaman**
  (newest first, FR-08 AC-1 v1.30) → `account:page:{n}` navigasi + indikator
  non-aksi (`account:noop`, answer tanpa edit — parity reference
  `accounts:page:{n}`); **status display v1.34 (AC-1)**: per item
  `Aktif`/`Hampir Habis`/`Expired` (teks polos — icon policy; Hampir Habis =
  nonaktif atau kuota ≥90% parity AC-3), badge `Trial ·` untuk akun trial,
  sisa waktu smart (jam untuk <24 jam, hari untuk paid) → tombol
  `Lihat Detail` per akun
  (`account:view:{id}`) → detail lengkap: kredensial **protocol-aware**
  (UUID vless/vmess, Password trojan/shadowsocks — v1.36) + **Limit IP &
  traffic terpakai/kuota** (AC-1 penuh v1.35); **URL config build TIDAK
  ditampilkan di detail ATAU view `Config V2Ray`** (v1.36, lebih clean —
  view config = parameter manual + hint ekspor) — URL full HANYA di
  **Ekspor .txt**; pesan sukses **Beli/Trial** juga tanpa URL (hint ekspor
  saja, v1.36) + tombol `Config V2Ray` (`account:config:{id}`,
  v1.26) + tombol **Convert YAML** (`account:convert:{id}`, FR-08 AC-2
  v1.33 — Clash/Meta: 2 blok proxy TLS 443 / NTLS 80, transport asli
  ws/grpc + password trojan asli, tag konsisten dengan remark URL;
  reality/ss/hysteria → fallback ConfigLink native, tanpa YAML ws palsu)
  + tombol `Ekspor .txt` (`account:export:{id}`) → dokumen `.txt`
  berisi kredensial & config link dikirim via `sendDocument` + tombol
  **Hapus Akun 2 langkah** (`account:delete:{id}` konfirmasi →
  `account:delete_confirm:{id}` eksekusi, FR-08 AC-4 v1.31) — panel
  `delClient` dulu, DB row setelah (ownership guard `GetViewOwned` di kedua
  langkah; peringatan tidak bisa dikembalikan; panel gagal → DB tidak
  dihapus; sukses → **tercatat di Riwayat FR-14** sebagai order
  `deletion` — label "Hapus Akun", nominal "—" (v1.35)).
- **Traffic per akun (FR-08 AC-3)** — tombol `Traffic` di detail
  (`account:traffic:{id}`) → sync live dari panel dulu
  (`trafficsvc.RefreshClient` — `getClientTraffics/:email`, protocol-
  agnostic, verified dari source panel) → re-read DB → halaman usage:
  **progress bar** 10 blok + **status warna 🟢🟡🔴** (≥90% Hampir Habis,
  ≥70% Perhatian, else Normal), Upload/Download/Total/Kuota/Sisa + waktu
  sync terakhir; tombol `Refresh` (callback sama, re-sync); sync gagal →
  tetap render data terakhir (best effort). Worker `traffic sync` dan
  refresh manual memakai SATU instance trafficsvc (shared panel factory).
- **Riwayat (FR-14)** — `history:menu` → list order user **pagination
  5/halaman** (newest first) → `history:page:{n}` navigasi + indikator
  non-aksi (`history:noop`, answer tanpa edit) → `history:detail:{id}`
  detail order (order ID, tipe, status, nominal, tanggal, akun terkait) —
  **hanya order milik user** (ownership guard `GetOwned`, foreign =
  "Transaksi tidak ditemukan"); kosong → pesan + tombol Beli/Top Up.
  Status/tipe dilabeli id-ID (`completed`→Selesai, `failed`→Gagal, dst.).
- **Bantuan (FR-15)** — `help:menu` → kategori (`help:order`, `help:topup`,
  `help:disclaimer`, `help:info`) → sub-halaman statis id-ID (`help:tos:account`,
  `help:tos:payment`) + shortcut aksi (`Beli VPN`/`Top Up`); setiap halaman
  punya tombol kembali (`⬅️ Kembali`) & `🏠 Menu Utama` (icon policy);
  konten murni statis — `service/telegram/menu_help.go` +
  `menu_help_tos.go` (split §1.1), routing `handler/telegram/help.go`
  tanpa service seam (parity `help_handler` reference).
- **Keyboard layout zigzag (v1.42, UX)** — semua sub-menu tidak lagi
  vertikal 1-1-1-1: tombol ditata pola **2-1-2-1-2** (baris pertama 2 tombol,
  berikutnya 1, berikutnya 2, dst. — baris terakhir mengambil sisa). Helper
  tunggal `service/telegram/menu_rows.go` (`packRows` + `backBtn`) dipakai
  semua picker (country/inbound/plan/server/client), menu aksi (admin,
  detail akun, sukses beli/trial) dan layar konfirmasi (Konfirmasi + Batal
  satu baris); urutan tombol dipertahankan; pager list Akun/Riwayat tetap
  baris sendiri. Tidak diubah (sudah 2-kolom): menu utama, quick-pick topup,
  kategori Bantuan.

## Config V2Ray (v1.26 → v1.27 path dinamis)

View config per akun — **dua opsi URL TLS/non-TLS** untuk import v2rayNG
(format reference `client-vpn`):

- `service/server/link_dual.go` — `DualConfigLinks`: pair **ws/grpc TLS port
  443** / **non-TLS port 80**, host dari `vpn_servers.host`; **path = asli
  dari API** (streamSettings inbound, disimpan saat provisioning — v1.27):
  `InboundStream()` mengekstrak `wsSettings.path` (ws) / `grpcSettings.serviceName`
  (grpc). Contoh nyata panel staging: `/vlessws`, `/vmessws`, `/trojanws`,
  `trojan-grpc`. **ws**→`type=ws&path=%2F…&host=…`, **grpc**→`type=grpc&serviceName=…`.
  **tcp/reality, shadowsocks, hysteria → tanpa varian → fallback `ConfigLink`
  native** (tidak ada link ws palsu utk akun reality). **Legacy row** (network
  kosong) tetap ws `/{protocol}` (backward compat). Kolom baru:
  `vpn_clients.inbound_network` + `inbound_path` (migrasi `000004`).
- `service/telegram/menu_config.go` — `AccountConfigText`: detail konfigurasi
  lengkap (domain, port TLS/non-TLS, ID/Password, **Network + Path/Service
  Name dinamis**) + 2 URL + cara pakai & tips; keyboard kembali ke detail.
  Tombol `Config V2Ray` juga di keyboard sukses Beli/Trial; ekspor `.txt`
  menyertakan kedua URL.
- Callback `account:config:{id}` — ownership `GetViewOwned` (id + user_id),
  sama seperti view/export.

> **Catatan staging**: port 80 saat ini 301 → HTTPS; bila non-TLS mau dipakai
> benar-benar, arahkan ke port non-TLS panel langsung atau buka location di
> nginx (lihat UAT). Path dual link WAJIB match lokasi nginx — mapping manual
> per deployment (UAT v1.27).
- **Ledger** — `balance_transactions` immutable; debit/kredit atomik via
  `UPDATE users SET balance = balance ± x WHERE balance >= x` dalam satu tx
  (`ErrInsufficientBalance` untuk saldo kurang/banned).
- **Notifikasi order ke grup admin (FR-04 AC, v1.41)** — saat order
  **berbayar** (beli/renew) **completed**, bot mengirim notice ke
  `NOTIFICATION_GROUP_ID` (order id, tipe, user, paket, nominal, email akun,
  aktif sampai, sisa saldo — body emoji-free, reuse label `orderTypeLabel`);
  **trial dikecualikan** (akun gratis); **best-effort** — gagal kirim hanya
  di-log, order tetap completed (seam `ordersvc.OrderNotifier` variadic,
  adapter di `cmd/bot/notify.go`; **gate `!= 0`** karena Telegram group ID
  negatif).
- **Brand KENTANG TECH (v1.43 → v1.44 lengkap)** — semua **template
  notifikasi/pesan transaksi** dibuka dengan banner brand `🏪 KENTANG TECH`
  + separator `━━━` (parity legacy reference `client-vpn`): notice admin
  grup, reminder kadaluarsa (FR-09), sukses **dan** gagal
  Beli/Perpanjang/Trial, ringkasan konfirmasi (beli/perpanjang/trial),
  ringkasan & QR Top Up. Brand = **KENTANG TECH** (BUKAN "KENTANG TECH
  STORE" — brand legacy); banner brand adalah **satu-satunya pengecualian
  icon** pada icon policy (keputusan user). **Ejaan diseragamkan (v1.44)**:
  header ekspor `.txt` (`=== AKUN VPN KENTANG TECH ===`), sambutan `/start`
  dan `help:info` memakai `BrandName` — tidak ada ejaan legacy
  (KentangTech/KENTANGTECH) di copy user-facing. Sumber tunggal:
  `service/telegram/menu_brand.go` (`BrandHeader()`/`BrandName`).

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
- `internal/job/interval.go` — **`IntervalWorker` generik** (dipakai
  notifikasi, traffic sync, health check & trial cleanup — bukan duplikasi
  loop): stdlib `time.Ticker` (bukan robfig/cron; AGENTS.md prefer stdlib),
  sweep pertama langsung saat boot, lalu tiap interval. Setiap sweep
  ber-timeout dan panic-recovered (AGENTS.md §1.6); loop berhenti saat
  shutdown (ctx cancel + WaitGroup drain). Tanggal di pesan diformat sesuai
  `TIME_LOCATION`.

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

## Health Check Panel (v1.45, PRD §17)

Worker interval memeriksa ketersediaan tiap panel aktif dan menulis hasilnya
ke `vpn_servers.health_status` (`ok`/`down`) + `last_health_check`:

- `service/health` — `RunOnce` list panel aktif → per panel
  `GET /xui/API/server/status` (timeout `XUI_API_TIMEOUT`) → tulis status;
  **satu panel gagal tidak menggagalkan sweep** (isolasi sama dengan sync
  traffic). Panel yang tidak terjangkau (network/decrypt error) ditandai
  `down`. **DB write pakai context terpisah** (`healthWriteTimeout` 10s,
  parent ctx) — panel mati yang menghabiskan budget connect tetap tercatat
  `down` (fix E2E v1.45: tanpa ini status tidak pernah tersimpan, server mati
  tetap dijual).
- **Server mati tidak dijual**: `ListBuyable` (menu Beli/Trial & country
  picker) mengecualikan panel `health_status = 'down'` — `IS DISTINCT FROM
  'down'` agar status lain (NULL / default `'unknown'` / `ok`) tetap dijual
  (boot pertama sebelum health sweep tidak menghilang). Status `down` juga
  terlihat di admin server list (`admin:server`, kolom health).
- Worker ini melengkapi kolom yang selama ini tidak pernah diisi
  (`health_status`/`last_health_check` ada sejak migrasi awal).

Env: `HEALTH_CHECK_ENABLED` (true), `HEALTH_CHECK_INTERVAL_SEC` (60 — 1-3600).

## Trial Cleanup (v1.45, PRD worker)

Akun trial berdurasi 1 jam (FR-07); tanpa pembersihan, trial yang lewat masa
pakai tetap `enable` di panel dan mencuri kuota. Worker interval
menonaktifkannya:

- `service/trialcleanup` — `RunOnce` list kandidat (trial `is_active`,
  belum `is_expired`, `expires_at <= now()`) → group per server →
  `serversvc.DisableClients` (**satu `GetInbounds` per panel**, anti N+1 §1.7:
  spec raw tiap client di-patch `enable=false` — kuota/ipLimit/flow
  dipertahankan, kunci updateClient per-protocol dari spec: vless/vmess→id,
  trojan→password, ss→email, hysteria→auth) → **baru** `MarkTrialExpired`
  (is_active=false + is_expired=true; row tetap ada di Akun Saya dengan
  badge Trial + status Expired).
- **Panel gagal → DB tidak ditandai** (retry sweep berikutnya); client yang
  sudah hilang dari panel dihitung sukses; satu panel gagal tidak
  menggagalkan sweep.

Env: `TRIAL_CLEANUP_ENABLED` (true), `TRIAL_CLEANUP_INTERVAL_MIN` (15 —
1-1440), `TRIAL_CLEANUP_BATCH` (50).

## Subscription URL (FR-13, v1.46)

Gap terakhir FR-13 — bagian (a) **subscription URL** (bagian (b) share link
per protokol sudah selesai v1.26–v1.33, dikirim via Ekspor .txt):

- Setiap client yang dibuat bot (beli **dan** trial) sudah mengirim `subId`
  ke panel sejak awal (`provisionClient`); sejak v1.46 nilai itu **di-persist**
  (`vpn_clients.sub_id`, migrasi `000006`) dan bot membangun
  **subscription URL** = `{SUB_BASE_URL}{SUB_PATH}/{subId}` +
  **JSON/Clash URL** = `{SUB_BASE_URL}{SUB_JSON_PATH}/{subId}`.
- **Opsi 2 (keputusan user)**: domain sama dengan panel, **port beda** —
  sub server panel default `:2096` (`sub/sub.go`, setting `subPort`).
  `SUB_BASE_URL` = `https://<panel-host>:2096` (harus match setting panel:
  `subEnable=true`, `subPath` default `/sub/`, `subJsonPath` default `/json/`;
  trailing slash di path dinormalisasi bot).
- **URL hanya di Ekspor .txt** (konsisten v1.36): `AccountTXTContent`
  menambahkan blok `Subscription URL (auto-update)` + `Subscription JSON
  (Clash/Meta)` — chat & detail akun tetap bersih; **akun lama** (dibuat
  sebelum v1.46, `sub_id` kosong) dilewati tanpa backfill (**legacy gap
  terdokumentasi** — keputusan user).
- Renew otomatis mempertahankan `subId` (updateClient patch raw spec) —
  URL tidak berubah saat perpanjang.
- **Prasyarat operasional**: sub server panel aktif (setting `subEnable`),
  path/port match config bot, dan port sub reachable publik (firewall);
  link di dalam konten sub memakai Host saat fetch → host sub harus resolve
  ke server yang sama dengan panel/VPN.

Env: `SUB_ENABLED` (false), `SUB_BASE_URL`,
`SUB_PATH` (`/sub`), `SUB_JSON_ENABLED` (false), `SUB_JSON_PATH` (`/json`).

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

- **Adjust Saldo (FR-11, v1.39)** — `admin:saldo` → `+ Kredit Saldo` /
  `- Debit Saldo` → FSM 2 langkah (ketik Telegram ID → ketik nominal rupiah) →
  konfirmasi → `adminsvc.AdjustBalance`: resolve tgID → PK, lalu `Credit`/
  `Debit` atomik **jalur yang sama dengan order** (SQL guard + ledger immutable,
  ref `ADJ-<random>`); debit > saldo ditolak ramah; unknown user → "belum
  terdaftar". **Idempotence (fix review v1.39)**: nominal input men-arm state
  `saldo:confirm:*` dan confirm **memverifikasi staging sebelum eksekusi** —
  tap ganda/retry pada tombol Konfirmasi tidak pernah menjalankan dua kali.
  Setiap mutasi tercatat di `balance_transactions`.
- **Manajemen Server (FR-11, v1.40)** — `admin:server` → daftar semua panel
  (aktif + nonaktif) → detail per server → **Toggle Buka/Penjualan**
  (`admin:server:open:{id}` — `is_open`, negara itu hilang dari pilihan Beli/
  Trial) dan **Toggle Aktif** (`admin:server:active:{id}` — `is_active`,
  panel dikecualikan dari sync traffic) → tombol **Tambah Server**
  (`admin:server:add`): **FSM 6 langkah** (nama → host → port → username →
  password → negara, opsional `CODE,FLAG`), draft menumpuk di FSM Redis
  (`srvadd:*`, TTL 10 mnt), input invalid → re-prompt langkah yang sama, dan
  langkah terakhir **men-arm state `srvadd:confirm:*`** — confirm
  **memverifikasi staging sebelum `AddServer`** (parity idempotence saldo
  v1.39: tap ganda hanya membuat SATU server); password **disegel
  AES-256-GCM** oleh serversvc sebelum disimpan (`password_enc`), tidak pernah
  di-echo ke chat.
- **Statistik & Audit Log (FR-11, v1.40)** — `admin:stats` → dashboard
  agregasi SQL satu round-trip (total/today orders + revenue dari
  `final_amount` order `completed`, breakdown status completed/failed/
  pending/processing/cancelled/refunded, total user, **client aktif**)
  + tombol **Order Terbaru** (10 baris bounded) → `admin:audit` → **audit
  log admin** (migrasi `000005` tabel `admin_audit_log` immutable, index
  `created_at DESC`): setiap aksi admin (ubah harga, toggle plan, reload,
  ban/unban, adjust saldo, broadcast, toggle/tambah server) tercatat siapa
  (admin id), aksi, target, detail — 15 baris terbaru per halaman.

FR-11 admin **lengkap** (harga, toggle, reload, broadcast, ban/unban, adjust
saldo, server, statistik, audit).

## Menjalankan dengan Docker Compose (produksi)

```bash
cd bot
cp .env.example .env          # set BOT_DOMAIN + BOT_TOKEN + secret
mkdir -p certs                # siapkan sertifikat TLS (lihat di bawah)
docker compose up -d --build
docker compose ps
curl https://<BOT_DOMAIN>/health
```

> **PostgreSQL & Redis HOST** (staging): stack ini TIDAK men-spin container
> DB. `DATABASE_URL`/`REDIS_URL` menunjuk ke service native host
> (`postgresql@16-main` :5432, `redis-server` :6379 — hanya bind loopback),
> jadi bot & nginx memakai `network_mode: host`. Konsekuensi: port bot
> (WEBHOOK_PORT, default `8443`) dan nginx (`80`/`443`) bind langsung di
> host — pastikan bebas, dan stop proses staging lama (mis.
> `/tmp/bot-staging` di `:8443`) sebelum `up`.

`nginx` menerima HTTPS di `443` dan meneruskan ke `127.0.0.1:8443` (host
network):
`https://<BOT_DOMAIN>/api/v1/webhooks/telegram → http://127.0.0.1:8443/api/v1/webhooks/telegram`.

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
| M5 | Topup menu/flow + PG Aggregate charge + webhook pg.charge | ✅ (v1.48) |

> **M5 status (v1.48)**: menu & flow top up (FR-06) **aktif** — quick-pick,
> nominal custom (FSM), ringkasan fee §15.7, `/cancel`. **API payment
> selesai**: `internal/repository/kts` (S2S HMAC signer + PG charge
> create/confirm/verify) menggantikan `StubGateway`; webhook
> `POST /api/v1/webhooks/payments` memverifikasi `X-Webhook-Signature`,
> branch `pg.charge`, dedup `X-Webhook-Id`, lalu `ApplySettlement` (kredit
> NET + mark dalam satu transaksi — anti double-credit). Amount = **NET**;
> gross-up (2,5% MDR + 11% PPN) di-handle gateway. Env: `KTS_BASE_URL`,
> `KTS_API_KEY`, `KTS_SECRET` (required, fail-fast), `KTS_CHARGE_TTL_MIN`,
> `MIN_TOPUP_AMOUNT`, `MAX_TOPUP_AMOUNT`, `QRIS_FEE_PERCENT`,
> `QRIS_PPN_PERCENT`, `QRIS_EXPIRY_MINUTES` (default ada di
> `config`/`.env.example`).
| M6 | Trial, notifikasi, sync traffic, admin     | ✅ (v1.21) |
| M7 | Hardening, test, UAT                      | 🔶 (v1.22: coverage gap ✅, load test ✅, UAT checklist ✅; v1.26: **config v2Ray dual TLS/non-TLS ✅**; v1.28: **Riwayat FR-14 ✅**; v1.29: **Bantuan FR-15 ✅**; v1.30: **pagination Akun FR-08 AC-1 ✅**; v1.31: **hapus akun FR-08 AC-4 ✅**; v1.32: **traffic + refresh manual FR-08 AC-3 ✅**; v1.33: **convert YAML Clash/Meta FR-08 AC-2 ✅**; v1.34: **status display list Akun FR-08 AC-1 ✅**; v1.35: **detail akun AC-1 lengkap (Limit IP + traffic terpakai) ✅ + hapus tercatat di Riwayat AC-4 ✅**; v1.36: **revisi minor UI akun (UUID/Password protocol-aware; URL build hanya di ekspor .txt; sukses Beli/Trial tanpa URL) ✅**; v1.37: **renew paid-only + idempotence (FindInFlight) + debit-first auto-refund ✅**; v1.38: **fix renew panel "empty client ID" — spec penuh client dipertahankan, PanelClientKey per-protocol ✅**; v1.39: **adjust saldo admin + idempotence confirm ✅**; v1.40: **manajemen server + statistik + audit log admin (FR-11 lengkap) ✅**; v1.41: **notifikasi order sukses ke grup admin (FR-04 AC) ✅**; v1.42: **UX keyboard zigzag 2-1-2-1 semua sub-menu (packRows di menu_rows.go) ✅**; v1.43: **brand KENTANG TECH di semua template notifikasi/pesan transaksi (banner 🏪 pengecualian icon policy) ✅**; v1.44: **konsistensi brand ditutup — ringkasan konfirmasi + pesan gagal ber-brand, ejaan diseragamkan (txt/home/info pakai BrandName) ✅**; v1.45: **worker health check panel (PRD §17 — server mati tidak dijual, ListBuyable filter health_status=down) + worker trial cleanup (disable akun trial expired di panel lalu tandai is_expired) ✅**; v1.46: **FR-13 subscription URL ✅ (sub_id dipersist migrasi 000006 + subscription_url/json di Ekspor .txt — Opsi 2 domain sama panel port beda; URL hanya di ekspor, akun lama tanpa backfill)**) |

> **M6 status (v1.21)**: **Trial (FR-07) ✅** — `service/trial` (daily limit
> 2x/hari via Redis counter TTL s.d. tengah malam, claim anti-race + rollback),
> flow `trial:menu` → `trial:server:{id}` → `trial:inbound:{server}:{inbound}`
> → `trial:confirm:{server}:{inbound}` (+ `/trial`) — protocol dipilih dari
> panel (v1.24), akun trial 1 jam / 1 GB / 1 IP (`is_trial=true`, **tanpa
> debit**), tombol
> "Beli VPN Premium" setelah sukses. **Notifikasi kadaluarsa (FR-09) ✅** —
> worker H-7/H-3/H-1 (section di bawah). **Admin (FR-11) ✅** — harga,
> toggle plan, broadcast, ban/unban (section di bawah). **Sync traffic
> (PRD §16.2) ✅** — worker interval sinkron kuota dari panel (section di
> bawah). Env baru: `TRIAL_ENABLED`, `TRIAL_DAILY_LIMIT`, `TRIAL_DURATION_HOURS`,
> `TRIAL_TRAFFIC_GB`, `TRIAL_IP_LIMIT`, `EXPIRY_NOTIFY_ENABLED`,
> `EXPIRY_NOTIFY_INTERVAL_MIN`, `EXPIRY_NOTIFY_BATCH`, `TRAFFIC_SYNC_ENABLED`,
> `TRAFFIC_SYNC_INTERVAL_MIN`, `TRAFFIC_SYNC_BATCH`.
