-- 000005_admin_audit_log.up.sql — v1.40: audit log admin (FR-11 AC).
-- Setiap aksi admin (harga, toggle plan, reload, ban/unban, adjust saldo,
-- broadcast, manajemen server) tercatat di sini: siapa (telegram id), apa
-- (action), target, dan detail bebas. Immutable — hanya insert + read.
CREATE TABLE IF NOT EXISTS admin_audit_log (
    id         BIGSERIAL PRIMARY KEY,
    admin_id   BIGINT      NOT NULL,
    action     TEXT        NOT NULL,
    target     TEXT        NOT NULL DEFAULT '',
    detail     TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_admin_audit_created ON admin_audit_log (created_at DESC);
