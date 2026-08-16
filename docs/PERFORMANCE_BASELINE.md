# Performance baseline

## Historical measured capacity

Environment: local Docker Compose, 2026-08-14. This is a single-host baseline, not production capacity.

### Smoke — PASS

- Profile: 2 max VUs, 20 seconds; 19 requests.
- Error rate: 0%; p95 17.47 ms; p99 18.82 ms.

### Baseline attempt 1 — FAIL

- Profile: 25 max VUs, 1m50s; 1,428 requests; 12.92 req/s.
- Error rate: 9.03% (129/1,428), all 429 rate limits; no 5xx.
- Cause: restrictive public-read limit and proxy identity handling.

### Baseline attempt 2 — PASS

- Profile: 25 max VUs, 1m50s; 1,431 requests; 12.85 req/s.
- Error rate: 0%; p50 6.26 ms; p95 16.49 ms; p99 21.77 ms; max 37.51 ms.
- Resource snapshot: API 41.72 MiB, PostgreSQL 69.48 MiB, Redis 19.29 MiB, worker 8.07 MiB; PostgreSQL had 9 connections.

## Phase 12 rerun status

- `make load-smoke`: not runnable in the agent environment. k6 starts, but requests to `127.0.0.1:8088` fail with `connect: operation not permitted`; no reachable local server/port is available.
- `make load-baseline`: blocked by the same agent environment limitation.
- No new measurements are claimed.
- `vu100` and `vu1000` profiles remain available, but must target isolated infrastructure with mocked provider calls.

## Capacity model

Keep measured results separate from projections. A laptop Docker run cannot verify one million concurrent users. Initial SLO targets: availability 99.9%, API p95 <1s, p99 <2.5s, and 5xx <1%. Higher-scale validation requires distributed k6 against isolated staging and measured database/Redis/network throughput.
