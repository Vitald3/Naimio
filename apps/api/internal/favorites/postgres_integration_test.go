//go:build integration

package favorites

import (
	"context"
	"database/sql"
	"errors"
	_ "github.com/jackc/pgx/v5/stdlib"
	"os"
	"testing"
)

func TestPostgresFavorites(t *testing.T) {
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
	owner, target := "c1101010-1010-4010-8010-101010101010", "c2202020-2020-4020-8020-202020202020"
	if _, err = db.ExecContext(ctx, `INSERT INTO users(id,email,email_normalized,password_hash,display_name)VALUES($1,'favorite-owner@example.invalid','favorite-owner@example.invalid','x','Owner'),($2,'favorite-target@example.invalid','favorite-target@example.invalid','x','Target')`, owner, target); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO professional_profiles(user_id,profile_visibility)VALUES($1,'PUBLIC')`, target); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM favorites WHERE user_id=$1", owner)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM professional_profiles WHERE user_id=$1", target)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM users WHERE id IN($1,$2)", owner, target)
	})
	r := PostgresRepository{DB: db}
	a, err := r.Put(ctx, owner, "FREELANCER", target)
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.Put(ctx, owner, "FREELANCER", target)
	if err != nil || !a.CreatedAt.Equal(b.CreatedAt) {
		t.Fatalf("idempotent=%#v err=%v", b, err)
	}
	page, err := r.List(ctx, owner, "FREELANCER", nil, 20)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("page=%#v err=%v", page, err)
	}
	if _, err = r.Put(ctx, owner, "PROJECT", target); !errors.Is(err, ErrNotFound) {
		t.Fatalf("visibility=%v", err)
	}
	if err = r.Delete(ctx, owner, "FREELANCER", target); err != nil {
		t.Fatal(err)
	}
	if err = r.Delete(ctx, owner, "FREELANCER", target); err != nil {
		t.Fatal(err)
	}
}
