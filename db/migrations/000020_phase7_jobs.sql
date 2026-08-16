ALTER TABLE services ADD COLUMN moderation_status text NOT NULL DEFAULT 'VISIBLE'
  CHECK (moderation_status IN ('VISIBLE','HIDDEN'));
CREATE INDEX services_moderation_status_idx ON services(moderation_status, status, published_at DESC)
  WHERE deleted_at IS NULL;

CREATE TABLE companies (
  id uuid PRIMARY KEY,
  owner_user_id uuid NOT NULL REFERENCES users(id),
  name varchar(180) NOT NULL CHECK (char_length(name) BETWEEN 1 AND 180),
  slug varchar(220) NOT NULL CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
  logo_object_key text,
  website text,
  description text CHECK (description IS NULL OR char_length(description) <= 4000),
  verification_status text NOT NULL DEFAULT 'UNVERIFIED' CHECK (verification_status IN ('UNVERIFIED','VERIFIED')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX companies_owner_slug_unique ON companies(owner_user_id, slug);

CREATE TABLE jobs (
  id uuid PRIMARY KEY,
  company_id uuid REFERENCES companies(id),
  customer_user_id uuid NOT NULL REFERENCES users(id),
  category_id uuid REFERENCES categories(id),
  title varchar(200) NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
  slug varchar(240) NOT NULL CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
  description text NOT NULL CHECK (char_length(description) BETWEEN 1 AND 20000),
  employment_type text NOT NULL CHECK (employment_type IN ('FULL_TIME','PART_TIME','CONTRACT','INTERNSHIP')),
  salary_min_kopecks bigint CHECK (salary_min_kopecks IS NULL OR salary_min_kopecks > 0),
  salary_max_kopecks bigint CHECK (salary_max_kopecks IS NULL OR salary_max_kopecks > 0),
  currency char(3) NOT NULL DEFAULT 'RUB' CHECK (currency = 'RUB'),
  location_text varchar(160),
  remote boolean NOT NULL DEFAULT true,
  experience_level text CHECK (experience_level IS NULL OR experience_level IN ('JUNIOR','MIDDLE','SENIOR','LEAD','ANY')),
  status text NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','PUBLISHED','CLOSED','ARCHIVED')),
  moderation_status text NOT NULL DEFAULT 'VISIBLE' CHECK (moderation_status IN ('VISIBLE','HIDDEN')),
  published_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  search_vector tsvector GENERATED ALWAYS AS (
    to_tsvector('simple'::regconfig, coalesce(title, '') || ' ' || coalesce(description, '') || ' ' || coalesce(location_text, ''))
  ) STORED,
  CHECK (salary_min_kopecks IS NULL OR salary_max_kopecks IS NULL OR salary_min_kopecks <= salary_max_kopecks),
  CHECK (status NOT IN ('PUBLISHED','CLOSED') OR published_at IS NOT NULL)
);
CREATE UNIQUE INDEX jobs_owner_slug_unique ON jobs(customer_user_id, slug) WHERE deleted_at IS NULL;
CREATE INDEX jobs_status_published_idx ON jobs(status, published_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX jobs_category_status_published_idx ON jobs(category_id, status, published_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX jobs_customer_status_created_idx ON jobs(customer_user_id, status, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX jobs_search_vector_idx ON jobs USING gin(search_vector);
CREATE INDEX jobs_title_trgm_idx ON jobs USING gin(title gin_trgm_ops);

CREATE TABLE job_skills (
  job_id uuid NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  skill_id uuid NOT NULL REFERENCES skills(id),
  PRIMARY KEY (job_id, skill_id)
);

CREATE TABLE job_applications (
  id uuid PRIMARY KEY,
  job_id uuid NOT NULL REFERENCES jobs(id),
  user_id uuid NOT NULL REFERENCES users(id),
  cover_message text CHECK (cover_message IS NULL OR char_length(cover_message) <= 5000),
  status text NOT NULL DEFAULT 'SUBMITTED' CHECK (status IN ('SUBMITTED','VIEWED','SHORTLISTED','REJECTED','ACCEPTED','WITHDRAWN')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (job_id, user_id)
);
CREATE INDEX job_applications_user_created_idx ON job_applications(user_id, created_at DESC, id DESC);
CREATE INDEX job_applications_job_created_idx ON job_applications(job_id, created_at DESC, id DESC);
