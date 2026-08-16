-- name: ListFavorites :many
SELECT * FROM favorites WHERE user_id=$1 AND ($2::text IS NULL OR entity_type=$2)
AND ($3::timestamptz IS NULL OR(created_at,entity_id)<($3,$4)) ORDER BY created_at DESC,entity_id DESC LIMIT $5;
