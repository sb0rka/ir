package psql

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
)

const loginSessionColumns = `id::text, subject_id::text, external_sid::text, refresh_hash, encrypted_tokens, profile, expires_at, revoked_at IS NOT NULL, generation`

func scanLoginSession(row pgx.Row) (model.LoginSession, error) {
	var s model.LoginSession
	err := row.Scan(
		&s.ID,
		&s.SubjectID,
		&s.ExternalSID,
		&s.RefreshHash,
		&s.EncryptedTokens,
		&s.Profile,
		&s.ExpiresAt,
		&s.Revoked,
		&s.Generation,
	)
	return s, err
}

func (d *DB) CreateLoginSession(ctx context.Context, s model.LoginSession) error {
	_, err := d.Pgx().Exec(ctx, `
        INSERT INTO ir_login_sessions (id, subject_id, external_sid, refresh_hash, encrypted_tokens, profile, expires_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
    `, s.ID, s.SubjectID, s.ExternalSID, s.RefreshHash, s.EncryptedTokens, s.Profile, s.ExpiresAt)
	return err
}

func (d *DB) LoginSessionByRefresh(ctx context.Context, hash []byte) (model.LoginSession, error) {
	return scanLoginSession(
		d.Pgx().
			QueryRow(ctx, `SELECT `+loginSessionColumns+` FROM ir_login_sessions WHERE refresh_hash = $1 AND revoked_at IS NULL AND expires_at > now()`, hash),
	)
}

func (d *DB) LoginSessionByIdentity(ctx context.Context, sid, subject string) (model.LoginSession, error) {
	return scanLoginSession(d.Pgx().QueryRow(ctx, `SELECT `+loginSessionColumns+`
        FROM ir_login_sessions
        WHERE external_sid = $1
          AND subject_id = $2
          AND revoked_at IS NULL
          AND expires_at > now()
    `, sid, subject))
}

func (d *DB) LoginSessionByID(ctx context.Context, id string) (model.LoginSession, error) {
	return scanLoginSession(
		d.Pgx().
			QueryRow(ctx, `SELECT `+loginSessionColumns+` FROM ir_login_sessions WHERE id = $1 AND revoked_at IS NULL AND expires_at > now()`, id),
	)
}

func (d *DB) ReserveLoginRefresh(ctx context.Context, id string, generation int64, operation string) (bool, error) {
	result, err := d.Pgx().Exec(ctx, `
        UPDATE ir_login_sessions
        SET refresh_operation = $3, refresh_lease_until = now() + interval '15 seconds'
        WHERE id = $1
          AND generation = $2
          AND revoked_at IS NULL
          AND expires_at > now()
          AND (refresh_lease_until IS NULL
          OR refresh_lease_until <= now())
    `, id, generation, operation)
	return result.RowsAffected() == 1, err
}

func (d *DB) CompleteLoginRefresh(
	ctx context.Context,
	id string,
	generation int64,
	operation string,
	encrypted []byte,
) (bool, error) {
	result, err := d.Pgx().Exec(ctx, `
        UPDATE ir_login_sessions
        SET encrypted_tokens = $4, generation = generation + 1, refresh_operation = NULL, refresh_lease_until = NULL
        WHERE id = $1
          AND generation = $2
          AND refresh_operation = $3
          AND revoked_at IS NULL
          AND expires_at > now()
    `, id, generation, operation, encrypted)
	return result.RowsAffected() == 1, err
}

func (d *DB) ReleaseLoginRefresh(ctx context.Context, id, operation string) error {
	_, err := d.Pgx().Exec(ctx, `
        UPDATE ir_login_sessions
        SET refresh_operation = NULL, refresh_lease_until = NULL
        WHERE id = $1
          AND refresh_operation = $2
    `, id, operation)
	return err
}

func (d *DB) RevokeLoginSession(ctx context.Context, id string) error {
	// Revoke locally and queue the current encrypted Auth credential atomically.
	_, err := d.Pgx().Exec(ctx, `
        WITH revoked AS (UPDATE ir_login_sessions
        SET revoked_at = now()
        WHERE id = $1
          AND revoked_at IS NULL
        RETURNING id, encrypted_tokens) INSERT INTO ir_auth_revocations (id, session_id, encrypted_tokens, expires_at) SELECT gen_random_uuid(), id, encrypted_tokens, now() + interval '7 days'
        FROM revoked
    `, id)
	return err
}

