-- Keep rejected owner-editable content in a real pre-publication state.
UPDATE projects
SET status='DRAFT', published_at=NULL, updated_at=now()
WHERE moderation_status='HIDDEN'
  AND moderation_reason IS NOT NULL
  AND status IN ('OPEN','MATCHING','PUBLISHED')
  AND deleted_at IS NULL;

UPDATE jobs
SET status='DRAFT', published_at=NULL, updated_at=now()
WHERE moderation_status='HIDDEN'
  AND moderation_reason IS NOT NULL
  AND status='PUBLISHED'
  AND deleted_at IS NULL;
