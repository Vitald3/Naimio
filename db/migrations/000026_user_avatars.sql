ALTER TABLE media_objects DROP CONSTRAINT IF EXISTS media_objects_purpose_check;
ALTER TABLE media_objects ADD CONSTRAINT media_objects_purpose_check
  CHECK (purpose IN ('PORTFOLIO','SERVICE','PROJECT','CHAT','AVATAR'));

ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar_media_object_id uuid REFERENCES media_objects(id);
CREATE INDEX IF NOT EXISTS users_avatar_media_idx ON users(avatar_media_object_id) WHERE avatar_media_object_id IS NOT NULL;
