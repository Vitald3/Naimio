-- Production-hardening additions: independent profile metadata, durable chat audio,
-- and a non-login support identity used for audited moderation messages.
ALTER TABLE users ADD COLUMN IF NOT EXISTS gender text NOT NULL DEFAULT 'UNSPECIFIED'
  CHECK (gender IN ('UNSPECIFIED','MALE','FEMALE'));

ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_type_check;
ALTER TABLE messages ADD CONSTRAINT messages_type_check CHECK(type IN('TEXT','IMAGE','FILE','AUDIO','SYSTEM'));
ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_check;
ALTER TABLE messages ADD CONSTRAINT messages_check CHECK(body IS NOT NULL OR deleted_at IS NOT NULL OR type IN('IMAGE','FILE','AUDIO','SYSTEM'));

INSERT INTO users(id,email,email_normalized,password_hash,username,username_normalized,display_name,status)
VALUES('00000000-0000-4000-8000-000000000011','support-system@naimio.invalid','support-system@naimio.invalid','!disabled!','naimio-support','naimio-support','Поддержка Naimio','ACTIVE')
ON CONFLICT (id) DO UPDATE SET display_name=EXCLUDED.display_name,updated_at=now();
