#!/bin/sh
set -eu
cd "$(dirname "$0")/../../apps/api"
go test ./internal/ai ./internal/matching -run 'TestGuestBrief|TestInvalidStructured|TestPromptInjection|TestDeterministicSignals|TestRerank|TestManual|TestHTTP' -count=1
go test ./internal/projects ./internal/platform/ratelimit ./cmd/api -count=1
