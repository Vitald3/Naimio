package notification

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

type Event struct {
	ID, Type, AggregateID string
	Payload               map[string]any
}
type Repository interface {
	ProcessOne(context.Context) (bool, error)
}
type Processor struct{ Repository Repository }

func (p Processor) Once(ctx context.Context) error { _, e := p.Repository.ProcessOne(ctx); return e }

type PostgresRepository struct{ DB *sql.DB }

func (r PostgresRepository) ProcessOne(ctx context.Context) (bool, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var event Event
	var raw []byte
	err = tx.QueryRowContext(ctx, `SELECT id::text,event_type,aggregate_id::text,payload
FROM outbox_events
WHERE published_at IS NULL
  AND available_at<=now()
  AND event_type IN('PROPOSAL_CREATED','PROPOSAL_ACCEPTED','PROJECT_COMPLETED','PROJECT_CANCELLED','REVIEW_CREATED','INVITE_ACCEPTED','REWARD_GRANTED','PROJECT_INVITE_CREATED','DEAL_FUNDING_REQUIRED','DEAL_FUNDED','DEAL_STARTED','DEAL_SUBMITTED','DEAL_REVISION_REQUESTED','DISPUTE_OPENED','DISPUTE_RESOLVED','DEAL_ACCEPTED','DEAL_RELEASE_PENDING','DEAL_COMPLETED','DEAL_REFUND_PENDING','DEAL_REFUNDED','DEAL_CANCELED')
ORDER BY created_at,id
FOR UPDATE SKIP LOCKED
LIMIT 1`).Scan(&event.ID, &event.Type, &event.AggregateID, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err = json.Unmarshal(raw, &event.Payload); err != nil {
		return false, err
	}

	targets, notificationType, actorID, err := resolveEventTargets(ctx, tx, event)
	if err != nil {
		return false, err
	}

	seen := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		if _, exists := seen[target]; exists {
			continue
		}
		seen[target] = struct{}{}

		_, err = tx.ExecContext(ctx, `INSERT INTO notifications(
    id,dedupe_key,user_id,type,actor_user_id,entity_type,entity_id,payload
)
SELECT
    gen_random_uuid(),
    'outbox:'||$1::uuid::text||':'||$2::uuid::text,
    $2::uuid,
    $3::text,
    NULLIF($4::text,'')::uuid,
    'event',
    $5::uuid,
    jsonb_build_object('event_type',$6::text,'entity_id',$5::uuid::text)
WHERE COALESCE((
    SELECT np.in_app
    FROM notification_preferences np
    WHERE np.user_id=$2::uuid AND np.event_type=$3::text
),true)
ON CONFLICT(dedupe_key) DO NOTHING`, event.ID, target, notificationType, actorID, event.AggregateID, event.Type)
		if err != nil {
			return false, err
		}

		_, err = tx.ExecContext(ctx, `INSERT INTO email_jobs(dedupe_key,user_id,template,payload)
SELECT
    'outbox-email:'||$1::uuid::text||':'||$2::uuid::text,
    $2::uuid,
    $3::text,
    jsonb_build_object('entity_id',$4::uuid::text,'preferences_path','/settings/notifications')
WHERE COALESCE((
    SELECT np.email
    FROM notification_preferences np
    WHERE np.user_id=$2::uuid AND np.event_type=$3::text
),true)
ON CONFLICT(dedupe_key) DO NOTHING`, event.ID, target, notificationType, event.AggregateID)
		if err != nil {
			return false, err
		}
	}

	if _, err = tx.ExecContext(ctx, `UPDATE outbox_events SET published_at=now(),attempts=attempts+1 WHERE id=$1::uuid`, event.ID); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func resolveEventTargets(ctx context.Context, tx *sql.Tx, event Event) ([]string, string, string, error) {
	notificationType := "project_status_changed"
	actorID := ""
	var targets []string

	one := func(query string, args ...any) error {
		var id string
		err := tx.QueryRowContext(ctx, query, args...).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		targets = append(targets, id)
		return nil
	}

	switch event.Type {
	case "PROPOSAL_CREATED":
		notificationType = "proposal_received"
		actorID = payloadString(event.Payload, "actor_user_id")
		if err := one(`SELECT customer_user_id::text FROM projects WHERE id=$1::uuid`, event.AggregateID); err != nil {
			return nil, "", "", err
		}
	case "PROPOSAL_ACCEPTED":
		proposalID := payloadString(event.Payload, "proposal_id")
		if proposalID != "" {
			if err := one(`SELECT freelancer_user_id::text FROM proposals WHERE id=$1::uuid`, proposalID); err != nil {
				return nil, "", "", err
			}
		}
	case "PROJECT_COMPLETED", "PROJECT_CANCELLED":
		if err := one(`SELECT freelancer_user_id::text FROM project_assignments WHERE project_id=$1::uuid`, event.AggregateID); err != nil {
			return nil, "", "", err
		}
	case "REVIEW_CREATED":
		notificationType = "new_review_received"
		if id := payloadString(event.Payload, "reviewee_user_id"); id != "" {
			targets = append(targets, id)
		}
	case "INVITE_ACCEPTED":
		notificationType = "invite_accepted"
		actorID = payloadString(event.Payload, "accepted_user_id")
		if id := payloadString(event.Payload, "inviter_user_id"); id != "" {
			targets = append(targets, id)
		}
	case "REWARD_GRANTED":
		notificationType = "reward_granted"
		rows, err := tx.QueryContext(ctx, `SELECT user_id::text FROM reward_ledger WHERE event_key LIKE '%:'||$1::uuid::text`, event.AggregateID)
		if err != nil {
			return nil, "", "", err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return nil, "", "", err
			}
			targets = append(targets, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, "", "", err
		}
		rows.Close()
	case "PROJECT_INVITE_CREATED":
		notificationType = "invited_to_project"
		if id := payloadString(event.Payload, "invited_user_id"); id != "" {
			targets = append(targets, id)
		}
	case "DEAL_FUNDING_REQUIRED", "DEAL_FUNDED", "DEAL_STARTED", "DEAL_SUBMITTED", "DEAL_REVISION_REQUESTED", "DISPUTE_OPENED", "DISPUTE_RESOLVED", "DEAL_ACCEPTED", "DEAL_RELEASE_PENDING", "DEAL_COMPLETED", "DEAL_REFUND_PENDING", "DEAL_REFUNDED", "DEAL_CANCELED":
		notificationType = "safe_deal_update"
		actorID = payloadString(event.Payload, "actor_user_id")
		rows, err := tx.QueryContext(ctx, `SELECT customer_user_id::text FROM safe_deals WHERE id=$1::uuid
UNION
SELECT freelancer_user_id::text FROM safe_deals WHERE id=$1::uuid`, event.AggregateID)
		if err != nil {
			return nil, "", "", err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return nil, "", "", err
			}
			targets = append(targets, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, "", "", err
		}
		rows.Close()
	}

	return targets, notificationType, actorID, nil
}

func payloadString(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}
