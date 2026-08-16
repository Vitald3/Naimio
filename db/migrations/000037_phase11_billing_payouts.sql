-- Phase 11: connect provider-neutral payment attempts to PRO billing and payout binding.
ALTER TABLE user_subscriptions
  ADD COLUMN IF NOT EXISTS payment_method_ref text,
  ADD COLUMN IF NOT EXISTS last_payment_attempt_id uuid REFERENCES payment_attempts(id),
  ADD COLUMN IF NOT EXISTS next_retry_at timestamptz;

CREATE TABLE IF NOT EXISTS subscription_billing_periods (
  subscription_id uuid NOT NULL REFERENCES user_subscriptions(id) ON DELETE CASCADE,
  period_start timestamptz NOT NULL,
  period_end timestamptz NOT NULL,
  payment_attempt_id uuid REFERENCES payment_attempts(id),
  status varchar(24) NOT NULL CHECK(status IN('PENDING','SUCCEEDED','FAILED')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(subscription_id,period_start),
  CHECK(period_end>period_start)
);
CREATE UNIQUE INDEX IF NOT EXISTS subscription_billing_period_attempt_unique
  ON subscription_billing_periods(payment_attempt_id) WHERE payment_attempt_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS payout_recipient_bindings (
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider varchar(80) NOT NULL,
  external_reference text,
  status varchar(32) NOT NULL DEFAULT 'NOT_CONFIGURED' CHECK(status IN('NOT_CONFIGURED','PENDING_VERIFICATION','VERIFIED','REJECTED','DISABLED')),
  safe_details jsonb NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(safe_details)='object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(user_id,provider),
  CHECK(external_reference IS NULL OR char_length(external_reference)<=500)
);
CREATE INDEX IF NOT EXISTS payout_recipient_bindings_provider_status_idx
  ON payout_recipient_bindings(provider,status,updated_at DESC);

CREATE TABLE IF NOT EXISTS safe_deal_provider_refs (
  deal_id uuid PRIMARY KEY REFERENCES safe_deals(id) ON DELETE CASCADE,
  provider varchar(80) NOT NULL,
  provider_deal_id varchar(200),
  provider_payment_id varchar(200),
  provider_payout_id varchar(200),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
