package projects

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"freelance/apps/api/internal/auth"
)

const (
	customerOne = "11111111-1111-4111-8111-111111111111"
	customerTwo = "22222222-2222-4222-8222-222222222222"
	projectOne  = "33333333-3333-4333-8333-333333333333"
	categoryID  = "55555555-5555-4555-8555-555555555555"
	skillID     = "66666666-6666-4666-8666-666666666666"
	mediaID     = "77777777-7777-4777-8777-777777777777"
)

func projectStore() *Store {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	return &Store{Items: map[string]Item{}, Categories: map[string]Reference{categoryID: {ID: categoryID, Slug: "development", Name: "Разработка"}}, Skills: map[string]Reference{skillID: {ID: skillID, Slug: "go", Name: "Go"}}, Media: map[string]MediaObject{mediaID: {ID: mediaID, OwnerID: customerOne, Purpose: "PROJECT", MIMEType: "image/png", SizeBytes: 100, Uploaded: true, ScanStatus: "CLEAN"}}, CustomerEligible: map[string]bool{customerOne: true, customerTwo: true}, Now: func() time.Time { return now }}
}
func validCreate() CreateRequest {
	amount := int64(1500000)
	return CreateRequest{CategoryID: categoryID, Title: "Разработать API", Slug: "api-development", Description: "Нужен надёжный API.", Budget: Budget{Type: "FIXED", MinKopecks: &amount, Currency: "RUB"}, ExperienceLevel: "ADVANCED", Visibility: "PUBLIC", SkillIDs: []string{skillID}, MediaObjectIDs: []string{mediaID}}
}

func TestProjectCRUDOwnershipAndReferences(t *testing.T) {
	store := projectStore()
	created, err := store.Create(context.Background(), customerOne, validCreate())
	if err != nil || created.Status != "DRAFT" || len(created.Skills) != 1 || len(created.Media) != 1 {
		t.Fatalf("created=%#v err=%v", created, err)
	}
	if _, err := store.Create(context.Background(), customerOne, validCreate()); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate err=%v", err)
	}
	if _, err := store.GetOwned(context.Background(), customerTwo, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user err=%v", err)
	}
	title := "Обновлённый проект"
	updated, err := store.Update(context.Background(), customerOne, created.ID, PatchRequest{Title: &title})
	if err != nil || updated.Title != title {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
	object := store.Media[mediaID]
	object.ScanStatus = "PENDING"
	store.Media[mediaID] = object
	other := validCreate()
	other.Slug = "unsafe-media"
	if _, err := store.Create(context.Background(), customerOne, other); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("unsafe media err=%v", err)
	}
	object.ScanStatus, object.OwnerID = "CLEAN", customerTwo
	store.Media[mediaID] = object
	other.Slug = "wrong-owner"
	if _, err := store.Create(context.Background(), customerOne, other); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("wrong owner err=%v", err)
	}
}

