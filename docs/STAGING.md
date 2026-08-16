# Phase 12 staging runbook

## Isolated environment

Copy `.env.staging.example` to a secret-managed `.env.staging` outside version control. Use a separate PostgreSQL instance/volume, Redis instance, object-storage bucket, SMTP/capture account, session/payment secrets, and provider sandbox credentials. Never reuse production values. `scripts/staging-check.sh` rejects the production hostname and validates the compose rendering without printing secrets.

## DNS and HTTPS

Create an operator-managed DNS `A`/`AAAA` record for the chosen staging hostname (for example `staging.naimio.ru`) pointing to the staging ingress. Set `PUBLIC_BASE_URL` to that exact HTTPS origin. Mount a valid staging certificate/key in `TLS_CERTS_DIR`; use Let's Encrypt HTTP-01/DNS-01 or an externally managed certificate, with renewal handled by the operator and reload after renewal. The staging nginx config redirects HTTP, preserves forwarded client headers, supports websocket routes, applies upload limits and `noindex`.

## Deploy and smoke

```bash
make staging-check
make staging-deploy
make staging-smoke
```

Deployment builds the same Docker production targets (`api`, `worker`, `web`) and runs the authoritative migration container before readiness checks. The current single-host compose deployment is restart-based, not a zero-downtime guarantee; use a second stack/traffic switch for low downtime.

## Data and integrations

Use a realistic S3-compatible staging bucket with private-by-default policy, CORS for presigned uploads, versioning/lifecycle/orphan cleanup as supported, and provider-side backup/retention. Use a capture SMTP provider or allowlisted test mailbox. Configure public HTTPS PSP callbacks to the staging host; signatures remain enabled and duplicate/out-of-order delivery must be tested. Missing merchant credentials, activation contracts, mTLS, SMTP, S3, DNS, monitoring, or certificate accounts are external requirements, not implementation passes.

## Backup and restore

Set `BACKUP_DIR` to an off-host or mounted staging destination and run `make staging-backup`. Run `make staging-restore-test` against a disposable PostgreSQL database and record the output before launch approval. A dump without restore is not validated.

## Load and rollback

Run `make load-smoke` and `make load-baseline` with isolated test data and mocked provider calls for volume tests. Record VUs, RPS, p50/p95/p99, errors, CPU/RAM, DB pool, Redis, and worker behavior in `docs/PERFORMANCE_BASELINE.md`. Do not load-test PSP sandboxes. For rollback, deploy a known-bad image in staging, confirm readiness failure, restore the previous image, and use forward fixes for irreversible additive migrations.

## DNS/production launch requirements

Production still requires real DNS/TLS, merchant accounts and provider products, production SMTP/S3, monitoring/error tracking, fiscal/receipt configuration, legal review, and operator approval. Staging is `noindex`; production must set its own canonical `https://naimio.ru` configuration.
