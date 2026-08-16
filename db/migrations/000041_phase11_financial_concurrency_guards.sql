-- Phase 11 hardening: one in-flight financial command per Safe Deal operation.
-- Provider idempotency keys protect retries of the same command; these indexes
-- additionally prevent a second browser/tab from starting another external
-- charge/refund/payout with a different command key while the first result is
-- still non-authoritative.
CREATE UNIQUE INDEX IF NOT EXISTS payment_attempts_one_open_safe_deal_payment
  ON payment_attempts(internal_reference_id)
  WHERE domain='SAFE_DEAL'
    AND operation_type='PAYMENT'
    AND status IN('CREATED','PENDING_USER_ACTION','PROCESSING','AUTHORIZED','UNKNOWN_REQUIRES_RECONCILIATION');

CREATE UNIQUE INDEX IF NOT EXISTS payment_attempts_one_open_safe_deal_refund
  ON payment_attempts(internal_reference_id)
  WHERE domain='SAFE_DEAL'
    AND operation_type='REFUND'
    AND status IN('CREATED','PENDING_USER_ACTION','PROCESSING','AUTHORIZED','UNKNOWN_REQUIRES_RECONCILIATION');

CREATE UNIQUE INDEX IF NOT EXISTS payment_attempts_one_open_safe_deal_payout
  ON payment_attempts(internal_reference_id)
  WHERE domain='SAFE_DEAL'
    AND operation_type='PAYOUT'
    AND status IN('CREATED','PENDING_USER_ACTION','PROCESSING','AUTHORIZED','UNKNOWN_REQUIRES_RECONCILIATION');
