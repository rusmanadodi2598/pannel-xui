# UAT Checklist — Bot Auto-Order (M7)

> Checklist UAT (User Acceptance Testing) untuk milestone M7 (Hardening).
> Referensi: `docs/001-PRD-BOT-ORDER.md` (FR-01 s.d. FR-15, exit criteria v1 §M7).
> Scope: hanya direktori `/bot` — panel x-ui **tidak boleh berubah**.
> Status: 🔲 belum · ⬜ tidak berlaku · ✅ lolos · ❌ gagal (wajib fix + re-test)

---

## 0. Pre-flight (staging)

- [x] `go build ./...` hijau (root + `bot/`) — v1.22, 20 package
- [x] `go vet ./...` hijau
- [x] `go test -race ./...` hijau (20 package, termasuk integration PG/Redis)
- [x] golangci-lint penuh `bot/` bersih (config `.golangci.yml`, v1.64.x)
- [x] `gofmt -l .` kosong; semua file < 250 baris (AGENTS.md §1.1)
- [x] Migrasi boot (`golang-migrate` embed) diterapkan bersih di staging DB
      (8 tabel; `schema_migrations` utuh)
- [x] `.env` staging lengkap: `BOT_TOKEN`, `BOT_DOMAIN` (HTTPS publik),
      `DATABASE_URL`, `REDIS_URL`, `ENCRYPTION_KEY`, `ADMIN_IDS`,
      `PANEL_1_*` (id2.kentangtechstore.net:54321), `TRIAL_*`
- [x] Webhook terdaftar (`setWebhook` 200, `pending:0`, `last_error:none`);
      health publik `https://bot-xui.kentangtechstore.com/api/v1/health` = 200
- [x] Redis & PG reachable; pool limits sesuai AGENTS.md §1.7
- [x] TLS: nginx + Let's Encrypt, Cloudflare proxy aktif, UFW 80/443 dibuka
- [x] Bot binary M6/M7 (build 15:37), run via `setsid`, PPID 1, health ok

## 1. Gate & akses (FR-01)

- [ ] User belum join grup → prompt join + tombol "✅ Sudah Join"
- [ ] User sudah join → langsung masuk menu; hasil membership di-cache (6 jam)
- [ ] User di-ban → semua aksi ditolak (fail-closed)
- [ ] `ADMIN_IDS` **bypass** gate grup
- [ ] Rate limit 30 req/menit/user: > 30 → ditolak dengan pesan ramah
- [ ] `update_id` duplikat (kirim ulang webhook) → dedup, tidak diproses 2×
- [ ] Per-user lock: 2 update user yang sama tidak diproses paralel

## 2. Order flow (FR-02/03/04/05/08)

- [ ] `/start` → menu utama (7 tombol: Beli, Top Up, Akun Saya, Trial, dst.)
      *(icon policy: icon hanya di tombol navigasi Home/Back/Cancel/Next/Prev;
      tombol aksi text-only)*
      *(staging ready: pricing 12 plan, panel id2 seeded, gate grup aktif)*
- [ ] Beli → pilih negara → **pilih inbound (server + protocol) dari panel**
      (vless reality/ws, vmess, trojan, shadowsocks, hysteria, grpc, dll) →
      pilih paket → konfirmasi → saldo cukup → **debit + akun X-UI dibuat
      pada inbound terpilih** (idempoten)
- [ ] Saldo tidak cukup → ditolak sebelum order dibuat
- [ ] Panel error saat provisioning → order `failed`, **tanpa debit**
- [ ] Debit gagal → order `failed`, client tetap tercatat (rollback aman)
- [ ] Renew: perpanjang expiry dari **sisa waktu** (bukan double-count)
- [ ] **Renew paid-only + idempotence (v1.37)**: akun trial TIDAK muncul di
      menu renew & callback renew ke trial → "Akun tidak ditemukan";
      eksekusi duplikat saat order masih pending/processing → ditolak
      `ErrOrderInFlight` (tanpa order/debit kedua); renew sukses → order
      `renewal completed`, debit tepat sekali, expiry DB = expiry panel
- [ ] **Renew saldo presisi debit-first + auto-refund (v1.37)**: panel gagal
      setelah debit → refund atomik (ledger credit orderID sama) + order
      `failed`, expiry TIDAK berubah; saldo tidak pernah minus (guard SQL)
