package growth

import (
	"context"
	"encoding/json"
	"freelance/apps/api/internal/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const freelancer = "10000000-0000-4000-8000-000000000001"
const customer = "10000000-0000-4000-8000-000000000002"
const other = "10000000-0000-4000-8000-000000000003"
const project = "20000000-0000-4000-8000-000000000001"
const admin = "10000000-0000-4000-8000-000000000004"

func testService() (Service, *Store) {
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	s := &Store{Invites: map[string]StoredInvite{}, Users: map[string]bool{freelancer: true, customer: true, other: true, admin: true}, Emails: map[string]string{freelancer: "free@example.test", customer: "customer@example.test", other: "other@example.test"}, Capabilities: map[string]map[string]bool{freelancer: {"FREELANCER": true}, customer: {"CUSTOMER": true}, other: {"FREELANCER": true}}, Projects: map[string]StoreProject{project: {ID: project, Owner: customer, Title: "Completed", Category: "Design", Slug: "completed", Status: "COMPLETED", Visibility: "PUBLIC", PreviousFreelancer: freelancer}}, Attributions: map[string]Attribution{}, Rewards: map[string]Reward{}, RulesMap: map[string]Rule{}, Admins: map[string]bool{admin: true}, TeamMap: map[string]TeamMember{}, Invited: map[string]bool{}, Now: func() time.Time { return now }}
	return Service{Repository: s, PublicBaseURL: "https://example.test", Now: s.Now}, s
}
func TestInviteDirectionsSafePreviewAcceptanceAndRewardIdempotency(t *testing.T) {
	svc, s := testService()
	rule, err := svc.CreateRule(context.Background(), admin, RuleInput{Code: "WELCOME", EventType: "INVITE_ACCEPTED", Beneficiary: "INVITER", RewardType: "BONUS", RewardValue: 3, RewardUnit: "CREDITS", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if rule.Code != "WELCOME" {
		t.Fatal("rule normalization failed")
	}
	created, err := svc.CreateInvite(context.Background(), freelancer, InviteInput{Type: "customer", IntendedEmail: "customer@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(mapKeys(s.Invites), ""), created.Token) {
		t.Fatal("raw token persisted")
	}
	preview, err := svc.Preview(context.Background(), created.Token)
	if err != nil || preview.InvitedRole != "CUSTOMER" {
		t.Fatalf("preview=%#v err=%v", preview, err)
	}
	raw, _ := json.Marshal(preview)
	if strings.Contains(string(raw), "customer@example") || strings.Contains(string(raw), "intended") {
		t.Fatalf("private intended email leaked: %s", raw)
	}
	accepted, err := svc.Accept(context.Background(), customer, created.Token, "accept-key-123")
	if err != nil || !accepted.AttributionCreated || accepted.RewardsIssued != 1 {
		t.Fatalf("accept=%#v err=%v", accepted, err)
	}
	again, err := svc.Accept(context.Background(), customer, created.Token, "accept-key-123")
	if err != nil || again.InviteID != accepted.InviteID {
		t.Fatalf("idempotent accept failed: %#v %v", again, err)
	}
	if len(s.Attributions) != 1 || len(s.Rewards) != 1 {
		t.Fatalf("duplicate growth rows: %d %d", len(s.Attributions), len(s.Rewards))
	}
	if _, err = svc.Accept(context.Background(), freelancer, created.Token, "self-key-123"); err != ErrInvalid {
		t.Fatalf("self referral=%v", err)
	}
}
func TestProjectInviteRepeatShareAndTeamAuthorization(t *testing.T) {
	svc, s := testService()
	invite, err := svc.CreateInvite(context.Background(), customer, InviteInput{Type: "FREELANCER", ProjectID: project, IntendedEmail: "free@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Accept(context.Background(), freelancer, invite.Token, "project-key-1"); err != nil {
		t.Fatal(err)
	}
	viewed, err := svc.InvitedProject(context.Background(), freelancer, project)
	if err != nil || viewed.ID != project {
		t.Fatalf("invited project=%#v err=%v", viewed, err)
	}
	if _, err = svc.InvitedProject(context.Background(), other, project); err != ErrNotFound {
		t.Fatalf("private invited project leaked: %v", err)
	}
	repeat, err := svc.Repeat(context.Background(), customer, project, RepeatInput{InvitePreviousFreelancer: true})
	if err != nil || repeat.Status != "DRAFT" || repeat.SourceType != "REPEAT" || repeat.Invite == nil {
		t.Fatalf("repeat=%#v err=%v", repeat, err)
	}
	draft := s.Projects[repeat.ProjectID]
	if draft.Status != "DRAFT" || draft.PreviousFreelancer != "" {
		t.Fatalf("unsafe repeat state: %#v", draft)
	}
	if _, err = svc.Share(context.Background(), customer, project); err != ErrNotFound {
		t.Fatalf("completed project shared: %v", err)
	}
	member, err := svc.PutTeam(context.Background(), customer, freelancer, TeamInput{Label: "Trusted", Notes: "Great"})
	if err != nil || member.Label != "Trusted" {
		t.Fatalf("team=%#v err=%v", member, err)
	}
	if _, err = svc.Team(context.Background(), other); err != ErrNotFound {
		t.Fatalf("non-customer team access=%v", err)
	}
	if err = svc.DeleteTeam(context.Background(), other, freelancer); err != nil {
		t.Fatalf("scoped delete should be idempotent in store: %v", err)
	}
	items, _ := svc.Team(context.Background(), customer)
	if len(items) != 1 {
		t.Fatal("cross-user delete removed team member")
	}
}
func TestGrowthHTTPUsesAuthenticatedActorAndStrictPayload(t *testing.T) {
	svc, _ := testService()
	h := Handler{Service: svc}
	w := httptest.NewRecorder()
	h.Invites(w, httptest.NewRequest(http.MethodPost, "/api/v1/me/invites", strings.NewReader(`{"type":"CUSTOMER"}`)))
	if w.Code != 401 {
		t.Fatalf("status=%d", w.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/me/invites", strings.NewReader(`{"type":"CUSTOMER","user_id":"forged"}`))
	req = req.WithContext(auth.WithActorID(req.Context(), freelancer))
	w = httptest.NewRecorder()
	h.Invites(w, req)
	if w.Code != 400 {
		t.Fatalf("strict status=%d", w.Code)
	}
}
func mapKeys[V any](m map[string]V) []string {
	v := []string{}
	for k := range m {
		v = append(v, k)
	}
	return v
}
