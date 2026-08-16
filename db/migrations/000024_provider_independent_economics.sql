-- Provider-independent payment economics.
--
-- Splits the single opaque Safe Deal fee into two independent, versioned classes:
--   * PLATFORM COMMISSION      — marketplace revenue.
--   * PROVIDER / SAFE-DEAL COST — pass-through cost of the payment provider.
-- Each class independently decides who pays it (CUSTOMER / FREELANCER / SPLIT /
-- PLATFORM). Every deal snapshots the full economic breakdown so that changing
-- an admin rule never mutates an existing deal.
--
-- This mirrors apps/api/internal/safedeal.CalculateDealQuote EXACTLY. All money
-- is integer kopecks; SQL integer division truncates toward zero for positive
-- operands, identical to Go's `work*bp/10000`, so the trigger and the domain
-- always agree. Backward compatible: existing deals are modelled as
-- platform commission payer = FREELANCER with a zero provider fee, which keeps
-- their customer charge, freelancer payout and commission numbers unchanged.

-- ---------------------------------------------------------------------------
-- 1. Fee-rule versioning: who pays each of the two fee classes.
-- ---------------------------------------------------------------------------
ALTER TABLE safe_deal_fee_rules
  ADD COLUMN IF NOT EXISTS platform_fee_payer_mode varchar(20) NOT NULL DEFAULT 'FREELANCER',
  ADD COLUMN IF NOT EXISTS platform_customer_share_basis_points int NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS provider_fee_payer_mode varchar(20) NOT NULL DEFAULT 'CUSTOMER',
  ADD COLUMN IF NOT EXISTS provider_customer_share_basis_points int NOT NULL DEFAULT 0;

DO $$ BEGIN
  IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conname='safe_deal_fee_rules_platform_payer_check') THEN
    ALTER TABLE safe_deal_fee_rules ADD CONSTRAINT safe_deal_fee_rules_platform_payer_check
      CHECK(platform_fee_payer_mode IN('CUSTOMER','FREELANCER','SPLIT','PLATFORM'));
  END IF;
  IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conname='safe_deal_fee_rules_provider_payer_check') THEN
    ALTER TABLE safe_deal_fee_rules ADD CONSTRAINT safe_deal_fee_rules_provider_payer_check
      CHECK(provider_fee_payer_mode IN('CUSTOMER','FREELANCER','SPLIT','PLATFORM'));
  END IF;
  IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conname='safe_deal_fee_rules_platform_share_check') THEN
    ALTER TABLE safe_deal_fee_rules ADD CONSTRAINT safe_deal_fee_rules_platform_share_check
      CHECK(platform_customer_share_basis_points BETWEEN 0 AND 10000);
  END IF;
  IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conname='safe_deal_fee_rules_provider_share_check') THEN
    ALTER TABLE safe_deal_fee_rules ADD CONSTRAINT safe_deal_fee_rules_provider_share_check
      CHECK(provider_customer_share_basis_points BETWEEN 0 AND 10000);
  END IF;
END $$;

-- ---------------------------------------------------------------------------
-- 2. Provider pricing rules, versioned per provider + payment method. Pure
--    cost structure (percent + fixed, clamped); the payer decision lives on
--    the fee rule above. No real bank is integrated — 'sandbox' priced at zero.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS safe_deal_provider_pricing(
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  version int NOT NULL CHECK(version>0),
  provider varchar(80) NOT NULL,
  payment_method varchar(20) NOT NULL CHECK(payment_method IN('CARD','SBP','T_PAY','SBER_PAY','OTHER')),
  percent_basis_points int NOT NULL CHECK(percent_basis_points BETWEEN 0 AND 10000),
  fixed_fee_kopecks bigint NOT NULL DEFAULT 0 CHECK(fixed_fee_kopecks>=0),
  minimum_fee_kopecks bigint NOT NULL DEFAULT 0 CHECK(minimum_fee_kopecks>=0),
  maximum_fee_kopecks bigint CHECK(maximum_fee_kopecks IS NULL OR maximum_fee_kopecks>=minimum_fee_kopecks),
  enabled boolean NOT NULL DEFAULT false,
  effective_from timestamptz NOT NULL DEFAULT now(),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(provider,payment_method,version)
);
-- At most one enabled pricing row per provider + payment method.
CREATE UNIQUE INDEX IF NOT EXISTS safe_deal_provider_pricing_enabled_unique
  ON safe_deal_provider_pricing(provider,payment_method) WHERE enabled;

INSERT INTO safe_deal_provider_pricing(version,provider,payment_method,percent_basis_points,fixed_fee_kopecks,enabled)
VALUES (1,'sandbox','CARD',0,0,true),(1,'sandbox','SBP',0,0,true)
ON CONFLICT(provider,payment_method,version) DO NOTHING;

