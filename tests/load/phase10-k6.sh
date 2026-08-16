#!/usr/bin/env sh
set -eu
command -v k6 >/dev/null 2>&1 || { echo 'k6 is required; install it and run make load-smoke' >&2; exit 2; }
: "${BASE_URL:=http://127.0.0.1:8088}"
case "$BASE_URL" in *naimio.ru*) [ "${ALLOW_PRODUCTION_LOAD_TEST:-}" = "yes" ] || { echo 'refusing production target without ALLOW_PRODUCTION_LOAD_TEST=yes' >&2; exit 2; };; esac
k6 run -e BASE_URL="$BASE_URL" -e LOAD_PROFILE="${LOAD_PROFILE:=smoke}" "$(dirname "$0")/phase10-k6.js"
