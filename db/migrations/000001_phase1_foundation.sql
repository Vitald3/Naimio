CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
  id uuid PRIMARY KEY,
  email varchar(320) NOT NULL,
  email_normalized varchar(320) NOT NULL,
  email_verified_at timestamptz,
  password_hash text NOT NULL,
  username varchar(40),
  username_normalized varchar(40),
  display_name varchar(120) NOT NULL,
  avatar_object_key text,
  locale varchar(10) NOT NULL DEFAULT 'ru',
  timezone varchar(64) NOT NULL DEFAULT 'Europe/Moscow',
  status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','SUSPENDED','BANNED','DELETED')),
  last_seen_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  CHECK (username IS NULL OR char_length(username) BETWEEN 3 AND 40)
);
CREATE UNIQUE INDEX IF NOT EXISTS users_email_unique ON users(email_normalized) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS users_username_unique ON users(username_normalized) WHERE username_normalized IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS users_status_created_idx ON users(status, created_at DESC);

CREATE TABLE IF NOT EXISTS user_capabilities (
  user_id uuid NOT NULL REFERENCES users(id),
  capability text NOT NULL CHECK (capability IN ('CUSTOMER','FREELANCER')),
  enabled_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, capability)
);
CREATE TABLE IF NOT EXISTS user_roles (
  user_id uuid NOT NULL REFERENCES users(id),
  role text NOT NULL CHECK (role IN ('MODERATOR','ADMIN','SUPER_ADMIN')),
  granted_by uuid REFERENCES users(id),
  granted_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, role)
);

CREATE TABLE IF NOT EXISTS sessions (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id),
  token_hash bytea NOT NULL UNIQUE,
  user_agent text,
  ip inet,
  last_used_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS sessions_user_expiry_idx ON sessions(user_id, expires_at DESC);
CREATE INDEX IF NOT EXISTS sessions_active_expiry_idx ON sessions(expires_at) WHERE revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS auth_tokens (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id),
  purpose text NOT NULL CHECK (purpose IN ('VERIFY_EMAIL','RESET_PASSWORD','CHANGE_EMAIL')),
  token_hash bytea NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS auth_tokens_user_purpose_idx ON auth_tokens(user_id, purpose, created_at DESC);

CREATE TABLE IF NOT EXISTS login_events (
  id uuid PRIMARY KEY,
  user_id uuid REFERENCES users(id),
  email_normalized varchar(320),
  success boolean NOT NULL,
  failure_code text,
  ip inet,
  user_agent text,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS login_events_user_created_idx ON login_events(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS login_events_email_created_idx ON login_events(email_normalized, created_at DESC);
CREATE INDEX IF NOT EXISTS login_events_ip_created_idx ON login_events(ip, created_at DESC);

CREATE TABLE IF NOT EXISTS feature_flags (
  key varchar(100) PRIMARY KEY,
  enabled boolean NOT NULL DEFAULT false,
  config jsonb NOT NULL DEFAULT '{}'::jsonb,
  updated_by uuid REFERENCES users(id),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS audit_logs (
  id uuid PRIMARY KEY,
  actor_user_id uuid REFERENCES users(id),
  action varchar(100) NOT NULL,
  target_type varchar(80),
  target_id uuid,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  ip inet,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS audit_logs_actor_created_idx ON audit_logs(actor_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS audit_logs_target_created_idx ON audit_logs(target_type, target_id, created_at DESC);
CREATE INDEX IF NOT EXISTS audit_logs_action_created_idx ON audit_logs(action, created_at DESC);

CREATE TABLE IF NOT EXISTS outbox_events (
  id uuid PRIMARY KEY,
  aggregate_type varchar(80) NOT NULL,
  aggregate_id uuid NOT NULL,
  event_type varchar(120) NOT NULL,
  payload jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  available_at timestamptz NOT NULL DEFAULT now(),
  published_at timestamptz,
  attempts int NOT NULL DEFAULT 0,
  last_error text
);
CREATE INDEX IF NOT EXISTS outbox_available_idx ON outbox_events(available_at, created_at) WHERE published_at IS NULL;
CREATE INDEX IF NOT EXISTS outbox_aggregate_idx ON outbox_events(aggregate_type, aggregate_id, created_at);
