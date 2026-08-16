package acquisition

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"freelance/apps/api/internal/ai"
)

const anonymous = "91919191-9191-4919-8919-919191919191"

func fixture() (*Store, Service) {
	repo := &Store{Items: DefaultDefinitions(), Taxonomy: map[string]Taxonomy{}, SitemapItems: []SitemapItem{{Path: "/freelancers/ivan"}}}
	drafts := &ai.MemoryRepository{Drafts: map[string]ai.Draft{}, TokenHashes: map[[32]byte]string{}}
	return repo, Service{Repository: repo, Drafts: ai.Service{Drafts: drafts}}
}
func TestDeterministicCalculatorCreatesPreservedDraftWithoutAI(t *testing.T) {
	repo, s := fixture()
	result, e := s.Estimate(context.Background(), "", "landing-page", map[string]any{"design": "custom", "sections": float64(8), "copywriting": true}, Attribution{AnonymousID: anonymous, LandingPath: "/price/landing-page", UTMSource: "search"})
	if e != nil {
		t.Fatal(e)
	}
	if result.EstimatedMinKopecks != 4525000 || result.EstimatedMaxKopecks != 10860000 || len(result.DraftToken) != 64 {
		t.Fatalf("result=%+v", result)
	}
	if len(repo.Events) != 1 || repo.Events[0].Type != "CALCULATOR_COMPLETED" {
		t.Fatalf("events=%+v", repo.Events)
	}
	draft, e := s.Drafts.GetDraft(context.Background(), "", result.DraftToken)
	if e != nil {
		t.Fatal(e)
	}
	if draft.SourceType != "CALCULATOR" || draft.NormalizedData["calculator_slug"] != "landing-page" {
		t.Fatalf("draft=%+v", draft)
	}
}
func TestCalculatorRejectsSchemaExpansionAndUnsafeAttribution(t *testing.T) {
	_, s := fixture()
	valid := map[string]any{"design": "custom", "sections": float64(8), "copywriting": true}
	for name, test := range map[string]struct {
		answers     map[string]any
		attribution Attribution
	}{"missing": {map[string]any{"design": "custom"}, Attribution{AnonymousID: anonymous}}, "unknown": {map[string]any{"design": "custom", "sections": float64(8), "copywriting": true, "extra": true}, Attribution{AnonymousID: anonymous}}, "range": {map[string]any{"design": "custom", "sections": float64(1000), "copywriting": true}, Attribution{AnonymousID: anonymous}}, "anonymous required": {valid, Attribution{}}, "unsafe path": {valid, Attribution{AnonymousID: anonymous, LandingPath: "//evil.test"}}} {
		t.Run(name, func(t *testing.T) {
			if _, e := s.Estimate(context.Background(), "", "landing-page", test.answers, test.attribution); e != ErrInvalid {
				t.Fatalf("error=%v", e)
			}
		})
	}
}
func TestEventsAreAllowlistedAndContentFree(t *testing.T) {
	repo, s := fixture()
	if e := s.Record(context.Background(), "", Event{Type: "CALCULATOR_STARTED", Attribution: Attribution{AnonymousID: anonymous, LandingPath: "/price/seo"}, Metadata: map[string]string{"calculator_slug": "seo"}}); e != nil {
		t.Fatal(e)
	}
	if e := s.Record(context.Background(), "", Event{Type: "EVERY_CLICK", Attribution: Attribution{AnonymousID: anonymous}}); e != ErrInvalid {
		t.Fatalf("event=%v", e)
	}
	if e := s.Record(context.Background(), "", Event{Type: "LANDING_VIEW", Attribution: Attribution{AnonymousID: anonymous}, Metadata: map[string]string{"prompt": "private text"}}); e != ErrInvalid {
		t.Fatalf("metadata=%v", e)
	}
	if len(repo.Events) != 1 {
		t.Fatalf("events=%d", len(repo.Events))
	}
}
func TestSitemapIsBoundedToRealDefinitions(t *testing.T) {
	repo, _ := fixture()
	items, e := repo.Sitemap(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if len(items) != 4 {
		t.Fatalf("items=%+v", items)
	}
	for _, v := range items {
		if strings.Contains(v.Path, "?") {
			t.Fatalf("query path=%s", v.Path)
		}
	}
}
func TestHTTPRejectsForgedUserAndUnknownFields(t *testing.T) {
	_, s := fixture()
	h := Handler{Service: s}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/acquisition/events", strings.NewReader(`{"event_type":"LANDING_VIEW","anonymous_id":"`+anonymous+`","user_id":"91919191-9191-4919-8919-919191919192"}`))
	w := httptest.NewRecorder()
	h.Event(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestAdminCanCreateUnlimitedCalculatorSectionsAndVersionEdits(t *testing.T) {
	repo, service := fixture()
	template := DefaultDefinitions()["seo"]
	input := AdminDefinitionInput{
		Slug: "mobile-app", Title: "Мобильное приложение", Intro: "Предварительная оценка разработки приложения.", Enabled: true,
		Questions: template.Questions, Pricing: template.Pricing, Reason: "Добавлен новый раздел",
	}
	created, err := service.CreateAdminDefinition(context.Background(), anonymous, input)
	if err != nil || created.Slug != input.Slug || created.Version != 1 {
		t.Fatalf("created=%+v error=%v", created, err)
	}
	input.Title = "Мобильное приложение под ключ"
	updated, err := service.UpdateAdminDefinition(context.Background(), anonymous, input.Slug, input)
	if err != nil || updated.Version != 2 || updated.Title != input.Title {
		t.Fatalf("updated=%+v error=%v", updated, err)
	}
	if _, err = repo.Definition(context.Background(), input.Slug); err != nil {
		t.Fatalf("public definition missing: %v", err)
	}
}
