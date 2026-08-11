-- 000001_init.down.sql — rollback schema 000001 (urutan terbalik dari FK).

DROP TABLE IF EXISTS pricing;
DROP TABLE IF EXISTS payments;
DROP TABLE IF EXISTS balance_transactions;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS vpn_clients;
DROP TABLE IF EXISTS vpn_servers;
DROP TABLE IF EXISTS users;
