#!/bin/sh
set -eu
cd "$(dirname "$0")/../../apps/api"
go test ./internal/safedeal ./internal/proposals ./internal/projects ./internal/reviews ./internal/auth ./internal/platform/ratelimit ./cmd/api -count=1
grep -q 'proxy_set_header Host \$http_host' ../../infra/nginx/nginx.conf
cd ../web
npm test -- --test-name-pattern='Safe Deal|authorization|SEO|communication|proposal'
