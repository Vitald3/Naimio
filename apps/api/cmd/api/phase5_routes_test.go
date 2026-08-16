package main

import (
	"os"
	"strings"
	"testing"
)

func TestPhase5RoutesUseSharedSecurityControls(t *testing.T) {
	raw, e := os.ReadFile("main.go")
	if e != nil {
		t.Fatal(e)
	}
	source := string(raw)
	for _, required := range []string{`ratelimit.AuthStrict`, `protectWrite(http.HandlerFunc(growthHandler.Invites))`, `protectWrite(http.HandlerFunc(growthHandler.PublicInvite))`, `protectAdmin(http.HandlerFunc(growthHandler.Rules))`, `/api/v1/me/customer-team`} {
		if !strings.Contains(source, required) {
			t.Fatalf("missing Phase 5 security wiring %q", required)
		}
	}
}
