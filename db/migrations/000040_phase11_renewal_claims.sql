-- Phase 11: lease PRO renewals so horizontally scaled workers cannot double-call a PSP.
ALTER TABLE user_subscriptions
  ADD COLUMN IF NOT EXISTS renewal_claimed_at timestamptz;

CREATE INDEX IF NOT EXISTS user_subscriptions_renewal_claim_idx
  ON user_subscriptions(COALESCE(next_retry_at,current_period_end),renewal_claimed_at,id)
  WHERE status IN('ACTIVE','PAST_DUE') AND NOT cancel_at_period_end;