- [ ] **Fix panel renew "empty client ID" (v1.38)**: `updateClient` x-ui
      mengganti seluruh objek client — renew memuat spec penuh client dari
      settings panel (totalGB/limitIp/flow/reverse tetap) dan hanya menaikkan
      `enable`+`expiryTime`; kunci panel per-protocol `PanelClientKey`
      (vless/vmess→UUID, trojan/hysteria→password, ss→email); **verified
      staging**: order `KTS-BUMMHXLK-VPN` completed, saldo 46000→42000 (debit
      4000 sekali, tanpa refund), expiry DB 2026-08-27→2026-09-11 (+15 hari
      dari sisa), panel expiry = DB, `totalGB=100`/`limitIp=1` dipertahankan
- [ ] Akun Saya: list + status, sisa waktu, server + tombol `Lihat Detail` per akun
- [ ] **Pagination akun (FR-08 AC-1, v1.30)**: `account:menu` → 5 akun/halaman
      (newest first) → `account:page:{n}` navigasi; indikator non-aksi
      (`account:noop`) dijawab tanpa edit; halaman di luar range di-clamp;
      header "Halaman X dari Y" tampil
- [ ] **Status display list (FR-08 AC-1, v1.34)**: per item menampilkan status
      teks `Aktif` / `Hampir Habis` / `Expired` (Hampir Habis = akun nonaktif
      atau kuota ≥90% — parity threshold AC-3; badge teks polos, TANPA emoji
      di body copy — icon policy), badge `Trial ·` untuk akun trial, sisa
      waktu smart (jam utk sisa <24 jam — akun trial 1 jam, hari utk paid)
- [ ] Sisa waktu parity (v1.34): akun 5 hari → "sisa 6 hari" (format lama
      tetap), akun trial baru → "sisa 1 jam", expired → "sudah habis"
- [ ] **Hapus akun 2 langkah (FR-08 AC-4, v1.31)**: detail akun → tombol
      `Hapus Akun` (`account:delete:{id}`) → halaman konfirmasi (peringatan
      tidak bisa dikembalikan) → `Ya, Hapus` (`account:delete_confirm:{id}`)
      → panel `delClient` dulu, DB row setelah; panel gagal → DB row tetap;
      akun milik user lain → ditolak; sukses → pesan "Akun dihapus"
- [ ] **Detail akun AC-1 (v1.35)**: detail menampilkan **Limit IP** dan
      **traffic terpakai/kuota** (parity AC-1 penuh); ekspor `.txt`
      menyertakan keduanya (`Traffic : X / Y GB`)
- [ ] **Hapus akun tercatat di Riwayat (AC-4, v1.35)**: setelah hapus sukses,
      transaksi tipe `Hapus Akun` (status Selesai, nominal "—") muncul di
      `Riwayat` (FR-14) milik user; hapus gagal (panel/DB) → TIDAK tercatat
- [ ] Detail akun (v1.25): config link (share URI per protocol) tampil lengkap
- [ ] Ekspor akun (v1.25): tombol `Ekspor .txt` mengirim dokumen berisi kredensial + config link
- [ ] Config V2Ray (v1.26): detail akun → tombol `Config V2Ray` (`account:config:{id}`) menampilkan **2 URL** (TLS 443 / Non-TLS 80) + detail konfigurasi (domain, ID/password, network ws, path) — format reference client-vpn
- [ ] **Path dinamis (v1.27)**: akun baru di inbound ws menampilkan path asli API (`/vlessws`, `/vmessws`, `/trojanws`, serviceName `trojan-grpc`) — bukan `/{protocol}` hardcoded; Network + Path/Service Name tampil dinamis
- [ ] **Reality/tcp & ss/hysteria (v1.27)**: akun dari inbound reality → TIDAK ada link ws palsu, fallback ConfigLink native
- [ ] **Legacy row (v1.27)**: akun lama (network kosong) tetap menampilkan link ws `/{protocol}` (backward compat)
- [ ] Migrasi `000004` (v1.27): boot di staging menerapkan `inbound_network`/`inbound_path`; detail akun lama tetap tampil normal
- [ ] Config V2Ray keyboard: kembali ke detail (`account:view:{id}`) + home; tombol aksi tanpa icon (policy)
- [ ] Ekspor `.txt` (v1.26/v1.27): dokumen berisi **kedua URL** dengan path dinamis; akun ss/hysteria/reality → ConfigLink native
- [ ] **Traffic + refresh manual (FR-08 AC-3, v1.32)**: detail akun → tombol
      `Traffic` (`account:traffic:{id}`) → sync live panel dulu → halaman usage:
      **progress bar** 10 blok + **status warna 🟢🟡🔴** (≥90% Hampir Habis,
      ≥70% Perhatian, else Normal), Upload/Download/Total/Kuota/Sisa + waktu
      sync terakhir; kuota Unlimited → tanpa bar; tombol `Refresh` memanggil
      callback sama (re-sync); panel error → tetap render data terakhir (best
      effort); akun milik user lain → ditolak tanpa refresh
