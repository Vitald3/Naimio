-- Phase 11 completion: durable provider-token handoff, PRO concurrency guards,
-- and searchable payment operations. Opaque provider references only: never PAN/CVV.
ALTER TABLE payment_attempts
  ADD COLUMN IF NOT EXISTS provider_payment_method_ref text;

-- The URL is provider-hosted confirmation metadata, not financial authority.
-- Persisting it lets an idempotent Safe Deal funding command return the same
-- checkout without recreating a charge.
ALTER TABLE payment_records
  ADD COLUMN IF NOT EXISTS checkout_url text;

CREATE UNIQUE INDEX IF NOT EXISTS payment_attempts_one_open_pro_purchase
  ON payment_attempts(internal_reference_id)
  WHERE domain='PRO_SUBSCRIPTION'
    AND operation_type='PAYMENT'
    AND status IN('CREATED','PENDING_USER_ACTION','PROCESSING','AUTHORIZED','UNKNOWN_REQUIRES_RECONCILIATION');

CREATE INDEX IF NOT EXISTS payment_provider_events_attempt_idx
  ON payment_provider_events(payment_attempt_id,received_at DESC,id DESC);

CREATE INDEX IF NOT EXISTS subscription_billing_due_idx
  ON user_subscriptions(COALESCE(next_retry_at,current_period_end),id)
  WHERE status IN('ACTIVE','PAST_DUE') AND NOT cancel_at_period_end;

ALTER TABLE payout_recipient_bindings
  ADD COLUMN IF NOT EXISTS verification_reason text,
  ADD COLUMN IF NOT EXISTS verified_at timestamptz,
  ADD COLUMN IF NOT EXISTS verified_by uuid REFERENCES users(id);
