-- 000003_expiry_notify_day.down.sql
-- Downgrade: nilai integer > 0 (sudah pernah dinotifikasi) kembali ke TRUE.
-- Default integer (0) tidak bisa auto-cast ke boolean — drop dulu.
ALTER TABLE vpn_clients ALTER COLUMN notified_expiry DROP DEFAULT;
ALTER TABLE vpn_clients
    ALTER COLUMN notified_expiry TYPE BOOLEAN
    USING (notified_expiry > 0);
ALTER TABLE vpn_clients ALTER COLUMN notified_expiry SET DEFAULT FALSE;