- [ ] Traffic kebenaran data (v1.32): setelah refresh, angka Upload/Download/
      Total match panel (`getClientTraffics/:email` — email lookup, protocol-
      agnostic); `last_sync` ter-update
- [ ] Sukses Beli/Trial: tombol `Config V2Ray` tersedia (1 tap menuju config view)
- [ ] NTLS port 80 (caveat): verifikasi link non-TLS; bila 301/redirect, catat keputusan (arahkan ke port non-TLS panel atau buka location nginx)
- [ ] Path vs nginx (v1.27 caveat): path dual link WAJIB match lokasi nginx (`location /vlessws` dst) — bila belum ada, tambahkan mapping nginx atau catat sebagai keputusan manual
- [ ] **Convert YAML Clash/Meta (FR-08 AC-2, v1.33)**: detail akun → tombol
      `Convert YAML` (`account:convert:{id}`) → 2 blok proxy `proxies:`
      (TLS 443 + Non-TLS 80): `type` per protocol, credential benar (trojan →
      password asli, bukan uuid), `tls`/`servername`/`skip-cert-verify`,
      transport asli (`ws-opts.path` dinamis / `grpc-opts.grpc-service-name`),
      `udp: true`; tag proxy konsisten dengan remark URL dual-link
- [ ] Convert YAML fallback (v1.33): akun reality/ss/hysteria → pesan "tidak
      tersedia" + ConfigLink native (TANPA blok YAML ws palsu); keyboard:
      `URL Config` (`account:config:{id}`) + kembali + home; tombol aksi
      icon-free (policy); akun user lain → ditolak
- [ ] Riwayat (FR-14): list order user pagination 5/halaman, newest first
      (`history:menu` → `history:page:{n}`); tipe & status dilabeli
