-- name: GetOwnedProject :one
SELECT * FROM projects WHERE id = $1 AND customer_user_id = $2 AND deleted_at IS NULL;

-- name: ListOwnedProjects :many
SELECT * FROM projects
WHERE customer_user_id = $1 AND deleted_at IS NULL
  AND ($2::timestamptz IS NULL OR (created_at, id) < ($2, $3))
ORDER BY created_at DESC, id DESC LIMIT $4;

-- name: ListPublicProjects :many
SELECT * FROM projects
WHERE status IN ('OPEN','MATCHING') AND visibility = 'PUBLIC' AND deleted_at IS NULL
  AND ($1::uuid IS NULL OR category_id = $1)
  AND ($2::text IS NULL OR budget_type = $2)
  AND ($3::text IS NULL OR experience_level = $3)
  AND ($4::timestamptz IS NULL OR (published_at, id) < ($4, $5))
ORDER BY published_at DESC, id DESC LIMIT $6;

-- Mutations and relation replacement run transactionally in the repository.
-- Publish/cancel/complete write their domain event to outbox_events in the same transaction.
