#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"

MODE=local
DB_CONTAINER=""
TEST_NETWORK=""
DB_USER=freelance
DB_PASSWORD=freelance_test_local_only
DB_NAME=freelance_test

cleanup() {
  if [ "$MODE" = docker ]; then
    [ -z "$DB_CONTAINER" ] || docker rm -f "$DB_CONTAINER" >/dev/null 2>&1 || true
    [ -z "$TEST_NETWORK" ] || docker network rm "$TEST_NETWORK" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT INT TERM

if [ -n "${DATABASE_URL:-}" ]; then
  command -v psql >/dev/null 2>&1 || {
    echo "DATABASE_URL is set, but psql is not installed. Unset DATABASE_URL to let make test-db use an isolated Docker PostgreSQL." >&2
    exit 1
  }

  db_query() {
    psql "$DATABASE_URL" -X -v ON_ERROR_STOP=1 -Atqc "$1"
  }
  db_exec() {
    psql "$DATABASE_URL" -X -v ON_ERROR_STOP=1 -qc "$1"
  }
  db_file() {
    psql "$DATABASE_URL" -X -v ON_ERROR_STOP=1 -f "$1"
  }
else
  MODE=docker
  command -v docker >/dev/null 2>&1 || {
    echo "DATABASE_URL is not set and Docker is unavailable. Set DATABASE_URL to a disposable PostgreSQL database or install/start Docker Desktop." >&2
    exit 1
  }

  TEST_SUFFIX="$$"
  TEST_NETWORK="freelance-test-${TEST_SUFFIX}"
  DB_CONTAINER="freelance-test-postgres-${TEST_SUFFIX}"
  DATABASE_URL="postgres://${DB_USER}:${DB_PASSWORD}@${DB_CONTAINER}:5432/${DB_NAME}?sslmode=disable"
  export DATABASE_URL

  echo "Starting isolated PostgreSQL for DB/integration tests..."
  docker network create "$TEST_NETWORK" >/dev/null
  docker run -d --rm \
    --name "$DB_CONTAINER" \
    --network "$TEST_NETWORK" \
    -e POSTGRES_USER="$DB_USER" \
    -e POSTGRES_PASSWORD="$DB_PASSWORD" \
    -e POSTGRES_DB="$DB_NAME" \
    postgres:16-alpine >/dev/null

  attempts=0
  until docker exec "$DB_CONTAINER" pg_isready -U "$DB_USER" -d "$DB_NAME" >/dev/null 2>&1; do
    attempts=$((attempts + 1))
    if [ "$attempts" -ge 30 ]; then
      echo "Disposable PostgreSQL did not become ready." >&2
      docker logs "$DB_CONTAINER" >&2 || true
      exit 1
    fi
    sleep 1
  done

  db_query() {
    docker exec "$DB_CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" -X -v ON_ERROR_STOP=1 -Atqc "$1"
  }
  db_exec() {
    docker exec "$DB_CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" -X -v ON_ERROR_STOP=1 -qc "$1"
  }
  db_file() {
    docker exec -i "$DB_CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" -X -v ON_ERROR_STOP=1 < "$1"
  }
fi

assert_eq() {
  actual=$1
  expected=$2
  message=$3
  if [ "$actual" != "$expected" ]; then
    echo "DB CHECK FAILED: $message (expected '$expected', got '$actual')" >&2
    exit 1
  fi
}

assert_eq "$(db_query "SELECT count(*) FROM pg_tables WHERE schemaname = 'public'")" "0" "test database must start empty"

if [ "$MODE" = docker ]; then
  echo "Applying migrations through the production migration tracker..."
  docker run --rm \
    --network "$TEST_NETWORK" \
    -v "$ROOT/db/migrations:/migrations:ro" \
    -v "$ROOT/scripts/migrate.sh:/migrate.sh:ro" \
    -e PGHOST="$DB_CONTAINER" \
    -e PGUSER="$DB_USER" \
    -e PGDATABASE="$DB_NAME" \
    -e PGPASSWORD="$DB_PASSWORD" \
    -e MIGRATIONS_DIR=/migrations \
    postgres:16-alpine /migrate.sh

  # A second run proves migration idempotency/skipping.
  docker run --rm \
    --network "$TEST_NETWORK" \
    -v "$ROOT/db/migrations:/migrations:ro" \
    -v "$ROOT/scripts/migrate.sh:/migrate.sh:ro" \
    -e PGHOST="$DB_CONTAINER" \
    -e PGUSER="$DB_USER" \
    -e PGDATABASE="$DB_NAME" \
    -e PGPASSWORD="$DB_PASSWORD" \
    -e MIGRATIONS_DIR=/migrations \
    postgres:16-alpine /migrate.sh

  migration_file_count=$(find db/migrations -maxdepth 1 -type f -name '*.sql' | wc -l | tr -d ' ')
  assert_eq "$(db_query "SELECT count(*) FROM schema_migrations")" "$migration_file_count" "migration tracker count"
else
  for migration in db/migrations/*.sql; do
    echo "Applying $migration"
    db_file "$migration"
  done
fi

assert_eq "$(db_query "SELECT to_regclass('public.users') IS NOT NULL AND to_regclass('public.sessions') IS NOT NULL AND to_regclass('public.professional_profiles') IS NOT NULL AND to_regclass('public.portfolio_items') IS NOT NULL AND to_regclass('public.media_objects') IS NOT NULL AND to_regclass('public.portfolio_media') IS NOT NULL AND to_regclass('public.services') IS NOT NULL AND to_regclass('public.service_skills') IS NOT NULL AND to_regclass('public.service_media') IS NOT NULL AND to_regclass('public.education_service_details') IS NOT NULL AND to_regclass('public.projects') IS NOT NULL AND to_regclass('public.project_skills') IS NOT NULL AND to_regclass('public.project_media') IS NOT NULL AND to_regclass('public.proposals') IS NOT NULL AND to_regclass('public.project_assignments') IS NOT NULL AND to_regclass('public.favorites') IS NOT NULL AND to_regclass('public.external_reputations') IS NOT NULL AND to_regclass('public.reputation_verification_challenges') IS NOT NULL AND to_regclass('public.reviews') IS NOT NULL AND to_regclass('public.review_dimensions') IS NOT NULL AND to_regclass('public.user_trust_stats') IS NOT NULL AND to_regclass('public.reports') IS NOT NULL AND to_regclass('public.fraud_signals') IS NOT NULL AND to_regclass('public.conversations') IS NOT NULL AND to_regclass('public.messages') IS NOT NULL AND to_regclass('public.notifications') IS NOT NULL AND to_regclass('public.email_jobs') IS NOT NULL AND to_regclass('public.invites') IS NOT NULL AND to_regclass('public.referral_rules') IS NOT NULL AND to_regclass('public.referral_attributions') IS NOT NULL AND to_regclass('public.reward_ledger') IS NOT NULL AND to_regclass('public.customer_team_members') IS NOT NULL AND to_regclass('public.project_drafts') IS NOT NULL AND to_regclass('public.ai_requests') IS NOT NULL AND to_regclass('public.matching_runs') IS NOT NULL AND to_regclass('public.matching_candidates') IS NOT NULL AND to_regclass('public.manual_project_recommendations') IS NOT NULL AND to_regclass('public.matching_quality_events') IS NOT NULL AND to_regclass('public.companies') IS NOT NULL AND to_regclass('public.jobs') IS NOT NULL AND to_regclass('public.job_skills') IS NOT NULL AND to_regclass('public.job_applications') IS NOT NULL AND to_regclass('public.calculator_definitions') IS NOT NULL AND to_regclass('public.acquisition_events') IS NOT NULL AND to_regclass('public.safe_deals') IS NOT NULL AND to_regclass('public.safe_deal_events') IS NOT NULL AND to_regclass('public.payment_records') IS NOT NULL AND to_regclass('public.payment_events') IS NOT NULL AND to_regclass('public.safe_deal_disputes') IS NOT NULL AND to_regclass('public.safe_deal_dispute_evidence') IS NOT NULL")" "t" "required MVP tables"
assert_eq "$(db_query "SELECT count(*) FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'media_objects' AND column_name IN ('purpose', 'uploaded_at', 'updated_at')")" "3" "media columns"
assert_eq "$(db_query "SELECT count(*) FROM pg_constraint WHERE conname IN ('profile_languages_code_check', 'profile_languages_level_check')")" "2" "profile language checks"
assert_eq "$(db_query "SELECT count(*) FROM pg_indexes WHERE indexname IN ('proposals_one_accepted_per_project','assignments_one_active_per_project','favorites_user_created_idx')")" "3" "project/proposal indexes"
assert_eq "$(db_query "SELECT count(*) FROM pg_indexes WHERE indexname IN ('external_reputations_verified_identity_unique','reviews_reviewee_status_created_idx','reports_status_created_idx')")" "3" "reputation/review/report indexes"
assert_eq "$(db_query "SELECT count(*) FROM pg_indexes WHERE indexname IN ('messages_conversation_created_idx','conversation_members_user_idx','notifications_user_unread_idx')")" "3" "communication indexes"
assert_eq "$(db_query "SELECT count(*) FROM pg_indexes WHERE indexname IN ('invites_inviter_created_idx','referral_rules_active_idx','reward_ledger_user_created_idx','customer_team_freelancer_idx')")" "4" "growth indexes"
assert_eq "$(db_query "SELECT count(*) FROM pg_indexes WHERE indexname IN ('project_drafts_owner_updated_idx','project_drafts_expiry_idx','project_drafts_guest_token_unique','ai_requests_capability_created_idx','ai_requests_user_created_idx')")" "5" "AI indexes"
assert_eq "$(db_query "SELECT count(*) FROM pg_indexes WHERE indexname IN ('matching_runs_project_created_idx','matching_candidates_run_rank_idx','matching_candidates_freelancer_run_idx','matching_quality_project_event_idx')")" "4" "matching indexes"
assert_eq "$(db_query "SELECT count(*) FROM pg_indexes WHERE indexname IN ('jobs_status_published_idx','jobs_category_status_published_idx','jobs_search_vector_idx','job_applications_user_created_idx','job_applications_job_created_idx')")" "5" "jobs indexes"
assert_eq "$(db_query "SELECT count(*) FROM pg_indexes WHERE indexname IN ('calculator_definitions_enabled_slug_unique','acquisition_events_type_created_idx','acquisition_events_user_created_idx','acquisition_events_anonymous_created_idx')")" "4" "acquisition indexes"
assert_eq "$(db_query "SELECT count(*) FROM calculator_definitions WHERE enabled=true AND slug IN ('telegram-bot','landing-page','seo')")" "3" "default calculators"
assert_eq "$(db_query "SELECT count(*) FROM pg_indexes WHERE indexname IN ('safe_deals_assignment_unique','safe_deals_active_project_unique','payment_records_provider_payment_unique','safe_deal_disputes_open_unique')")" "4" "Safe Deal indexes"
assert_eq "$(db_query "SELECT count(*) FROM pg_trigger WHERE tgname='project_assignment_safe_deal' AND NOT tgisinternal")" "1" "Safe Deal assignment trigger"
assert_eq "$(db_query "SELECT commission_basis_points=1000 AND minimum_fee_kopecks=0 AND enabled FROM safe_deal_fee_rules WHERE version=1")" "t" "default Safe Deal fee rule"
assert_eq "$(db_query "SELECT count(*) FROM pg_trigger WHERE tgname='categories_depth_guard' AND NOT tgisinternal")" "1" "category depth guard"

db_exec "EXPLAIN SELECT id FROM services WHERE search_vector @@ websearch_to_tsquery('simple','api') AND status='ACTIVE'; EXPLAIN SELECT id FROM projects WHERE search_vector @@ websearch_to_tsquery('simple','api') AND status IN('OPEN','MATCHING'); EXPLAIN SELECT user_id FROM professional_profiles WHERE to_tsvector('simple',coalesce(professional_title,'')||' '||coalesce(bio,'')) @@ websearch_to_tsquery('simple','go'); EXPLAIN SELECT id FROM jobs WHERE search_vector @@ websearch_to_tsquery('simple','go') AND status='PUBLISHED';"

API_TEST_PACKAGES="./internal/auth ./internal/catalog ./internal/profiles ./internal/portfolio ./internal/media ./internal/services ./internal/projects ./internal/proposals ./internal/favorites ./internal/reputation ./internal/reviews ./internal/communication ./internal/growth ./internal/ai ./internal/matching ./internal/jobs ./internal/acquisition ./internal/safedeal"

if [ "$MODE" = local ]; then
  command -v go >/dev/null 2>&1 || { echo "go is required for integration tests" >&2; exit 1; }
  (cd apps/api && go test -tags=integration $API_TEST_PACKAGES)
  (cd worker && go test -tags=integration ./internal/notification)
else
  echo "Running API integration tests in an isolated Go container..."
  docker run --rm \
    --network "$TEST_NETWORK" \
    -v "$ROOT/apps/api:/src" \
    -w /src \
    -e DATABASE_URL="$DATABASE_URL" \
    golang:1.22-alpine \
    go test -tags=integration $API_TEST_PACKAGES

  echo "Running worker integration tests in an isolated Go container..."
  docker run --rm \
    --network "$TEST_NETWORK" \
    -v "$ROOT/worker:/src" \
    -w /src \
    -e DATABASE_URL="$DATABASE_URL" \
    golang:1.22-alpine \
    go test -tags=integration ./internal/notification
fi

echo "DB/integration verification PASS"
