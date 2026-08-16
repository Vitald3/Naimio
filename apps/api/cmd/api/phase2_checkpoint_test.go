package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"freelance/apps/api/internal/auth"
	"freelance/apps/api/internal/portfolio"
	"freelance/apps/api/internal/profiles"
	"freelance/apps/api/internal/projects"
	"freelance/apps/api/internal/proposals"
)

func TestCriticalPhase2ProjectProposalAssignmentFlow(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	customer, freelancer := "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	category, skill := "33333333-3333-4333-8333-333333333333", "44444444-4444-4444-8444-444444444444"
	profileStore := &profiles.Store{Items: map[string]profiles.Profile{freelancer: {UserID: freelancer, Username: "freelancer", DisplayName: "Исполнитель", Availability: "AVAILABLE", ProfileVisibility: "PUBLIC"}}}
	if _, err := profileStore.Update(ctx, freelancer, profiles.UpdateRequest{ProfessionalTitle: "Go разработчик", Availability: "AVAILABLE", ProfileVisibility: "PUBLIC"}); err != nil {
		t.Fatal(err)
	}
	portfolioMedia := "55555555-5555-4555-8555-555555555555"
	portfolioStore := &portfolio.Store{Items: map[string]portfolio.Item{}, Media: map[string]portfolio.MediaObject{portfolioMedia: {ID: portfolioMedia, OwnerID: freelancer, MIMEType: "image/png", SizeBytes: 100, Purpose: "PORTFOLIO", ScanStatus: "CLEAN", Uploaded: true}}, Now: func() time.Time { return now }}
	work, err := portfolioStore.Create(ctx, freelancer, portfolio.WriteRequest{Title: "API portfolio", Slug: "api-portfolio", Description: "Пример API", Visibility: "PUBLIC", MediaObjectIDs: []string{portfolioMedia}})
	if err != nil || len(work.Media) != 1 {
		t.Fatalf("portfolio=%#v err=%v", work, err)
	}
	projectStore := &projects.Store{Items: map[string]projects.Item{}, Categories: map[string]projects.Reference{category: {ID: category, Slug: "development", Name: "Разработка"}}, Skills: map[string]projects.Reference{skill: {ID: skill, Slug: "go", Name: "Go"}}, CustomerEligible: map[string]bool{customer: true}, Now: func() time.Time { return now }}
	amount := int64(100000)
	created, err := projectStore.Create(ctx, customer, projects.CreateRequest{CategoryID: category, Title: "API", Slug: "critical-api", Description: "Разработать API", Budget: projects.Budget{Type: "FIXED", MinKopecks: &amount, Currency: "RUB"}, Visibility: "PUBLIC", SkillIDs: []string{skill}})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := projectStore.Transition(ctx, customer, created.ID, "publish")
	if err != nil {
		t.Fatal(err)
	}
	proposalStore := &proposals.Store{Items: map[string]proposals.Proposal{}, Projects: map[string]proposals.Project{opened.ID: {ID: opened.ID, CustomerID: customer, Status: opened.Status}}, Assignments: map[string]proposals.Assignment{}, FreelancerEligible: map[string]bool{freelancer: true}, Now: func() time.Time { return now }}
	handler := proposals.Handler{Repository: proposalStore}
	price := int64(90000)
	days := 7
	body, _ := json.Marshal(proposals.Input{Message: "Готов выполнить", PriceKopecks: &price, Currency: "RUB", DeliveryDays: &days})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+opened.ID+"/proposals", strings.NewReader(string(body)))
	req = req.WithContext(auth.WithActorID(req.Context(), freelancer))
	res := httptest.NewRecorder()
	handler.PublicProject(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("submit=%d body=%s", res.Code, res.Body.String())
	}
	var response struct {
		Data proposals.Proposal `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/me/projects/"+opened.ID+"/proposals/"+response.Data.ID+"/accept", nil)
	req.Header.Set("Idempotency-Key", "critical-flow")
	req = req.WithContext(auth.WithActorID(req.Context(), customer))
	res = httptest.NewRecorder()
	handler.CustomerProject(res, req)
	if res.Code != http.StatusOK || proposalStore.Projects[opened.ID].Status != "AWAITING_FUNDING" || len(proposalStore.Assignments) != 1 {
		t.Fatalf("accept=%d project=%#v assignments=%d", res.Code, proposalStore.Projects[opened.ID], len(proposalStore.Assignments))
	}
}

func TestPhase2PublicReadLoadSmoke(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health/live", health)
	profileHandler := profiles.Handler{Repository: &profiles.Store{Items: map[string]profiles.Profile{}}}
	mux.HandleFunc("/api/v1/freelancers", profileHandler.List)
	projectStore := &projects.Store{Items: map[string]projects.Item{}, Categories: map[string]projects.Reference{}, Skills: map[string]projects.Reference{}, Media: map[string]projects.MediaObject{}}
	projectHandler := projects.Handler{Repository: projectStore, Search: projectStore}
	mux.HandleFunc("/api/v1/projects", projectHandler.PublicCollection)
	handler := requestID(mux)
	const requests = 100
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			res := httptest.NewRecorder()
			paths := []string{"/health/live", "/api/v1/freelancers?limit=20", "/api/v1/projects?limit=20"}
			handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, paths[index%len(paths)], nil))
			if res.Code != http.StatusOK {
				t.Errorf("status=%d", res.Code)
			}
		}(i)
	}
	wg.Wait()
}
