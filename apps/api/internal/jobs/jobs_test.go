package jobs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"freelance/apps/api/internal/auth"
)

const owner = "018f1f77-7b70-7000-8000-000000000001"
const other = "018f1f77-7b70-7000-8000-000000000002"
const applicant = "018f1f77-7b70-7000-8000-000000000003"
const admin = "018f1f77-7b70-7000-8000-000000000004"
const category = "018f1f77-7b70-7000-8000-000000000005"
const skill = "018f1f77-7b70-7000-8000-000000000006"

func fixture() *Store {
	return &Store{Companies: map[string]Company{}, Items: map[string]Item{}, Applications: map[string]Application{}, Categories: map[string]Reference{category: {ID: category, Slug: "backend", Name: "Backend"}}, Skills: map[string]Reference{skill: {ID: skill, Slug: "go", Name: "Go"}}, Customers: map[string]bool{owner: true, other: true}, Applicants: map[string]bool{applicant: true}, Admins: map[string]bool{admin: true}, Now: func() time.Time { return time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC) }}
}
func createJob(t *testing.T, s *Store) Item {
	t.Helper()
	c, e := s.CreateCompany(context.Background(), owner, CompanyInput{Name: "Acme", Slug: "acme", Website: "https://example.com"})
	if e != nil {
		t.Fatal(e)
	}
	min, max := int64(10000000), int64(20000000)
	v, e := s.Create(context.Background(), owner, CreateRequest{CompanyID: c.ID, CategoryID: category, Title: "Go developer", Slug: "go-developer", Description: "Build reliable services", EmploymentType: "FULL_TIME", SalaryMinKopecks: &min, SalaryMaxKopecks: &max, Currency: "RUB", Remote: true, ExperienceLevel: "MIDDLE", SkillIDs: []string{skill}})
	if e != nil {
		t.Fatal(e)
	}
	return v
}

func TestVacancyLifecycleSearchAndApplications(t *testing.T) {
	s := fixture()
	v := createJob(t, s)
	if _, e := s.GetOwned(context.Background(), other, v.ID); e != ErrNotFound {
		t.Fatalf("cross owner = %v", e)
	}
	v, e := s.Transition(context.Background(), owner, v.ID, "publish")
	if e != nil || v.Status != "PUBLISHED" {
		t.Fatalf("publish=%+v %v", v, e)
	}
	remote := true
	p, e := s.ListPublic(context.Background(), Filter{Q: "reliable", Skill: "go", Remote: &remote, MinSalary: pointer(int64(15000000))}, nil, 20)
	if e != nil || len(p.Items) != 1 {
		t.Fatalf("search=%+v %v", p, e)
	}
	a, e := s.Apply(context.Background(), applicant, v.ID, "I have relevant experience")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Apply(context.Background(), applicant, v.ID, "again"); e != ErrConflict {
		t.Fatalf("duplicate=%v", e)
	}
	if _, e = s.ListApplicants(context.Background(), other, v.ID); e != ErrNotFound {
		t.Fatalf("private applicants=%v", e)
	}
	apps, e := s.ListApplicants(context.Background(), owner, v.ID)
	if e != nil || len(apps) != 1 || apps[0].CoverMessage == "" {
		t.Fatalf("applicants=%+v %v", apps, e)
	}
	a, e = s.SetApplicationStatus(context.Background(), owner, v.ID, a.ID, "SHORTLISTED")
	if e != nil || a.Status != "SHORTLISTED" {
		t.Fatalf("shortlist=%+v %v", a, e)
	}
	mine, e := s.ListMine(context.Background(), applicant, nil, 20)
	if e != nil || len(mine) != 1 || mine[0].CoverMessage != "" || mine[0].UserID != "" {
		t.Fatalf("mine=%+v %v", mine, e)
	}
}

func TestValidationModerationAndColdPrivacy(t *testing.T) {
	s := fixture()
	v := createJob(t, s)
	if _, e := s.Transition(context.Background(), owner, v.ID, "publish"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Apply(context.Background(), owner, v.ID, "self"); e != ErrIneligible {
		t.Fatalf("self apply=%v", e)
	}
	if _, e := s.Moderate(context.Background(), other, v.ID, "HIDE", "prohibited contact spam"); e != ErrForbidden {
		t.Fatalf("admin=%v", e)
	}
	if _, e := s.Moderate(context.Background(), admin, v.ID, "HIDE", "prohibited contact spam"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.GetPublic(context.Background(), v.ID); e != ErrNotFound {
		t.Fatalf("hidden public=%v", e)
	}
	if _, e := s.Moderate(context.Background(), admin, v.ID, "RESTORE", "reviewed and restored"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.GetPublic(context.Background(), v.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := s.CreateCompany(context.Background(), owner, CompanyInput{Name: "Unsafe", Slug: "unsafe", Website: "http://127.0.0.1"}); e != ErrInvalidInput {
		t.Fatalf("website=%v", e)
	}
}

func TestHTTPUsesTrustedActorAndStrictPayload(t *testing.T) {
	s := fixture()
	h := Handler{Repository: s}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/me/vacancies", strings.NewReader(`{"customer_id":"`+other+`"}`))
	request = request.WithContext(auth.WithActorID(request.Context(), owner))
	response := httptest.NewRecorder()
	h.OwnerCollection(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("forged field status=%d body=%s", response.Code, response.Body.String())
	}
	v := createJob(t, s)
	_, _ = s.Transition(context.Background(), owner, v.ID, "publish")
	request = httptest.NewRequest(http.MethodGet, "/api/v1/me/vacancies/"+v.ID+"/applications", nil)
	request = request.WithContext(auth.WithActorID(request.Context(), other))
	response = httptest.NewRecorder()
	h.OwnerItem(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("BOLA status=%d body=%s", response.Code, response.Body.String())
	}
}

func pointer[T any](v T) *T { return &v }

func TestCanonicalUUIDValidationAcceptsDeterministicSeedIDs(t *testing.T) {
	if !uuidPattern.MatchString("f9730c2b-7384-b947-e45d-7e6997df8b4a") {
		t.Fatal("deterministic vacancy seed UUID must be accepted")
	}
}
