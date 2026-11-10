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
    previous_refresh_hash BYTEA,
    refresh_handle_rotated_at TIMESTAMPTZ,
    encrypted_refresh_handle BYTEA,
    created_at TIMESTAMPTZ DEFAULT now() NOT NULL,
    CONSTRAINT ck_ir_login_sessions_refresh_hash CHECK (octet_length(refresh_hash) = 32)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_ir_login_sessions_previous_refresh
    ON ir_login_sessions(previous_refresh_hash) WHERE previous_refresh_hash IS NOT NULL;

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

CREATE TABLE IF NOT EXISTS ir_agent_secret_digests (
    project_id VARCHAR(12) NOT NULL,
    secret_id UUID NOT NULL,
    version_no INT NOT NULL CHECK (version_no > 0),
    digest_alg VARCHAR(16) NOT NULL CHECK (digest_alg = 'sha256'),
    digest BYTEA NOT NULL CHECK (octet_length(digest) = 32),
    prefix VARCHAR(14) NOT NULL,
    postfix VARCHAR(3) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now() NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT now() NOT NULL,

    CONSTRAINT pk_ir_agent_secret_digests PRIMARY KEY (secret_id, version_no),
    CONSTRAINT uq_ir_agent_secret_digests_project UNIQUE (project_id),
    CONSTRAINT uq_ir_agent_secret_digests_digest UNIQUE (digest)
);

CREATE TABLE IF NOT EXISTS ir_gateway_configurations (
    project_id VARCHAR(12) PRIMARY KEY,
    secret_id UUID,
    version_no INT NOT NULL CHECK (version_no >= 0),
    encrypted_config BYTEA NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT now() NOT NULL,
    CONSTRAINT ck_ir_gateway_configurations_version CHECK (
        (secret_id IS NULL AND version_no = 0) OR (secret_id IS NOT NULL AND version_no > 0)
    )
);

UPDATE version_investigations SET version_num = '202611030001';

COMMIT;
