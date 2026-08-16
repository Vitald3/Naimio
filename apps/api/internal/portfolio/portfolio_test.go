package portfolio

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"freelance/apps/api/internal/auth"
)

const (
	userOne    = "11111111-1111-4111-8111-111111111111"
	userTwo    = "22222222-2222-4222-8222-222222222222"
	itemOne    = "33333333-3333-4333-8333-333333333333"
	itemTwo    = "44444444-4444-4444-8444-444444444444"
	itemThree  = "55555555-5555-4555-8555-555555555555"
	categoryID = "66666666-6666-4666-8666-666666666666"
	skillID    = "77777777-7777-4777-8777-777777777777"
	mediaID    = "88888888-8888-4888-8888-888888888888"
	mediaTwo   = "99999999-9999-4999-8999-999999999998"
)

func validWrite() WriteRequest {
	minPrice, maxPrice := int64(10000), int64(20000)
	return WriteRequest{Title: "Проект", Slug: "project", Description: "Описание", ExternalURL: "https://example.com/work",
		PriceMinKopecks: &minPrice, PriceMaxKopecks: &maxPrice, CompletedOn: "2025-01-01", Visibility: "PUBLIC",
		CategoryIDs: []string{categoryID}, SkillIDs: []string{skillID}}
}

func TestPortfolioCRUDOwnershipSlugAndSoftDelete(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	store := Store{Items: map[string]Item{}, Now: func() time.Time { return now }}
	created, err := store.Create(context.Background(), userOne, validWrite())
	if err != nil || created.ID == "" || created.UserID != userOne {
		t.Fatalf("created = %#v, error = %v", created, err)
	}
	if _, err := store.Create(context.Background(), userOne, validWrite()); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate slug error = %v", err)
	}
	if _, err := store.GetOwned(context.Background(), userTwo, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user get error = %v", err)
	}
	updatedInput := validWrite()
	updatedInput.Title = "Обновлённый проект"
	updated, err := store.Update(context.Background(), userOne, created.ID, updatedInput)
	if err != nil || updated.Title != updatedInput.Title {
		t.Fatalf("updated = %#v, error = %v", updated, err)
	}
	if err := store.Delete(context.Background(), userTwo, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user delete error = %v", err)
	}
	if err := store.Delete(context.Background(), userOne, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetOwned(context.Background(), userOne, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted item error = %v", err)
	}
	if _, err := store.Create(context.Background(), userOne, validWrite()); err != nil {
		t.Fatalf("slug was not reusable after soft delete: %v", err)
	}
}

func TestPortfolioValidation(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	store := Store{Items: map[string]Item{}, Now: func() time.Time { return now }}
	for name, mutate := range map[string]func(*WriteRequest){
		"unsafe url": func(input *WriteRequest) { input.ExternalURL = "javascript:alert(1)" },
		"price range": func(input *WriteRequest) {
			min, max := int64(2), int64(1)
			input.PriceMinKopecks, input.PriceMaxKopecks = &min, &max
		},
		"future completion": func(input *WriteRequest) { input.CompletedOn = "2027-01-01" },
		"duplicate skill":   func(input *WriteRequest) { input.SkillIDs = []string{skillID, skillID} },
	} {
		t.Run(name, func(t *testing.T) {
			input := validWrite()
			mutate(&input)
			if _, err := store.Create(context.Background(), userOne, input); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestMediaOwnershipSafetyAndIdempotency(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	store := Store{Items: map[string]Item{itemOne: {ID: itemOne, UserID: userOne, Visibility: "PUBLIC"}}, Media: map[string]MediaObject{
		mediaID:  {ID: mediaID, OwnerID: userOne, MIMEType: "image/jpeg", SizeBytes: 10, Purpose: "PORTFOLIO", ScanStatus: "CLEAN"},
		mediaTwo: {ID: mediaTwo, OwnerID: userOne, MIMEType: "image/jpeg", SizeBytes: 10, Purpose: "PORTFOLIO", ScanStatus: "PENDING", Uploaded: true},
	}, Now: func() time.Time { return now }}
	if _, err := store.AttachMedia(context.Background(), userOne, itemOne, mediaID, 0); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("incomplete media error = %v", err)
	}
	if _, err := store.AttachMedia(context.Background(), userOne, itemOne, mediaTwo, 0); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("pending media error = %v", err)
	}
	media := store.Media[mediaID]
	media.Uploaded = true
	store.Media[mediaID] = media
	for attempt := 0; attempt < 2; attempt++ {
		item, err := store.AttachMedia(context.Background(), userOne, itemOne, mediaID, 0)
		if err != nil || len(item.Media) != 1 {
			t.Fatalf("attempt %d item = %#v, error = %v", attempt, item, err)
		}
	}
	if _, err := store.AttachMedia(context.Background(), userTwo, itemOne, mediaID, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user media error = %v", err)
	}
	if err := store.DetachMedia(context.Background(), userOne, itemOne, mediaID); err != nil {
		t.Fatal(err)
	}
}

func TestPublicVisibilityAndCursorPagination(t *testing.T) {
	created := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	store := Store{Items: map[string]Item{
		itemOne:   {ID: itemOne, UserID: userOne, Username: "ivan", Title: "One", Slug: "one", Visibility: "PUBLIC", SortOrder: 0, CreatedAt: created},
		itemTwo:   {ID: itemTwo, UserID: userOne, Username: "ivan", Title: "Two", Slug: "two", Visibility: "PUBLIC", SortOrder: 1, CreatedAt: created},
		itemThree: {ID: itemThree, UserID: userOne, Username: "ivan", Title: "Private", Slug: "private", Visibility: "PRIVATE", SortOrder: 2, CreatedAt: created},
	}}
	first, err := store.ListPublic(context.Background(), "IVAN", nil, 1)
	if err != nil || len(first.Items) != 1 || first.NextCursor == nil || first.Items[0].UserID != "" {
		t.Fatalf("first page = %#v, error = %v", first, err)
	}
	second, err := store.ListPublic(context.Background(), "ivan", first.NextCursor, 1)
	if err != nil || len(second.Items) != 1 || second.Items[0].ID != itemTwo || second.NextCursor != nil {
		t.Fatalf("second page = %#v, error = %v", second, err)
	}
	cursor := encodeCursor(*first.NextCursor)
	decoded, err := decodeCursor(cursor)
	if err != nil || decoded.ID != first.NextCursor.ID {
		t.Fatalf("cursor = %#v, error = %v", decoded, err)
	}
}

func TestPortfolioHandlersRejectIdentityAndLargeBodies(t *testing.T) {
	store := Store{Items: map[string]Item{}}
	handler := Handler{Repository: &store}
	res := httptest.NewRecorder()
	handler.Collection(res, httptest.NewRequest(http.MethodPost, "/api/v1/me/portfolio", strings.NewReader(`{"user_id":"`+userTwo+`"}`)))
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", res.Code)
	}
	res = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/me/portfolio", strings.NewReader(strings.Repeat(" ", (128<<10)+1)+`{}`))
	req = req.WithContext(auth.WithActorID(req.Context(), userOne))
	handler.Collection(res, req)
	if res.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized status = %d", res.Code)
	}
}

