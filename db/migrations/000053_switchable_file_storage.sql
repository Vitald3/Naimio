-- Switchable file storage migration
ALTER TABLE media_objects
  ADD COLUMN IF NOT EXISTS storage_provider text NOT NULL DEFAULT 'local';

ALTER TABLE media_objects
  DROP CONSTRAINT IF EXISTS media_objects_storage_provider_check;

ALTER TABLE media_objects
  ADD CONSTRAINT media_objects_storage_provider_check CHECK (storage_provider IN ('local', 's3'));

CREATE INDEX IF NOT EXISTS media_objects_storage_provider_idx
  ON media_objects(storage_provider, created_at DESC) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS storage_credentials (
  provider text PRIMARY KEY CHECK (provider IN ('s3')),
  encrypted_config text NOT NULL,
  updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
);
REVOKE ALL ON storage_credentials FROM PUBLIC;

INSERT INTO feature_flags (key, enabled, description, config)
VALUES (
  'file_storage',
  true,
  'Настройки файлового хранилища (Локальный сервер / S3)',
  '{"provider":"local","s3":{"endpoint":"","region":"ru-central1","bucket":"","access_key":"","use_ssl":true,"path_style":false,"public_url":""}}'::jsonb
)
ON CONFLICT (key) DO NOTHING;
