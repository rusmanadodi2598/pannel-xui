# UAT Checklist — Bot Auto-Order (M7)

> Checklist UAT (User Acceptance Testing) untuk milestone M7 (Hardening).
> Referensi: `docs/001-PRD-BOT-ORDER.md` (FR-01 s.d. FR-15, exit criteria v1 §M7).
> Scope: hanya direktori `/bot` — panel x-ui **tidak boleh berubah**.
> Status: 🔲 belum · ⬜ tidak berlaku · ✅ lolos · ❌ gagal (wajib fix + re-test)

---

## 0. Pre-flight (staging)

- [ ] `go build ./...` hijau (root + `bot/`)
- [ ] `go vet ./...` hijau
- [ ] `go test -race ./...` hijau (19 package, termasuk integration PG/Redis)
- [ ] golangci-lint penuh `bot/` bersih (config `.golangci.yml`, v1.64.x)
- [ ] `gofmt -l .` kosong; semua file < 250 baris (AGENTS.md §1.1)
- [ ] Migrasi boot (`golang-migrate` embed) diterapkan bersih di staging DB
- [ ] `.env` staging lengkap: `BOT_TOKEN`, `BOT_DOMAIN` (HTTPS publik),
      `DATABASE_URL`, `REDIS_URL`, `ENCRYPTION_KEY`, `ADMIN_IDS`, `PANEL_N_*`
- [ ] Webhook terdaftar (`setWebhook` 200); health `/healthz` = 200
- [ ] Redis & PG reachable; pool limits sesuai AGENTS.md §1.7

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
- [ ] Beli → pilih negara → pilih paket → pilih server (hanya `is_open=true`) →
      konfirmasi → saldo cukup → **debit + akun X-UI dibuat** (idempoten)
- [ ] Saldo tidak cukup → ditolak sebelum order dibuat
- [ ] Panel error saat provisioning → order `failed`, **tanpa debit**
- [ ] Debit gagal → order `failed`, client tetap tercatat (rollback aman)
- [ ] Renew: perpanjang expiry dari **sisa waktu** (bukan double-count)
- [ ] Akun Saya: list + status (✅/⚠️/❌), sisa waktu, traffic, server
- [ ] Konfigurasi akun: config link TLS & Non-TLS, convert YAML (AC-2) — *jika sudah diimplementasi* (M4 subset masih read-only)
- [ ] Hapus akun 2 langkah konfirmasi (AC-4) — jika sudah diimplementasi

## 3. Trial (FR-07)

- [ ] `/trial` → cek fitur aktif + sisa kuota harian (2×/hari)
- [ ] Pilih server (aktif saja) → konfirmasi → akun trial 1 jam/1 GB/1 IP
      (**tanpa debit**)
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

## 8. Keamanan & operasional

- [ ] Webhook secret: body tanpa/token salah → 403 tanpa diproses
- [ ] HMAC/token tidak bocor ke log; kredensial panel terenkripsi AES-256-GCM
- [ ] Worker panic (update rusak) → process tetap hidup (panic-recover §1.6)
- [ ] Graceful shutdown (SIGTERM): server + worker + notifier + traffic drain,
      tidak ada goroutine bocor
- [ ] Panel x-ui: `git status` di repo panel **bersih** (tidak tersentuh)

## 9. Load test (M7)

- [ ] Benchmark worker pool: `go test -run '^$' -bench BenchmarkWorker ./internal/handler/http/`
      — catat hasil, bandingkan baseline (queue cap = batch, tanpa drop):
      - cheap handler ≥ ~100k ops/s (8 worker, batch 1.000)
      - realistic (1 ms) ≥ ~3k ops/s (8 worker, batch 200)
      - drain 1.000 update (batch 1.000) ≤ ~350 ms
- [ ] Burst 1.000 update dalam 1 dtk → pipeline memproses batch penuh tanpa
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
