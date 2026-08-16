#!/bin/sh
set -eu
cd "$(dirname "$0")/../../apps/api"
go test -tags=integration ./internal/safedeal ./internal/proposals ./internal/projects ./internal/reviews -run '^$'
