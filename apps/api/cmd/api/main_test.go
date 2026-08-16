package main

import (
	"context"
	"errors"
	"freelance/apps/api/internal/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealth(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w := httptest.NewRecorder()
	health(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestReadinessChecksDependencies(t *testing.T) {
	for name, testCase := range map[string]struct {
		check  func(context.Context) error
		status int
	}{
		"ready":     {check: func(context.Context) error { return nil }, status: http.StatusOK},
		"not ready": {check: func(context.Context) error { return errors.New("dependency unavailable") }, status: http.StatusServiceUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			res := httptest.NewRecorder()
			readiness(testCase.check).ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
			if res.Code != testCase.status || (testCase.status != http.StatusOK && strings.Contains(res.Body.String(), "dependency unavailable")) {
				t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
			}
		})
	}
}
func TestRegisterMethod(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/register", nil)
	w := httptest.NewRecorder()
	(auth.Handler{}).Register(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestJSONContractAndRequestID(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w := httptest.NewRecorder()
	requestID(http.HandlerFunc(health)).ServeHTTP(w, r)
	if got := w.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
	if w.Header().Get("X-Request-ID") == "" {
		t.Fatal("request id is missing")
	}
}

func TestPhase8CachePoliciesStaySeparated(t *testing.T) {
	public := httptest.NewRecorder()
	publicCache(300, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })).ServeHTTP(public, httptest.NewRequest(http.MethodGet, "/public", nil))
	if got := public.Header().Get("Cache-Control"); !strings.Contains(got, "public") || !strings.Contains(got, "stale-while-revalidate") {
		t.Fatalf("public cache policy = %q", got)
	}
	private := httptest.NewRecorder()
	privateNoStore(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })).ServeHTTP(private, httptest.NewRequest(http.MethodGet, "/private", nil))
	if got := private.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("private cache policy = %q", got)
	}
}
