#!/bin/sh
set -eu
cd "$(dirname "$0")/../../apps/api"
if [ -n "${DATABASE_URL:-}" ]; then
  go test -count=1 -tags=integration ./internal/catalog ./internal/profiles ./internal/portfolio ./internal/media ./internal/services ./internal/projects ./internal/proposals ./internal/favorites ./internal/reputation ./internal/reviews
else
  go test -tags=integration -run '^$' ./internal/catalog ./internal/profiles ./internal/portfolio ./internal/media ./internal/services ./internal/projects ./internal/proposals ./internal/favorites ./internal/reputation ./internal/reviews
fi