-- ---------------------------------------------------------------------------
-- 3. Per-deal immutable economic snapshot. gross_amount_kopecks keeps its
--    meaning as the customer's funding charge (== customer_total) and
--    freelancer_amount_kopecks as the release payout; platform_fee_kopecks is
--    the full platform commission (== gross revenue). The new columns record
--    the work amount, both fee allocations, adjustments and platform economics.
-- ---------------------------------------------------------------------------
ALTER TABLE safe_deals
  ADD COLUMN IF NOT EXISTS work_amount_kopecks bigint,
  ADD COLUMN IF NOT EXISTS platform_fee_customer_kopecks bigint NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS platform_fee_freelancer_kopecks bigint NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS platform_fee_platform_kopecks bigint NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS provider_fee_kopecks bigint NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS provider_fee_customer_kopecks bigint NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS provider_fee_freelancer_kopecks bigint NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS provider_fee_platform_kopecks bigint NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS customer_discount_kopecks bigint NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS freelancer_bonus_kopecks bigint NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS platform_provider_cost_kopecks bigint NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS platform_subsidy_kopecks bigint NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS platform_net_revenue_kopecks bigint NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS platform_fee_payer_mode varchar(20) NOT NULL DEFAULT 'FREELANCER',
  ADD COLUMN IF NOT EXISTS platform_customer_share_basis_points int NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS provider_fee_payer_mode varchar(20) NOT NULL DEFAULT 'CUSTOMER',
  ADD COLUMN IF NOT EXISTS provider_customer_share_basis_points int NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS provider_pricing_version int NOT NULL DEFAULT 1,
  -- The ACTUAL provider fee, populated from settlement/reconciliation. NULL
  -- until known; never overwrites the estimated economics above.
  ADD COLUMN IF NOT EXISTS actual_provider_fee_kopecks bigint;

-- Backfill existing deals as the legacy model: the old gross_amount was the
-- work amount, the whole commission was borne by the freelancer, provider fee
-- was zero. This preserves every historical customer charge and payout exactly.
UPDATE safe_deals SET
  work_amount_kopecks             = gross_amount_kopecks,
  platform_fee_customer_kopecks   = 0,
  platform_fee_freelancer_kopecks = platform_fee_kopecks,
  platform_fee_platform_kopecks   = 0,
  provider_fee_kopecks            = 0,
  provider_fee_customer_kopecks   = 0,
  provider_fee_freelancer_kopecks = 0,
  provider_fee_platform_kopecks   = 0,
  customer_discount_kopecks       = 0,
  freelancer_bonus_kopecks        = 0,
  platform_provider_cost_kopecks  = 0,
  platform_subsidy_kopecks        = 0,
  platform_net_revenue_kopecks    = platform_fee_kopecks,
  platform_fee_payer_mode         = 'FREELANCER',
  platform_customer_share_basis_points = 0,
  provider_fee_payer_mode         = 'CUSTOMER',
  provider_customer_share_basis_points = 0,
  provider_pricing_version        = 1
WHERE work_amount_kopecks IS NULL;

ALTER TABLE safe_deals ALTER COLUMN work_amount_kopecks SET NOT NULL;

-- ---------------------------------------------------------------------------
-- 4. Replace the legacy single-fee invariants with the dual-class invariants
--    from CalculateDealQuote.validateInvariants. Drop by definition so we do
--    not depend on the auto-generated constraint names.
-- ---------------------------------------------------------------------------
DO $$ DECLARE n text; BEGIN
  FOR n IN SELECT conname FROM pg_constraint
      WHERE conrelid='safe_deals'::regclass AND contype='c'
        AND (pg_get_constraintdef(oid) LIKE '%gross_amount_kopecks - platform_fee_kopecks%'
             OR pg_get_constraintdef(oid) LIKE '%platform_fee_kopecks < gross_amount_kopecks%')
  LOOP EXECUTE format('ALTER TABLE safe_deals DROP CONSTRAINT %I',n); END LOOP;
END $$;

