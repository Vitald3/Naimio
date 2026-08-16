ALTER TABLE media_objects
  ADD COLUMN IF NOT EXISTS purpose text NOT NULL DEFAULT 'PORTFOLIO',
  ADD COLUMN IF NOT EXISTS uploaded_at timestamptz,
  ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE media_objects
  ADD CONSTRAINT media_objects_purpose_check CHECK (purpose IN ('PORTFOLIO'));

CREATE INDEX IF NOT EXISTS media_objects_owner_active_idx
  ON media_objects(owner_user_id, created_at DESC, id) WHERE deleted_at IS NULL;
