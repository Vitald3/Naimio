#!/bin/sh
set -eu
cd "$(dirname "$0")/../../apps/api"
go test ./internal/acquisition ./internal/ai ./cmd/api -count=1
cd ../web
npm test -- --test-name-pattern='SEO|calculator|analytics|structured data'
