CREATE TABLE IF NOT EXISTS payment_provider_credentials (
  provider text PRIMARY KEY REFERENCES payment_provider_settings(provider) ON DELETE CASCADE,
  environment text NOT NULL CHECK (environment IN ('sandbox','production')),
  encrypted_config text NOT NULL,
  updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
);
REVOKE ALL ON payment_provider_credentials FROM PUBLIC;
