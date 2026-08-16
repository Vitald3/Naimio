package reviews

import (
	"context"
	"encoding/json"
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
	outsider   = "33333333-3333-4333-8333-333333333333"
	project    = "44444444-4444-4444-8444-444444444444"
)

func reviewStore() *Store {
	return &Store{Items: map[string]Item{}, Projects: map[string]Relationship{project: {Customer: customer, Freelancer: freelancer, Status: "COMPLETED"}}, Usernames: map[string]string{"freelancer": freelancer}, Names: map[string]string{customer: "Елена Заказчик", freelancer: "Максим Исполнитель"}, ProjectTitles: map[string]string{project: "Telegram-бот для продаж"}, Stats: map[string]TrustStats{}, Now: func() time.Time { return time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC) }}
}
func TestReviewEligibilityDimensionsSelfAndDuplicate(t *testing.T) {
	s := reviewStore()
	service := Service{Repository: s}
	yes := true
	in := Input{RatingOverall: 5, WouldWorkAgain: &yes, Text: "Отличная работа", Dimensions: map[string]int{"quality": 5, "deadline": 5}}
	v, err := service.Create(context.Background(), customer, project, in)
	if err != nil || v.ReviewerRole != "CUSTOMER" || v.RevieweeID != freelancer {
		t.Fatalf("review=%#v err=%v", v, err)
	}
	if _, err = service.Create(context.Background(), customer, project, in); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate=%v", err)
	}
	if _, err = service.Create(context.Background(), outsider, project, in); !errors.Is(err, ErrNotFound) {
		t.Fatalf("outsider=%v", err)
	}
	bad := in
	bad.Dimensions = map[string]int{"BRIEF_QUALITY": 5}
	s2 := reviewStore()
	if _, err = (Service{Repository: s2}).Create(context.Background(), customer, project, bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("dimension=%v", err)
	}
	s2.Projects[project] = Relationship{Customer: customer, Freelancer: customer, Status: "COMPLETED"}
	if _, err = (Service{Repository: s2}).Create(context.Background(), customer, project, in); !errors.Is(err, ErrIneligible) {
		t.Fatalf("self=%v", err)
	}
}
func TestTrustMetricsAndPublicPagination(t *testing.T) {
	s := reviewStore()
	service := Service{Repository: s}
	yes, no := true, false
	for i, answer := range []*bool{&yes, &yes, &no} {
		p := project[:35] + string(rune('4'+i))
		s.Projects[p] = Relationship{Customer: customer, Freelancer: freelancer, Status: "COMPLETED"}
		if _, err := service.Create(context.Background(), customer, p, Input{RatingOverall: 4 + i%2, WouldWorkAgain: answer, Dimensions: map[string]int{"QUALITY": 5}}); err != nil {
			t.Fatal(err)
		}
	}
	data, err := service.Public(context.Background(), "freelancer", nil, 2)
	if err != nil || len(data.Reviews.Items) != 2 || data.Reviews.NextCursor == nil || data.Trust.ReviewsCount != 3 || data.Trust.CompletedProjectsCount != 3 || data.Trust.RecommendationRate == nil {
		t.Fatalf("data=%#v err=%v", data, err)
	}
	if *data.Trust.RecommendationRate < 66 || *data.Trust.RecommendationRate > 67 {
		t.Fatalf("recommendation=%v", *data.Trust.RecommendationRate)
	}
}
func TestReviewHandlerSecurityAndNoIdentityInput(t *testing.T) {
	s := reviewStore()
	h := Handler{Service: Service{Repository: s}}
	res := httptest.NewRecorder()
	h.Project(res, httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project+"/reviews", strings.NewReader(`{}`)))
	if res.Code != 401 {
		t.Fatalf("unauth=%d", res.Code)
	}
	res = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project+"/reviews", strings.NewReader(`{"reviewee_user_id":"`+outsider+`","rating_overall":5,"dimensions":{"QUALITY":5}}`))
	req = req.WithContext(auth.WithActorID(req.Context(), customer))
	h.Project(res, req)
	if res.Code != 400 || len(s.Items) != 0 {
		t.Fatalf("identity=%d body=%s", res.Code, res.Body.String())
	}
}
func TestPublicReviewJSONDoesNotExposeUserIDs(t *testing.T) {
	s := reviewStore()
	_, _ = (Service{Repository: s}).Create(context.Background(), customer, project, Input{RatingOverall: 5, Dimensions: map[string]int{"QUALITY": 5}})
	data, _ := (Service{Repository: s}).Public(context.Background(), "freelancer", nil, 20)
	b, _ := json.Marshal(data)
	if strings.Contains(string(b), "reviewer_id") || strings.Contains(string(b), "reviewee_id") {
		t.Fatalf("leak=%s", b)
	}
}
func TestPublicReviewSurfacesReviewerNameAndProjectTitle(t *testing.T) {
	s := reviewStore()
	_, _ = (Service{Repository: s}).Create(context.Background(), customer, project, Input{RatingOverall: 5, Dimensions: map[string]int{"QUALITY": 5}})
	data, err := (Service{Repository: s}).Public(context.Background(), "freelancer", nil, 20)
	if err != nil || len(data.Reviews.Items) != 1 {
		t.Fatalf("data=%#v err=%v", data, err)
	}
	got := data.Reviews.Items[0]
	if got.ReviewerName != "Елена Заказчик" || got.ProjectTitle != "Telegram-бот для продаж" {
		t.Fatalf("identity/project not surfaced: %#v", got)
	}
	b, _ := json.Marshal(got)
	if !strings.Contains(string(b), "reviewer_name") || !strings.Contains(string(b), "project_title") {
		t.Fatalf("payload missing fields: %s", b)
	}
}

