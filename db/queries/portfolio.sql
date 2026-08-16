-- name: GetOwnedPortfolioItem :one
SELECT * FROM portfolio_items WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL;

-- name: ListPublicPortfolioByUsername :many
SELECT pi.*
FROM portfolio_items pi JOIN users u ON u.id = pi.user_id
WHERE u.username_normalized = lower($1) AND u.status = 'ACTIVE' AND u.deleted_at IS NULL
  AND pi.visibility = 'PUBLIC' AND pi.deleted_at IS NULL
ORDER BY pi.sort_order, pi.created_at DESC, pi.id
LIMIT $2;

-- Create/update/delete and category/skill/media replacement run transactionally in the repository.
