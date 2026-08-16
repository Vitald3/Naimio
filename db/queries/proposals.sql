-- name: ListProjectProposals :many
SELECT * FROM proposals WHERE project_id=$1 AND ($2::timestamptz IS NULL OR (submitted_at,id)<($2,$3))
ORDER BY (status='SHORTLISTED') DESC,submitted_at DESC,id DESC LIMIT $4;

-- name: ListFreelancerProposals :many
SELECT * FROM proposals WHERE freelancer_user_id=$1 AND ($2::timestamptz IS NULL OR (submitted_at,id)<($2,$3))
ORDER BY submitted_at DESC,id DESC LIMIT $4;

-- Acceptance is implemented in one repository transaction: lock project/proposals, accept one proposal,
-- reject competing proposals, create assignment, transition project, update counter, and insert outbox event.
