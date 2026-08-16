CREATE TABLE project_drafts (
  id uuid PRIMARY KEY,
  owner_user_id uuid REFERENCES users(id),
  guest_token_hash bytea,
  source_type text NOT NULL CHECK (source_type IN ('AI_BRIEF','IMPORT','COMMERCIAL_OFFER','MANUAL')),
  raw_input jsonb NOT NULL DEFAULT '{}',
  normalized_data jsonb NOT NULL DEFAULT '{}',
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK (owner_user_id IS NOT NULL OR guest_token_hash IS NOT NULL)
);
CREATE INDEX project_drafts_owner_updated_idx ON project_drafts(owner_user_id, updated_at DESC) WHERE owner_user_id IS NOT NULL;
CREATE INDEX project_drafts_expiry_idx ON project_drafts(expires_at);
CREATE UNIQUE INDEX project_drafts_guest_token_unique ON project_drafts(guest_token_hash) WHERE guest_token_hash IS NOT NULL;

CREATE TABLE ai_requests (
  id uuid PRIMARY KEY,
  user_id uuid REFERENCES users(id),
  capability varchar(80) NOT NULL,
  provider varchar(80) NOT NULL,
  model varchar(160) NOT NULL,
  status text NOT NULL CHECK (status IN ('SUCCEEDED','FAILED','TIMEOUT','INVALID_OUTPUT')),
  input_tokens int CHECK (input_tokens IS NULL OR input_tokens >= 0),
  output_tokens int CHECK (output_tokens IS NULL OR output_tokens >= 0),
  cost_microunits bigint CHECK (cost_microunits IS NULL OR cost_microunits >= 0),
  latency_ms int CHECK (latency_ms IS NULL OR latency_ms >= 0),
  error_code varchar(100),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ai_requests_capability_created_idx ON ai_requests(capability, created_at DESC);
CREATE INDEX ai_requests_provider_model_created_idx ON ai_requests(provider, model, created_at DESC);
CREATE INDEX ai_requests_user_created_idx ON ai_requests(user_id, created_at DESC) WHERE user_id IS NOT NULL;
