-- Phase 11 hardening: Safe Deal provider-cost pricing must follow the PSP selected
-- for new SAFE_DEAL operations. Existing deals remain unchanged because economics
-- are snapshotted on deal creation.

CREATE OR REPLACE FUNCTION create_safe_deal_for_assignment() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  fee_rule safe_deal_fee_rules%ROWTYPE;
  pricing  safe_deal_provider_pricing%ROWTYPE;
  work bigint; plat_fee bigint; prov_fee bigint;
  plat_cust bigint; plat_free bigint; plat_plat bigint;
  prov_cust bigint; prov_free bigint; prov_plat bigint;
  discount bigint := 0; bonus bigint := 0;
  customer_total bigint; freelancer_payout bigint;
  gross_revenue bigint; provider_cost bigint; subsidy bigint; net_revenue bigint;
  pricing_version int := 1; has_pricing boolean := false;
  customer uuid; proposal jsonb; project jsonb;
  routed_provider text := 'sandbox';
BEGIN
  IF NEW.agreed_price_kopecks IS NULL OR NEW.agreed_price_kopecks<=0 THEN
    RAISE EXCEPTION 'accepted proposal requires agreed price' USING ERRCODE='23514';
  END IF;
  work := NEW.agreed_price_kopecks;

  SELECT * INTO fee_rule FROM safe_deal_fee_rules WHERE enabled AND effective_from<=now() ORDER BY effective_from DESC LIMIT 1;
  IF NOT FOUND THEN RAISE EXCEPTION 'safe deal fee rule missing'; END IF;

  -- Estimated provider pricing follows the current SAFE_DEAL route. Absent pricing => zero.
  SELECT provider INTO routed_provider FROM payment_provider_routes WHERE domain='SAFE_DEAL';
  IF routed_provider IS NULL OR routed_provider='' THEN routed_provider := 'sandbox'; END IF;
  SELECT * INTO pricing FROM safe_deal_provider_pricing
    WHERE enabled AND provider=routed_provider AND payment_method='CARD' AND effective_from<=now()
    ORDER BY effective_from DESC LIMIT 1;
  has_pricing := FOUND;

  -- Platform commission: percent, clamped to [min,max].
  plat_fee := (work*fee_rule.commission_basis_points)/10000;
  IF plat_fee<fee_rule.minimum_fee_kopecks THEN plat_fee:=fee_rule.minimum_fee_kopecks; END IF;
  IF fee_rule.maximum_fee_kopecks IS NOT NULL AND plat_fee>fee_rule.maximum_fee_kopecks THEN plat_fee:=fee_rule.maximum_fee_kopecks; END IF;

  -- Provider cost: percent + fixed, clamped to [min,max].
  IF has_pricing THEN
    prov_fee := (work*pricing.percent_basis_points)/10000 + pricing.fixed_fee_kopecks;
    IF prov_fee<pricing.minimum_fee_kopecks THEN prov_fee:=pricing.minimum_fee_kopecks; END IF;
    IF pricing.maximum_fee_kopecks IS NOT NULL AND prov_fee>pricing.maximum_fee_kopecks THEN prov_fee:=pricing.maximum_fee_kopecks; END IF;
    pricing_version := pricing.version;
  ELSE
    prov_fee := 0;
  END IF;

  -- Allocate the platform commission by its payer mode (SPLIT remainder -> freelancer).
  IF fee_rule.platform_fee_payer_mode='CUSTOMER' THEN
    plat_cust:=plat_fee; plat_free:=0; plat_plat:=0;
  ELSIF fee_rule.platform_fee_payer_mode='FREELANCER' THEN
    plat_cust:=0; plat_free:=plat_fee; plat_plat:=0;
  ELSIF fee_rule.platform_fee_payer_mode='PLATFORM' THEN
    plat_cust:=0; plat_free:=0; plat_plat:=plat_fee;
  ELSE -- SPLIT
    plat_cust:=(plat_fee*fee_rule.platform_customer_share_basis_points)/10000; plat_free:=plat_fee-plat_cust; plat_plat:=0;
  END IF;

  -- Allocate the provider cost by its payer mode.
  IF fee_rule.provider_fee_payer_mode='CUSTOMER' THEN
    prov_cust:=prov_fee; prov_free:=0; prov_plat:=0;
  ELSIF fee_rule.provider_fee_payer_mode='FREELANCER' THEN
    prov_cust:=0; prov_free:=prov_fee; prov_plat:=0;
  ELSIF fee_rule.provider_fee_payer_mode='PLATFORM' THEN
    prov_cust:=0; prov_free:=0; prov_plat:=prov_fee;
  ELSE -- SPLIT
    prov_cust:=(prov_fee*fee_rule.provider_customer_share_basis_points)/10000; prov_free:=prov_fee-prov_cust; prov_plat:=0;
  END IF;

  customer_total    := work + plat_cust + prov_cust - discount;
  freelancer_payout := work - plat_free - prov_free + bonus;
  IF customer_total<=0 OR freelancer_payout<=0 THEN
    RAISE EXCEPTION 'safe deal economics leave non-positive customer total or payout' USING ERRCODE='23514';
  END IF;

  gross_revenue := plat_fee;
  provider_cost := prov_plat;
  subsidy       := plat_plat + prov_plat + discount + bonus;
  net_revenue   := customer_total - prov_fee - freelancer_payout;

  SELECT p.customer_user_id,
         jsonb_build_object('title',p.title,'description',p.description,'deadline_at',p.deadline_at,'budget_type',p.budget_type),
         jsonb_build_object('proposal_id',pr.id,'message',pr.message,'price_kopecks',pr.price_kopecks,'currency',pr.currency,'delivery_days',pr.delivery_days)
    INTO customer,project,proposal
    FROM projects p JOIN proposals pr ON pr.id=NEW.proposal_id WHERE p.id=NEW.project_id;

  INSERT INTO safe_deals(
    project_id,assignment_id,customer_user_id,freelancer_user_id,currency,
    work_amount_kopecks,gross_amount_kopecks,platform_fee_kopecks,freelancer_amount_kopecks,
    platform_fee_customer_kopecks,platform_fee_freelancer_kopecks,platform_fee_platform_kopecks,
    provider_fee_kopecks,provider_fee_customer_kopecks,provider_fee_freelancer_kopecks,provider_fee_platform_kopecks,
    customer_discount_kopecks,freelancer_bonus_kopecks,
    platform_provider_cost_kopecks,platform_subsidy_kopecks,platform_net_revenue_kopecks,
    platform_fee_payer_mode,platform_customer_share_basis_points,
    provider_fee_payer_mode,provider_customer_share_basis_points,
    fee_rule_version,provider_pricing_version,status,proposal_snapshot,project_snapshot)
  VALUES(
    NEW.project_id,NEW.id,customer,NEW.freelancer_user_id,'RUB',
    work,customer_total,gross_revenue,freelancer_payout,
    plat_cust,plat_free,plat_plat,
    prov_fee,prov_cust,prov_free,prov_plat,
    discount,bonus,
    provider_cost,subsidy,net_revenue,
    fee_rule.platform_fee_payer_mode,fee_rule.platform_customer_share_basis_points,
    fee_rule.provider_fee_payer_mode,fee_rule.provider_customer_share_basis_points,
    fee_rule.version,pricing_version,'AWAITING_FUNDING',proposal,project);
  RETURN NEW;
END $$;
