//go:build integration

package matching

import (
	"context"
	"database/sql"
	"errors"
	_ "github.com/jackc/pgx/v5/stdlib"
	"os"
	"testing"
)

func TestPostgresEligibilityPersistenceManualAndMetrics(t *testing.T) {
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
	owner, eligible, privateUser, banned, admin := "fc101010-1010-4010-8010-101010101010", "fc202020-2020-4020-8020-202020202020", "fc303030-3030-4030-8030-303030303030", "fc404040-4040-4040-8040-404040404040", "fc505050-5050-4050-8050-505050505050"
	category, skill, project := "fc606060-6060-4060-8060-606060606060", "fc707070-7070-4070-8070-707070707070", "fc808080-8080-4080-8080-808080808080"
	if _, err = db.ExecContext(ctx, `INSERT INTO users(id,email,email_normalized,password_hash,username,username_normalized,display_name,status)VALUES
($1,'match-owner@example.invalid','match-owner@example.invalid','x','match-owner','match-owner','Owner','ACTIVE'),
($2,'match-ok@example.invalid','match-ok@example.invalid','x','match-ok','match-ok','Eligible','ACTIVE'),
($3,'match-private@example.invalid','match-private@example.invalid','x','match-private','match-private','Private','ACTIVE'),
($4,'match-banned@example.invalid','match-banned@example.invalid','x','match-banned','match-banned','Banned','BANNED'),
($5,'match-admin@example.invalid','match-admin@example.invalid','x','match-admin','match-admin','Admin','ACTIVE')`, owner, eligible, privateUser, banned, admin); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO user_capabilities(user_id,capability)VALUES($1,'CUSTOMER'),($2,'FREELANCER'),($3,'FREELANCER'),($4,'FREELANCER')`, owner, eligible, privateUser, banned); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO user_roles(user_id,role)VALUES($1,'ADMIN')`, admin); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO categories(id,slug,name)VALUES($1,'match-category','Match category')`, category); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO skills(id,slug,name)VALUES($1,'match-skill','Match skill')`, skill); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO professional_profiles(user_id,professional_title,availability,profile_visibility,profile_completion,minimum_order_kopecks)VALUES
($1,'Eligible','AVAILABLE','PUBLIC',90,5000000),
($2,'Private','AVAILABLE','PRIVATE',100,100),
($3,'Banned','AVAILABLE','PUBLIC',100,100)`, eligible, privateUser, banned); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO profile_categories(user_id,category_id,is_primary)VALUES($1,$2,true),($3,$2,true),($4,$2,true)`, eligible, category, privateUser, banned); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO profile_skills(user_id,skill_id,is_featured)VALUES($1,$2,true),($3,$2,true),($4,$2,true)`, eligible, skill, privateUser, banned); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO projects(id,customer_user_id,category_id,title,slug,description,budget_type,budget_max_kopecks,budget_min_kopecks,currency,visibility,status,source_type,published_at)VALUES($1,$2,$3,'Match','match-integration','Go work','RANGE',10000000,5000000,'RUB','PUBLIC','OPEN','MANUAL',now())`, project, owner, category); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO project_skills(project_id,skill_id,importance)VALUES($1,$2,100)`, project, skill); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, q := range []struct {
			query string
			args  []any
		}{
			{query: "DELETE FROM audit_logs WHERE actor_user_id=$1", args: []any{admin}},
			{query: "DELETE FROM matching_quality_events WHERE project_id=$1", args: []any{project}},
			{query: "DELETE FROM manual_project_recommendations WHERE project_id=$1", args: []any{project}},
			{query: "DELETE FROM matching_runs WHERE project_id=$1", args: []any{project}},
			{query: "DELETE FROM project_skills WHERE project_id=$1", args: []any{project}},
			{query: "DELETE FROM projects WHERE id=$1", args: []any{project}},
			{query: "DELETE FROM profile_skills WHERE user_id IN($1,$2,$3)", args: []any{eligible, privateUser, banned}},
			{query: "DELETE FROM profile_categories WHERE user_id IN($1,$2,$3)", args: []any{eligible, privateUser, banned}},
			{query: "DELETE FROM professional_profiles WHERE user_id IN($1,$2,$3)", args: []any{eligible, privateUser, banned}},
			{query: "DELETE FROM skills WHERE id=$1", args: []any{skill}},
			{query: "DELETE FROM categories WHERE id=$1", args: []any{category}},
			{query: "DELETE FROM user_roles WHERE user_id=$1", args: []any{admin}},
			{query: "DELETE FROM user_capabilities WHERE user_id IN($1,$2,$3,$4)", args: []any{owner, eligible, privateUser, banned}},
			{query: "DELETE FROM users WHERE id IN($1,$2,$3,$4,$5)", args: []any{owner, eligible, privateUser, banned, admin}},
		} {
			_, _ = db.ExecContext(context.Background(), q.query, q.args...)
		}
	})
	repository := PostgresRepository{DB: db}
	service := Service{Repository: repository, Weights: DefaultWeights(), RetrievalLimit: 100, ShortlistLimit: 20}
	run, err := service.Run(ctx, owner, project, Constraints{RequireImmediateAvailability: true, RequireCategoryMatch: true, MaxMinimumOrderKopecks: pointer[int64](10_000_000)})
	if err != nil || run.CandidateCount != 1 || run.Recommendations[0].FreelancerID != eligible {
		t.Fatalf("run=%#v err=%v", run, err)
	}
	if _, err = service.Run(ctx, eligible, project, Constraints{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("BOLA=%v", err)
	}
	if err = service.PutManual(ctx, admin, project, eligible, "Concierge fit"); err != nil {
		t.Fatal(err)
	}
	latest, err := service.Latest(ctx, owner, project)
	if err != nil || !latest.Recommendations[0].Manual {
		t.Fatalf("latest=%#v err=%v", latest, err)
	}
	if err = service.Event(ctx, owner, project, run.ID, eligible, "IMPRESSION", "integration-event"); err != nil {
		t.Fatal(err)
	}
	if err = service.Event(ctx, owner, project, run.ID, eligible, "IMPRESSION", "integration-event"); err != nil {
		t.Fatalf("idempotent event=%v", err)
	}
	metrics, err := service.Metrics(ctx, admin)
	if err != nil || metrics.Runs < 1 || metrics.Impressions < 1 {
		t.Fatalf("metrics=%#v err=%v", metrics, err)
	}
}
