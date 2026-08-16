-- name: CreateReview :one
INSERT INTO reviews(project_id,reviewer_user_id,reviewee_user_id,reviewer_role,rating_overall,would_work_again,text)
VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,'')) RETURNING id;

-- name: ListPublishedReviewsForUser :many
SELECT id,project_id,reviewer_user_id,reviewee_user_id,reviewer_role,rating_overall,would_work_again,text,status,created_at,updated_at
FROM reviews WHERE reviewee_user_id=$1 AND status='PUBLISHED'
ORDER BY created_at DESC,id DESC LIMIT $2;

-- name: GetUserTrustStats :one
SELECT user_id,native_rating,reviews_count,completed_projects_count,recommendation_rate,updated_at
FROM user_trust_stats WHERE user_id=$1;