func TestProjectValidation(t *testing.T) {
	for name, mutate := range map[string]func(*CreateRequest){
		"fixed missing": func(in *CreateRequest) { in.Budget.MinKopecks = nil },
		"range order": func(in *CreateRequest) {
			min, max := int64(20), int64(10)
			in.Budget = Budget{Type: "RANGE", MinKopecks: &min, MaxKopecks: &max, Currency: "RUB"}
		},
		"future source cannot enter": func(in *CreateRequest) { in.Budget.Currency = "USD" },
		"duplicate skill":            func(in *CreateRequest) { in.SkillIDs = []string{skillID, skillID} },
	} {
		t.Run(name, func(t *testing.T) {
			store := projectStore()
			in := validCreate()
			mutate(&in)
			if _, err := store.Create(context.Background(), customerOne, in); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestProjectTransitionsPublicPrivacyPaginationAndOutbox(t *testing.T) {
	store := projectStore()
	first, err := store.Create(context.Background(), customerOne, validCreate())
	if err != nil {
		t.Fatal(err)
	}
	opened, err := store.Transition(context.Background(), customerOne, first.ID, "publish")
	if err != nil || opened.Status != "OPEN" || opened.PublishedAt == nil || len(store.Events) != 1 {
		t.Fatalf("opened=%#v events=%#v err=%v", opened, store.Events, err)
	}
	if _, err := store.Update(context.Background(), customerOne, first.ID, PatchRequest{}); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("update err=%v", err)
	}
	privateInput := validCreate()
	privateInput.Slug, privateInput.Visibility, privateInput.MediaObjectIDs = "private", "PRIVATE", nil
	private, err := store.Create(context.Background(), customerOne, privateInput)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Transition(context.Background(), customerOne, private.ID, "publish"); err != nil {
		t.Fatal(err)
	}
	page, err := store.ListPublic(context.Background(), Filter{BudgetType: "FIXED", ExperienceLevel: "ADVANCED"}, nil, 1)
	if err != nil || len(page.Items) != 1 || page.Items[0].CustomerID != "" || page.NextCursor != nil {
		t.Fatalf("page=%#v err=%v", page, err)
	}
	if _, err := store.GetPublic(context.Background(), private.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("private err=%v", err)
	}
	searched, err := store.ListPublic(context.Background(), Filter{Q: "надёжный"}, nil, 20)
	if err != nil || len(searched.Items) != 1 || searched.Items[0].ID != first.ID {
		t.Fatalf("searched=%#v err=%v", searched, err)
	}
	seed := opened
	seed.ID, seed.Status = projectOne, "IN_PROGRESS"
	store.Items[seed.ID] = seed
	store.DealCompleted = map[string]bool{seed.ID: true}
	completed, err := store.Transition(context.Background(), customerOne, seed.ID, "complete")
	if err != nil || completed.Status != "COMPLETED" {
		t.Fatalf("completed=%#v err=%v", completed, err)
	}
	before := len(store.Events)
	if _, err := store.Transition(context.Background(), customerOne, seed.ID, "complete"); err != nil || len(store.Events) != before {
		t.Fatalf("idempotent err=%v events=%d", err, len(store.Events))
	}
	if err := store.Delete(context.Background(), customerOne, seed.ID); err != nil {
		t.Fatal(err)
	}
}

func TestPublishedPrivateProjectCanBeMadePublic(t *testing.T) {
	store := projectStore()
	in := validCreate()
	in.Visibility = "PRIVATE"
	draft, err := store.Create(context.Background(), customerOne, in)
	if err != nil {
		t.Fatal(err)
	}
	published, err := store.Transition(context.Background(), customerOne, draft.ID, "publish")
	if err != nil || published.Status != "OPEN" || published.Visibility != "PRIVATE" {
		t.Fatalf("published=%#v err=%v", published, err)
	}
	if _, err := store.GetPublic(context.Background(), draft.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("private public lookup err=%v", err)
	}
	public, err := store.Transition(context.Background(), customerOne, draft.ID, "make-public")
	if err != nil || public.Visibility != "PUBLIC" || public.Status != "OPEN" {
		t.Fatalf("public=%#v err=%v", public, err)
	}
	if _, err := store.GetPublic(context.Background(), draft.ID); err != nil {
		t.Fatalf("public lookup err=%v", err)
	}
}

func TestProjectHandlerSecurityValidationAndEscaping(t *testing.T) {
	store := projectStore()
	handler := Handler{Repository: store, Search: store}
	res := httptest.NewRecorder()
	handler.OwnerCollection(res, httptest.NewRequest(http.MethodPost, "/api/v1/me/projects", strings.NewReader(`{}`)))
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated=%d", res.Code)
	}
	res = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/me/projects", strings.NewReader(`{"user_id":"`+customerTwo+`"}`))
	req = req.WithContext(auth.WithActorID(req.Context(), customerOne))
	handler.OwnerCollection(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("identity=%d", res.Code)
	}
	draft, err := store.Create(context.Background(), customerOne, validCreate())
	if err != nil {
		t.Fatal(err)
	}
	res = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/me/projects/"+draft.ID+"/publish", strings.NewReader(`{}`))
	req = req.WithContext(auth.WithActorID(req.Context(), customerOne))
	handler.OwnerItem(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("publish=%d body=%s", res.Code, res.Body.String())
	}
	res = httptest.NewRecorder()
	handler.PublicItem(res, httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+draft.ID, nil))
	if res.Code != http.StatusOK || strings.Contains(res.Body.String(), customerOne) {
		t.Fatalf("public detail=%d body=%s", res.Code, res.Body.String())
	}
	now := store.now()
	store.Items[projectOne] = Item{ID: projectOne, CustomerID: customerOne, Category: &Reference{ID: categoryID, Name: "Разработка"}, Title: `<script>alert(1)</script>`, Slug: "safe", Description: "text", Budget: Budget{Type: "NEGOTIABLE", Currency: "RUB"}, Visibility: "PUBLIC", Status: "OPEN", SourceType: "MANUAL", PublishedAt: &now, Skills: []Skill{}, Media: []Media{}, CreatedAt: now, UpdatedAt: now}
	res = httptest.NewRecorder()
	handler.PublicCollection(res, httptest.NewRequest(http.MethodGet, "/api/v1/projects?limit=1", nil))
	if res.Code != http.StatusOK || strings.Contains(res.Body.String(), "<script>") {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil || body["page"] == nil {
		t.Fatalf("body=%#v err=%v", body, err)
	}
	res = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/me/projects/"+projectOne+"/complete", strings.NewReader(`{}`))
	req.Header.Set("Idempotency-Key", strings.Repeat("x", 129))
	req = req.WithContext(auth.WithActorID(req.Context(), customerOne))
	handler.OwnerItem(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("idempotency=%d", res.Code)
	}
}

func TestCanonicalUUIDValidationAcceptsDeterministicSeedIDs(t *testing.T) {
	if !validUUID("ffe8f926-0f6c-e1c7-a416-973a6e000c8f") {
		t.Fatal("deterministic project seed UUID must be accepted")
	}
}

func TestProjectPublicAdvancedFilters(t *testing.T) {
	store := projectStore()
	now := store.now()

	firstInput := validCreate()
	firstInput.Slug = "short-deadline"
	firstDeadline := now.Add(48 * time.Hour)
	firstInput.DeadlineAt = &firstDeadline
	first, err := store.Create(context.Background(), customerOne, firstInput)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Transition(context.Background(), customerOne, first.ID, "publish"); err != nil {
		t.Fatal(err)
	}

	secondInput := validCreate()
	secondInput.Slug = "large-budget"
	largeBudget := int64(3_000_000)
	secondInput.Budget.MinKopecks = &largeBudget
	secondDeadline := now.Add(10 * 24 * time.Hour)
	secondInput.DeadlineAt = &secondDeadline
	second, err := store.Create(context.Background(), customerOne, secondInput)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Transition(context.Background(), customerOne, second.ID, "publish"); err != nil {
		t.Fatal(err)
	}

	minimum := int64(2_000_000)
	budgetPage, err := store.ListPublic(context.Background(), Filter{MinBudgetKopecks: &minimum}, nil, 20)
	if err != nil || len(budgetPage.Items) != 1 || budgetPage.Items[0].ID != second.ID {
		t.Fatalf("budget filter=%#v err=%v", budgetPage, err)
	}

	deadlineBefore := now.Add(72 * time.Hour)
	deadlinePage, err := store.ListPublic(context.Background(), Filter{DeadlineBefore: &deadlineBefore}, nil, 20)
	if err != nil || len(deadlinePage.Items) != 1 || deadlinePage.Items[0].ID != first.ID {
		t.Fatalf("deadline filter=%#v err=%v", deadlinePage, err)
	}
}
