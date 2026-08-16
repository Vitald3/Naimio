-- Phase 9 follow-up: privacy-safe engagement events for PRO analytics.

CREATE TABLE IF NOT EXISTS profile_engagement_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  subject_user_id uuid NOT NULL REFERENCES users(id),
  viewer_user_id uuid REFERENCES users(id),
  event_type text NOT NULL CHECK(event_type IN('PROFILE_VIEW','PORTFOLIO_VIEW','SERVICE_VIEW')),
  entity_type text NOT NULL CHECK(entity_type IN('PROFILE','PORTFOLIO','SERVICE')),
  entity_id uuid,
  dedupe_key varchar(220) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK(viewer_user_id IS NULL OR viewer_user_id <> subject_user_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS profile_engagement_events_dedupe_uidx ON profile_engagement_events(dedupe_key);
CREATE INDEX IF NOT EXISTS profile_engagement_events_subject_type_created_idx
  ON profile_engagement_events(subject_user_id, event_type, created_at DESC);
