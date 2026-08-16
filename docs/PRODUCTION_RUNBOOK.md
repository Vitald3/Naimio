# Production runbook

## Deploy and rollback

Build immutable images in CI, run `make production-check`, then run migrations exactly once before replacing API/worker replicas. Wait for `/health/ready`, run public smoke checks, and retain the previous image digest. Roll back application images only when migrations remain backward-compatible; otherwise disable the relevant feature flag and restore from a tested backup rather than applying ad-hoc down migrations.

## Incidents

- API outage/high 5xx: inspect JSON request logs by request ID, readiness and database pool usage; roll back the last image if correlated with deployment.
- Database outage: stop writes by removing unhealthy API replicas, restore service, then confirm migration state and `/health/ready`.
- Redis outage: rate limiting/readiness fail closed in production; restore Redis, do not treat it as a source of truth.
- Worker outage: scale/restart workers; outbox jobs are idempotent and retryable. Investigate poison jobs after maximum attempts rather than looping forever.
- Failed migration: stop deployment, preserve migration output, restore a compatible application image. Use forward recovery or tested backup restore for irreversible schema changes.
- Backup restore: use `BACKUP_FILE=... RESTORE_DATABASE_URL=... make backup-test` only with a disposable database.
- Object storage incident: stop accepting uploads, preserve metadata in PostgreSQL, verify bucket versioning/lifecycle and provider incident status.
- Future payment-provider outage: keep `SAFE_DEAL_PROVIDER=disabled`/payment flag off; never fabricate a provider success. Reconcile verified provider events after recovery.
- Security incident: revoke sessions, rotate affected secrets outside the repository, preserve audit/log evidence, and assess notification obligations.

## Staging verification

Run `make staging-check` with a secret-managed `.env.staging`, then `make staging-deploy` and `make staging-smoke`. The overlay uses production image builds but isolated PostgreSQL, Redis and media volumes. Run `make staging-backup` followed by `BACKUP_FILE=... RESTORE_DATABASE_URL=... make staging-restore-test`. Run `make load-smoke` and `make load-baseline` only against isolated test data; provider calls must be mocked for volume tests. Staging is noindex and must never use the production hostname or credentials.

## Monitoring and alerts

Alert on sustained 5xx >1%, p95 latency above 1s, failed readiness, DB pool exhaustion, worker failure spikes, Redis errors, disk pressure, backup/restore failure, stale reconciliation backlog, webhook verification spikes, Safe Deal payout failures, PRO renewal failures and certificate expiry. Alerts require an owner, threshold, duration and runbook action. Use managed Prometheus/Grafana or equivalent; monitoring containers are intentionally not a production dependency.

## Data protection

Use encrypted off-host PostgreSQL backups (daily full plus point-in-time/WAL where supported), retain at least 35 daily and 12 monthly recovery points, and test restoration monthly. Object storage must use provider durability, versioning, lifecycle and orphan cleanup; Docker volumes are not production media backups.
