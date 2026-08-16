-- Phase 11 hardening: keep the payment-attempt domain enum aligned with the
-- durable routing domain and the Go constant. 000033 used PLATFORM_PAYMENT,
-- while 000034+ and the application use OTHER_PLATFORM_PAYMENT.
-- Drop first so any legacy row can be normalized without violating the old check.
ALTER TABLE payment_attempts
  DROP CONSTRAINT IF EXISTS payment_attempts_domain_check;

UPDATE payment_attempts
SET domain='OTHER_PLATFORM_PAYMENT'
WHERE domain='PLATFORM_PAYMENT';

ALTER TABLE payment_attempts
  ADD CONSTRAINT payment_attempts_domain_check
  CHECK(domain IN('SAFE_DEAL','PRO_SUBSCRIPTION','OTHER_PLATFORM_PAYMENT'));


-- Manual PAST_DUE recovery and the renewal worker share the same operation.
-- Enforce distributed single-flight at the database layer so multiple tabs or
-- worker replicas cannot create concurrent recurring charges for one subscription.
CREATE UNIQUE INDEX IF NOT EXISTS payment_attempts_one_open_pro_renewal
  ON payment_attempts(internal_reference_id)
  WHERE domain='PRO_SUBSCRIPTION'
    AND operation_type='RENEWAL'
    AND status IN('CREATED','PENDING_USER_ACTION','PROCESSING','AUTHORIZED','UNKNOWN_REQUIRES_RECONCILIATION');
