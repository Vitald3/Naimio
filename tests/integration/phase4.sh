#!/bin/sh
set -eu
cd "$(dirname "$0")/../../apps/api"
if [ -n "${DATABASE_URL:-}" ]; then
  go test -count=1 -tags=integration ./internal/communication
  cd ../../worker
  go test -count=1 -tags=integration ./internal/notification
else
  go test -tags=integration -run '^$' ./internal/communication
  cd ../../worker
  go test -tags=integration -run '^$' ./internal/notification
fi
