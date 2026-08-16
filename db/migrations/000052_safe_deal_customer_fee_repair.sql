-- Repair the short-lived debug default introduced by 000051. Naimio's
-- untouched v1 Safe Deal policy charges the customer the platform/provider
-- fees on top of the agreed work amount. Later admin-created fee versions are
-- left unchanged.
UPDATE safe_deal_fee_rules
SET platform_fee_payer_mode='CUSTOMER',
    platform_customer_share_basis_points=0,
    provider_fee_payer_mode='CUSTOMER',
    provider_customer_share_basis_points=0
WHERE version=1
  AND enabled=true;

-- Repair only still-unfunded deals that are safe to change: no real PSP
-- operation exists, or the only payment record is the local debug sandbox.
-- Funded/terminal deals and real-provider attempts keep their immutable
-- economic snapshots.
WITH repairable AS (
  SELECT d.id
  FROM safe_deals d
  WHERE d.fee_rule_version=1
    AND d.status='AWAITING_FUNDING'
    AND d.funded_at IS NULL
    AND NOT EXISTS (
      SELECT 1
      FROM payment_records pr
      WHERE pr.deal_id=d.id
        AND pr.provider <> 'sandbox'
    )
), repaired AS (
  UPDATE safe_deals d
  SET platform_fee_customer_kopecks=d.platform_fee_kopecks,
      platform_fee_freelancer_kopecks=0,
      platform_fee_platform_kopecks=0,
      provider_fee_customer_kopecks=d.provider_fee_kopecks,
      provider_fee_freelancer_kopecks=0,
      provider_fee_platform_kopecks=0,
      platform_fee_payer_mode='CUSTOMER',
      platform_customer_share_basis_points=0,
      provider_fee_payer_mode='CUSTOMER',
      provider_customer_share_basis_points=0,
      gross_amount_kopecks=d.work_amount_kopecks+d.platform_fee_kopecks+d.provider_fee_kopecks-d.customer_discount_kopecks,
      freelancer_amount_kopecks=d.work_amount_kopecks+d.freelancer_bonus_kopecks,
      platform_provider_cost_kopecks=0,
      platform_subsidy_kopecks=d.customer_discount_kopecks+d.freelancer_bonus_kopecks,
      platform_net_revenue_kopecks=d.platform_fee_kopecks-d.customer_discount_kopecks-d.freelancer_bonus_kopecks,
      updated_at=now()
  FROM repairable r
  WHERE d.id=r.id
  RETURNING d.id,d.gross_amount_kopecks
)
UPDATE payment_records pr
SET amount_kopecks=r.gross_amount_kopecks,
    updated_at=now()
FROM repaired r
WHERE pr.deal_id=r.id
  AND pr.provider='sandbox'
  AND pr.provider_status IN ('PENDING','CREATED');
