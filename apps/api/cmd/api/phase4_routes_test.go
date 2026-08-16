package main

import (
	"os"
	"strings"
	"testing"
)

func TestPhase4RoutesUseSessionAndNamedChatRateLimit(t *testing.T) {
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{`ratelimit.ChatSend`, `mux.Handle("/api/v1/ws", sessionMiddleware.RequireSession(realtimeHandler))`, `/api/v1/conversations`, `/api/v1/notifications`, `/api/v1/notification-preferences`} {
		if !strings.Contains(source, required) {
			t.Fatalf("missing Phase 4 route control %q", required)
		}
	}
}