func (d *DB) CreateLoginRequest(ctx context.Context, r model.LoginRequest) error {
	_, err := d.Pgx().Exec(ctx, `
        INSERT INTO ir_login_requests (id, kind, session_id, state_hash, encrypted_context, expires_at)
        VALUES ($1, $2, NULLIF($3, '')::uuid, $4, $5, $6)
    `, r.ID, r.Kind, r.SessionID, r.StateHash, r.EncryptedContext, r.ExpiresAt)
	return err
}

const loginRequestColumns = `id::text, kind, COALESCE(session_id::text, ''), state_hash, encrypted_context, handoff_hash, encrypted_result, expires_at`

func scanLoginRequest(row pgx.Row) (model.LoginRequest, error) {
	var r model.LoginRequest
	err := row.Scan(
		&r.ID,
		&r.Kind,
		&r.SessionID,
		&r.StateHash,
		&r.EncryptedContext,
		&r.HandoffHash,
		&r.EncryptedResult,
		&r.ExpiresAt,
	)
	return r, err
}

func (d *DB) ConsumeLoginCallback(ctx context.Context, state []byte) (model.LoginRequest, error) {
	return scanLoginRequest(d.Pgx().QueryRow(ctx, `
        UPDATE ir_login_requests
        SET callback_consumed_at = now()
        WHERE state_hash = $1
          AND expires_at > now()
          AND callback_consumed_at IS NULL
        RETURNING
    `+loginRequestColumns, state))
}

func (d *DB) FinishLoginCallback(ctx context.Context, id string, hash, encrypted []byte) error {
	_, err := d.Pgx().Exec(ctx, `
        UPDATE ir_login_requests
        SET handoff_hash = $2, encrypted_result = $3, expires_at = LEAST(expires_at, now() + interval '60 seconds')
        WHERE id = $1
          AND callback_consumed_at IS NOT NULL
          AND redeemed_at IS NULL
    `, id, hash, encrypted)
	return err
}

func (d *DB) RedeemLoginHandoff(ctx context.Context, hash []byte) (model.LoginRequest, error) {
	return scanLoginRequest(d.Pgx().QueryRow(ctx, `
        UPDATE ir_login_requests
        SET redeemed_at = now()
        WHERE handoff_hash = $1
          AND expires_at > now()
          AND redeemed_at IS NULL
        RETURNING
    `+loginRequestColumns, hash))
}

func (d *DB) QueueAuthRevocation(ctx context.Context, r model.AuthRevocation) error {
	_, err := d.Pgx().Exec(ctx, `
		INSERT INTO ir_auth_revocations (id, session_id, encrypted_tokens, expires_at)
		VALUES ($1, $2, $3, now() + interval '7 days')
	`, r.ID, r.SessionID, r.EncryptedTokens)
	return err
}

func (d *DB) ExpireAuthRevocations(ctx context.Context) ([]string, error) {
	rows, err := d.Pgx().Query(ctx, `
		DELETE FROM ir_auth_revocations
		WHERE expires_at <= now()
		RETURNING session_id::text
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sessions []string
	for rows.Next() {
		var sessionID string
		if err := rows.Scan(&sessionID); err != nil {
			return nil, err
		}
		sessions = append(sessions, sessionID)
	}
	return sessions, rows.Err()
}

func (d *DB) ClaimAuthRevocations(ctx context.Context, limit int) ([]model.AuthRevocation, error) {
	tx, err := d.Pgx().Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `
		SELECT id::text, session_id::text, encrypted_tokens, attempts
		FROM ir_auth_revocations
		WHERE next_attempt_at <= now() AND expires_at > now()
		ORDER BY created_at
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.AuthRevocation{}
	for rows.Next() {
		var r model.AuthRevocation
		if err := rows.Scan(&r.ID, &r.SessionID, &r.EncryptedTokens, &r.Attempts); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for i := range result {
		nextAttempts := result[i].Attempts + 1
		if _, err = tx.Exec(ctx, `
			UPDATE ir_auth_revocations
			SET attempts = $2, next_attempt_at = $3
			WHERE id = $1::uuid
		`, result[i].ID, nextAttempts, time.Now().Add(revocationBackoff(nextAttempts))); err != nil {
			return nil, err
		}
		result[i].Attempts = nextAttempts
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func revocationBackoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	shift := attempts - 1
	if shift > 6 {
		shift = 6
	}
	delay := 10 * time.Second * time.Duration(uint(1)<<shift)
	if delay > 15*time.Minute {
		delay = 15 * time.Minute
	}
	return delay
}

func (d *DB) CompleteAuthRevocation(ctx context.Context, id string) error {
	_, err := d.Pgx().Exec(ctx, `DELETE FROM ir_auth_revocations WHERE id = $1`, id)
	return err
}
