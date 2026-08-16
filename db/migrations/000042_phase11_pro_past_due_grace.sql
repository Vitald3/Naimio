-- Phase 11: remember the first failed renewal so PAST_DUE cannot retry forever.
ALTER TABLE user_subscriptions
  ADD COLUMN IF NOT EXISTS past_due_since timestamptz;

CREATE INDEX IF NOT EXISTS subscription_past_due_expiry_idx
  ON user_subscriptions(past_due_since,id)
  WHERE status='PAST_DUE';