- [ ] Detail order (FR-14): order ID, tipe, status, nominal, tanggal, akun
      terkait — **hanya order milik user** (foreign → "Transaksi tidak
      ditemukan")
- [ ] Riwayat kosong → pesan "belum punya transaksi" + tombol Beli/Top Up;
      pagination indikator non-aksi (`history:noop`) dijawab tanpa edit
- [ ] Bantuan (FR-15, v1.29): `help:menu` → 4 kategori (Cara Order, Cara Top
      Up, Disclaimer & ToS, Info) — setiap halaman punya tombol kembali
      (`⬅️ Kembali`) & `🏠 Menu Utama` (icon policy: aksi text-only)
- [ ] `help:order` / `help:topup`: konten statis id-ID + shortcut aksi
      (Beli VPN / Top Up) langsung ke flow
- [ ] `help:disclaimer` → `help:tos:account` / `help:tos:payment`: konten
      ToS lengkap (larangan, sanksi, kebijakan saldo/refund); navigasi
      silang antar ToS + kembali ke disclaimer

## 3. Trial (FR-07)

- [ ] `/trial` → cek fitur aktif + sisa kuota harian (2×/hari)
- [ ] Pilih server (aktif saja) → **pilih protocol dari panel** (vless reality,
      vmess, trojan, shadowsocks, hysteria, grpc, dll) → konfirmasi → akun
      trial 1 jam/1 GB/1 IP pada inbound terpilih (**tanpa debit**)
- [ ] Limit harian tercapai → ditolak ramah; counter reset tengah malam
      (`TIME_LOCATION`)
- [ ] Claim ganda (2 user menekan konfirmasi bersamaan) → hanya 1 berhasil
      (anti-race)
- [ ] Trial muncul di Akun Saya dengan badge 🎁; tombol "Beli VPN Premium"

## 4. Topup (FR-06, M5 partial)

- [ ] Top Up → pilih nominal / input custom → ringkasan (fee QRIS + PPN)
- [ ] Nominal < MIN / > MAX → ditolak dengan pesan jelas
- [ ] Konfirmasi → stub gateway: teks unavailable yang ramah (**tanpa panic**)
- [ ] FSM custom-input: teks tidak valid → re-prompt; `/cancel` & `/start`
      membersihkan FSM
- [ ] *API KentangTech (QRIS) masih di-defer* — verifikasi ulang saat Go
      rewrite ship (StubGateway → client nyata)

## 5. Admin (FR-11)

- [ ] `/admin` dari non-admin → ditolak; dari `ADMIN_IDS` → menu admin
- [ ] Harga: list semua plan (termasuk disabled) → ubah harga → tampil baru
- [ ] Toggle plan on/off → hilang/muncul dari menu beli
- [ ] Reload seed → sinkronkan harga **tanpa** membalik toggle admin
- [ ] Broadcast: input teks → pratinjau → kirim chunked 100/6 dtk → laporan
      selesai; broadcast kedua saat masih jalan → ditolak (lock)
- [ ] Ban/unban: input ID → konfirmasi → gate marker Redis + flag DB
- [ ] Callback `admin:*` di-re-check `ADMIN_IDS` di setiap surface
- [ ] **Adjust saldo admin (FR-11, v1.39)**: `admin:saldo` → `+ Kredit` /
      `- Debit` → ketik Telegram ID (unknown → "belum terdaftar") → ketik
      nominal → konfirmasi → saldo user berubah + **ledger tercatat dengan
      ref `ADJ-...`**; debit melebihi saldo → ditolak "tidak mencukupi";
      non-admin → ditolak; tap ganda tombol Konfirmasi → eksekusi SATU kali
- [ ] **Manajemen server (FR-11, v1.40)**: `admin:server` → daftar semua
      panel (aktif+nonaktif) → detail per server → **Toggle Buka**
      (`admin:server:open:{id}`): negara server hilang dari pilihan Beli/Trial
      dan muncul lagi setelah di-toggle balik → **Toggle Aktif**
      (`admin:server:active:{id}`): server dikecualikan dari sync traffic
      saat nonaktif; aksi non-admin → ditolak; setiap toggle tercatat di
      Audit Log
- [ ] **Tambah server (FR-11, v1.40)**: `admin:server:add` → FSM 6 langkah
      (nama → host → port → username → password → negara) — input invalid
      (port non-angka/out-of-range, negara kosong) → re-prompt langkah yang
      sama; `/cancel` membatalkan; konfirmasi → server baru muncul di daftar;
      **password TIDAK tampil di chat manapun** (disegel AES-256-GCM,
      `password_enc`); tap ganda tombol Konfirmasi → hanya SATU server dibuat;
      tambah server dengan host+port+username duplikat → ditolak
- [ ] **Statistik (FR-11, v1.40)**: `admin:stats` → total/today orders &
      revenue (hanya order `completed`), breakdown status (completed/failed/
      pending/processing/cancelled/refunded), total user, client aktif;
      `Order Terbaru` → 10 order bounded (newest first); angka revenue match
      `SUM(final_amount)` order completed di DB
- [ ] **Audit log (FR-11, v1.40)**: `admin:audit` → 15 baris terbaru tiap aksi
      admin (harga, toggle plan, reload, ban/unban, adjust saldo, broadcast,
      toggle/tambah server); kolom siapa (admin id), aksi, target, detail;
      tidak ada password/token di detail; tabel `admin_audit_log` ter-migrasi
      (index `created_at DESC`)
- [x] **Notifikasi order ke grup admin (FR-04 AC, v1.41)**: order beli sukses
      (saldo cukup, panel OK) → pesan notice masuk ke `NOTIFICATION_GROUP_ID`
      (order id, tipe, user, paket, nominal, email akun, aktif sampai, sisa
      saldo; body emoji-free); renew sukses → notice `Perpanjang` masuk;
      order GAGAL (panel/saldo) → TIDAK ada notice; trial sukses → TIDAK ada
      notice; `NOTIFICATION_GROUP_ID=0` (kosong) → tidak ada notice sama sekali
      *(verified staging v141: beli ×2 + renew notice terkirim ke grup join;
      trial ditolak 2/2 & beli saldo kurang → tanpa notice; email tampil
      penuh — masking test: uuid/password/config link tidak pernah bocor)*
- [x] **Keyboard layout zigzag (UX, v1.42)**: semua sub-menu TIDAK lagi
      vertikal 1-1-1-1 — tombol ditata pola **2-1-2-1-2** (baris pertama 2
      tombol, berikutnya 1, berikutnya 2, dst. sesuai jumlah tombol yang
      ada; baris terakhir mengambil sisa); urutan tombol & callback data
      TIDAK berubah; **pager** list Akun/Riwayat tetap baris sendiri; menu
      utama, quick-pick Top Up, kategori Bantuan tetap 2-kolom (tidak
      diubah); handler routing tidak terpengaruh (semua baca keyboard
      secara flat)
      *(verified staging v142: deploy + 7 callback webhook nyata
      (buy:menu, admin:menu, account:menu, trial:menu, buy:country:ID,
      topup:menu, renew:menu) → semua `ok`; dump layout data staging —
      BuyCountries 4 negara `[🇮🇩,🇸🇬] [🇯🇵] [🇨🇳,🏠]`, Inbounds
      `[vless,vmess] [trojan] [⬅️]`, AdminMenu 9 tombol
      `[Harga,Server] [Broadcast] [Ban,Unban] [Adjust] [Statistik,Audit]
      [🏠]` (6 baris), AccountDetail `[Traffic,Config] [Convert]
      [Ekspor,Hapus] [⬅️]`, BuySuccess `[Config,Ekspor] [🏠]`, konfirmasi
      2-tombol `[Konfirmasi,⬅️]`/`[Ya Hapus,Batal]`/`[Refresh,⬅️]` satu
      baris)*
- [x] **Brand KENTANG TECH pada template notifikasi (v1.43 → v1.44)**: banner
      `🏪 KENTANG TECH` + separator `━━━` tampil di awal SEMUA pesan
      transaksi: notice order ke grup admin (FR-04), reminder kadaluarsa
      (FR-09), pesan sukses Beli/Perpanjang/Trial, **ringkasan konfirmasi
      (beli/perpanjang/trial, v1.44)**, **pesan gagal Beli/Trial (v1.44)**, ringkasan
      & QR Top Up; brand = **KENTANG TECH** — TIDAK pernah "KENTANG TECH
      STORE"; banner brand = satu-satunya emoji di body copy (icon policy
      exception, keputusan user); sisa pesan tetap emoji-free
      *(verified: unit test `menu_brand_test.go` — semua template
      HasPrefix `🏪 KENTANG TECH` tanpa STORE; E2E staging v143/v144 beli →
      pesan sukses & notice grup memuat banner)*
