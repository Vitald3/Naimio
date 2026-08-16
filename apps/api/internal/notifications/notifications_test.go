package notifications

import (
	"context"
	"freelance/apps/api/internal/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const owner = "00000000-0000-4000-8000-000000000001"
const other = "00000000-0000-4000-8000-000000000002"
const notificationID = "10000000-0000-4000-8000-000000000001"

func TestNotificationOwnershipReadAndPreferences(t *testing.T) {
	n := Notification{ID: notificationID, Type: "new_message", CreatedAt: time.Now()}
	s := &Store{Items: map[string]map[string]Notification{owner: {notificationID: n}, other: {}}, Prefs: map[string][]Preference{}}
	svc := Service{Repository: s}
	if err := svc.Read(context.Background(), other, notificationID); err != ErrNotFound {
		t.Fatalf("cross-user read allowed: %v", err)
	}
	if err := svc.Read(context.Background(), owner, notificationID); err != nil {
		t.Fatal(err)
	}
	page, err := svc.List(context.Background(), owner, nil, 50)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("notification list failed: %#v %v", page, err)
	}
	p, err := svc.ReplacePreferences(context.Background(), owner, []Preference{{EventType: "new_message", InApp: true, Email: false}})
	if err != nil || len(p) != 1 || p[0].Email {
		t.Fatalf("preferences failed: %#v %v", p, err)
	}
}
func TestNotificationHandlerAuthenticationAndStrictInput(t *testing.T) {
	h := Handler{Service: Service{Repository: &Store{Items: map[string]map[string]Notification{}, Prefs: map[string][]Preference{}}}}
	w := httptest.NewRecorder()
	h.Collection(w, httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil))
	if w.Code != 401 {
		t.Fatalf("status=%d", w.Code)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/notification-preferences", strings.NewReader(`{"preferences":[],"user_id":"forged"}`))
	req = req.WithContext(auth.WithActorID(req.Context(), owner))
	w = httptest.NewRecorder()
	h.Preferences(w, req)
	if w.Code != 400 {
		t.Fatalf("status=%d", w.Code)
	}
}
