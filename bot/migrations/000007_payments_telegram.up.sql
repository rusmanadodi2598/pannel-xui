-- 000007_payments_telegram.up.sql — topup settlement notif (FR-06, Phase 4).
-- payments.user_id is the DB id; the webhook-triggered topup notification
-- needs the Telegram id to deliver the success message to the user.
ALTER TABLE payments ADD COLUMN telegram_id BIGINT NOT NULL DEFAULT 0;
CREATE INDEX idx_payments_telegram ON payments (telegram_id);
