CREATE TABLE invites (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  inviter_user_id uuid NOT NULL REFERENCES users(id),
  invite_type text NOT NULL CHECK(invite_type IN('CUSTOMER','FREELANCER','PROJECT')),
  project_id uuid REFERENCES projects(id),
  token_hash bytea NOT NULL UNIQUE,
  intended_email varchar(320),
  expires_at timestamptz,
  accepted_by_user_id uuid REFERENCES users(id),
  accepted_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK(inviter_user_id IS DISTINCT FROM accepted_by_user_id),
  CHECK(invite_type='CUSTOMER' OR project_id IS NOT NULL)
);
CREATE INDEX invites_inviter_created_idx ON invites(inviter_user_id,created_at DESC,id DESC);
CREATE INDEX invites_project_idx ON invites(project_id,created_at DESC) WHERE project_id IS NOT NULL;

CREATE TABLE project_invited_users (
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id),
  invite_id uuid NOT NULL REFERENCES invites(id),
  invited_role text NOT NULL CHECK(invited_role IN('CUSTOMER','FREELANCER')),
  accepted_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(project_id,user_id),
  UNIQUE(invite_id)
);

CREATE TABLE referral_rules (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code varchar(100) NOT NULL UNIQUE,
  event_type text NOT NULL,
  beneficiary text NOT NULL CHECK(beneficiary IN('INVITER','INVITED')),
  reward_type text NOT NULL CHECK(reward_type IN('COMMISSION_DISCOUNT','BONUS','FIXED_REWARD','PERCENT_REWARD')),
  reward_value bigint NOT NULL CHECK(reward_value>0),
  reward_unit text NOT NULL CHECK(reward_unit IN('KOPECKS','BASIS_POINTS','COUNT','CREDITS')),
  max_uses int CHECK(max_uses IS NULL OR max_uses>0),
  starts_at timestamptz,
  ends_at timestamptz,
  enabled boolean NOT NULL DEFAULT true,
  config jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(config)='object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK(starts_at IS NULL OR ends_at IS NULL OR starts_at<ends_at),
  CHECK(reward_type<>'PERCENT_REWARD' OR reward_unit='BASIS_POINTS'),
  CHECK(reward_unit<>'BASIS_POINTS' OR reward_value<=10000)
);
CREATE INDEX referral_rules_active_idx ON referral_rules(event_type,starts_at,ends_at) WHERE enabled=true;

CREATE TABLE referral_attributions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  inviter_user_id uuid NOT NULL REFERENCES users(id),
  invited_user_id uuid NOT NULL REFERENCES users(id),
  invite_id uuid REFERENCES invites(id),
  first_touch_at timestamptz NOT NULL,
  source varchar(100),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(invited_user_id),
  CHECK(inviter_user_id<>invited_user_id)
);
CREATE INDEX referral_attributions_inviter_idx ON referral_attributions(inviter_user_id,created_at DESC,id DESC);

CREATE TABLE reward_ledger (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id),
  rule_id uuid REFERENCES referral_rules(id),
  event_key varchar(160) NOT NULL,
  reward_type text NOT NULL,
  amount bigint NOT NULL CHECK(amount>0),
  unit text NOT NULL,
  expires_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(user_id,event_key)
);
CREATE INDEX reward_ledger_user_created_idx ON reward_ledger(user_id,created_at DESC,id DESC);

CREATE TABLE customer_team_members (
  customer_user_id uuid NOT NULL REFERENCES users(id),
  freelancer_user_id uuid NOT NULL REFERENCES users(id),
  label varchar(120),
  notes text CHECK(notes IS NULL OR char_length(notes)<=2000),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(customer_user_id,freelancer_user_id),
  CHECK(customer_user_id<>freelancer_user_id)
);
CREATE INDEX customer_team_freelancer_idx ON customer_team_members(freelancer_user_id,updated_at DESC);
