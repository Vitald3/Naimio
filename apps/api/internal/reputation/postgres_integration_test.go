//go:build integration

package reputation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresExternalReputationCRUDAndPublicProjection(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is required")
	}
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()
	owner, other := "d1101010-1010-4010-8010-101010101010", "d2202020-2020-4020-8020-202020202020"
	if _, err = database.ExecContext(ctx, `INSERT INTO users(id,email,email_normalized,password_hash,username,username_normalized,display_name)VALUES($1,'reputation-owner@example.invalid','reputation-owner@example.invalid','x','reputation-owner','reputation-owner','Owner'),($2,'reputation-other@example.invalid','reputation-other@example.invalid','x','reputation-other','reputation-other','Other')`, owner, other); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO professional_profiles(user_id,profile_visibility)VALUES($1,'PUBLIC'),($2,'PUBLIC')`, owner, other); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.ExecContext(context.Background(), `DELETE FROM external_reputations WHERE user_id IN($1,$2)`, owner, other)
		_, _ = database.ExecContext(context.Background(), `DELETE FROM professional_profiles WHERE user_id IN($1,$2)`, owner, other)
		_, _ = database.ExecContext(context.Background(), `DELETE FROM users WHERE id IN($1,$2)`, owner, other)
	})
	service := Service{Repository: PostgresRepository{DB: database}}
	created, err := service.Create(ctx, owner, CreateRequest{Platform: "GITHUB", ProfileURL: "https://github.com/reputation-owner", ExternalUsername: "reputation-owner"})
	if err != nil || created.VerificationStatus != StatusUnverified {
		t.Fatalf("created=%#v err=%v", created, err)
	}
	if _, err = service.Create(ctx, owner, CreateRequest{Platform: "GITHUB", ProfileURL: "https://github.com/reputation-owner"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate error=%v", err)
	}
	username := "owner-updated"
	updated, err := service.Update(ctx, owner, created.ID, PatchRequest{ExternalUsername: &username})
	if err != nil || updated.ExternalUsername != username {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
	if _, err = service.Update(ctx, other, created.ID, PatchRequest{ExternalUsername: &username}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user error=%v", err)
	}
	_, err = database.ExecContext(ctx, `UPDATE external_reputations SET verification_status='VERIFIED',verification_method='MANUAL',verified_at=now(),rating=4.9,reviews_count=10,evidence='{"private":true}',source_snapshot='{"raw":true}' WHERE id=$1`, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	public, err := service.ListPublic(ctx, "reputation-owner")
	if err != nil || len(public) != 1 || !public[0].Verified {
		t.Fatalf("public=%#v err=%v", public, err)
	}
	bodyBytes, _ := json.Marshal(public)
	body := string(bodyBytes)
	if strings.Contains(body, "evidence") || strings.Contains(body, "source_snapshot") || strings.Contains(body, "MANUAL") {
		t.Fatalf("public leak: %s", body)
	}
}

func TestPostgresModeratorCanVerifyPendingExternalReputation(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is required")
	}
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()
	owner := "d3303030-3030-4030-8030-303030303030"
	admin := "d4404040-4040-4040-8040-404040404040"
	if _, err = database.ExecContext(ctx, `INSERT INTO users(id,email,email_normalized,password_hash,username,username_normalized,display_name)
VALUES($1,'reputation-pending@example.invalid','reputation-pending@example.invalid','x','reputation-pending','reputation-pending','Pending Owner'),
      ($2,'reputation-admin@example.invalid','reputation-admin@example.invalid','x','reputation-admin','reputation-admin','Reputation Admin')`, owner, admin); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO user_roles(user_id,role) VALUES($1,'ADMIN') ON CONFLICT DO NOTHING`, admin); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, q := range []struct {
			query string
			args  []any
		}{
			{query: "DELETE FROM audit_logs WHERE actor_user_id=$1 OR target_id IN (SELECT id FROM external_reputations WHERE user_id=$2)", args: []any{admin, owner}},
			{query: "DELETE FROM fraud_signals WHERE entity_type='EXTERNAL_REPUTATION' AND entity_id IN (SELECT id FROM external_reputations WHERE user_id=$1)", args: []any{owner}},
			{query: "DELETE FROM external_reputations WHERE user_id=$1", args: []any{owner}},
			{query: "DELETE FROM user_roles WHERE user_id=$1", args: []any{admin}},
			{query: "DELETE FROM users WHERE id IN($1,$2)", args: []any{owner, admin}},
		} {
			_, _ = database.ExecContext(context.Background(), q.query, q.args...)
		}
	})
	service := Service{Repository: PostgresRepository{DB: database}}
	created, err := service.Create(ctx, owner, CreateRequest{Platform: "GITHUB", ProfileURL: "https://github.com/reputation-pending", ExternalUsername: "reputation-pending"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.StartVerification(ctx, owner, created.ID, StartVerificationRequest{Method: "MANUAL", Evidence: map[string]any{"note": "manual proof"}}); err != nil {
		t.Fatal(err)
	}
	verified, err := service.Decide(ctx, admin, created.ID, "verify", DecisionRequest{Note: "<p>Проверено модератором</p>"})
	if err != nil {
		t.Fatalf("verify pending external reputation: %v", err)
	}
	if verified.VerificationStatus != "VERIFIED" || verified.VerifiedAt == nil {
		t.Fatalf("verified=%#v", verified)
	}
	var challengeStatus string
	if err = database.QueryRowContext(ctx, `SELECT status FROM reputation_verification_challenges WHERE external_reputation_id=$1 ORDER BY created_at DESC LIMIT 1`, created.ID).Scan(&challengeStatus); err != nil {
		t.Fatal(err)
	}
	if challengeStatus != "VERIFIED" {
		t.Fatalf("challenge status=%q", challengeStatus)
	}
	var auditCount int
	if err = database.QueryRowContext(ctx, `SELECT count(*) FROM audit_logs WHERE actor_user_id=$1 AND target_id=$2 AND action='EXTERNAL_REPUTATION_VERIFIED'`, admin, created.ID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("audit count=%d", auditCount)
	}
}
