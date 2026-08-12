-- 000004_inbound_stream_path.up.sql — v1.27: simpan transport asli inbound
-- (network + path dari streamSettings) agar dual config link memakai path
-- nyata per inbound (/vlessws, trojan-grpc, …), bukan /{protocol} hardcoded.
ALTER TABLE vpn_clients
    ADD COLUMN inbound_network text NOT NULL DEFAULT '',
    ADD COLUMN inbound_path text NOT NULL DEFAULT '';
