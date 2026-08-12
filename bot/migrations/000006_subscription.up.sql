-- 000006_subscription.up.sql — FR-13: simpan subId panel + URL subscription.
-- sub_id            = nilai yang dikirim ke panel (spec.SubID, UUID) — basis
--                     URL subscription; akun lama (sebelum fitur ini) NULL
--                     (legacy gap terdokumentasi, tanpa backfill).
-- subscription_json_url = URL JSON/Clash sub (kosong bila SUB_JSON disabled).
ALTER TABLE vpn_clients
    ADD COLUMN sub_id text,
    ADD COLUMN subscription_json_url text;
