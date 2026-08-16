package services

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
	sellerOne  = "11111111-1111-4111-8111-111111111111"
	sellerTwo  = "22222222-2222-4222-8222-222222222222"
	serviceOne = "33333333-3333-4333-8333-333333333333"
	serviceTwo = "44444444-4444-4444-8444-444444444444"
	categoryID = "55555555-5555-4555-8555-555555555555"
	skillID    = "66666666-6666-4666-8666-666666666666"
	mediaID    = "77777777-7777-4777-8777-777777777777"
)

func serviceStore() *Store {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	return &Store{Items: map[string]Item{}, Categories: map[string]Reference{categoryID: {ID: categoryID, Slug: "development", Name: "Разработка"}},
		Skills:         map[string]Reference{skillID: {ID: skillID, Slug: "go", Name: "Go"}},
		Media:          map[string]MediaObject{mediaID: {ID: mediaID, OwnerID: sellerOne, Purpose: "SERVICE", MIMEType: "image/png", SizeBytes: 100, Uploaded: true, ScanStatus: "CLEAN"}},
		SellerEligible: map[string]bool{sellerOne: true, sellerTwo: true}, Publishable: map[string]bool{sellerOne: true, sellerTwo: true}, Now: func() time.Time { return now }}
}

func validCreate() CreateRequest {
	return CreateRequest{CategoryID: categoryID, ServiceType: "PROFESSIONAL_SERVICE", Title: "Разработаю API", Slug: "api-development",
		ShortDescription: "Надёжный API", Description: "Спроектирую и реализую API.", PriceType: "FROM",
		PriceFrom: &Money{AmountKopecks: 1500000, Currency: "RUB"}, Visibility: "PUBLIC", SkillIDs: []string{skillID}, MediaObjectIDs: []string{mediaID}}
}

func TestServiceCRUDOwnershipAndSlug(t *testing.T) {
	store := serviceStore()
	created, err := store.Create(context.Background(), sellerOne, validCreate())
	if err != nil || created.Status != "DRAFT" || len(created.Skills) != 1 || len(created.Media) != 1 {
		t.Fatalf("created = %#v, error = %v", created, err)
	}
	if _, err := store.Create(context.Background(), sellerOne, validCreate()); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate error = %v", err)
	}
	if _, err := store.GetOwned(context.Background(), sellerTwo, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user get error = %v", err)
	}
	title := "Обновлённая услуга"
	updated, err := store.Update(context.Background(), sellerOne, created.ID, PatchRequest{Title: &title})
	if err != nil || updated.Title != title || updated.Description != created.Description {
		t.Fatalf("updated = %#v, error = %v", updated, err)
	}
	if err := store.Delete(context.Background(), sellerTwo, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user delete error = %v", err)
	}
	if err := store.Delete(context.Background(), sellerOne, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(context.Background(), sellerOne, validCreate()); err != nil {
		t.Fatalf("slug reuse error = %v", err)
	}
}

func TestServiceValidationAndConditionalEducation(t *testing.T) {
	store := serviceStore()
	for name, mutate := range map[string]func(*CreateRequest){
		"zero price":                func(input *CreateRequest) { input.PriceFrom.AmountKopecks = 0 },
		"negotiable price":          func(input *CreateRequest) { input.PriceType = "NEGOTIABLE" },
		"education on professional": func(input *CreateRequest) { input.Education = &EducationDetails{Format: "ONLINE"} },
		"education details missing": func(input *CreateRequest) { input.ServiceType = "EDUCATION" },
		"duplicate skill":           func(input *CreateRequest) { input.SkillIDs = []string{skillID, skillID} },
	} {
		t.Run(name, func(t *testing.T) {
			input := validCreate()
			mutate(&input)
			if _, err := store.Create(context.Background(), sellerOne, input); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("error = %v", err)
			}
		})
	}
	education := validCreate()
	education.ServiceType, education.Slug = "EDUCATION", "go-course"
	education.Education = &EducationDetails{Format: "ONLINE", AudienceType: "INDIVIDUAL"}
	educationItem, err := store.Create(context.Background(), sellerOne, education)
	if err != nil {
		t.Fatalf("education error = %v", err)
	}
	if _, err = store.Transition(context.Background(), sellerOne, educationItem.ID, "publish"); err != nil {
		t.Fatal(err)
	}
	filtered, err := store.ListPublic(context.Background(), Filter{ServiceType: "EDUCATION", Format: "ONLINE", Audience: "INDIVIDUAL"}, nil, 20)
	if err != nil || len(filtered.Items) != 1 || filtered.Items[0].Education == nil {
		t.Fatalf("education filter = %#v, error = %v", filtered, err)
	}
	store.Media[mediaID] = MediaObject{ID: mediaID, OwnerID: sellerOne, Purpose: "PORTFOLIO", Uploaded: true, ScanStatus: "CLEAN"}
	wrongPurpose := validCreate()
	wrongPurpose.Slug = "wrong-purpose"
	if _, err := store.Create(context.Background(), sellerOne, wrongPurpose); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("media purpose error = %v", err)
	}
}

