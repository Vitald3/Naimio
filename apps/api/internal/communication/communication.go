package communication

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrUnauthorized = errors.New("authentication required")
	ErrNotFound     = errors.New("conversation not found")
	ErrInvalid      = errors.New("invalid communication input")
	ErrConflict     = errors.New("communication conflict")
)

type Conversation struct {
	ID                   string    `json:"id"`
	Kind                 string    `json:"kind"`
	ProjectID            *string   `json:"project_id,omitempty"`
	ProjectTitle         string    `json:"project_title,omitempty"`
	CounterpartyName     string    `json:"counterparty_name,omitempty"`
	CounterpartyID       string    `json:"counterparty_user_id,omitempty"`
	CounterpartyUsername string    `json:"counterparty_username,omitempty"`
	CounterpartyRole     string    `json:"counterparty_role,omitempty"`
	MemberIDs            []string  `json:"-"`
	UnreadCount          int       `json:"unread_count"`
	UpdatedAt            time.Time `json:"updated_at"`
	CreatedAt            time.Time `json:"created_at"`
}
type CreateConversation struct {
	ParticipantUserID string `json:"participant_user_id"`
	ProjectID         string `json:"project_id"`
}
type MessageInput struct {
	ClientMessageID  string   `json:"client_message_id"`
	Type             string   `json:"type"`
	Body             string   `json:"body"`
	ReplyToMessageID string   `json:"reply_to_message_id"`
	ReplyQuote       string   `json:"reply_quote"`
	MediaIDs         []string `json:"media_ids"`
}
type Message struct {
	ID               string     `json:"id"`
	ConversationID   string     `json:"conversation_id"`
	SenderID         string     `json:"sender_user_id"`
	Type             string     `json:"type"`
	Body             string     `json:"body,omitempty"`
	ReplyToMessageID string     `json:"reply_to_message_id,omitempty"`
	ReplyQuote       string     `json:"reply_quote,omitempty"`
	ClientMessageID  string     `json:"client_message_id,omitempty"`
	MediaIDs         []string   `json:"media_ids"`
	EditedAt         *time.Time `json:"edited_at,omitempty"`
	DeletedAt        *time.Time `json:"deleted_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}
type Cursor struct {
	At time.Time
	ID string
}
type MessagePage struct {
	Items      []Message
	NextCursor *Cursor
}
type Event struct {
	Event      string    `json:"event"`
	Version    int       `json:"version"`
	ID         string    `json:"id"`
	OccurredAt time.Time `json:"occurred_at"`
	Data       any       `json:"data"`
}
type Repository interface {
	CreateConversation(context.Context, string, CreateConversation) (Conversation, error)
	ListConversations(context.Context, string) ([]Conversation, error)
	GetConversation(context.Context, string, string) (Conversation, error)
	ListMessages(context.Context, string, string, *Cursor, int) (MessagePage, error)
	Send(context.Context, string, string, MessageInput) (Message, error)
	Edit(context.Context, string, string, string) (Message, error)
	Delete(context.Context, string, string) (Message, error)
	Read(context.Context, string, string, string) error
	Members(context.Context, string) ([]string, error)
}
type Publisher interface {
	Publish(context.Context, []string, Event) error
}

type RecipientEvent struct {
	UserID string
	Data   any
}

type NotificationEventRepository interface {
	MessageNotificationEvents(context.Context, string) ([]RecipientEvent, error)
}
type Service struct {
	Repository Repository
	Publisher  Publisher
}

func (s Service) CreateConversation(ctx context.Context, a string, in CreateConversation) (Conversation, error) {
	if a == "" {
		return Conversation{}, ErrUnauthorized
	}
	if (in.ProjectID == "") == (in.ParticipantUserID == "") || in.ProjectID != "" && !uuid(in.ProjectID) || in.ParticipantUserID != "" && (!uuid(in.ParticipantUserID) || in.ParticipantUserID == a) {
		return Conversation{}, ErrInvalid
	}
	return s.Repository.CreateConversation(ctx, a, in)
}
func (s Service) ListConversations(ctx context.Context, a string) ([]Conversation, error) {
	if a == "" {
		return nil, ErrUnauthorized
	}
	return s.Repository.ListConversations(ctx, a)
}
func (s Service) GetConversation(ctx context.Context, a, id string) (Conversation, error) {
	if a == "" {
		return Conversation{}, ErrUnauthorized
	}
	if !uuid(id) {
		return Conversation{}, ErrNotFound
	}
	return s.Repository.GetConversation(ctx, a, id)
}
func (s Service) Messages(ctx context.Context, a, id string, c *Cursor, l int) (MessagePage, error) {
	if a == "" {
		return MessagePage{}, ErrUnauthorized
	}
	if !uuid(id) {
		return MessagePage{}, ErrNotFound
	}
	if l < 1 {
		l = 50
	}
	if l > 100 {
		l = 100
	}
	return s.Repository.ListMessages(ctx, a, id, c, l)
}
func (s Service) Send(ctx context.Context, a, id string, in MessageInput) (Message, error) {
	if a == "" {
		return Message{}, ErrUnauthorized
	}
	in.Type = strings.ToUpper(strings.TrimSpace(in.Type))
	in.Body = strings.TrimSpace(in.Body)
	in.ReplyQuote = strings.TrimSpace(in.ReplyQuote)
	if !uuid(id) || !uuid(in.ClientMessageID) || !validMessage(in) {
		return Message{}, ErrInvalid
	}
	v, err := s.Repository.Send(ctx, a, id, in)
	if err == nil && s.Publisher != nil {
		members, _ := s.Repository.Members(ctx, id)
		_ = s.Publisher.Publish(ctx, members, Event{Event: "message.created", Version: 1, ID: newID(), OccurredAt: time.Now().UTC(), Data: v})
		for _, memberID := range members {
			if memberID == SupportUserID {
				if conversation, conversationErr := s.Repository.GetConversation(ctx, SupportUserID, id); conversationErr == nil {
					_ = s.Publisher.Publish(ctx, []string{SupportUserID}, Event{Event: "support.conversation.updated", Version: 1, ID: newID(), OccurredAt: time.Now().UTC(), Data: conversation})
				}
				break
			}
		}
		if source, ok := s.Repository.(NotificationEventRepository); ok {
			events, eventErr := source.MessageNotificationEvents(ctx, v.ID)
			if eventErr == nil {
				for _, notification := range events {
					_ = s.Publisher.Publish(ctx, []string{notification.UserID}, Event{Event: "notification.created", Version: 1, ID: newID(), OccurredAt: time.Now().UTC(), Data: notification.Data})
				}
			}
		}
	}
	return v, err
}
func (s Service) Edit(ctx context.Context, a, id, body string) (Message, error) {
	body = strings.TrimSpace(body)
	if a == "" {
		return Message{}, ErrUnauthorized
	}
	if !uuid(id) || body == "" || len([]rune(body)) > 10000 {
		return Message{}, ErrInvalid
	}
	v, err := s.Repository.Edit(ctx, a, id, body)
	if err == nil {
		s.publish(ctx, v.ConversationID, "message.updated", v)
	}
	return v, err
}
func (s Service) Delete(ctx context.Context, a, id string) (Message, error) {
	if a == "" {
		return Message{}, ErrUnauthorized
	}
	if !uuid(id) {
		return Message{}, ErrNotFound
	}
	v, err := s.Repository.Delete(ctx, a, id)
	if err == nil {
		s.publish(ctx, v.ConversationID, "message.deleted", v)
	}
	return v, err
}
func (s Service) Read(ctx context.Context, a, id, message string) error {
	if a == "" {
		return ErrUnauthorized
	}
	if !uuid(id) || !uuid(message) {
		return ErrInvalid
	}
	err := s.Repository.Read(ctx, a, id, message)
	if err == nil {
		s.publish(ctx, id, "conversation.read", map[string]string{"conversation_id": id, "user_id": a, "last_read_message_id": message})
	}
	return err
}
func (s Service) publish(ctx context.Context, conversation, event string, data any) {
	if s.Publisher == nil {
		return
	}
	members, err := s.Repository.Members(ctx, conversation)
	if err != nil {
		return
	}
	_ = s.Publisher.Publish(ctx, members, Event{Event: event, Version: 1, ID: newID(), OccurredAt: time.Now().UTC(), Data: data})
}
func validMessage(in MessageInput) bool {
	if len([]rune(in.Body)) > 10000 || len([]rune(in.ReplyQuote)) > 1000 || len(in.MediaIDs) > 5 {
		return false
	}
	if in.Type != "TEXT" && in.Type != "IMAGE" && in.Type != "FILE" && in.Type != "AUDIO" {
		return false
	}
	if in.Type == "TEXT" && in.Body == "" || in.Type != "TEXT" && len(in.MediaIDs) == 0 {
		return false
	}
	if in.ReplyToMessageID != "" && !uuid(in.ReplyToMessageID) {
		return false
	}
	if in.ReplyQuote != "" && in.ReplyToMessageID == "" {
		return false
	}
	seen := map[string]bool{}
	for _, id := range in.MediaIDs {
		if !uuid(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}
func uuid(v string) bool {
	if len(v) != 36 {
		return false
	}
	for i, c := range strings.ToLower(v) {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func newID() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	b[6] = b[6]&15 | 64
	b[8] = b[8]&63 | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

type Membership struct {
	Joined   time.Time
	LastRead string
}
type Media struct{ Owner, Purpose, Scan string }
type Store struct {
	mu            sync.Mutex
	Conversations map[string]Conversation
	Memberships   map[string]map[string]Membership
	Messages      map[string]Message
	Projects      map[string][]string
	Users         map[string]bool
	Media         map[string]Media
	Now           func() time.Time
}

func (s *Store) CreateConversation(_ context.Context, a string, in CreateConversation) (Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	members := []string{}
	kind := "DIRECT"
	var project *string
	if in.ProjectID != "" {
		kind = "PROJECT"
		p := in.ProjectID
		project = &p
		members = append(members, s.Projects[p]...)
		if !contains(members, a) {
			return Conversation{}, ErrNotFound
		}
		for _, c := range s.Conversations {
			if c.ProjectID != nil && *c.ProjectID == p {
				return c, nil
			}
		}
	} else {
		if !s.Users[in.ParticipantUserID] {
			return Conversation{}, ErrNotFound
		}
		members = []string{a, in.ParticipantUserID}
		for _, c := range s.Conversations {
			if c.Kind == "DIRECT" && same(c.MemberIDs, members) {
				return c, nil
			}
		}
	}
	if len(members) < 2 {
		return Conversation{}, ErrInvalid
	}
	now := s.now()
	c := Conversation{ID: newID(), Kind: kind, ProjectID: project, MemberIDs: members, CreatedAt: now, UpdatedAt: now}
	if s.Conversations == nil {
		s.Conversations = map[string]Conversation{}
	}
	if s.Memberships == nil {
		s.Memberships = map[string]map[string]Membership{}
	}
	s.Conversations[c.ID] = c
	s.Memberships[c.ID] = map[string]Membership{}
	for _, m := range members {
		s.Memberships[c.ID][m] = Membership{Joined: now}
	}
	return c, nil
}
func (s *Store) ListConversations(_ context.Context, a string) ([]Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Conversation{}
	for id, c := range s.Conversations {
		m, ok := s.Memberships[id][a]
		if ok {
			c.UnreadCount = s.unread(id, a, m.LastRead)
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
func (s *Store) GetConversation(_ context.Context, a, id string) (Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.Conversations[id]
	if !ok || s.Memberships[id] == nil {
		return Conversation{}, ErrNotFound
	}
	m, ok := s.Memberships[id][a]
	if !ok {
		return Conversation{}, ErrNotFound
	}
	c.UnreadCount = s.unread(id, a, m.LastRead)
	return c, nil
}
func (s *Store) ListMessages(_ context.Context, a, id string, c *Cursor, l int) (MessagePage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Memberships[id][a]; !ok {
		return MessagePage{}, ErrNotFound
	}
	items := []Message{}
	for _, m := range s.Messages {
		if m.ConversationID == id && (c == nil || m.CreatedAt.Before(c.At) || m.CreatedAt.Equal(c.At) && m.ID < c.ID) {
			if m.DeletedAt != nil {
				m.Body = ""
				m.MediaIDs = nil
			}
			items = append(items, m)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].ID > items[j].ID
	})
	p := MessagePage{Items: items}
	if len(items) > l {
		x := items[l-1]
		p.Items = items[:l]
		p.NextCursor = &Cursor{x.CreatedAt, x.ID}
	}
	return p, nil
}
func (s *Store) Send(_ context.Context, a, id string, in MessageInput) (Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Memberships[id][a]; !ok {
		return Message{}, ErrNotFound
	}
	for _, m := range s.Messages {
		if m.SenderID == a && m.ClientMessageID == in.ClientMessageID {
			return m, nil
		}
	}
	for _, mid := range in.MediaIDs {
		x, ok := s.Media[mid]
		if !ok || x.Owner != a || x.Purpose != "CHAT" || x.Scan != "CLEAN" {
			return Message{}, ErrNotFound
		}
	}
	if in.ReplyToMessageID != "" {
		m, ok := s.Messages[in.ReplyToMessageID]
		if !ok || m.ConversationID != id {
			return Message{}, ErrInvalid
		}
	}
	now := s.now()
	m := Message{ID: newID(), ConversationID: id, SenderID: a, Type: in.Type, Body: in.Body, ReplyToMessageID: in.ReplyToMessageID, ReplyQuote: in.ReplyQuote, ClientMessageID: in.ClientMessageID, MediaIDs: append([]string{}, in.MediaIDs...), CreatedAt: now}
	if s.Messages == nil {
		s.Messages = map[string]Message{}
	}
	s.Messages[m.ID] = m
	c := s.Conversations[id]
	c.UpdatedAt = now
	s.Conversations[id] = c
	return m, nil
}
func (s *Store) Edit(_ context.Context, a, id, body string) (Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.Messages[id]
	if !ok || m.SenderID != a || m.DeletedAt != nil || m.Type != "TEXT" {
		return Message{}, ErrNotFound
	}
	now := s.now()
	m.Body = body
	m.EditedAt = &now
	s.Messages[id] = m
	return m, nil
}
func (s *Store) Delete(_ context.Context, a, id string) (Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.Messages[id]
	if !ok || m.SenderID != a {
		return Message{}, ErrNotFound
	}
	if m.DeletedAt == nil {
		now := s.now()
		m.DeletedAt = &now
		m.Body = ""
		m.MediaIDs = nil
		s.Messages[id] = m
	}
	return m, nil
}
func (s *Store) Read(_ context.Context, a, id, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	member, ok := s.Memberships[id][a]
	target, exists := s.Messages[message]
	if !ok || !exists || target.ConversationID != id {
		return ErrNotFound
	}
	if member.LastRead != "" {
		old := s.Messages[member.LastRead]
		if old.CreatedAt.After(target.CreatedAt) || old.CreatedAt.Equal(target.CreatedAt) && old.ID >= target.ID {
			return nil
		}
	}
	member.LastRead = message
	s.Memberships[id][a] = member
	return nil
}
func (s *Store) Members(_ context.Context, id string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Memberships[id] == nil {
		return nil, ErrNotFound
	}
	v := []string{}
	for id := range s.Memberships[id] {
		v = append(v, id)
	}
	return v, nil
}
func (s *Store) unread(id, a, last string) int {
	n := 0
	var marker Message
	if last != "" {
		marker = s.Messages[last]
	}
	for _, m := range s.Messages {
		if m.ConversationID == id && m.SenderID != a && (last == "" || m.CreatedAt.After(marker.CreatedAt) || m.CreatedAt.Equal(marker.CreatedAt) && m.ID > marker.ID) {
			n++
		}
	}
	return n
}
func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func contains(v []string, x string) bool {
	for _, a := range v {
		if a == x {
			return true
		}
	}
	return false
}
func same(a, b []string) bool { return len(a) == len(b) && contains(a, b[0]) && contains(a, b[1]) }
