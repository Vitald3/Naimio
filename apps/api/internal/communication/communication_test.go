package communication

import (
	"context"
	"freelance/apps/api/internal/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const userA = "00000000-0000-4000-8000-000000000001"
const userB = "00000000-0000-4000-8000-000000000002"
const userC = "00000000-0000-4000-8000-000000000003"

func communicationService() (Service, *Store) {
	s := &Store{Conversations: map[string]Conversation{}, Memberships: map[string]map[string]Membership{}, Messages: map[string]Message{}, Projects: map[string][]string{}, Users: map[string]bool{userA: true, userB: true, userC: true}, Media: map[string]Media{}}
	return Service{Repository: s}, s
}

func TestConversationAuthorizationIdempotencyAndReadState(t *testing.T) {
	svc, store := communicationService()
	ctx := context.Background()
	c, err := svc.CreateConversation(ctx, userA, CreateConversation{ParticipantUserID: userB})
	if err != nil {
		t.Fatal(err)
	}
	again, err := svc.CreateConversation(ctx, userA, CreateConversation{ParticipantUserID: userB})
	if err != nil || again.ID != c.ID {
		t.Fatalf("direct conversation was not idempotent: %v", err)
	}
	if _, err = svc.GetConversation(ctx, userC, c.ID); err != ErrNotFound {
		t.Fatalf("non-member disclosure: %v", err)
	}
	first, err := svc.Send(ctx, userA, c.ID, MessageInput{ClientMessageID: "10000000-0000-4000-8000-000000000001", Type: "text", Body: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := svc.Send(ctx, userA, c.ID, MessageInput{ClientMessageID: first.ClientMessageID, Type: "TEXT", Body: "changed"})
	if err != nil || duplicate.ID != first.ID || duplicate.Body != "hello" {
		t.Fatalf("message idempotency failed: %#v %v", duplicate, err)
	}
	second, err := svc.Send(ctx, userA, c.ID, MessageInput{ClientMessageID: "10000000-0000-4000-8000-000000000002", Type: "TEXT", Body: "later"})
	if err != nil {
		t.Fatal(err)
	}
	quoted, err := svc.Send(ctx, userB, c.ID, MessageInput{ClientMessageID: "10000000-0000-4000-8000-000000000005", Type: "TEXT", Body: "reply", ReplyToMessageID: second.ID, ReplyQuote: "later"})
	if err != nil || quoted.ReplyToMessageID != second.ID || quoted.ReplyQuote != "later" {
		t.Fatalf("quoted reply was not preserved: %#v %v", quoted, err)
	}
	if _, err = svc.Send(ctx, userB, c.ID, MessageInput{ClientMessageID: "10000000-0000-4000-8000-000000000006", Type: "TEXT", Body: "invalid", ReplyQuote: "orphan quote"}); err != ErrInvalid {
		t.Fatalf("orphan quote accepted: %v", err)
	}
	if err = svc.Read(ctx, userB, c.ID, second.ID); err != nil {
		t.Fatal(err)
	}
	if err = svc.Read(ctx, userB, c.ID, first.ID); err != nil {
		t.Fatal(err)
	}
	if got := store.Memberships[c.ID][userB].LastRead; got != second.ID {
		t.Fatalf("read marker moved backwards: %s", got)
	}
	if _, err = svc.Edit(ctx, userB, first.ID, "intrusion"); err != ErrNotFound {
		t.Fatalf("non-sender edit allowed: %v", err)
	}
	deleted, err := svc.Delete(ctx, userA, first.ID)
	if err != nil || deleted.DeletedAt == nil || deleted.Body != "" {
		t.Fatalf("soft delete failed: %#v %v", deleted, err)
	}
}

func TestChatAttachmentMustBeOwnedCleanAndChatPurpose(t *testing.T) {
	svc, s := communicationService()
	c, _ := svc.CreateConversation(context.Background(), userA, CreateConversation{ParticipantUserID: userB})
	id := "20000000-0000-4000-8000-000000000001"
	s.Media[id] = Media{Owner: userA, Purpose: "CHAT", Scan: "PENDING"}
	_, err := svc.Send(context.Background(), userA, c.ID, MessageInput{ClientMessageID: "10000000-0000-4000-8000-000000000003", Type: "FILE", MediaIDs: []string{id}})
	if err != ErrNotFound {
		t.Fatalf("unscanned media accepted: %v", err)
	}
	s.Media[id] = Media{Owner: userA, Purpose: "CHAT", Scan: "CLEAN"}
	if _, err = svc.Send(context.Background(), userA, c.ID, MessageInput{ClientMessageID: "10000000-0000-4000-8000-000000000004", Type: "FILE", MediaIDs: []string{id}}); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPRejectsUnauthenticatedAndUnknownFields(t *testing.T) {
	svc, _ := communicationService()
	h := Handler{Service: svc}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/conversations", nil)
	w := httptest.NewRecorder()
	h.Conversations(w, req)
	if w.Code != 401 {
		t.Fatalf("status=%d", w.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/conversations", strings.NewReader(`{"participant_user_id":"00000000-0000-4000-8000-000000000002","user_id":"forged"}`))
	req = req.WithContext(auth.WithActorID(req.Context(), userA))
	w = httptest.NewRecorder()
	h.Conversations(w, req)
	if w.Code != 400 {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestHubPublishesOnlyToNamedUsers(t *testing.T) {
	h := NewHub()
	a := &socketClient{userID: userA, send: make(chan Event, 1)}
	b := &socketClient{userID: userB, send: make(chan Event, 1)}
	h.add(a)
	h.add(b)
	defer h.remove(a)
	defer h.remove(b)
	_ = h.Publish(context.Background(), []string{userA}, Event{Event: "message.created", OccurredAt: time.Now()})
	select {
	case <-a.send:
	default:
		t.Fatal("member did not receive event")
	}
	select {
	case <-b.send:
		t.Fatal("non-member received event")
	default:
	}
}

func TestRealtimeRequiresAuthentication(t *testing.T) {
	h := RealtimeHandler{Hub: NewHub()}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestWebsocketOriginAllowedBehindReverseProxy(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://api:8080/api/v1/ws", nil)
	req.Host = "api:8080"
	req.Header.Set("Origin", "http://192.168.31.98:8088")
	req.Header.Set("X-Forwarded-Host", "192.168.31.98:8088")
	if !websocketOriginAllowed(req) {
		t.Fatal("same public origin forwarded by nginx must be accepted")
	}
	req.Header.Set("Origin", "http://evil.example")
	if websocketOriginAllowed(req) {
		t.Fatal("cross-origin websocket must be rejected")
	}
}
