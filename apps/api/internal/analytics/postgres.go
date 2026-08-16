package analytics

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type PostgresRepository struct{ DB *sql.DB }

func (r PostgresRepository) Record(ctx context.Context, in EventInput) error {
	viewerKey := "anon"
	var viewer any
	if in.ViewerUserID != "" {
		viewer = in.ViewerUserID
		viewerKey = in.ViewerUserID
	}
	entityKey := in.EntityType
	var entity any
	if in.EntityID != "" {
		entity = in.EntityID
		entityKey = in.EntityID
	}
	dedupe := fmt.Sprintf("%s:%s:%s:%s:%s", in.EventType, in.SubjectUserID, entityKey, viewerKey, in.DayKey)
	_, err := r.DB.ExecContext(ctx, `
INSERT INTO profile_engagement_events(subject_user_id,viewer_user_id,event_type,entity_type,entity_id,dedupe_key)
VALUES($1::uuid,$2,$3,$4,$5,$6)
ON CONFLICT(dedupe_key) DO NOTHING`, in.SubjectUserID, viewer, in.EventType, in.EntityType, entity, dedupe)
	return err
}

func (r PostgresRepository) ResolveSubjectUserID(ctx context.Context, entityType, entityID string) (string, error) {
	if entityType != "SERVICE" {
		return "", ErrInvalid
	}
	var id string
	err := r.DB.QueryRowContext(ctx, `
SELECT s.seller_user_id::text
FROM services s
JOIN users u ON u.id=s.seller_user_id AND u.status='ACTIVE' AND u.deleted_at IS NULL
WHERE s.id=$1::uuid AND s.deleted_at IS NULL AND s.status='PUBLISHED'`, entityID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}

func (r PostgresRepository) CountEvents(ctx context.Context, subject, eventType string, since time.Time) (int64, error) {
	var n int64
	err := r.DB.QueryRowContext(ctx, `
SELECT COUNT(*) FROM profile_engagement_events
WHERE subject_user_id=$1::uuid AND event_type=$2 AND created_at>=$3`, subject, eventType, since.UTC()).Scan(&n)
	return n, err
}

func (r PostgresRepository) CountProposals(ctx context.Context, subject string, since time.Time) (int64, error) {
	var n int64
	err := r.DB.QueryRowContext(ctx, `
SELECT COUNT(*) FROM proposals
WHERE freelancer_user_id=$1::uuid AND submitted_at>=$2`, subject, since.UTC()).Scan(&n)
	return n, err
}

func (r PostgresRepository) CountJobApplications(ctx context.Context, subject string, since time.Time) (int64, error) {
	var n int64
	err := r.DB.QueryRowContext(ctx, `
SELECT COUNT(*) FROM job_applications
WHERE user_id=$1::uuid AND created_at>=$2`, subject, since.UTC()).Scan(&n)
	return n, err
}

type MemoryRepository struct {
	Events       []EventInput
	Subjects     map[string]string
	Proposals    map[string]int64
	Applications map[string]int64
}

func (m *MemoryRepository) Record(_ context.Context, in EventInput) error {
	key := fmt.Sprintf("%s:%s:%s:%s:%s", in.EventType, in.SubjectUserID, in.EntityID, in.ViewerUserID, in.DayKey)
	for _, existing := range m.Events {
		existingKey := fmt.Sprintf("%s:%s:%s:%s:%s", existing.EventType, existing.SubjectUserID, existing.EntityID, existing.ViewerUserID, existing.DayKey)
		if existingKey == key {
			return nil
		}
	}
	m.Events = append(m.Events, in)
	return nil
}

func (m *MemoryRepository) ResolveSubjectUserID(_ context.Context, entityType, entityID string) (string, error) {
	if m.Subjects == nil {
		return "", ErrNotFound
	}
	id, ok := m.Subjects[entityType+":"+entityID]
	if !ok {
		return "", ErrNotFound
	}
	return id, nil
}

func (m *MemoryRepository) CountEvents(_ context.Context, subject, eventType string, since time.Time) (int64, error) {
	var n int64
	for _, event := range m.Events {
		if event.SubjectUserID == subject && event.EventType == eventType {
			n++
		}
	}
	_ = since
	return n, nil
}

func (m *MemoryRepository) CountProposals(_ context.Context, subject string, _ time.Time) (int64, error) {
	if m.Proposals == nil {
		return 0, nil
	}
	return m.Proposals[subject], nil
}

func (m *MemoryRepository) CountJobApplications(_ context.Context, subject string, _ time.Time) (int64, error) {
	if m.Applications == nil {
		return 0, nil
	}
	return m.Applications[subject], nil
}
