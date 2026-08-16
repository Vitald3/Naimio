# External provisioning checklist

| Item | Why | Value/secret needed | Configure | Verify |
|---|---|---|---|---|
| Staging DNS | Public callbacks/browser QA | staging hostname and A/AAAA target | `PUBLIC_BASE_URL` | HTTPS resolves to staging |
| TLS | HTTPS/webhooks | certificate/key or ACME account | `TLS_CERTS_DIR` | `curl -I http://host` redirects; TLS inspection |
| PostgreSQL | isolated durable data | URL/password or staging host | `DATABASE_URL`, Compose env | clean/repeat migrations and readiness |
| Redis | isolated cache/rate limits | URL/password | `REDIS_URL`, `REDIS_PASSWORD` | authenticated `PING`, readiness |
| Object storage | durable user media | endpoint, region, bucket, access keys | `OBJECT_STORAGE_*` | presign/upload/read/delete smoke |
| SMTP capture | safe transactional email | host, port, credentials, sender | `SMTP_*` | allowlisted capture mailbox |
| YooKassa | PRO/Safe Deal sandbox | shop ID, secret, activation/webhook | `YOOKASSA_*` | provider smoke and signed webhook |
| T-Bank | acquiring/recurring tests | terminal/password, mTLS/nominal contract | `TBANK_*` | signed notification/status smoke |
| CloudPayments | acquiring/recurring tests | public ID/API secret | `CLOUDPAYMENTS_*` | payment/status/refund smoke |
| Yandex Pay | payment tests | merchant/API/JWKS configuration | `YANDEX_PAY_*` | JWT webhook and status smoke |
| Robokassa | checkout tests | login/passwords | `ROBOKASSA_*` | Test/ResultURL signature smoke |
| Monitoring | alerts and retention | Prometheus/Grafana or managed endpoint | operator deployment | scrape, dashboard, alert delivery |
| Off-host backup | disaster recovery | encrypted destination credentials | `BACKUP_DIR`/backup job | restore into disposable DB |

Credentials are never committed. Missing provider/account access is reported as `SKIPPED — credentials unavailable` and is not a repository defect.
