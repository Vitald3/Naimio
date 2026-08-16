#!/bin/sh
set -eu

: "${PGHOST:?PGHOST is required}"
: "${PGUSER:?PGUSER is required}"
: "${PGDATABASE:?PGDATABASE is required}"
: "${PGPASSWORD:?PGPASSWORD is required}"

MIGRATIONS_DIR=${MIGRATIONS_DIR:-/migrations}

psql_cmd() {
  psql -X -v ON_ERROR_STOP=1 -h "$PGHOST" -U "$PGUSER" -d "$PGDATABASE" "$@"
}

# Migration versions come from repository-controlled filenames. Restrict them to
# a conservative character set so they can be embedded in SQL literals without
# depending on psql variable interpolation (which is not reliable in every -c
# invocation/build combination).
validate_version() {
  case "$1" in
    ''|*[!0-9A-Za-z._-]*)
      echo "ERROR: unsafe migration filename/version: $1" >&2
      exit 1
      ;;
  esac
}

psql_cmd -qc "CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())"

# Compatibility with databases created before migration tracking was introduced.
# If the schema clearly contains the latest known MVP/admin migration, baseline it
# instead of replaying non-idempotent historical CREATE TABLE statements.
applied_count=$(psql_cmd -Atqc "SELECT count(*) FROM schema_migrations")
has_users=$(psql_cmd -Atqc "SELECT to_regclass('public.users') IS NOT NULL")
if [ "$applied_count" = "0" ] && [ "$has_users" = "t" ]; then
  latest_present=$(psql_cmd -Atqc "SELECT
    to_regclass('public.safe_deals') IS NOT NULL
    AND to_regclass('public.calculator_definitions') IS NOT NULL
    AND to_regclass('public.jobs') IS NOT NULL
    AND EXISTS (
      SELECT 1 FROM information_schema.columns
      WHERE table_schema='public' AND table_name='feature_flags' AND column_name='description'
    )")
  if [ "$latest_present" = "t" ]; then
    echo "Existing fully-migrated legacy schema detected; recording migration baseline."
    for migration in "$MIGRATIONS_DIR"/*.sql; do
      version=$(basename "$migration")
      validate_version "$version"
      psql_cmd -qc "INSERT INTO schema_migrations(version) VALUES ('$version') ON CONFLICT DO NOTHING"
    done
    exit 0
  fi

  echo "ERROR: existing database has an untracked partial schema." >&2
  echo "Refusing to replay historical migrations because some are intentionally non-idempotent." >&2
  echo "For disposable local data, run 'make dev-reset' and then 'make dev'." >&2
  echo "For valuable data, back it up and baseline/migrate it explicitly before continuing." >&2
  exit 1
fi

for migration in "$MIGRATIONS_DIR"/*.sql; do
  version=$(basename "$migration")
  validate_version "$version"
  already=$(psql_cmd -Atqc "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version='$version')")
  if [ "$already" = "t" ]; then
    echo "Skipping already applied migration: $version"
    continue
  fi

  echo "Applying migration: $version"
  # All current repository migrations are transaction-safe (no CREATE INDEX CONCURRENTLY etc.).
  psql_cmd -1 -f "$migration" -c "INSERT INTO schema_migrations(version) VALUES ('$version')"
done

echo "Migrations are up to date."
