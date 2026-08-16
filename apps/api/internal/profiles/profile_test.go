package profiles

import (
	"bytes"
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
	categoryID = "11111111-1111-4111-8111-111111111111"
	skillID    = "22222222-2222-4222-8222-222222222222"
)

type fixedSessionRepository struct{ session auth.Session }

func (r fixedSessionRepository) FindByTokenHash(context.Context, []byte) (auth.Session, error) {
	return r.session, nil
}

func TestPublicProfileDoesNotExposeUserID(t *testing.T) {
	s := Store{Items: map[string]Profile{"u1": {UserID: "u1", Username: "ivan", DisplayName: "Иван", Availability: "AVAILABLE", ProfileVisibility: "PUBLIC"}}}
	p, err := s.Public(context.Background(), "ivan")
	if err != nil || p.UserID != "" || p.ID != "u1" {
		t.Fatalf("public profile = %#v, %v", p, err)
	}
	body, _ := json.Marshal(p)
	if strings.Contains(string(body), "user_id") {
		t.Fatalf("public JSON exposes user_id: %s", body)
	}
}

func TestDeterministicTaxonomyUUIDIsAccepted(t *testing.T) {
	if err := validateCategories([]Category{{ID: "878dfaf9-24be-6b22-85db-f1076d701bff", IsPrimary: true}}); err != nil {
		t.Fatalf("deterministic category id rejected: %v", err)
	}
}

func TestPrivateProfileIsNotPubliclyDiscoverable(t *testing.T) {
	s := Store{Items: map[string]Profile{"u1": {UserID: "u1", Username: "private", ProfileVisibility: "PRIVATE"}}}
	page, err := s.PublicList(context.Background(), "", nil, 20)
	if _, publicErr := s.Public(context.Background(), "private"); !errors.Is(publicErr, ErrNotFound) || err != nil || len(page.Items) != 0 {
		t.Fatal("private profile was exposed")
	}
}

func TestPublicListUsesCompactCards(t *testing.T) {
	minimumOrder := int64(10000)
	s := Store{Items: map[string]Profile{"u1": {
		UserID: "u1", Username: "public", Bio: "full bio", LocationText: "private card detail",
		MinimumOrderKopecks: &minimumOrder, Availability: "AVAILABLE", ProfileVisibility: "PUBLIC",
	}}}
	page, err := s.PublicList(context.Background(), "", nil, 20)
	if err != nil || len(page.Items) != 1 || page.Items[0].Bio != "" || page.Items[0].LocationText != "" || page.Items[0].MinimumOrderKopecks != nil {
		t.Fatalf("items = %#v, error = %v", page.Items, err)
	}
}

func TestPublicListSearch(t *testing.T) {
	s := &Store{Items: map[string]Profile{
		"11111111-1111-4111-8111-111111111111": {UserID: "11111111-1111-4111-8111-111111111111", Username: "go-dev", DisplayName: "Иван", ProfessionalTitle: "Go разработчик", Availability: "AVAILABLE", ProfileVisibility: "PUBLIC"},
		"22222222-2222-4222-8222-222222222222": {UserID: "22222222-2222-4222-8222-222222222222", Username: "designer", DisplayName: "Анна", ProfessionalTitle: "Дизайнер", Availability: "AVAILABLE", ProfileVisibility: "PUBLIC"},
	}}
	page, err := s.PublicList(context.Background(), "Go", nil, 20)
	if err != nil || len(page.Items) != 1 || page.Items[0].Username != "go-dev" {
		t.Fatalf("items=%#v err=%v", page.Items, err)
	}
}

func TestUpdateRejectsUnauthenticatedRequest(t *testing.T) {
	res := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH", "/api/v1/me/professional-profile", nil)
	(Handler{}).Update(res, req)
	if res.Code != 401 {
		t.Fatalf("status = %d", res.Code)
	}
}

func TestProfileUpdateRequiresOwner(t *testing.T) {
	s := Store{Items: map[string]Profile{"u1": {UserID: "u1", Username: "ivan", DisplayName: "Иван", Availability: "AVAILABLE", ProfileVisibility: "PUBLIC"}}}
	input := UpdateRequest{Availability: "BUSY", ProfileVisibility: "PUBLIC"}
	if _, err := s.Update(context.Background(), "u2", input); err != ErrNotFound {
		t.Fatalf("error = %v", err)
	}
	if _, err := s.Update(context.Background(), "", input); err != ErrUnauthorized {
		t.Fatalf("error = %v", err)
	}
}