DO $$ BEGIN
  -- Payer modes are drawn from the fixed set.
  IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conname='safe_deals_platform_payer_check') THEN
    ALTER TABLE safe_deals ADD CONSTRAINT safe_deals_platform_payer_check
      CHECK(platform_fee_payer_mode IN('CUSTOMER','FREELANCER','SPLIT','PLATFORM'));
  END IF;
  IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conname='safe_deals_provider_payer_check') THEN
    ALTER TABLE safe_deals ADD CONSTRAINT safe_deals_provider_payer_check
      CHECK(provider_fee_payer_mode IN('CUSTOMER','FREELANCER','SPLIT','PLATFORM'));
  END IF;
  -- Non-negativity of every component (net revenue may be negative when subsidised).
  IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conname='safe_deals_amounts_nonneg_check') THEN
    ALTER TABLE safe_deals ADD CONSTRAINT safe_deals_amounts_nonneg_check CHECK(
      work_amount_kopecks>0 AND platform_fee_kopecks>=0 AND provider_fee_kopecks>=0
      AND platform_fee_customer_kopecks>=0 AND platform_fee_freelancer_kopecks>=0 AND platform_fee_platform_kopecks>=0
      AND provider_fee_customer_kopecks>=0 AND provider_fee_freelancer_kopecks>=0 AND provider_fee_platform_kopecks>=0
      AND customer_discount_kopecks>=0 AND freelancer_bonus_kopecks>=0
      AND platform_provider_cost_kopecks>=0 AND platform_subsidy_kopecks>=0
      AND (actual_provider_fee_kopecks IS NULL OR actual_provider_fee_kopecks>=0));
  END IF;
  -- Each fee's three parts sum exactly to its total.
  IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conname='safe_deals_fee_parts_sum_check') THEN
    ALTER TABLE safe_deals ADD CONSTRAINT safe_deals_fee_parts_sum_check CHECK(
      platform_fee_customer_kopecks+platform_fee_freelancer_kopecks+platform_fee_platform_kopecks=platform_fee_kopecks
      AND provider_fee_customer_kopecks+provider_fee_freelancer_kopecks+provider_fee_platform_kopecks=provider_fee_kopecks);
  END IF;
  -- Customer and freelancer legs.
  IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conname='safe_deals_customer_leg_check') THEN
    ALTER TABLE safe_deals ADD CONSTRAINT safe_deals_customer_leg_check CHECK(
      gross_amount_kopecks=work_amount_kopecks+platform_fee_customer_kopecks+provider_fee_customer_kopecks-customer_discount_kopecks);
  END IF;
  IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conname='safe_deals_freelancer_leg_check') THEN
    ALTER TABLE safe_deals ADD CONSTRAINT safe_deals_freelancer_leg_check CHECK(
      freelancer_amount_kopecks=work_amount_kopecks-platform_fee_freelancer_kopecks-provider_fee_freelancer_kopecks+freelancer_bonus_kopecks);
  END IF;
  -- Platform economics provenance.
  IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conname='safe_deals_provider_cost_check') THEN
    ALTER TABLE safe_deals ADD CONSTRAINT safe_deals_provider_cost_check
      CHECK(platform_provider_cost_kopecks=provider_fee_platform_kopecks);
  END IF;
  IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conname='safe_deals_subsidy_check') THEN
    ALTER TABLE safe_deals ADD CONSTRAINT safe_deals_subsidy_check CHECK(
      platform_subsidy_kopecks=platform_fee_platform_kopecks+provider_fee_platform_kopecks+customer_discount_kopecks+freelancer_bonus_kopecks);
  END IF;
  -- Master invariant: money in equals money out.
  IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conname='safe_deals_conservation_check') THEN
    ALTER TABLE safe_deals ADD CONSTRAINT safe_deals_conservation_check CHECK(
      gross_amount_kopecks=freelancer_amount_kopecks+provider_fee_kopecks+platform_net_revenue_kopecks);
  END IF;
  -- Net revenue reconciles with gross commission minus subsidy.
  IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conname='safe_deals_net_revenue_check') THEN
    ALTER TABLE safe_deals ADD CONSTRAINT safe_deals_net_revenue_check CHECK(
      platform_net_revenue_kopecks=platform_fee_kopecks-platform_subsidy_kopecks);
  END IF;
END $$;

-- ---------------------------------------------------------------------------
-- 5. Rewrite the assignment trigger to mirror CalculateDealQuote. It estimates
--    the provider fee from the default provider pricing (sandbox CARD) at
--    creation time; the ACTUAL fee is recorded later during settlement.
-- ---------------------------------------------------------------------------
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
BEGIN
  IF NEW.agreed_price_kopecks IS NULL OR NEW.agreed_price_kopecks<=0 THEN
    RAISE EXCEPTION 'accepted proposal requires agreed price' USING ERRCODE='23514';
  END IF;
  work := NEW.agreed_price_kopecks;

  SELECT * INTO fee_rule FROM safe_deal_fee_rules WHERE enabled AND effective_from<=now() ORDER BY effective_from DESC LIMIT 1;
  IF NOT FOUND THEN RAISE EXCEPTION 'safe deal fee rule missing'; END IF;

  -- Estimated provider pricing: default provider + CARD. Absent pricing => zero.
  SELECT * INTO pricing FROM safe_deal_provider_pricing
    WHERE enabled AND payment_method='CARD' AND effective_from<=now()
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
