CREATE TABLE external_reputations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id),
  platform varchar(80) NOT NULL CHECK (char_length(trim(platform)) BETWEEN 1 AND 80),
  profile_url text NOT NULL CHECK (char_length(profile_url) BETWEEN 1 AND 2048),
  external_username varchar(160),
  rating numeric(4,2) CHECK (rating IS NULL OR rating >= 0),
  reviews_count int CHECK (reviews_count IS NULL OR reviews_count >= 0),
  completed_orders_count int CHECK (completed_orders_count IS NULL OR completed_orders_count >= 0),
  account_since date,
  verification_status text NOT NULL DEFAULT 'UNVERIFIED'
    CHECK (verification_status IN ('UNVERIFIED','PENDING','VERIFIED','REJECTED','EXPIRED')),
  verification_method text CHECK (verification_method IS NULL OR verification_method IN ('PROFILE_CODE','MANUAL','OFFICIAL_API')),
  verified_at timestamptz,
  expires_at timestamptz,
  last_checked_at timestamptz,
  evidence jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(evidence) = 'object'),
  source_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(source_snapshot) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (user_id, platform, profile_url)
);

CREATE INDEX external_reputations_user_status_idx ON external_reputations(user_id, verification_status);
CREATE INDEX external_reputations_verified_user_idx ON external_reputations(user_id, verified_at DESC)
  WHERE verification_status = 'VERIFIED';

CREATE TABLE reputation_verification_challenges (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  external_reputation_id uuid NOT NULL REFERENCES external_reputations(id) ON DELETE CASCADE,
  method text NOT NULL CHECK (method IN ('PROFILE_CODE','MANUAL')),
  code_hash bytea,
  expires_at timestamptz NOT NULL,
  attempts int NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 10),
  status text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','VERIFIED','REJECTED','EXPIRED')),
  created_at timestamptz NOT NULL DEFAULT now(),
  verified_at timestamptz,
  CHECK ((method = 'PROFILE_CODE' AND code_hash IS NOT NULL) OR (method = 'MANUAL' AND code_hash IS NULL))
);
CREATE UNIQUE INDEX reputation_challenges_one_pending_idx ON reputation_verification_challenges(external_reputation_id)
  WHERE status = 'PENDING';
CREATE INDEX reputation_challenges_expiry_idx ON reputation_verification_challenges(expires_at)
  WHERE status = 'PENDING';
