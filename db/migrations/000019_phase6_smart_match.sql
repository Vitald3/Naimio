CREATE TABLE matching_runs (
  id uuid PRIMARY KEY,
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  requested_by_user_id uuid NOT NULL REFERENCES users(id),
  algorithm_version varchar(80) NOT NULL,
  ai_used boolean NOT NULL DEFAULT false,
  candidate_count int NOT NULL CHECK (candidate_count BETWEEN 0 AND 200),
  constraints jsonb NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX matching_runs_project_created_idx ON matching_runs(project_id,created_at DESC,id DESC);

CREATE TABLE matching_candidates (
  matching_run_id uuid NOT NULL REFERENCES matching_runs(id) ON DELETE CASCADE,
  freelancer_user_id uuid NOT NULL REFERENCES users(id),
  deterministic_score int NOT NULL CHECK (deterministic_score BETWEEN 0 AND 10000),
  final_score int NOT NULL CHECK (final_score BETWEEN 0 AND 10000),
  rank int NOT NULL CHECK (rank BETWEEN 1 AND 200),
  reasons jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(reasons)='array'),
  PRIMARY KEY(matching_run_id,freelancer_user_id),
  UNIQUE(matching_run_id,rank)
);
CREATE INDEX matching_candidates_run_rank_idx ON matching_candidates(matching_run_id,rank);
CREATE INDEX matching_candidates_freelancer_run_idx ON matching_candidates(freelancer_user_id,matching_run_id);

CREATE TABLE manual_project_recommendations (
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  freelancer_user_id uuid NOT NULL REFERENCES users(id),
  recommended_by_user_id uuid NOT NULL REFERENCES users(id),
  internal_reason varchar(1000) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(project_id,freelancer_user_id)
);
CREATE INDEX manual_recommendations_admin_created_idx ON manual_project_recommendations(recommended_by_user_id,created_at DESC);

CREATE TABLE matching_quality_events (
  id uuid PRIMARY KEY,
  matching_run_id uuid NOT NULL REFERENCES matching_runs(id) ON DELETE CASCADE,
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  freelancer_user_id uuid NOT NULL REFERENCES users(id),
  actor_user_id uuid REFERENCES users(id),
  event_type varchar(40) NOT NULL CHECK (event_type IN ('IMPRESSION','PROFILE_OPEN','INVITE','SHORTLIST','PROPOSAL','ACCEPTANCE','COMPLETED','REPEAT')),
  event_key varchar(128) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(actor_user_id,event_key)
);
CREATE INDEX matching_quality_project_event_idx ON matching_quality_events(project_id,event_type,created_at DESC);
CREATE INDEX matching_quality_run_event_idx ON matching_quality_events(matching_run_id,event_type,created_at DESC);
