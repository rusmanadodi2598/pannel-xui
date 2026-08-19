# Checklist E2E Topup — PG Aggregate (v1.48)

> Verifikasi end-to-end alur topup: **pilih nominal → persist order → charge →
> checkout QRIS → bayar → webhook `pg.charge` → kredit saldo → akun VPN**.
> Referensi: `docs/001-PRD-BOT-ORDER.md` §15.7 (kontrak) & FR-06; `docs/002` §4.
> Status: 🔲 belum · ✅ lolos · ❌ gagal (wajib fix + re-test).

## 0. Prasyarat (sekali sebelum mulai)

- [ ] Merchant onboarding backend (`create-merchant`) selesai → dapat `API key`
      + `secretKey` (sekali tampil) → isi `.env` staging:
      - `KTS_BASE_URL=https://gateway.kentangtechstore.com`
      - `KTS_API_KEY=<api-key>`
      - `KTS_SECRET=<secret-key>`
      - `KTS_CHARGE_TTL_MIN=1440`
- [ ] `webhook_url` merchant di sisi gateway diarahkan ke
      `https://{BOT_DOMAIN}/api/v1/webhooks/payments` (deploy note §15.7).
- [ ] Bot di-restart setelah `.env` diisi; boot TIDAK fail (config `KTS_*`
      required fail-fast — kalau kosong boot gagal, bukan jalan diam-diam).
- [ ] `https://{BOT_DOMAIN}/api/v1/health` = 200.
- [ ] PostgreSQL & Redis reachable; migrasi `000007` sudah diterapkan
      (`payments.telegram_id` ada, `SELECT column_name FROM information_schema.columns WHERE table_name='payments'`).
- [ ] User test sudah terdaftar + join grup + saldo awal diketahui
      (`SELECT id, telegram_id, balance FROM users WHERE telegram_id=<id>`).

## 1. Create → Checkout (jalur utama)

- [ ] `/topup` → pilih nominal (mis. Rp 10.000) → konfirmasi.
- [ ] Order topup ter-persist **sebelum** panggil gateway:
      `SELECT order_id, order_type, status, amount FROM orders WHERE order_id LIKE 'tp_%' ORDER BY created_at DESC LIMIT 1;`
      → `order_type=topup`, `status=pending`, `amount=10000` (NET).
- [ ] Baris `payments` terbuat dengan `amount_net` = nominal & `telegram_id` user:
      `SELECT order_id, amount_net, status, telegram_id FROM payments WHERE order_id='tp_...';`
- [ ] Bot membalas dengan **link checkout QRIS** (caption memuat order id,
      saldo diterima, total bayar, fee — `TopupPaymentText`).

## 2. Bayar & settlement (butuh provider nyata)

- [ ] Scan/buka checkout → bayar QRIS di sandbox/live Midtrans.
- [ ] Gateway mengirim webhook `pg.charge` → bot settle → user dapat pesan
      `TopupSettledText` + grup admin dapat `AdminTopupNoticeText`.
- [ ] Saldo user bertambah **NET** (bukan gross):
      `SELECT balance FROM users WHERE telegram_id=<id>;` → `balance_awal + 10000`.
- [ ] Ledger kredit tercatat dengan ref = orderId:
      `SELECT order_id, type, amount FROM balance_transactions WHERE user_id=<pk> ORDER BY id DESC LIMIT 3;`
- [ ] `payments.status` = `success` & `orders.status` = `completed`.

## 3. Simulasi webhook (tanpa bayar nyata — smoke test settlement)

> Untuk memverifikasi jalur kredit/dedup/signature **tanpa** menunggu pembayaran
> nyata. PENTING: `orderId` harus **topup pending yang benar-benar ada** di
> `payments` (dibuat lewat `/topup` di step 1), bukan id karangan.

Hitung signature & kirim `succeeded`:

```bash
SECRET="<KTS_SECRET>"
BOT_BASE="https://{BOT_DOMAIN}"
ORDER_ID="tp_...            # ambil dari step 1"
BODY=$(printf '{"eventType":"pg.charge","orderId":"%s","refId":"%s","status":"succeeded","amount":{"amount":10280,"currency":"IDR"},"providerTrxId":"sim-1","paidAt":"2026-08-18T10:00:00Z","occurredAt":"2026-08-18T10:00:00Z"}' "$ORDER_ID" "$ORDER_ID")
SIG="sha256=$(printf '%s' "$BODY" | openssl dgst -sha256 -hmac "$SECRET" -hex | awk '{print $2}')"
curl -i -X POST "$BOT_BASE/api/v1/webhooks/payments" \
  -H "X-Webhook-Signature: $SIG" \
  -H "X-Webhook-Event: pg.charge" \
  -H "X-Webhook-Id: pg.charge.$ORDER_ID.succeeded" \
  -H "Content-Type: application/json" \
  --data "$BODY"
```

- [ ] Respon `200 ok`; saldo kredit NET sekali (cek step 2 SQL).

## 4. Kasus negatif & idempotensi

- [ ] **Signature salah** → kirim ulang dengan `SIG` diubah → `403` dan saldo
      TIDAK berubah.
- [ ] **Event bukan `pg.charge`** (`X-Webhook-Event: vpn.purchase`) → `400`.
- [ ] **Duplikat `X-Webhook-Id`** → kirim ulang persis sama → `200 dedup:true`,
      saldo TIDAK kredit kedua kali (anti double-credit).
- [ ] **`failed` / `expired`** → body `status:"expired"` dengan orderId pending
      lain → `200` tanpa kredit; `payments.status` = `expired`/`failed`,
      `orders.status` = `failed`.
- [ ] **orderId tidak dikenal** → `5xx` (gateway akan retry — pastikan log
      `settlement failed` tercatat, bukan crash).

## 5. Close loop: beli akun pakai saldo baru

- [ ] Setelah saldo kredit, `/beli` → pilih negara/inbound/paket → konfirmasi.
- [ ] Debit-first: order `completed`, saldo berkurang presisi, akun dibuat di
      panel, config link/sub URL terkirim.
- [ ] Notice order sukses masuk grup admin (`NOTIFICATION_GROUP_ID`).

## Exit criteria

- [ ] Semua kotak di atas ✅.
- [ ] Demo end-to-end **beli → QRIS → akun aktif** terekam (order id + saldo
      sebelum/sesudah + akun panel).
- [ ] Tidak ada saldo minus, tidak ada double-credit/double-charge.