func TestUpdateUsesAuthenticatedActorAndIsIdempotent(t *testing.T) {
	store := Store{Items: map[string]Profile{"u1": {UserID: "u1", Username: "ivan", DisplayName: "Иван", Availability: "UNAVAILABLE", ProfileVisibility: "PRIVATE"}}}
	input := UpdateRequest{
		ProfessionalTitle: "Go-разработчик",
		Availability:      "AVAILABLE",
		ProfileVisibility: "PUBLIC",
		Categories:        []CategorySelection{{ID: categoryID, IsPrimary: true}},
		Skills:            []SkillSelection{{ID: skillID, Level: "EXPERT"}},
		Languages:         []Language{{Code: "ru", Level: "NATIVE"}},
	}
	body, _ := json.Marshal(input)
	handler := Handler{Repository: &store}
	for attempt := 0; attempt < 2; attempt++ {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/me/professional-profile", bytes.NewReader(body))
		req = req.WithContext(auth.WithActorID(req.Context(), "u1"))
		handler.Update(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("attempt %d status = %d, body = %s", attempt, res.Code, res.Body.String())
		}
	}
	got := store.Items["u1"]
	if got.UserID != "u1" || len(got.Skills) != 1 || got.Skills[0].ID != skillID {
		t.Fatalf("profile = %#v", got)
	}
}

func TestAssociationPutEndpointsAreIdempotent(t *testing.T) {
	store := Store{Items: map[string]Profile{"u1": {UserID: "u1", Availability: "AVAILABLE", ProfileVisibility: "PUBLIC"}}}
	handler := Handler{Repository: &store}
	tests := []struct {
		path string
		body string
		call http.HandlerFunc
	}{
		{path: "/api/v1/me/categories", body: `{"categories":[{"id":"` + categoryID + `","is_primary":true}]}`, call: handler.ReplaceCategories},
		{path: "/api/v1/me/skills", body: `{"skills":[{"id":"` + skillID + `","level":"EXPERT","is_featured":true}]}`, call: handler.ReplaceSkills},
		{path: "/api/v1/me/languages", body: `{"languages":[{"code":"RU","level":"NATIVE"}]}`, call: handler.ReplaceLanguages},
	}
	for _, testCase := range tests {
		for attempt := 0; attempt < 2; attempt++ {
			res := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, testCase.path, strings.NewReader(testCase.body))
			req = req.WithContext(auth.WithActorID(req.Context(), "u1"))
			testCase.call.ServeHTTP(res, req)
			if res.Code != http.StatusOK {
				t.Fatalf("%s attempt %d status = %d, body = %s", testCase.path, attempt, res.Code, res.Body.String())
			}
		}
	}
	profile := store.Items["u1"]
	if len(profile.Categories) != 1 || len(profile.Skills) != 1 || len(profile.Languages) != 1 || profile.Languages[0].Code != "ru" {
		t.Fatalf("profile = %#v", profile)
	}
}

func TestUpdateRejectsInvalidAssociations(t *testing.T) {
	s := Store{Items: map[string]Profile{"u1": {UserID: "u1", Availability: "AVAILABLE", ProfileVisibility: "PUBLIC"}}}
	input := UpdateRequest{Availability: "AVAILABLE", ProfileVisibility: "PUBLIC", Categories: []CategorySelection{{ID: categoryID, IsPrimary: true}, {ID: "33333333-3333-4333-8333-333333333333", IsPrimary: true}}}
	if _, err := s.Update(context.Background(), "u1", input); err == nil {
		t.Fatal("expected multiple primary categories to be rejected")
	}
}

func TestUpdateRejectsInvalidProfileRanges(t *testing.T) {
	negativeRate := int64(-1)
	s := Store{Items: map[string]Profile{"u1": {UserID: "u1", Availability: "AVAILABLE", ProfileVisibility: "PUBLIC"}}}
	input := UpdateRequest{Availability: "AVAILABLE", ProfileVisibility: "PUBLIC", CountryCode: "rus", HourlyRateKopecks: &negativeRate}
	if _, err := s.Update(context.Background(), "u1", input); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("error = %v", err)
	}
}

