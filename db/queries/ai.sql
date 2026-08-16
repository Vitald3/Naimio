-- name: GetProjectDraftByToken :one
SELECT * FROM project_drafts WHERE guest_token_hash=$1 AND expires_at>now();

-- name: GetOwnedProjectDraft :one
SELECT * FROM project_drafts WHERE id=$1 AND owner_user_id=$2 AND expires_at>now();

-- Draft claim/update and AI request accounting are explicit transactional repository operations.
