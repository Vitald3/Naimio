//go:build integration

package notification

import (
	"context"
	"database/sql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"os"
	"testing"
)

func TestPostgresDispatcherHonorsIndependentChannels(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Fatal("DATABASE_URL is required")
	}
	db, e := sql.Open("pgx", url)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	ctx := context.Background()
	var user, eventOne, eventTwo, entity string
	if err := db.QueryRowContext(ctx, `SELECT gen_random_uuid()::text,gen_random_uuid()::text,gen_random_uuid()::text,gen_random_uuid()::text`).Scan(&user, &eventOne, &eventTwo, &entity); err != nil {
		t.Fatal(err)
	}
	email := user + "@example.invalid"
	cleanup := func() {
		ctx := context.Background()
		_, _ = db.ExecContext(ctx, `DELETE FROM email_jobs WHERE user_id=$1`, user)
		_, _ = db.ExecContext(ctx, `DELETE FROM notifications WHERE user_id=$1`, user)
		_, _ = db.ExecContext(ctx, `DELETE FROM outbox_events WHERE id IN($1,$2)`, eventOne, eventTwo)
		_, _ = db.ExecContext(ctx, `DELETE FROM notification_preferences WHERE user_id=$1`, user)
		_, _ = db.ExecContext(ctx, `DELETE FROM users WHERE id=$1 OR email_normalized=$2`, user, email)
	}
	cleanup()
	t.Cleanup(cleanup)
	if _, e = db.ExecContext(ctx, `INSERT INTO users(id,email,email_normalized,password_hash,display_name)VALUES($1,$2,$2,'x','Notify')`, user, email); e != nil {
		t.Fatal(e)
	}
	if _, e = db.ExecContext(ctx, `INSERT INTO notification_preferences(user_id,event_type,in_app,email)VALUES($1,'new_review_received',false,true)`, user); e != nil {
		t.Fatal(e)
	}
	if _, e = db.ExecContext(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload)VALUES($2,'REVIEW',$3,'REVIEW_CREATED',jsonb_build_object('reviewee_user_id',$1::text))`, user, eventOne, entity); e != nil {
		t.Fatal(e)
	}
	repo := PostgresRepository{DB: db}
	processEvent := func(eventID string) {
		t.Helper()
		for i := 0; i < 100; i++ {
			var published bool
			if err := db.QueryRowContext(ctx, `SELECT published_at IS NOT NULL FROM outbox_events WHERE id=$1`, eventID).Scan(&published); err != nil {
				t.Fatal(err)
			}
			if published {
				return
			}
			ok, err := repo.ProcessOne(ctx)
			if err != nil {
				t.Fatalf("dispatch event %s: %v", eventID, err)
			}
			if !ok {
				t.Fatalf("dispatch event %s: no processable outbox event", eventID)
			}
		}
		t.Fatalf("dispatch event %s: not published after draining pending events", eventID)
	}
	processEvent(eventOne)
	var notifications, emails int
	if e = db.QueryRowContext(ctx, `SELECT(SELECT count(*)FROM notifications WHERE user_id=$1),(SELECT count(*)FROM email_jobs WHERE user_id=$1)`, user).Scan(&notifications, &emails); e != nil || notifications != 0 || emails != 1 {
		t.Fatalf("independent email channel notifications=%d emails=%d err=%v", notifications, emails, e)
	}
	if _, e = db.ExecContext(ctx, `UPDATE notification_preferences SET in_app=true,email=false WHERE user_id=$1 AND event_type='new_review_received'`, user); e != nil {
		t.Fatal(e)
	}
	if _, e = db.ExecContext(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload)VALUES($2,'REVIEW',$3,'REVIEW_CREATED',jsonb_build_object('reviewee_user_id',$1::text))`, user, eventTwo, entity); e != nil {
		t.Fatal(e)
	}
	processEvent(eventTwo)
	if e = db.QueryRowContext(ctx, `SELECT(SELECT count(*)FROM notifications WHERE user_id=$1),(SELECT count(*)FROM email_jobs WHERE user_id=$1)`, user).Scan(&notifications, &emails); e != nil || notifications != 1 || emails != 1 {
		t.Fatalf("independent in-app channel notifications=%d emails=%d err=%v", notifications, emails, e)
	}
}
