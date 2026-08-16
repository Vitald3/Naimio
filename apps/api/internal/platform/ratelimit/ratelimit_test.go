package ratelimit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	values := map[string]string{"RATE_LIMIT_WRITE_STANDARD_LIMIT": "2", "RATE_LIMIT_WRITE_STANDARD_WINDOW": "30s"}
	config, err := LoadConfig(func(key string) string { return values[key] })
	if err != nil || config[WriteStandard].Limit != 2 || config[WriteStandard].Window != 30*time.Second {
		t.Fatalf("config = %#v, error = %v", config, err)
	}
	if _, err := LoadConfig(func(string) string { return "invalid" }); err == nil {
		t.Fatal("expected invalid configuration to fail")
	}
}

func TestMemoryLimiterLimitsAndIsolatesKeys(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	limiter := NewMemory(Config{WriteStandard: {Limit: 2, Window: time.Minute}}, func() time.Time { return now })
	for attempt := 0; attempt < 2; attempt++ {
		decision, err := limiter.Allow(context.Background(), WriteStandard, "user:1")
		if err != nil || !decision.Allowed {
			t.Fatalf("attempt %d decision = %#v, error = %v", attempt, decision, err)
		}
	}
	decision, _ := limiter.Allow(context.Background(), WriteStandard, "user:1")
	if decision.Allowed {
		t.Fatal("expected third request to be limited")
	}
	isolated, _ := limiter.Allow(context.Background(), WriteStandard, "user:2")
	if !isolated.Allowed {
		t.Fatal("second key was not isolated")
	}
}

func TestMiddlewareReturnsStandardRateLimitError(t *testing.T) {
	limiter := NewMemory(Config{WriteStandard: {Limit: 1, Window: time.Minute}}, nil)
	handler := Middleware{Limiter: limiter}.Limit(WriteStandard, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/me/professional-profile", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	handler.ServeHTTP(httptest.NewRecorder(), req)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusTooManyRequests || !strings.Contains(res.Body.String(), `"code":"RATE_LIMITED"`) || res.Header().Get("Retry-After") == "" {
		t.Fatalf("status = %d, headers = %#v, body = %s", res.Code, res.Header(), res.Body.String())
	}
}

func TestPublicAIUsesDedicatedConfigurableLimit(t *testing.T) {
	config, err := LoadConfig(func(key string) string {
		if key == "RATE_LIMIT_PUBLIC_AI_LIMIT" {
			return "1"
		}
		if key == "RATE_LIMIT_PUBLIC_AI_WINDOW" {
			return "1h"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := Middleware{Limiter: NewMemory(config, nil)}.Limit(PublicAI, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/ai/project-brief", nil)
	request.RemoteAddr = "192.0.2.10:1234"
	handler.ServeHTTP(httptest.NewRecorder(), request)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRequestKeyUsesProxySuppliedClientIP(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "172.20.0.3:8080"
	request.Header.Set("X-Forwarded-For", "203.0.113.9")
	if got, want := requestKey(request), "ip:203.0.113.9"; got != want {
		t.Fatalf("request key=%q want=%q", got, want)
	}
}
