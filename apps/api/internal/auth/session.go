package auth

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrSessionNotFound = errors.New("session not found")

type Session struct {
	UserID     string
	UserStatus string
	ExpiresAt  time.Time
	RevokedAt  *time.Time
}

type SessionRepository interface {
	FindByTokenHash(context.Context, []byte) (Session, error)
}

type SessionMiddleware struct {
	Repository SessionRepository
	CookieName string
	Now        func() time.Time
}

// OptionalSession authenticates a valid cookie when present while allowing a
// guest request through. Invalid/revoked cookies are never treated as guests.
func (m SessionMiddleware) OptionalSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookieName := m.CookieName
		if cookieName == "" {
			cookieName = "session"
		}
		cookie, err := r.Cookie(cookieName)
		if errors.Is(err, http.ErrNoCookie) {
			next.ServeHTTP(w, r)
			return
		}
		if err != nil || cookie.Value == "" || len(cookie.Value) > 4096 || m.Repository == nil {
			writeAuthError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
			return
		}
		hash := sha256.Sum256([]byte(cookie.Value))
		session, err := m.Repository.FindByTokenHash(r.Context(), hash[:])
		if err != nil {
			writeAuthError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
			return
		}
		now := time.Now().UTC()
		if m.Now != nil {
			now = m.Now().UTC()
		}
		if session.UserID == "" || session.RevokedAt != nil || !session.ExpiresAt.After(now) || session.UserStatus != "ACTIVE" {
			writeAuthError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
			return
		}
		if unsafeMethod(r.Method) && !sameOrigin(r) {
			writeAuthError(w, r, http.StatusForbidden, "CSRF_REJECTED", "request origin is not allowed")
			return
		}
		next.ServeHTTP(w, r.WithContext(WithActorID(r.Context(), session.UserID)))
	})
}

func (m SessionMiddleware) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookieName := m.CookieName
		if cookieName == "" {
			cookieName = "session"
		}
		cookie, err := r.Cookie(cookieName)
		if err != nil || cookie.Value == "" || len(cookie.Value) > 4096 {
			writeAuthError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
			return
		}
		if m.Repository == nil {
			writeAuthError(w, r, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "authentication is temporarily unavailable")
			return
		}
		hash := sha256.Sum256([]byte(cookie.Value))
		session, err := m.Repository.FindByTokenHash(r.Context(), hash[:])
		if errors.Is(err, ErrSessionNotFound) {
			writeAuthError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
			return
		}
		if err != nil {
			writeAuthError(w, r, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "authentication is temporarily unavailable")
			return
		}
		now := time.Now().UTC()
		if m.Now != nil {
			now = m.Now().UTC()
		}
		if session.UserID == "" || session.RevokedAt != nil || !session.ExpiresAt.After(now) || session.UserStatus != "ACTIVE" {
			writeAuthError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
			return
		}
		if unsafeMethod(r.Method) && !sameOrigin(r) {
			writeAuthError(w, r, http.StatusForbidden, "CSRF_REJECTED", "request origin is not allowed")
			return
		}
		next.ServeHTTP(w, r.WithContext(WithActorID(r.Context(), session.UserID)))
	})
}

// UploadSessionMiddleware specifically allows either a user session or an admin session
// for file upload operations, while keeping regular user/admin endpoints strictly isolated.
type UploadSessionMiddleware struct {
	Repository  SessionRepository
	UserCookie  string
	AdminCookie string
	Now         func() time.Time
}

func (m UploadSessionMiddleware) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userCookieName := m.UserCookie
		if userCookieName == "" {
			userCookieName = "session"
		}
		adminCookieName := m.AdminCookie
		if adminCookieName == "" {
			adminCookieName = "session_admin"
		}
		isAdmin := false
		cookie, err := r.Cookie(userCookieName)
		if (errors.Is(err, http.ErrNoCookie) || (err == nil && cookie.Value == "")) {
			if adminCookie, adminErr := r.Cookie(adminCookieName); adminErr == nil && adminCookie.Value != "" {
				cookie = adminCookie
				err = nil
				isAdmin = true
			}
		}
		if err != nil || cookie.Value == "" || len(cookie.Value) > 4096 {
			writeAuthError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
			return
		}
		if m.Repository == nil {
			writeAuthError(w, r, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "authentication is temporarily unavailable")
			return
		}
		hash := sha256.Sum256([]byte(cookie.Value))
		session, err := m.Repository.FindByTokenHash(r.Context(), hash[:])
		if errors.Is(err, ErrSessionNotFound) {
			writeAuthError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
			return
		}
		if err != nil {
			writeAuthError(w, r, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "authentication is temporarily unavailable")
			return
		}
		now := time.Now().UTC()
		if m.Now != nil {
			now = m.Now().UTC()
		}
		if session.UserID == "" || session.RevokedAt != nil || !session.ExpiresAt.After(now) || session.UserStatus != "ACTIVE" {
			writeAuthError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
			return
		}
		if unsafeMethod(r.Method) && !sameOrigin(r) {
			writeAuthError(w, r, http.StatusForbidden, "CSRF_REJECTED", "request origin is not allowed")
			return
		}
		ctx := WithActorID(r.Context(), session.UserID)
		if isAdmin {
			ctx = WithAdminSession(ctx, true)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func unsafeMethod(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}
func sameOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}
	return strings.EqualFold(parsed.Host, r.Host)
}

func writeAuthError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	requestID := w.Header().Get("X-Request-ID")
	if requestID == "" {
		requestID = r.Header.Get("X-Request-ID")
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message, "request_id": requestID}})
}
