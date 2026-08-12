-- 000006_subscription.down.sql
ALTER TABLE vpn_clients
    DROP COLUMN sub_id,
    DROP COLUMN subscription_json_url;
