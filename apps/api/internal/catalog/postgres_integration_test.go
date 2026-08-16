//go:build integration

package catalog

import (
	"context"
	"database/sql"
	"errors"
	_ "github.com/jackc/pgx/v5/stdlib"
	"os"
	"testing"
)

func TestPostgresCatalogRepository(t *testing.T) {
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
	admin := "a1101010-1010-4010-8010-101010101010"
	user := "a2202020-2020-4020-8020-202020202020"
	if _, err = db.ExecContext(ctx, `INSERT INTO users(id,email,email_normalized,password_hash,display_name)VALUES($1,'catalog-admin@example.invalid','catalog-admin@example.invalid','x','Admin'),($2,'catalog-user@example.invalid','catalog-user@example.invalid','x','User')`, admin, user); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO user_roles(user_id,role)VALUES($1,'ADMIN')`, admin); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, q := range []struct {
			query string
			args  []any
		}{
			{query: "DELETE FROM categories WHERE slug LIKE 'catalog-it-%'"},
			{query: "DELETE FROM skills WHERE slug LIKE 'catalog-it-%'"},
			{query: "DELETE FROM audit_logs WHERE actor_user_id=$1", args: []any{admin}},
			{query: "DELETE FROM user_roles WHERE user_id=$1", args: []any{admin}},
			{query: "DELETE FROM users WHERE id IN($1,$2)", args: []any{admin, user}},
		} {
			_, _ = db.ExecContext(context.Background(), q.query, q.args...)
		}
	})
	r := PostgresRepository{DB: db}
	root, err := r.CreateCategory(ctx, admin, CategoryInput{Slug: "catalog-it-root", Name: "Root", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	child, err := r.CreateCategory(ctx, admin, CategoryInput{ParentID: &root.ID, Slug: "catalog-it-child", Name: "Child", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	grand, err := r.CreateCategory(ctx, admin, CategoryInput{ParentID: &child.ID, Slug: "catalog-it-grand", Name: "Grand", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.CreateCategory(ctx, admin, CategoryInput{ParentID: &grand.ID, Slug: "catalog-it-too-deep", Name: "Deep", Active: true}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("depth=%v", err)
	}
	if _, err = r.CreateSkill(ctx, user, SkillInput{Slug: "catalog-it-denied", Name: "Denied", Active: true}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("role=%v", err)
	}
	skill, err := r.CreateSkill(ctx, admin, SkillInput{Slug: "catalog-it-go", Name: "Catalog Go", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	found, err := r.SearchSkills(ctx, "Catalog Go")
	if err != nil || len(found) != 1 || found[0].ID != skill.ID {
		t.Fatalf("found=%#v err=%v", found, err)
	}
	tree, err := r.CategoryTree(ctx)
	if err != nil || len(tree) == 0 {
		t.Fatalf("tree=%#v err=%v", tree, err)
	}
}
