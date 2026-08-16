package notification

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

type DigestRepository interface {
	ProcessDigest(context.Context) (bool, error)
}
type DigestProcessor struct{ Repository DigestRepository }

func (p DigestProcessor) Once(ctx context.Context) error {
	_, err := p.Repository.ProcessDigest(ctx)
	return err
}

type digestConfig struct {
	enabled                    bool
	threshold, intervalMinutes int
}

func (r PostgresRepository) ProcessDigest(ctx context.Context) (bool, error) {
	config := digestConfig{enabled: true, threshold: 10, intervalMinutes: 60}
	err := r.DB.QueryRowContext(ctx, `SELECT COALESCE((config->>'marketplace_digest_enabled')::boolean,true),COALESCE((config->>'marketplace_digest_threshold')::int,10),COALESCE((config->>'marketplace_digest_interval_minutes')::int,60) FROM feature_flags WHERE key='site_appearance'`).Scan(&config.enabled, &config.threshold, &config.intervalMinutes)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if !config.enabled || config.threshold < 1 || config.intervalMinutes < 1 {
		return false, nil
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var eventType string
	err = tx.QueryRowContext(ctx, `SELECT event_type FROM outbox_events WHERE published_at IS NULL AND available_at<=now() AND event_type IN('PROJECT_PUBLISHED','VACANCY_PUBLISHED','SERVICE_PUBLISHED') GROUP BY event_type HAVING count(*) >= $1 OR min(created_at) <= now()-make_interval(mins=>$2) ORDER BY min(created_at) LIMIT 1`, config.threshold, config.intervalMinutes).Scan(&eventType)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id::text,aggregate_id::text FROM outbox_events WHERE published_at IS NULL AND available_at<=now() AND event_type=$1 ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 100`, eventType)
	if err != nil {
		return false, err
	}
	var eventIDs, entityIDs []string
	for rows.Next() {
		var eventID, entityID string
		if err = rows.Scan(&eventID, &entityID); err != nil {
			rows.Close()
			return false, err
		}
		eventIDs = append(eventIDs, eventID)
		entityIDs = append(entityIDs, entityID)
	}
	if err = rows.Close(); err != nil {
		return false, err
	}
	if len(eventIDs) == 0 {
		return false, nil
	}
	notificationType, catalogPath := digestPresentation(eventType)
	if notificationType == "" {
		return false, fmt.Errorf("unsupported digest event: %s", eventType)
	}
	idsJSON, _ := json.Marshal(entityIDs)
	eventIDsJSON, _ := json.Marshal(eventIDs)
	dedupe := "digest:" + eventIDs[0]
	_, err = tx.ExecContext(ctx, `INSERT INTO notifications(id,dedupe_key,user_id,type,entity_type,entity_id,payload)
SELECT gen_random_uuid(),$1||':'||np.user_id::text,np.user_id,$2::varchar,'digest',$3::uuid,jsonb_build_object('entity_id',$3::text,'entity_ids',$4::jsonb,'count',$5)
FROM notification_preferences np WHERE np.event_type=$2::varchar AND np.in_app=true ON CONFLICT(dedupe_key)DO NOTHING`, dedupe, notificationType, entityIDs[0], string(idsJSON), len(entityIDs))
	if err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO email_jobs(dedupe_key,user_id,template,payload)
SELECT $1||':email:'||np.user_id::text,np.user_id,$2::varchar,jsonb_build_object('count',$3,'catalog_path',$4,'preferences_path','/settings/notifications')
FROM notification_preferences np WHERE np.event_type=$2::varchar AND np.email=true ON CONFLICT(dedupe_key)DO NOTHING`, dedupe, notificationType, len(entityIDs), catalogPath)
	if err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE outbox_events SET published_at=now(),attempts=attempts+1 WHERE id IN(SELECT jsonb_array_elements_text($1::jsonb)::uuid)`, string(eventIDsJSON))
	if err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func digestPresentation(eventType string) (string, string) {
	switch eventType {
	case "PROJECT_PUBLISHED":
		return "new_project_available", "/projects"
	case "VACANCY_PUBLISHED":
		return "new_vacancy_available", "/vacancies"
	case "SERVICE_PUBLISHED":
		return "new_service_available", "/services"
	default:
		return "", ""
	}
}
