-- name: GetPublicProfileByUsername :one
SELECT p.user_id, u.username, u.display_name, p.professional_title, p.bio, p.location_text, p.country_code,
       p.experience_years, p.hourly_rate_kopecks, p.minimum_order_kopecks, p.response_time_minutes,
       p.availability, p.profile_visibility
FROM professional_profiles p
JOIN users u ON u.id = p.user_id
WHERE u.username_normalized = lower($1)
  AND u.status = 'ACTIVE' AND u.deleted_at IS NULL
  AND p.profile_visibility = 'PUBLIC';

-- name: ListPublicFreelancers :many
SELECT p.user_id, u.username, u.display_name, p.professional_title, p.country_code, p.experience_years,
       p.hourly_rate_kopecks, p.response_time_minutes, p.availability
FROM professional_profiles p
JOIN users u ON u.id = p.user_id
WHERE u.status = 'ACTIVE' AND u.deleted_at IS NULL
  AND u.username IS NOT NULL AND p.profile_visibility = 'PUBLIC'
ORDER BY u.username_normalized, u.id
LIMIT $1;

-- name: UpdateProfessionalProfile :execrows
UPDATE professional_profiles
SET professional_title = NULLIF($2, ''), bio = NULLIF($3, ''), location_text = NULLIF($4, ''),
    country_code = NULLIF($5, ''), experience_years = $6, hourly_rate_kopecks = $7,
    minimum_order_kopecks = $8, availability = $9, profile_visibility = $10, updated_at = now()
WHERE user_id = $1;

-- Association replacement is transactional in the repository: delete the actor's existing rows,
-- insert validated active category/skill IDs or normalized languages, then update profile.updated_at.
