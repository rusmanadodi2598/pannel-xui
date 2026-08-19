# PRD — Bot Auto-Order Telegram (Go) untuk X-UI Panel

| Field            | Value                                                              |
|------------------|--------------------------------------------------------------------|
| **Dokumen**      | 001-PRD-BOT-ORDER                                                  |
| **Status**       | Draft v1.48 — M0–M4 ✅ · M5 ✅ (v1.48: PG Aggregate charge + webhook pg.charge — §15.7) · M6 ✅ · M7 ✅ (hardening + **detail akun & ekspor .txt** + **config v2Ray dual TLS/non-TLS path dinamis** + **Riwayat FR-14 ✅** + **Bantuan FR-15 ✅** + **pagination Akun FR-08 AC-1 ✅** + **hapus akun FR-08 AC-4 ✅** + **traffic & refresh manual FR-08 AC-3 ✅** + **convert YAML Clash/Meta FR-08 AC-2 ✅** + **status display list Akun FR-08 AC-1 ✅** + **detail akun lengkap AC-1 (Limit IP + traffic terpakai) ✅** + **hapus akun tercatat di Riwayat AC-4 ✅** + **revisi minor UI akun (UUID/Password protocol-aware; URL build hanya di ekspor .txt; sukses Beli/Trial tanpa URL) ✅** + **renew paid-only + idempotence + debit-first auto-refund ✅** + **adjust saldo admin ✅** + **FR-11 lengkap (manajemen server, statistik, audit log) ✅** + **notifikasi order sukses ke grup admin (FR-04 AC) ✅** + **UX keyboard zigzag 2-1-2-1 ✅** + **brand KENTANG TECH konsisten ✅** + **worker health check (server mati tidak dijual) + trial cleanup ✅** + **FR-13 subscription URL ✅ (sub_id dipersist, URL di ekspor .txt — Opsi 2, akun lama tanpa backfill)**) — **UI copy policy: teks tanpa emoji (icon hanya di tombol navigasi; banner brand pengecualian)** |
| **Tanggal**      | 2026-08-12                                                         |
| **Penulis**      | Dodi Rusmana `<rusmanadodi@kentangtechstore.com>`                  |
| **Scope Build**  | Hanya direktori `/bot` — **tidak menyentuh source panel x-ui**     |
| **Governance**   | Wajib patuh & ter-map ke `AGENTS.md` (lihat §10) — tanpa menulis ulang isinya |
| **Referensi**    | Bot Python: `/home/rusmanadodi/kentangtech-xui/client-vpn`        |
| **Stack**        | Go · PostgreSQL · Redis · Telegram Bot API (Webhook) · X-UI REST API |

---

## 1. Ringkasan Eksekutif

Kita membangun **bot Telegram auto-order** dalam **Go** sebagai **layanan terpisah
(standalone)** yang berkomunikasi dengan **X-UI Panel** hanya melalui **REST API**
panel — tidak menyentuh kode, database, atau proses panel sama sekali.

Pendekatan ini meng-upgrade bot Python yang sudah berjalan di
`client-vpn` (python-telegram-bot + aiohttp) ke implementasi Go yang:

- **Webhook-only** (WAJIB, bukan polling) — performa tinggi untuk ratusan user
  concurrent dan minim resiko rate-limit `getUpdates`.
- Memakai **PostgreSQL + Redis** sesuai stack yang ditetapkan `AGENTS.md`.
- Di-deploy dengan **Docker Compose** (bot + reverse proxy **Nginx** untuk TLS
  HTTPS yang disyaratkan Telegram webhook).
- Menggunakan library **`github.com/go-telegram/bot`** untuk Telegram Bot API.

Seluruh fitur yang ada di bot Python (order, renewal, topup QRIS, trial,
notifikasi, multi-server, admin, gate grup) di-port ke scope v1.

---

## 2. Latar Belakang & Problem Statement

### 2.1 Kondisi Saat Ini

| Aspek          | Bot Python (`client-vpn`)                      | Panel X-UI (`pannel-xui`)       |
|----------------|------------------------------------------------|---------------------------------|
| Bahasa         | Python 3.10+ (PTB v21, aiohttp, SQLAlchemy)    | Go 1.26 (Gin, GORM, SQLite)     |
| Database       | PostgreSQL (asyncpg)                           | SQLite (`/etc/x-ui/x-ui.db`)     |
| State/Redis    | Redis (session cache, idempotency, rate limit) | —                                |
| Mode Telegram  | Webhook (aiohttp, port 8300)                   | Tgbot bawaan (notifikasi, polling) |
| Fungsionalitas | Bot jualan VPN lengkap (saldo, QRIS, trial, dsb.) | Manajemen inbound/client VPN  |

### 2.2 Masalah

1. **Dua bahasa di satu domain bisnis** — maintenance ganda (Python + Go),
   konteks pindah tangan antar developer sulit.
2. **Bot Python tidak terkelola oleh standar codebase** — tidak ada enforcement
   `AGENTS.md`, testing, maupun struktur layer yang konsisten.
3. **Integrasi rapuh dengan panel** — kredensial panel, format payload, dan
   perilaku endpoint hanya tersimpan sebagai pengetahuan implisit di kode Python.
4. **Skalabilitas & keandalan** — target 500–1000 user concurrent menuntut
   arsitektur concurrent yang matang (worker pool, idempotency, retry).

### 2.3 Keputusan Strategis

- Bot dibangun sebagai **modul Go mandiri di `/bot`** (punya `go.mod` sendiri),
  sehingga **tidak mengganggu build panel** (root `go.mod` tidak berubah).
- Komunikasi bot ↔ panel **hanya lewat HTTP API panel** (login + session cookie),
  **dilarang** akses langsung ke DB/process panel.
- Mode Telegram **WAJIB Webhook** — polling dilarang (lihat §14).

---

## 3. Referensi: Bot Python `client-vpn`

Sumber referensi: `/home/rusmanadodi/kentangtech-xui/client-vpn`

### 3.1 Yang Di-port ke Go (parity fitur v1)

| Modul Python                     | Fungsi                               | Port ke Go |
|----------------------------------|--------------------------------------|------------|
| `main.py`, `helpers/webhook.py`  | Lifecycle + webhook server           | `cmd/bot` + `handler/telegram` |
| `services/xui_client.py`         | X-UI API client (login, session, CRUD client, trial) | `service/xui` + `repository` |
| `services/order_service.py`      | Order lifecycle (KTS-XXXX-VPN)       | `service/order` |
| `services/user_service.py`       | User, balance, ledger                | `service/user` |
| `services/client_service.py`     | VPN client (akun) tracking           | `service/client` |
| `services/pricing_service.py`    | Pricing dinamis (hot-reload)         | `service/pricing` (DB + admin cmd) |
| `services/topup_service.py`      | QRIS topup + fee                     | `service/payment` |
| `services/scheduler_service.py`  | Scheduler (expiry notify, dsb.)      | `worker/` (robfig/cron) |
| `handlers/*`                     | Flow Telegram (start, menu, buy, account, admin, dsb.) | `handler/telegram` |
| `handlers/history_handler.py`    | Riwayat transaksi (pagination + detail) | `handler/telegram` (history) |
| `handlers/help_handler.py`       | Bantuan, ToS & disclaimer            | `handler/telegram` (help) |
| `keyboards/*`                    | Inline keyboard & callback pattern   | `handler/telegram` (helper) |
| `helpers/decorators.py`          | Gate: admin, membership, rate limit  | middleware Telegram |
| `core/redis_client.py`           | Redis (state, idempotency)           | `repository/redis` |
| `api/server_registration.py`     | API pendaftaran server (opsional)    | `handler/api` (v1 opsional) |

### 3.2 Yang Ditinggalkan / Dimodifikasi

- **`addTrialClient`** — **tidak ada** di fork x-ui ini (diverifikasi: tidak ada
  route tsb. di `web/controller/api.go`). Trial dihitung sendiri oleh bot:
  `expiryTime = now + durationHours * 3600 * 1000` lalu `addClient` biasa
  (persis fallback `add_client(..., expiry_time=...)` di reference).
- **JWT/ENCRYPTION_KEY (Fernet)** → diganti **AES-256-GCM** (Go stdlib) untuk
  enkripsi kredensial panel server di rest.
- **aiohttp** → `net/http` + `go-telegram/bot`.
- **Pricing JSON hot-reload** → tabel DB + command admin (tetap mendukung import
  JSON sekali pakai dari file `server_price.json` untuk migrasi data lama).
- Bahasa *copy* Telegram default `id-ID` (dipertahankan dari reference).

### 3.3 Kontrak Bisnis yang Dipertahankan (dari reference)

- Format order ID: `KTS-XXXXXXXX-VPN` (unik, uppercase).
- Status order: `pending → processing → completed | failed | cancelled | refunded`.
- Fee QRIS: 2,5% + PPN 11% atas fee → saldo yang masuk = `gross − fee` (net).
- Trial: maks **2 akun/hari/user**, durasi 1 jam, kuota 1 GB (default).
- Notifikasi kadaluarsa: **H-7, H-3, H-1** (dapat dikonfigurasi).
- Rate limit: **30 request/menit/user** (window 60 detik).
- Gate grup: wajib join grup Telegram (cache keanggotaan 6 jam).
- Nominal topup: min Rp 5.000, maks Rp 5.000.000 (value = saldo **bersih** yang diterima).
- QRIS berlaku sesuai `KTS_CHARGE_TTL_MIN` (default 24 jam — v1.48; sebelumnya
  15 menit); nominal **quick-pick** (Rp 10.000 / 25.000 / 50.000 / 100.000 /
  200.000 / 500.000) + **custom input**; input FSM bisa dibatalkan dengan `/cancel`.
- Formula fee (v1.48 dihitung **gateway**): `gross = ceil(net / 0.97225)`
  (2,5% MDR + 11% PPN — effective rate **2,775%**); saldo kredit = `net`
  (detail §15.7).

---

## 4. Tujuan (Goals) & Non-Goals

### 4.1 Goals (v1)

- **G1** Bot Telegram auto-order mandiri dalam Go, webhook-only, live tanpa
  mengganggu panel x-ui.
- **G2** Parity fitur penuh dengan reference Python: beli, renewal, topup QRIS,
  trial, notifikasi, multi-server, admin, gate grup.
- **G3** Integrasi X-UI Panel via REST API yang terdokumentasi & diuji
  (auth session, addClient, updateClient, delClient, traffic, onlines).
- **G4** Kepatuhan penuh terhadap `AGENTS.md` untuk seluruh kode di `/bot`
  (header file, layer, type-safety, resilience, testing).
- **G5** Deployment sekali-klik via Docker Compose (bot + Nginx TLS).

### 4.2 Non-Goals (v1)

- ❌ Mengubah/menambah endpoint di source panel x-ui (termasuk root `go.mod`,
  `web/`, `xray/`, dll.).
- ❌ Membaca/menulis database panel (`x-ui.db`) — API hanya lewat HTTP.
- ❌ Mode polling Telegram dalam bentuk apapun (termasuk fallback).
- ❌ Manajemen inbound/outbound penuh (bot cukup: list inbound, CRUD client,
  traffic, onlines, status server, restart opsional untuk admin).
- ❌ Auto-renew dari sisi panel (`reset > 0`) — renew ditangani flow order bot.
- ❌ Payment selain QRIS (topup) pada v1.

---

## 5. Keputusan Kunci (Hasil Konsultasi)

| #   | Keputusan             | Pilihan                                   | Alasan singkat                                  |
|-----|-----------------------|-------------------------------------------|-------------------------------------------------|
| D1  | Scope fitur v1        | **Semua fitur** (order, renewal, topup QRIS, trial, notifikasi, multi-server, admin, gate grup) | Parity penuh dengan reference |
| D2  | Database              | **PostgreSQL + Redis**                    | Sesuai stack `AGENTS.md`, skalabel 500–1000 user |
| D3  | Deployment            | **Docker Compose** (bot + Nginx)          | Webhook butuh HTTPS publik; deploy terisolasi   |
| D4  | Library Telegram      | **`github.com/go-telegram/bot`**          | Ringan, webhook-first, mudah di-debug           |
| D5  | Posisi kode           | **`/bot` modul Go mandiri**               | Tidak mengganggu build panel (root go.mod utuh) |

---

## 6. Persona & User Stories

### P1 — Pelanggan (End User Telegram)
- Sebagai pelanggan, saya bisa `/start`, melihat menu, dan **membeli akun VPN**
  (pilih negara/server → paket → protokol → konfirmasi → saldo didebit →
  terima config link + sub URL).
- Sebagai pelanggan, saya bisa **perpanjang** akun yang hampir habis.
- Sebagai pelanggan, saya bisa **topup saldo** via QRIS dan saldo langsung masuk.
- Sebagai pelanggan, saya bisa **coba trial** (1 jam, kuota terbatas, 2×/hari).
- Sebagai pelanggan, saya bisa lihat **daftar akun, sisa kuota, masa aktif**,
  dan **copy config link / subscription URL**.
- Sebagai pelanggan, saya dapat **notifikasi H-7/H-3/H-1** sebelum akun expired.
- Sebagai pelanggan, saya bisa lihat **riwayat transaksi** (beli, perpanjang,
  topup) dan mengakses **menu bantuan/ToS/disclaimer**.

### P2 — Admin (Pemilik Toko VPN)
- Sebagai admin, saya bisa **set harga per negara/paket**, buka/tutup penjualan
  per server (`is_open`), dan set maintenance message.
- Sebagai admin, saya bisa **ban/unban user** dan lihat **statistik order &
  revenue** (hari ini, total, breakdown status).
- Sebagai admin, saya bisa **broadcast** pengumuman ke semua user.
- Sebagai admin, saya bisa **adjust saldo user**, **tambah/kelola server X-UI**
  lewat bot, dan **ban/unban** tanpa menyentuh database.
- Sebagai admin, saya mendapat **notifikasi order & topup** di grup admin.

### P3 — Operator Panel X-UI
- Sebagai operator, saya memastikan bot **tidak pernah** mengubah konfigurasi
  panel di luar endpoint API yang disetujui.
- Sebagai operator, saya bisa melihat **health bot** (`/health`) dan log
  terstruktur.

---

## 7. Fitur Fungsional (FR)

> Setiap FR memiliki acceptance criteria (AC). Format callback data mengikuti
> pola reference: `menu:home`, `buy:menu`, `buy:confirm:<server>:<days>`, dst.

### FR-01 Onboarding & Gate Grup
- `/start` → registrasi user (auto), cek ban, cek keanggotaan grup wajib.
- **AC**: user baru tersimpan di `users`; jika `REQUIRED_GROUP_ID` diset dan user
  belum join → kirim tombol join + verifikasi ulang (cache 6 jam via Redis);
  user banned → ditolak dengan pesan jelas.

### FR-02 Menu Utama & Navigasi
- Inline keyboard: 🛒 Beli VPN · 🔄 Perpanjang · 👤 Akun Saya · 💳 Top Up ·
  🎁 Trial · 📜 Riwayat · ℹ️ Bantuan.
- **AC**: semua callback `menu:*` bernavigasi tanpa re-render ganda (edit message,
  bukan kirim baru); state per-user diserialkan (tidak ada race double-tap);
  tombol pagination non-aksi menjawab callback (`answer`) tanpa edit pesan
  (noop — parity reference).

### FR-03 Beli VPN (Auto-Order Core)
1. Pilih negara (dari server aktif & open) → 2. Pilih paket (15/30/90 hari,
   sesuai pricing) → 3. Konfirmasi ringkasan + cek saldo.
- **AC**: harga selalu diambil live dari pricing; saldo tidak cukup → arahkan
  topup; order dibuat hanya setelah konfirmasi eksplisit; order ID
  `KTS-XXXXXXXX-VPN` unik; balance `balance_before` tercatat.

### FR-04 Order Fulfillment (State Machine)
- `pending → processing → completed | failed`; pada `completed`: akun dibuat di
  panel, saldo didebit atomik, config link + sub URL dikirim ke user, notifikasi
  ke grup admin.
- **AC-1 (Atomicity)**: alur **debit-first + auto-refund** (v1.47, parity
  renew v1.37): order `pending→processing` → **prepare client** (read-only,
  tanpa sentuh panel) → **insert row `vpn_clients`** (kredensial sudah
  digenerate) → **debit saldo atomik** (guard `balance >= harga`, tidak pernah
  minus) → **commit ke panel** (`addClient`). Gagal sebelum debit → `failed`
  bersih (tanpa akun panel, tanpa uang terpotong); gagal setelah debit →
  **refund atomik + ledger** + hapus row, order `failed` — akun aktif tanpa
  bayar mustahil. Debit saldo & pembuatan client X-UI idempoten (tidak ada
  double-charge jika webhook/retry ganda); error dari panel tercatat di
  `error_message`; transaksi DB atomik + unique constraint `order_id` + lock
  per-user (`bot:lock:user:{id}`) mencegah order ganda.
- **AC-2 (User Feedback)**: selama `processing` tampilkan "⏳ Memproses order...";
  selesai → kirim config link + sub URL; gagal → pesan error ramah + tombol
  ulangi; user bisa batalkan input FSM dengan `/cancel`.

### FR-05 Renewal
- Pilih akun aktif → pilih paket → konfirmasi → **updateClient** di panel
  (`expiryTime` diperpanjang dari sisa waktu; `totalGB` opsional ditambah).
- **AC**: `order_type = renewal`; waktu tidak ganda dihitung; notifikasi sukses
  berisi masa aktif baru.

### FR-06 Topup Saldo QRIS (Payment)
- `/topup` → pilih nominal **quick-pick** (10k/25k/50k/100k/200k/500k) atau
  **custom input** → bot hitung gross (fee) → **persist order topup** (orderId
  `tp_*` → row `orders` + `payments`) → **KentangTech PG Aggregate**
  `POST /api/v1/pg/charges` (amount = **NET**) →
  `POST /api/v1/pg/charges/{orderId}/confirm` → tampilkan **checkout QRIS**
  (URL checkout + caption: order ID, saldo diterima, total bayar, fee) →
  webhook masuk `POST /api/v1/webhooks/payments` (`X-Webhook-Event:
  pg.charge`) → kredit saldo net → notifikasi user + grup admin.
- **AC-1 (Alur)**: nominal di antara min/max (Rp 5.000 – Rp 5.000.000, value =
  saldo **bersih** yang diterima); gross (fee) dihitung **gateway** — quote
  bot = estimasi formula §15.7 untuk display; user dapat **cek status** charge
  (`GET /api/v1/pg/charges/{orderId}` — `kts.GetCharge`, ops manual; webhook
  tetap jalur primer karena reconciler gateway re-enqueue `pg.notify`); input
  custom bisa dibatalkan `/cancel`; checkout gagal → row ditandai, order
  failed bersih (tanpa kredit).
- **AC-2 (Keamanan)**: webhook diverifikasi **`X-Webhook-Signature`**
  (HMAC-SHA256 atas **raw body**, constant-time) → 403 bila salah; branch
  `X-Webhook-Event: pg.charge`; **idempotency via Redis `SETNX`
  (`bot:webhook:{X-Webhook-Id}`, TTL 7 hari)** — webhook ganda tidak
  double-credit; settlement (kredit NET + mark) dalam **satu transaksi
  conditional** `pending → terminal` — webhook & poll race aman; fee & net
  dicatat di `payments` + ledger; event `succeeded / failed / expired`
  ditangani (hanya `succeeded` yang kredit).
- **AC-3 (Notifikasi)**: sukses → pesan topup berhasil (gross, net, fee,
  saldo baru) + notifikasi grup admin (`notify_topup` pattern reference).

### FR-07 Trial
- `/trial` → cek fitur aktif (config trial) → tampilkan sisa kuota trial
  hari ini → pilih server (hanya server aktif) → konfirmasi → buat akun trial
  (1 jam, 1 GB, 1 IP) dengan `expiryTime` dihitung bot (karena tidak ada
  `addTrialClient`).
- **AC-1 (Limit)**: maks 2×/hari/user; limit di-re-check saat menu, pilih
  server, DAN konfirmasi (anti race); counter di Redis + riwayat trial.
- **AC-2 (Akun)**: `is_trial=true`; trial tidak bisa diperpanjang; setelah
  expired → worker menonaktifkan akun (`updateClient enable=false`);
  detail sukses menampilkan sisa trial & tombol beli premium.

### FR-08 Manajemen Akun (User)
- Menu "Akun Saya": daftar akun **pagination 5 item/halaman** → detail akun →
  aksi per akun (config, convert, traffic, refresh/sync, hapus).
