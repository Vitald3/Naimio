-- Production admin/moderation support. This migration only extends existing Phase 1-8 schema.

ALTER TABLE feature_flags ADD COLUMN IF NOT EXISTS description text;

ALTER TABLE projects ADD COLUMN IF NOT EXISTS moderation_status text NOT NULL DEFAULT 'VISIBLE';
ALTER TABLE projects ADD COLUMN IF NOT EXISTS moderation_reason text;
ALTER TABLE projects ADD COLUMN IF NOT EXISTS moderated_by uuid REFERENCES users(id);
ALTER TABLE projects ADD COLUMN IF NOT EXISTS moderated_at timestamptz;
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='projects_moderation_status_check') THEN
    ALTER TABLE projects ADD CONSTRAINT projects_moderation_status_check CHECK (moderation_status IN ('VISIBLE','HIDDEN'));
  END IF;
END $$;
CREATE INDEX IF NOT EXISTS projects_moderation_status_idx ON projects(moderation_status,status,published_at DESC) WHERE deleted_at IS NULL;

ALTER TABLE services ADD COLUMN IF NOT EXISTS moderation_reason text;
ALTER TABLE services ADD COLUMN IF NOT EXISTS moderated_by uuid REFERENCES users(id);
ALTER TABLE services ADD COLUMN IF NOT EXISTS moderated_at timestamptz;

ALTER TABLE jobs ADD COLUMN IF NOT EXISTS moderation_reason text;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS moderated_by uuid REFERENCES users(id);
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS moderated_at timestamptz;

ALTER TABLE reports ADD COLUMN IF NOT EXISTS reviewed_at timestamptz;
ALTER TABLE reports ADD COLUMN IF NOT EXISTS reviewed_by_user_id uuid REFERENCES users(id);
DO $$ DECLARE n text; BEGIN
  SELECT conname INTO n FROM pg_constraint WHERE conrelid='reports'::regclass AND contype='c' AND pg_get_constraintdef(oid) LIKE '%entity_type%';
  IF n IS NOT NULL THEN EXECUTE format('ALTER TABLE reports DROP CONSTRAINT %I',n); END IF;
END $$;
ALTER TABLE reports ADD CONSTRAINT reports_entity_type_check CHECK(entity_type IN('USER','PROJECT','SERVICE','VACANCY','REVIEW','MESSAGE','PORTFOLIO'));
DO $$ DECLARE n text; BEGIN
  SELECT conname INTO n FROM pg_constraint WHERE conrelid='reports'::regclass AND contype='c' AND pg_get_constraintdef(oid) LIKE '%status%';
  IF n IS NOT NULL THEN EXECUTE format('ALTER TABLE reports DROP CONSTRAINT %I',n); END IF;
END $$;
ALTER TABLE reports ADD CONSTRAINT reports_status_check CHECK(status IN('OPEN','IN_REVIEW','RESOLVED','DISMISSED'));
CREATE INDEX IF NOT EXISTS reports_assigned_status_idx ON reports(assigned_to_user_id,status,created_at DESC);

ALTER TABLE fraud_signals ADD COLUMN IF NOT EXISTS reviewed_by_user_id uuid REFERENCES users(id);
ALTER TABLE fraud_signals ADD COLUMN IF NOT EXISTS resolution text;
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='fraud_signals_status_check') THEN
    ALTER TABLE fraud_signals ADD CONSTRAINT fraud_signals_status_check CHECK(status IN('OPEN','REVIEWING','CONFIRMED','DISMISSED','RESOLVED'));
  END IF;
END $$;
CREATE INDEX IF NOT EXISTS fraud_signals_user_status_idx ON fraud_signals(user_id,status,created_at DESC);
