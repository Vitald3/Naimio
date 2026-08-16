package auth

import (
	"context"
	"database/sql"
	"errors"
)

type PostgresSessionRepository struct{ DB *sql.DB }

func (r PostgresSessionRepository) FindByTokenHash(ctx context.Context, tokenHash []byte) (Session, error) {
	var session Session
	err := r.DB.QueryRowContext(ctx, `
WITH touched AS (UPDATE sessions SET last_used_at=now() WHERE token_hash=$1 RETURNING user_id,expires_at,revoked_at)
SELECT s.user_id::text, u.status, s.expires_at, s.revoked_at
FROM touched s
JOIN users u ON u.id = s.user_id
WHERE true
  AND u.deleted_at IS NULL
LIMIT 1`, tokenHash).Scan(&session.UserID, &session.UserStatus, &session.ExpiresAt, &session.RevokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrSessionNotFound
	}
	return session, err
}
