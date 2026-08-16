//go:build integration

package safedeal_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"freelance/apps/api/internal/proposals"
	"freelance/apps/api/internal/reviews"
	"freelance/apps/api/internal/safedeal"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresAcceptedProposalSafeDealAndReviewGate(t *testing.T) {
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
	customer, freelancer, admin := "da101010-1010-4010-8010-101010101010", "da202020-2020-4020-8020-202020202020", "da303030-3030-4030-8030-303030303030"
	project := "da404040-4040-4040-8040-404040404040"
	if _, e = db.ExecContext(ctx, `INSERT INTO users(id,email,email_normalized,password_hash,display_name)VALUES($1,'deal-c@example.invalid','deal-c@example.invalid','x','Customer'),($2,'deal-f@example.invalid','deal-f@example.invalid','x','Freelancer'),($3,'deal-a@example.invalid','deal-a@example.invalid','x','Admin')`, customer, freelancer, admin); e != nil {
		t.Fatal(e)
	}
	if _, e = db.ExecContext(ctx, `INSERT INTO user_capabilities(user_id,capability)VALUES($1,'CUSTOMER'),($2,'FREELANCER')`, customer, freelancer); e != nil {
		t.Fatal(e)
	}
	if _, e = db.ExecContext(ctx, `INSERT INTO user_roles(user_id,role)VALUES($1,'ADMIN')`, admin); e != nil {
		t.Fatal(e)
	}
	if _, e = db.ExecContext(ctx, `INSERT INTO professional_profiles(user_id,profile_visibility)VALUES($1,'PUBLIC')`, freelancer); e != nil {
		t.Fatal(e)
	}
	if _, e = db.ExecContext(ctx, `INSERT INTO projects(id,customer_user_id,title,slug,description,budget_type,budget_min_kopecks,currency,visibility,status,source_type,published_at)VALUES($1,$2,'Safe Deal','safe-deal-integration','Agreed integration scope','FIXED',1000000,'RUB','PUBLIC','OPEN','MANUAL',now())`, project, customer); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		for _, q := range []struct {
			query string
			args  []any
		}{
			{query: "DELETE FROM review_dimensions WHERE review_id IN(SELECT id FROM reviews WHERE project_id=$1)", args: []any{project}},
			{query: "DELETE FROM reviews WHERE project_id=$1", args: []any{project}},
			{query: "DELETE FROM notifications WHERE entity_id IN(SELECT id FROM safe_deals WHERE project_id=$1)", args: []any{project}},
			{query: "DELETE FROM email_jobs WHERE payload->>'entity_id' IN(SELECT id::text FROM safe_deals WHERE project_id=$1)", args: []any{project}},
			{query: "DELETE FROM payment_events WHERE payment_record_id IN(SELECT id FROM payment_records WHERE deal_id IN(SELECT id FROM safe_deals WHERE project_id=$1))", args: []any{project}},
			{query: "DELETE FROM safe_deal_dispute_evidence WHERE dispute_id IN(SELECT id FROM safe_deal_disputes WHERE deal_id IN(SELECT id FROM safe_deals WHERE project_id=$1))", args: []any{project}},
			{query: "DELETE FROM safe_deal_disputes WHERE deal_id IN(SELECT id FROM safe_deals WHERE project_id=$1)", args: []any{project}},
			{query: "DELETE FROM safe_deal_submissions WHERE deal_id IN(SELECT id FROM safe_deals WHERE project_id=$1)", args: []any{project}},
			{query: "DELETE FROM safe_deal_command_results WHERE deal_id IN(SELECT id FROM safe_deals WHERE project_id=$1)", args: []any{project}},
			{query: "DELETE FROM safe_deal_events WHERE deal_id IN(SELECT id FROM safe_deals WHERE project_id=$1)", args: []any{project}},
			{query: "DELETE FROM payment_records WHERE deal_id IN(SELECT id FROM safe_deals WHERE project_id=$1)", args: []any{project}},
			{query: "DELETE FROM outbox_events WHERE aggregate_id=$1 OR aggregate_id IN(SELECT id FROM safe_deals WHERE project_id=$1)", args: []any{project}},
			{query: "DELETE FROM safe_deals WHERE project_id=$1", args: []any{project}},
			{query: "DELETE FROM project_assignments WHERE project_id=$1", args: []any{project}},
			{query: "DELETE FROM proposals WHERE project_id=$1", args: []any{project}},
			{query: "DELETE FROM projects WHERE id=$1", args: []any{project}},
			{query: "DELETE FROM professional_profiles WHERE user_id=$1", args: []any{freelancer}},
			{query: "DELETE FROM user_roles WHERE user_id=$1", args: []any{admin}},
			{query: "DELETE FROM user_capabilities WHERE user_id IN($1,$2)", args: []any{freelancer, customer}},
			{query: "DELETE FROM users WHERE id IN($1,$2,$3)", args: []any{freelancer, admin, customer}},
		} {
			_, _ = db.ExecContext(context.Background(), q.query, q.args...)
		}
	})
	price := int64(1000000)
	days := 7
	proposalRepo := proposals.PostgresRepository{DB: db}
	proposal, e := proposalRepo.Submit(ctx, freelancer, project, proposals.Input{Message: "Agreed work", PriceKopecks: &price, Currency: "RUB", DeliveryDays: &days})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = proposalRepo.Act(ctx, customer, project, proposal.ID, "accept"); e != nil {
		t.Fatal(e)
	}
	repo := safedeal.PostgresRepository{DB: db}
	deals, e := repo.List(ctx, customer, project)
	if e != nil || len(deals) != 1 || deals[0].Status != "AWAITING_FUNDING" || deals[0].PlatformFeeKopecks != 100000 {
		t.Fatalf("deals=%+v err=%v", deals, e)
	}
	secret := "integration-sandbox-secret"
	provider := safedeal.NewSandboxProvider(secret)
	service := safedeal.Service{Repository: repo, Provider: provider}
	deal := deals[0]
	deal, e = service.Funding(ctx, customer, deal.ID, "integration-funding")
	if e != nil {
		t.Fatal(e)
	}
	payment := deal.Payment.ProviderPaymentID
	send := func(eventType, state, eventID string) {
		t.Helper()
		now := time.Now().UTC()
		amount := deal.GrossAmountKopecks
		if eventType == "RELEASE_CONFIRMED" {
			amount = deal.FreelancerAmountKopecks
		}
		body := []byte(fmt.Sprintf(`{"id":%q,"payment_id":%q,"type":%q,"state":%q,"amount_kopecks":%d,"currency":"RUB"}`, eventID, payment, eventType, state, amount))
		stamp := fmt.Sprint(now.Unix())
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(stamp + "."))
		_, _ = mac.Write(body)
		if _, _, err := service.Webhook(ctx, map[string][]string{"X-Sandbox-Timestamp": {stamp}, "X-Sandbox-Signature": {hex.EncodeToString(mac.Sum(nil))}}, body); err != nil {
			t.Fatal(err)
		}
	}
	send("FUNDING_CONFIRMED", "FUNDED", "integration-funded")
	if _, e = service.Start(ctx, freelancer, deal.ID, "integration-start"); e != nil {
		t.Fatal(e)
	}
	if _, e = service.Submit(ctx, freelancer, deal.ID, "integration-submit", safedeal.SubmitInput{Summary: "Delivered"}); e != nil {
		t.Fatal(e)
	}
	if _, e = service.Accept(ctx, customer, deal.ID, "integration-accept"); e != nil {
		t.Fatal(e)
	}
	send("RELEASE_CONFIRMED", "RELEASED", "integration-released")
	var status string
	if e = db.QueryRowContext(ctx, `SELECT status FROM projects WHERE id=$1`, project).Scan(&status); e != nil || status != "COMPLETED" {
		t.Fatalf("project=%s err=%v", status, e)
	}
	again := true
	reviewRepo := reviews.PostgresRepository{DB: db}
	if _, e = reviewRepo.Create(ctx, customer, project, reviews.Input{RatingOverall: 5, WouldWorkAgain: &again, Text: "Completed Safe Deal", Dimensions: map[string]int{"QUALITY": 5, "COMMUNICATION": 5, "DEADLINE": 5}}); e != nil {
		t.Fatalf("review gate=%v", e)
	}
}
