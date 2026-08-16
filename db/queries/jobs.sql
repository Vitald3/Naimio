-- name: ListPublishedJobs :many
SELECT id, title, slug, employment_type, salary_min_kopecks, salary_max_kopecks, currency,
       location_text, remote, experience_level, published_at
FROM jobs
WHERE status = 'PUBLISHED' AND moderation_status = 'VISIBLE' AND deleted_at IS NULL
ORDER BY published_at DESC, id DESC
LIMIT $1;

-- name: ListJobApplicationsForOwner :many
SELECT a.id, a.job_id, a.user_id, a.cover_message, a.status, a.created_at, a.updated_at
FROM job_applications a JOIN jobs j ON j.id = a.job_id
WHERE j.id = $1 AND j.customer_user_id = $2
ORDER BY a.created_at DESC, a.id DESC;