- **AC-1 (List & Detail)**: item menampilkan status (✅ Aktif / ⚠️ Hampir
  habis / ❌ Expired), sisa waktu (`time_remaining_display`), badge 🎁 utk
  trial; detail menampilkan email, server (flag+name), protokol, limit IP,
  traffic terpakai/kuota, masa aktif (`status_display` reference).
  Implementasi Go (v1.34): status sebagai **teks polos** (`Aktif` /
  `Hampir Habis` / `Expired` — icon policy; Hampir Habis = nonaktif atau
  kuota ≥90% parity AC-3), badge trial `Trial ·`, sisa waktu smart (jam utk
  <24 jam, hari utk paid). Implementasi Go (v1.35): detail menampilkan
  **Limit IP** + **traffic terpakai/kuota** (parity AC-1 penuh; ekspor `.txt`
  menyertakan keduanya).
- **AC-2 (Config & Convert)**: view **URL config TLS & Non-TLS** (`vless://`,
  `vmess://` base64 JSON, `trojan://`) + **convert YAML Clash/Meta** (TLS &
  Non-TLS) — pola `build_config_links`/`build_yaml_configs` reference; port
  443/80, path `/{protocol}`, network `ws`, SNI = host server. Implementasi
  Go (v1.33): `account:convert:{id}` memakai **transport asli** (ws path
  dinamis / grpc serviceName, v1.27) + **password trojan asli** (fix quirk
  reference yang memakai uuid); reality/ss/hysteria → fallback ConfigLink
  native (tanpa YAML ws palsu, konsisten dual-link).
- **AC-3 (Traffic & Sync)**: halaman traffic dengan **progress bar + persen**
  & status warna (🔴≥90% 🟡≥70% 🟢); tombol **refresh/sync manual** per akun
  (panggil `getClientTraffics/:email` → update DB — email lookup verified
  protocol-agnostic dari source panel; refresh gagal → tampil data terakhir
  best-effort); data juga disinkron worker (§16.2). Satu instance trafficsvc
  dipakai worker sweep + refresh manual (v1.32).
- **AC-4 (Hapus Akun)**: hapus akun **2 langkah konfirmasi**
  (`account:delete:{id}` → `account:delete_confirm:{id}`); hapus dari panel
  (`delClient`) + DB; peringatan akun tidak bisa dikembalikan; aksi tercatat
  di log & riwayat. Implementasi Go (v1.35): setelah DB delete sukses, bot
  menulis **order `order_type=deletion`** (status `completed`, amount 0,
  `account_email` + protocol) sehingga aksi tampil di **Riwayat FR-14**
  (label "Hapus Akun", nominal "—") — satu sumber riwayat, tanpa tabel audit.

### FR-09 Notifikasi Kadaluarsa & Traffic
- Worker harian: scan akun expired dalam H-7/H-3/H-1 → kirim peringatan →
  tandai `notified_expiry` (anti-spam).
- **AC**: hanya mengirim sekali per ambang; waktu berdasarkan `TIME_LOCATION`.

### FR-10 Multi-Server X-UI
- Tabel `vpn_servers` (beberapa instance panel); health check berkala; server
  `is_open=false` tidak muncul di menu beli.
- **AC**: pilih server per negara (flag emoji); failure satu server tidak
  memblokir server lain; kredensial terenkripsi AES-256-GCM.

### FR-11 Perintah & Menu Admin (`/admin` + callback `admin:*`)
- **Manajemen harga**: set/ubah harga per negara+paket, toggle plan enabled,
  reload pricing.
- **Manajemen server**: **tambah server via input chat** (nama, host, port
  panel, username, password, negara+flag, SSL, protokol) — pola
  `handle_add_server_input` reference; buka/tutup penjualan (`is_open`) +
  maintenance message; nonaktifkan/hapus server.
- **Manajemen user**: **adjust saldo** (tambah/kurang via input —
  `handle_admin_balance_input`), **ban/unban** (input user + konfirmasi —
  `handle_admin_ban_input`/`ban_confirm`).
- **Statistik & audit**: statistik order & revenue (hari ini, total,
  breakdown status), recent orders (10 terakhir), broadcast chunked
  (100 msg/6 detik).
- **AC**: hanya `ADMIN_IDS`; setiap aksi (harga, server, ban, saldo)
  tercatat di audit log; input FSM admin dibersihkan setelah selesai/batal;
  aksi balance memakai transaksi atomik + ledger entry.

### FR-12 Rate Limiting & Anti-Abuse
- **AC**: 30 aksi/menit/user (Redis sliding window); limit trial; validasi input
  nominal & email; deteksi pola spam (opsional).

### FR-13 Subscription & Config Link
- Bot menyediakan: (a) **subscription URL** `https://{sub-domain}/{subPath}/{subId}`
  (server `sub` panel), dan (b) **share link per protokol** (vless://, vmess://,
  trojan://) yang dibangun dari `streamSettings` inbound.
- **AC**: link sesuai host/port inbound; VLESS Reality menyertakan `flow` &
  `pbk/sid/fp`; link tidak membocorkan private key.
- **Implementasi (v1.46, FR-13 gap ditutup)**: (a) subscription URL — **Opsi 2**
  (domain sama dengan panel, port sub server beda — keputusan user): bot
  menyimpan `sub_id` (UUID yang dikirim ke panel saat provisioning) di
  `vpn_clients.sub_id` (migrasi `000006`, + `subscription_json_url`), lalu
  membangun URL dari config `SUB_BASE_URL` + `SUB_PATH`/`SUB_JSON_PATH`
  (`ordersvc.SubLinks` — join kanonik, robust thd trailing slash); persist
  saat `Purchase` & `CreateTrial`; URL **hanya** ditampilkan di Ekspor `.txt`
  (keputusan user v1.36); akun lama (kolom kosong) = legacy gap
  terdokumentasi, tanpa backfill. (b) share link per protokol — selesai
  v1.26–v1.33 (`service/server/linkgen.go` + `link_*.go`, path dinamis dari
  streamSettings inbound). **Prasyarat ops**: sub server panel aktif
  (`subEnable=true`, `subPath`/`subJsonPath` match config, port reachable).

### FR-14 Riwayat Transaksi (Order History)
- Menu "Riwayat": daftar order user (pagination, 5/halaman) → detail per order
  (order ID, tipe, status, nominal, tanggal, akun terkait).
- **AC**: hanya order milik user sendiri; parity `history_handler` reference
  (`menu:history`, `history:page:{n}`, `history:detail:{id}`); status order
  dilabeli (pending/processing/completed/failed/cancelled/refunded); jika
  kosong → pesan "belum ada transaksi" + tombol beli/topup.

### FR-15 Bantuan, ToS & Disclaimer
- Menu "Bantuan": cara order, cara topup, disclaimer, ToS akun, ToS
  pembayaran, info kontak.
- **AC**: callback `help:menu`, `help:order`, `help:topup`, `help:disclaimer`,
  `help:tos:account`, `help:tos:payment`, `help:info` (parity `help_handler`
  reference); konten statis dari i18n (`id-ID`); setiap halaman punya tombol
  kembali ke menu help & home.

---

## 8. Non-Functional Requirements (NFR)

| ID  | Kategori        | Requirement                                                              |
|-----|-----------------|--------------------------------------------------------------------------|
| N1  | Performa        | Target **500–1000 user concurrent**; p95 response webhook < 1 detik (selain aksi XUI). |
| N2  | Concurrency     | Webhook diproses worker pool terbatas; **serialisasi per-user** (Redis lock) agar state machine aman; dedup `update_id`. |
| N3  | Keandalan       | Retry policy eksplisit (XUI: 2 retry + backoff + auto-relogin; Telegram 429: hormati `Retry-After`); **panic recovery di setiap goroutine**; graceful shutdown. |
| N4  | Keamanan        | Secret via env (tidak pernah di log); webhook secret token; HMAC payment; rate limit; ban; enkripsi kredensial server (AES-256-GCM). |
| N5  | Observability   | Structured logging (`log/slog`) + request ID; endpoint `/health`; metrik dasar (opsional Prometheus). |
| N6  | Idempotensi     | Seluruh efek samping (debit saldo, addClient, kredit topup) idempoten. |
| N7  | Compliance      | **Patuh `AGENTS.md`** (lihat §10). Error payload API internal **English**; *copy* Telegram = konten i18n (id-ID) — bukan error payload. |
| N8  | Portabilitas    | Build `linux/amd64` & `linux/arm64`; Docker image kecil (multi-stage). |

---

## 9. Arsitektur

### 9.1 Diagram Konteks

```
                    ┌──────────────────────────────────┐
   User (Telegram)  │          PUBLIC / TLS            │
        │           │                                  │
        ▼           │   Nginx (TLS)                    │
   Telegram Bot API │   │  (docker network)            │
        │ HTTPS     │   ▼                              │
        └──────────►│  /bot :8443  (Go service)        │
                    │  ├─ POST /api/v1/webhooks/telegram │
                    │  ├─ POST /api/v1/webhooks/payments │
                    │  └─ GET  /api/v1/health          │
                    └──────────────┬───────────────────┘
                                │
              ┌─────────────────┼──────────────────────┐
              │                 │                      │
              ▼                 ▼                      ▼
   ┌──────────────────┐  ┌──────────────┐   ┌────────────────────┐
   │ X-UI Panel A     │  │ PostgreSQL   │   │ Redis              │
   │ (REST API,       │  │ (orders,     │   │ (session xui, state│
   │  login + cookie) │  │  users, ...) │   │  idempotency, rl)  │
   └──────────────────┘  └──────────────┘   └────────────────────┘
   (Multi-server: A,B,C...)
                              │
                              ▼
   ┌──────────────────────────────────────────────┐
   │ KentangTech PG Aggregate (QRIS, spec 015)    │
   │ POST /api/v1/pg/charges → webhook pg.charge  │
   └──────────────────────────────────────────────┘
```

### 9.2 Komponen Bot (`/bot`)

| Komponen              | Tanggung jawab                                                              |
|-----------------------|-----------------------------------------------------------------------------|
| `cmd/bot`             | Composition root: wiring config, DB, Redis, handler, worker. **Tanpa logika.** |
| `handler/telegram`    | Webhook HTTP + dispatcher go-telegram/bot; keyboard; middleware (gate, admin, rate limit). |
| `handler/api`         | `/api/v1/*`: health, webhooks (telegram, payments), registrasi server.      |
| `service/order`       | State machine order, debit saldo, orchestration XUI.                        |
| `service/user`        | User, balance, ledger, ban.                                                  |
| `service/client`      | Tracking akun VPN (sync traffic, status).                                   |
| `repository/xui`      | Klien REST panel: login/session cache, inbounds, CRUD client, traffic, onlines (M2). |
| `service/topup`       | Quote fee §15.7, persist order, PG Aggregate charge + settlement webhook idempoten (v1.48).      |
| `service/pricing`     | Pricing per negara/paket (DB) + reload.                                     |
| `service/notification`| Kirim pesan ke user/grup; template i18n.                                    |
| `worker/`             | Scheduler jobs (notifikasi, sync traffic, trial cleanup, health check).     |
| `repository/`         | Akses PostgreSQL (GORM) & Redis; enkripsi secret.                           |
| `domain/`             | Entitas & value object (Order, Balance, Money, Email, Server, Client).      |
| `schema/`             | DTO + validasi (CDD).                                                        |
| `config/`             | Typed config env, fail-fast boot.                                            |

### 9.3 Aturan Batas (Boundary Rules) — Krusial

1. **`/bot` adalah Go module mandiri** (`bot/go.mod`) — tidak ada perubahan pada
   root `go.mod` panel; panel tetap build apa adanya.
2. **Bot TIDAK pernah** meng-import package internal panel
   (`github.com/alireza0/x-ui/...`). Satu-satunya jalur komunikasi = **HTTP REST
   API panel** (login session). (Selaras larangan cross-module reach-in
   `AGENTS.md` §1.5.)
3. Bot **tidak** membaca/menulis `x-ui.db`, tidak menyentuh proses xray,
   tidak restart panel (kecuali endpoint `restartXrayService` dipanggil via API
   pada perintah admin tertentu — default: **tidak**).
4. Tgbot bawaan panel (`web/service/tgbot.go`) **tidak disentuh**; bot ini
   berdiri sendiri dengan akun bot Telegram berbeda.
5. Struktur folder internal mengikuti `AGENTS.md` §1.5:
   `schema → domain → repository → service → handler`.

---

## 10. Governance: Mapping ke `AGENTS.md`

> Aturan build di bawah ini **di-link & di-map** ke `AGENTS.md` (dokumen sumber
> aturan) — tidak ditulis ulang. Setiap PR di `/bot` wajib lolos semua gate.

| `AGENTS.md`        | Penerapan di `/bot`                                                                                          |
|--------------------|---------------------------------------------------------------------------------------------------------------|
| §1.1 Batas 250 baris/file | Setiap file `.go` baru/modified < 250 baris; CI gate (`wc -l`). Split sebelum limit.                   |
| §1.2 Header wajib  | Setiap file `.go` wajib header: `@file`, `@for`, `@uses`, `@reason`, `@author` (Dodi Rusmana), `@layer` (`schema/domain/repository/service/handler/router/worker/job/util/config`), `@stability`, `@since`. |
| §1.3 Bahasa error  | Error payload API internal bot (HTTP JSON) **English** & terstruktur (`code`, `message`). *Copy* Telegram = konten i18n `id-ID` (bukan error payload) — keputusan produk, didokumentasikan di sini. |
| §1.4 Type safety   | Tanpa `any`/`interface{}` di application layer; semua input eksternal → struct schema + validasi; config typed fail-fast. |
| §1.5 Layer         | Aliran `schema → domain → repository → service → handler`; tanpa cross-module import ke panel.                |
| §1.6 Resilience    | Timeout `context` semua outbound; panic recovery di setiap goroutine boundary; retry policy eksplisit; rate limiting; log terstruktur. |
| §1.7 DB discipline | Index pada kolom lookup; batch query; pool limit PG eksplisit; migration up/down; tidak ada `SELECT *` tanpa batas. |
| §1.9 Doc sync      | Diagram arsitektur = dokumen ini (PRD). `SYSTEM_MAP.md` untuk `/bot` ditambahkan saat M1 bila struktur repo berubah signifikan. |
| §2.1 Testing       | TDD; tidak ada `t.Skip`; `httptest` untuk handler; `-race` wajib untuk paket ber-goroutine.                     |
| §2.2 DDD           | Rich domain models (mis. `Order`, `Balance`); value object (`Money`, `Email`, `Duration`); ubiquitous language. |
| §2.3 EDD           | Event domain (mis. `OrderCompleted`, `TopupCredited`) → reaksi asinkron (notifikasi, admin).                   |
| §2.4 CDD           | Schema-first: struct DTO + tag validasi ditulis sebelum handler.                                              |
| §2.5 BDD           | Penamaan test Given-When-Then (`TestBuy_GivenSufficientBalance_ThenOrderCompletedAndBalanceDebited`). |
| §2.6 FDD           | Task tracker = fitur bernilai bisnis kecil, bisa selesai & di-merge dalam beberapa hari.                      |

---

## 11. Struktur Modul `/bot`

```
bot/
├── go.mod                       # Modul mandiri (mis. github.com/kentangtech/bot-order)
├── go.sum
├── cmd/bot/main.go              # Composition root
├── internal/
│   ├── config/config.go         # Typed env config (fail-fast)
│   ├── schema/                  # DTO request/response + validasi (CDD)
│   ├── domain/                  # Entitas & value object (DDD)
│   │   ├── user.go
│   │   ├── order.go             # Order + OrderStatus state machine
│   │   ├── client.go
│   │   ├── server.go
│   │   ├── money.go             # Value object Money (IDR, anti float)
│   │   └── event.go             # Domain events
│   ├── repository/
│   │   ├── postgres/            # GORM: users, orders, vpn_clients, vpn_servers, payments, pricing, ledger
│   │   ├── redis/               # session xui, state, idempotency, rate limit
│   │   └── crypto.go            # AES-256-GCM secret box
│   ├── service/
│   │   ├── xui/                 # X-UI REST client (repository layer — M2)
│   │   ├── order/               # Fulfillment state machine
│   │   ├── user/                # Balance, ledger, ban
│   │   ├── client/              # Akun VPN tracking + sync traffic
│   │   ├── payment/             # QRIS KentangTech + webhook processor
│   │   ├── pricing/             # Pricing per negara/paket
│   │   ├── notification/        # Notifikasi user/grup + i18n
│   │   └── telegram/            # View builder (keyboard, teks) — murni presentasi
│   ├── handler/
│   │   ├── http/                # /api/v1/* (health, webhooks, servers)
│   │   └── telegram/            # Dispatcher + middleware (gate/admin/ratelimit) + handlers per fitur
│   ├── worker/                  # cron jobs (robfig/cron/v3)
│   └── util/                    # format, id generator (KTS-XXXX-VPN)
├── migrations/                  # SQL up/down (golang-migrate)
├── i18n/id-ID.toml              # Copy Telegram
├── Dockerfile                   # multi-stage
├── docker-compose.yml           # bot + nginx (TLS)
├── .env.example
└── README.md                    # Setup & run
```

---

## 12. Stack Teknologi & Dependensi (usulan, dipin saat M1)

| Kebutuhan        | Pilihan                                                       | Alasan                          |
|------------------|---------------------------------------------------------------|---------------------------------|
| Bahasa           | Go 1.26 (toolchain mengikuti `AGENTS.md`)                     | Standar codebase                |
| Telegram         | `github.com/go-telegram/bot` (v1.x, pin versi saat M3)        | Webhook-first, ringan (D4)      |
| HTTP server      | `net/http` (stdlib)                                           | Sesuai preferensi `AGENTS.md`   |
| DB               | PostgreSQL + **`gorm.io/gorm`** (diputus M1: sama dgn panel, pengalaman tim) | Konsisten & familiar             |
| Migrasi          | **`golang-migrate/v4`** (SQL up/down embedded via `embed` + iofs, diterapkan saat boot) | `AGENTS.md` §1.7; terverifikasi M1 |
| Redis            | **`redis/go-redis/v9`** (pool: `REDIS_POOL_SIZE`, `REDIS_DIAL_TIMEOUT_SEC`) | Standar; terverifikasi M1         |
| Cron             | `robfig/cron/v3` (sama dgn panel)                             | Terbukti di panel               |
| QR code (QRIS)   | `github.com/skip2/go-qrcode`                                  | Render `qr_string` → PNG (parity topup_handler; dipin saat M5) |
| Enkripsi secret  | stdlib `crypto/aes` (AES-256-GCM)                             | Tanpa dep tambahan              |
| Logging          | stdlib `log/slog`                                             | Standar                         |

---

## 13. Data Model (PostgreSQL)

> Kolom inti meniru reference Python (SQLAlchemy) agar migrasi data mudah.
> Semua tabel punya `created_at`/`updated_at`. Angka uang memakai `NUMERIC(15,2)`
> (bukan float) — value object `Money`.

### 13.1 `users`
`id PK`, `telegram_id BIGINT UNIQUE`, `username`, `first_name`, `last_name`,
`phone`, `language` (default `id`), `is_active BOOL`, `is_banned BOOL`,
`is_admin BOOL`, `balance NUMERIC(15,2)`, `total_spent NUMERIC(15,2)`,
`referral_code UNIQUE`, `referred_by`, `last_active`, `created_at`, `updated_at`.

### 13.2 `vpn_servers`
`id PK`, `name`, `host`, `port` (panel), `username`, `password_enc TEXT`
(AES-256-GCM), `api_path`, `use_ssl BOOL`, `country_code`, `flag_emoji`,
`location`, `max_clients INT`, `current_clients INT`, `is_active BOOL`,
`is_premium BOOL`, `is_open BOOL`, `priority INT`, `maintenance_message`,
`protocols JSONB`, `last_sync`, `last_health_check`, `health_status`,
`created_at`, `updated_at`.

### 13.3 `vpn_clients`
`id PK`, `user_id FK`, `server_id FK`, `inbound_id INT`, `email UNIQUE`,
`uuid`, `password`, `protocol`, `flow`, `traffic_limit BIGINT`,
`traffic_used/up/down BIGINT`, `ip_limit INT`, `is_banned BOOL`,
`is_active BOOL`, `is_expired BOOL`, `is_trial BOOL`, `expires_at TIMESTAMPTZ`,
`config_link TEXT`, `subscription_url TEXT`, `subscription_json_url TEXT`,
`sub_id TEXT` (FR-13 v1.46: subId yang dikirim ke panel — basis sub URL),
`notified_expiry BOOL`,
`last_sync`, `last_online`, `created_at`, `updated_at`.
*Index:* `email`, `user_id`, `expires_at`, `is_trial`.

### 13.4 `orders`
`id PK`, `order_id UNIQUE` (`KTS-XXXXXXXX-VPN`), `order_type`
(purchase/renewal/topup/trial/deletion), `user_id FK`, `server_id FK`, `client_id FK`,
`protocol`, `duration_days`, `traffic_gb`, `ip_limit`, `amount`, `discount`,
`final_amount`, `currency`, `status` (pending/processing/completed/failed/
cancelled/refunded), `notes`, `error_message`, `account_email`,
`account_remark`, `balance_before`, `balance_after`, `completed_at`,
`created_at`, `updated_at`.

### 13.5 `balance_transactions` (ledger, immutable)
`id PK`, `user_id FK`, `order_id`, `type` (credit/debit), `amount`,
`balance_after`, `created_at`.

### 13.6 `payments` (topup QRIS)
`id PK`, `order_id UNIQUE`, `user_id FK`, `amount_gross`, `amount_net`,
`fee_amount`, `fee_pct`, `provider_ref`, `provider_status`, `status`
(pending/success/failed/expired), `paid_at`, `created_at`, `updated_at`.

### 13.7 `pricing`
`id PK`, `country_code`, `plan_days INT`, `price NUMERIC(15,2)`, `enabled BOOL`,
`updated_at`. *(Data awal di-seed dari `server_price.json` reference.)*

---

## 14. Desain Telegram Webhook (WAJIB — Tanpa Polling)

> **Aturan produk: mode komunikasi Telegram = webhook 100%. Polling dilarang
> dalam bentuk apapun** (tidak ada `getUpdates`, tidak ada fallback polling).

### 14.1 Startup (SetWebhook)
Saat boot (sebelum menerima traffic):
```
POST https://api.telegram.org/bot<TOKEN>/setWebhook
  url                = https://<BOT_DOMAIN>/api/v1/webhooks/telegram  # = WEBHOOK_PATH
  secret_token       = <WEBHOOK_SECRET>          # wajib, acak ≥ 32 char
  allowed_updates    = ["message", "callback_query", "chosen_inline_result", ...]
  drop_pending_updates = true
  max_connections    = 40 (tunable 1–100)
```
- Verifikasi `getWebhookInfo` saat start; log URL + `pending_update_count`.
- `go-telegram/bot` menyediakan opsi webhook (`WithWebhookDomain`,
  `WithWebhookPort`, `WithWebhookSecretToken`) & `StartWebhook` /
  `StartWebhookWithHandler` — detail API dipin terhadap versi library saat M3.

### 14.2 HTTP Handler (`POST /api/v1/webhooks/telegram`)
> Path dapat diubah via `WEBHOOK_PATH`, namun wajib sama dengan URL yang
> didaftarkan ke `setWebhook`. Default mengikuti konvensi `/api/v1` (§26).
1. Verifikasi header **`X-Telegram-Bot-Api-Secret-Token`** → `constant-time
   compare`; mismatch → **403**, jangan proses.
2. Parse `Update`; **dedup `update_id`** (Redis, TTL 24 jam) → duplikat → 200 OK.
3. Enqueue ke **worker pool** (bounded, mis. 64) → **respond 200 secepatnya**
   (Telegram akan retry bila > timeout).
4. **Serialisasi per-user**: kunci Redis `bot:lock:user:{id}` — dua update dari
   user yang sama tidak diproses bersamaan (state machine aman).
5. Middleware chain: membership gate → ban check → admin check (bila perlu) →
   rate limit (30/menit) → dispatch.

### 14.3 Idempotensi Update
- `update_id` disimpan di Redis (SETNX). Update lama/duplikat langsung di-drop.

### 14.4 Anti-Polling Enforcement
- Tidak ada kode pemanggil `getUpdates`; code review checklist + grep CI:
  `getUpdates|DeleteWebhook|SetWebhook("")` dilarang.
- `.env` hanya berisi `WEBHOOK_*` — tidak ada opsi `POLLING_ENABLED`.

### 14.5 HTTPS
- Telegram **mewajibkan** HTTPS. **Nginx** di Docker Compose menangani TLS
  (sertifikat dari `./certs` atau Let's Encrypt via certbot) untuk
  `BOT_DOMAIN` → proxy ke `127.0.0.1:8443` (host network; instalasi:
  `bot/README.md`).

---

## 15. Integrasi X-UI Panel (REST API)

### 15.1 Autentikasi (Session Cookie)
- `POST {basePath}/login` (form `username`, `password`) → session cookie
  (`x-ui`). Dukungan `basePath` panel & web domain middleware.
- **Session cache di Redis** `xui:session:{serverID}` (TTL 1 jam) — sebelum
  setiap request, coba pakai session cached; verifikasi cepat; pada
  `401/403` → **auto re-login** lalu retry sekali (pola sama dgn
  `xui_client.py` reference).
- Timeout eksplisit (10s connect / 30s read); `InsecureSkipVerify` hanya untuk
  panel self-signed (env `XUI_ALLOW_INSECURE=true` per server, default false).
- **Kebijakan kredensial panel (VERIFIED)** — fork ini **single-admin**: tidak
  ada endpoint user management (`UserService` hanya `GetFirstUser/CheckUser/
  UpdateFirstUser`; controller hanya `/login` & `/logout`; `initUser` membuat
  `admin/admin`). Bot **wajib memakai kredensial admin panel**, disimpan
  terenkripsi AES-256-GCM (`password_enc`). Kebijakan: (1) buat password kuat
  khusus integrasi & ubah default; (2) jangan bagikan ke manusia; (3)
  minimalkan frekuensi login via session cookie cache di Redis; (4) pantau
  jejak login (panel log `Successful Login` + notifikasi tgbot bawaan); (5)
  rotasi berkala.

### 15.2 Endpoint yang Dipakai (diverifikasi dari `web/controller/api.go`)

| Method | Path (setelah basePath)                          | Fungsi bot            |
|--------|--------------------------------------------------|-----------------------|
| POST   | `/login`                                         | Auth session          |
| GET    | `/xui/API/inbounds/`                             | List inbound (pilih server/target, baca streamSettings) |
| GET    | `/xui/API/inbounds/getClientTraffics/:email`     | Traffic by email      |
| GET    | `/xui/API/inbounds/getClientTrafficsById/:id`    | Traffic by UUID/id    |
| POST   | `/xui/API/inbounds/addClient`                    | Buat akun (form `id` + `settings` JSON) |
| POST   | `/xui/API/inbounds/:id/delClient/:clientId`      | Hapus akun            |
| POST   | `/xui/API/inbounds/updateClient/:clientId`       | Update/renew akun     |
| POST   | `/xui/API/inbounds/onlines`                      | Cek user online       |
| GET    | `/xui/API/server/status`                         | Health check server   |
| POST   | `/xui/API/server/restartXrayService` (opsional, admin) | Restart xray  |

> Catatan: endpoint `addTrialClient` **tidak ada** di fork ini → trial via
> `addClient` dengan `expiryTime` dihitung bot (§3.2).

### 15.3 Payload `addClient` (mengikuti reference Python)
```
POST /xui/API/inbounds/addClient
  id       = <inboundId>
  settings = {"clients":[{"id"|"password"|"auth": <credential>,   // per protokol
                          "email": <email>,
                          "limitIp": 2,
                          "totalGB": <bytes>,        // 0 = unlimited
                          "expiryTime": <ms epoch>,  // dihitung bot
                          "enable": true,
                          "flow": "xtls-rprx-vision",// khusus VLESS Reality
                          "subId": <subId>,
                          "tgId": "<telegram_id>",
                          "reset": 0}]}
```

### 15.4 Perbedaan Protokol (dari `web/service/inbound.go`)
| Protokol      | Field credential | Catatan                                      |
|---------------|------------------|----------------------------------------------|
| VLESS / VMess | `id` (UUID)      | VLESS bisa `flow` untuk Reality/vision       |
| Trojan        | `password`       | —                                            |
| Shadowsocks   | `email` (credential = email) | `method`/cipher diambil dari settings inbound |
| Hysteria      | `auth`           | —                                            |

- **Email unik** dipaksa panel (`Duplicate email:` error) → bot pre-check
  dengan pola email unik `user{telegramID}-{timestamp}@domain` atau query traffic.
- `subId` default = 8 karakter pertama UUID → **subscription URL**
  `https://{sub-host}/{subPath}/{subId}` (server sub panel, lihat `sub/sub.go`).

### 15.5 Share Link Builder
- Parse `streamSettings` inbound (address/host, port, security, reality, ws, dll.)
  untuk membangun `vless://`, `vmess://`, `trojan://` (encode Base64 URL-safe).
- Utama disarankan: kirim **subscription URL** (lebih tahan terhadap perubahan
  konfigurasi inbound); share link per protokol sebagai pelengkap.
- **Implementasi Go (v1.26–v1.46)**: share link dibangun bot sendiri
  (`serversvc.ShareLink` — vless/vmess/trojan/ss/hysteria + reality
  `flow`/`pbk/sid/fp`, tanpa private key) dan dikirim HANYA via Ekspor .txt
  (keputusan user v1.36); subscription URL (`{SUB_BASE_URL}{SUB_PATH}/{subId}`)
  di-persist saat order (migrasi `000006`: `sub_id` + `subscription_json_url`)
  dan ikut di ekspor .txt — prasyarat: sub server panel aktif (`subEnable`,
  `subPath`/`subJsonPath` match config bot, port sub reachable; Opsi 2 = domain
  sama dengan panel, port beda — default 2096).

### 15.6 Kontrak Error
- Respon panel `{"success": false, "msg": "..."}` → diterjemahkan ke error
  terstruktur bot (`XUIError`) dengan kode kategori: `AUTH`, `NETWORK`,
  `DUPLICATE_EMAIL`, `INBOUND_FULL`, `TIMEOUT`, `UNKNOWN`.

### 15.7 Kontrak API Pembayaran — PG Aggregate (KentangTech, spec 015)
Base URL: `KTS_BASE_URL` (env; default `https://gateway.kentangtechstore.com`).
Provider: **Midtrans QRIS** via kentangtech payment gateway (white-label —
spec `015-SPEC-PG-AGGREGATE.md`, transport webhook `013-PUBLIC-MERCHANT-OUTBOX-WEBHOOKS.md`).
Timeout: 30 s.

Satu id E2E: `orderId` client-supplied (bot: `tp_*`, charset `[A-Za-z0-9._-]`,
4–50) → payment_id + Midtrans order_id + webhook refId. **Amount = NET** yang
diterima merchant; gross (2,5% MDR + 11% PPN, `ceil(net / 0.97225)`) dihitung
**gateway** — bot mengirim NET dan mengkredit NET dari order lokal.

**S2S outbound (semua request) — chain HMAC (001 §2.3):**
```
X-API-Key: {KTS_API_KEY}
X-Timestamp: {unix_seconds}            // toleransi ±300 s
X-Nonce: {uuidv4}
Idempotency-Key: {orderId}
X-Signature: sha256=hex(hmac_sha256({KTS_SECRET}, canonical))
canonical = "v1\n{apiKey}\n{ts}\n{nonce}\n{METHOD}\n{path}\n{hex_sha256(body)}"
```
Envelope respon: `{ "data": {...} }`; error: `{ "error": { "code", "message" } }`.

**1) Create charge — `POST /api/v1/pg/charges`**
Body:
```json
{ "orderId": "tp_...", "amount": { "amount": 10000, "currency": "IDR" },
  "description": "topup saldo" }
```
→ 201 `created` (tanpa provider call); replay `orderId` sama → 200;
`orderId` dipakai merchant lain → 409 `DUPLICATE_ORDER`; auth gagal → 401.

**2) Confirm — `POST /api/v1/pg/charges/{orderId}/confirm`**
→ 202 `pending` + `checkoutUrl` (state-first: pending di-persist sebelum
Midtrans dipanggil). Bot menampilkan checkout QRIS ke user.

