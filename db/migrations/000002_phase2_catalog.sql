CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS categories (
  id uuid PRIMARY KEY,
  parent_id uuid REFERENCES categories(id),
  slug varchar(120) NOT NULL UNIQUE,
  name varchar(160) NOT NULL,
  description text,
  sort_order int NOT NULL DEFAULT 0,
  is_active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS categories_parent_order_idx ON categories(parent_id, sort_order, name);
CREATE INDEX IF NOT EXISTS categories_active_parent_idx ON categories(is_active, parent_id);

CREATE TABLE IF NOT EXISTS skills (
  id uuid PRIMARY KEY,
  slug varchar(120) NOT NULL UNIQUE,
  name varchar(160) NOT NULL,
  is_active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS skills_name_trgm_idx ON skills USING gin (name gin_trgm_ops);

CREATE TABLE IF NOT EXISTS category_skills (
  category_id uuid NOT NULL REFERENCES categories(id),
  skill_id uuid NOT NULL REFERENCES skills(id),
  weight smallint NOT NULL DEFAULT 100,
  PRIMARY KEY (category_id, skill_id)
);
