-- Bootstrap an explicit estimated CARD acquiring cost for PSPs that already
-- have an admin-managed credential record but predate provider-pricing rows.
-- The payment_provider_settings table intentionally has no `configured`
-- column; configuration is represented by payment_provider_credentials.
-- Administrators should replace this 2% estimate with the contractual tariff
-- in "Экономика и комиссии". Existing deal snapshots remain versioned.
WITH configured AS (
  SELECT DISTINCT s.provider
  FROM payment_provider_settings s
  JOIN payment_provider_credentials c ON c.provider = s.provider
  WHERE c.encrypted_config <> ''
    AND s.provider IN ('yookassa','tbank','cloudpayments','yandex_pay','robokassa')
)
INSERT INTO safe_deal_provider_pricing(
  version,
  provider,
  payment_method,
  percent_basis_points,
  fixed_fee_kopecks,
  minimum_fee_kopecks,
  enabled,
  effective_from
)
SELECT 1,c.provider,'CARD',200,0,0,true,now()
FROM configured c
WHERE NOT EXISTS (
  SELECT 1
  FROM safe_deal_provider_pricing p
  WHERE p.provider=c.provider AND p.payment_method='CARD'
)
ON CONFLICT(provider,payment_method,version) DO NOTHING;
