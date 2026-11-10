package psql

import (
	"context"
	"github.com/sb0rka/ir/apps/investigations/internal/domain/model"
)

func (d *DB) RotateLoginHandle(ctx context.Context, id string, current, next, encrypted []byte) (bool, error) {
	result, err := d.Pgx().Exec(ctx, `UPDATE ir_login_sessions
        SET previous_refresh_hash = refresh_hash, refresh_hash = $3,
            refresh_handle_rotated_at = now(), encrypted_refresh_handle = $4
        WHERE id = $1 AND refresh_hash = $2 AND revoked_at IS NULL AND expires_at > now()`, id, current, next, encrypted)
	return result.RowsAffected() == 1, err
}

func (d *DB) PreviousLoginRefresh(ctx context.Context, hash []byte) (model.PreviousRefresh, error) {
	var value model.PreviousRefresh
	s := &value.Session
	err := d.Pgx().QueryRow(ctx, `SELECT `+loginSessionColumns+`, encrypted_refresh_handle, refresh_handle_rotated_at
        FROM ir_login_sessions WHERE previous_refresh_hash = $1 AND revoked_at IS NULL AND expires_at > now()`, hash).Scan(
		&s.ID, &s.SubjectID, &s.ExternalSID, &s.RefreshHash, &s.EncryptedTokens, &s.Profile, &s.ExpiresAt, &s.Revoked, &s.Generation,
		&value.EncryptedHandle, &value.RotatedAt,
	)
	return value, err
}