- [x] **Ejaan brand seragam (v1.44)**: header ekspor `.txt` =
      `=== AKUN VPN KENTANG TECH ===` (bukan KENTANGTECH), sambutan `/start`
      & `help:info` memakai `KENTANG TECH VPN Bot` (bukan "KentangTech VPN
      Bot") — tidak ada ejaan legacy (KentangTech/KENTANGTECH/KENTANG TECH
      STORE) di copy user-facing *(verified: unit test
      `TestBrandSpelling_ThenSingleBrandEverywhere` — txt/home/info memuat
      BrandName, tanpa ejaan lama; cek manual di Telegram: pesan sukses &
      ekspor .txt)*

## 6. Notifikasi kadaluarsa (FR-09)

- [ ] Client masuk jendela H-7 → dapat pengingat 7 hari; **sekali per ambang**
- [ ] H-3, H-1 → pengingat masing-masing sekali (tidak spam)
- [ ] Renewal → siklus notifikasi reset (dapat H-7 lagi)
- [ ] Trial / nonaktif / expired → tidak di-notifikasi
- [ ] Tanggal di pesan sesuai `TIME_LOCATION`; pesan tanpa emoji (copy policy)

## 7. Sync traffic (PRD §16.2)

- [ ] Setelah client dipakai (traffic panel > 0) → `traffic_used/up/down`
      ter-update di DB dalam ≤ 1 interval (default 5 mnt)
- [ ] Client online saat sweep → `last_online` ter-update; offline → dipertahankan
- [ ] Satu panel mati → server lain tetap sync; error ter-log (bukan crash)
- [ ] Client dihapus dari panel → di-skip (bukan gagal)
- [ ] Sweep tidak overlap (interval vs durasi sweep)

## 7b. Health check panel (v1.45, PRD §17)

