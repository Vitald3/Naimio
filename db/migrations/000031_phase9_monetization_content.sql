-- Phase 9: provider-neutral PRO subscriptions, entitlements and editorial content.

INSERT INTO feature_flags(key,enabled,config,description)
VALUES
  ('pro_subscriptions_enabled',true,'{}'::jsonb,'Публичные PRO-поверхности и применение PRO-привилегий'),
  ('blog_enabled',true,'{}'::jsonb,'Публичный блог и его навигация')
ON CONFLICT(key) DO NOTHING;

CREATE TABLE IF NOT EXISTS subscription_plans (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code varchar(80) NOT NULL UNIQUE CHECK(code ~ '^[a-z0-9]+(?:_[a-z0-9]+)*$'),
  name varchar(120) NOT NULL CHECK(char_length(name) BETWEEN 1 AND 120),
  description text NOT NULL DEFAULT '' CHECK(char_length(description)<=1000),
  tier text NOT NULL CHECK(tier IN('FREE','PRO')),
  billing_period text NOT NULL CHECK(billing_period IN('NONE','MONTH','YEAR')),
  currency char(3) NOT NULL DEFAULT 'RUB' CHECK(currency ~ '^[A-Z]{3}$'),
  amount_kopecks bigint NOT NULL DEFAULT 0 CHECK(amount_kopecks>=0),
  active boolean NOT NULL DEFAULT true,
  display_order int NOT NULL DEFAULT 0 CHECK(display_order BETWEEN 0 AND 10000),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK((tier='FREE' AND billing_period='NONE' AND amount_kopecks=0) OR
        (tier='PRO' AND billing_period IN('MONTH','YEAR') AND amount_kopecks>0))
);
CREATE UNIQUE INDEX IF NOT EXISTS subscription_plans_free_unique ON subscription_plans(tier) WHERE tier='FREE';
CREATE INDEX IF NOT EXISTS subscription_plans_public_idx ON subscription_plans(active,display_order,created_at);

CREATE TABLE IF NOT EXISTS subscription_plan_entitlements (
  plan_id uuid NOT NULL REFERENCES subscription_plans(id) ON DELETE CASCADE,
  feature_key varchar(120) NOT NULL CHECK(feature_key ~ '^[a-z][a-z0-9_]*(?:\.[a-z][a-z0-9_]*)+$'),
  kind text NOT NULL CHECK(kind IN('BOOLEAN','LIMIT')),
  enabled boolean NOT NULL DEFAULT true,
  limit_value bigint CHECK(limit_value IS NULL OR limit_value>=0),
  unlimited boolean NOT NULL DEFAULT false,
  config jsonb NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(config)='object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(plan_id,feature_key),
  CHECK((kind='BOOLEAN' AND limit_value IS NULL AND unlimited=false) OR
        (kind='LIMIT' AND enabled=true AND ((unlimited=true AND limit_value IS NULL) OR
                                            (unlimited=false AND limit_value IS NOT NULL))))
);
CREATE INDEX IF NOT EXISTS subscription_plan_entitlements_feature_idx ON subscription_plan_entitlements(feature_key,plan_id);

CREATE TABLE IF NOT EXISTS user_subscriptions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id),
  plan_id uuid NOT NULL REFERENCES subscription_plans(id),
  status text NOT NULL CHECK(status IN('PENDING','ACTIVE','PAST_DUE','CANCELED','EXPIRED')),
  starts_at timestamptz NOT NULL,
  current_period_start timestamptz NOT NULL,
  current_period_end timestamptz NOT NULL,
  cancel_at_period_end boolean NOT NULL DEFAULT false,
  canceled_at timestamptz,
  provider varchar(80),
  provider_customer_id text,
  provider_subscription_id text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK(current_period_start>=starts_at),
  CHECK(current_period_end>current_period_start),
  CHECK(canceled_at IS NULL OR status IN('CANCELED','EXPIRED')),
  CHECK((provider IS NULL AND provider_customer_id IS NULL AND provider_subscription_id IS NULL) OR provider IS NOT NULL)
);
CREATE UNIQUE INDEX IF NOT EXISTS user_subscriptions_provider_unique
  ON user_subscriptions(provider,provider_subscription_id) WHERE provider_subscription_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS user_subscriptions_one_current
  ON user_subscriptions(user_id) WHERE status IN('PENDING','ACTIVE','PAST_DUE');
