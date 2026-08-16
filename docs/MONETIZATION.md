# Monetization and PRO

Phase 9 introduces platform subscriptions as a domain separate from Safe Deal. PostgreSQL is authoritative for plans, entitlements, subscription periods and the append-only event history. Redis and frontend state never determine access.

## Effective access

`pro_subscriptions_enabled` is the global kill switch. When it is off, upgrade UI is hidden and PRO-only limits are not enforced, while plans, subscriptions and history are retained. When it is on, a subscription is effective only when its status is `ACTIVE`, its start is not in the future and its nullable end is still in the future.

Entitlements belong to a plan and use one of three value kinds: boolean, numeric, or unlimited. Server-side policy resolution is the only enforcement point. The initial integration enforces the portfolio item limit centrally; the same resolver exposes configuration for badge visibility, discovery eligibility and future supported benefits without scattering plan-name checks through business code. FREE retains the core marketplace.

## Lifecycle and administration

Supported lifecycle states are `PENDING`, `ACTIVE`, `CANCELED`, `EXPIRED` and `PAST_DUE`. Administrative grant, cancel and expire operations are authorized, reasoned and audited. Every lifecycle mutation appends a subscription event; external provider/customer/subscription references are nullable and are never required for manual grants.

The control center manages plan presentation, price and entitlement configuration, grants and lifecycle operations. Aggregates are derived from subscription records. Feature-flag changes preserve every record.

## Payment-provider boundary

`SubscriptionPaymentProvider` is the integration boundary for a future checkout and webhook adapter. The Phase 9 implementation deliberately installs a disabled provider that reports checkout as unavailable. The public PRO page therefore shows plan comparison and current status but cannot simulate payment, success, revenue or renewal.

A real provider integration must add an adapter behind that interface, verified and idempotent webhook ingestion, external-reference reconciliation, secret handling and provider-specific failure tests. It must not reuse Safe Deal payment state or callbacks.

## Content platform

The Blog domain is independently controlled by `blog_enabled`. Public list, article, sitemap and navigation discovery disappear when disabled; editorial data and staff CMS remain intact. Posts support draft, scheduled, published and archived states, categories, tags, cover/content media, canonical metadata and server-side HTML sanitization. Scheduled posts become publicly eligible from their UTC schedule time.

Staff media uploads reuse the existing scan/ownership pipeline with the dedicated `BLOG_COVER` and `BLOG_CONTENT` purposes. Published media delivery verifies that a public post references the object.

## Primary APIs

- Public: `/api/v1/monetization`, `/api/v1/blog`, `/api/v1/blog/{slug}`.
- Account: `/api/v1/me/subscription`.
- Staff: `/api/v1/admin/monetization/*`, `/api/v1/admin/blog/*`.

All routes use the existing `/api/v1` error envelope, authorization middleware and privacy-aware audit conventions.