func TestReviewReportAndModerationRecalculateTrust(t *testing.T) {
	admin := "55555555-5555-4555-8555-555555555555"
	s := reviewStore()
	s.Admins = map[string]bool{admin: true}
	s.Reports = map[string]bool{}
	service := Service{Repository: s}
	v, err := service.Create(context.Background(), customer, project, Input{RatingOverall: 5, Dimensions: map[string]int{"QUALITY": 5}})
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Report(context.Background(), outsider, v.ID, ReportInput{ReasonCode: "SPAM", Description: "duplicate content"}); err != nil {
		t.Fatal(err)
	}
	if err = service.Report(context.Background(), outsider, v.ID, ReportInput{ReasonCode: "SPAM"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate report=%v", err)
	}
	if _, err = service.Moderate(context.Background(), outsider, v.ID, "hide", "spam"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("permission=%v", err)
	}
	hidden, err := service.Moderate(context.Background(), admin, v.ID, "hide", "spam")
	if err != nil || hidden.Status != "HIDDEN" || s.Stats[freelancer].ReviewsCount != 0 || len(s.Audits) != 1 {
		t.Fatalf("hidden=%#v stats=%#v audits=%#v err=%v", hidden, s.Stats[freelancer], s.Audits, err)
	}
	restored, err := service.Moderate(context.Background(), admin, v.ID, "restore", "appeal accepted")
	if err != nil || restored.RatingOverall != 5 || restored.Text != v.Text || s.Stats[freelancer].ReviewsCount != 1 {
		t.Fatalf("restored=%#v err=%v", restored, err)
	}
}

func TestReviewAutomaticModerationReturnsSpecificError(t *testing.T) {
	s := reviewStore()
	_, err := (Service{Repository: s}).Create(context.Background(), customer, project, Input{
		RatingOverall: 5,
		Text:          "asdfghasdfghasdfgh",
		Dimensions:    map[string]int{"QUALITY": 5},
	})
	if !errors.Is(err, ErrModeration) {
		t.Fatalf("moderation error=%v", err)
	}
}
