-- 000002_add_panel_insecure.up.sql — per-panel TLS verification switch (M4 review fix).
-- Default FALSE: panel credentials never transit a MITM-able channel unless the
-- operator explicitly opts into insecure TLS for a self-signed staging panel.
ALTER TABLE vpn_servers ADD COLUMN insecure_tls BOOLEAN NOT NULL DEFAULT FALSE;
