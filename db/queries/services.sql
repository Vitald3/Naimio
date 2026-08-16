-- name: GetOwnedService :one
SELECT * FROM services WHERE id = $1 AND seller_user_id = $2 AND deleted_at IS NULL;

-- name: ListOwnedServices :many
SELECT * FROM services
WHERE seller_user_id = $1 AND deleted_at IS NULL
  AND ($2::timestamptz IS NULL OR (created_at, id) < ($2, $3))
ORDER BY created_at DESC, id DESC LIMIT $4;

-- name: ListPublicServices :many
SELECT * FROM services
WHERE status = 'ACTIVE' AND visibility = 'PUBLIC' AND deleted_at IS NULL
  AND ($1::uuid IS NULL OR category_id = $1)
  AND ($2::text IS NULL OR service_type = $2)
  AND ($3::text IS NULL OR price_type = $3)
  AND ($4::timestamptz IS NULL OR (published_at, id) < ($4, $5))
ORDER BY published_at DESC, id DESC LIMIT $6;

-- Create/update and skill/media/education replacement run transactionally in the repository.
-- Publish/pause/resume use status predicates and check affected rows.
