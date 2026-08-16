-- name: ListOwnedExternalReputations :many
SELECT id, user_id, platform, profile_url, external_username, rating, reviews_count,
       completed_orders_count, account_since, verification_status, verification_method,
       verified_at, expires_at, last_checked_at, created_at, updated_at
FROM external_reputations
WHERE user_id = $1
ORDER BY created_at DESC, id DESC;

-- name: CreateExternalReputation :one
INSERT INTO external_reputations (user_id, platform, profile_url, external_username)
VALUES ($1, $2, $3, NULLIF($4, ''))
RETURNING id;

-- name: UpdateOwnedUnverifiedExternalReputation :execrows
UPDATE external_reputations
SET platform = $3, profile_url = $4, external_username = NULLIF($5, ''), updated_at = now()
WHERE id = $1 AND user_id = $2 AND verification_status = 'UNVERIFIED';

-- name: DeleteOwnedExternalReputation :execrows
DELETE FROM external_reputations WHERE id = $1 AND user_id = $2;

-- name: ListPublicVerifiedExternalReputations :many
SELECT er.platform, er.profile_url, er.rating, er.reviews_count, er.completed_orders_count,
       er.account_since, er.verified_at
FROM external_reputations er
JOIN users u ON u.id = er.user_id
JOIN professional_profiles p ON p.user_id = u.id
WHERE u.username_normalized = lower($1)
  AND u.status = 'ACTIVE' AND u.deleted_at IS NULL
  AND p.profile_visibility = 'PUBLIC'
  AND er.verification_status = 'VERIFIED'
ORDER BY er.verified_at DESC NULLS LAST, er.id;

-- name: CreateReputationVerificationChallenge :one
INSERT INTO reputation_verification_challenges (external_reputation_id, method, code_hash, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING id;

-- name: GetOwnedReputationVerificationChallenge :one
SELECT c.id, c.external_reputation_id, c.method, c.expires_at, c.attempts, c.status, c.created_at, c.verified_at
FROM reputation_verification_challenges c
JOIN external_reputations er ON er.id = c.external_reputation_id
WHERE c.external_reputation_id = $1 AND er.user_id = $2 AND c.status = 'PENDING';

-- name: ListPendingExternalReputations :many
SELECT er.id, er.user_id, er.platform, er.profile_url, er.external_username, er.rating, er.reviews_count,
       er.completed_orders_count, er.account_since, er.evidence, er.created_at,
       c.id AS challenge_id, c.method, c.expires_at, c.attempts
FROM external_reputations er
LEFT JOIN reputation_verification_challenges c ON c.external_reputation_id = er.id AND c.status = 'PENDING'
WHERE er.verification_status = 'PENDING'
ORDER BY er.created_at, er.id;
