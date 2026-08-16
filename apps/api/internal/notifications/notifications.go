package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"
)

var ErrNotFound = errors.New("notification not found")

type Notification struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	ActorUserID *string         `json:"actor_user_id,omitempty"`
	EntityType  *string         `json:"entity_type,omitempty"`
	EntityID    *string         `json:"entity_id,omitempty"`
	Payload     json.RawMessage `json:"payload"`
	ReadAt      *time.Time      `json:"read_at,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}
type Preference struct {
	EventType string `json:"event_type"`
	InApp     bool   `json:"in_app"`
	Email     bool   `json:"email"`
}
type Cursor struct {
	At time.Time
	ID string
}
type Page struct {
	Items []Notification
	Next  *Cursor
}
type Repository interface {
	List(context.Context, string, *Cursor, int) (Page, error)
	Read(context.Context, string, string) error
	ReadAll(context.Context, string) error
	Preferences(context.Context, string) ([]Preference, error)
	ReplacePreferences(context.Context, string, []Preference) ([]Preference, error)
}
type Service struct{ Repository Repository }

func (s Service) List(ctx context.Context, u string, c *Cursor, l int) (Page, error) {
	if l < 1 {
		l = 50
	}
	if l > 100 {
		l = 100
	}
	return s.Repository.List(ctx, u, c, l)
}
func (s Service) Read(ctx context.Context, u, id string) error {
	if !validID(id) {
		return ErrNotFound
	}
	return s.Repository.Read(ctx, u, id)
}
func (s Service) ReadAll(ctx context.Context, u string) error { return s.Repository.ReadAll(ctx, u) }
func (s Service) Preferences(ctx context.Context, u string) ([]Preference, error) {
	return s.Repository.Preferences(ctx, u)
}
func (s Service) ReplacePreferences(ctx context.Context, u string, p []Preference) ([]Preference, error) {
	if len(p) > 50 {
		return nil, errors.New("invalid preferences")
	}
	seen := map[string]bool{}
	for _, v := range p {
		if v.EventType == "" || len(v.EventType) > 100 || seen[v.EventType] {
			return nil, errors.New("invalid preferences")
		}
		seen[v.EventType] = true
	}
	return s.Repository.ReplacePreferences(ctx, u, p)
}

type Store struct {
	Mu    sync.Mutex
	Items map[string]map[string]Notification
	Prefs map[string][]Preference
}

func (s *Store) List(_ context.Context, u string, c *Cursor, l int) (Page, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	v := []Notification{}
	for _, n := range s.Items[u] {
		if c == nil || n.CreatedAt.Before(c.At) || n.CreatedAt.Equal(c.At) && n.ID < c.ID {
			v = append(v, n)
		}
	}
	sort.Slice(v, func(i, j int) bool { return v[i].CreatedAt.After(v[j].CreatedAt) })
	p := Page{Items: v}
	if len(v) > l {
		x := v[l-1]
		p.Items = v[:l]
		p.Next = &Cursor{At: x.CreatedAt, ID: x.ID}
	}
	return p, nil
}
func (s *Store) Read(_ context.Context, u, id string) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	n, ok := s.Items[u][id]
	if !ok {
		return ErrNotFound
	}
	if n.ReadAt == nil {
		v := time.Now().UTC()
		n.ReadAt = &v
		s.Items[u][id] = n
	}
	return nil
}
func (s *Store) ReadAll(_ context.Context, u string) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	v := time.Now().UTC()
	for id, n := range s.Items[u] {
		if n.ReadAt == nil {
			n.ReadAt = &v
			s.Items[u][id] = n
		}
	}
	return nil
}
func (s *Store) Preferences(_ context.Context, u string) ([]Preference, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	return append([]Preference{}, s.Prefs[u]...), nil
}
func (s *Store) ReplacePreferences(_ context.Context, u string, p []Preference) ([]Preference, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if s.Prefs == nil {
		s.Prefs = map[string][]Preference{}
	}
	s.Prefs[u] = append([]Preference{}, p...)
	return p, nil
}

type PostgresRepository struct{ DB *sql.DB }

func (r PostgresRepository) List(ctx context.Context, u string, c *Cursor, l int) (Page, error) {
	var at, id any
	if c != nil {
		at, id = c.At, c.ID
	}
	rows, e := r.DB.QueryContext(ctx, `SELECT id::text,type,actor_user_id::text,entity_type,entity_id::text,payload,read_at,created_at FROM notifications WHERE user_id=$1 AND($2::timestamptz IS NULL OR(created_at,id)<($2,$3::uuid))ORDER BY created_at DESC,id DESC LIMIT $4`, u, at, id, l+1)
	if e != nil {
		return Page{}, e
	}
	defer rows.Close()
	v := []Notification{}
	for rows.Next() {
		var n Notification
		var a, t, id sql.NullString
		var raw []byte
		if e = rows.Scan(&n.ID, &n.Type, &a, &t, &id, &raw, &n.ReadAt, &n.CreatedAt); e != nil {
			return Page{}, e
		}
		if a.Valid {
			n.ActorUserID = &a.String
		}
		if t.Valid {
			n.EntityType = &t.String
		}
		if id.Valid {
			n.EntityID = &id.String
		}
		n.Payload = raw
		v = append(v, n)
	}
	if e = rows.Err(); e != nil {
		return Page{}, e
	}
	p := Page{Items: v}
	if len(v) > l {
		x := v[l-1]
		p.Items = v[:l]
		p.Next = &Cursor{At: x.CreatedAt, ID: x.ID}
	}
	return p, nil
}
func (r PostgresRepository) Read(ctx context.Context, u, id string) error {
	q, e := r.DB.ExecContext(ctx, `UPDATE notifications SET read_at=COALESCE(read_at,now()) WHERE id=$1 AND user_id=$2`, id, u)
	if e != nil {
		return e
	}
	if n, _ := q.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
func (r PostgresRepository) ReadAll(ctx context.Context, u string) error {
	_, e := r.DB.ExecContext(ctx, `UPDATE notifications SET read_at=now() WHERE user_id=$1 AND read_at IS NULL`, u)
	return e
}
func (r PostgresRepository) Preferences(ctx context.Context, u string) ([]Preference, error) {
	rows, e := r.DB.QueryContext(ctx, `SELECT event_type,in_app,email FROM notification_preferences WHERE user_id=$1 ORDER BY event_type`, u)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	v := []Preference{}
	for rows.Next() {
		var p Preference
		if e = rows.Scan(&p.EventType, &p.InApp, &p.Email); e != nil {
			return nil, e
		}
		v = append(v, p)
	}
	return v, rows.Err()
}
func (r PostgresRepository) ReplacePreferences(ctx context.Context, u string, p []Preference) ([]Preference, error) {
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `DELETE FROM notification_preferences WHERE user_id=$1`, u); e != nil {
		return nil, e
	}
	for _, v := range p {
		if _, e = tx.ExecContext(ctx, `INSERT INTO notification_preferences(user_id,event_type,in_app,email)VALUES($1,$2,$3,$4)`, u, v.EventType, v.InApp, v.Email); e != nil {
			return nil, e
		}
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return p, nil
}
func validID(v string) bool { return len(v) == 36 }
