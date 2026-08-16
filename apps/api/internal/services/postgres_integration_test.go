//go:build integration

package services

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresServiceRepository(t *testing.T) {
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
	owner := "10101010-1010-4010-8010-101010101010"
	other := "20202020-2020-4020-8020-202020202020"
	category := "30303030-3030-4030-8030-303030303030"
	skill := "40404040-4040-4040-8040-404040404040"
	cleanMedia := "50505050-5050-4050-8050-505050505050"
	pendingMedia := "60606060-6060-4060-8060-606060606060"
	if _, err = database.ExecContext(ctx, `INSERT INTO users (id,email,email_normalized,password_hash,username,username_normalized,display_name) VALUES
($1,'service-owner@example.invalid','service-owner@example.invalid','test','ServiceOwner','serviceowner','Service Owner'),
($2,'service-other@example.invalid','service-other@example.invalid','test','ServiceOther','serviceother','Service Other')`, owner, other); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO user_capabilities (user_id, capability) VALUES ($1,'FREELANCER'),($2,'FREELANCER')`, owner, other); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO professional_profiles (user_id, profile_visibility) VALUES ($1,'PUBLIC'),($2,'PUBLIC')`, owner, other); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO categories (id,slug,name) VALUES ($1,'service-category','Service Category')`, category); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO skills (id,slug,name) VALUES ($1,'service-skill','Service Skill')`, skill); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO media_objects (id,owner_user_id,object_key,bucket,mime_type,size_bytes,purpose,uploaded_at,scan_status) VALUES
($1,$2,'service/clean.png','test','image/png',100,'SERVICE',now(),'CLEAN'),
($3,$2,'service/pending.png','test','image/png',100,'SERVICE',now(),'PENDING')`, cleanMedia, owner, pendingMedia); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, q := range []struct {
			query string
			args  []any
		}{
			{query: "DELETE FROM services WHERE seller_user_id IN ($1,$2)", args: []any{owner, other}},
			{query: "DELETE FROM media_objects WHERE owner_user_id IN ($1,$2)", args: []any{owner, other}},
			{query: "DELETE FROM professional_profiles WHERE user_id IN ($1,$2)", args: []any{owner, other}},
			{query: "DELETE FROM user_capabilities WHERE user_id IN ($1,$2)", args: []any{owner, other}},
			{query: "DELETE FROM users WHERE id IN ($1,$2)", args: []any{owner, other}},
			{query: "DELETE FROM skills WHERE id=$1", args: []any{skill}},
			{query: "DELETE FROM categories WHERE id=$1", args: []any{category}},
		} {
			_, _ = database.ExecContext(context.Background(), q.query, q.args...)
		}
	})

	repository := PostgresRepository{DB: database}
	input := CreateRequest{CategoryID: category, ServiceType: "PROFESSIONAL_SERVICE", Title: "API", Slug: "api", Description: "API service", PriceType: "FROM", PriceFrom: &Money{AmountKopecks: 10000, Currency: "RUB"}, Visibility: "PUBLIC", SkillIDs: []string{skill}, MediaObjectIDs: []string{cleanMedia}}
	created, err := repository.Create(ctx, owner, input)
	if err != nil || created.Status != "DRAFT" || len(created.Skills) != 1 || len(created.Media) != 1 {
		t.Fatalf("created = %#v, error = %v", created, err)
	}
	if _, err := repository.GetOwned(ctx, other, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user get error = %v", err)
	}
	bad := input
	bad.Slug, bad.MediaObjectIDs = "bad-media", []string{pendingMedia}
	if _, err := repository.Create(ctx, owner, bad); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("pending media error = %v", err)
	}
	title := "Updated API"
	for attempt := 0; attempt < 2; attempt++ {
		updated, updateErr := repository.Update(ctx, owner, created.ID, PatchRequest{Title: &title})
		if updateErr != nil || updated.Title != title {
			t.Fatalf("attempt %d updated = %#v, error = %v", attempt, updated, updateErr)
		}
	}
	if _, err := repository.Transition(ctx, other, created.ID, "publish"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user publish error = %v", err)
	}
	active, err := repository.Transition(ctx, owner, created.ID, "publish")
	if err != nil || active.Status != "ACTIVE" {
		t.Fatalf("active = %#v, error = %v", active, err)
	}
	page, err := repository.ListPublic(ctx, Filter{ServiceType: "PROFESSIONAL_SERVICE"}, nil, 1)
	if err != nil || len(page.Items) != 1 || page.Items[0].SellerID != owner {
		t.Fatalf("page = %#v, error = %v", page, err)
	}
	searched, err := repository.ListPublic(ctx, Filter{Q: "Updated API"}, nil, 20)
	if err != nil || len(searched.Items) != 1 || searched.Items[0].ID != created.ID {
		t.Fatalf("search = %#v, error = %v", searched, err)
	}
	public, err := repository.GetPublic(ctx, created.ID)
	if err != nil || public.ID != created.ID {
		t.Fatalf("public = %#v, error = %v", public, err)
	}
	if _, err := repository.Transition(ctx, owner, created.ID, "pause"); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.GetPublic(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("paused public error = %v", err)
	}
	if _, err := repository.Transition(ctx, owner, created.ID, "resume"); err != nil {
		t.Fatal(err)
	}
	if err := repository.Delete(ctx, other, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user delete error = %v", err)
	}
	if err := repository.Delete(ctx, owner, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Create(ctx, owner, input); err != nil {
		t.Fatalf("slug reuse error = %v", err)
	}
}
