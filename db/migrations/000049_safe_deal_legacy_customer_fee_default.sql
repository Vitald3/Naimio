-- Hardening: the original MVP fee row (version 1) predated the explicit payer
-- controls and was backfilled as FREELANCER in migration 000024. For untouched
-- installations that legacy default makes the customer quote equal the proposal
-- price even though a platform commission exists. Newer/admin-created versions
-- are never changed: their payer mode is an explicit business decision.
UPDATE safe_deal_fee_rules
SET platform_fee_payer_mode='CUSTOMER',
    platform_customer_share_basis_points=0
WHERE version=1
  AND enabled=true
  AND platform_fee_payer_mode='FREELANCER';
