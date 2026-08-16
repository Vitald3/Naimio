-- name: CreateMediaObject :one
INSERT INTO media_objects
  (id, owner_user_id, object_key, bucket, original_filename, mime_type, size_bytes, purpose)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetOwnedMediaObject :one
SELECT * FROM media_objects
WHERE id = $1 AND owner_user_id = $2 AND deleted_at IS NULL;

-- name: CompleteOwnedMediaObject :one
UPDATE media_objects
SET uploaded_at = COALESCE(uploaded_at, now()), updated_at = now()
WHERE id = $1 AND owner_user_id = $2 AND deleted_at IS NULL
RETURNING *;

-- name: DeleteOwnedMediaObject :execrows
UPDATE media_objects SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND owner_user_id = $2 AND deleted_at IS NULL;

-- Scan results are written by the trusted scanning boundary, never by the public API.
