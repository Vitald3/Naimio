CREATE TABLE IF NOT EXISTS portfolio_items (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id),
  title varchar(180) NOT NULL,
  slug varchar(220) NOT NULL,
  description text,
  external_url text,
  price_min_kopecks bigint CHECK (price_min_kopecks IS NULL OR price_min_kopecks >= 0),
  price_max_kopecks bigint CHECK (price_max_kopecks IS NULL OR price_max_kopecks >= 0),
  completed_on date,
  visibility text NOT NULL DEFAULT 'PUBLIC' CHECK (visibility IN ('PUBLIC','PRIVATE')),
  sort_order int NOT NULL DEFAULT 0 CHECK (sort_order BETWEEN 0 AND 10000),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  CHECK (char_length(title) BETWEEN 1 AND 180),
  CHECK (char_length(slug) BETWEEN 1 AND 220),
  CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
  CHECK (description IS NULL OR char_length(description) <= 5000),
  CHECK (external_url IS NULL OR char_length(external_url) <= 2048),
  CHECK (price_min_kopecks IS NULL OR price_max_kopecks IS NULL OR price_min_kopecks <= price_max_kopecks)
);
CREATE INDEX IF NOT EXISTS portfolio_items_owner_order_idx
  ON portfolio_items(user_id, sort_order, created_at DESC) WHERE deleted_at IS NULL;
-- Soft deletion preserves portfolio history while allowing the owner to reuse a slug.
CREATE UNIQUE INDEX IF NOT EXISTS portfolio_items_owner_slug_unique
  ON portfolio_items(user_id, slug) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS portfolio_categories (
  portfolio_item_id uuid NOT NULL REFERENCES portfolio_items(id) ON DELETE CASCADE,
  category_id uuid NOT NULL REFERENCES categories(id),
  PRIMARY KEY (portfolio_item_id, category_id)
);

CREATE TABLE IF NOT EXISTS portfolio_skills (
  portfolio_item_id uuid NOT NULL REFERENCES portfolio_items(id) ON DELETE CASCADE,
  skill_id uuid NOT NULL REFERENCES skills(id),
  PRIMARY KEY (portfolio_item_id, skill_id)
);

-- Metadata only: the upload lifecycle and object-storage integration remain in slice 2.4.
CREATE TABLE IF NOT EXISTS media_objects (
  id uuid PRIMARY KEY,
  owner_user_id uuid NOT NULL REFERENCES users(id),
  object_key text NOT NULL,
  bucket varchar(120) NOT NULL,
  original_filename text,
  mime_type varchar(160) NOT NULL,
  size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
  sha256 bytea,
  scan_status text NOT NULL DEFAULT 'PENDING' CHECK (scan_status IN ('PENDING','CLEAN','INFECTED','FAILED')),
  visibility text NOT NULL DEFAULT 'PRIVATE' CHECK (visibility IN ('PRIVATE','PUBLIC')),
  created_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS media_objects_bucket_key_unique ON media_objects(bucket, object_key);
CREATE INDEX IF NOT EXISTS media_objects_owner_created_idx ON media_objects(owner_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS media_objects_sha256_idx ON media_objects(sha256) WHERE sha256 IS NOT NULL;

CREATE TABLE IF NOT EXISTS portfolio_media (
  portfolio_item_id uuid NOT NULL REFERENCES portfolio_items(id) ON DELETE CASCADE,
  media_object_id uuid NOT NULL REFERENCES media_objects(id),
  sort_order int NOT NULL DEFAULT 0 CHECK (sort_order BETWEEN 0 AND 10000),
  PRIMARY KEY (portfolio_item_id, media_object_id)
);
