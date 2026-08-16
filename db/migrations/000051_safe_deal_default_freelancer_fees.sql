-- Default marketplace economics: both the Naimio service commission and the
-- payment-provider cost are deducted from the freelancer payout. Existing
-- Safe Deals keep their immutable snapshots; only the active untouched v1 rule
-- used for new deals is corrected here. Admin-created later versions are not
-- changed.
UPDATE safe_deal_fee_rules
SET platform_fee_payer_mode='FREELANCER',
    platform_customer_share_basis_points=0,
    provider_fee_payer_mode='FREELANCER',
    provider_customer_share_basis_points=0
WHERE version=1
  AND enabled=true;
