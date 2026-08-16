package favorites

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotFound = errors.New("favorite target not found")
	ErrInvalid  = errors.New("invalid favorite")
)
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Item struct {
	EntityType    string    `json:"entity_type"`
	EntityID      string    `json:"entity_id"`
	CreatedAt     time.Time `json:"created_at"`
	Title         string    `json:"title,omitempty"`
	Subtitle      string    `json:"subtitle,omitempty"`
	Username      string    `json:"username,omitempty"`
	Slug          string    `json:"slug,omitempty"`
	Category      string    `json:"category,omitempty"`
	AmountKopecks *int64    `json:"amount_kopecks,omitempty"`
	Rating        *float64  `json:"rating,omitempty"`
	ReviewsCount  int       `json:"reviews_count,omitempty"`
}
type Cursor struct {
	At time.Time
	ID string
}
type Page struct {
	Items      []Item
	NextCursor *Cursor
}
type Repository interface {
	Put(context.Context, string, string, string) (Item, error)
	Delete(context.Context, string, string, string) error
	List(context.Context, string, string, *Cursor, int) (Page, error)
}
type Store struct {
	mu      sync.Mutex
	Items   map[string]Item
	Visible map[string]bool
	Now     func() time.Time
}

func (s *Store) Put(_ context.Context, actor, kind, id string) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kind = strings.ToUpper(kind)
	if !validType(kind) || !uuidPattern.MatchString(strings.ToLower(id)) {
		return Item{}, ErrInvalid
	}
	if s.Visible != nil && !s.Visible[kind+":"+id] {
		return Item{}, ErrNotFound
	}
	key := actor + ":" + kind + ":" + id
	if v, ok := s.Items[key]; ok {
		return v, nil
	}
	v := Item{EntityType: kind, EntityID: id, CreatedAt: s.now()}
	if s.Items == nil {
		s.Items = map[string]Item{}
	}
	s.Items[key] = v
	return v, nil
}
func (s *Store) Delete(_ context.Context, actor, kind, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	kind = strings.ToUpper(kind)
	if !validType(kind) || !uuidPattern.MatchString(strings.ToLower(id)) {
		return ErrInvalid
	}
	delete(s.Items, actor+":"+kind+":"+id)
	return nil
}
func (s *Store) List(_ context.Context, actor, kind string, c *Cursor, l int) (Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kind = strings.ToUpper(strings.TrimSpace(kind))
	if kind != "" && !validType(kind) {
		return Page{}, ErrInvalid
	}
	items := []Item{}
	prefix := actor + ":"
	for key, v := range s.Items {
		if strings.HasPrefix(key, prefix) && (kind == "" || v.EntityType == kind) && (s.Visible == nil || s.Visible[v.EntityType+":"+v.EntityID]) && (c == nil || v.CreatedAt.Before(c.At) || v.CreatedAt.Equal(c.At) && v.EntityID < c.ID) {
			items = append(items, v)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].EntityID > items[j].EntityID
	})
	if l < 1 {
		l = 20
	}
	if l > 50 {
		l = 50
	}
	p := Page{Items: items}
	if len(items) > l {
		last := items[l-1]
		p.Items = items[:l]
		p.NextCursor = &Cursor{At: last.CreatedAt, ID: last.EntityID}
	}
	return p, nil
}
func validType(v string) bool { return v == "FREELANCER" || v == "SERVICE" || v == "PROJECT" }
func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