func TestPublicHandlerResponseContract(t *testing.T) {
	created := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	store := Store{Items: map[string]Item{itemOne: {ID: itemOne, UserID: userOne, Username: "ivan", Title: `<script>alert(1)</script>`, Slug: "safe", Visibility: "PUBLIC", CreatedAt: created}}}
	res := httptest.NewRecorder()
	(Handler{Repository: &store}).PublicList(res, httptest.NewRequest(http.MethodGet, "/api/v1/profiles/ivan/portfolio?limit=1", nil))
	if res.Code != http.StatusOK || strings.Contains(res.Body.String(), "<script>") {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil || response["page"] == nil {
		t.Fatalf("response = %#v, error = %v", response, err)
	}
}

func TestPublicHandlerRejectsMalformedCursor(t *testing.T) {
	store := Store{Items: map[string]Item{}}
	for _, cursor := range []string{"not-base64!", base64.RawURLEncoding.EncodeToString([]byte(`{"sort_order":0,"created_at":"2026-08-12T12:00:00Z","id":"` + itemOne + `"}{}`))} {
		res := httptest.NewRecorder()
		(Handler{Repository: &store}).PublicList(res, httptest.NewRequest(http.MethodGet, "/api/v1/profiles/ivan/portfolio?cursor="+url.QueryEscape(cursor), nil))
		if res.Code != http.StatusBadRequest {
			t.Fatalf("cursor %q status = %d", cursor, res.Code)
		}
	}
}

type fixedLimitPolicy struct {
	itemLimit, mediaLimit int64
	enforced              bool
}

func (p fixedLimitPolicy) Limit(_ context.Context, _, key string) (int64, bool, bool, error) {
	if !p.enforced {
		return 0, true, false, nil
	}
	if key == "portfolio.media_limit" {
		return p.mediaLimit, false, true, nil
	}
	return p.itemLimit, false, true, nil
}

func TestPortfolioPlanLimitsAreEnforced(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	inner := &Store{Items: map[string]Item{}, Now: func() time.Time { return now }}
	repo := LimitedRepository{Repository: inner, Policy: fixedLimitPolicy{itemLimit: 1, mediaLimit: 1, enforced: true}}
	first := validWrite()
	first.Slug = "one"
	if _, err := repo.Create(context.Background(), userOne, first); err != nil {
		t.Fatal(err)
	}
	second := validWrite()
	second.Slug = "two"
	if _, err := repo.Create(context.Background(), userOne, second); !errors.Is(err, ErrItemLimit) {
		t.Fatalf("item limit err=%v", err)
	}
	mediaWrite := validWrite()
	mediaWrite.Slug = "media"
	mediaWrite.MediaObjectIDs = []string{mediaID, mediaTwo}
	repo = LimitedRepository{Repository: &Store{Items: map[string]Item{}, Now: func() time.Time { return now }}, Policy: fixedLimitPolicy{itemLimit: 8, mediaLimit: 1, enforced: true}}
	if _, err := repo.Create(context.Background(), userOne, mediaWrite); !errors.Is(err, ErrMediaLimit) {
		t.Fatalf("media limit err=%v", err)
	}
	disabled := LimitedRepository{Repository: &Store{Items: map[string]Item{}, Now: func() time.Time { return now }}, Policy: fixedLimitPolicy{itemLimit: 1, mediaLimit: 1, enforced: false}}
	for i := 0; i < 3; i++ {
		in := validWrite()
		in.Slug = "free-" + string(rune('a'+i))
		if _, err := disabled.Create(context.Background(), userOne, in); err != nil {
			t.Fatalf("disabled create %d: %v", i, err)
		}
	}
}
