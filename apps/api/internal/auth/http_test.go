package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestArgon2PasswordHashAndSecureSessionCookie(t *testing.T) {
	encoded, err := hashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$") || !verifyPassword("correct horse battery staple", encoded) || verifyPassword("wrong password", encoded) {
		t.Fatal("password hashing verification failed")
	}
	h := Handler{CookieName: "session", Secure: true}
	w := httptest.NewRecorder()
	h.setCookie(w, "opaque", time.Now().Add(time.Hour))
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("insecure cookie: %#v", cookie)
	}
}
func TestRegisterValidationAndCSRF(t *testing.T) {
	h := Handler{}
	req := httptest.NewRequest(http.MethodPost, "http://example.test/api/v1/auth/register", strings.NewReader(`{"email":"u@example.test","password":"long-enough-password","display_name":"User","account_type":"CUSTOMER"}`))
	req.Header.Set("Origin", "https://evil.test")
	w := httptest.NewRecorder()
	h.Register(w, req)
	if w.Code != 403 {
		t.Fatalf("csrf status=%d", w.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"u@example.test","password":"short","display_name":"User"}`))
	w = httptest.NewRecorder()
	h.Register(w, req)
	if w.Code != 422 {
		t.Fatalf("validation status=%d", w.Code)
	}
}

func TestRegisterRequiresKnownAccountType(t *testing.T) {
	h := Handler{}
	for _, accountType := range []string{"", "ADMIN", "BOTH"} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"u@example.test","password":"long-enough-password","display_name":"User","account_type":"`+accountType+`"}`))
		w := httptest.NewRecorder()
		h.Register(w, req)
		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("account_type=%q status=%d", accountType, w.Code)
		}
	}
}

func TestSessionReturnsAnonymousStateWithoutCookieActor(t *testing.T) {
	h := Handler{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	w := httptest.NewRecorder()
	h.Session(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("anonymous session status=%d", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, `"data":null`) {
		t.Fatalf("anonymous session body=%s", body)
	}
}
