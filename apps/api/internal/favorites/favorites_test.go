package favorites

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFavoriteIdempotencyVisibilityTypeAndCursor(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	projectID, serviceID, privateID := "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333"
	s := &Store{Items: map[string]Item{}, Visible: map[string]bool{"PROJECT:" + projectID: true, "SERVICE:" + serviceID: true, "PROJECT:" + privateID: false}, Now: func() time.Time { return now }}
	a, err := s.Put(context.Background(), "u1", "project", projectID)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Put(context.Background(), "u1", "PROJECT", projectID)
	if err != nil || a.CreatedAt != b.CreatedAt || len(s.Items) != 1 {
		t.Fatalf("idempotent=%#v err=%v", b, err)
	}
	if _, err = s.Put(context.Background(), "u1", "PROJECT", privateID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("private=%v", err)
	}
	if _, err = s.Put(context.Background(), "u1", "VACANCY", "x"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("type=%v", err)
	}
	if _, err = s.Put(context.Background(), "u1", "SERVICE", serviceID); err != nil {
		t.Fatal(err)
	}
	p, err := s.List(context.Background(), "u1", "", nil, 1)
	if err != nil || len(p.Items) != 1 || p.NextCursor == nil {
		t.Fatalf("page=%#v err=%v", p, err)
	}
	if err = s.Delete(context.Background(), "u1", "PROJECT", projectID); err != nil {
		t.Fatal(err)
	}
	if err = s.Delete(context.Background(), "u1", "PROJECT", projectID); err != nil {
		t.Fatal(err)
	}
}
func TestFavoriteHandlerRequiresAuthentication(t *testing.T) {
	h := Handler{Repository: &Store{Items: map[string]Item{}}}
	res := httptest.NewRecorder()
	h.Collection(res, httptest.NewRequest(http.MethodGet, "/api/v1/me/favorites", nil))
	if res.Code != 401 {
		t.Fatalf("status=%d", res.Code)
	}
}