func TestCrossUserUpdateReturnsNotFoundAndDoesNotMutateOwner(t *testing.T) {
	store := Store{Items: map[string]Profile{"u1": {UserID: "u1", Availability: "AVAILABLE", ProfileVisibility: "PUBLIC"}}}
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/me/professional-profile", strings.NewReader(`{"availability":"BUSY","profile_visibility":"PUBLIC"}`))
	req = req.WithContext(auth.WithActorID(req.Context(), "u2"))
	(Handler{Repository: &store}).Update(res, req)
	if res.Code != http.StatusNotFound || store.Items["u1"].Availability != "AVAILABLE" {
		t.Fatalf("status = %d, owner profile = %#v", res.Code, store.Items["u1"])
	}
}

func TestSessionAuthenticatedCrossUserUpdateIsIsolated(t *testing.T) {
	store := Store{Items: map[string]Profile{"u1": {UserID: "u1", Availability: "AVAILABLE", ProfileVisibility: "PUBLIC"}}}
	handler := auth.SessionMiddleware{Repository: fixedSessionRepository{session: auth.Session{UserID: "u2", UserStatus: "ACTIVE", ExpiresAt: time.Now().Add(time.Hour)}}}.
		RequireSession(http.HandlerFunc((Handler{Repository: &store}).Update))
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/me/professional-profile", strings.NewReader(`{"availability":"BUSY","profile_visibility":"PUBLIC"}`))
	req.AddCookie(&http.Cookie{Name: "session", Value: "opaque-session-token"})
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusNotFound || store.Items["u1"].Availability != "AVAILABLE" {
		t.Fatalf("status = %d, owner profile = %#v", res.Code, store.Items["u1"])
	}
}

func TestUpdateRejectsOversizedAndMultipleJSONBodies(t *testing.T) {
	store := Store{Items: map[string]Profile{"u1": {UserID: "u1", Availability: "AVAILABLE", ProfileVisibility: "PUBLIC"}}}
	handler := Handler{Repository: &store}
	for name, testCase := range map[string]struct {
		body   string
		status int
	}{
		"oversized": {body: strings.Repeat(" ", (64<<10)+1) + `{}`, status: http.StatusRequestEntityTooLarge},
		"multiple":  {body: `{"availability":"BUSY","profile_visibility":"PUBLIC"}{}`, status: http.StatusBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			res := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/me/professional-profile", strings.NewReader(testCase.body))
			req = req.WithContext(auth.WithActorID(req.Context(), "u1"))
			handler.Update(res, req)
			if res.Code != testCase.status || store.Items["u1"].Availability != "AVAILABLE" {
				t.Fatalf("status = %d, profile = %#v", res.Code, store.Items["u1"])
			}
		})
	}
}

func TestPublicProfileEscapesUserContent(t *testing.T) {
	store := Store{Items: map[string]Profile{"u1": {UserID: "u1", Username: "ivan", Bio: `<script>alert(1)</script>`, Availability: "AVAILABLE", ProfileVisibility: "PUBLIC"}}}
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/ivan", nil)
	(Handler{Repository: &store}).Public(res, req)
	if res.Code != http.StatusOK || strings.Contains(res.Body.String(), "<script>") {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestUpdateRejectsClientSuppliedIdentity(t *testing.T) {
	store := Store{Items: map[string]Profile{"u1": {UserID: "u1", Availability: "AVAILABLE", ProfileVisibility: "PUBLIC"}}}
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/me/professional-profile", strings.NewReader(`{"user_id":"u2","availability":"BUSY","profile_visibility":"PUBLIC"}`))
	req.Header.Set("X-Request-ID", "req_security_test")
	req = req.WithContext(auth.WithActorID(req.Context(), "u1"))
	(Handler{Repository: &store}).Update(res, req)
	if res.Code != http.StatusBadRequest || store.Items["u1"].Availability != "AVAILABLE" || !strings.Contains(res.Body.String(), "req_security_test") {
		t.Fatalf("status = %d, profile = %#v", res.Code, store.Items["u1"])
	}
}
