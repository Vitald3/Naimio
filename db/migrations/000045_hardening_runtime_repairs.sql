-- Runtime repair migration for admin-managed PSP configuration. Safe to run on
-- databases that already applied the original Phase 11 credential migration.
CREATE TABLE IF NOT EXISTS payment_provider_credentials (
  provider text PRIMARY KEY REFERENCES payment_provider_settings(provider) ON DELETE CASCADE,
  environment text NOT NULL CHECK (environment IN ('sandbox','production')),
  encrypted_config text NOT NULL,
  updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
);
REVOKE ALL ON payment_provider_credentials FROM PUBLIC;

INSERT INTO payment_provider_settings(provider,enabled,environment)
VALUES ('yookassa',false,'sandbox'),('tbank',false,'sandbox'),('yandex_pay',false,'sandbox'),
       ('cloudpayments',false,'sandbox'),('robokassa',false,'sandbox')
ON CONFLICT(provider) DO NOTHING;

CREATE INDEX IF NOT EXISTS payment_provider_credentials_updated_at_idx
  ON payment_provider_credentials(updated_at DESC);
