#!/bin/sh
set -eu
cd "$(dirname "$0")/../../apps/api"
go test -tags=integration ./internal/ai ./internal/matching ./internal/projects -run '^$'