**3) Verify — `GET /api/v1/pg/charges/{orderId}`**
→ 200 detail charge (status `created|pending|paid|expired|failed|refunded`,
gross) / 404 tidak ditemukan. Dipakai poll manual/ops; webhook tetap jalur
primer — reconciler gateway re-enqueue `pg.notify` utk charge yang webhook-nya
hilang (015 Phase 6.1).

**4) Webhook callback — `POST /api/v1/webhooks/payments` (di bot)**
Header: `X-Webhook-Signature: sha256=hex(hmac_sha256({KTS_SECRET}, RAW body))`
(diverifikasi constant-time), `X-Webhook-Event: pg.charge`,
`X-Webhook-Id: pg.charge.{orderId}.{status}` (alphanumeric + `._-`, tanpa `:`).
**Deploy note:** webhook_url merchant di sisi gateway harus mengarah ke
`https://{BOT_DOMAIN}/api/v1/webhooks/payments`. Body:
```json
{
  "eventType": "pg.charge",
  "orderId": "tp_xxx", "refId": "tp_xxx",
  "status": "succeeded | failed | expired",
  "amount": { "amount": 10280, "currency": "IDR" },   // GROSS — jangan utk kredit
  "providerTrxId": "...", "paidAt": "...",
  "errorCode": "", "errorMsg": "", "occurredAt": "..."
}
```
Behavior (v1.48): `succeeded` → kredit **NET dari order lokal** + mark
`pending → terminal` dalam satu transaksi conditional (webhook & poll race
aman, §FR-06 AC-2); `failed`/`expired` → tanpa kredit, order ditandai;
**2xx hanya setelah persist durable** — non-2xx → gateway retry (max 5,
backoff 30 s × attempt) → dead-letter (013 §2).

**4) Formula fee (parity `calculate_qris_gross_amount`)**
```
effective_rate = QRIS_FEE_PERCENT * (1 + QRIS_PPN_PERCENT)   // 2.5% * 1.11 = 2.775%
gross          = ceil( net / (1 - effective_rate) ) → dibulatkan ke atas kelipatan 100
qris_fee       = gross * 2.5%
ppn_fee        = qris_fee * 11%
total_fee      = qris_fee + ppn_fee
saldo kredit   = net   // user memilih nominal BERSIH
```
QRIS berlaku **15 menit** (capture `expires_at` dari respon create).

---

## 16. Scheduler & Notifikasi (`worker/`)

| Job               | Jadwal        | Aksi                                                            |
|-------------------|---------------|-----------------------------------------------------------------|
| Expiry notify     | harian        | Scan `vpn_clients` expired dalam H-7/H-3/H-1 (`notified_expiry=false`) → notif → tandai. |
| Traffic sync      | tiap 5–10 mnt | Ambil traffic dari panel (`getClientTrafficsById`) → update `traffic_used/up/down`, `last_online`. |
| Trial cleanup     | tiap jam      | Trial expired → `updateClient enable=false` (atau hapus via `delClient`). |
| Server health     | tiap 60 detik | `/xui/API/server/status` per server → update `health_status`; server mati tidak dijual. |
| Topup expiry      | tiap menit    | QRIS pending lewat waktu → `PAYMENT_EXPIRED` → notif user; sinkron status via `GET status/{orderId}` utk order yg webhook belum diterima. |

- Semua job: panic recovery, context timeout, error log terstruktur, tidak
  restart bila error (log + metric).
- Timezone dari config `TIME_LOCATION` (Asia/Jakarta default).

---

## 17. Keamanan

1. **Secret management** — semua via env; tanpa hardcode; tanpa secret di log.
   Kredensial panel per server dienkripsi AES-256-GCM (`ENCRYPTION_KEY`).
2. **Webhook Telegram** — secret token diverifikasi constant-time; HTTPS wajib.
3. **Payment webhook** — `X-Webhook-Signature` (HMAC-SHA256 raw body, 013 §2.2)
   constant-time compare; branch `X-Webhook-Event`; idempotency `SETNX`
   (`X-Webhook-Id`); replay ditolak.
4. **Rate limiting** — 30 aksi/menit/user (Redis sliding window); limit trial.
5. **Admin** — hanya `ADMIN_IDS`; route admin dicek middleware.
6. **Input validation** — nominal topup (min/max), durasi, email; semua input
   eksternal → schema tervalidasi (`AGENTS.md` §1.4).
7. **Goroutine safety** — panic recovery di setiap goroutine (`AGENTS.md` §1.6);
   graceful shutdown (SIGTERM → drain worker → tutup DB/Redis).
8. **Least privilege panel** — bot hanya memakai endpoint yang disetujui
   (§15.2); tidak ada akses file/DB panel.

---

## 18. Observability

- `log/slog` JSON: request ID per update; field kontekstual
  (`user_id`, `order_id`, `server_id`, `duration_ms`).
- `GET /api/v1/health` → `{"status":"ok|degraded","db":"ok|error","redis":"ok|error","webhook":"not_registered","version":"..."}`
  (alias infra `GET /health` untuk Docker/Nginx probe). HTTP 200 = `ok`; 503 = `degraded`
  saat DB/Redis error. `webhook` → `not_registered` hingga M3 mendaftarkan `setWebhook` saat boot.
- Endpoint `/metrics` (opsional, Prometheus): counter order, topup, error XUI,
  panic, latency webhook.
- Audit log: order lifecycle, perubahan harga, ban/unban, broadcast.

---

## 19. Deployment (Docker Compose)

### 19.1 Topologi
```
docker-compose.yml
  services:
    bot:     build ./bot, env dari .env, network_mode: host, :8443 di host
    nginx:   image nginx:1.27-alpine, network_mode: host, TLS (certs ./certs / Let's Encrypt), proxy → 127.0.0.1:8443
  PostgreSQL & Redis = service native HOST (postgresql@16-main :5432,
    redis-server :6379) — TIDAK ada container DB; akses via loopback karena
    kedua service host hanya bind 127.0.0.1 (staging server)
```
- Webhook di Nginx: `https://{BOT_DOMAIN}/api/v1/webhooks/telegram →
  http://127.0.0.1:8443/api/v1/webhooks/telegram`.
- TLS: sertifikat di `./certs` (`fullchain.pem` + `privkey.pem`) atau mount
  Let's Encrypt; renewal via certbot (lihat `bot/README.md`).
- Healthcheck Docker: `wget -qO- http://127.0.0.1:8443/health`.

### 19.2 Env Vars (`.env.example`, mengikuti reference Python)

| Env                      | Contoh                                  | Keterangan                        |
|--------------------------|-----------------------------------------|-----------------------------------|
| `BOT_TOKEN`              | `123456:ABC...`                         | Token bot Telegram               |
| `BOT_DOMAIN`             | `bot-xui.kentangtechstore.com`          | Domain webhook (HTTPS)           |
| `WEBHOOK_PORT`           | `8443`                                  | Port internal bot                |
| `WEBHOOK_PATH`           | `/api/v1/webhooks/telegram`            | Path webhook Telegram (REST API v1) |
| `WEBHOOK_SECRET`         | (acak ≥ 32 char)                        | Secret token Telegram            |
| `DATABASE_URL`           | `postgres://bot:bot@127.0.0.1:5432/bot` | PostgreSQL (host-native service)  |
| `REDIS_URL`              | `redis://127.0.0.1:6379/3`              | Redis (host-native service)       |
| `ENCRYPTION_KEY`         | (32-byte base64)                        | AES-256-GCM secret box           |
| `ADMIN_IDS`              | `123456789,987654321`                   | Admin Telegram IDs               |
| `REQUIRED_GROUP_ID`      | `-100123456789`                         | Gate grup (kosong = nonaktif)    |
| `REQUIRED_GROUP_LINK`    | `https://t.me/kentangtech`              | Invite link grup                 |
| `NOTIFICATION_GROUP_ID`  | `-100123456789`                         | Grup notifikasi admin            |
| `EXPIRY_NOTIFY_DAYS`     | `7,3,1`                                 | Ambang notifikasi kadaluarsa     |
| `RATE_LIMIT_REQUESTS`    | `30`                                    | Rate limit per menit             |
| `TIME_LOCATION`          | `Asia/Jakarta`                          | Timezone                         |
| `XUI_API_TIMEOUT`        | `30`                                    | Timeout panggil panel            |
| `KTS_BASE_URL`           | `https://gateway.kentangtechstore.com`  | Base URL PG Aggregate (015)      |
| `KTS_API_KEY`            | (secret)                                | API key merchant (S2S)           |
| `KTS_SECRET`             | (secret, ≥ 16 char)                     | secretKey: sign S2S + verify webhook |
| `KTS_CHARGE_TTL_MIN`     | `1440`                                  | TTL charge (default 24 jam)      |
| `MIN/MAX_TOPUP_AMOUNT`   | `5000` / `5000000`                      | Batas nominal topup              |
| `QRIS_FEE_PERCENT`       | `0.025`                                 | Fee QRIS 2,5% (quote display; gross-up nyata di gateway) |
| `QRIS_PPN_PERCENT`       | `0.11`                                  | PPN atas fee 11%                 |
| `QRIS_EXPIRY_MINUTES`    | `15`                                    | Referensi (TTL aktual = `KTS_CHARGE_TTL_MIN`) |
| `LOG_LEVEL`              | `INFO`                                  | `DEBUG` untuk development        |
| `PANEL_*` (opsional)     | —                                       | Kredensial panel awal / seed     |