func TestServiceTransitionsVisibilityFiltersAndPagination(t *testing.T) {
	store := serviceStore()
	first, err := store.Create(context.Background(), sellerOne, validCreate())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Transition(context.Background(), sellerOne, first.ID, "resume"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("invalid transition = %v", err)
	}
	active, err := store.Transition(context.Background(), sellerOne, first.ID, "publish")
	if err != nil || active.Status != "ACTIVE" || active.PublishedAt == nil {
		t.Fatalf("active = %#v, error = %v", active, err)
	}
	if _, err := store.Update(context.Background(), sellerOne, first.ID, PatchRequest{}); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("active update error = %v", err)
	}
	paused, err := store.Transition(context.Background(), sellerOne, first.ID, "pause")
	if err != nil || paused.Status != "PAUSED" {
		t.Fatalf("paused = %#v, error = %v", paused, err)
	}
	if _, err := store.Transition(context.Background(), sellerOne, first.ID, "resume"); err != nil {
		t.Fatal(err)
	}

	secondInput := validCreate()
	secondInput.Slug, secondInput.ServiceType = "consultation", "CONSULTATION"
	secondInput.Title = "Консультация по API"
	secondInput.MediaObjectIDs = nil
	second, err := store.Create(context.Background(), sellerOne, secondInput)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Transition(context.Background(), sellerOne, second.ID, "publish"); err != nil {
		t.Fatal(err)
	}
	page, err := store.ListPublic(context.Background(), Filter{}, nil, 1)
	if err != nil || len(page.Items) != 1 || page.NextCursor == nil || page.Items[0].SellerID != sellerOne {
		t.Fatalf("page = %#v, error = %v", page, err)
	}
	next, err := store.ListPublic(context.Background(), Filter{}, page.NextCursor, 1)
	if err != nil || len(next.Items) != 1 || next.NextCursor != nil {
		t.Fatalf("next = %#v, error = %v", next, err)
	}
	filtered, err := store.ListPublic(context.Background(), Filter{ServiceType: "CONSULTATION"}, nil, 20)
	if err != nil || len(filtered.Items) != 1 || filtered.Items[0].ID != second.ID {
		t.Fatalf("filtered = %#v, error = %v", filtered, err)
	}
	searched, err := store.ListPublic(context.Background(), Filter{Q: "консульта"}, nil, 20)
	if err != nil || len(searched.Items) != 1 || searched.Items[0].ID != second.ID {
		t.Fatalf("searched = %#v, error = %v", searched, err)
	}
}

func TestPublishRevalidatesMediaAndSeller(t *testing.T) {
	store := serviceStore()
	created, err := store.Create(context.Background(), sellerOne, validCreate())
	if err != nil {
		t.Fatal(err)
	}
	medium := store.Media[mediaID]
	medium.ScanStatus = "INFECTED"
	store.Media[mediaID] = medium
	if _, err := store.Transition(context.Background(), sellerOne, created.ID, "publish"); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("infected media error = %v", err)
	}
	medium.ScanStatus = "CLEAN"
	store.Media[mediaID] = medium
	store.Publishable[sellerOne] = false
	if _, err := store.Transition(context.Background(), sellerOne, created.ID, "publish"); !errors.Is(err, ErrSellerIneligible) {
		t.Fatalf("seller error = %v", err)
	}
}

