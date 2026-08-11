-- 000003_expiry_notify_day.up.sql — FR-09: notified_expiry BOOLEAN → INTEGER.
-- Semantik: 0 = belum pernah dinotifikasi, N = ambang (hari) terakhir yang sudah
-- dikirim (7/3/1). Notifikasi multi-ambang H-7/H-3/H-1 butuh menyimpan ambang,
-- bukan flag biner — kolom bool lama hanya bisa "sudah pernah / belum pernah".
-- Default lama (FALSE) tidak bisa auto-cast ke integer — drop dulu.
ALTER TABLE vpn_clients ALTER COLUMN notified_expiry DROP DEFAULT;
ALTER TABLE vpn_clients
    ALTER COLUMN notified_expiry TYPE INTEGER
    USING (CASE WHEN notified_expiry THEN 7 ELSE 0 END);
ALTER TABLE vpn_clients ALTER COLUMN notified_expiry SET DEFAULT 0;
