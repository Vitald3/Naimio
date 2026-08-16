#!/bin/sh
set -eu
cd "$(dirname "$0")/../../apps/api"
go test ./internal/jobs ./internal/services -run 'TestVacancy|TestValidation|TestHTTP|TestServiceValidation|TestServiceTransitions' -count=1
go test ./internal/auth ./internal/platform/ratelimit ./cmd/api -count=1
