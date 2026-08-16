#!/bin/sh
set -eu
cd "$(dirname "$0")/../.."
cd apps/api
go test ./internal/ai -run 'TestGuestBrief|TestInvalidStructured|TestTimeout|TestHTTP' -count=1
go test ./internal/projects ./internal/platform/ratelimit ./cmd/api -count=1
