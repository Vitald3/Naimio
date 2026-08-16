# Payment providers

Status: Phase 11 repository adapters and product integration. Capabilities below describe code that can be routed only when the deployment adapter is configured and the corresponding provider product is commercially activated.

| Provider | One-time | Recurring PRO | Safe Deal | Payout | Webhooks | Reconciliation |
| --- | --- | --- | --- | --- | --- | --- |
| YooKassa | implemented | merchant-managed, implemented | implemented/routable | Safe Deal payout implemented | authoritative merchant-status verification | implemented |
| T-Bank | implemented | recurrent/rebill implemented | nominal-account primitives implemented; route disabled pending contracted activation/integration semantics | nominal recipient/payout primitives implemented | verified notification token | implemented |
| CloudPayments | implemented | token recurring implemented | disabled unless contracted Escrow semantics are approved for Naimio | not used as Naimio Safe Deal payout | HMAC verification | implemented |
| Yandex Pay | implemented | subscription/recurring API support implemented | disabled | disabled for Safe Deal | ES256/JWKS | implemented |
| Robokassa | hosted checkout implemented | child recurring payment implemented | disabled; split payment is not Safe Deal | disabled for Safe Deal | signed ResultURL | authoritative query implemented |

## Operational invariants

- PSP credentials are managed through the staff payment-provider control center and encrypted at rest with the deployment-level `PAYMENT_CONFIG_MASTER_KEY` (or the existing protected Safe Deal secret as a compatibility fallback). Secret values are write-only in the admin API: they are never returned to the browser or logs.
- Production routing rejects sandbox providers. New attempts use the currently assigned capable route; existing attempts are permanently pinned to their provider.
- A browser redirect, query parameter or frontend callback never activates PRO or marks Safe Deal funded.
- Provider events are verified, persisted replay-safely, linked to their payment attempt and normalized before entering PRO/Safe Deal services.
- Reconciliation is claimed with PostgreSQL leases/`SKIP LOCKED`; timeout/429/5xx/unknown states do not trigger cross-provider retry or a second charge.
- All Naimio money is integer kopecks; decimal provider formats exist only at HTTP adapter boundaries.
- Provider adapters do not duplicate commission/SPLIT economics. The existing Safe Deal quote/state machine remains authoritative.
- Payout-recipient persistence stores opaque provider identifiers and safe summaries only. PAN/CVV/raw card credentials are forbidden.
- PRO initial purchase and renewal/recovery have database-level one-open-operation constraints in addition to idempotency keys.

## Provider notes

### YooKassa

Recurring charges use the saved payment-method reference only after provider-authoritative confirmation. Safe Deal uses the dedicated deal-linked payment/refund/payout API rather than card authorization holds. Production requires the separate platform Safe Deal/payout agreement.

### T-Bank

Acquiring and recurring/rebill are separate from the nominal-account marketplace product. The repository contains nominal deal/step/deponent/recipient/payout primitives and expects Bearer + merchant mTLS for that product. Beneficiary type/self-employed status belongs to provider onboarding/verification and must never be inferred by Naimio.

T-Bank acquiring and nominal-account credentials, including the mTLS certificate/private-key PEM, can be entered in the staff control center. Live Safe Deal routing remains disabled until the contracted nominal-account integration is activated and verified.

### CloudPayments

Status/refund/two-stage confirm/void and token recurring are implemented. CloudPayments Escrow is not automatically equivalent to Naimio Safe Deal; SAFE_DEAL remains fail-closed unless the merchant contract and legal settlement semantics are validated.

### Yandex Pay

Webhook payloads are parsed only after ES256 verification through the configured JWKS. Payment/refund/recurring support does not imply marketplace escrow, therefore SAFE_DEAL is false.

### Robokassa

ResultURL is signature-verified and authoritative status is queried server-side. Recurring child payments and refund API support are implemented. Split-payment capabilities are explicitly not treated as Safe Deal escrow.

## Official documentation used for Phase 11 adapter semantics

- YooKassa Safe Deal: https://yookassa.ru/developers/solutions-for-platforms/safe-deal/basics
- YooKassa Safe Deal payouts: https://yookassa.ru/developers/solutions-for-platforms/safe-deal/integration/payouts
- T-Bank nominal accounts / beneficiaries: https://developer.tbank.ru/docs/api/get-api-v-1-nominal-accounts-beneficiaries
- T-Bank beneficiary card-details request: https://developer.tbank.ru/docs/api/post-api-v-1-nominal-accounts-beneficiaries-beneficiaryid-add-card-requests
- Yandex Pay merchant API/webhooks: https://pay.yandex.ru/docs/ru/custom/backend/merchant-api/webhook
