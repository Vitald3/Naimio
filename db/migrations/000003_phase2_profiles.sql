CREATE TABLE IF NOT EXISTS professional_profiles (
  user_id uuid PRIMARY KEY REFERENCES users(id),
  professional_title varchar(160),
  bio text,
  location_text varchar(160),
  country_code char(2),
  experience_years smallint CHECK (experience_years IS NULL OR experience_years BETWEEN 0 AND 80),
  hourly_rate_kopecks bigint CHECK (hourly_rate_kopecks IS NULL OR hourly_rate_kopecks >= 0),
  minimum_order_kopecks bigint CHECK (minimum_order_kopecks IS NULL OR minimum_order_kopecks >= 0),
  availability text NOT NULL DEFAULT 'UNAVAILABLE' CHECK (availability IN ('AVAILABLE','PARTIALLY_BUSY','BUSY','UNAVAILABLE')),
  profile_visibility text NOT NULL DEFAULT 'PUBLIC' CHECK (profile_visibility IN ('PUBLIC','PRIVATE')),
  response_time_minutes int CHECK (response_time_minutes IS NULL OR response_time_minutes >= 0),
  profile_completion smallint NOT NULL DEFAULT 0 CHECK (profile_completion BETWEEN 0 AND 100),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS professional_profiles_availability_idx ON professional_profiles(availability, updated_at DESC);
CREATE INDEX IF NOT EXISTS professional_profiles_hourly_rate_idx ON professional_profiles(hourly_rate_kopecks);

CREATE TABLE IF NOT EXISTS profile_categories (
  user_id uuid NOT NULL REFERENCES users(id), category_id uuid NOT NULL REFERENCES categories(id),
  is_primary boolean NOT NULL DEFAULT false, sort_order smallint NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, category_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS profile_categories_primary_idx ON profile_categories(user_id) WHERE is_primary;

CREATE TABLE IF NOT EXISTS profile_skills (
  user_id uuid NOT NULL REFERENCES users(id), skill_id uuid NOT NULL REFERENCES skills(id),
  level text CHECK (level IS NULL OR level IN ('BEGINNER','INTERMEDIATE','ADVANCED','EXPERT')),
  years smallint CHECK (years IS NULL OR years BETWEEN 0 AND 80), is_featured boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY (user_id, skill_id)
);

CREATE TABLE IF NOT EXISTS profile_languages (
  user_id uuid NOT NULL REFERENCES users(id), language_code varchar(10) NOT NULL, level text NOT NULL,
  PRIMARY KEY (user_id, language_code)
);
