-- 000054_storage_backends.sql
-- Storage Backends for persistent multi-endpoint S3 configuration identity

CREATE TABLE IF NOT EXISTS storage_backends (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  provider text NOT NULL CHECK (provider IN ('local', 's3')),
  endpoint text NOT NULL DEFAULT '',
  region text NOT NULL DEFAULT '',
  bucket text NOT NULL DEFAULT '',
  access_key_id text NOT NULL DEFAULT '',
  secret_key_encrypted text NOT NULL DEFAULT '',
  use_ssl boolean NOT NULL DEFAULT false,
  force_path_style boolean NOT NULL DEFAULT true,
  public_url text NOT NULL DEFAULT '',
  is_active boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE media_objects
  ADD COLUMN IF NOT EXISTS storage_backend_id uuid REFERENCES storage_backends(id) ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS media_objects_storage_backend_idx ON media_objects(storage_backend_id) WHERE deleted_at IS NULL;

-- Backfill legacy records from feature_flags, storage_credentials and existing media_objects
DO $$
DECLARE
  v_ff_config JSONB;
  v_cred TEXT;
  v_s3_id UUID;
  v_local_id UUID := '00000000-0000-0000-0000-000000000001'::uuid;
  v_endpoint TEXT;
  v_region TEXT;
  v_bucket TEXT;
  v_access_key TEXT;
  v_use_ssl BOOLEAN;
  v_path_style BOOLEAN;
  v_public_url TEXT;
  v_active_provider TEXT := 'local';
  v_s3_active BOOLEAN := false;
BEGIN
  SELECT config INTO v_ff_config FROM feature_flags WHERE key = 'file_storage';
  SELECT encrypted_config INTO v_cred FROM storage_credentials WHERE provider = 's3';

  IF v_ff_config IS NOT NULL THEN
    v_active_provider := COALESCE(v_ff_config->>'provider', 'local');
    IF (v_ff_config->'s3') IS NOT NULL THEN
      v_endpoint := COALESCE(v_ff_config->'s3'->>'endpoint', '');
      v_region := COALESCE(v_ff_config->'s3'->>'region', 'ru-central1');
      v_bucket := COALESCE(v_ff_config->'s3'->>'bucket', '');
      v_access_key := COALESCE(v_ff_config->'s3'->>'access_key', '');
      v_use_ssl := COALESCE((v_ff_config->'s3'->>'use_ssl')::boolean, false);
      v_path_style := COALESCE((v_ff_config->'s3'->>'path_style')::boolean, true);
      v_public_url := COALESCE(v_ff_config->'s3'->>'public_url', '');
    END IF;
  END IF;

  IF v_active_provider = 's3' AND v_endpoint != '' THEN
    v_s3_active := true;
  END IF;

  -- Insert or update default local backend
  INSERT INTO storage_backends (id, provider, bucket, is_active)
  VALUES (v_local_id, 'local', 'local-media', NOT v_s3_active)
  ON CONFLICT (id) DO UPDATE SET is_active = NOT v_s3_active;

  -- Backfill legacy local media objects to default local backend
  UPDATE media_objects
  SET storage_backend_id = v_local_id
  WHERE storage_backend_id IS NULL AND (storage_provider = 'local' OR storage_provider IS NULL);

  -- If S3 configuration was present in feature_flags/credentials, insert S3 backend
  IF v_endpoint != '' AND v_bucket != '' THEN
    INSERT INTO storage_backends (provider, endpoint, region, bucket, access_key_id, secret_key_encrypted, use_ssl, force_path_style, public_url, is_active)
    VALUES ('s3', v_endpoint, v_region, v_bucket, v_access_key, COALESCE(v_cred, ''), v_use_ssl, v_path_style, v_public_url, v_s3_active)
    RETURNING id INTO v_s3_id;

    UPDATE media_objects
    SET storage_backend_id = v_s3_id
    WHERE storage_backend_id IS NULL AND storage_provider = 's3';
  END IF;

  -- For any remaining S3 media objects without backend ID, create corresponding S3 backends
  FOR v_bucket IN SELECT DISTINCT COALESCE(bucket, 'files') FROM media_objects WHERE storage_backend_id IS NULL AND storage_provider = 's3' LOOP
    INSERT INTO storage_backends (provider, bucket, is_active)
    VALUES ('s3', v_bucket, false)
    RETURNING id INTO v_s3_id;

    UPDATE media_objects
    SET storage_backend_id = v_s3_id
    WHERE storage_backend_id IS NULL AND storage_provider = 's3' AND COALESCE(bucket, 'files') = v_bucket;
  END LOOP;
END $$;

-- Enforce exactly at most one active storage backend in database
CREATE UNIQUE INDEX IF NOT EXISTS storage_backends_single_active_idx ON storage_backends ((is_active)) WHERE is_active = true;
