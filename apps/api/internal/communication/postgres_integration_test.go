//go:build integration

package communication

import (
	"context"
	"database/sql"
	"errors"
	_ "github.com/jackc/pgx/v5/stdlib"
	"os"
	"testing"
)

func TestPostgresCommunicationPersistenceAuthorizationAndFanoutJobs(t *testing.T) {
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
	a, b, c := "f1101010-1010-4010-8010-101010101010", "f2202020-2020-4020-8020-202020202020", "f3303030-3030-4030-8030-303030303030"
	_, e = db.ExecContext(ctx, `INSERT INTO users(id,email,email_normalized,password_hash,display_name)VALUES($1,'chat-a@example.invalid','chat-a@example.invalid','x','A'),($2,'chat-b@example.invalid','chat-b@example.invalid','x','B'),($3,'chat-c@example.invalid','chat-c@example.invalid','x','C')`, a, b, c)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		for _, q := range []string{
			"DELETE FROM email_jobs WHERE user_id IN($1,$2,$3)",
			"DELETE FROM notifications WHERE user_id IN($1,$2,$3)",
			"DELETE FROM outbox_events WHERE aggregate_type='CONVERSATION'",
			"DELETE FROM conversations WHERE id IN(SELECT conversation_id FROM conversation_members WHERE user_id IN($1,$2,$3))",
			"DELETE FROM users WHERE id IN($1,$2,$3)",
		} {
			_, _ = db.ExecContext(context.Background(), q, a, b, c)
		}
	})
	s := Service{Repository: PostgresRepository{DB: db}}
	conversation, e := s.CreateConversation(ctx, a, CreateConversation{ParticipantUserID: b})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.GetConversation(ctx, c, conversation.ID); !errors.Is(e, ErrNotFound) {
		t.Fatalf("non-member disclosure: %v", e)
	}
	message, e := s.Send(ctx, a, conversation.ID, MessageInput{ClientMessageID: "f4404040-4040-4040-8040-404040404040", Type: "TEXT", Body: "persisted"})
	if e != nil {
		t.Fatal(e)
	}
	duplicate, e := s.Send(ctx, a, conversation.ID, MessageInput{ClientMessageID: message.ClientMessageID, Type: "TEXT", Body: "duplicate"})
	if e != nil || duplicate.ID != message.ID {
		t.Fatalf("idempotency failed: %v", e)
	}
	var notifications, jobs int
	if e = db.QueryRowContext(ctx, `SELECT(SELECT count(*)FROM notifications WHERE user_id=$1),(SELECT count(*)FROM email_jobs WHERE user_id=$1)`, b).Scan(&notifications, &jobs); e != nil || notifications != 1 || jobs != 1 {
		t.Fatalf("fanout rows notifications=%d jobs=%d error=%v", notifications, jobs, e)
	}
	if e = s.Read(ctx, b, conversation.ID, message.ID); e != nil {
		t.Fatal(e)
	}
}
