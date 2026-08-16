package reputation

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
	ownerOne = "11111111-1111-4111-8111-111111111111"
	ownerTwo = "22222222-2222-4222-8222-222222222222"
)

func TestExternalReputationCRUDOwnershipAndState(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	store := &Store{Items: map[string]Item{}, Now: func() time.Time { return now }}
	service := Service{Repository: store}
	created, err := service.Create(context.Background(), ownerOne, CreateRequest{Platform: "github", ProfileURL: "HTTPS://GitHub.com/example/", ExternalUsername: " example "})
	if err != nil || created.VerificationStatus != StatusUnverified || created.ProfileURL != "https://github.com/example" || created.ExternalUsername != "example" {
		t.Fatalf("created=%#v err=%v", created, err)
	}
	if _, err = service.Create(context.Background(), ownerOne, CreateRequest{Platform: "GITHUB", ProfileURL: "https://github.com/example"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate error=%v", err)
	}
	profileURL := "https://github.com/updated"
	updated, err := service.Update(context.Background(), ownerOne, created.ID, PatchRequest{ProfileURL: &profileURL})
	if err != nil || updated.ProfileURL != profileURL {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
	if _, err = service.Update(context.Background(), ownerTwo, created.ID, PatchRequest{ProfileURL: &profileURL}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user update error=%v", err)
	}
	item := store.Items[created.ID]
	item.VerificationStatus = "VERIFIED"
	store.Items[created.ID] = item
	if _, err = service.Update(context.Background(), ownerOne, created.ID, PatchRequest{ProfileURL: &profileURL}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("verified update error=%v", err)
	}
	if err = service.Delete(context.Background(), ownerTwo, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user delete error=%v", err)
	}
}

func TestExternalURLValidation(t *testing.T) {
	service := Service{Repository: &Store{Items: map[string]Item{}}}
	for name, input := range map[string]CreateRequest{
		"javascript":    {Platform: "OTHER", ProfileURL: "javascript:alert(1)"},
		"credentials":   {Platform: "OTHER", ProfileURL: "https://user:pass@example.com/profile"},
		"ip literal":    {Platform: "OTHER", ProfileURL: "http://127.0.0.1/profile"},
		"wrong host":    {Platform: "GITHUB", ProfileURL: "https://github.example/profile"},
		"platform root": {Platform: "KWORK", ProfileURL: "https://kwork.ru/"},
		"unknown":       {Platform: "UNKNOWN", ProfileURL: "https://example.com/profile"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := service.Create(context.Background(), ownerOne, input); !errors.Is(err, ErrInvalid) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestPublicProjectionContainsOnlyVerifiedSafeFields(t *testing.T) {
	rating, reviews, orders, since := 4.9, 132, 168, "2021-04-01"
	verified := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	store := &Store{PublicUsers: map[string]string{"ivan": ownerOne}, Items: map[string]Item{
		"verified": {ID: "verified", UserID: ownerOne, Platform: "KWORK", ProfileURL: "https://kwork.ru/user/example", VerificationStatus: "VERIFIED", Rating: &rating, ReviewsCount: &reviews, CompletedOrdersCount: &orders, AccountSince: &since, VerifiedAt: &verified},
		"private":  {ID: "private", UserID: ownerOne, Platform: "GITHUB", ProfileURL: "https://github.com/example", VerificationStatus: "UNVERIFIED"},
	}}
	items, err := (Service{Repository: store}).ListPublic(context.Background(), "IVAN")
	if err != nil || len(items) != 1 || !items[0].Verified {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	body, _ := json.Marshal(items)
	for _, forbidden := range []string{"evidence", "source_snapshot", "verification_status", "user_id", "internal"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("public JSON contains %q: %s", forbidden, body)
		}
	}
}

func TestHandlersRejectForgedStateAndCrossUserMutation(t *testing.T) {
	store := &Store{Items: map[string]Item{}}
	handler := Handler{Service: Service{Repository: store}}
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/me/external-reputations", strings.NewReader(`{"platform":"GITHUB","profile_url":"https://github.com/example","verification_status":"VERIFIED"}`))
	req = req.WithContext(auth.WithActorID(req.Context(), ownerOne))
	handler.OwnerCollection(res, req)
	if res.Code != http.StatusBadRequest || len(store.Items) != 0 {
		t.Fatalf("forged status=%d body=%s", res.Code, res.Body.String())
	}
	created, err := (Service{Repository: store}).Create(context.Background(), ownerOne, CreateRequest{Platform: "GITHUB", ProfileURL: "https://github.com/example"})
	if err != nil {
		t.Fatal(err)
	}
	res = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/me/external-reputations/"+created.ID, strings.NewReader(`{"external_username":"attacker"}`))
	req = req.WithContext(auth.WithActorID(req.Context(), ownerTwo))
	handler.OwnerItem(res, req)
	if res.Code != http.StatusNotFound || store.Items[created.ID].ExternalUsername != "" {
		t.Fatalf("cross-user status=%d item=%#v", res.Code, store.Items[created.ID])
	}
}

func TestOwnerHandlerRequiresAuthenticationAndBoundsBody(t *testing.T) {
	handler := Handler{Service: Service{Repository: &Store{Items: map[string]Item{}}}}
	res := httptest.NewRecorder()
	handler.OwnerCollection(res, httptest.NewRequest(http.MethodGet, "/api/v1/me/external-reputations", nil))
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", res.Code)
	}
	res = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/me/external-reputations", strings.NewReader(strings.Repeat(" ", (16<<10)+1)+`{}`))
	req = req.WithContext(auth.WithActorID(req.Context(), ownerOne))
	handler.OwnerCollection(res, req)
	if res.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d", res.Code)
	}
}

func TestVerificationChallengeAndModeratorDecision(t *testing.T) {
	now := time.Now().UTC()
	admin := "33333333-3333-4333-8333-333333333333"
	store := &Store{Items: map[string]Item{}, Challenges: map[string]Challenge{}, Evidence: map[string]map[string]any{}, Admins: map[string]bool{admin: true}, Now: func() time.Time { return now }}
	service := Service{Repository: store}
	created, err := service.Create(context.Background(), ownerOne, CreateRequest{Platform: "GITHUB", ProfileURL: "https://github.com/example"})
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := service.StartVerification(context.Background(), ownerOne, created.ID, StartVerificationRequest{Method: "PROFILE_CODE", Evidence: map[string]any{"context": "profile bio"}})
	if err != nil || !strings.HasPrefix(challenge.Code, "VERIFY-FR-") || store.Items[created.ID].VerificationStatus != "PENDING" {
		t.Fatalf("challenge=%#v item=%#v err=%v", challenge, store.Items[created.ID], err)
	}
	if _, err = service.StartVerification(context.Background(), ownerOne, created.ID, StartVerificationRequest{Method: "PROFILE_CODE"}); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("replay start=%v", err)
	}
	expired, err := store.GetVerification(context.Background(), ownerOne, created.ID, challenge.ExpiresAt.Add(time.Second))
	if err != nil || expired.Status != "EXPIRED" || store.Items[created.ID].VerificationStatus != "EXPIRED" {
		t.Fatalf("expired=%#v item=%#v err=%v", expired, store.Items[created.ID], err)
	}
	challenge, err = service.StartVerification(context.Background(), ownerOne, created.ID, StartVerificationRequest{Method: "PROFILE_CODE", Evidence: map[string]any{"context": "profile bio"}})
	if err != nil || challenge.Status != "PENDING" {
		t.Fatalf("restart=%#v err=%v", challenge, err)
	}
	if _, err = service.ListPending(context.Background(), ownerTwo); !errors.Is(err, ErrForbidden) {
		t.Fatalf("permission=%v", err)
	}
	queue, err := service.ListPending(context.Background(), admin)
	if err != nil || len(queue) != 1 || queue[0].Evidence["context"] != "profile bio" {
		t.Fatalf("queue=%#v err=%v", queue, err)
	}
	verified, err := service.Decide(context.Background(), admin, created.ID, "verify", DecisionRequest{})
	if err != nil || verified.VerificationStatus != "VERIFIED" || len(store.Audits) != 1 {
		t.Fatalf("verified=%#v audits=%#v err=%v", verified, store.Audits, err)
	}
	if _, err = service.Decide(context.Background(), admin, created.ID, "verify", DecisionRequest{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("repeat decision=%v", err)
	}
}

func TestVerificationRejectionRequiresReason(t *testing.T) {
	admin := "33333333-3333-4333-8333-333333333333"
	store := &Store{Items: map[string]Item{}, Challenges: map[string]Challenge{}, Evidence: map[string]map[string]any{}, Admins: map[string]bool{admin: true}}
	service := Service{Repository: store}
	created, _ := service.Create(context.Background(), ownerOne, CreateRequest{Platform: "OTHER", ProfileURL: "https://example.com/profile"})
	_, _ = service.StartVerification(context.Background(), ownerOne, created.ID, StartVerificationRequest{Method: "MANUAL", Evidence: map[string]any{"note": "ownership evidence"}})
	if _, err := service.Decide(context.Background(), admin, created.ID, "reject", DecisionRequest{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing reason=%v", err)
	}
	item, err := service.Decide(context.Background(), admin, created.ID, "reject", DecisionRequest{ReasonCode: "OWNERSHIP_NOT_PROVEN", Note: "insufficient evidence"})
	if err != nil || item.VerificationStatus != "REJECTED" {
		t.Fatalf("item=%#v err=%v", item, err)
	}
}
