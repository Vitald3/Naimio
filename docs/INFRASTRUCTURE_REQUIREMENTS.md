# Infrastructure requirements

| Tier | API/web/worker | Data and edge | Trigger to scale |
|---|---|---|---|
| STARTER | 2 API (1 vCPU/1–2 GB), 2 web (0.5 vCPU/1 GB), 1 worker | managed PostgreSQL 2 vCPU/4 GB, managed Redis, S3-compatible bucket/CDN | p95 >700 ms, DB CPU >60%, pool wait or worker lag |
| GROWTH | 4–8 API, 2–4 web, 2 workers | PostgreSQL HA 4–8 vCPU, Redis HA, CDN/object storage | sustained RPS, websocket count, storage or queue growth |
| HIGH SCALE | autoscaled API/web, partitioned worker groups | HA PostgreSQL with replicas/PITR, Redis cluster, CDN/WAF, managed metrics | load tests demonstrate a tier limit before increasing capacity |

Pool sizing is per replica: reserve database connections for migrations/admin and keep `replicas × DB_MAX_OPEN_CONNS` below the database budget. API state is PostgreSQL/Redis-backed, allowing horizontal API replicas. Worker operations are idempotent and outbox-backed, allowing additional workers without duplicate business effects. Static Next assets should use CDN caching; private media stays in object storage through signed URLs.
