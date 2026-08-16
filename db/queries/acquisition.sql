-- name: GetEnabledCalculator :one
SELECT id,slug,title,category_id,version,schema,pricing_config,updated_at
FROM calculator_definitions WHERE slug=$1 AND enabled=true;

-- name: RecordAcquisitionEvent :exec
INSERT INTO acquisition_events(id,anonymous_id,user_id,event_type,landing_path,utm_source,utm_medium,utm_campaign,utm_content,referral_code,metadata)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11);