CREATE INDEX IF NOT EXISTS user_subscriptions_user_history_idx ON user_subscriptions(user_id,created_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS user_subscriptions_status_expiry_idx ON user_subscriptions(status,current_period_end,id);

CREATE TABLE IF NOT EXISTS subscription_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  subscription_id uuid NOT NULL REFERENCES user_subscriptions(id),
  event_type varchar(100) NOT NULL,
  from_status text,
  to_status text,
  actor_user_id uuid REFERENCES users(id),
  reason text,
  provider_event_id text,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(metadata)='object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK(from_status IS NULL OR from_status IN('PENDING','ACTIVE','PAST_DUE','CANCELED','EXPIRED')),
  CHECK(to_status IS NULL OR to_status IN('PENDING','ACTIVE','PAST_DUE','CANCELED','EXPIRED')),
  CHECK(reason IS NULL OR char_length(reason)<=1000)
);
CREATE UNIQUE INDEX IF NOT EXISTS subscription_events_provider_unique ON subscription_events(provider_event_id) WHERE provider_event_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS subscription_events_subscription_idx ON subscription_events(subscription_id,created_at DESC,id DESC);

CREATE TABLE IF NOT EXISTS blog_categories (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name varchar(100) NOT NULL CHECK(char_length(name) BETWEEN 1 AND 100),
  slug varchar(120) NOT NULL UNIQUE CHECK(slug ~ '^[a-z0-9]+(?:-[a-z0-9]+)*$'),
  description text NOT NULL DEFAULT '' CHECK(char_length(description)<=500),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS blog_tags (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name varchar(80) NOT NULL CHECK(char_length(name) BETWEEN 1 AND 80),
  slug varchar(100) NOT NULL UNIQUE CHECK(slug ~ '^[a-z0-9]+(?:-[a-z0-9]+)*$'),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS blog_posts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  author_user_id uuid NOT NULL REFERENCES users(id),
  category_id uuid REFERENCES blog_categories(id),
  cover_media_object_id uuid REFERENCES media_objects(id),
  title varchar(220) NOT NULL CHECK(char_length(title) BETWEEN 1 AND 220),
  slug varchar(240) NOT NULL UNIQUE CHECK(slug ~ '^[a-z0-9]+(?:-[a-z0-9]+)*$'),
  excerpt text NOT NULL CHECK(char_length(excerpt) BETWEEN 1 AND 600),
  content_html text NOT NULL CHECK(char_length(content_html) BETWEEN 1 AND 200000),
  cover_alt varchar(300),
  status text NOT NULL DEFAULT 'DRAFT' CHECK(status IN('DRAFT','SCHEDULED','PUBLISHED','ARCHIVED')),
  seo_title varchar(220),
  seo_description varchar(320),
  canonical_url text,
  published_at timestamptz,
  scheduled_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK(cover_alt IS NULL OR char_length(cover_alt)<=300),
  CHECK(seo_title IS NULL OR char_length(seo_title)<=220),
  CHECK(seo_description IS NULL OR char_length(seo_description)<=320),
  CHECK(canonical_url IS NULL OR char_length(canonical_url)<=2048),
  CHECK((status='SCHEDULED' AND scheduled_at IS NOT NULL AND published_at IS NULL) OR status<>'SCHEDULED'),
  CHECK((status='PUBLISHED' AND published_at IS NOT NULL) OR status<>'PUBLISHED')
);
CREATE INDEX IF NOT EXISTS blog_posts_public_idx ON blog_posts(published_at DESC,id DESC) WHERE status='PUBLISHED';
CREATE INDEX IF NOT EXISTS blog_posts_schedule_idx ON blog_posts(scheduled_at,id) WHERE status='SCHEDULED';
CREATE INDEX IF NOT EXISTS blog_posts_admin_idx ON blog_posts(status,updated_at DESC,id DESC);
CREATE TABLE IF NOT EXISTS blog_post_tags (
  post_id uuid NOT NULL REFERENCES blog_posts(id) ON DELETE CASCADE,
  tag_id uuid NOT NULL REFERENCES blog_tags(id) ON DELETE CASCADE,
  PRIMARY KEY(post_id,tag_id)
);
CREATE INDEX IF NOT EXISTS blog_post_tags_tag_idx ON blog_post_tags(tag_id,post_id);

ALTER TABLE media_objects DROP CONSTRAINT IF EXISTS media_objects_purpose_check;
ALTER TABLE media_objects ADD CONSTRAINT media_objects_purpose_check
  CHECK(purpose IN('PORTFOLIO','SERVICE','PROJECT','CHAT','AVATAR','BLOG_COVER','BLOG_CONTENT'));
