-- Migration: 202611030001
BEGIN;
SET LOCAL search_path = :"DB_INV_SCHEMA_NAME", pg_temp;

CREATE TABLE IF NOT EXISTS ir_login_sessions (
    id UUID PRIMARY KEY,
    subject_id UUID NOT NULL,
    external_sid UUID NOT NULL UNIQUE,
    refresh_hash BYTEA NOT NULL UNIQUE,
    encrypted_tokens BYTEA NOT NULL,
    profile JSONB NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    generation BIGINT DEFAULT 0 NOT NULL,
    refresh_operation UUID,
    refresh_lease_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT now() NOT NULL,
    CONSTRAINT ck_ir_login_sessions_refresh_hash CHECK (octet_length(refresh_hash) = 32)
);

CREATE TABLE IF NOT EXISTS ir_login_requests (
    id UUID PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('login', 'platform', 'gateway')),
    session_id UUID REFERENCES ir_login_sessions(id),
    state_hash BYTEA NOT NULL UNIQUE,
    encrypted_context BYTEA NOT NULL,
    handoff_hash BYTEA UNIQUE,
    encrypted_result BYTEA,
    expires_at TIMESTAMPTZ NOT NULL,
    callback_consumed_at TIMESTAMPTZ,
    redeemed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT now() NOT NULL
);

CREATE TABLE IF NOT EXISTS ir_auth_revocations (
    id UUID PRIMARY KEY,
    session_id UUID NOT NULL,
    encrypted_tokens BYTEA NOT NULL,
    attempts INTEGER DEFAULT 0 NOT NULL,
    next_attempt_at TIMESTAMPTZ DEFAULT now() NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now() NOT NULL,
    CONSTRAINT ck_ir_auth_revocations_attempts CHECK (attempts >= 0)
);

CREATE INDEX IF NOT EXISTS ix_ir_auth_revocations_due
    ON ir_auth_revocations (next_attempt_at, created_at);

UPDATE version_investigations SET version_num = '202611030001';
COMMIT;
