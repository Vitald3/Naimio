//go:build integration

package jobs

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresVacancyFlow(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Fatal("DATABASE_URL is required for integration tests")
	}
	db, e := sql.Open("pgx", url)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	ctx := context.Background()
	owner := "71717171-7171-4717-8717-717171717171"
	other := "72727272-7272-4727-8727-727272727272"
	applicant := "73737373-7373-4737-8737-737373737373"
	admin := "74747474-7474-4747-8747-747474747474"
	category := "75757575-7575-4757-8757-757575757575"
	skill := "76767676-7676-4767-8767-767676767676"
	if _, e = db.ExecContext(ctx, `INSERT INTO users(id,email,email_normalized,password_hash,username,username_normalized,display_name)VALUES
($1,'job-owner@example.invalid','job-owner@example.invalid','x','job-owner','job-owner','Owner'),
($2,'job-other@example.invalid','job-other@example.invalid','x','job-other','job-other','Other'),
($3,'job-applicant@example.invalid','job-applicant@example.invalid','x','job-applicant','job-applicant','Applicant'),
($4,'job-admin@example.invalid','job-admin@example.invalid','x','job-admin','job-admin','Admin')`, owner, other, applicant, admin); e != nil {
		t.Fatal(e)
	}
	if _, e = db.ExecContext(ctx, `INSERT INTO user_capabilities(user_id,capability)VALUES($1,'CUSTOMER'),($2,'CUSTOMER'),($3,'FREELANCER')`, owner, other, applicant); e != nil {
		t.Fatal(e)
	}
	if _, e = db.ExecContext(ctx, `INSERT INTO user_roles(user_id,role)VALUES($1,'MODERATOR')`, admin); e != nil {
		t.Fatal(e)
	}
	if _, e = db.ExecContext(ctx, `INSERT INTO categories(id,slug,name)VALUES($1,'job-category','Job category')`, category); e != nil {
		t.Fatal(e)
	}
	if _, e = db.ExecContext(ctx, `INSERT INTO skills(id,slug,name)VALUES($1,'job-skill','Job skill')`, skill); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		for _, q := range []struct {
			query string
			args  []any
		}{
			{query: "DELETE FROM audit_logs WHERE target_type='JOB' AND actor_user_id=$1", args: []any{admin}},
			{query: "DELETE FROM outbox_events WHERE aggregate_type='JOB'"},
			{query: "DELETE FROM job_applications WHERE user_id=$1", args: []any{applicant}},
			{query: "DELETE FROM jobs WHERE customer_user_id IN($1,$2)", args: []any{owner, other}},
			{query: "DELETE FROM companies WHERE owner_user_id IN($1,$2)", args: []any{owner, other}},
			{query: "DELETE FROM user_roles WHERE user_id=$1", args: []any{admin}},
			{query: "DELETE FROM user_capabilities WHERE user_id IN($1,$2,$3)", args: []any{applicant, owner, other}},
			{query: "DELETE FROM users WHERE id IN($1,$2,$3,$4)", args: []any{admin, applicant, owner, other}},
			{query: "DELETE FROM skills WHERE id=$1", args: []any{skill}},
			{query: "DELETE FROM categories WHERE id=$1", args: []any{category}},
		} {
			_, _ = db.ExecContext(context.Background(), q.query, q.args...)
		}
	})
	r := PostgresRepository{DB: db}
	company, e := r.CreateCompany(ctx, owner, CompanyInput{Name: "Job Co", Slug: "job-co", Website: "https://example.com", Description: "Employer"})
	if e != nil {
		t.Fatal(e)
	}
	min, max := int64(10000000), int64(20000000)
	v, e := r.Create(ctx, owner, CreateRequest{CompanyID: company.ID, CategoryID: category, Title: "Go role", Slug: "go-role", Description: "Build Go APIs", EmploymentType: "FULL_TIME", SalaryMinKopecks: &min, SalaryMaxKopecks: &max, Currency: "RUB", Remote: true, ExperienceLevel: "MIDDLE", SkillIDs: []string{skill}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.GetOwned(ctx, other, v.ID); !errors.Is(e, ErrNotFound) {
		t.Fatalf("BOLA=%v", e)
	}
	v, e = r.Transition(ctx, owner, v.ID, "publish")
	if e != nil || v.Status != "PUBLISHED" {
		t.Fatalf("publish=%+v %v", v, e)
	}
	remote := true
	p, e := r.ListPublic(ctx, Filter{Q: "Go", Skill: "job-skill", Remote: &remote}, nil, 20)
	if e != nil || len(p.Items) != 1 {
		t.Fatalf("search=%+v %v", p, e)
	}
	a, e := r.Apply(ctx, applicant, v.ID, "Relevant background")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.Apply(ctx, applicant, v.ID, "duplicate"); !errors.Is(e, ErrConflict) {
		t.Fatalf("duplicate=%v", e)
	}
	if _, e = r.ListApplicants(ctx, other, v.ID); !errors.Is(e, ErrNotFound) {
		t.Fatalf("private=%v", e)
	}
	apps, e := r.ListApplicants(ctx, owner, v.ID)
	if e != nil || len(apps) != 1 {
		t.Fatalf("apps=%+v %v", apps, e)
	}
	if _, e = r.SetApplicationStatus(ctx, owner, v.ID, a.ID, "SHORTLISTED"); e != nil {
		t.Fatal(e)
	}
	if _, e = r.Moderate(ctx, admin, v.ID, "HIDE", "prohibited vacancy"); e != nil {
		t.Fatal(e)
	}
	if _, e = r.GetPublic(ctx, v.ID); !errors.Is(e, ErrNotFound) {
		t.Fatalf("hidden=%v", e)
	}
}