---

## 20. Testing Strategy (`AGENTS.md` §2.1)

| Level              | Cakupan                                                              |
|--------------------|----------------------------------------------------------------------|
| Unit               | Service order (state machine, debit), pricing, money (aritmatika IDR), format link, crypto. |
| Repository         | **Integration test terhadap PG/Redis lokal (staging host)**: `bot_test` DB + Redis DB 15; verifikasi up (7 tabel/kolom/index/UNIQUE per §13), down (rollback semua), idempotensi rerun; Redis: ping, set/get, SetNX idempotency. Override DSN via `TEST_DATABASE_URL` / `TEST_REDIS_URL`. Tanpa `t.Skip` (AGENTS.md §2.1). |
| Handler (httptest) | `POST /api/v1/webhooks/telegram` (valid/invalid secret), `POST /api/v1/webhooks/payments` (signature valid/salah, event salah, duplikat `X-Webhook-Id`, settle sukses/expired), `GET /api/v1/health`. |
| KTS client         | Signer vector HMAC (canonical §15.7, byte-exact) + httptest gateway: 201 create, 202 confirm, 404 not-found, 409 duplicate, 401 unauthorized. |
| XUI client         | Mock HTTP server (httptest) untuk login, session expire → relogin, addClient payload golden test. |
| Race               | `go test -race ./...` — wajib untuk paket goroutine (webhook, worker). |
| E2E (opsional)     | Bot + panel uji terhadap instance panel dev (docker) untuk alur beli penuh. |
| BDD naming         | `TestBuy_GivenSufficientBalance_ThenOrderCompletedAndBalanceDebited`. |

---

## 21. Milestone & Roadmap

| Milestone | Isi                                                                  | Estimasi |
|-----------|----------------------------------------------------------------------|----------|
| **M0**    | PRD final (dokumen ini) + scaffolding `/bot` + `.env.example` + CI lint config | ✅ selesai (v1.0 awal / v1.3 konvensi REST) |
| **M1**    | Config typed, PostgreSQL+Redis connect, migration, `/health`, Docker Compose (bot+Nginx) | ✅ selesai (v1.4) |
| **M2**    | X-UI client (login, session cache, CRUD client, traffic) + unit test mock server | ✅ selesai (v1.7) |
| **M3**    | Core webhook go-telegram/bot: setWebhook, secret token, dispatcher, middleware (gate/ban/rate limit), menu | ✅ selesai (v1.8) |
| **M4**    | Order flow: pricing, beli, renewal, fulfillment state machine, ledger | ✅ selesai (v1.11) |
| **M5**    | Topup QRIS: PG Aggregate charge + webhook HMAC + idempotency + notif | ✅ selesai (v1.13 flow + **v1.48 API**): menu/flow ✅; v1.48 `internal/repository/kts` (S2S HMAC + create/confirm/verify charge, amount = NET) + webhook `pg.charge` + settlement idempoten (migrasi 000007, `KTS_*`) |
| **M6**    | Trial, notifikasi kadaluarsa, sync traffic, multi-server, perintah admin | ✅ selesai (v1.21 → v1.40): Trial · Notifikasi kadaluarsa · Sync traffic · **Admin lengkap (harga/broadcast/ban-unban + manajemen server + statistik + audit log, v1.40)** · adjust saldo (v1.39) |
| **M7**    | Hardening: test penuh, race, load test, staging, UAT, dokumentasi | 🔶 partial (v1.22 → v1.48): coverage gap ✅ · race ✅ · load test ✅ · UAT checklist ✅ · **FR-08/14/15 lengkap (v1.25–v1.35) · renew paid-only + idempotence + auto-refund (v1.37–v1.38) · UX zigzag + brand (v1.42–v1.44) · worker health check + trial cleanup (v1.45) · FR-13 subscription URL (v1.46) ✅** + **purchase debit-first + auto-refund (v1.47) ✅** + **M5 selesai — PG Aggregate charge + webhook pg.charge (v1.48) ✅** — sisa: eksekusi UAT item yang butuh user non-admin/akun mendekati expiry/prasyarat panel (docs/002, 46 item) + demo E2E beli → QRIS → akun aktif (prasyarat merchant `KTS_*` live) |
| **Total** |                                                                      | **± 4–6 minggu** |

**Exit criteria v1:** semua AC FR-01 s.d. FR-15 lolos; `go test -race ./...`
hijau; demo end-to-end beli → QRIS → akun aktif di staging; panel x-ui tidak
terkena perubahan apa pun.

---

## 22. Risiko & Mitigasi

| Risiko | Dampak | Mitigasi |
|--------|--------|----------|
| Session panel expire / kena rate-limit login | Semua operasi XUI gagal | Session cache + auto-relogin + backoff; health check per server; alert ke grup admin |
| Webhook Telegram tidak terdaftar (HTTPS/domain) | Bot mati total | Nginx TLS + certbot (renew otomatis); verifikasi `getWebhookInfo` saat start; healthcheck |
| Panel di-upgrade fork lain (kontrak API berubah) | Integrasi rusak | Kontrak API dipin di dokumentasi ini (§15); integration test terhadap panel dev |
| Double-charge / double-credit topup | Kerugian finansial | Idempotency `SETNX`, ledger immutable, transaksi DB atomik, unique constraint `order_id` |
| Update Telegram bersamaan pada user sama | State machine korup | Serialisasi per-user (Redis lock) + worker pool bounded |
| Rate-limit Telegram 429 | Notifikasi/aksi gagal | Hormati `Retry-After`; antrian notifikasi; backoff |
| Secret bocor lewat log | Kompromi akses | Redaksi secret; `log/slog` tanpa nilai rahasia; audit log akses |
| Gangguan panel saat order diproses | Order failed | Retry 2× backoff; alur debit-first + auto-refund (v1.47) — gagal sebelum debit → tanpa potong, gagal setelah debit → refund otomatis + ledger; user bisa ulang |
| Kehilangan kredibilitas harga (salah hitung) | Klaim refund | Harga hanya dari `pricing`; audit `balance_before/after`; test aritmatika `Money` |
| API payment (KentangTech PG) berubah / down | Topup gagal, komplain user | Kontrak dipin §15.7 (015/013); timeout eksplisit; webhook non-2xx → retry gateway (max 5, backoff 30 s×attempt → dead-letter); poll `GET /api/v1/pg/charges/{orderId}` utk fallback; alert admin; pesan ramah ke user |
| QRIS QR gagal dirender (lib QR) | User tak bisa scan | Fallback tampilkan `qr_string` teks; pilih lib stabil; golden test |

---

## 23. Open Questions (untuk dikonfirmasi sebelum M4/M5)

1. **Kontrak KentangTech API** — **SEBAGIAN TERJAWAB**: kontrak lengkap
   (create/status/webhook/fee) didokumentasikan di §15.7 dari kode reference.
   Yang perlu dikonfirmasi ke tim payment: versioning endpoint, konsistensi
   field error `detail`, SLA/rate limit server payment.
2. **Kredensial panel untuk bot** — **RESOLVED**: fork ini single-admin (tidak
   ada endpoint user management, diverifikasi `web/service/user.go` & semua
   controller). Kebijakan: pakai kredensial admin panel terenkripsi (§15.1) —
   mohon konfirmasi pembuatan password kuat khusus integrasi.
3. **Share link per protokol** — cukup subscription URL, atau wajib link
   `vless://` dll. untuk semua protokol yang dijual?
4. **Bahasa copy Telegram** — cukup `id-ID`, atau perlu i18n multi-bahasa?
5. **Domain & TLS webhook** — **RESOLVED (v1.2)**: reverse proxy = **Nginx**
   (bukan Caddy). Domain `bot-xui.kentangtechstore.com` + sertifikat TLS via
   certbot/Let's Encrypt (atau sertifikat yang sudah ada di server).
6. **Restart Xray via bot** (admin) — diizinkan atau tidak di v1?
7. **Pricing** — seed awal dari `server_price.json` reference, atau set harga
   baru via command admin saat go-live?

---

## 24. Glossary

| Istilah | Definisi |
|---------|----------|
| Inbound | Entitas listener Xray (port + protokol) di panel |
| Client  | Akun VPN dalam sebuah inbound (email + credential) |
| Subscription URL | URL `https://{sub-host}/{subPath}/{subId}` untuk import config otomatis |
| Share link | URI `vless://`, `vmess://`, `trojan://` yang bisa diimport manual |
| Ledger  | Catatan mutasi saldo immutable (audit) |
| Idempotency | Efek samping hanya terjadi sekali walau request diulang |

---

## 25. Lampiran

### A. Sumber Referensi Kode
- X-UI API routes: `web/controller/api.go`, `web/controller/inbound.go`
- X-UI auth: `web/controller/index.go` (`POST /login`), `web/controller/base.go`
  (`checkLogin`)
- X-UI subscription: `sub/sub.go`, `sub/subController.go`
- Reference bot Python: `/home/rusmanadodi/kentangtech-xui/client-vpn`
  (`main.py`, `services/xui_client.py`, `services/order_service.py`,
  `services/topup_service.py`, `helpers/webhook.py`, `models/*`)
- Reference handlers: `client-vpn/handlers/*.py` (start, menu, buy, account,
  admin, topup, history, trial, renewal, help)
- Panel user (single-admin): `database/model/*`, `web/service/user.go`,
  `web/controller/index.go`
- Governance: `AGENTS.md` (root)

### B. Kontrak Error Payload (API internal bot)
```json
{ "code": "INSUFFICIENT_BALANCE", "message": "Insufficient balance for this plan." }
```

---

## 26. REST API Convention — Versioned `/api/v1`

> Berlaku untuk seluruh endpoint HTTP bot. Webhook Telegram & payment,
> health API — semuanya di bawah prefix `/api/v1`; hanya probe infra `GET
> /health` yang berada di luar (alias).

### 26.1 Prinsip Versioning
- Semua endpoint API bisnis & integrasi di bawah prefix **`/api/v1/`**.
- Kontrak yang sudah dipublikasikan **tidak boleh breaking** dalam satu versi
  (CDD §2.4). Perubahan breaking → `/api/v2`; endpoint lama di-*deprecate*
  minimal satu rilis sebelum dihapus.
- Versi di URL, bukan header — eksplisit, mudah di-debug.

### 26.2 Penamaan Resource (pronounceable & readable)
| Aturan | Contoh ✅ | Hindari ❌ |
|--------|-----------|------------|
| Resource = kata benda jamak, lowercase | `/api/v1/servers`, `/api/v1/orders` | `/api/v1/server`, `/api/v1/getOrders` |
| Sub-resource `/{parentId}/{child}` | `/api/v1/users/{id}/orders` | `/api/v1/userOrders` |
| Aksi non-CRUD → `/{resource}/{id}/action` | `/api/v1/servers/{id}/health` | `/api/v1/checkServerHealth` |
| CRUD lewat HTTP method | `POST` create, `PATCH` update parsial | `POST /api/v1/updateServer` |
| ID eksplisit (numerik internal / string external ref) | `/api/v1/orders/KTS-XXXXXXXX-VPN` | ID campur baur |
| Webhook integrasi → `/webhooks/{domain}` | `/api/v1/webhooks/payments` | `/api/v1/kts-vpn` |
| Query: `page`, `limit`, `status`, `sort`, `q` | `?page=2&limit=10&status=completed` | Filter di body GET |

### 26.3 Semantik Method & Status
| Method | Semantik | Sukses | Gagal umum |
|--------|----------|--------|------------|
| GET    | baca (list/detail) | 200 | 400, 404 |
| POST   | buat resource / aksi | 201 (create) / 200 (aksi) | 400, 401, 403, 409, 422 |
| PATCH  | update parsial | 200 | 400, 404, 409 |
| DELETE | hapus | 204 | 404, 409 |
| —      | validasi input | — | 422 |
| —      | rate limit | — | 429 |

### 26.4 Envelope Respon & Error
```json
// sukses (list):   { "data": [...], "meta": { "page": 1, "limit": 10, "total": 42 } }
// sukses (detail): { "data": { ... } }
// error:           { "code": "INSUFFICIENT_BALANCE", "message": "Insufficient balance for this plan.", "details": { ... } }
```
- `code` machine-parseable, uppercase snake, **English** (AGENTS.md §1.3).
- Webhook pihak ketiga (Telegram/KentangTech) tidak memakai envelope —
  mengikuti kontrak masing-masing.

### 26.5 Katalog Endpoint v1
| Method | Path | Auth | Fungsi | Milestone |
|--------|------|------|--------|-----------|
| GET | `/api/v1/health` | — | Liveness/readiness bot | M0 ✅ |
| POST | `/api/v1/webhooks/telegram` | secret token header | Ingestion update Telegram (`WEBHOOK_PATH`, configurable) | M3 |
| POST | `/api/v1/webhooks/payments` | HMAC `X-Webhook-Signature` (+ `X-Webhook-Event`, dedup `X-Webhook-Id`) | Callback `pg.charge` pembayaran QRIS | M5 ✅ |
| POST | `/api/v1/servers` | `X-API-Key` | Registrasi server X-UI (porting `server_registration.py`) | ✅ v1.49 |
| GET | `/api/v1/servers` | `X-API-Key` | List server (admin view, tanpa kredensial) | ✅ v1.49 |
| GET | `/api/v1/servers/{id}` | `X-API-Key` | Detail server (tanpa password/username) | ✅ v1.49 |
| PATCH | `/api/v1/servers/{id}` | `X-API-Key` | Update server (password di-seal ulang) | ✅ v1.49 |
| DELETE | `/api/v1/servers/{id}` | `X-API-Key` | Hapus server (guard: 409 bila ada client) | ✅ v1.49 |
| GET | `/api/v1/servers/{id}/health` | `X-API-Key` | Health per server (probe live) | ✅ v1.49 |
| GET | `/api/v1/orders` (v1.1, admin) | `X-API-Key` | List/statistik order (stats + recent bounded) | ✅ v1.49 |
| GET | `/api/v1/orders/{orderId}` | `X-API-Key` | Detail order | ✅ v1.49 |
| GET | `/api/v1/users/{telegramID}/orders` | `X-API-Key` | Riwayat order user (paged) | ✅ v1.49 |
| GET | `/api/v1/users/{telegramID}/clients` | `X-API-Key` | Akun VPN user (paged, tanpa kredensial) | ✅ v1.49 |
| POST | `/api/v1/payments/topups` (v1.1, admin) | `X-API-Key` | Trigger topup QRIS | ✅ v1.49 |

**Auth & enablement (v1.49):** seluruh endpoint di atas dijaga header
`X-API-Key` (constant-time) terhadap env `REST_API_KEY`. `REST_API_KEY` kosong
→ surface admin REST **tidak terdaftar** (404). Read server/client **tidak
pernah** mengekspos `password_enc`, `username`, `uuid`/`password`/`subId`/
`config_link`/`subscription_url`; `DELETE /servers/{id}` menolak 409 bila server
masih punya client (guard terhadap `ON DELETE CASCADE` di `vpn_clients`).

### 26.6 Aturan Implementasi
- Router memakai prefix grup `apiBase = "/api/v1"` (terpasang di M0).
- Tanpa kata kerja di path resource; aksi via method atau `/{id}/action`.
- Kebab-case hanya untuk resource multi-kata (mis. `payment-accounts`);
  prefer kata tunggal yang jelas & mudah diucapkan.
- Response selalu JSON (`application/json`), kecuali kontrak webhook pihak ketiga.

---

## 27. Changelog

