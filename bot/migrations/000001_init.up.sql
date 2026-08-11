-- 000001_init.up.sql — Bot-order initial schema (PRD §13).
-- Semua tabel: created_at/updated_at. Uang: NUMERIC(15,2) — value object Money.

CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    telegram_id   BIGINT NOT NULL UNIQUE,
    username      TEXT,
    first_name    TEXT,
    last_name     TEXT,
    phone         TEXT,
    language      TEXT NOT NULL DEFAULT 'id',
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    is_banned     BOOLEAN NOT NULL DEFAULT FALSE,
    is_admin      BOOLEAN NOT NULL DEFAULT FALSE,
    balance       NUMERIC(15,2) NOT NULL DEFAULT 0,
    total_spent   NUMERIC(15,2) NOT NULL DEFAULT 0,
    referral_code TEXT UNIQUE,
    referred_by   BIGINT REFERENCES users(id),
    last_active   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE vpn_servers (
    id                 BIGSERIAL PRIMARY KEY,
    name               TEXT NOT NULL,
    host               TEXT NOT NULL,
    port               INT NOT NULL,
    username           TEXT NOT NULL,
    password_enc       TEXT NOT NULL,           -- AES-256-GCM
    api_path           TEXT NOT NULL DEFAULT '/panel',
    use_ssl            BOOLEAN NOT NULL DEFAULT FALSE,
    country_code       TEXT NOT NULL,
    flag_emoji         TEXT NOT NULL DEFAULT '',
    location           TEXT,
    max_clients        INT,
    current_clients    INT NOT NULL DEFAULT 0,
    is_active          BOOLEAN NOT NULL DEFAULT TRUE,
    is_premium         BOOLEAN NOT NULL DEFAULT FALSE,
    is_open            BOOLEAN NOT NULL DEFAULT TRUE,
    priority           INT NOT NULL DEFAULT 0,
    maintenance_message TEXT,
    protocols          JSONB NOT NULL DEFAULT '[]',
    last_sync          TIMESTAMPTZ,
    last_health_check  TIMESTAMPTZ,
    health_status      TEXT NOT NULL DEFAULT 'unknown',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE vpn_clients (
    id               BIGSERIAL PRIMARY KEY,
    user_id          BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    server_id        BIGINT NOT NULL REFERENCES vpn_servers(id) ON DELETE CASCADE,
    inbound_id       INT NOT NULL,
    email            TEXT NOT NULL UNIQUE,
    uuid             TEXT,
    password         TEXT,
    protocol         TEXT NOT NULL,
    flow             TEXT,
    traffic_limit    BIGINT NOT NULL DEFAULT 0,
    traffic_used     BIGINT NOT NULL DEFAULT 0,
    traffic_up       BIGINT NOT NULL DEFAULT 0,
    traffic_down     BIGINT NOT NULL DEFAULT 0,
    ip_limit         INT NOT NULL DEFAULT 1,
    is_banned        BOOLEAN NOT NULL DEFAULT FALSE,
    is_active        BOOLEAN NOT NULL DEFAULT TRUE,
    is_expired       BOOLEAN NOT NULL DEFAULT FALSE,
    is_trial         BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at       TIMESTAMPTZ,
    config_link      TEXT,
    subscription_url TEXT,
    notified_expiry  BOOLEAN NOT NULL DEFAULT FALSE,
    last_sync        TIMESTAMPTZ,
    last_online      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_vpn_clients_user_id    ON vpn_clients (user_id);
CREATE INDEX idx_vpn_clients_expires_at ON vpn_clients (expires_at);
CREATE INDEX idx_vpn_clients_is_trial   ON vpn_clients (is_trial);

CREATE TABLE orders (
    id             BIGSERIAL PRIMARY KEY,
    order_id       TEXT NOT NULL UNIQUE,          -- KTS-XXXXXXXX-VPN
    order_type     TEXT NOT NULL,                 -- purchase|renewal|topup|trial
    user_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    server_id      BIGINT REFERENCES vpn_servers(id) ON DELETE SET NULL,
    client_id      BIGINT REFERENCES vpn_clients(id) ON DELETE SET NULL,
    protocol       TEXT,
    duration_days  INT,
    traffic_gb     INT,
    ip_limit       INT,
    amount         NUMERIC(15,2) NOT NULL DEFAULT 0,
    discount       NUMERIC(15,2) NOT NULL DEFAULT 0,
    final_amount   NUMERIC(15,2) NOT NULL DEFAULT 0,
    currency       TEXT NOT NULL DEFAULT 'IDR',
    status         TEXT NOT NULL DEFAULT 'pending',
                   -- pending|processing|completed|failed|cancelled|refunded
    notes          TEXT,
    error_message  TEXT,
    account_email  TEXT,
    account_remark TEXT,
    balance_before NUMERIC(15,2),
    balance_after  NUMERIC(15,2),
    completed_at   TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_orders_user_id  ON orders (user_id);
CREATE INDEX idx_orders_status   ON orders (status);
CREATE INDEX idx_orders_created  ON orders (created_at);

CREATE TABLE balance_transactions (
    id            BIGSERIAL PRIMARY KEY,
    user_id       BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    order_id      TEXT,
    type          TEXT NOT NULL,                  -- credit|debit
    amount        NUMERIC(15,2) NOT NULL,
    balance_after NUMERIC(15,2) NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_balance_tx_user    ON balance_transactions (user_id);
CREATE INDEX idx_balance_tx_created ON balance_transactions (created_at);

CREATE TABLE payments (
    id              BIGSERIAL PRIMARY KEY,
    order_id        TEXT NOT NULL UNIQUE,
    user_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount_gross    NUMERIC(15,2) NOT NULL,
    amount_net      NUMERIC(15,2) NOT NULL,
    fee_amount      NUMERIC(15,2) NOT NULL,
    fee_pct         NUMERIC(5,4) NOT NULL,
    provider_ref    TEXT,
    provider_status TEXT,
    status          TEXT NOT NULL DEFAULT 'pending',
                    -- pending|success|failed|expired
    paid_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_payments_user   ON payments (user_id);
CREATE INDEX idx_payments_status ON payments (status);

CREATE TABLE pricing (
    id           BIGSERIAL PRIMARY KEY,
    country_code TEXT NOT NULL,
    plan_days    INT NOT NULL,
    price        NUMERIC(15,2) NOT NULL,
    enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (country_code, plan_days)
);
