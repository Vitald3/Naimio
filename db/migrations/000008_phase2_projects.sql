ALTER TABLE media_objects DROP CONSTRAINT media_objects_purpose_check;
ALTER TABLE media_objects
  ADD CONSTRAINT media_objects_purpose_check CHECK (purpose IN ('PORTFOLIO','SERVICE','PROJECT'));

CREATE TABLE IF NOT EXISTS projects (
  id uuid PRIMARY KEY,
  customer_user_id uuid NOT NULL REFERENCES users(id),
  category_id uuid REFERENCES categories(id),
  title varchar(200) NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
  slug varchar(240) NOT NULL CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
  description text NOT NULL CHECK (char_length(description) BETWEEN 1 AND 15000),
  budget_type text NOT NULL CHECK (budget_type IN ('FIXED','RANGE','NEGOTIABLE','HOURLY')),
  budget_min_kopecks bigint,
  budget_max_kopecks bigint,
  currency char(3) NOT NULL DEFAULT 'RUB' CHECK (currency = 'RUB'),
  deadline_at timestamptz,
  experience_level text CHECK (experience_level IS NULL OR experience_level IN ('BEGINNER','INTERMEDIATE','ADVANCED','EXPERT')),
  visibility text NOT NULL DEFAULT 'PUBLIC' CHECK (visibility IN ('PUBLIC','PRIVATE')),
  status text NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','OPEN','MATCHING','IN_PROGRESS','COMPLETED','CANCELLED','ARCHIVED')),
  source_type text NOT NULL DEFAULT 'MANUAL' CHECK (source_type IN ('MANUAL','AI_BRIEF','IMPORT','COMMERCIAL_OFFER','REPEAT','INVITE')),
  published_at timestamptz,
  proposal_count int NOT NULL DEFAULT 0 CHECK (proposal_count >= 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  search_vector tsvector GENERATED ALWAYS AS (
    to_tsvector('simple'::regconfig, coalesce(title, '') || ' ' || coalesce(description, ''))
  ) STORED,
  CHECK (
    (budget_type = 'NEGOTIABLE' AND budget_min_kopecks IS NULL AND budget_max_kopecks IS NULL)
    OR (budget_type = 'FIXED' AND budget_min_kopecks IS NOT NULL AND budget_min_kopecks > 0 AND budget_max_kopecks IS NULL)
    OR (budget_type IN ('RANGE','HOURLY') AND budget_min_kopecks IS NOT NULL AND budget_max_kopecks IS NOT NULL
      AND budget_min_kopecks > 0 AND budget_max_kopecks >= budget_min_kopecks)
  ),
  CHECK (status NOT IN ('OPEN','MATCHING','IN_PROGRESS','COMPLETED') OR published_at IS NOT NULL)
);
CREATE UNIQUE INDEX IF NOT EXISTS projects_customer_slug_unique
  ON projects(customer_user_id, slug) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS projects_status_published_idx
  ON projects(status, published_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS projects_category_status_published_idx
  ON projects(category_id, status, published_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS projects_customer_status_created_idx
  ON projects(customer_user_id, status, created_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS projects_open_deadline_idx
  ON projects(deadline_at) WHERE status IN ('OPEN','MATCHING');
CREATE INDEX IF NOT EXISTS projects_search_vector_idx ON projects USING gin(search_vector);
CREATE INDEX IF NOT EXISTS projects_title_trgm_idx ON projects USING gin(title gin_trgm_ops);

CREATE TABLE IF NOT EXISTS project_skills (
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  skill_id uuid NOT NULL REFERENCES skills(id),
  importance smallint NOT NULL DEFAULT 100 CHECK (importance BETWEEN 1 AND 100),
  PRIMARY KEY (project_id, skill_id)
);

CREATE TABLE IF NOT EXISTS project_media (
  project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  media_object_id uuid NOT NULL REFERENCES media_objects(id),
  sort_order int NOT NULL DEFAULT 0 CHECK (sort_order BETWEEN 0 AND 10000),
  PRIMARY KEY (project_id, media_object_id)
);
