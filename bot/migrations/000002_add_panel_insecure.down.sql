-- 000002_add_panel_insecure.down.sql
ALTER TABLE vpn_servers DROP COLUMN insecure_tls;
