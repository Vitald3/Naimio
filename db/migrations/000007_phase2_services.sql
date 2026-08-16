ALTER TABLE media_objects DROP CONSTRAINT media_objects_purpose_check;
ALTER TABLE media_objects
  ADD CONSTRAINT media_objects_purpose_check CHECK (purpose IN ('PORTFOLIO','SERVICE'));
CREATE INDEX IF NOT EXISTS media_objects_owner_purpose_idx
  ON media_objects(owner_user_id, purpose, created_at DESC) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS services (
  id uuid PRIMARY KEY,
  seller_user_id uuid NOT NULL REFERENCES users(id),
  category_id uuid NOT NULL REFERENCES categories(id),
  service_type text NOT NULL CHECK (service_type IN ('PROFESSIONAL_SERVICE','CONSULTATION','EDUCATION','MENTORING')),
  title varchar(180) NOT NULL CHECK (char_length(title) BETWEEN 1 AND 180),
  slug varchar(220) NOT NULL CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
  short_description varchar(320),
  description text NOT NULL CHECK (char_length(description) BETWEEN 1 AND 10000),
  price_type text NOT NULL CHECK (price_type IN ('FIXED','FROM','HOURLY','NEGOTIABLE')),
  price_from_kopecks bigint,
  currency char(3) NOT NULL DEFAULT 'RUB' CHECK (currency = 'RUB'),
  delivery_days int CHECK (delivery_days IS NULL OR delivery_days BETWEEN 1 AND 365),
  included_revisions int CHECK (included_revisions IS NULL OR included_revisions BETWEEN 0 AND 100),
  status text NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','PENDING_MODERATION','ACTIVE','PAUSED','REJECTED','ARCHIVED')),
  visibility text NOT NULL DEFAULT 'PUBLIC' CHECK (visibility IN ('PUBLIC','PRIVATE')),
  published_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  search_vector tsvector GENERATED ALWAYS AS (
    to_tsvector('simple'::regconfig, coalesce(title, '') || ' ' || coalesce(short_description, '') || ' ' || coalesce(description, ''))
  ) STORED,
  CHECK ((price_type = 'NEGOTIABLE' AND price_from_kopecks IS NULL)
    OR (price_type <> 'NEGOTIABLE' AND price_from_kopecks IS NOT NULL AND price_from_kopecks > 0)),
  CHECK (status NOT IN ('ACTIVE','PAUSED') OR published_at IS NOT NULL)
);
CREATE UNIQUE INDEX IF NOT EXISTS services_seller_slug_unique
  ON services(seller_user_id, slug) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS services_category_status_published_idx
  ON services(category_id, status, published_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS services_seller_status_created_idx
  ON services(seller_user_id, status, created_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS services_public_type_published_idx
  ON services(service_type, status, published_at DESC, id DESC) WHERE deleted_at IS NULL AND visibility = 'PUBLIC';
CREATE INDEX IF NOT EXISTS services_search_vector_idx ON services USING gin(search_vector);
CREATE INDEX IF NOT EXISTS services_title_trgm_idx ON services USING gin(title gin_trgm_ops);

CREATE TABLE IF NOT EXISTS service_skills (
  service_id uuid NOT NULL REFERENCES services(id) ON DELETE CASCADE,
  skill_id uuid NOT NULL REFERENCES skills(id),
  PRIMARY KEY (service_id, skill_id)
);

CREATE TABLE IF NOT EXISTS service_media (
  service_id uuid NOT NULL REFERENCES services(id) ON DELETE CASCADE,
  media_object_id uuid NOT NULL REFERENCES media_objects(id),
  sort_order int NOT NULL DEFAULT 0 CHECK (sort_order BETWEEN 0 AND 10000),
  PRIMARY KEY (service_id, media_object_id)
);

CREATE TABLE IF NOT EXISTS education_service_details (
  service_id uuid PRIMARY KEY REFERENCES services(id) ON DELETE CASCADE,
  format text NOT NULL CHECK (format IN ('ONLINE','OFFLINE','ASYNC','HYBRID')),
  duration_minutes int CHECK (duration_minutes IS NULL OR duration_minutes BETWEEN 15 AND 10080),
  sessions_count int CHECK (sessions_count IS NULL OR sessions_count BETWEEN 1 AND 365),
  audience_type text CHECK (audience_type IS NULL OR audience_type IN ('INDIVIDUAL','GROUP','BOTH')),
  group_size_max int CHECK (group_size_max IS NULL OR group_size_max BETWEEN 2 AND 1000)
);
