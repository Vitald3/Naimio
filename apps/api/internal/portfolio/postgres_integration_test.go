//go:build integration

package portfolio

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresPortfolioRepository(t *testing.T) {
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
	ownerID := "99999999-9999-4999-8999-999999999999"
	otherID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaab"
	categoryID := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	skillID := "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	cleanMediaID := "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	pendingMediaID := "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	incompleteMediaID := "ffffffff-ffff-4fff-8fff-ffffffffffff"
	if _, err = database.ExecContext(ctx, `INSERT INTO users (id, email, email_normalized, password_hash, username, username_normalized, display_name) VALUES
($1, 'portfolio-owner@example.invalid', 'portfolio-owner@example.invalid', 'test-only', 'PortfolioOwner', 'portfolioowner', 'Portfolio Owner'),
($2, 'portfolio-other@example.invalid', 'portfolio-other@example.invalid', 'test-only', 'PortfolioOther', 'portfolioother', 'Portfolio Other')`, ownerID, otherID); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO categories (id, slug, name) VALUES ($1, 'portfolio-category', 'Portfolio Category')`, categoryID); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO skills (id, slug, name) VALUES ($1, 'portfolio-skill', 'Portfolio Skill')`, skillID); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO media_objects (id, owner_user_id, object_key, bucket, mime_type, size_bytes, scan_status, uploaded_at) VALUES
($1, $2, 'portfolio/clean.jpg', 'test', 'image/jpeg', 10, 'CLEAN', now()),
($3, $2, 'portfolio/pending.jpg', 'test', 'image/jpeg', 10, 'PENDING', now()),
($4, $2, 'portfolio/incomplete.jpg', 'test', 'image/jpeg', 10, 'CLEAN', NULL)`, cleanMediaID, ownerID, pendingMediaID, incompleteMediaID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, q := range []struct {
			query string
			args  []any
		}{
			{query: "DELETE FROM portfolio_items WHERE user_id IN ($1, $2)", args: []any{ownerID, otherID}},
			{query: "DELETE FROM media_objects WHERE owner_user_id IN ($1, $2)", args: []any{ownerID, otherID}},
			{query: "DELETE FROM users WHERE id IN ($1, $2)", args: []any{ownerID, otherID}},
			{query: "DELETE FROM skills WHERE id = $1", args: []any{skillID}},
			{query: "DELETE FROM categories WHERE id = $1", args: []any{categoryID}},
		} {
			_, _ = database.ExecContext(context.Background(), q.query, q.args...)
		}
	})

	repository := PostgresRepository{DB: database}
	input := WriteRequest{Title: "Portfolio Work", Slug: "portfolio-work", Description: "Plain text",
		ExternalURL: "https://example.com/work", CompletedOn: "2025-01-01", Visibility: "PUBLIC",
		CategoryIDs: []string{categoryID}, SkillIDs: []string{skillID}, MediaObjectIDs: []string{cleanMediaID}}
	created, err := repository.Create(ctx, ownerID, input)
	if err != nil || len(created.Categories) != 1 || len(created.Skills) != 1 || len(created.Media) != 1 {
		t.Fatalf("created = %#v, error = %v", created, err)
	}
	if _, err := repository.GetOwned(ctx, otherID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user get error = %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		updated, updateErr := repository.Update(ctx, ownerID, created.ID, input)
		if updateErr != nil || len(updated.Media) != 1 {
			t.Fatalf("attempt %d item = %#v, error = %v", attempt, updated, updateErr)
		}
	}
	if _, err := repository.AttachMedia(ctx, ownerID, created.ID, pendingMediaID, 1); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("pending media error = %v", err)
	}
	if _, err := repository.AttachMedia(ctx, ownerID, created.ID, incompleteMediaID, 1); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("incomplete media error = %v", err)
	}
	if _, err := repository.Update(ctx, otherID, created.ID, input); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user update error = %v", err)
	}

	secondInput := input
	secondInput.Slug, secondInput.Title, secondInput.MediaObjectIDs, secondInput.SortOrder = "second-work", "Second", nil, 1
	second, err := repository.Create(ctx, ownerID, secondInput)
	if err != nil {
		t.Fatal(err)
	}
	firstPage, err := repository.ListPublic(ctx, "PORTFOLIOOWNER", nil, 1)
	if err != nil || len(firstPage.Items) != 1 || firstPage.NextCursor == nil || firstPage.Items[0].Media[0].ScanStatus != "" {
		t.Fatalf("first page = %#v, error = %v", firstPage, err)
	}
	secondPage, err := repository.ListPublic(ctx, "portfolioowner", firstPage.NextCursor, 1)
	if err != nil || len(secondPage.Items) != 1 || secondPage.Items[0].ID != second.ID || secondPage.NextCursor != nil {
		t.Fatalf("second page = %#v, error = %v", secondPage, err)
	}
	if err := repository.Delete(ctx, otherID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user delete error = %v", err)
	}
	if err := repository.Delete(ctx, ownerID, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.GetOwned(ctx, ownerID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted item error = %v", err)
	}
	if _, err := repository.Create(ctx, ownerID, input); err != nil {
		t.Fatalf("slug reuse after soft delete: %v", err)
	}
}
