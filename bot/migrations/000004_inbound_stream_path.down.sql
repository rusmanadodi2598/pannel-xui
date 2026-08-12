-- 000004_inbound_stream_path.down.sql
ALTER TABLE vpn_clients
    DROP COLUMN inbound_network,
    DROP COLUMN inbound_path;
