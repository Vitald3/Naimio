//go:build integration

package reviews

import (
	"context"
	"database/sql"
	"errors"
	_ "github.com/jackc/pgx/v5/stdlib"
	"os"
	"testing"
)

func TestPostgresReviewEligibilityTrustAndModeration(t *testing.T) {
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
	customer, freelancer, admin := "e1101010-1010-4010-8010-101010101010", "e2202020-2020-4020-8020-202020202020", "e3303030-3030-4030-8030-303030303030"
	project, assignment := "e4404040-4040-4040-8040-404040404040", "e5505050-5050-4050-8050-505050505050"
	if _, err = db.ExecContext(ctx, `INSERT INTO users(id,email,email_normalized,password_hash,username,username_normalized,display_name)VALUES($1,'review-customer@example.invalid','review-customer@example.invalid','x','review-customer','review-customer','Customer'),($2,'review-freelancer@example.invalid','review-freelancer@example.invalid','x','review-freelancer','review-freelancer','Freelancer'),($3,'review-admin@example.invalid','review-admin@example.invalid','x','review-admin','review-admin','Admin')`, customer, freelancer, admin); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO professional_profiles(user_id,profile_visibility)VALUES($1,'PUBLIC')`, freelancer); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO user_roles(user_id,role)VALUES($1,'MODERATOR')`, admin); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO projects(id,customer_user_id,title,slug,description,budget_type,currency,visibility,status,published_at)VALUES($1,$2,'Done','review-done','Done project','NEGOTIABLE','RUB','PRIVATE','COMPLETED',now())`, project, customer); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO project_assignments(id,project_id,freelancer_user_id,agreed_price_kopecks,status,started_at,completed_at)VALUES($1,$2,$3,100000,'COMPLETED',now()-interval '1 day',now())`, assignment, project, freelancer); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO safe_deals(project_id,assignment_id,customer_user_id,freelancer_user_id,work_amount_kopecks,gross_amount_kopecks,platform_fee_kopecks,platform_fee_freelancer_kopecks,freelancer_amount_kopecks,platform_net_revenue_kopecks,fee_rule_version,status,proposal_snapshot,project_snapshot,completed_at)VALUES($1,$2,$3,$4,100000,100000,10000,10000,90000,10000,1,'COMPLETED','{}','{}',now())`, project, assignment, customer, freelancer); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, q := range []struct {
			query string
			args  []any
		}{
			{query: "DELETE FROM outbox_events WHERE aggregate_type='REVIEW'"},
			{query: "DELETE FROM audit_logs WHERE actor_user_id=$1", args: []any{admin}},
			{query: "DELETE FROM fraud_signals WHERE user_id IN($1,$2) OR entity_type='REVIEW'", args: []any{customer, freelancer}},
			{query: "DELETE FROM reports WHERE reporter_user_id IN($1,$2)", args: []any{customer, freelancer}},
			{query: "DELETE FROM user_trust_stats WHERE user_id IN($1,$2)", args: []any{customer, freelancer}},
			{query: "DELETE FROM review_dimensions WHERE review_id IN(SELECT id FROM reviews WHERE project_id=$1)", args: []any{project}},
			{query: "DELETE FROM reviews WHERE project_id=$1", args: []any{project}},
			{query: "DELETE FROM safe_deals WHERE project_id=$1", args: []any{project}},
			{query: "DELETE FROM project_assignments WHERE project_id=$1", args: []any{project}},
			{query: "DELETE FROM projects WHERE id=$1", args: []any{project}},
			{query: "DELETE FROM user_roles WHERE user_id=$1", args: []any{admin}},
			{query: "DELETE FROM professional_profiles WHERE user_id=$1", args: []any{freelancer}},
			{query: "DELETE FROM users WHERE id IN($1,$2,$3)", args: []any{admin, customer, freelancer}},
		} {
			_, _ = db.ExecContext(context.Background(), q.query, q.args...)
		}
	})
	service := Service{Repository: PostgresRepository{DB: db}}
	yes := true
	review, err := service.Create(ctx, customer, project, Input{RatingOverall: 5, WouldWorkAgain: &yes, Text: "Great", Dimensions: map[string]int{"QUALITY": 5}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Create(ctx, customer, project, Input{RatingOverall: 5, Dimensions: map[string]int{"QUALITY": 5}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate=%v", err)
	}
	data, err := service.Public(ctx, "review-freelancer", nil, 20)
	if err != nil || len(data.Reviews.Items) != 1 || data.Trust.ReviewsCount != 1 || data.Trust.RecommendationRate != nil {
		t.Fatalf("data=%#v err=%v", data, err)
	}
	if err = service.Report(ctx, freelancer, review.ID, ReportInput{ReasonCode: "OTHER"}); err != nil {
		t.Fatal(err)
	}
	hidden, err := service.Moderate(ctx, admin, review.ID, "hide", "reported")
	if err != nil || hidden.Status != "HIDDEN" {
		t.Fatalf("hidden=%#v err=%v", hidden, err)
	}
	data, err = service.Public(ctx, "review-freelancer", nil, 20)
	if err != nil || len(data.Reviews.Items) != 0 || data.Trust.ReviewsCount != 0 {
		t.Fatalf("hidden data=%#v err=%v", data, err)
	}
}
