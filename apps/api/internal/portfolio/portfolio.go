package portfolio

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrUnauthorized     = errors.New("unauthenticated")
	ErrNotFound         = errors.New("portfolio item not found")
	ErrInvalidInput     = errors.New("invalid portfolio input")
	ErrInvalidReference = errors.New("invalid portfolio reference")
	ErrConflict         = errors.New("portfolio conflict")
	ErrLimit            = errors.New("portfolio plan limit reached")
	ErrItemLimit        = errors.New("portfolio item plan limit reached")
	ErrMediaLimit       = errors.New("portfolio media plan limit reached")
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type Reference struct {
	ID   string `json:"id"`
	Slug string `json:"slug,omitempty"`
	Name string `json:"name,omitempty"`
}

type Media struct {
	ID         string `json:"id"`
	MIMEType   string `json:"mime_type"`
	SizeBytes  int64  `json:"size_bytes"`
	ScanStatus string `json:"scan_status,omitempty"`
	SortOrder  int    `json:"sort_order"`
}

type MediaObject struct {
	ID         string
	OwnerID    string
	MIMEType   string
	SizeBytes  int64
	Purpose    string
	ScanStatus string
	Uploaded   bool
	Deleted    bool
}

type Item struct {
	ID              string      `json:"id"`
	UserID          string      `json:"-"`
	Username        string      `json:"-"`
	Title           string      `json:"title"`
	Slug            string      `json:"slug"`
	Description     string      `json:"description,omitempty"`
	ExternalURL     string      `json:"external_url,omitempty"`
	PriceMinKopecks *int64      `json:"price_min_kopecks,omitempty"`
	PriceMaxKopecks *int64      `json:"price_max_kopecks,omitempty"`
	CompletedOn     string      `json:"completed_on,omitempty"`
	Visibility      string      `json:"visibility"`
	SortOrder       int         `json:"sort_order"`
	Categories      []Reference `json:"categories"`
	Skills          []Reference `json:"skills"`
	Media           []Media     `json:"media"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
	DeletedAt       *time.Time  `json:"-"`
}

type WriteRequest struct {
	Title           string   `json:"title"`
	Slug            string   `json:"slug"`
	Description     string   `json:"description"`
	ExternalURL     string   `json:"external_url"`
	PriceMinKopecks *int64   `json:"price_min_kopecks"`
	PriceMaxKopecks *int64   `json:"price_max_kopecks"`
	CompletedOn     string   `json:"completed_on"`
	Visibility      string   `json:"visibility"`
	SortOrder       int      `json:"sort_order"`
	CategoryIDs     []string `json:"category_ids"`
	SkillIDs        []string `json:"skill_ids"`
	MediaObjectIDs  []string `json:"media_object_ids"`
}

type Cursor struct {
	SortOrder int
	CreatedAt time.Time
	ID        string
}

type Page struct {
	Items      []Item
	NextCursor *Cursor
}

type Repository interface {
	Create(context.Context, string, WriteRequest) (Item, error)
	GetOwned(context.Context, string, string) (Item, error)
	Update(context.Context, string, string, WriteRequest) (Item, error)
	Delete(context.Context, string, string) error
	AttachMedia(context.Context, string, string, string, int) (Item, error)
	DetachMedia(context.Context, string, string, string) error
	ListOwned(context.Context, string) ([]Item, error)
	ListPublic(context.Context, string, *Cursor, int) (Page, error)
}

func (s *Store) ListOwned(_ context.Context, actorID string) ([]Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]Item, 0)
	for _, item := range s.Items {
		if item.UserID == actorID && item.DeletedAt == nil {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].SortOrder == items[j].SortOrder {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].SortOrder < items[j].SortOrder
	})
	return items, nil
}

type Store struct {
	mu    sync.RWMutex
	Items map[string]Item
	Media map[string]MediaObject
	Now   func() time.Time
}

func (s *Store) Create(_ context.Context, actorID string, input WriteRequest) (Item, error) {
	if actorID == "" {
		return Item{}, ErrUnauthorized
	}
	itemID, err := newUUIDv7()
	if err != nil {
		return Item{}, err
	}
	item, err := s.itemFromInput(actorID, itemID, input)
	if err != nil {
		return Item{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Items == nil {
		s.Items = make(map[string]Item)
	}
	if slugExists(s.Items, actorID, item.Slug, "") {
		return Item{}, ErrConflict
	}
	if err := s.attachInputMedia(actorID, &item, input.MediaObjectIDs); err != nil {
		return Item{}, err
	}
	s.Items[item.ID] = item
	return item, nil
}

func (s *Store) GetOwned(_ context.Context, actorID, itemID string) (Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.Items[itemID]
	if !ok || item.UserID != actorID || item.DeletedAt != nil {
		return Item{}, ErrNotFound
	}
	return item, nil
}

func (s *Store) Update(_ context.Context, actorID, itemID string, input WriteRequest) (Item, error) {
	if actorID == "" {
		return Item{}, ErrUnauthorized
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.Items[itemID]
	if !ok || existing.UserID != actorID || existing.DeletedAt != nil {
		return Item{}, ErrNotFound
	}
	updated, err := s.itemFromInput(actorID, itemID, input)
	if err != nil {
		return Item{}, err
	}
	if slugExists(s.Items, actorID, updated.Slug, itemID) {
		return Item{}, ErrConflict
	}
	updated.Username = existing.Username
	updated.CreatedAt = existing.CreatedAt
	if err := s.attachInputMedia(actorID, &updated, input.MediaObjectIDs); err != nil {
		return Item{}, err
	}
	s.Items[itemID] = updated
	return updated, nil
}

func (s *Store) Delete(_ context.Context, actorID, itemID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.Items[itemID]
	if !ok || item.UserID != actorID || item.DeletedAt != nil {
		return ErrNotFound
	}
	now := s.now()
	item.DeletedAt = &now
	item.UpdatedAt = now
	s.Items[itemID] = item
	return nil
}

func (s *Store) AttachMedia(_ context.Context, actorID, itemID, mediaID string, sortOrder int) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.Items[itemID]
	if !ok || item.UserID != actorID || item.DeletedAt != nil {
		return Item{}, ErrNotFound
	}
	if sortOrder < 0 || sortOrder > 10000 || !validUUID(mediaID) {
		return Item{}, invalid("invalid media reference")
	}
	for _, media := range item.Media {
		if media.ID == mediaID {
			return item, nil
		}
	}
	if len(item.Media) >= 20 {
		return Item{}, invalid("too many media references")
	}
	media, err := s.validMedia(actorID, mediaID)
	if err != nil {
		return Item{}, err
	}
	item.Media = append(item.Media, Media{ID: media.ID, MIMEType: media.MIMEType, SizeBytes: media.SizeBytes, ScanStatus: media.ScanStatus, SortOrder: sortOrder})
	sort.Slice(item.Media, func(i, j int) bool { return item.Media[i].SortOrder < item.Media[j].SortOrder })
	item.UpdatedAt = s.now()
	s.Items[itemID] = item
	return item, nil
}

func (s *Store) DetachMedia(_ context.Context, actorID, itemID, mediaID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.Items[itemID]
	if !ok || item.UserID != actorID || item.DeletedAt != nil {
		return ErrNotFound
	}
	filtered := item.Media[:0]
	for _, media := range item.Media {
		if media.ID != mediaID {
			filtered = append(filtered, media)
		}
	}
	item.Media = filtered
	item.UpdatedAt = s.now()
	s.Items[itemID] = item
	return nil
}

func (s *Store) ListPublic(_ context.Context, username string, cursor *Cursor, limit int) (Page, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]Item, 0)
	for _, item := range s.Items {
		if strings.EqualFold(item.Username, username) && item.Visibility == "PUBLIC" && item.DeletedAt == nil && afterCursor(item, cursor) {
			public := item
			public.UserID = ""
			public.Media = make([]Media, 0, len(item.Media))
			for _, attached := range item.Media {
				media, ok := s.Media[attached.ID]
				if !ok || media.Deleted || media.ScanStatus != "CLEAN" {
					continue
				}
				attached.ScanStatus = ""
				public.Media = append(public.Media, attached)
			}
			items = append(items, public)
		}
	}
	sortItems(items)
	if limit < 1 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	page := Page{Items: items}
	if len(page.Items) > limit {
		last := page.Items[limit-1]
		page.Items = page.Items[:limit]
		page.NextCursor = &Cursor{SortOrder: last.SortOrder, CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return page, nil
}

func (s *Store) itemFromInput(actorID, itemID string, input WriteRequest) (Item, error) {
	now := s.now()
	item := Item{
		ID: itemID, UserID: actorID, Title: strings.TrimSpace(input.Title), Slug: strings.ToLower(strings.TrimSpace(input.Slug)),
		Description: strings.TrimSpace(input.Description), ExternalURL: strings.TrimSpace(input.ExternalURL),
		PriceMinKopecks: input.PriceMinKopecks, PriceMaxKopecks: input.PriceMaxKopecks,
		CompletedOn: strings.TrimSpace(input.CompletedOn), Visibility: input.Visibility, SortOrder: input.SortOrder,
		Categories: references(input.CategoryIDs), Skills: references(input.SkillIDs), Media: []Media{}, CreatedAt: now, UpdatedAt: now,
	}
	if err := Validate(item, now); err != nil {
		return Item{}, err
	}
	return item, nil
}

func (s *Store) attachInputMedia(actorID string, item *Item, ids []string) error {
	if len(ids) > 20 {
		return invalid("too many media references")
	}
	seen := make(map[string]struct{}, len(ids))
	for index, id := range ids {
		id = strings.ToLower(strings.TrimSpace(id))
		if !validUUID(id) {
			return invalid("invalid media id")
		}
		if _, ok := seen[id]; ok {
			return invalid("duplicate media id")
		}
		seen[id] = struct{}{}
		media, err := s.validMedia(actorID, id)
		if err != nil {
			return err
		}
		item.Media = append(item.Media, Media{ID: id, MIMEType: media.MIMEType, SizeBytes: media.SizeBytes, ScanStatus: media.ScanStatus, SortOrder: index})
	}
	return nil
}

func (s *Store) validMedia(actorID, mediaID string) (MediaObject, error) {
	media, ok := s.Media[mediaID]
	if !ok || media.OwnerID != actorID || media.Deleted || !media.Uploaded || media.Purpose != "PORTFOLIO" || media.ScanStatus != "CLEAN" {
		return MediaObject{}, fmt.Errorf("%w: media is not attachable", ErrInvalidReference)
	}
	return media, nil
}

func Validate(item Item, now time.Time) error {
	if len([]rune(item.Title)) < 1 || len([]rune(item.Title)) > 180 || !slugPattern.MatchString(item.Slug) || len(item.Slug) > 220 {
		return invalid("invalid title or slug")
	}
	if len([]rune(item.Description)) > 5000 || len(item.ExternalURL) > 2048 {
		return invalid("portfolio text is too long")
	}
	if item.ExternalURL != "" {
		parsed, err := url.ParseRequestURI(item.ExternalURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return invalid("invalid external url")
		}
	}
	if (item.PriceMinKopecks != nil && *item.PriceMinKopecks < 0) || (item.PriceMaxKopecks != nil && *item.PriceMaxKopecks < 0) {
		return invalid("invalid portfolio price")
	}
	if item.PriceMinKopecks != nil && item.PriceMaxKopecks != nil && *item.PriceMinKopecks > *item.PriceMaxKopecks {
		return invalid("invalid portfolio price range")
	}
	if item.CompletedOn != "" {
		completed, err := time.Parse("2006-01-02", item.CompletedOn)
		if err != nil || completed.After(now.UTC().Truncate(24*time.Hour)) {
			return invalid("invalid completion date")
		}
	}
	if (item.Visibility != "PUBLIC" && item.Visibility != "PRIVATE") || item.SortOrder < 0 || item.SortOrder > 10000 {
		return invalid("invalid visibility or sort order")
	}
	if len(item.Categories) > 10 || len(item.Skills) > 50 {
		return invalid("too many portfolio associations")
	}
	if err := validateReferences(item.Categories, "category"); err != nil {
		return err
	}
	return validateReferences(item.Skills, "skill")
}

func references(ids []string) []Reference {
	out := make([]Reference, len(ids))
	for index, id := range ids {
		out[index] = Reference{ID: strings.ToLower(strings.TrimSpace(id))}
	}
	return out
}

func validateReferences(items []Reference, kind string) error {
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if !validUUID(item.ID) {
			return invalid("invalid " + kind + " id")
		}
		if _, ok := seen[item.ID]; ok {
			return invalid("duplicate " + kind + " id")
		}
		seen[item.ID] = struct{}{}
	}
	return nil
}

func validateIDList(ids []string, maximum int, kind string) error {
	if len(ids) > maximum {
		return invalid("too many " + kind + " references")
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.ToLower(strings.TrimSpace(id))
		if !validUUID(id) {
			return invalid("invalid " + kind + " id")
		}
		if _, ok := seen[id]; ok {
			return invalid("duplicate " + kind + " id")
		}
		seen[id] = struct{}{}
	}
	return nil
}

func slugExists(items map[string]Item, actorID, slug, exceptID string) bool {
	for _, item := range items {
		if item.UserID == actorID && item.Slug == slug && item.ID != exceptID && item.DeletedAt == nil {
			return true
		}
	}
	return false
}

func sortItems(items []Item) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].SortOrder != items[j].SortOrder {
			return items[i].SortOrder < items[j].SortOrder
		}
		if !items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].ID < items[j].ID
	})
}

func afterCursor(item Item, cursor *Cursor) bool {
	if cursor == nil {
		return true
	}
	if item.SortOrder != cursor.SortOrder {
		return item.SortOrder > cursor.SortOrder
	}
	if !item.CreatedAt.Equal(cursor.CreatedAt) {
		return item.CreatedAt.Before(cursor.CreatedAt)
	}
	return item.ID > cursor.ID
}

func validUUID(id string) bool {
	return uuidPattern.MatchString(strings.ToLower(strings.TrimSpace(id)))
}
func invalid(message string) error { return fmt.Errorf("%w: %s", ErrInvalidInput, message) }

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func newUUIDv7() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	milliseconds := uint64(time.Now().UTC().UnixMilli())
	bytes[0] = byte(milliseconds >> 40)
	bytes[1] = byte(milliseconds >> 32)
	bytes[2] = byte(milliseconds >> 24)
	bytes[3] = byte(milliseconds >> 16)
	bytes[4] = byte(milliseconds >> 8)
	bytes[5] = byte(milliseconds)
	bytes[6] = (bytes[6] & 0x0f) | 0x70
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	var encoded [36]byte
	hex.Encode(encoded[0:8], bytes[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], bytes[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], bytes[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], bytes[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], bytes[10:16])
	return string(encoded[:]), nil
}