| Versi | Tanggal     | Perubahan                                                                                                   |
|-------|-------------|-------------------------------------------------------------------------------------------------------------|
| v1.49 | 2026-08-18  | **Admin REST API §26.5 di-wire (keputusan user — endpoint deferred "nanti" selesai)**: guard `X-API-Key` (constant-time) terhadap env `REST_API_KEY`; `REST_API_KEY` kosong → surface tidak terdaftar (404). (1) `handler/http/api.go` — auth middleware `withAPIKey` + envelope §26.4 (`writeData`/`writeList`/`writeAPIError`) + seam `ServerAdmin`/`OrderAdmin`/`ClientReader`/`UserResolver`/`TopupTrigger` + `registerAdminAPI`; (2) `handler/http/servers_api.go` — `POST/GET/PATCH/DELETE /servers` + `GET /servers/{id}/health` (read **tanpa password/username**, DTO `serverDTO`; password di-seal ulang via `serversvc.UpdateServer`; `DELETE` guard 409 `SERVER_HAS_CLIENTS`); (3) `handler/http/orders_api.go` + `users_api.go` — `GET /orders` (stats + recent bounded), `GET /orders/{orderId}`, `GET /users/{tgID}/orders` + `/clients` (paged, clientDTO **credential-free** — tanpa uuid/password/subId/configLink/subscription_url); (4) `handler/http/topup_api.go` — `POST /payments/topups` (Quote → CreatePayment, amount = NET); (5) repo `server_rest.go` — `GetAdminByID`/`UpdateServer`/`DeleteServer` (guard count `vpn_clients` sebelum delete — mencegah cascade `ON DELETE CASCADE` menghapus akun user); (6) service `server_rest.go` — `GetAdminByID`/`UpdateServer`/`DeleteServer`/`CheckHealth` (seam `statusFactory`); (7) config `REST_API_KEY` (opsional) + wiring `main.go`/`shop.go` (bundle expose `OrderRepo`/`ClientRepo`/`UserRepo`). Test: handler httptest (401 missing/wrong, surface-off 404, servers CRUD 201/200/204/404/409, credential-free, orders/users paged, topup 201/422), service `server_rest_test.go` (password re-seal, invalid port, delete guard, health ok/down), repo `repo_server_rest_test.go` (integration: no-credential view, patch, delete guard). Semua hijau: build/vet/gofmt + `test -race` (handler/http, service/server); file < 250 baris. |
| v1.48 | 2026-08-18  | **M5 selesai — PG Aggregate charge + webhook HMAC (FR-06; kontrak nyata 015/013, bukan `autobuy-saldo`; keputusan user: amount = NET, gross-up di-handle gateway)**. (1) `internal/repository/kts` (baru): `signer.go` — canonical S2S `v1\n{apiKey}\n{ts}\n{nonce}\n{METHOD}\n{path}\n{hex_sha256(body)}` + `X-Signature: sha256=...` (001 §2.3) & `WebhookSignature` (HMAC raw body, 013 §2.2); `gateway.go` — `CreateCharge`/`ConfirmCharge`/`GetCharge` (poll), header `X-API-Key/X-Timestamp/X-Nonce/Idempotency-Key`, timeout eksplisit, mapping 409→duplicate / 404→not-found / 401→unauthorized, envelope `data` / `error{code,message}`; `types.go` — DTO schema-first (CDD §2.4). (2) Persistensi: migration `000007_payments_telegram` (kolom `payments.telegram_id` + index) + `PaymentRepo` (`Create`/`Get`/`SaveProviderRef`/`MarkFailed`/`MarkSettledTx` — conditional pending→terminal, kunci anti double-credit; `schema_test` disinkron). (3) `service/topup` rewrite: **`StubGateway` DIHAPUS** — `CreatePayment` = persist row dulu (orderId `tp_*` → `orders`+`payments`) → create charge (NET) → confirm → `checkoutUrl` (pola Phase 3); `ApplySettlement` = kredit NET lokal + mark dalam SATU transaksi (webhook & poll race aman); `succeeded`→kredit, `failed`/`expired`→tanpa kredit; notif user + grup admin best-effort (adapter `cmd/bot/topup_notify.go`). (4) Webhook `POST /api/v1/webhooks/payments` (sebelumnya 501): verifikasi `X-Webhook-Signature` (raw body, constant-time) → 403; branch `X-Webhook-Event: pg.charge`; dedup `X-Webhook-Id` Redis SETNX (TTL 7 hari); settle → **2xx hanya setelah durable**, 5xx → gateway retry. (5) Config + wiring: `KTS_BASE_URL`/`KTS_API_KEY`/`KTS_SECRET` (required, fail-fast) + `KTS_CHARGE_TTL_MIN` (default 24 jam); `buildShop` wire kts client + notifier; `main.go` Options. (6) Teks: `TopupPaymentText` (link checkout QRIS), `TopupSettledText`, `AdminTopupNoticeText`. Test: signer vector, gateway httptest (201/202/404/409/401), topup service (Quote/CreatePayment/ApplySettlement), webhook (signature valid/salah, event salah, duplikat, settle sukses/expired), handler telegram topup. Semua hijau: build/vet/gofmt + `test -race` (kts/topup/http/config), semua file < 250 baris. Poll fallback dilimpahkan ke gateway (reconciler 015 Phase 6.1); `kts.GetCharge` siap utk poll manual. |
| v1.47 | 2026-08-17  | **Purchase debit-first + auto-refund (FR-04 AC-1; keputusan user — parity renew v1.37; menggantikan rencana worker reconciliation Phase 3)**. Alur baru: `prepare client` (read-only: inbound + kredensial + share link, TANPA sentuh panel) → **insert row `vpn_clients`** → **debit saldo atomik** (guard `balance >= harga`, tidak pernah minus) → **commit panel** (`addClient`); gagal commit → **refund otomatis** (Credit + ledger + hapus row), order `failed` — akun aktif tanpa bayar mustahil; gagal sebelum debit → `failed` bersih. (1) `domain.PreparedClient` (record bot-side + param commit); (2) `service/server/provision.go` (baru) — split `provisionClient` → `PrepareClient` + `CommitClient`; `CreateClient`/`CreateTrialClient` rebuild dari pola ini; (3) interface `PanelGateway`/`ClientStore` (`service/order/order.go`) di-update; (4) `service/order/purchase.go` rewrite; (5) test di-update (asersi urutan + commit-failure → refund + hapus row), `provision_subid_test.go` baru (fix bug: `prepareClient` tidak mengisi `SubID` → sub URL FR-13 kosong). Semua hijau: build/vet/gofmt + `test -race` (domain/order/server/telegram), file < 250 baris. |
| v1.44 | 2026-08-12  | **Konsistensi brand ditutup (3 catatan review v1.43, keputusan user)**: (1) **Ringkasan konfirmasi** kini ber-brand — `BuyConfirmText`, `RenewConfirmText`, `TrialConfirmText` dibuka `BrandHeader()` (simetris dengan Ringkasan Top Up yang sudah ber-brand v1.43); (2) **Ejaan brand diseragamkan** — header ekspor `.txt` `=== AKUN VPN KENTANGTECH ===` → `=== AKUN VPN KENTANG TECH ===` (dari `BrandName`), `HomeText` sambutan `/start` "KentangTech VPN Bot" → `KENTANG TECH VPN Bot` (dari `BrandName`), `HelpInfoText` (`help:info`) "KentangTech VPN Bot" → `%s VPN Bot` via `fmt.Sprintf(BrandName)` (raw string dikonversi, import `fmt` ditambah) — tidak ada lagi ejaan legacy di copy user-facing; (3) **Pesan gagal ber-brand** — `BuyFailedText` + `TrialFailedText` dibuka `BrandHeader()`. Test: `menu_test.go` `TestHomeText` update `KentangTech`→`BrandName`; `menu_brand_test.go` diperluas — 5 template baru (3 ringkasan konfirmasi + 2 pesan gagal) masuk daftar HasPrefix banner + `TestBrandSpelling_ThenSingleBrandEverywhere` baru (txt/home/info memuat `BrandName`, TIDAK pernah `KentangTech`/`KENTANGTECH`/`KENTANG TECH STORE`). Semua hijau: build/vet/gofmt/`test -race` (21 paket) + golangci-lint, semua file < 250 baris. Review: tanpa bug (1 nit komentar usang v1.43→v1.43/v1.44, diperbaiki). E2E staging v144: verifikasi brand di alur nyata (bawah). |
| v1.46 | 2026-08-12  | **FR-13 Subscription URL — gap terakhir (bagian (a) subscription URL; bagian (b) share link per protokol sudah selesai v1.26–v1.33)**. Sub server panel (Opsi 2 — domain sama dengan panel, port beda; default subPort 2096) sudah menerima `subId` di setiap client sejak provisioning (beli & trial), tapi bot tidak pernah menyimpannya. Implementasi: (1) config `internal/config/subscription.go` — `SUB_ENABLED` (default false) / `SUB_BASE_URL` / `SUB_PATH` (default `/sub`, trailing slash dinormalisasi) / `SUB_JSON_ENABLED` + `SUB_JSON_PATH` (default `/json`); validate: saat enabled `SUB_BASE_URL` wajib http(s) valid; (2) migrasi `000006` — `vpn_clients.sub_id` + `subscription_json_url` (nullable, tanpa index — bot tidak pernah query by subId); akun lama kosong = **legacy gap terdokumentasi (keputusan user: tanpa backfill)**; (3) domain `PanelClient.SubID` + `VPNClient.SubID/SubscriptionJSONURL`; `serversvc.provisionClient` kini mengembalikan `pc.SubID = spec.SubID` (nilai yang sama yang dikirim ke panel); (4) `ordersvc.SubLinks` (value type, join URL kanonik — robust thd trailing slash; `SetSubLinks` setter agar variadic notify di `New` tetap backward-compatible) → `Purchase` & `CreateTrial` persist `sub_id` + `subscription_url` + `subscription_json_url` (JSON hanya bila `SUB_JSON_ENABLED` — gating di wiring `cmd/bot/shop.go`); `toClientRow` memetakan field baru; (5) **Ekspor .txt** (keputusan user: URL HANYA di ekspor — konsisten v1.36): `AccountTXTContent` menambahkan blok `Subscription URL (auto-update)` + `Subscription JSON (Clash/Meta)`; akun legacy (kolom kosong) dilewati; hint "Cara pakai" update (import Config Link atau Subscription URL). Test baru: config (defaults off, enabled-valid, invalid base URL / path), `provision_subid_test.go` (SubID terkirim ke panel == yang dikembalikan), ordersvc (client row membawa SubID/SubscriptionURL/JSONURL + join kanonik), export (blok sub tampil bila ada / absen untuk legacy). Semua hijau: build/vet/gofmt/`test -race` + golangci-lint, semua file < 250 baris. Review: 2 cleanup diterapkan — guard join `//` saat path bare "/" (SubLinks public type), closure JSONPath di shop.go diganti variabel lokal. |
| v1.45 | 2026-08-12  | **Worker health check (PRD §17) + worker trial cleanup (gap audit — 2 kolom yang selama ini ada tapi tidak pernah diisi worker)**: (1) `internal/service/health` — `RunOnce` ping tiap panel aktif (`GET /xui/API/server/status`, per-server timeout `XUI_API_TIMEOUT`) → tulis `health_status` (`ok`/`down`) + `last_health_check` (`server_health.go` `ListHealthTargets`/`UpdateHealth`); **server mati tidak dijual** — `ListBuyable` kini `IS DISTINCT FROM 'down'` (default kolom `'unknown'` tetap dijual agar boot pertama tidak hilang; filter HANYA mengecualikan `down`); (2) `internal/service/trialcleanup` — `RunOnce` list kandidat (trial aktif, `expires_at <= now()`) → group per server → `serversvc.DisableClients` (satu `GetInbounds` per panel, spec raw client di-patch `enable=false` — kuota/ipLimit/flow dipertahankan; kunci updateClient per-protocol di-extract dari spec: vless/vmess→id, trojan→password, ss→email, hysteria→auth) → **baru** `MarkTrialExpired` (is_active=false + is_expired=true; row tetap di Akun Saya, badge Trial + status Expired); **panel gagal → DB tidak ditandai** (retry sweep berikutnya); client sudah hilang di panel = sukses; (3) wiring `cmd/bot`: kedua worker via `job.NewIntervalWorker` (sweep pertama langsung, ctx cancel + WaitGroup drain — pola sama traffic sync), sweep timeout fleet-sized; config baru `HEALTH_CHECK_ENABLED/INTERVAL_SEC` (default true/60s) + `TRIAL_CLEANUP_ENABLED/INTERVAL_MIN/BATCH` (default true/15m/50) di `config/health.go` + `config/trialcleanup.go` (Defaults dipindah ke `config/defaults.go` agar config.go < 250 baris; main.go tetap 248 baris). Test: `health_test.go` (4: all-ok, down-isolasi, build-fail→down, no-target), `trialcleanup_test.go` (4: disable+mark, panel-fail tidak menandai, no-candidate, satu server gagal→lain tetap), `disable_test.go` (5: key vless/trojan/hysteria, missing skip, panel fail), `repo_health_test.go` (3 integrasi: target/status persist, buyable filter down-excluded, kandidat trial + mark), config_test diperluas. Review: 2 perbaikan diterapkan — error `MarkTrialExpired` kini dihitung sebagai kegagalan server, `DisableClients` early-return saat emails kosong. **Fix E2E (ditemukan uji fake server mati)**: budget per-server XUI habis oleh connect timeout panel mati → `UpdateHealth`/`MarkTrialExpired` dipanggil dengan context kadaluarsa → status `down` TIDAK pernah tersimpan (server mati tetap dijual). Solusi: DB write memakai context terpisah — `healthWriteTimeout` (10s, parent ctx) untuk health, parent ctx untuk mark trial; error wrap dobel dibersihkan. Test baru: `TestRunOnce_GivenPanelExhaustsBudget_ThenStatusStillPersisted` + `TestRunOnce_GivenPartialDisableThenTimeout_ThenOnlyConfirmedMarked`. Verified staging: fake server `192.0.2.1:1` → `health_status='down'` terpersist, JP hilang dari ListBuyable/buy menu. Semua hijau: build/vet/gofmt/`test -race` + golangci-lint, semua file < 250 baris. |
| v1.43 | 2026-08-12  | **Brand pada template notifikasi (keputusan user — parity legacy reference `notification_service.py`)**: `service/telegram/menu_brand.go` baru — `BrandName = "KENTANG TECH"` (eksplisit BUKAN "KENTANG TECH STORE" — brand lama legacy) + `BrandHeader()` = `🏪 KENTANG TECH` + separator `━━━`. **Pengecualian icon khusus brand** (keputusan user "1 icon"): banner brand adalah SATU-SATUNYA emoji di body copy — sisa icon policy (body emoji-free, icon hanya di tombol navigasi) tetap berlaku, didokumentasikan di header `menu_brand.go` + README. **Diterapkan ke 7 template pesan transaksi** (scope "Semua pesan transaksi"): `AdminOrderNoticeText` (notice grup admin FR-04 v1.41), `ExpiryNotifyText` (reminder FR-09), `BuySuccessText`, `RenewSuccessText`, `TrialSuccessText`, `TopupSummaryText`, `TopupPaymentText` — semua dibuka dengan `BrandHeader()+"\n\n"` (tanpa separator ganda). **Tidak diubah** (di luar scope): menu intro (HomeText/TopupMenuText/TrialMenuText), ringkasan konfirmasi beli/renew/trial (BuyConfirmText/RenewConfirmText/TrialConfirmText), pesan gagal (BuyFailedText/TrialFailedText), header ekspor `.txt` (masih `=== AKUN VPN KENTANGTECH ===`), HelpInfoText/HomeText (masih "KentangTech VPN Bot") — tercatat sebagai kandidat konsistensi brand lanjutan (review). Test baru `menu_brand_test.go`: `TestBrandHeader_ThenIconNameAndSeparator` (🏪 + nama + separator, tanpa STORE, BrandName exact) + `TestTransactionalTemplates_ThenBrandBannerPresent` (semua 7 template HasPrefix `🏪 KENTANG TECH`, mengandung separator, tidak pernah "KENTANG TECH STORE") — semua test lama Contains-based tetap lulus. Semua hijau: build/vet/gofmt/`test -race` (21 paket) + golangci-lint, semua file < 250 baris. Review: tanpa temuan blokir (3 catatan konsistensi minor non-blokir disampaikan ke user). E2E staging v143: verifikasi brand di pesan nyata (bawah). |
| v1.42 | 2026-08-12  | **UX: semua keyboard sub-menu dari vertikal 1-1-1-1 → zigzag 2-1-2-1-2** (`service/telegram/menu_rows.go` baru, keputusan user): helper `packRows` menata tombol per pola "pertama 2, selanjutnya 1, selanjutnya 2, dst." — baris terakhir mengambil sisa (1 bila hanya 1 tombol tersisa), **urutan tombol dipertahankan**; `backBtn` = varian single-button untuk packer (backRow kini delegasi ke backBtn — tanpa duplikasi). **Diterapkan ke semua keyboard sub-menu** (list picker, aksi, konfirmasi): country/inbounds/plans (buy), renew clients/plans, trial servers, admin menu (9 tombol → [2,1,2,1,2,1] = 6 baris), admin price/servers list, account list detail (pager + home tetap baris sendiri), account detail ([Traffic,Config],[Convert],[Export,Delete],[Kembali]), success buy/trial, semua layar konfirmasi (Konfirmasi + Batal satu baris), nav 2-tombol (config/convert/traffic/delete/history-detail/account-empty → satu baris). **Tidak diubah** (sudah 2-kolom / full-width sengaja): HomeKeyboard, TopupKeyboard quick-pick, HelpMenu/Disclaimer, HistoryEmpty, keyboard help/Tos nav. Handler tidak terpengaruh (semua baca keyboard secara flat — assertButtonInMarkup). Test: `menu_rows_test.go` baru table-driven (lebar baris N=0–9 + urutan tombol dipertahankan) + update posisi row di test admin/config/yaml/traffic/delete/account-list. Semua hijau: build/vet/gofmt/`test -race` (21 paket) + golangci-lint, semua file < 250 baris. Review: tidak ada temuan blokir (saran DRY backRow→backBtn diterapkan). **E2E staging v142 (verifikasi layout nyata)**: deploy `/tmp/bot-staging-v142` + 7 callback via webhook (buy:menu, admin:menu, account:menu, trial:menu, buy:country:ID, topup:menu, renew:menu) → semua `ok`, handler routing tidak terpengaruh layout; layout diverifikasi via dump keyboard dengan data nyata staging — BuyCountries 4 negara: `[🇮🇩,🇸🇬] [🇯🇵] [🇨🇳,🏠]`; Inbounds 3: `[vless,vmess] [trojan] [⬅️]`; AdminMenu 9: `[Harga,Server] [Broadcast] [Ban,Unban] [Adjust] [Statistik,Audit] [🏠]` (6 baris); AccountDetail 6: `[Traffic,Config] [Convert] [Ekspor,Hapus] [⬅️]`; confirm 2-tombol: `[Konfirmasi,⬅️]`/`[Ya, Hapus,Batal]`/`[Refresh,⬅️]` satu baris; Topup & HelpMenu tetap 2-kolom (tidak diubah). |
| v1.41 | 2026-08-12  | **AC FR-04 terakhir ditutup — notifikasi order sukses ke grup admin (`NOTIFICATION_GROUP_ID`)**: gap `NotificationGroupID` yang ada di config sejak M3 tapi tidak pernah dipakai kini di-wiring. **Service** (`service/order/notify.go` baru): seam `OrderNotifier` + payload `OrderNotice` (order id, tipe, user label, plan label, nominal, email akun, saldo setelah, expiry baru) — `New` jadi **variadic** `New(..., notify ...OrderNotifier)` (nil-safe, semua call site & test lama tetap kompil). `Purchase` & `Renew` memanggil `notifyCompleted` **setelah `order.Complete` + Save** (hanya order berbayar yang completed; **trial dikecualikan** — akun gratis, keputusan: notice admin untuk order berbayar saja); **best-effort** — notifikasi gagal TIDAK pernah menggagalkan order (test: notifier error → order tetap completed). **Presentation** (`service/telegram/menu_order_notice.go` baru): `AdminOrderNoticeText` menerima primitives (tanpa import ordersvc — service telegram tetap bebas edge service-to-service), reuse `orderTypeLabel` (Beli VPN/Perpanjang), body emoji-free (icon policy), menampilkan `Aktif sampai` (parity `RenewSuccessText`). **Wiring** (`cmd/bot/notify.go` + `shop.go`): adapter `orderNotifier` (SendMessage ke `NOTIFICATION_GROUP_ID`, error di-log tanpa memengaruhi order); gate **`!= 0`** — Telegram group chat ID **negatif** (`-100...`), fix review v1.41: gate `> 0` awal akan membuat fitur mati total di produksi. Test baru: `service/order/notify_test.go` (purchase sukses → **1×** notice payload lengkap; panel gagal → 0; debit gagal → 0; renew sukses → 1×; renew panel gagal → 0; **trial → 0**; nil notifier → no panic; notifier error → order tetap completed), `menu_order_notice_test.go` (label purchase/renewal + semua field). Semua hijau: build/vet/gofmt/`test -race` (21 paket) + golangci-lint, file < 250 baris. Docs: changelog, README, SYSTEM_MAP, UAT. **E2E staging penuh (v141, `.env` diisi `NOTIFICATION_GROUP_ID` = `REQUIRED_GROUP_ID` — grup join, user test = id admin, email tampil penuh per keputusan user)**: (1) **beli sukses** → `KTS-VNB8XC47-VPN` purchase completed debit 4000 (42000→38000), notice terkirim ke grup join tanpa error; (2) **beli sukses kedua** → `KTS-8WG5AEWR-VPN` completed (38000→34000), notice terkirim; (3) **renew sukses** → `KTS-2QSSDSXN-VPN` renewal completed debit 4000 (34000→30000), expiry client 12 `2026-08-27 → 2026-09-11` (+15 hari dari sisa, tanpa double-count), notice `Perpanjang` terkirim; (4) **trial ditolak** (counter 2/2) → TIDAK ada order trial baru, TIDAK ada notice; (5) **beli gagal saldo kurang** (debit ADJ 29000 → saldo 1000, beli SG 90 harga 23000) → TIDAK ada order baru, TIDAK ada notice, lalu kredit ADJ 29000 → saldo balik 30000 (ledger ADJ berpasangan, presisi penuh); (6) **masking (keputusan user: email tampil penuh)** — test baru `TestAdminOrderNoticeText_GivenSensitiveFields_ThenNeverLeaked`: teks notice TIDAK pernah mengandung uuid/password/vless://vmess://trojan:///sub/config — payload `OrderNotice` memang tidak membawa field kredensial (hanya order id, label, nominal, email, expiry, saldo); notice gagal kirim di-log tanpa menggagalkan order (best-effort). **Catatan deploy**: `NOTIFICATION_GROUP_ID` kini terisi di `.env` staging (nilai grup join) — bot restart memakai nilai itu. |
| v1.40 | 2026-08-12  | **Gap FR-11 ditutup — manajemen server + statistik + audit log (FR-11 LENGKAP, keputusan user — gap M6 selesai)**: (1) **Manajemen server** — `admin:server` → daftar semua panel (aktif+nonaktif) → detail per server (`admin:server:{id}`) → **Toggle Buka** (`admin:server:open:{id}`, flip `is_open` — negara hilang dari pilihan Beli/Trial) & **Toggle Aktif** (`admin:server:active:{id}`, flip `is_active` — dikecualikan dari sync traffic); **Tambah Server** (`admin:server:add`) = FSM 6 langkah (nama → host → port → username → password → negara, opsional `CODE,FLAG`) menumpuk di FSM Redis `srvadd:*` (TTL 10 mnt); input invalid → re-prompt langkah yang sama; langkah terakhir **men-arm `srvadd:confirm:<encoded>`** dan confirm **memverifikasi state persis sebelum `AddServer`** — tap ganda hanya membuat SATU server (parity idempotence saldo v1.39; fix review v1.40: arm-state awalnya check-then-clear = TOCTOU bila webhook concurrent). Password **disegel AES-256-GCM** di serversvc (`password_enc`), **tidak pernah di-echo** ke chat maupun audit. (2) **Statistik** — `admin:stats` → `OrderRepo.Stats` agregasi SQL satu tx (total/today orders, revenue `final_amount` order `completed` total+today, breakdown status completed/failed/pending/processing/cancelled/refunded, total user, **client aktif** `is_active AND NOT expired`) + `admin:recent` 10 order terbaru bounded. (3) **Audit log** — migrasi `000005` tabel `admin_audit_log` immutable (admin_id, action, target, detail, created_at + **index created_at DESC** §1.7); `AuditRepo.Record/Recent(limit)`; **setiap aksi admin merekam audit** (SetPrice, SetEnabled, ReloadPricing, BanUser, UnbanUser, AdjustBalance, Broadcast, AddServer, ToggleServerOpen/Active) — siapa (admin id), aksi, target, detail; `admin:audit` menampilkan 15 baris terbaru. **Layer**: repo `server_admin.go`/`order_stats.go`/`models_audit.go` (split §1.1); serversvc `admin.go` (AddServer + toggles — `Store` diperluas); adminsvc `server_stats.go` (seam `ServerOps`/`StatsStore`/`AuditStore`); menu `menu_admin_servers.go`/`menu_admin_stats.go`/`menu_admin_audit.go`; handler `admin_servers.go`+`admin_server_add.go`+`admin_server_draft.go` (split 326→3 file §1.1) / `admin_stats.go`; routing order fix (prefix open/active SEBELUM generic server); wiring `buildShop`. **Fix review**: (a) srvadd confirm arm-state; (b) dead params `adminServerDraft(ctx,userID,…)` dihapus; (c) setelah add → list di-refresh (bukan keyboard nil); (d) error add generik (bukan echo raw); (e) `menu_admin.go` 249 baris → split `menu_admin_ban.go` (batas 250 §1.1). Test baru: `server/admin_test.go` (AddServer enkripsi + ID, toggle, dedup host/port), `admin/server_stats_test.go` (stats delegate, audit nil-safe, toggle/add record audit), `handler/admin_servers_test.go` (list/detail/toggle/add 6 langkah + confirm + **tap ganda → AddServer 1×**, stats, audit), `repo_admin_test.go` integration (audit record/recent + index, server admin ops, Stats agregasi + RecentOrders bounded), update menu_admin_test (tombol baru). Semua hijau: `go build`/`vet`/gofmt/`test -race` (20 paket) + golangci-lint, semua file < 250 baris. Docs: changelog, README, SYSTEM_MAP. E2E staging menyusul (toggle server, add server, stats, audit). |
| v1.39 | 2026-08-12  | **Adjust saldo admin (FR-11, keputusan user — gap M6 ditutup)**: menu admin baru **`Adjust Saldo`** (`admin:saldo` → `admin:saldo:kredit`/`admin:saldo:debit`). Alur FSM 2 langkah (pola ban/broadcast): ketik **Telegram ID** → lookup user (`LookupUser`, unknown → "belum terdaftar" + FSM cleared) → ketik **nominal rupiah** (`parseMoney`, invalid → re-prompt) → layar konfirmasi (`AdminSaldoConfirmText` + keyboard `admin:saldo:confirm:{kredit|debit}:{tgid}:{amount}`) → eksekusi `adminsvc.AdjustBalance`. **Idempotence & copy (fix review v1.39)**: step nominal kini **men-arm state `saldo:confirm:{kind}:{tgid}:{amount}`** (bukan clear) dan callback confirm **memverifikasi FSM state sama persis dengan payload sebelum eksekusi** — tap ganda / retry Telegram pada tombol Konfirmasi TIDAK pernah menjalankan `AdjustBalance` dua kali (tap kedua → "Konfirmasi sudah kedaluwarsa"); re-prompt nominal invalid menampilkan **label user asli** (bukan generik). **Service**: `AdjustBalance(ctx, tgID, amount, credit)` (file `service/admin/adjust.go`) — resolve tgID → **primary key** user (`Get`; `gorm.ErrRecordNotFound` → `ErrUserNotFound`), amount ≤ 0 ditolak, lalu `Credit`/`Debit` atomik **yang SAMA dengan jalur order** (SQL guard, `balance_transactions` immutable, `total_spent` ikut di-debit) dengan **ledger ref `ADJ-<random>`** (traceable, beda dari order KTS-*). Debit > saldo → `postgres.ErrInsufficientBalance` → answer ramah "Saldo user tidak mencukupi". `AdminOps` + `adminsvc.UserStore` interface diperluas (Credit/Debit — usersvc.Service sudah implement). Test baru: `service/admin/adjust_test.go` (kredit → PK+ADJ- ref, debit, unknown → ErrUserNotFound tanpa move, insufficient, zero-amount), `handler/telegram/admin_saldo_test.go` (menu, arm FSM, ID input staged, unknown cleared, nominal input confirm + keyboard payload + **state confirm di-arm**, **tap ganda → eksekusi sekali + answer kedaluwarsa**, re-prompt nominal invalid dengan label user, confirm kredit/debit eksekusi, insufficient answer, non-admin ditolak), update `TestAdminMenuKeyboard` (6 tombol + tombol Adjust Saldo). Semua hijau: `test -race` 20 paket, golangci-lint, gofmt, file < 250 baris. Docs: changelog, README (Belum di M6 dikurangi), SYSTEM_MAP, UAT. **E2E staging (v139)**: alur webhook nyata — kredit 10000 (`ADJ-ce5dbbce`) saldo 37000→47000, **tap ganda confirm → 0 eksekusi kedua** (1 baris ledger), debit 5000 (`ADJ-3d869b0f`) saldo 47000→42000 — presisi penuh, FSM transisi `saldo:confirm:{kind}:{id}:{amount}` → clear setelah eksekusi. |
| v1.38 | 2026-08-12  | **Fix renew panel "empty client ID" + presisi spec (ditemukan E2E staging v1.37)**: (1) **`serversvc.RenewClient` dirombak** (file baru `service/server/renew.go`, dipindah dari server.go utk §1.1) — root cause: x-ui `updateClient` **mengganti seluruh objek client** dan **memvalidasi kredensial per-protocol** (web/service/inbound.go `UpdateInboundClient`: vless/vmess→`id`, trojan→`password`, shadowsocks→`email`, hysteria→`auth`; `newClientId == "" → "empty client ID"`), sedangkan RenewClient lama mengirim spec kosong `{email, enable, expiryTime}` → **selalu gagal** (dan andai lolos akan **menghapus kuota/ipLimit/flow** client di panel). Sekarang RenewClient: `panelFactory` (seam testable, sama dgn delete) → `GetInbounds` → **cari client by email di settings JSON tiap inbound** (`findClientInbound`/`clientSpecInInbound` — spec penuh client saat ini adalah source of truth) → hanya ubah `enable=true` + `expiryTime` baru → `updateClient` dengan **spec penuh** (kredensial + `totalGB` + `limitIp` + `flow` + `subId` + `reset` dipertahankan) + re-set kredensial per-protocol (`setClientCredential`). (2) **Kunci panel per-protocol disatukan** — `domain.PanelClientKey(protocol, uuid, password, email)`: vless/vmess→UUID, trojan/hysteria→password (auth), **shadowsocks→email** (x-ui memakai email sbg kunci ss, bukan password) — dipakai renew (order svc) dan delete (handler account_delete.go; **fix latent: hapus akun ss sebelumnya mengirim password ≠ email → delClient gagal**). Interface `inboundLister` + `UpdateClient` (fakePanel ikut). Test baru: `server/renew_test.go` (vless spec penuh dipertahankan + expiry di-bump, trojan password + re-enable expired, ss key=email + password tak hilang, client tak ketemu → error tanpa updateClient), `domain/panel_key_test.go` (6 mapping), order `renewClientID` (vless→uuid, ss→email, trojan→password). Semua hijau: `test -race` 20 paket, golangci-lint, gofmt, file < 250 baris. Docs: changelog, README, SYSTEM_MAP. |
| v1.37 | 2026-08-12  | **Renew paid-only + idempotence + saldo presisi (keputusan user)**: (1) **Renew hanya akun PAID** (FR-05) — trial account `is_trial=true` ditolak di service (`ErrTrialNotRenewable`, sebelum order/debit/panel) dan **disaring dari UI** (handler `renewableClients`: renewMenu/renewPlans/clientForConfirm; callback renew ke akun trial = "Akun tidak ditemukan."). (2) **Idempotence order & renew (v1.37)** — guard DB `OrderRepo.FindInFlight` (order user+type masih `pending/processing`) → `ErrOrderInFlight` ("masih diproses") di `Purchase` & `Renew`; eksekusi duplikat (retry Telegram / tap ganda) TIDAK pernah membuat order/debit kedua. (3) **Alur uang renew dirombak: debit-first + auto-refund** — urutan baru: guard → order `pending→processing` (referensi client sejak awal) → **debit atomik** (SQL `balance >= amount AND NOT banned` — saldo tidak pernah minus; panel TIDAK disentuh sebelum uang aman) → panel `RenewClient` → DB `UpdateExpiry` → `completed`. Gagal SETELAH debit (panel/DB) → **refund atomik `Credit` + ledger dengan orderID sama** (`Service.refund`) + order `failed`; gagal SEBELUM/Saat debit → tanpa panel, tanpa refund, order `failed` (insufficient → `ErrInsufficientBalance`). Purchase tetap panel-first (FR-04 AC-1). Test baru `order_renew_test.go` (7 kasus Given-When-Then: trial ditolak tanpa side-effect, in-flight purchase/renew ditolak, panel-error → refund orderID sama + failed + DB expiry tidak berubah, expiry-update error → refund, debit-error → tanpa panel/refund, debit-insufficient → tanpa panel/refund), `renew_test.go` handler (menu paid-only, trial callback not-found, pesan trial/in-flight), `repo_inflight_test.go` (pending/processing matched, completed/type/lain-owner ignored), update order_test.go (urutan debit-before-panel). Semua hijau: `test -race` 21 paket (termasuk repo integration), golangci-lint, gofmt, file < 250 baris. Docs: changelog, README, SYSTEM_MAP. |
| v1.36 | 2026-08-12  | **Revisi minor UI akun (keputusan user)**: (1) **label kredensial protocol-aware** — helper `accountCredential` di `service/telegram/menu_account.go`: **UUID** untuk vless/vmess, **Password** untuk trojan/shadowsocks (serta hysteria/hysteria2 — auth secret), sesuai field client alireza0/x-ui (`id` vs `password`); dipakai di `AccountDetailText`, `AccountTXTContent` (ekspor `.txt`, alignment `%-12s`), dan `AccountConfigText` (label `ID` → `UUID`). (2) **URL config hasil build TIDAK lagi tampil di halaman Detail Akun** — seksi `Config Link:` (+ fallback "belum tersedia" + hint import) dihapus dari `AccountDetailText`, diganti satu baris hint "Config lengkap (URL import): gunakan tombol Ekspor .txt.". (3) **URL dual juga dihapus dari view `Config V2Ray`** — `AccountConfigText` kini referensi parameter manual (server/domain/Port TLS/Port Non-TLS/kredensial protocol-aware/Network/Path atau Service Name + hint ekspor + tips; tanpa `URL Config TLS/NTLS`, tanpa fallback native `ConfigLink`, tanpa "cara pakai" yang stale); fallback native `ConfigLink` di `AccountConvertText` (ss/hysteria/reality) juga dibuang → note "tidak tersedia" + hint ekspor; tombol keyboard convert di-relabel `URL Config` → `Config V2Ray`. (4) **URL juga dihapus dari pesan sukses Beli/Trial** — `BuySuccessText`/`TrialSuccessText` (menu_shop.go/menu_trial.go) tidak lagi menerima argumen configLink; blok `Config Link:` + baris "Import link ini di aplikasi VPN…" diganti satu baris hint "Config lengkap (URL import): gunakan tombol Ekspor .txt di bawah." (keyboard sukses tetap menawarkan tombol `Ekspor .txt`); field `PurchaseResult.ConfigLink` yang kini mati dihapus dari struct (purchase.go, order/trial.go, fixture handler shop_test.go); copy footer list akun disesuaikan ("ekspor config (.txt)"). **URL full (dual TLS/NTLS) kini HANYA di `Ekspor .txt`** — detail, view `Config V2Ray`, dan pesan sukses Beli/Trial semuanya bersih dari URL. Test baru/update: detail vless (UUID-only, tanpa Password & tanpa Config Link), detail trojan (Password-only), TXT ekspor label UUID, config view `UUID :`. Semua hijau: build/vet/`test -race` 21 paket, golangci-lint, gofmt, file < 250 baris. Docs: changelog, README, SYSTEM_MAP. |
| v1.35 | 2026-08-12  | **M7 — Gap FR-08 ditutup ✅**: (1) **AC-1 detail lengkap** — `AccountDetailText` + `AccountTXTContent` kini menampilkan **Limit IP** (`ip_limit`, field sudah ada sejak M4) dan **traffic terpakai** (`trafficBytes(TrafficUsed)`; ekspor `.txt` memakai format gabungan `traffic / kuota GB`); (2) **AC-4 aksi tercatat di riwayat** — setelah hapus sukses (panel `delClient` + `DeleteOwned`), `ordersvc.RecordDeletion` menulis **order `order_type=deletion`** (status `completed`, amount 0, `account_email` + protocol, `completed_at` set) via `domain.NewDeletionRecord` (factory status final — tanpa FSM, tanpa migrasi; `orders.order_type` sudah TEXT). Riwayat FR-14 menampilkan tipe `deletion` → label **"Hapus Akun"** + nominal **"—"** (`orderTypeLabel`/`historyAmountLabel`); seam `OrderRunner.RecordDeletion` di-handler. Test baru: domain `NewDeletionRecord`/`ParseOrderType(deletion)`, service `RecordDeletion` (row type/status/zero-amount/email), handler delete (record dipanggil saat sukses, TIDAK saat panel gagal / DB delete gagal), view detail/TXT (Limit IP + traffic), history label (Hapus Akun + dash). Semua hijau: build/vet/test (domain/order/telegram/handler), golangci-lint, gofmt, file < 250 baris. Docs: PRD §13.4 + FR-08, README, SYSTEM_MAP, UAT v1.35. |
| v1.27 | 2026-08-11  | **Revisi path dinamis per inbound (v2Ray config)** — path hardcoded `/{protocol}` diganti **path asli dari API** (streamSettings inbound, terverifikasi di panel staging: VMess-WS→`/vmessws`, VLESS-WS→`/vlessws`, Trojan-WS→`/trojanws`, Trojan-gRPC→`trojan-grpc`, reality/ss/hysteria tanpa path): `InboundStream(streamSettings)` di `link_dual.go` mengekstrak network+path (ws→`wsSettings.path`, grpc→`grpcSettings.serviceName`, httpupgrade/xhttp); `provisionClient` menyimpan `InboundNetwork`+`InboundPath` di client row (migrasi `000004`: 2 kolom `text NOT NULL DEFAULT ''`; `domain.PanelClient`/`VPNClient` + `toClientRow` + purchase/trial copy). `DualConfigLinks` kini menerima network+path: **ws**→`type=ws&path=` (normalisasi slash, %2F), **grpc**→`type=grpc&serviceName=` (tanpa slash), **tcp/reality/ss/hysteria→empty pair → fallback ConfigLink native** (tidak ada lagi link ws palsu utk reality); **legacy row (network kosong) → tetap ws `/{protocol}`** (backward compat v1.26). UI config menampilkan `Network` + `Path`/`Service Name` dinamis. Test: link_dual 10 kasus (ws real path, slash-normalisasi, legacy, grpc vless/trojan/vmess, reality kosong, host kosong, remark), InboundStream parse fixture asli panel, convert mapping, provision stream captured, menu_config ws/grpc/legacy/reality. Semua hijau: `test -race` 20 package (termasuk schema migration 000004), lint, gofmt, file < 250 baris. |
| v1.26 | 2026-08-11  | **Config v2Ray per akun (dual TLS/non-TLS)** — port setia format reference `client-vpn` (`account_handler.py` + `client_service._build_connection_link`): `service/server/link_dual.go` (`ConfigPair` + `DualConfigLinks` — ws pair TLS **443** / non-TLS **80**, path `/{protocol}` nginx, network ws, host = `vpn_servers.host`; vless/vmess(JSON base64)/trojan saja — ss/hysteria tanpa varian ws); `ClientView.ServerHost` (join `s.host`); `service/telegram/menu_config.go` (`AccountConfigLinks`/`AccountConfigText` — detail konfigurasi lengkap: remarks, quota, domain, port TLS/Non-TLS, ID/Password, network, path + 2 URL + cara pakai + tips; fallback native `ConfigLink` bila host kosong/protokol non-ws; `AccountConfigKeyboard`); callback baru `account:config:{id}` (handler `accountConfig`, ownership `GetViewOwned`); tombol **Config V2Ray** (icon-free) di keyboard detail akun **dan** sukses Beli/Trial; ekspor `.txt` kini menyertakan kedua URL. **Catatan deployment**: non-TLS port 80 di staging saat ini 301 → HTTPS — UAT harus verifikasi apakah link NTLS perlu diubah (mis. port non-TLS panel langsung). Test: link_dual 6 kasus (vless/vmess/trojan dual, ss/hysteria kosong, host kosong, remark trial), menu_config text/keyboard/.txt, handler config flow 4 kasus (owned ws, non-ws fallback, unowned, malformed). Semua hijau: `test -race` 20 package, gofmt, lint, file < 250 baris. |
| v1.25 | 2026-08-11  | **M7 — Detail akun (config link) di UI end-user + ekspor `.txt`**: (1) **Gap FR-08 tertutup** — config link/share URI kini di-build bot sendiri (port generator dari `sub/subService.go`: `service/server/linkgen.go` + `link_vless/vmess/trojan/ss/hysteria.go` — murni fungsi string dari settings inbound + host server, **tanpa ubah panel** karena sub server staging disabled). `provisionClient` kini set `SubID` (panel menyimpannya) + generate `ConfigLink` (host dari `store.GetByID`); `domain.PanelClient.ConfigLink`; ordersvc simpan ke `vpn_clients.config_link` (fix review: `toClientRow` awalnya tidak memetakan field ini); `PurchaseResult.ConfigLink`. (2) **UI**: `BuySuccessText`/`TrialSuccessText` menampilkan config link + tombol `Ekspor .txt`; menu Akun Saya → tombol `Lihat Detail` per akun (`account:view:{id}`) → detail lengkap (server, protocol, email, UUID/password, expiry, config link) + tombol `Ekspor .txt` (`account:export:{id}`) → kirim dokumen `.txt` via `sendDocument` (`repository/telegram.SendDocument`, `InputFileUpload`; test wire-contract multipart). Handler `accountView`/`accountExport` ownership-check via `ClientRepo.GetViewOwned` (id + user_id). Refactor: parser callback shop.go → parse.go. **Fix review**: `toClientRow` ConfigLink (tanpa ini kolom DB tetap kosong → detail/export "belum tersedia"); nil-message guard `accountExport` (regression inaccessible-message); `GetViewOwned` pakai Scan konsisten. Test: linkgen 6 kasus fixture asli panel (vless reality/wss, vmess ws, trojan grpc, ss 2022 3-part secret, hysteria2, unknown), provision ConfigLink, handler detail/export, api SendDocument, convert ConfigLink regression. Semua hijau: `go build`/`vet`/`test -race` (20 package), gofmt clean, file < 250 baris. |
| v1.20 | 2026-08-11  | **M6 partial — Perintah & Menu Admin (FR-11) selesai & teruji** (keputusan user: harga + toggle plan + broadcast + ban/unban; server mgmt/statistik/adjust saldo/audit log ⬜): `/admin` + callback `admin:*` (hanya `ADMIN_IDS` — di-re-check di setiap surface, AC FR-11). Baru: `service/admin` (price ops via pricingsvc: `ListAll`/`Get`(state apa pun)/`SetPrice`/`SetEnabled`/`Reload`; `BanUser`/`UnbanUser` **dua layer** — marker gate Redis `bot:ban:{id}` TTL 1 thn + flag persisten `users.is_banned` (guard debit); broadcast), `repository/redis/admin_fsm.go` (state FSM `bot:fsm:admin:{id}` TTL 10 mnt — harga/broadcast/ban/unban input teks), broadcast **chunked 100 msg/6 dtk** via goroutine terbound (timeout 15 mnt, panic-recover §1.6, lock Redis `bot:admin:broadcast` anti-double, laporan selesai ke admin), repo pricing `SetPrice/SetEnabled/ListAll/Get` + user `SetBanned/ListTelegramIDs/CountUsers`, menu `menu_admin.go` (copy emoji-free), handler `admin.go` + `admin_user.go` (FSM mirip topup custom-input; input invalid → re-prompt; cancel membersihkan FSM; `/cancel` & `/start` juga bersihkan FSM admin), wiring `buildShop` → bundle `{Shop, Admin}`. Test baru: service admin (delegate, ban dua layer, broadcast chunked+lock+busy+empty), handler admin 12 kasus (deny non-admin, menu, price list, detail, set price FSM, toggle, broadcast flow, ban/unban flow, cancel), pricing/user repo integration. **Fix review**: typed-nil interface gotcha pada fake FSM (nil `*fakeAdminFSM` dalam interface non-nil); GORM omits `Enabled:false` saat INSERT karena tag `default:true` — toggle admin memakai UPDATE (benar), test disesuaikan. **Fix review (round 2, 5 temuan)**: 1) `Reload` seed tidak lagi membalik toggle admin — `UpsertMany` hanya sinkronkan harga, kolom operasional `enabled` dipertahankan; 2) broadcast kini ber-base context dari signal ctx (bukan `Background`) + akurasi laporan; 3) wiring `buildShop` menerima shutdown ctx; 4) nil-guard di `handleAdmin`; 5) copy reload. Semua hijau: `go build`/`vet`/`test -race` (18 package), gofmt clean, file < 250 baris. |
| v1.21 | 2026-08-11  | **M6 selesai — Sync Traffic (PRD §16.2) ✅**: worker interval sinkron `traffic_used/up/down` + `last_online` + `last_sync` dari panel X-UI ke `vpn_clients` (tanpa migrasi — kolom sudah ada sejak M4). Job `ExpiryNotifier` **direfactor jadi generik `IntervalWorker`** (`internal/job/interval.go`) dan dipakai dua worker (notifikasi + traffic) — bukan duplikasi loop. Baru: `service/traffic` (candidate aktif + server aktif, **group per server** → `GetInbounds` sekali per panel — `clientStats` membawa traffic semua client, sumber sama dengan `getClientTrafficsById` tanpa N+1 §1.7 — + `GetOnlineClients` utk `last_online`; per-server timeout (`XUI_API_TIMEOUT`); **satu panel gagal tidak menggagalkan sweep** — log + lanjut, PRD §16.2), repo `ListTrafficCandidates` (bounded, `last_sync ASC NULLS FIRST` = fair round-robin antar sweep) + `SyncTrafficBatch` (**satu statement** `UPDATE ... FROM (VALUES ...)` anti N+1, `last_online = COALESCE(v.online, c.last_online)` — offline mempertahankan timestamp lama), config `TRAFFIC_SYNC_ENABLED/INTERVAL_MIN/BATCH` (default 5 mnt/200, di file `config/traffic.go` agar config.go < 250 §1.1), wiring helper `startTrafficSync` di shop.go (main.go tetap < 250). Test: service traffic 5 kasus (group per server, online set LastOnline, client hilang di panel di-skip, no-candidate no-call, server gagal → yang lain tetap sync + error), repo integration 3 kasus (filter is_active/is_expired/server aktif, bounded + prioritas NULLS FIRST, batch COALESCE + microsecond-tolerant compare), config test baru. **Fix review**: GORM omits `IsActive:false` saat INSERT (tag `default:true`) — test memakai UPDATE (jalur nyata); `config.go` & `main.go` dirapikan agar < 250 baris. **Fix review (round 2)**: `sweepTimeout` 2 mnt yang di-hardcode tidak muat untuk fleet panel — kini **parameterized per worker** di `NewIntervalWorker(interval, timeout, svc, logger)`: expiry 2 mnt (Telegram), traffic di-sizing `len(panels) × XUI_API_TIMEOUT + 2 mnt` agar sweep tidak terpotong di tengah fleet (error palsu panel sehat). **M6 selesai.** Semua hijau: `go build`/`vet`/`test -race` (19 package), gofmt clean, file < 250 baris. |
| v1.31 | 2026-08-11  | **M7 — Hapus Akun FR-08 AC-4 ✅**: detail akun + tombol `Hapus Akun` → `account:delete:{id}` (halaman konfirmasi: detail akun + peringatan tidak bisa dikembalikan) → `account:delete_confirm:{id}` eksekusi — **panel `delClient` PALING DULU** (`serversvc.DeleteClient`, file `service/server/delete.go`), DB row setelah (`ClientRepo.DeleteOwned`, ownership guard; panel gagal → DB tidak dihapus; sukses → "Akun dihapus"). Parity reference `account_handler` delete flow (Ya, Hapus / Batal → detail). Seam baru `ClientDeleter` (serversvc) + `ClientReader.DeleteOwned`. Test: handler 2-langkah (step 1 tidak menghapus, panel-first ordering, foreign ditolak), view (konfirmasi + keyboard + sukses + icon policy), service `DeleteClient` (fake panel), integration repo `DeleteOwned` (row hilang, foreign ditolak). Fake `fakeClients` dipindah ke `client_fakes_test.go` (§1.1). Semua hijau: build/vet/test -race/gofmt/golangci-lint, file < 250 baris. Docs: README/SYSTEM_MAP/UAT. |
| v1.30 | 2026-08-11  | **M7 — Pagination Akun FR-08 AC-1 ✅**: `account:menu` → list **5/halaman** (newest first) → `account:page:{n}` navigasi + indikator non-aksi `account:noop` (answer tanpa edit, parity reference `accounts:page:{n}`); header "Halaman X dari Y"; halaman di luar range di-clamp (sama pola FR-14). Repo: `ClientRepo.CountByUser` + `ListByUserPage` (split `client_repo_page.go` §1.1; `ListByUser` = delegate page 1). View: `AccountsText`/`AccountListKeyboard` + `pagerRow` generic (dipakai juga history FR-14). Handler: `accountList` + `parsePage` generic di `parse.go` (dipakai juga history). Test: handler (page 1/2, clamp, noop, bad page), view (pager golden), integration repo (count, offset, bound clamp). Semua hijau: build/vet/test -race/gofmt/golangci-lint, file < 250 baris. Docs: README/SYSTEM_MAP/UAT. |
| v1.29 | 2026-08-11  | **M7 — Bantuan FR-15 ✅**: menu `help:menu` → kategori statis (`help:order`, `help:topup`, `help:disclaimer`, `help:info`) + sub-ToS (`help:tos:account`, `help:tos:payment`) parity `help_handler` reference; konten id-ID statis di `service/telegram/menu_help.go` + `menu_help_tos.go` (split §1.1), routing `handler/telegram/help.go` tanpa service seam (konten murni presentasi), shortcut aksi (Beli VPN/Top Up), tombol kembali & home di setiap halaman (icon policy). Test view + handler (semua callback help:* → edit-in-place, unknown → noop). Semua hijau: build/vet/test -race/gofmt/golangci-lint, file < 250 baris. UAT checklist FR-15 + SYSTEM_MAP/README diperbarui. |
| v1.24 | 2026-08-11  | **M7 fix — Trial pakai inbound picker + icon policy navigasi**: (1) **FR-07 trial kini memilih server → protocol dari panel** (sama seperti FR-03): callback `trial:server:{id}` → `trial:inbound:{server}:{inbound}` → `trial:confirm:{server}:{inbound}`; handler re-check limit di menu, pilih server, DAN confirm (AC-1 tetap), protocol di-resolve live (callback basi ditolak); `TrialSpec` + field `InboundID`+`Protocol`, `CreateTrial` pass ke `CreateTrialClient` (pin inbound, fallback vless), order protocol tercatat sesuai pilihan; keyboard inbound di-share via `InboundsKeyboard(cb func, backCb)` (buy + trial render data panel identik). (2) **Icon policy (keputusan user)**: icon HANYA di tombol navigasi — Home/Back/Cancel/Next/Prev (`🏠 Menu Utama`, `⬅️ Kembali`, `Batal ✕`, `Beli VPN Premium ➡️`); semua tombol aksi jadi text-only (menu utama Beli/Perpanjang/Akun Saya/Top Up/Trial/Riwayat/Bantuan, admin Harga/Broadcast/Ban/Unban/Reload/Ubah Harga/Toggle/Kirim, konfirmasi Beli/Perpanjang/Top Up/Trial/Ban/Unban, Nominal Lain, Join Grup ↗, Sudah Join ✓). Flag server di label tombol tetap (data, bukan icon tombol). Refactor split: `handler/telegram/trial_parse.go` (parseTrialInbound), `buyableServer` pindah ke buy.go (dipakai buy+trial), semua file < 250 baris. Test: trial flow 4-step (menu → server → inbound → confirm) verifikasi `CreateTrial` dipanggil dengan inbound 4/protocol vless, pin inbound di ordersvc (`trialInboundID`), icon-policy assertion (tombol confirm tanpa ✅), golden Home keyboard text-only. Semua hijau: `go build`/`vet`/`test -race` (20 package), gofmt clean, file < 250 baris. |
| v1.23 | 2026-08-11  | **M7 fix — FR-03 protocol/inbound picker (gap UAT)**: flow beli kini **negara → pilih inbound (server + protocol) → paket → konfirmasi → eksekusi** — inbound di-*fetch live* dari panel, bukan hardcode. Baru: `serversvc.ListInbounds` (GET `/xui/API/inbounds/`, filter enabled + port>0, read-model `InboundOption` tanpa secret), callback `buy:inbound:{serverID}:{inboundID}:{country}` + keyboard (label `Server · PROTOCOL · remark`), `parseBuyInbound`/`parseBuySelection` (`serverID:inboundID:CODE:DAYS`), handler `buyCountry` render picker per server di negara itu (panel error → skip + log, bukan gagal total), `buyInbound` resolve protocol live dari panel (callback basi tidak nembus ke inbound mati), `buyConfirm`/`buyExecute` re-resolve protocol, order `Purchase` kini menerima `serverID+inboundID+protocol` eksplisit (legacy path 0/0/"vless" tetap jalan), `provisionClient` pakai **inboundID pin** (`matchInboundByID`) fallback `matchInbound(protocol)`, credential per protocol lengkap (**hysteria/hysteria2 → field `Auth`** — sebelumnya hanya ID/password; fallback password untuk protocol tak dikenal), `PanelGateway.CreateClient/CreateTrialClient` + param inboundID, `BuyConfirmText` tampil baris Protocol. Test baru: handler flow 5-step (country → inbound → plan → confirm → execute) memverifikasi `Purchase` dipanggil dengan server=1/inbound=4/protocol=vless, `ListInbounds` filter enabled+port, `matchInboundByID`, provisioning hysteria (auth credential + AddClient inbound 5), keyboard inbound callback contract. Refactor split file demi §1.1: `server/inbound.go`, `order/purchase.go`, `telegram/menu_buy.go`, `server/server_inbound_test.go`. Semua hijau: `go build`/`vet`/`test -race` (20 package), golangci-lint bersih, gofmt clean, semua file < 250 baris. |
| v1.22 | 2026-08-11  | **M7 partial — Hardening audit & coverage gap closure**: baseline coverage diukur per package → 3 gap ditutup. (1) **`service/user` 0% → 100%** — `user_test.go` baru (EnsureUser/Get/Balance/Debit/Credit/SetBanned/List/Count via fake store; wrapper error propagated). (2) **`service/telegram` 25% → 66.8%** — `ban_test.go` (IsBanned/Ban/Unban marker key+TTL, fail-closed store error) + `menu_shop_test.go` (format harga Rupiah di tombol plan, callback `buy:plan:ID:30`, saldo cukup/tidak cukup, ringkasan renewal, status akun Expired/Aktif + sisa hari, topup keyboard 2-per-row + custom, trial menu/confirm/success, `flagOrGlobe`/`shortEmail`) + `menu_admin_test.go` (marker 🚫 plan disabled, callback `admin:plan:*`, confirm ban/unban `admin:{ban,unban}:confirm:*`, broadcast preview + report, label user). (3) **`service/server` 32% → 45.1%** — `matchInbound` (protocol case-insensitive, skip disabled/port-0, fallback) + error path `RenewClient` (server tak dikenal, password korup). **Load test (M7)**: benchmark worker pool baru `worker_bench_test.go` — cheap handler **~3.8 ms/batch-1.000 (~266k ops/s)**, realistic 1 ms handler **~62 ms/batch-200 (~3.2k ops/s)** (8 worker), drain 1.000 update **~322 ms**; **fix review round 3**: benchmark awal memakai queue cap 64 + `b.Fatal` saat drop → FAIL di bawah default benchtime (b.N ribuan > cap) dan klaim drain "1.000 update" palsu (~930 di-drop) — kini **queue cap = batch** (semua update benar-benar masuk + diproses, assert handled == batch), angka load test jujur; ban test kini memverifikasi TTL & value marker yang benar-benar di-pass (bukan hanya konstanta); `active := now.Add(...)` deterministik di test accounts; UAT doc meng-hedge AC-2 (config link belum diimplementasi di M4 subset). **UAT**: `docs/002-UAT-BOT-ORDER.md` baru — checklist FR-01 s.d. FR-15 + keamanan + load + exit criteria v1 (status 🔲/⬜/✅/❌, siap diisi manual di staging). Tanpa `t.Skip`, tanpa TODO/FIXME di bot/. Semua hijau: `go build`/`vet`/`test -race` (20 package), gofmt clean, file < 250 baris. Sisa M7: eksekusi UAT di staging + demo end-to-end (QRIS menunggu API final). |
| v1.19 | 2026-08-11  | **M6 partial — Notifikasi kadaluarsa (FR-09) selesai & teruji**: worker interval `internal/job/expiry_notify.go` (stdlib `time.Ticker` — bukan robfig/cron: AGENTS.md §Stack prefer stdlib & module cache kosong; sweep pertama langsung lalu tiap `EXPIRY_NOTIFY_INTERVAL_MIN` default 6 jam, timeout 2 mnt/sweep, panic-recover §1.6, berhenti via ctx cancel + WaitGroup drain), `service/expiry` (jendela eksklusif `(lower, upper]` dari `EXPIRY_NOTIFY_DAYS` diurutkan turun → **sekali per ambang H-7/H-3/H-1 — AC FR-09**; send gagal → TIDAK ditandai → retry sweep berikutnya; tanggal diformat `TIME_LOCATION`), repo `ListExpiryCandidates` (JOIN `users` utk telegram_id + LEFT JOIN `vpn_servers`; bounded; akun trial/nonaktif/expired dikecualikan) + `MarkNotified`, migrasi `000003`: `notified_expiry` BOOLEAN → INTEGER (0/7/3/1; `DROP DEFAULT` dulu — default lama tidak auto-cast, terverifikasi via integration test), `UpdateExpiry` reset `notified_expiry=0` saat renewal (siklus notif dimulai ulang). Copy `menu_expiry.go` (emoji-free per policy). Env baru: `EXPIRY_NOTIFY_ENABLED` (true), `EXPIRY_NOTIFY_INTERVAL_MIN` (360), `EXPIRY_NOTIFY_BATCH` (50). Test baru: service expiry (jendela 7/3/1, gagal kirim tidak ditandai, list error tetap lanjut), job (cancel → stop, panic recover), copy, integration repo (jendela, sekali-per-ambang, renewal reset, trial exclude, batch limit) + config. Semua hijau: `go build`/`vet`/`test -race` (17 package), gofmt clean, file < 250 baris. **Fix review**: defer shutdown memanggil `stop()` SEBELUM `notifyWG.Wait()` — urutan LIFO defer membuat jalur error `errCh` (server gagal bind) menggantung di `Wait()`; `stop()` idempoten dipanggil eksplisit di dalam defer. Sisa M6 (sync traffic, perintah admin) ⬜. |
| v1.18 | 2026-08-11  | **M6 partial — Trial (FR-07) selesai & teruji** (keputusan user: M6 didahulukan daripada menunggu API KentangTech). Baru: `service/trial` (policy: `TRIAL_ENABLED`, `TRIAL_DAILY_LIMIT`=2, `Remaining`, `Claim` anti-race + rollback, TTL s.d. tengah malam `TIME_LOCATION`), `repository/redis/trial_counter.go` (INCR+EXPIRE-on-first, rollback; ops `Incr`/`Decr`/`Expire` + key `bot:trial:{userID}`), `domain.NewTrialClient` (is_trial, expiry = now+jam, kuota 1 GB / 1 IP), `serversvc.CreateTrialClient` via helper `provisionClient` (CreateClient di-refactor berbagi helper — inbound matching & spec build tidak duplikat), `ordersvc.CreateTrial` (order type `trial`, **tanpa debit** — balance tetap; client row `is_trial=true`; PanelGateway +method `CreateTrialClient`), menu `menu_trial.go` (`trial:menu` → `trial:server:{id}` → `trial:confirm:{id}`; teks tanpa emoji, tombol navigasi ber-icon), handler `trial.go` (**limit di-re-check di menu, pilih server, DAN confirm — AC-1 anti-race**; `/trial` text command; guard nil result — lesson bug staging v1.11; disabled → `TRIAL_ENABLED=false`). Wiring: `buildShop` dipindah ke `cmd/bot/shop.go` (main.go tetap < 250 baris). Env baru: `TRIAL_ENABLED`, `TRIAL_DAILY_LIMIT`, `TRIAL_DURATION_HOURS`, `TRIAL_TRAFFIC_GB`, `TRIAL_IP_LIMIT` (default 1 jam/1 GB/1 IP). Test baru: service trial (claim limit + rollback + TTL midnight), redis counter (miniredis: TTL rollover), domain trial client, order CreateTrial (tanpa debit, is_trial, panel fail → failed), handler trial flow 9 kasus. **Fix review**: (1) `TRIAL_DURATION_HOURS/TRAFFIC_GB/IP_LIMIT` di-wire penuh — `trialsvc.New` menerima hours/GB/IP, `TrialLimiter` expose accessor, handler & teks pakai nilai config (tidak hardcode 1/1/1); (2) Claim dipindah **setelah** EnsureUser + cek server — pre-check gagal tidak membakar kuota; (3) INCR+EXPIRE **atomik via Lua** (`redis.NewScript`) — crash antara INCR & EXPIRE tidak mungkin menyisakan key tanpa TTL (lockout permanen); (4) `duration_days=0` untuk trial + jam dicatat di `Notes` (history tidak terbaca "1 hari"); (5) dead code dibersihkan (`Incr`/`Expire` ops tak terpakai setelah Lua; type `trialCall`). Semua hijau: `go build`/`vet`/`test -race`, file < 250 baris, headers lengkap. Sisa M6 (notifikasi, sync traffic, perintah admin) ⬜. |
| v1.17 | 2026-08-11  | **Progres dokumentasi disinkronkan** (docs-only, tanpa perubahan kode): status header → v1.17 (M0–M4 ✅ · M5 partial · M6–M7 ⬜); tabel milestone §21 di-update (M0 ✅ v1.0/v1.3, M1 ✅ v1.4, M4 ✅ v1.11, M5 🔶 v1.13); changelog §27 diurutkan descending (v1.16 → v1.0) agar konsisten; `SYSTEM_MAP.md` note sync → v1.17. Kondisi kode terverifikasi: `go build`/`vet` hijau, semua file < 250 baris (max 247), headers lengkap. |
| v1.16 | 2026-08-11  | **AGENTS.md v1.1 disinkronkan dengan tooling repo** (keputusan user): section baru **"Git Hooks & Secret Scanning (Enforced)"** — tabel gate (pre-commit: gitleaks+gofmt+golangci-lint bot/ baris baru+whitespace; commit-msg: Conventional Commits; pre-push: gitleaks range+go build/vet+lint bot/; CI security.yml), aturan wajib agent (no `--no-verify` untuk secret/lint, golangci-lint `(cd bot && golangci-lint run ./...)`, gofmt-clean, commit convention, allowlist gitleaks berjustifikasi), + catatan teknis (`git diff --check` tanpa `--quiet`; golangci-lint v1.64.x jangan upgrade v2 tanpa migrasi config). §1.2 bertambah: kontinuasi header `// text` (1 spasi) — gofmt reflow style 12-spasi jadi non-stable. Compliance Checklist bertambah 3 item (gofmt/lint-clean, gitleaks allowlist, Conventional Commits). |
| v1.15 | 2026-08-11  | **golangci-lint ditambahkan ke pre-commit/pre-push/CI** (opsional lokal, wajib CI — lebih ketat dari go vet). `golangci-lint` **v1.64.8** terinstall; config `.golangci.yml` di root (format v1 — tanpa field `version`, karena v1.64 memperlakukan key itu sebagai format v2): enable `errcheck, gosimple, govet, ineffassign, staticcheck, unused, misspell, bodyclose` (selaras AGENTS.md §1.4/§1.6); exclude-rules: `_test.go` (bodyclose/misspell) + dir upstream root (misspell/gosimple, ditandai "manual only" — root tidak di-lint di gate mana pun). **Lint penuh bot/ → bersih** setelah menghapus dead code `newXUIError` (`repository/xui/errors.go` — semua konstruksi error pakai literal `&XUIError{...}`, terverifikasi via search). **Temuan teknis**: (1) `--new-from-patch` gagal bila patch dibuat dari root sementara linter jalan dari `bot/` (path mismatch) — solusi: `git diff --cached --relative=bot` + jalankan dari `bot/`; terverifikasi flag issue baru (unused fn) exit 1; (2) **bug laten pre-commit v1.14**: `git diff --cached --check --quiet` — `--quiet` implies `--exit-code` yang TIDAK kompatibel dengan `--check` (exit 1 = "ada perbedaan" selalu true saat staging) → **setiap commit dengan file staged pasti gagal whitespace check**. Fix: hapus `--quiet`. Ditemukan saat uji lint berulang (3x reproduksi) — regression test: file bersih lolos 3x, trailing space & unused fn diblokir, pesan error yang tepat. (3) **Normalisasi gofmt 34 file** (17 header + alignment/import): style kontinuasi header `//            text` (12 spasi) TIDAK gofmt-stable — gofmt reflow jadi `//\n//\ttext\n//`; eksperimen 3 style → `// text` (1 spasi) stabil. Dikonversi 23 baris di 17 file + `gofmt -w` seluruh modul (91 file kini bersih: `gofmt -l` kosong); test-file `topup_test.go` 253 baris → dipecah ke `topup_edge_test.go` (203+68). **pre-commit** §2b: lint baris baru bot/; exit 1 = isu lint, exit lain (mis. 7 typecheck error) = pesan "gagal scan" (fail-closed). **pre-push** §3: lint penuh bot/. **CI**: job `golangci` (golangci/golangci-lint-action@v6, `working-directory: bot`, pin `v1.64.8`). Semua hijau: build/vet/lint/test -race + line limits + headers; staging di-restart (health db/redis/webhook ok). |
| v1.14 | 2026-08-11  | **Security tooling developer-standard**: gitleaks (secret scan) + git hooks + CI hardening. `gitleaks.toml` — allowlist HANYA false positive terverifikasi (source map vendor `antd.min.js.map` via path, identifier JS panel `getNewX25519.obj.privateKey` via per-commit 95bdb06); scan penuh 1.295 commit → **0 leak**. `.githooks/` (di-commit, aktif via `scripts/install-githooks.sh` → `core.hooksPath`): `pre-commit` = gitleaks `protect --staged` (fail-closed bila gitleaks absen) + gofmt + `git diff --check`; `commit-msg` = Conventional Commits (feat/fix/docs/style/refactor/perf/test/build/ci/chore/revert + scope + `!`; Merge/Revert/fixup!/squash! diizinkan); `pre-push` = gitleaks scan range commit baru dengan **validasi `git rev-list`** (gitleaks diam-diam menganggap range invalid sebagai kosong → fail-closed duluan; skip saat 0 commit; blokir saat `leaks found: [1-9][0-9]*` — fix review: regex `[1-9]` lama miss 10+ leak) + `go build`/`vet` root & bot. CI `.github/workflows/security.yml`: gitleaks-action (push/PR, fetch-depth 0) + job Go root (build/vet) + job Go bot (build/vet/test -race dengan service PG16+Redis7, `TEST_DATABASE_URL`/`TEST_REDIS_URL`). `.gitignore` hardening root & bot: `.env*` (negasi `.env.example`), certs/`*.pem`/`*.key`/`*.crt`, `*.save` (nano backup), gitleaks-report*, coverage.out. **Kebijakan secret**: kredensial HANYA di `bot/.env` (gitignored terverifikasi `git check-ignore`); `.env.example` placeholder saja; `--no-verify` tidak boleh untuk secret (CI tetap scan). **Teruji**: pre-commit memblokir slack-token staged; commit-msg lolos/tolak; pre-push memblokir commit ber-secret, memblokir range invalid, skip range kosong; bot/.env & nano.1797784.save ter-ignore. |
| v1.13 | 2026-08-11  | **M5 partial — Topup flow (FR-06) dibangun, payment API di-defer** (keputusan user: API KentangTech sedang rewrite ke Go — bot hanya bergantung pada seam `PaymentGateway`; saat API final tinggal swap client, tanpa rewrite ulang menu/flow). Baru: `service/topup` (formula fee §15.7: `gross = ceil(net/(1-fee·(1+PPN)))` dibulatkan ×100, min/max dari config, `StubGateway` → `ErrPaymentAPIUnavailable`), `repository/redis/topup_fsm.go` (marker FSM `bot:fsm:topup:{id}` TTL 10 menit — crash guard input custom), `service/telegram/menu_topup.go` (quick-pick 6 nominal + "Nominal Lain" + ringkasan + custom prompt + teks unavailable/QR), `handler/telegram/topup.go` (flow `topup:menu` → `topup:amount:N` → `topup:confirm:N`; custom input FSM-aware; `/cancel` + tombol batal; quote **selalu dihitung server-side** dari callback payload — tidak trust client). Dispatcher: field `topup`, routing prefix `topup:`, teks FSM-aware, `/cancel`; **fix review**: `/start` & `topup:menu` clear FSM (stale state tak menelan teks berikutnya), balasan HelpHint hanya di chat private (bot diam di grup), `NewDispatcher` +param `topup` (test call sites diupdate). **Staging smoke test**: `topup:menu`/`amount`/`confirm`/`custom` sintetis → semua `ok:true`, stub → teks unavailable **tanpa panic**, bot tetap hidup. Test baru: fee math (10K → gross 10.300, kelipatan 100, range), FSM Redis (miniredis TTL), flow handler 13 kasus. Semua hijau: `go build`/`vet`/`test -race`, semua file < 250 baris, headers lengkap. |
| v1.12 | 2026-08-11  | **UI copy policy (keputusan user): semua teks pesan bebas emoji — icon hanya di tombol navigasi**. Dibersihkan di `service/telegram/menu.go` (HomeText/JoinText/BannedText/RateLimitText/UnavailableText), `menu_shop.go` (semua teks buy/renew/accounts — separator `━━━` & bullet `•` dipertahankan sebagai elemen modern non-emoji), dan handler (`buy.go`/`renew.go`/`accounts.go`/`dispatcher.go`/`outbox.go` — pesan error/gate/insufficient tanpa `🙏😅⚠️` dst). **Tombol inline tetap memakai icon**: HomeKeyboard (`🛒 Beli VPN`, `👤 Akun Saya`, ...), join (`🔗 Join Grup`, `✅ Sudah Join`), navigasi (`🏠 Menu Utama`, `⬅️ Kembali`, `✅ Konfirmasi Beli`), dan flag negara pada tombol pilih negara. Verified: grep emoji → hanya tersisa di `Text:` tombol + komentar; `go build`/`vet`/`test -race` hijau; staging di-restart, `/start` sintetis terkirim bersih tanpa error log. |
| v1.11 | 2026-08-11  | **M4 — Order flow selesai & teruji** (FR-03/FR-04/FR-05/FR-08 subset). Domain: `money.go` (Money int64 + `sql.Scanner`/`driver.Valuer` — NUMERIC(15,2) ⇄ int64 untuk GORM), `order.go` (state machine `pending→processing→completed|failed`, `OrderType`), `plan.go` (`VpnPlan` + `CountryNames`), `client.go` (`VPNClient` + `PanelClient`), `random.go` (`KTS-XXXXXXXX-VPN`, UUID v4, secret hex). **Multi-panel dinamis** (FR-10): `config/servers.go` parse blok `PANEL_N_*` (NAME/HOST/PORT/USERNAME/PASSWORD/API_PATH/USE_SSL/INSECURE/COUNTRY_CODE/FLAG_EMOJI/LOCATION/PROTOCOLS) + `PRICING_SEED_FILE` (default `seed/pricing.json` — 12 paket ID/SG/JP/CN dari data user). **Postgres**: GORM models 7 tabel (PRD §13) + repos `user` (debit/kredit atomik `UPDATE ... WHERE balance >= amount` via `Exec`+`RowsAffected` + ledger immutable di tx yang sama — `ErrInsufficientBalance`), `pricing` (upsert idempotent), `server` (seed terenkripsi AES-256-GCM), `client` (list join server, ownership guard), `order` (+ migration `000002` kolom `vpn_servers.insecure_tls` per-panel, default secure). **Services**: `pricing` (seed JSON → DB), `user` (balance/ledger), `server` (seed enkripsi, `PickForCountry`, gateway XUI `CreateClient`/`RenewClient` — password panel didekripsi saat runtime), `order` (`Purchase`/`Renew` — harga selalu live dari pricing, saldo dicek pre-order, order dibuat hanya setelah konfirmasi; **debit terjadi SETELAH panel sukses & client row dibuat** — tidak ada uang hilang tanpa akun; renewal memperpanjang dari sisa waktu, tidak double-count). **Handler Telegram**: flow beli (`buy:menu`→`buy:country:X`→`buy:plan:X:days`→`buy:confirm:X:days`), renewal (`renew:client:id`→`renew:plan`→`renew:confirm`), akun (`account:menu`) + dispatcher routing. **Code review fix**: (1) order debit reorder — client row dibuat SEBELUM debit (debit gagal → akun unpaid recoverable, bukan uang hilang); (2) `PurchaseResult.NewExpiry` — pesan renewal tampilkan expiry asli (dari sisa waktu), bukan `now+days`; (3) `insecure_tls` per-panel (default secure, opt-in untuk panel self-signed staging); (4) **bug nyata staging**: callback shop dengan `message` inaccessible (nil) → panic → worker goroutine tanpa recover → **seluruh proses mati**. Fix: guard nil di semua handler flow (`editCB`) + **panic recovery di worker goroutine** (AGENTS.md §1.6) + regression test. **Staging smoke test**: boot → migration 000002 OK → pricing 12 paket ter-seed di DB → health ok → `buy:menu` sintetis (tanpa panel → pesan no-server, tanpa panic) → `buy:country` tanpa message → aman → bot tetap hidup. Total **152+ unit/integration test** hijau (`go build`/`vet`/`test -race`), semua file < 250 baris, headers lengkap. **Blocker webhook live tetap**: DNS Cloudflare `bot-xui` → `46.250.232.48` (proxy OFF) + nginx/certbot. |
| v1.10 | 2026-08-11  | **Gate policy dikunci: bypass untuk ADMIN_IDS** (keputusan user — owner/admin memakai identitas anonim Channel, tidak perlu join discussion group). `Dispatcher` menerima `adminIDs` (dari config): chain `ban → [gate hanya jika non-admin] → rate limit → route`; **ban check tetap berlaku untuk admin**. Staging group `-1003628678328` terverifikasi sebagai **discussion group** dari channel `@kentangtechstore_id` (`linked_chat_id` bolak-balik: `-1003628678328` ↔ `-1003392238313`); Telegram **tidak mendukung** query keanggotaan via identitas channel/group (`getChatMember` dengan `user_id=channel_id` → `invalid user_id specified`), dan anonymous admin tetap punya akun user (status `administrator`+`is_anonymous`, bukan `left`) — didokumentasikan. Test baru: admin bypass gate → menu, admin banned → tetap ditolak, non-admin → tetap join prompt (+ dispatcher test dipecah 2 file demi limit 250 baris). **Verifikasi staging live**: `/start` sintetis admin → **menu terkirim** (tanpa error log); `/start` user biasa → ditolak gate (fail-closed). Semua hijau: `go build`/`vet`/`test -race`. |
| v1.9  | 2026-08-11  | **Staging IDs dikunci + smoke test M3**. `bot/.env`: `ADMIN_IDS=8297101882` (KENTANG TECH STORE), `REQUIRED_GROUP_ID=-1003628678328` (grup **"KENTANG TECH STORE ID DEV"** — bot sudah jadi member, `getChat` terverifikasi), `REQUIRED_GROUP_LINK=https://t.me/+v6grK-D8mf9lYzk9`. **Level AUTHOR vs ADMIN**: `GateService.Role()` baru (`service/telegram/role.go`) — memetakan status Telegram `creator` → `RoleOwner` (AUTHOR/pemilik grup, `8297101882` adalah pemilik grup staging) vs `administrator` → `RoleAdmin` (admin biasa), plus `member/restricted/left/banned/unknown`; helper `IsStaff()` (owner ∪ admin) untuk perintah admin FR-11/M6. Role di-lookup **fresh** (tanpa cache) karena keputusan authorization harus akurat; gate tetap allow semua member-level termasuk owner (test: owner → gate allowed + cache terisi). 9 unit test role baru, semua hijau. **Smoke test end-to-end via update sintetis** (DNS belum live): boot → migrations → Redis → `setWebhook` registered (pending 0) → `health` `db/redis/webhook` ok → dedup `update_id` (ke-2 → `dedup:true`) → secret salah → 403 → malformed JSON → 400 → `/start` sintetis dari admin → gate **Denied** (admin status `left` di grup) → join prompt dengan tombol **🔗 Join Grup** terkirim ke admin (tanpa error log) → callback `gate:check` sintetis → `CheckFresh` → Denied (answer gagal hanya karena callback ID palsu dari test, wajar). **Bug ditemukan & diperbaiki saat smoke test**: `JoinKeyboard` dengan `REQUIRED_GROUP_LINK` kosong mengirim tombol URL tanpa URL → Telegram error `Text buttons are unallowed in the inline keyboard`. Fix: tombol URL di-*skip* saat link kosong (+ unit test). Bot staging berjalan di background (binary `/tmp/bot-staging`, log `/tmp/bot-staging.log`; stop: `pkill -x bot-staging`). **Blocker webhook live**: (1) admin `8297101882` join grup (status saat ini `left`), (2) [DONE] `REQUIRED_GROUP_LINK` diisi, (3) DNS Cloudflare `bot-xui` → `46.250.232.48` (proxy OFF), (4) nginx + sertifikat (certbot) — lihat README. |
| v1.8  | 2026-08-11  | **M3 selesai — core webhook go-telegram/bot v1.23.0**. Baru: `repository/telegram` (wrapper typed: setWebhook/getWebhookInfo/sendMessage/editMessageText/answerCallbackQuery/getChatMember — SDK mengirim params form-encoded, fake server httptest parse `r.Form`), `service/telegram` (webhook.go: setWebhook boot + verifikasi getWebhookInfo, fail-fast; gate.go: gate grup + cache Redis 6 jam (FR-01), `CheckFresh` untuk tombol "✅ Sudah Join"; ban.go: marker `bot:ban:{id}`; ratelimit.go: sliding window ZSET 30/menit; menu.go: keyboard FR-02 7 tombol + copy; keys.go: TTL policy), `handler/telegram` (dispatcher: chain **ban → gate → rate limit → route**; `/start` → menu, callback `menu:*` → **edit in-place** (FR-02 AC), `gate:check` → re-check, tombol non-aksi → answer noop; fail-closed utk ban/gate, fail-open utk rate limit), `handler/http` (webhook route real: secret token constant-time → parse `models.Update` → **dedup `update_id` SETNX 24 jam** → enqueue **worker pool bounded 64** → 200 cepat; per-user lock `bot:lock:user:{id}` TTL 30s **di-release setelah handle** (crash-guard TTL tetap) — review fix: lock tak pernah di-release sebelumnya menyebabkan tap kedua dalam 30s ke-drop; **plain text tanpa parse mode** — fix review: nama user berkarakter markdown (`*`,`_`) mem-boom parse), config baru `WEBHOOK_MAX_CONNECTIONS` (40, 1–100), `WEBHOOK_DROP_PENDING` (true), `WEBHOOK_WORKERS` (8), `WEBHOOK_QUEUE_BUFFER` (64). **Deviasi order dari §14.2.5**: implementasi jalankan ban → gate (PRD tulis gate → ban); ban-first dipilih karena Redis EXISTS murah & user banned mendapat pesan ban yang tepat (didokumentasikan). Health `/api/v1/health` → `webhook: registered`. Anti-polling terverifikasi: grep CI `getUpdates|DeleteWebhook` → 0 hasil. Semua hijau: `go build`/`vet`/`test -race` (30+ test), semua file < 250 baris, headers lengkap. |
| v1.7  | 2026-08-11  | **M2 selesai — X-UI client diverifikasi dari SOURCE panel (bukan reference Python)**. Endpoint & payload diambil langsung dari `web/controller/api.go`, `web/controller/inbound.go`, `web/service/inbound.go`, `web/web.go`, `database/model/model.go`, `xray/`: cookie session **`x-ui`** (gin-contrib sessions), base path dinamis `GetBasePath()`, envelope `{success,msg,obj}` (HTTP 200 utk bisnis; **401** saat `isAjax`), `getClientTrafficsById` → **array**, `addTrialClient` **tidak ada** (trial via `addClient` + `expiryTime` dihitung bot). **Deviasi struktur dari §11**: klien REST panel ditempatkan di `internal/repository/xui` (bukan `service/xui`) karena `service` dilarang import `net/http` (AGENTS.md §1.5). Baru: `internal/crypto` (AES-256-GCM, Encrypt/Decrypt base64) + `repository/xui` (login+session cache Redis `xui:session:{id}` TTL 1 jam, auto-relogin sekali pada 401/403, CRUD client add/update/delete, traffic by email & by id, onlines, server status, restart xray) + mock panel httptest (11 test). Ditemukan & diperbaiki selama review: cache session best-effort (Redis down tak gagalkan login), `classify` false-positive "full"→"inbound is full", simplifikasi helper pakai `strings`. Semua hijau: `go build`/`vet`/`test -race`. |
| v1.6  | 2026-08-11  | **Staging dikunci**: bot **`@kentangtechidcloudhost_bot`** (id 8363146903, "NOTIFICATION IDCLOUDHOST") dipakai untuk staging — token tersimpan **hanya** di `bot/.env` (gitignored, diverifikasi `git check-ignore`). Domain **`bot-xui.kentangtechstore.com`** ditetapkan (server: VPS Contabo `46.250.232.48`, hostname `vmi3491075`). Referensi domain di `.env.example`/`README`/PRD diperbarui. **Catatan DNS**: `bot-xui.kentangtechstore.com` masih resolve ke IP Cloudflare (104.21.85.123/172.67.205.181, proxied) — **A record harus diubah ke `46.250.232.48` (proxy OFF)** di panel Cloudflare sebelum webhook Telegram aktif (M3). DB staging `bot` + Redis host siap; `ADMIN_IDS`/`REQUIRED_GROUP_ID` belum diisi (menunggu input). |
| v1.5  | 2026-08-11  | **Integration test migration & Redis**: PostgreSQL 16 & Redis 7 diinstall di host (staging): cluster `16/main` + user `bot` + DB `bot`/`bot_test`; Redis default 6379 (dibersihkan dari sisa konfigurasi lama port 6380). Test baru `internal/repository/postgres/migrate_test.go` + `schema_test.go` (up → 7 tabel + kolom §13 + index + UNIQUE constraint anti double-order/credit, down → rollback semua, up/down/up konsisten, rerun idempoten, unreachable DB → clean error) & `internal/repository/redis/redis_test.go` (ping, malformed URL, unreachable, set/get roundtrip, SetNX idempotency). `MigrateDown` ditambahkan; runner migration di-refactor jadi helper bersama (`runMigration`) dengan panic recovery (AGENTS.md §1.6). DSN test override: `TEST_DATABASE_URL` / `TEST_REDIS_URL`. Semua hijau: `go build`/`vet`/`test -race`. |
| v1.4  | 2026-08-11  | **M1 selesai**: PostgreSQL (GORM) + Redis (go-redis) + migration golang-migrate 7 tabel (PRD §13) + `/api/v1/health` penuh (cek DB/Redis). HTTP handler dipindah ke `internal/handler/http`; `cmd/bot/main.go` jadi composition root murni (open DB → migrate → open Redis → HTTP). Pool limits eksplisit (`DB_MAX_OPEN_CONNS`, `DB_MAX_IDLE_CONNS`, `DB_CONN_MAX_LIFETIME_MIN`, `REDIS_POOL_SIZE`, `REDIS_DIAL_TIMEOUT_SEC`). Migration embedded & idempoten (rerun aman, `ErrNoChange` ditangani). Terverifikasi: `go build`/`vet`/`test -race` hijau + integrasi nyata docker compose (postgres+redis) → health `db:ok, redis:ok` (200), 7 tabel + `schema_migrations` ter-create. |
| v1.3  | 2026-08-11  | **Konvensi REST API dikunci**: semua endpoint di bawah `/api/v1` + penamaan resource pronounceable (section baru §26). Telegram webhook → `/api/v1/webhooks/telegram`; payment callback `/kts-vpn` → `/api/v1/webhooks/payments` (update URL callback di sisi KentangTech); health API → `/api/v1/health` (alias `/health` untuk probe). Diimplementasikan di M0: router `apiBase` di `bot/cmd/bot/main.go`. |
| v1.2  | 2026-08-11  | Keputusan deployment: reverse proxy **Caddy → Nginx** (D3, §9.1, §14.5, §18, §19, M1); **OQ#5 RESOLVED**; file `bot/nginx.conf` menggantikan `bot/Caddyfile`; TLS via `./certs` atau Let's Encrypt (certbot). |
| v1.1  | 2026-08-11  | Review parity vs reference: tambah **FR-14 (Riwayat)** & **FR-15 (Help/ToS)**; perluas AC **FR-02/04/06/07/08/11** (topup quick-pick + cek status, anti double-order, config TLS/NTLS + YAML, hapus akun, admin adjust saldo/tambah server); perjelas **kontrak KentangTech API** (§15.7, diverifikasi dari kode reference); **kebijakan kredensial panel single-admin** (§15.1, OQ#2 RESOLVED); tambah dep QR lib (§12); update risiko, env, lampiran. |
| v1.0  | 2026-08-11  | PRD awal: arsitektur, scope, stack, data model, webhook, milestone M0–M7.                                   |

---

*Dokumen ini adalah living document — setiap perubahan keputusan arsitektur
wajib di-review & di-commit bersamaan dengan PR terkait (selaras `AGENTS.md`
§1.9).*
