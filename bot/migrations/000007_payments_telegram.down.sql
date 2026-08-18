-- 000007_payments_telegram.down.sql
DROP INDEX IF EXISTS idx_payments_telegram;
ALTER TABLE payments DROP COLUMN IF EXISTS telegram_id;