- [ ] Boot → worker health check langsung sweep pertama; semua panel staging
      yang reachable berstatus `ok` (`health_status='ok'`, `last_health_check`
      terisi) — cek via `admin:server` atau SQL
- [ ] Panel dimatikan (stop proses panel / salahkan host) → dalam ≤ 1 interval
      (default 60 dtk) `health_status='down'`; panel **hilang dari menu
      Beli/Trial** (server mati tidak dijual) — negara tetap tampil bila
      masih ada server lain yang `ok`/`unknown`
- [ ] Panel dihidupkan lagi → status balik `ok` dalam ≤ 1 interval → muncul
      lagi di menu Beli/Trial
- [ ] Server nonaktif (`is_active=false`) → tidak di-ping (bukan health target)
- [ ] Satu panel gagal → server lain tetap di-check; error ter-log (bukan crash)
- [ ] Status `down` terlihat di admin server list (`admin:server`); bot tetap
      melayani update lain (worker tidak memblokir webhook)
- [ ] **Fix context write (E2E v1.45)**: panel mati yang connect-timeout
      (menghabiskan budget XUI 30s) TETAP tercatat `down` + `last_health_check`
      terisi — DB write memakai context terpisah (verified staging: fake server
      `192.0.2.1:1` → `down` terpersist, hilang dari buy:menu; sebelum fix,
      write gagal `context deadline exceeded` dan status tetap `unknown`)
- [ ] **Semantik health 'unknown'**: panel baru (belum pernah di-check) tetap
      dijual sampai health sweep pertama menandainya — boot pertama tidak
      menghilangkan semua server

## 7c. Trial cleanup (v1.45, PRD worker)

- [ ] Akun trial expired (1 jam lewat) → dalam ≤ 1 interval (default 15 mnt)
      di-disable di panel (`enable=false`, spec lain dipertahankan) + DB
      `is_active=false`/`is_expired=true`; akun tetap tampil di Akun Saya
      (badge Trial + status Expired) & tidak mencuri kuota
- [ ] Akun trial masih aktif (belum lewat 1 jam) → TIDAK disentuh
- [ ] Akun paid yang expired → TIDAK disentuh (hanya `is_trial=true`)
- [ ] Panel gagal saat disable → row DB TIDAK ditandai (retry sweep berikutnya)
- [ ] Client trial yang sudah dihapus manual dari panel → dihitung sukses
      (skip, bukan error)
- [ ] Trial baru yang dibuat setelah sweep terakhir → tidak di-disable prematur
      (guard `expires_at <= now()`)

## 8. Keamanan & operasional

- [x] Webhook secret: body tanpa/token salah → **403 tanpa diproses**
      (verified via Cloudflare publik); GET → 405
- [ ] HMAC/token tidak bocor ke log; kredensial panel terenkripsi AES-256-GCM
      (password panel tersimpan `enc_len=60` = cipher AES-256-GCM + nonce)
- [ ] Worker panic (update rusak) → process tetap hidup (panic-recover §1.6)
- [x] Graceful shutdown (SIGTERM): log `shutdown signal received, draining` →
      `bot-order stopped cleanly`; proses keluar bersih (< 1 dtk drain)
- [ ] Panel x-ui: `git status` di repo panel **bersih** (tidak tersentuh)

## 9. Load test (M7)

- [x] Benchmark worker pool: `go test -run '^$' -bench BenchmarkWorker ./internal/handler/http/`
      — baseline v1.22 (queue cap = batch, tanpa drop):
      - cheap handler **~266k ops/s** (8 worker, batch 1.000)
      - realistic (1 ms) **~3.2k ops/s** (8 worker, batch 200)
      - drain 1.000 update (batch 1.000) **~322 ms**
- [x] Burst 1.000 update dalam 1 dtk → pipeline memproses batch penuh tanpa
      drop (benchmark drain mengukur batch yang benar-benar masuk queue)
- [ ] PG & Redis tidak saturasi saat burst (pool limits §1.7)

---

## Exit criteria v1 (PRD §M7)

- [ ] Semua AC FR-01 s.d. FR-15 lolos
- [ ] `go test -race ./...` hijau
- [ ] Demo end-to-end **beli → (QRIS saat API final) → akun aktif** di staging
- [ ] Panel x-ui tidak terkena perubahan apa pun

*Catatan: checklist diisi manual saat UAT di staging; setiap ❌ wajib
di-fix dan di-re-test sebelum tanda tangan.*
