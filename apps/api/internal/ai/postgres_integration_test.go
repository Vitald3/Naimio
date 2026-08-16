//go:build integration

package ai

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresDraftTaxonomyAndCostAccounting(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Fatal("DATABASE_URL is required")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	user, category, skill := "fa101010-1010-4010-8010-101010101010", "fa202020-2020-4020-8020-202020202020", "fa303030-3030-4030-8030-303030303030"
	if _, err = db.ExecContext(ctx, `INSERT INTO users(id,email,email_normalized,password_hash,username,username_normalized,display_name)VALUES($1,'ai@example.invalid','ai@example.invalid','x','ai-test','ai-test','AI Test')`, user); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO categories(id,slug,name)VALUES($1,'ai-category','AI Category')`, category); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO skills(id,slug,name)VALUES($1,'ai-skill','AI Skill')`, skill); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, q := range []struct {
			query string
			args  []any
		}{
			{query: "DELETE FROM ai_requests WHERE user_id=$1", args: []any{user}},
			{query: "DELETE FROM project_drafts WHERE owner_user_id=$1", args: []any{user}},
			{query: "DELETE FROM skills WHERE id=$1", args: []any{skill}},
			{query: "DELETE FROM categories WHERE id=$1", args: []any{category}},
			{query: "DELETE FROM users WHERE id=$1", args: []any{user}},
		} {
			_, _ = db.ExecContext(context.Background(), q.query, q.args...)
		}
	})
	repository := PostgresRepository{DB: db}
	draft, token, err := repository.Create(ctx, "", "IMPORT", map[string]any{"text": "material"})
	if err != nil || draft.ID == "" || len(token) != 64 {
		t.Fatalf("draft=%#v token=%q err=%v", draft, token, err)
	}
	claimed, err := repository.Claim(ctx, user, token)
	if err != nil || claimed.OwnerUserID != user {
		t.Fatalf("claim=%#v err=%v", claimed, err)
	}
	updated, err := repository.Update(ctx, user, token, nil, map[string]any{"title": "Editable"})
	if err != nil || updated.NormalizedData["title"] != "Editable" {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
	categories, skills, err := repository.Candidates(ctx)
	if err != nil || len(categories) == 0 || len(skills) == 0 {
		t.Fatalf("taxonomy: %d %d %v", len(categories), len(skills), err)
	}
	err = repository.Record(ctx, RequestMetric{UserID: user, Capability: ProjectBrief, Provider: "mock", Model: "mock-v1", Status: "SUCCEEDED", InputTokens: 100, OutputTokens: 20, CostMicrounits: 7, Latency: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	var cost int64
	if err = db.QueryRowContext(ctx, `SELECT cost_microunits FROM ai_requests WHERE user_id=$1`, user).Scan(&cost); err != nil || cost != 7 {
		t.Fatalf("cost=%d err=%v", cost, err)
	}
}
