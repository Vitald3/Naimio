package matching

import (
	"context"
	"errors"
	"freelance/apps/api/internal/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const projectID = "018f57a4-79d2-7c35-8548-77b4c96f1d11"
const ownerID = "018f57a4-79d2-7c35-8548-77b4c96f1d12"
const establishedID = "018f57a4-79d2-7c35-8548-77b4c96f1d13"
const newID = "018f57a4-79d2-7c35-8548-77b4c96f1d14"
const mismatchID = "018f57a4-79d2-7c35-8548-77b4c96f1d15"
const adminID = "018f57a4-79d2-7c35-8548-77b4c96f1d16"

func pointer[T any](v T) *T { return &v }
func testStore() *Store {
	return &Store{Projects: map[string]Project{projectID: {ID: projectID, CustomerID: ownerID, Title: "Telegram bot", Description: "Go bot integration", CategoryID: "cat", SkillIDs: []string{"go", "api"}, BudgetMax: pointer[int64](10_000_000), Status: "OPEN"}}, Candidates: map[string][]Candidate{projectID: {{UserID: establishedID, Username: "expert", DisplayName: "Expert", ProfessionalTitle: "Go developer", Availability: "AVAILABLE", PrimaryCategoryID: "cat", MinimumOrder: pointer[int64](8_000_000), ProfileCompletion: 100, SkillMatches: 2, FeaturedSkillMatches: 1, RequiredSkills: 2, RelevantPortfolio: 2, SimilarCompleted: 2, Reviews: 10, VerifiedExternal: 1, NativeRating: pointer(4.9), CompletionRate: pointer(98.0), RecommendationRate: pointer(95.0)}, {UserID: newID, Username: "new", DisplayName: "New", ProfessionalTitle: "New Go developer", Availability: "AVAILABLE", PrimaryCategoryID: "cat", MinimumOrder: pointer[int64](5_000_000), ProfileCompletion: 90, SkillMatches: 2, RequiredSkills: 2, RelevantPortfolio: 2, Reviews: 0}, {UserID: mismatchID, Username: "other", DisplayName: "Other", Availability: "UNAVAILABLE", PrimaryCategoryID: "other", MinimumOrder: pointer[int64](20_000_000), ProfileCompletion: 100}}}, Runs: map[string]Run{}, LatestRun: map[string]string{}, Manual: map[string]map[string]bool{}, Admins: map[string]bool{adminID: true}, Events: map[string]bool{}}
}
func TestDeterministicSignalsAndColdStart(t *testing.T) {
	store := testStore()
	service := Service{Repository: store, Weights: DefaultWeights(), RetrievalLimit: 50, ShortlistLimit: 2}
	run, err := service.Run(context.Background(), ownerID, projectID, Constraints{RequireImmediateAvailability: true, RequireCategoryMatch: true, MaxMinimumOrderKopecks: pointer[int64](10_000_000)})
	if err != nil {
		t.Fatal(err)
	}
	if run.AIUsed || run.CandidateCount != 2 || len(run.Recommendations) != 2 {
		t.Fatalf("run=%#v", run)
	}
	if run.Recommendations[0].Score < run.Recommendations[1].Score || run.Recommendations[0].Score > 10000 {
		t.Fatalf("scores=%#v", run.Recommendations)
	}
	foundCold := false
	for _, item := range run.Recommendations {
		for _, reason := range item.Reasons {
			if reason.Code == "COLD_START_POTENTIAL" {
				foundCold = true
			}
		}
	}
	if !foundCold {
		t.Fatalf("cold start missing: %#v", run.Recommendations)
	}
	if _, err := service.Run(context.Background(), "other", projectID, Constraints{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("BOLA=%v", err)
	}
}

type fakeReranker struct {
	result RerankResult
	err    error
	seen   int
}

func (f *fakeReranker) Rerank(_ context.Context, _ string, req RerankRequest) (RerankResult, error) {
	f.seen = len(req.Candidates)
	return f.result, f.err
}
func TestRerankSmallValidatedShortlistAndFallback(t *testing.T) {
	store := testStore()
	invalid := &fakeReranker{result: RerankResult{OrderedIDs: []string{"forged"}}}
	run, err := (&Service{Repository: store, Reranker: invalid, Weights: DefaultWeights(), RetrievalLimit: 100, ShortlistLimit: 2}).Run(context.Background(), ownerID, projectID, Constraints{})
	if err != nil || run.AIUsed || invalid.seen != 2 {
		t.Fatalf("fallback=%#v seen=%d err=%v", run, invalid.seen, err)
	}
	valid := &fakeReranker{result: RerankResult{OrderedIDs: []string{newID, establishedID}, ReasonCodes: map[string][]string{newID: {"STRONG_SCOPE_FIT"}}}}
	run, err = (&Service{Repository: store, Reranker: valid, Weights: DefaultWeights(), RetrievalLimit: 100, ShortlistLimit: 2}).Run(context.Background(), ownerID, projectID, Constraints{})
	if err != nil || !run.AIUsed || run.Recommendations[0].FreelancerID != newID {
		t.Fatalf("rerank=%#v err=%v", run, err)
	}
}
func TestManualRecommendationEventsAndMetrics(t *testing.T) {
	store := testStore()
	service := Service{Repository: store, Weights: DefaultWeights(), RetrievalLimit: 50, ShortlistLimit: 20}
	run, err := service.Run(context.Background(), ownerID, projectID, Constraints{})
	if err != nil {
		t.Fatal(err)
	}
	if err = service.PutManual(context.Background(), "not-admin", projectID, newID, "good fit"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("admin auth=%v", err)
	}
	if err = service.PutManual(context.Background(), adminID, projectID, newID, "Concierge reviewed category fit"); err != nil {
		t.Fatal(err)
	}
	latest, err := service.Latest(context.Background(), ownerID, projectID)
	if err != nil || !latest.Recommendations[0].Manual {
		t.Fatalf("manual=%#v err=%v", latest, err)
	}
	if err = service.Event(context.Background(), ownerID, projectID, run.ID, establishedID, "IMPRESSION", "event-key-01"); err != nil {
		t.Fatal(err)
	}
	if err = service.Event(context.Background(), ownerID, projectID, run.ID, "forged", "INVITE", "event-key-02"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("forged event=%v", err)
	}
	metrics, err := service.Metrics(context.Background(), adminID)
	if err != nil || metrics.Runs != 1 {
		t.Fatalf("metrics=%#v err=%v", metrics, err)
	}
}
func TestHTTPUsesTrustedActorAndStrictPayload(t *testing.T) {
	store := testStore()
	handler := Handler{Service: Service{Repository: store, Weights: DefaultWeights(), RetrievalLimit: 50, ShortlistLimit: 20}}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/me/projects/"+projectID+"/matching-runs", strings.NewReader(`{"user_id":"`+ownerID+`"}`))
	request = request.WithContext(auth.WithActorID(request.Context(), ownerID))
	response := httptest.NewRecorder()
	handler.Customer(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("forged identity status=%d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/api/v1/me/projects/"+projectID+"/matching-runs", strings.NewReader(`{"require_immediate_availability":true}`))
	request = request.WithContext(auth.WithActorID(request.Context(), ownerID))
	response = httptest.NewRecorder()
	handler.Customer(response, request)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), "algorithm_version") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
func TestConfigBoundsAndWeights(t *testing.T) {
	config, err := LoadConfig(func(key string) string {
		if key == "MATCHING_RETRIEVAL_LIMIT" {
			return "150"
		}
		if key == "MATCHING_SHORTLIST_LIMIT" {
			return "10"
		}
		return ""
	})
	if err != nil || config.RetrievalLimit != 150 || config.ShortlistLimit != 10 {
		t.Fatalf("config=%#v err=%v", config, err)
	}
	if _, err := LoadConfig(func(key string) string {
		if key == "MATCHING_RETRIEVAL_LIMIT" {
			return "10000"
		}
		return ""
	}); err == nil {
		t.Fatal("expected invalid limit")
	}
	if _, err := LoadConfig(func(key string) string {
		if key == "MATCHING_WEIGHTS_JSON" {
			return `{"Skills":1}`
		}
		return ""
	}); err == nil {
		t.Fatal("expected invalid weights")
	}
}
