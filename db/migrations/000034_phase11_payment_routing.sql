-- Phase 11: durable, non-secret payment routing.  Credentials deliberately
-- stay in deployment configuration; these rows only select a capable adapter
-- for new operations.
CREATE TABLE payment_provider_settings (
  provider varchar(80) PRIMARY KEY,
  enabled boolean NOT NULL DEFAULT false,
  environment varchar(16) NOT NULL DEFAULT 'sandbox' CHECK(environment IN('sandbox','production')),
  updated_by uuid REFERENCES users(id),
  updated_at timestamptz NOT NULL DEFAULT now()
);

-- Align the initial attempt schema with the public routing domain name.
ALTER TABLE payment_attempts DROP CONSTRAINT payment_attempts_domain_check;
ALTER TABLE payment_attempts ADD CONSTRAINT payment_attempts_domain_check
  CHECK(domain IN('SAFE_DEAL','PRO_SUBSCRIPTION','OTHER_PLATFORM_PAYMENT'));

CREATE TABLE payment_provider_routes (
  domain varchar(32) PRIMARY KEY CHECK(domain IN('SAFE_DEAL','PRO_SUBSCRIPTION','OTHER_PLATFORM_PAYMENT')),
  provider varchar(80) NOT NULL REFERENCES payment_provider_settings(provider),
  updated_by uuid REFERENCES users(id),
  updated_at timestamptz NOT NULL DEFAULT now()
);

-- A claim is lease based, so a worker crash cannot strand an attempt forever.
ALTER TABLE payment_attempts ADD COLUMN reconciliation_claimed_at timestamptz;
ALTER TABLE payment_attempts ADD COLUMN reconciliation_attempts integer NOT NULL DEFAULT 0 CHECK(reconciliation_attempts >= 0);
ALTER TABLE payment_attempts ADD COLUMN next_reconciliation_at timestamptz NOT NULL DEFAULT now();
CREATE INDEX payment_attempts_reconciliation_due_idx ON payment_attempts(next_reconciliation_at,id)
  WHERE reconciliation_state IN('REQUIRED','IN_PROGRESS');
