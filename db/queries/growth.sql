-- name: InviteByTokenHash :one
SELECT id,inviter_user_id,invite_type,project_id,expires_at,accepted_by_user_id,accepted_at FROM invites WHERE token_hash=$1;
-- name: ReferralAttributionForUser :one
SELECT id,inviter_user_id,invited_user_id,invite_id,source,first_touch_at FROM referral_attributions WHERE invited_user_id=$1;
-- name: RewardLedgerForUser :many
SELECT id,rule_id,event_key,reward_type,amount,unit,expires_at,created_at FROM reward_ledger WHERE user_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2;
-- name: CustomerTeam :many
SELECT customer_user_id,freelancer_user_id,label,notes,created_at,updated_at FROM customer_team_members WHERE customer_user_id=$1 ORDER BY updated_at DESC,freelancer_user_id DESC;
