//go:build integration

package proposals

import (
	"context"
	"database/sql"
	"errors"
	_ "github.com/jackc/pgx/v5/stdlib"
	"os"
	"testing"
)

func TestPostgresProposalAcceptance(t *testing.T) {
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
	customer, free, other := "b1101010-1010-4010-8010-101010101010", "b2202020-2020-4020-8020-202020202020", "b3303030-3030-4030-8030-303030303030"
	project := "b4404040-4040-4040-8040-404040404040"
	if _, err = db.ExecContext(ctx, `INSERT INTO users(id,email,email_normalized,password_hash,display_name)VALUES($1,'proposal-customer@example.invalid','proposal-customer@example.invalid','x','Customer'),($2,'proposal-free@example.invalid','proposal-free@example.invalid','x','Free'),($3,'proposal-other@example.invalid','proposal-other@example.invalid','x','Other')`, customer, free, other); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO user_capabilities(user_id,capability)VALUES($1,'CUSTOMER'),($2,'FREELANCER'),($3,'FREELANCER')`, customer, free, other); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO professional_profiles(user_id,profile_visibility)VALUES($1,'PUBLIC'),($2,'PUBLIC')`, free, other); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO projects(id,customer_user_id,title,slug,description,budget_type,currency,visibility,status,source_type,published_at)VALUES($1,$2,'Proposal project','proposal-it','Text','NEGOTIABLE','RUB','PUBLIC','OPEN','MANUAL',now())`, project, customer); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, q := range []struct {
			query string
			args  []any
		}{
			{query: "DELETE FROM outbox_events WHERE aggregate_id=$1 OR aggregate_id IN(SELECT id FROM safe_deals WHERE project_id=$1)", args: []any{project}},
			{query: "DELETE FROM safe_deal_events WHERE deal_id IN(SELECT id FROM safe_deals WHERE project_id=$1)", args: []any{project}},
			{query: "DELETE FROM safe_deals WHERE project_id=$1", args: []any{project}},
			{query: "DELETE FROM project_assignments WHERE project_id=$1", args: []any{project}},
			{query: "DELETE FROM proposals WHERE project_id=$1", args: []any{project}},
			{query: "DELETE FROM projects WHERE id=$1", args: []any{project}},
			{query: "DELETE FROM professional_profiles WHERE user_id IN($1,$2)", args: []any{free, other}},
			{query: "DELETE FROM user_capabilities WHERE user_id IN($1,$2,$3)", args: []any{free, other, customer}},
			{query: "DELETE FROM users WHERE id IN($1,$2,$3)", args: []any{free, other, customer}},
		} {
			_, _ = db.ExecContext(context.Background(), q.query, q.args...)
		}
	})
	r := PostgresRepository{DB: db}
	input := proposalInput()
	first, err := r.Submit(ctx, free, project, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Submit(ctx, free, project, input); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate=%v", err)
	}
	second, err := r.Submit(ctx, other, project, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Act(ctx, other, project, first.ID, "accept"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("owner=%v", err)
	}
	accepted, err := r.Act(ctx, customer, project, first.ID, "accept")
	if err != nil || accepted.Status != "ACCEPTED" || accepted.SafeDealID == "" {
		t.Fatalf("accepted=%#v err=%v", accepted, err)
	}
	if _, err = r.Act(ctx, customer, project, first.ID, "accept"); err != nil {
		t.Fatal(err)
	}
	var status, otherStatus string
	var assignments, events int
	if err = db.QueryRowContext(ctx, `SELECT status,(SELECT status FROM proposals WHERE id=$2),(SELECT count(*) FROM project_assignments WHERE project_id=$1),(SELECT count(*) FROM outbox_events WHERE aggregate_id=$1 AND event_type='PROPOSAL_ACCEPTED')FROM projects WHERE id=$1`, project, second.ID).Scan(&status, &otherStatus, &assignments, &events); err != nil || status != "AWAITING_FUNDING" || otherStatus != "REJECTED" || assignments != 1 || events != 1 {
		t.Fatalf("state=%s other=%s assignments=%d events=%d err=%v", status, otherStatus, assignments, events, err)
	}
}
