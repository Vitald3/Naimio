-- Phase 11: provider-neutral trace of every externally initiated operation.
-- Safe Deal's existing payment_records remain authoritative for its state machine;
-- this table also covers PRO and future platform charges without duplicating a ledger.
CREATE TABLE payment_attempts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  domain varchar(32) NOT NULL CHECK(domain IN('SAFE_DEAL','PRO_SUBSCRIPTION','PLATFORM_PAYMENT')),
  internal_reference_id uuid NOT NULL,
  provider varchar(80) NOT NULL CHECK(provider <> ''),
  operation_type varchar(32) NOT NULL CHECK(operation_type IN('PAYMENT','RENEWAL','REFUND','PAYOUT','CAPTURE','VOID')),
  status varchar(48) NOT NULL CHECK(status IN('CREATED','PENDING_USER_ACTION','PROCESSING','AUTHORIZED','SUCCEEDED','FAILED','CANCELED','REFUNDED','PARTIALLY_REFUNDED','UNKNOWN_REQUIRES_RECONCILIATION')),
  amount_kopecks bigint NOT NULL CHECK(amount_kopecks > 0),
  currency char(3) NOT NULL CHECK(currency='RUB'),
  idempotency_key varchar(160) NOT NULL,
  payment_method varchar(32),
  provider_operation_id varchar(200),
  provider_raw_status varchar(120),
  error_category varchar(64),
  reconciliation_state varchar(32) NOT NULL DEFAULT 'NOT_REQUIRED' CHECK(reconciliation_state IN('NOT_REQUIRED','REQUIRED','IN_PROGRESS','RECONCILED','FAILED')),
  terminal_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK((status IN('SUCCEEDED','FAILED','CANCELED','REFUNDED','PARTIALLY_REFUNDED') AND terminal_at IS NOT NULL) OR
        (status NOT IN('SUCCEEDED','FAILED','CANCELED','REFUNDED','PARTIALLY_REFUNDED')))
);
CREATE UNIQUE INDEX payment_attempts_business_idempotency_unique ON payment_attempts(domain,internal_reference_id,operation_type,idempotency_key);
CREATE UNIQUE INDEX payment_attempts_provider_operation_unique ON payment_attempts(provider,provider_operation_id) WHERE provider_operation_id IS NOT NULL;
CREATE INDEX payment_attempts_reconciliation_idx ON payment_attempts(updated_at,id) WHERE reconciliation_state IN('REQUIRED','IN_PROGRESS');
CREATE INDEX payment_attempts_admin_filter_idx ON payment_attempts(domain,provider,status,created_at DESC,id DESC);

CREATE TABLE payment_provider_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  provider varchar(80) NOT NULL,
  provider_event_id varchar(200) NOT NULL,
  payment_attempt_id uuid REFERENCES payment_attempts(id),
  event_type varchar(120) NOT NULL,
  verified_at timestamptz,
  processed_at timestamptz,
  attempts int NOT NULL DEFAULT 0 CHECK(attempts >= 0),
  processing_result varchar(64),
  external_reference varchar(200),
  payload_hash char(64) NOT NULL,
  received_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(provider,provider_event_id)
);
CREATE INDEX payment_provider_events_pending_idx ON payment_provider_events(received_at,id) WHERE processed_at IS NULL;
