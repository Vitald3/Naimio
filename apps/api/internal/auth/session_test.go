package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeSessionRepository struct {
	session Session
	err     error
}

func (r fakeSessionRepository) FindByTokenHash(context.Context, []byte) (Session, error) {
	return r.session, r.err
}

func TestSessionMiddleware(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	revokedAt := now.Add(-time.Minute)
	tests := map[string]struct {
		repository SessionRepository
		cookie     bool
		status     int
		actor      string
	}{
		"valid":              {repository: fakeSessionRepository{session: Session{UserID: "user-1", UserStatus: "ACTIVE", ExpiresAt: now.Add(time.Hour)}}, cookie: true, status: http.StatusNoContent, actor: "user-1"},
		"missing":            {repository: fakeSessionRepository{}, status: http.StatusUnauthorized},
		"expired":            {repository: fakeSessionRepository{session: Session{UserID: "user-1", UserStatus: "ACTIVE", ExpiresAt: now}}, cookie: true, status: http.StatusUnauthorized},
		"revoked":            {repository: fakeSessionRepository{session: Session{UserID: "user-1", UserStatus: "ACTIVE", ExpiresAt: now.Add(time.Hour), RevokedAt: &revokedAt}}, cookie: true, status: http.StatusUnauthorized},
		"unknown":            {repository: fakeSessionRepository{err: ErrSessionNotFound}, cookie: true, status: http.StatusUnauthorized},
		"suspended":          {repository: fakeSessionRepository{session: Session{UserID: "user-1", UserStatus: "SUSPENDED", ExpiresAt: now.Add(time.Hour)}}, cookie: true, status: http.StatusUnauthorized},
		"banned":             {repository: fakeSessionRepository{session: Session{UserID: "user-1", UserStatus: "BANNED", ExpiresAt: now.Add(time.Hour)}}, cookie: true, status: http.StatusUnauthorized},
		"repository failure": {repository: fakeSessionRepository{err: errors.New("database unavailable")}, cookie: true, status: http.StatusServiceUnavailable},
	}
	for name, testCase := range tests {
		t.Run(name, func(t *testing.T) {
			middleware := SessionMiddleware{Repository: testCase.repository, Now: func() time.Time { return now }}
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				actor, _ := ActorID(r.Context())
				if actor != testCase.actor {
					t.Fatalf("actor = %q", actor)
				}
				w.WriteHeader(http.StatusNoContent)
			})
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/me/professional-profile", nil)
			if testCase.cookie {
				req.AddCookie(&http.Cookie{Name: "session", Value: "opaque-session-token"})
			}
			res := httptest.NewRecorder()
			middleware.RequireSession(next).ServeHTTP(res, req)
			if res.Code != testCase.status {
				t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
			}
		})
	}
}

func TestSessionMiddlewareRejectsCrossOriginWrite(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	middleware := SessionMiddleware{Repository: fakeSessionRepository{session: Session{UserID: "user-1", UserStatus: "ACTIVE", ExpiresAt: now.Add(time.Hour)}}, Now: func() time.Time { return now }}
	req := httptest.NewRequest(http.MethodPost, "https://market.example/api/v1/me/favorites/PROJECT/11111111-1111-4111-8111-111111111111", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.AddCookie(&http.Cookie{Name: "session", Value: "opaque-session-token"})
	res := httptest.NewRecorder()
	middleware.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })).ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestSessionSecurityAndCookiePrecedence(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	userRepo := fakeSessionRepository{session: Session{UserID: "user-1", UserStatus: "ACTIVE", ExpiresAt: now.Add(time.Hour)}}
	adminRepo := fakeSessionRepository{session: Session{UserID: "admin-1", UserStatus: "ACTIVE", ExpiresAt: now.Add(time.Hour)}}

	userMiddleware := SessionMiddleware{Repository: userRepo, CookieName: "session", Now: func() time.Time { return now }}
	uploadMiddleware := UploadSessionMiddleware{Repository: userRepo, UserCookie: "session", AdminCookie: "session_admin", Now: func() time.Time { return now }}

	t.Run("user middleware strictly rejects session_admin cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
		req.AddCookie(&http.Cookie{Name: "session_admin", Value: "admin-token"})
		res := httptest.NewRecorder()
		userMiddleware.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(res, req)
		if res.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized for admin cookie on user route, got %d", res.Code)
		}
	})

	t.Run("upload middleware accepts user cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/presign", nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: "user-token"})
		res := httptest.NewRecorder()
		var actor string
		uploadMiddleware.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor, _ = ActorID(r.Context())
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(res, req)
		if res.Code != http.StatusOK || actor != "user-1" {
			t.Fatalf("expected 200 and user-1, got code=%d, actor=%s", res.Code, actor)
		}
	})

	t.Run("upload middleware accepts admin cookie when user cookie absent", func(t *testing.T) {
		uploadWithAdminRepo := UploadSessionMiddleware{Repository: adminRepo, UserCookie: "session", AdminCookie: "session_admin", Now: func() time.Time { return now }}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/presign", nil)
		req.AddCookie(&http.Cookie{Name: "session_admin", Value: "admin-token"})
		res := httptest.NewRecorder()
		var actor string
		uploadWithAdminRepo.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor, _ = ActorID(r.Context())
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(res, req)
		if res.Code != http.StatusOK || actor != "admin-1" {
			t.Fatalf("expected 200 and admin-1, got code=%d, actor=%s", res.Code, actor)
		}
	})

	t.Run("cookie precedence: user cookie takes priority over admin cookie on user middleware", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: "user-token"})
		req.AddCookie(&http.Cookie{Name: "session_admin", Value: "admin-token"})
		res := httptest.NewRecorder()
		var actor string
		userMiddleware.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor, _ = ActorID(r.Context())
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(res, req)
		if res.Code != http.StatusOK || actor != "user-1" {
			t.Fatalf("expected 200 and user-1, got code=%d, actor=%s", res.Code, actor)
		}
	})
}
