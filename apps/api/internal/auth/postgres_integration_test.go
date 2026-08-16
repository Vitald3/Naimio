//go:build integration

package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresSessionRepository(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is required for integration tests")
	}
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	userID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	sessionID := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	tokenHash := sha256.Sum256([]byte("integration-session-token"))
	if _, err = database.ExecContext(ctx, `INSERT INTO users (id, email, email_normalized, password_hash, display_name) VALUES ($1, 'session-test@example.invalid', 'session-test@example.invalid', 'test-only', 'Session Test')`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO sessions (id, user_id, token_hash, last_used_at, expires_at) VALUES ($1, $2, $3, now(), now() + interval '1 hour')`, sessionID, userID, tokenHash[:]); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.ExecContext(context.Background(), "DELETE FROM sessions WHERE id = $1", sessionID)
		_, _ = database.ExecContext(context.Background(), "DELETE FROM users WHERE id = $1", userID)
	})

	repository := PostgresSessionRepository{DB: database}
	session, err := repository.FindByTokenHash(ctx, tokenHash[:])
	if err != nil || session.UserID != userID || session.UserStatus != "ACTIVE" || !session.ExpiresAt.After(time.Now()) {
		t.Fatalf("session = %#v, error = %v", session, err)
	}
	unknownHash := sha256.Sum256([]byte("unknown-session-token"))
	if _, err := repository.FindByTokenHash(ctx, unknownHash[:]); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("unknown session error = %v", err)
	}
}
