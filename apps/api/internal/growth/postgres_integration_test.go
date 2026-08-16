//go:build integration

package growth

import (
	"context"
	"database/sql"
	"errors"
	_ "github.com/jackc/pgx/v5/stdlib"
	"os"
	"testing"
	"time"
)

func TestPostgresGrowthInvitesRewardsRepeatShareAndTeam(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Fatal("DATABASE_URL is required")
	}
	db, e := sql.Open("pgx", url)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	ctx := context.Background()
	free, customer, other, admin := "fb101010-1010-4010-8010-101010101010", "fb202020-2020-4020-8020-202020202020", "fb303030-3030-4030-8030-303030303030", "fb404040-4040-4040-8040-404040404040"
	category, openProject, completedProject, assignment := "fb505050-5050-4050-8050-505050505050", "fb606060-6060-4060-8060-606060606060", "fb707070-7070-4070-8070-707070707070", "fb808080-8080-4080-8080-808080808080"
	if _, e = db.ExecContext(ctx, `INSERT INTO users(id,email,email_normalized,password_hash,username,username_normalized,display_name)VALUES($1,'growth-free@example.invalid','growth-free@example.invalid','x','growth-free','growth-free','Free'),($2,'growth-customer@example.invalid','growth-customer@example.invalid','x','growth-customer','growth-customer','Customer'),($3,'growth-other@example.invalid','growth-other@example.invalid','x','growth-other','growth-other','Other'),($4,'growth-admin@example.invalid','growth-admin@example.invalid','x','growth-admin','growth-admin','Admin')`, free, customer, other, admin); e != nil {
		t.Fatal(e)
	}
	if _, e = db.ExecContext(ctx, `INSERT INTO user_capabilities(user_id,capability)VALUES($1,'FREELANCER'),($2,'CUSTOMER'),($3,'FREELANCER')`, free, customer, other); e != nil {
		t.Fatal(e)
	}
	if _, e = db.ExecContext(ctx, `INSERT INTO user_roles(user_id,role)VALUES($1,'ADMIN')`, admin); e != nil {
		t.Fatal(e)
	}
	if _, e = db.ExecContext(ctx, `INSERT INTO professional_profiles(user_id,availability,profile_visibility)VALUES($1,'AVAILABLE','PUBLIC'),($2,'PARTIALLY_BUSY','PUBLIC')`, free, other); e != nil {
		t.Fatal(e)
	}
	if _, e = db.ExecContext(ctx, `INSERT INTO categories(id,slug,name)VALUES($1,'growth','Growth')`, category); e != nil {
		t.Fatal(e)
	}
	if _, e = db.ExecContext(ctx, `INSERT INTO projects(id,customer_user_id,category_id,title,slug,description,budget_type,currency,visibility,status,source_type,published_at)VALUES($1,$2,$3,'Open','growth-open','Open project','NEGOTIABLE','RUB','PUBLIC','OPEN','MANUAL',now()),($4,$2,$3,'Completed','growth-completed','Completed project','NEGOTIABLE','RUB','PUBLIC','COMPLETED','MANUAL',now())`, openProject, customer, category, completedProject); e != nil {
		t.Fatal(e)
	}
	if _, e = db.ExecContext(ctx, `INSERT INTO project_assignments(id,project_id,freelancer_user_id,status,started_at,completed_at)VALUES($1,$2,$3,'COMPLETED',now()-interval '2 days',now()-interval '1 day')`, assignment, completedProject, free); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		for _, q := range []struct {
			query string
			args  []any
		}{
			{query: "DELETE FROM email_jobs WHERE user_id IN($1,$2,$3,$4)", args: []any{free, customer, other, admin}},
			{query: "DELETE FROM notifications WHERE user_id IN($1,$2,$3,$4)", args: []any{free, customer, other, admin}},
			{query: "DELETE FROM audit_logs WHERE actor_user_id=$1", args: []any{admin}},
			{query: "DELETE FROM outbox_events WHERE aggregate_type IN('INVITE','PROJECT')"},
			{query: "DELETE FROM customer_team_members WHERE customer_user_id=$1", args: []any{customer}},
			{query: "DELETE FROM reward_ledger WHERE user_id IN($1,$2,$3)", args: []any{free, customer, other}},
			{query: "DELETE FROM referral_attributions WHERE invited_user_id IN($1,$2,$3)", args: []any{free, customer, other}},
			{query: "DELETE FROM project_invited_users WHERE user_id IN($1,$2,$3)", args: []any{free, customer, other}},
			{query: "DELETE FROM invites WHERE inviter_user_id IN($1,$2,$3)", args: []any{free, customer, other}},
			{query: "DELETE FROM referral_rules WHERE code='GROWTH_TEST'"},
			{query: "DELETE FROM project_skills WHERE project_id IN(SELECT id FROM projects WHERE customer_user_id=$1)", args: []any{customer}},
			{query: "DELETE FROM project_assignments WHERE project_id IN(SELECT id FROM projects WHERE customer_user_id=$1)", args: []any{customer}},
			{query: "DELETE FROM projects WHERE customer_user_id=$1", args: []any{customer}},
			{query: "DELETE FROM categories WHERE id=$1", args: []any{category}},
			{query: "DELETE FROM professional_profiles WHERE user_id IN($1,$2)", args: []any{free, other}},
			{query: "DELETE FROM user_roles WHERE user_id=$1", args: []any{admin}},
			{query: "DELETE FROM user_capabilities WHERE user_id IN($1,$2,$3)", args: []any{free, customer, other}},
			{query: "DELETE FROM users WHERE id IN($1,$2,$3,$4)", args: []any{free, customer, other, admin}},
		} {
			_, _ = db.ExecContext(context.Background(), q.query, q.args...)
		}
	})
	svc := Service{Repository: PostgresRepository{DB: db}, PublicBaseURL: "https://example.test", Now: func() time.Time { return time.Now().UTC() }}
	if _, e = svc.CreateRule(ctx, admin, RuleInput{Code: "GROWTH_TEST", EventType: "INVITE_ACCEPTED", Beneficiary: "INVITER", RewardType: "BONUS", RewardValue: 1, RewardUnit: "CREDITS", Enabled: true}); e != nil {
		t.Fatal(e)
	}
	invite, e := svc.CreateInvite(ctx, free, InviteInput{Type: "CUSTOMER", IntendedEmail: "growth-customer@example.invalid"})
	if e != nil {
		t.Fatal(e)
	}
	accepted, e := svc.Accept(ctx, customer, invite.Token, "growth-accept-key")
	if e != nil || accepted.RewardsIssued != 1 || !accepted.AttributionCreated {
		t.Fatalf("accept=%#v err=%v", accepted, e)
	}
	again, e := svc.Accept(ctx, customer, invite.Token, "growth-accept-key")
	if e != nil || again.InviteID != accepted.InviteID {
		t.Fatalf("replay=%#v err=%v", again, e)
	}
	if _, e = svc.Accept(ctx, free, invite.Token, "growth-self-key"); !errors.Is(e, ErrInvalid) {
		t.Fatalf("self=%v", e)
	}
	projectInvite, e := svc.CreateInvite(ctx, customer, InviteInput{Type: "PROJECT", ProjectID: openProject, IntendedEmail: "growth-other@example.invalid"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Accept(ctx, other, projectInvite.Token, "growth-project-key"); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Share(ctx, customer, openProject); e != nil {
		t.Fatal(e)
	}
	repeat, e := svc.Repeat(ctx, customer, completedProject, RepeatInput{InvitePreviousFreelancer: true})
	if e != nil || repeat.Invite == nil || repeat.Status != "DRAFT" {
		t.Fatalf("repeat=%#v err=%v", repeat, e)
	}
	if _, e = svc.PutTeam(ctx, customer, free, TeamInput{Label: "Trusted", Notes: "Safe note"}); e != nil {
		t.Fatal(e)
	}
	team, e := svc.Team(ctx, customer)
	if e != nil || len(team) != 1 || team[0].LastProjectID != completedProject {
		t.Fatalf("team=%#v err=%v", team, e)
	}
	var attribution, rewards int
	if e = db.QueryRowContext(ctx, `SELECT(SELECT count(*)FROM referral_attributions WHERE invited_user_id=$1),(SELECT count(*)FROM reward_ledger WHERE user_id=$2)`, customer, free).Scan(&attribution, &rewards); e != nil || attribution != 1 || rewards != 1 {
		t.Fatalf("attribution=%d rewards=%d err=%v", attribution, rewards, e)
	}
}
