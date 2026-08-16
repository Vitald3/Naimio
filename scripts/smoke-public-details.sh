#!/bin/sh
set -eu
BASE_URL="${BASE_URL:-http://localhost:8088}"
check() {
  path="$1"
  code="$(curl -sS -o /tmp/freelance-smoke-body -w '%{http_code}' "$BASE_URL$path")"
  if [ "$code" != "200" ]; then
    echo "FAIL $path -> HTTP $code"
    cat /tmp/freelance-smoke-body || true
    exit 1
  fi
  echo "PASS $path"
}
check "/api/v1/services/demo-service-1"
check "/services/demo-service-1"
check "/api/v1/services/demo-service-12"
check "/services/demo-service-12"
check "/api/v1/services/demo-service-13"
check "/services/demo-service-13"
check "/api/v1/vacancies/demo-vacancy-1"
check "/vacancies/demo-vacancy-1"
check "/api/v1/projects/demo-project-2"
check "/projects/demo-project-2"
echo "Public detail smoke PASS"
