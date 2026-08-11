# PRD — Bot Auto-Order Telegram (Go) untuk X-UI Panel

| Field            | Value                                                              |
|------------------|--------------------------------------------------------------------|
| **Dokumen**      | 001-PRD-BOT-ORDER                                                  |
| **Status**       | Draft v1.17 — M0–M4 ✅ · M5 partial (flow ✅, API 🔜) · M6–M7 ⬜ — **UI copy policy: teks tanpa emoji (icon hanya di tombol navigasi)** |
| **Tanggal**      | 2026-08-11                                                         |
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
- QRIS berlaku **15 menit**; nominal **quick-pick** (Rp 10.000 / 25.000 / 50.000 /
  100.000 / 200.000 / 500.000) + **custom input**; input FSM bisa dibatalkan
  dengan `/cancel`.
- Formula fee: `gross = net / (1 − fee_rate × (1 + PPN))`, dibulatkan ke atas
  kelipatan 100 (effective rate **2,775%**); saldo kredit = `net` (detail §15.7).

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
- **AC-1 (Atomicity)**: debit saldo & pembuatan client X-UI idempoten (tidak ada
  double-charge jika webhook/retry ganda); error dari panel tercatat di
  `error_message` dan order berstatus `failed` (saldo **tidak** didebit);
  transaksi DB atomik + unique constraint `order_id` + lock per-user
  (`bot:lock:user:{id}`) mencegah order ganda.
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
  **custom input** → bot hitung gross (fee) → panggil **KentangTech API**
  `POST /api/v1/autobuy-saldo/topup` → tampilkan **QR PNG** + caption
  (order ID, saldo diterima, total bayar, fee, berlaku 15 menit) → webhook
  `POST /api/v1/webhooks/payments` (HMAC `X-KTS-Signature`) → kredit saldo
  net → notifikasi user + grup admin.
- **AC-1 (Alur)**: nominal di antara min/max (Rp 5.000 – Rp 5.000.000, value =
  saldo **bersih** yang diterima); gross dihitung formula §15.7; user dapat
  **cek status** (`topup:status:{orderID}` → popup alert, status
  `PENDING/SUCCESS/FAILED/EXPIRED`); input custom bisa dibatalkan `/cancel`;
  QR gagal dirender → fallback tampilkan `qr_string` teks.
- **AC-2 (Keamanan)**: HMAC `X-KTS-Signature` diverifikasi constant-time;
  **idempotency via Redis `SETNX` (`bot:topup:processed:{orderID}`, TTL
  7 hari)** — webhook ganda tidak double-credit; fee & net dicatat di
  `payments` + ledger; event `TOPUP_SUCCESS / TOPUP_FAILED / PAYMENT_FAILED /
  PAYMENT_EXPIRED / PAYMENT_PENDING` ditangani sesuai reference.
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
- **AC-2 (Config & Convert)**: view **URL config TLS & Non-TLS** (`vless://`,
  `vmess://` base64 JSON, `trojan://`) + **convert YAML Clash/Meta** (TLS &
  Non-TLS) — pola `build_config_links`/`build_yaml_configs` reference; port
  443/80, path `/{protocol}`, network `ws`, SNI = host server.
- **AC-3 (Traffic & Sync)**: halaman traffic dengan **progress bar + persen**
  & status warna (🔴≥90% 🟡≥70% 🟢); tombol **refresh/sync manual** per akun
  (panggil `getClientTrafficsById` → update DB); data juga disinkron worker
  (§16.2).
- **AC-4 (Hapus Akun)**: hapus akun **2 langkah konfirmasi**
  (`account:delete:{id}` → `account:delete_confirm:{id}`); hapus dari panel
  (`delClient`) + DB; peringatan akun tidak bisa dikembalikan; aksi tercatat
  di log & riwayat.

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
   │ KentangTech Payment API (QRIS)               │
   │ POST create payment → callback /api/v1        │
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
| `service/payment`     | Integrasi QRIS KentangTech + pemrosesan webhook (idempoten).                |
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
`config_link TEXT`, `subscription_url TEXT`, `notified_expiry BOOL`,
`last_sync`, `last_online`, `created_at`, `updated_at`.
*Index:* `email`, `user_id`, `expires_at`, `is_trial`.

### 13.4 `orders`
`id PK`, `order_id UNIQUE` (`KTS-XXXXXXXX-VPN`), `order_type`
(purchase/renewal/topup/trial), `user_id FK`, `server_id FK`, `client_id FK`,
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
  `BOT_DOMAIN` → proxy ke `bot:8443` (instalasi: `bot/README.md`).

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

