# Production launch checklist

- [ ] EXTERNAL PRODUCTION LAUNCH REQUIREMENTS: DNS points `naimio.ru` and `www` to the edge; www redirects to canonical host.
- [ ] EXTERNAL PRODUCTION LAUNCH REQUIREMENTS: trusted TLS certificate is mounted as `fullchain.pem`/`privkey.pem`; renewal alert exists.
- [ ] MANUAL LEGAL REVIEW: terms, privacy, cookies, payment/refund, PRO renewal/cancellation, Safe Deal and commission disclosures.
- [ ] MANUAL: `.env.production` uses distinct production secrets, database, Redis, bucket and SMTP identity.
- [ ] AUTOMATED: `make staging-check`, clean migrations, repeat migrations and `make checkpoint-mvp` pass.
- [ ] AUTOMATED: staging backup succeeds and `make staging-restore-test` restores into a disposable database.
- [ ] MANUAL: monitoring, alert routing, JSON log retention and on-call ownership are configured.
- [ ] MANUAL: staging uses isolated database, Redis, bucket, SMTP sandbox and payment sandbox credentials.
- [ ] AUTOMATED: deployment readiness and public smoke checks pass; rollback to prior image has been exercised.
- [ ] MANUAL: provider sandbox/production activation is recorded per `docs/PAYMENT_PROVIDERS.md`; unavailable credentials remain external requirements.
- [ ] MANUAL: admin accounts/roles, provider routing, reconciliation and audit logs reviewed.
- [ ] AUTOMATED: staging is noindex; production canonical is `https://naimio.ru`; robots/sitemap/structured data reviewed.
- [ ] MANUAL: feature flags and payment kill switches match launch policy.
- [ ] MANUAL: rollback image and forward-fix migration compatibility are recorded.
- [ ] EXTERNAL PRODUCTION LAUNCH REQUIREMENTS: production SMTP/S3/monitoring accounts, merchant contracts, fiscal configuration and operator approval.

Do not mark this checklist complete from local code alone. Record each item as implemented, locally verified, staging verified, sandbox verified, external requirement, or not verified.
