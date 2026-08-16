-- Phase 11: persist provider-hosted confirmation URLs so idempotent checkout retries can resume safely.
ALTER TABLE payment_attempts ADD COLUMN IF NOT EXISTS provider_confirmation_url text;
