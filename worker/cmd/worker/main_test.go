package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWorkerHealthEndpoints(t *testing.T) {
	server := newServer(":0")
	for _, path := range []string{"/health/live", "/health/ready"} {
		res := httptest.NewRecorder()
		server.Handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		if res.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, res.Code)
		}
	}
}
