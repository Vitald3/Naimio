#!/bin/sh
set -eu
cd "$(dirname "$0")/../../apps/api"
if [ -n "${DATABASE_URL:-}" ]; then
  go test -count=1 -tags=integration ./internal/auth ./internal/growth
else
  go test -tags=integration -run '^$' ./internal/auth ./internal/growth
fi