### 15.6 Kontrak Error
- Respon panel `{"success": false, "msg": "..."}` → diterjemahkan ke error
  terstruktur bot (`XUIError`) dengan kode kategori: `AUTH`, `NETWORK`,
  `DUPLICATE_EMAIL`, `INBOUND_FULL`, `TIMEOUT`, `UNKNOWN`.

### 15.7 Kontrak API Pembayaran (KentangTech) — diverifikasi dari reference
Base URL: `{API_BASE_URL}` (env; default `https://hostinger.kentangtechstore.com`).
Provider: **Midtrans QRIS** via kentangtech-api. Timeout: 30 s.

**Headers (semua request):**
```
Content-Type: application/json
Accept: application/json
X-API-Key: {TOPUP_API_KEY}
X-Webhook-Secret: {TOPUP_WEBHOOK_SECRET}
```

**1) Buat QRIS — `POST /api/v1/autobuy-saldo/topup`**
Body:
```json
{
  "telegram_user_id": 123456789,
  "amount": 10280,          // GROSS yang dibayar user (sudah + fee)
  "net_amount": 10000,      // saldo bersih yang dikredit (opsional)
  "first_name": "Budi",
  "username": "@budi",
  "phone_number": "123456789@telegram"   // default "{tgid}@telegram"
}
```
Respon 200:
```json
{ "order_id": "...", "qr_string": "000201010211...", "amount": 10280,
  "expires_at": "...", "message": "QR Code berhasil dibuat" }
```
Error (non-200): field `detail` (fallback teks HTTP).

**2) Cek status — `GET /api/v1/autobuy-saldo/status/{order_id}`**
Respon 200:
```json
{ "order_id": "...", "status": "PENDING|SUCCESS|FAILED|EXPIRED",
  "amount": 10280, "created_at": "...", "settled_at": null }
```
`404` → order tidak ditemukan (tidak ada data).

