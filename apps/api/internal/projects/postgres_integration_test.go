//go:build integration

package projects

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresProjectRepository(t *testing.T) {
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
	owner, other := "11101010-1010-4010-8010-101010101010", "22202020-2020-4020-8020-202020202020"
	category, skill := "33303030-3030-4030-8030-303030303030", "44404040-4040-4040-8040-404040404040"
	cleanMedia, pendingMedia := "55505050-5050-4050-8050-505050505050", "66606060-6060-4060-8060-606060606060"
	if _, err = database.ExecContext(ctx, `INSERT INTO users(id,email,email_normalized,password_hash,username,username_normalized,display_name) VALUES
($1,'project-owner@example.invalid','project-owner@example.invalid','test','ProjectOwner','projectowner','Project Owner'),
($2,'project-other@example.invalid','project-other@example.invalid','test','ProjectOther','projectother','Project Other')`, owner, other); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO user_capabilities(user_id,capability) VALUES($1,'CUSTOMER'),($2,'CUSTOMER')`, owner, other); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO categories(id,slug,name) VALUES($1,'project-category','Project Category')`, category); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO skills(id,slug,name) VALUES($1,'project-skill','Project Skill')`, skill); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO media_objects(id,owner_user_id,object_key,bucket,mime_type,size_bytes,purpose,uploaded_at,scan_status) VALUES
($1,$2,'project/clean.png','test','image/png',100,'PROJECT',now(),'CLEAN'),
($3,$2,'project/pending.png','test','image/png',100,'PROJECT',now(),'PENDING')`, cleanMedia, owner, pendingMedia); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, q := range []struct {
			query string
			args  []any
		}{
			{query: "DELETE FROM outbox_events WHERE aggregate_type='PROJECT'"},
			{query: "DELETE FROM projects WHERE customer_user_id IN($1,$2)", args: []any{owner, other}},
			{query: "DELETE FROM project_drafts WHERE owner_user_id IN($1,$2)", args: []any{owner, other}},
			{query: "DELETE FROM media_objects WHERE owner_user_id IN($1,$2)", args: []any{owner, other}},
			{query: "DELETE FROM user_capabilities WHERE user_id IN($1,$2)", args: []any{owner, other}},
			{query: "DELETE FROM users WHERE id IN($1,$2)", args: []any{owner, other}},
			{query: "DELETE FROM skills WHERE id=$1", args: []any{skill}},
			{query: "DELETE FROM categories WHERE id=$1", args: []any{category}},
		} {
			_, _ = database.ExecContext(context.Background(), q.query, q.args...)
		}
	})

	repository := PostgresRepository{DB: database}
	draftToken := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	draftHash := sha256.Sum256([]byte(draftToken))
	if _, err := database.ExecContext(ctx, `INSERT INTO project_drafts(id,owner_user_id,guest_token_hash,source_type,expires_at)VALUES(gen_random_uuid(),$1,$2,'AI_BRIEF',now()+interval '1 day')`, owner, draftHash[:]); err != nil {
		t.Fatal(err)
	}
	amount := int64(10000)
	input := CreateRequest{CategoryID: category, Title: "API", Slug: "api", Description: "API project", Budget: Budget{Type: "FIXED", MinKopecks: &amount, Currency: "RUB"}, ExperienceLevel: "ADVANCED", Visibility: "PUBLIC", SkillIDs: []string{skill}, MediaObjectIDs: []string{cleanMedia}, SourceDraftToken: draftToken}
	created, err := repository.Create(ctx, owner, input)
	if err != nil || created.Status != "DRAFT" || created.SourceType != "AI_BRIEF" || len(created.Skills) != 1 || len(created.Media) != 1 {
		t.Fatalf("created=%#v err=%v", created, err)
	}
	if _, err := repository.GetOwned(ctx, other, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user=%v", err)
	}
	bad := input
	bad.Slug, bad.MediaObjectIDs, bad.SourceDraftToken = "bad-media", []string{pendingMedia}, ""
	if _, err := repository.Create(ctx, owner, bad); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("pending media=%v", err)
	}
	opened, err := repository.Transition(ctx, owner, created.ID, "publish")
	if err != nil || opened.Status != "OPEN" {
		t.Fatalf("opened=%#v err=%v", opened, err)
	}
	page, err := repository.ListPublic(ctx, Filter{BudgetType: "FIXED"}, nil, 20)
	if err != nil || len(page.Items) != 1 || page.Items[0].CustomerID != "" {
		t.Fatalf("page=%#v err=%v", page, err)
	}
	searched, err := repository.ListPublic(ctx, Filter{Q: "API project"}, nil, 20)
	if err != nil || len(searched.Items) != 1 || searched.Items[0].ID != created.ID {
		t.Fatalf("search=%#v err=%v", searched, err)
	}
	if _, err := repository.Transition(ctx, other, created.ID, "cancel"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user transition=%v", err)
	}
	if _, err := repository.Transition(ctx, owner, created.ID, "cancel"); err != nil {
		t.Fatal(err)
	}

	second := input
	second.Slug, second.SourceDraftToken = "complete", ""
	inProgress, err := repository.Create(ctx, owner, second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE projects SET status='IN_PROGRESS',published_at=now() WHERE id=$1`, inProgress.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Transition(ctx, owner, inProgress.ID, "complete"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("completion without Safe Deal must be blocked: %v", err)
	}
	var eventCount int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM outbox_events WHERE aggregate_id=$1 AND event_type='PROJECT_COMPLETED'`, inProgress.ID).Scan(&eventCount); err != nil || eventCount != 0 {
		t.Fatalf("events=%d err=%v", eventCount, err)
	}
}
