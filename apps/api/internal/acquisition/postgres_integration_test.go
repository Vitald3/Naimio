//go:build integration

package acquisition

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"freelance/apps/api/internal/ai"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresDefinitionsDraftAndPrivacySafeEvent(t *testing.T) {
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
	repository := PostgresRepository{DB: db}
	service := Service{Repository: repository, Drafts: ai.Service{Drafts: ai.PostgresRepository{DB: db}}}

	definition, err := repository.Definition(ctx, "landing-page")
	if err != nil || definition.Version != 2 || len(definition.Questions) != 3 {
		t.Fatalf("definition=%+v err=%v", definition, err)
	}
	result, err := service.Estimate(ctx, "", "landing-page", map[string]any{"design": "template", "sections": float64(5), "copywriting": false}, Attribution{AnonymousID: anonymous, LandingPath: "/price/landing-page", UTMSource: "integration"})
	if err != nil || len(result.DraftToken) != 64 || result.EstimatedMinKopecks <= 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM acquisition_events WHERE anonymous_id=$1`, anonymous)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM project_drafts WHERE source_type='CALCULATOR' AND raw_input->'attribution'->>'anonymous_id'=$1`, anonymous)
	})
	var count int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM acquisition_events WHERE anonymous_id=$1 AND event_type='CALCULATOR_COMPLETED'`, anonymous).Scan(&count); err != nil || count != 1 {
		t.Fatalf("events=%d err=%v", count, err)
	}
}