**3) Webhook callback — `POST /api/v1/webhooks/payments` (di bot)**
Header `X-KTS-Signature: hmac_sha256_hex(compact_json_body, TOPUP_WEBHOOK_SECRET)`
(compact JSON = `json.dumps(data, separators=(',',':'))`, diverifikasi
constant-time). **Deploy note:** URL callback di sisi KentangTech harus
mengarah ke `https://{BOT_DOMAIN}/api/v1/webhooks/payments`.
Body:
```json
{
  "event": "TOPUP_SUCCESS | TOPUP_FAILED | PAYMENT_FAILED | PAYMENT_EXPIRED | PAYMENT_PENDING",
  "order_id": "VPN-TOPUP-xxx",
  "telegram_user_id": 123456789,
  "amount": 10280,             // gross
  "net_amount": 10000,         // net yang dikredit
  "execution_result": { },     // opsional (detail error)
  "error" | "message" | "status_message": "..."
}
```
Behavior: sukses → kredit `net_amount` (idempoten, §FR-06); gagal/expired →
notif user tanpa kredit; pending → diabaikan (user lihat QR).

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
3. **Payment webhook** — HMAC-SHA256 (`X-KTS-Signature`) constant-time compare;
   idempotency `SETNX`; replay ditolak.
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
    bot:     build ./bot, env dari .env, expose :8443 (internal)
    nginx:   image nginx:1.27-alpine, TLS (certs ./certs / Let's Encrypt), proxy → bot:8443
    postgres:  (bisa eksternal/terkelola; atau service tambahan)
    redis:     (bisa eksternal/terkelola; atau service tambahan)
  network: botnet (internal)
```
- Webhook di Nginx: `https://{BOT_DOMAIN}/api/v1/webhooks/telegram →
  http://bot:8443/api/v1/webhooks/telegram`.
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
| `DATABASE_URL`           | `postgres://user:pass@pg:5432/bot`      | PostgreSQL                       |
| `REDIS_URL`              | `redis://:pass@redis:6379/3`            | Redis                            |
| `ENCRYPTION_KEY`         | (32-byte base64)                        | AES-256-GCM secret box           |
| `ADMIN_IDS`              | `123456789,987654321`                   | Admin Telegram IDs               |
| `REQUIRED_GROUP_ID`      | `-100123456789`                         | Gate grup (kosong = nonaktif)    |
| `REQUIRED_GROUP_LINK`    | `https://t.me/kentangtech`              | Invite link grup                 |
| `NOTIFICATION_GROUP_ID`  | `-100123456789`                         | Grup notifikasi admin            |
| `EXPIRY_NOTIFY_DAYS`     | `7,3,1`                                 | Ambang notifikasi kadaluarsa     |
| `RATE_LIMIT_REQUESTS`    | `30`                                    | Rate limit per menit             |
| `TIME_LOCATION`          | `Asia/Jakarta`                          | Timezone                         |
| `XUI_API_TIMEOUT`        | `30`                                    | Timeout panggil panel            |
| `API_BASE_URL`           | `https://hostinger.kentangtechstore.com`| KentangTech payment API          |
| `TOPUP_API_KEY`          | (secret)                                | API key topup                    |
| `TOPUP_WEBHOOK_SECRET`   | (secret)                                | HMAC payment webhook             |
| `MIN/MAX_TOPUP_AMOUNT`   | `5000` / `5000000`                      | Batas nominal topup              |
| `QRIS_FEE_PERCENT`       | `0.025`                                 | Fee QRIS 2,5%                    |
| `QRIS_PPN_PERCENT`       | `0.11`                                  | PPN atas fee 11%                 |
| `QRIS_EXPIRY_MINUTES`    | `15`                                    | Masa berlaku QRIS (referensi)    |
| `LOG_LEVEL`              | `INFO`                                  | `DEBUG` untuk development        |
| `PANEL_*` (opsional)     | —                                       | Kredensial panel awal / seed     |

---

## 20. Testing Strategy (`AGENTS.md` §2.1)

| Level              | Cakupan                                                              |
|--------------------|----------------------------------------------------------------------|
| Unit               | Service order (state machine, debit), pricing, money (aritmatika IDR), format link, crypto. |
| Repository         | **Integration test terhadap PG/Redis lokal (staging host)**: `bot_test` DB + Redis DB 15; verifikasi up (7 tabel/kolom/index/UNIQUE per §13), down (rollback semua), idempotensi rerun; Redis: ping, set/get, SetNX idempotency. Override DSN via `TEST_DATABASE_URL` / `TEST_REDIS_URL`. Tanpa `t.Skip` (AGENTS.md §2.1). |
| Handler (httptest) | `POST /api/v1/webhooks/telegram` (valid/invalid secret), `POST /api/v1/webhooks/payments` (HMAC valid/gagal/duplikat), `GET /api/v1/health`. |
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
| **M5**    | Topup QRIS: KentangTech API + webhook HMAC + idempotency + notif | 🔶 partial (v1.13): menu/flow ✅, API di-defer (StubGateway) |
| **M6**    | Trial, notifikasi kadaluarsa, sync traffic, multi-server, perintah admin | 4–5 hari |
| **M7**    | Hardening: test penuh, race, load test, staging, UAT, dokumentasi | 3–4 hari |
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
| Gangguan panel saat order diproses | Order failed | Retry 2× backoff; status `failed` tanpa debit; user bisa ulang |
| Kehilangan kredibilitas harga (salah hitung) | Klaim refund | Harga hanya dari `pricing`; audit `balance_before/after`; test aritmatika `Money` |
| API payment (KentangTech) berubah / down | Topup gagal, komplain user | Kontrak dipin §15.7; retry + timeout 30s; alert admin; pesan ramah ke user |
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
| POST | `/api/v1/webhooks/payments` | HMAC `X-KTS-Signature` | Callback pembayaran QRIS | M5 |
| POST | `/api/v1/servers` | `X-API-Key` | Registrasi server X-UI (porting `server_registration.py`) | M2/M6 |
| GET | `/api/v1/servers` | `X-API-Key` | List server | M6 |
| GET | `/api/v1/servers/{id}` | `X-API-Key` | Detail server | M6 |
| PATCH | `/api/v1/servers/{id}` | `X-API-Key` | Update server | M6 |
| DELETE | `/api/v1/servers/{id}` | `X-API-Key` | Hapus server | M6 |
| GET | `/api/v1/servers/{id}/health` | `X-API-Key` | Health per server | M2 |
| GET | `/api/v1/orders` (v1.1, admin) | `X-API-Key` | List/statistik order | nanti |
| GET | `/api/v1/orders/{orderId}` | `X-API-Key` | Detail order | nanti |
| GET | `/api/v1/users/{telegramID}/orders` | `X-API-Key` | Riwayat order user | nanti |
| GET | `/api/v1/users/{telegramID}/clients` | `X-API-Key` | Akun VPN user | nanti |
| POST | `/api/v1/payments/topups` (v1.1, admin) | `X-API-Key` | Trigger topup QRIS | nanti |

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
