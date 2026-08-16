package proposals

import (
	"context"
	"errors"
	"freelance/apps/api/internal/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	customer   = "11111111-1111-4111-8111-111111111111"
	freelancer = "22222222-2222-4222-8222-222222222222"
	other      = "33333333-3333-4333-8333-333333333333"
	project    = "44444444-4444-4444-8444-444444444444"
)

func proposalStore() *Store {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	return &Store{Items: map[string]Proposal{}, Projects: map[string]Project{project: {ID: project, CustomerID: customer, Status: "OPEN"}}, Assignments: map[string]Assignment{}, FreelancerEligible: map[string]bool{freelancer: true, other: true}, Now: func() time.Time { return now }}
}
func proposalInput() Input {
	price := int64(10000)
	days := 10
	return Input{Message: "Готов выполнить проект", PriceKopecks: &price, Currency: "RUB", DeliveryDays: &days}
}
func TestProposalCriticalFlowOwnershipAndIdempotency(t *testing.T) {
	s := proposalStore()
	if _, err := s.Submit(context.Background(), customer, project, proposalInput()); !errors.Is(err, ErrIneligible) {
		t.Fatalf("self=%v", err)
	}
	first, err := s.Submit(context.Background(), freelancer, project, proposalInput())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Submit(context.Background(), freelancer, project, proposalInput()); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate=%v", err)
	}
	second, err := s.Submit(context.Background(), other, project, proposalInput())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.GetMine(context.Background(), other, first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross=%v", err)
	}
	if _, err = s.Act(context.Background(), other, project, first.ID, "accept"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("customer ownership=%v", err)
	}
	accepted, err := s.Act(context.Background(), customer, project, first.ID, "accept")
	if err != nil || accepted.Status != "ACCEPTED" || s.Projects[project].Status != "AWAITING_FUNDING" || len(s.Assignments) != 1 || s.Items[second.ID].Status != "REJECTED" {
		t.Fatalf("accepted=%#v err=%v", accepted, err)
	}
	if _, err = s.Act(context.Background(), customer, project, first.ID, "accept"); err != nil || len(s.Assignments) != 1 {
		t.Fatalf("idempotent=%v assignments=%d", err, len(s.Assignments))
	}
}
func TestProposalValidationWithdrawAndHandlerSecurity(t *testing.T) {
	s := proposalStore()
	bad := proposalInput()
	bad.Message = ""
	if _, err := s.Submit(context.Background(), freelancer, project, bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("validation=%v", err)
	}
	v, err := s.Submit(context.Background(), freelancer, project, proposalInput())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Withdraw(context.Background(), freelancer, v.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Withdraw(context.Background(), freelancer, v.ID); err != nil {
		t.Fatal(err)
	}
	h := Handler{Repository: s}
	res := httptest.NewRecorder()
	h.PublicProject(res, httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project+"/proposals", strings.NewReader(`{}`)))
	if res.Code != 401 {
		t.Fatalf("auth=%d", res.Code)
	}
	res = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project+"/proposals", strings.NewReader(`{"user_id":"`+customer+`"}`))
	req = req.WithContext(auth.WithActorID(req.Context(), other))
	h.PublicProject(res, req)
	if res.Code != 400 {
		t.Fatalf("identity=%d", res.Code)
	}
}