func TestServiceHandlersSecurityAndEscaping(t *testing.T) {
	store := serviceStore()
	handler := Handler{Repository: store, Search: store}
	res := httptest.NewRecorder()
	handler.OwnerCollection(res, httptest.NewRequest(http.MethodPost, "/api/v1/me/services", strings.NewReader(`{}`)))
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", res.Code)
	}
	res = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/me/services", strings.NewReader(strings.Repeat(" ", (128<<10)+1)+`{}`))
	req = req.WithContext(auth.WithActorID(req.Context(), sellerOne))
	handler.OwnerCollection(res, req)
	if res.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized status = %d", res.Code)
	}
	res = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/me/services", strings.NewReader(`{"user_id":"`+sellerTwo+`"}`))
	req = req.WithContext(auth.WithActorID(req.Context(), sellerOne))
	handler.OwnerCollection(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("identity status = %d", res.Code)
	}

	now := store.now()
	store.Items[serviceOne] = Item{ID: serviceOne, SellerID: sellerOne, SellerUsername: "ivan", SellerDisplayName: "Иван", Category: store.Categories[categoryID], ServiceType: "PROFESSIONAL_SERVICE", Title: `<script>alert(1)</script>`, Slug: "safe", Description: "text", PriceType: "NEGOTIABLE", Status: "ACTIVE", Visibility: "PUBLIC", PublishedAt: &now, CreatedAt: now, UpdatedAt: now, Skills: []Reference{}, Media: []Media{}}
	res = httptest.NewRecorder()
	handler.PublicCollection(res, httptest.NewRequest(http.MethodGet, "/api/v1/services?limit=1", nil))
	if res.Code != http.StatusOK || strings.Contains(res.Body.String(), "<script>") {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil || body["page"] == nil {
		t.Fatalf("body = %#v, error = %v", body, err)
	}
}

func TestPublicSlugAmbiguityIsHidden(t *testing.T) {
	store := serviceStore()
	now := store.now()
	base := Item{Category: store.Categories[categoryID], ServiceType: "PROFESSIONAL_SERVICE", Slug: "same", Description: "text", PriceType: "NEGOTIABLE", Status: "ACTIVE", Visibility: "PUBLIC", PublishedAt: &now, CreatedAt: now, UpdatedAt: now}
	first, second := base, base
	first.ID, first.SellerID, first.Title = serviceOne, sellerOne, "One"
	second.ID, second.SellerID, second.Title = serviceTwo, sellerTwo, "Two"
	store.Items[first.ID], store.Items[second.ID] = first, second
	if _, err := store.GetPublic(context.Background(), "same"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v", err)
	}
}

func TestServiceModerationIsPrivilegedAndPublicSafe(t *testing.T) {
	store := serviceStore()
	created, err := store.Create(context.Background(), sellerOne, validCreate())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Transition(context.Background(), sellerOne, created.ID, "publish"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Moderate(context.Background(), sellerTwo, created.ID, "HIDE", "prohibited content"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("unauthorized moderation = %v", err)
	}
	store.Admins = map[string]bool{sellerTwo: true}
	if _, err = store.Moderate(context.Background(), sellerTwo, created.ID, "HIDE", "prohibited content"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.GetPublic(context.Background(), created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("hidden public = %v", err)
	}
	if _, err = store.Moderate(context.Background(), sellerTwo, created.ID, "RESTORE", "reviewed content"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.GetPublic(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
}

func TestCanonicalUUIDValidationAcceptsDeterministicSeedIDs(t *testing.T) {
	// PostgreSQL's uuid type accepts any canonical 128-bit UUID. Production IDs
	// are generated as UUIDv7, while deterministic development fixtures use an
	// MD5-derived canonical UUID. Routing must accept both.
	if !validUUID("262d09c7-ad16-21f6-1aab-57958217e433") {
		t.Fatal("deterministic service seed UUID must be accepted")
	}
}

func TestServicePublicMaximumDurationFilter(t *testing.T) {
	store := serviceStore()
	input := validCreate()
	input.ServiceType, input.Slug = "EDUCATION", "duration-filter"
	duration := 60
	input.Education = &EducationDetails{Format: "ONLINE", AudienceType: "INDIVIDUAL", DurationMinutes: &duration}
	item, err := store.Create(context.Background(), sellerOne, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Transition(context.Background(), sellerOne, item.ID, "publish"); err != nil {
		t.Fatal(err)
	}

	maxShort := 30
	shortPage, err := store.ListPublic(context.Background(), Filter{ServiceType: "EDUCATION", MaxDurationMinutes: &maxShort}, nil, 20)
	if err != nil || len(shortPage.Items) != 0 {
		t.Fatalf("short duration filter=%#v err=%v", shortPage, err)
	}
	maxExact := 60
	exactPage, err := store.ListPublic(context.Background(), Filter{ServiceType: "EDUCATION", MaxDurationMinutes: &maxExact}, nil, 20)
	if err != nil || len(exactPage.Items) != 1 || exactPage.Items[0].ID != item.ID {
		t.Fatalf("exact duration filter=%#v err=%v", exactPage, err)
	}
}
