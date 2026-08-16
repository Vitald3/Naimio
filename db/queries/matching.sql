-- name: LatestMatchingRun :one
SELECT * FROM matching_runs WHERE project_id=$1 ORDER BY created_at DESC,id DESC LIMIT 1;

-- Candidate retrieval is an explicit bounded aggregate query in the matching repository.
-- Run persistence, candidate rows, manual recommendations and quality events are transactional/audited there.
