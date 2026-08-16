package projects

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrUnauthorized       = errors.New("unauthenticated")
	ErrNotFound           = errors.New("project not found")
	ErrInvalidInput       = errors.New("invalid project input")
	ErrInvalidReference   = errors.New("invalid project reference")
	ErrConflict           = errors.New("project conflict")
	ErrInvalidState       = errors.New("invalid project state transition")
	ErrCustomerIneligible = errors.New("customer is not eligible")
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type Budget struct {
	Type       string `json:"type"`
	MinKopecks *int64 `json:"min_kopecks,omitempty"`
	MaxKopecks *int64 `json:"max_kopecks,omitempty"`
	Currency   string `json:"currency"`
}

type Reference struct {
	ID   string `json:"id"`
	Slug string `json:"slug,omitempty"`
	Name string `json:"name,omitempty"`
}
type Skill struct {
	Reference
	Importance int `json:"importance"`
}
type Media struct {
	ID               string `json:"id"`
	OriginalFilename string `json:"original_filename,omitempty"`
	MIMEType         string `json:"mime_type"`
	SizeBytes        int64  `json:"size_bytes"`
	SortOrder        int    `json:"sort_order"`
}
type MediaObject struct {
	ID, OwnerID, Purpose, OriginalFilename, MIMEType, ScanStatus string
	SizeBytes                                                    int64
	Uploaded, Deleted                                            bool
}

type Item struct {
	ID                  string     `json:"id"`
	CustomerID          string     `json:"-"`
	CustomerDisplayName string     `json:"customer_display_name,omitempty"`
	CustomerUsername    string     `json:"customer_username,omitempty"`
	Category            *Reference `json:"category,omitempty"`
	Title               string     `json:"title"`
	Slug                string     `json:"slug"`
	Description         string     `json:"description"`
	Budget              Budget     `json:"budget"`
	DeadlineAt          *time.Time `json:"deadline_at,omitempty"`
	ExperienceLevel     string     `json:"experience_level,omitempty"`
	Visibility          string     `json:"visibility"`
	Status              string     `json:"status"`
	ModerationStatus    string     `json:"moderation_status,omitempty"`
	ModerationReason    string     `json:"moderation_reason,omitempty"`
	SourceType          string     `json:"source_type"`
	PublishedAt         *time.Time `json:"published_at,omitempty"`
	ProposalCount       int        `json:"proposal_count"`
	Skills              []Skill    `json:"skills"`
	Media               []Media    `json:"media"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	DeletedAt           *time.Time `json:"-"`
}

type CreateRequest struct {
	CategoryID       string     `json:"category_id"`
	Title            string     `json:"title"`
	Slug             string     `json:"slug"`
	Description      string     `json:"description"`
	Budget           Budget     `json:"budget"`
	DeadlineAt       *time.Time `json:"deadline_at"`
	ExperienceLevel  string     `json:"experience_level"`
	Visibility       string     `json:"visibility"`
	SkillIDs         []string   `json:"skill_ids"`
	MediaObjectIDs   []string   `json:"media_ids"`
	SourceDraftToken string     `json:"source_draft_token,omitempty"`
}

type PatchRequest struct {
	CategoryID      *string    `json:"category_id"`
	Title           *string    `json:"title"`
	Slug            *string    `json:"slug"`
	Description     *string    `json:"description"`
	Budget          *Budget    `json:"budget"`
	DeadlineAt      *time.Time `json:"deadline_at"`
	ExperienceLevel *string    `json:"experience_level"`
	Visibility      *string    `json:"visibility"`
	SkillIDs        *[]string  `json:"skill_ids"`
	MediaObjectIDs  *[]string  `json:"media_ids"`
}

type Filter struct {
	Q                string
	Category         string
	BudgetType       string
	ExperienceLevel  string
	MinBudgetKopecks *int64
	DeadlineBefore   *time.Time
}
type Cursor struct {
	At time.Time
	ID string
}
type Page struct {
	Items      []Item
	NextCursor *Cursor
}
type Event struct {
	AggregateID, Type string
	CreatedAt         time.Time
}

type Repository interface {
	Create(context.Context, string, CreateRequest) (Item, error)
	ListOwned(context.Context, string, *Cursor, int) (Page, error)
	GetOwned(context.Context, string, string) (Item, error)
	Update(context.Context, string, string, PatchRequest) (Item, error)
	Delete(context.Context, string, string) error
	Transition(context.Context, string, string, string) (Item, error)
}
type SearchEngine interface {
	ListPublic(context.Context, Filter, *Cursor, int) (Page, error)
	GetPublic(context.Context, string) (Item, error)
}

type Store struct {
	mu               sync.RWMutex
	Items            map[string]Item
	Categories       map[string]Reference
	Skills           map[string]Reference
	Media            map[string]MediaObject
	CustomerEligible map[string]bool
	Events           []Event
	DealCompleted    map[string]bool
	Now              func() time.Time
}

func (s *Store) Create(_ context.Context, actorID string, input CreateRequest) (Item, error) {
	if actorID == "" {
		return Item{}, ErrUnauthorized
	}
	if input.SourceDraftToken != "" && (len(input.SourceDraftToken) != 64 || strings.IndexFunc(input.SourceDraftToken, func(r rune) bool { return !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') }) >= 0) {
		return Item{}, fmt.Errorf("%w: invalid source draft", ErrInvalidInput)
	}
	id, err := newUUIDv7()
	if err != nil {
		return Item{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.CustomerEligible != nil && !s.CustomerEligible[actorID] {
		return Item{}, ErrCustomerIneligible
	}
	item := itemFromCreate(actorID, id, input, s.now())
	if err := Validate(item, s.now(), false); err != nil {
		return Item{}, err
	}
	if err := s.resolveReferences(actorID, &item, false); err != nil {
		return Item{}, err
	}
	if slugExists(s.Items, actorID, item.Slug, "") {
		return Item{}, ErrConflict
	}
	if s.Items == nil {
		s.Items = map[string]Item{}
	}
	s.Items[id] = item
	return item, nil
}

func (s *Store) ListOwned(_ context.Context, actorID string, cursor *Cursor, limit int) (Page, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]Item, 0)
	for _, item := range s.Items {
		if item.CustomerID == actorID && item.DeletedAt == nil && after(item.CreatedAt, item.ID, cursor) {
			items = append(items, item)
		}
	}
	return paginate(items, limit, func(item Item) time.Time { return item.CreatedAt }), nil
}
func (s *Store) GetOwned(_ context.Context, actorID, id string) (Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.Items[id]
	if !ok || item.CustomerID != actorID || item.DeletedAt != nil {
		return Item{}, ErrNotFound
	}
	return item, nil
}
func (s *Store) Update(_ context.Context, actorID, id string, patch PatchRequest) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.Items[id]
	if !ok || item.CustomerID != actorID || item.DeletedAt != nil {
		return Item{}, ErrNotFound
	}
	if item.Status != "DRAFT" {
		return Item{}, ErrInvalidState
	}
	item = mergePatch(item, patch, s.now())
	if err := Validate(item, s.now(), false); err != nil {
		return Item{}, err
	}
	if err := s.resolveReferences(actorID, &item, false); err != nil {
		return Item{}, err
	}
	if slugExists(s.Items, actorID, item.Slug, id) {
		return Item{}, ErrConflict
	}
	s.Items[id] = item
	return item, nil
}
func (s *Store) Delete(_ context.Context, actorID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.Items[id]
	if !ok || item.CustomerID != actorID || item.DeletedAt != nil {
		return ErrNotFound
	}
	if item.Status != "DRAFT" && item.Status != "CANCELLED" && item.Status != "COMPLETED" {
		return ErrInvalidState
	}
	now := s.now()
	item.Status, item.DeletedAt, item.UpdatedAt = "ARCHIVED", &now, now
	s.Items[id] = item
	return nil
}
func (s *Store) Transition(_ context.Context, actorID, id, action string) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.Items[id]
	if !ok || item.CustomerID != actorID || item.DeletedAt != nil {
		return Item{}, ErrNotFound
	}
	if action == "complete" && item.Status == "COMPLETED" {
		return item, nil
	}
	target, allowed := transition(item.Status, action)
	if !allowed {
		return Item{}, ErrInvalidState
	}
	if action == "publish" {
		if s.CustomerEligible != nil && !s.CustomerEligible[actorID] {
			return Item{}, ErrCustomerIneligible
		}
		if err := Validate(item, s.now(), true); err != nil {
			return Item{}, err
		}
		if err := s.resolveReferences(actorID, &item, true); err != nil {
			return Item{}, err
		}
	}
	if action == "complete" && (s.DealCompleted == nil || !s.DealCompleted[id]) {
		return Item{}, ErrInvalidState
	}
	now := s.now()
	item.Status, item.UpdatedAt = target, now
	if action == "make-public" {
		item.Visibility = "PUBLIC"
	}
	if action == "publish" && item.PublishedAt == nil {
		item.PublishedAt = &now
	}
	s.Items[id] = item
	s.Events = append(s.Events, Event{AggregateID: id, Type: eventType(action), CreatedAt: now})
	return item, nil
}

func (s *Store) ListPublic(_ context.Context, filter Filter, cursor *Cursor, limit int) (Page, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := ValidateFilter(filter); err != nil {
		return Page{}, err
	}
	items := make([]Item, 0)
	for _, item := range s.Items {
		categoryActive := item.Category != nil
		if categoryActive {
			_, categoryActive = s.Categories[item.Category.ID]
		}
		if !categoryActive || !oneOf(item.Status, "OPEN", "MATCHING") || item.Visibility != "PUBLIC" || item.DeletedAt != nil || item.PublishedAt == nil || !after(*item.PublishedAt, item.ID, cursor) || !matches(item, filter) {
			continue
		}
		items = append(items, s.publicItem(item))
	}
	return paginate(items, limit, func(item Item) time.Time { return *item.PublishedAt }), nil
}
func (s *Store) GetPublic(_ context.Context, reference string) (Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if validUUID(reference) {
		item, ok := s.Items[normalizeID(reference)]
		if !ok || !s.publiclyVisible(item) {
			return Item{}, ErrNotFound
		}
		return s.publicItem(item), nil
	}
	var found *Item
	for _, item := range s.Items {
		if item.Slug != reference || !s.publiclyVisible(item) {
			continue
		}
		if found != nil {
			return Item{}, ErrNotFound
		}
		copy := s.publicItem(item)
		found = &copy
	}
	if found == nil {
		return Item{}, ErrNotFound
	}
	return *found, nil
}
func (s *Store) publiclyVisible(item Item) bool {
	if item.Category == nil {
		return false
	}
	_, active := s.Categories[item.Category.ID]
	return active && oneOf(item.Status, "OPEN", "MATCHING") && item.Visibility == "PUBLIC" && item.PublishedAt != nil && item.DeletedAt == nil
}
func (s *Store) publicItem(item Item) Item {
	item.CustomerID = ""
	skills := make([]Skill, 0, len(item.Skills))
	for _, skill := range item.Skills {
		if current, ok := s.Skills[skill.ID]; ok {
			skills = append(skills, Skill{Reference: current, Importance: skill.Importance})
		}
	}
	item.Skills = skills
	media := make([]Media, 0, len(item.Media))
	for _, attached := range item.Media {
		current, ok := s.Media[attached.ID]
		if ok && !current.Deleted && current.Purpose == "PROJECT" && current.Uploaded && current.ScanStatus == "CLEAN" {
			media = append(media, attached)
		}
	}
	item.Media = media
	return item
}

func Validate(item Item, now time.Time, publish bool) error {
	if len([]rune(strings.TrimSpace(item.Title))) < 1 || len([]rune(item.Title)) > 200 || !slugPattern.MatchString(item.Slug) || len(item.Slug) > 240 {
		return invalid("invalid title or slug")
	}
	if len([]rune(strings.TrimSpace(item.Description))) < 1 || len([]rune(item.Description)) > 15000 {
		return invalid("invalid project description")
	}
	if err := validateBudget(item.Budget); err != nil {
		return err
	}
	if item.DeadlineAt != nil && !item.DeadlineAt.After(now) {
		return invalid("deadline must be in the future")
	}
	if item.ExperienceLevel != "" && !oneOf(item.ExperienceLevel, "BEGINNER", "INTERMEDIATE", "ADVANCED", "EXPERT") {
		return invalid("invalid experience level")
	}
	if item.Visibility != "PUBLIC" && item.Visibility != "PRIVATE" {
		return invalid("invalid visibility")
	}
	if !oneOf(item.SourceType, "MANUAL", "AI_BRIEF", "IMPORT", "COMMERCIAL_OFFER", "CALCULATOR", "REPEAT", "INVITE") {
		return invalid("unsupported project source")
	}
	if len(item.Skills) > 30 || len(item.Media) > 12 {
		return invalid("too many project references")
	}
	if publish && item.Category == nil {
		return invalid("category is required to publish")
	}
	return nil
}
func validateBudget(budget Budget) error {
	if budget.Currency != "RUB" || !oneOf(budget.Type, "FIXED", "RANGE", "NEGOTIABLE", "HOURLY") {
		return invalid("invalid project budget")
	}
	switch budget.Type {
	case "NEGOTIABLE":
		if budget.MinKopecks != nil || budget.MaxKopecks != nil {
			return invalid("negotiable budget cannot set amounts")
		}
	case "FIXED":
		if budget.MinKopecks == nil || *budget.MinKopecks <= 0 || budget.MaxKopecks != nil {
			return invalid("fixed budget requires one positive amount")
		}
	case "RANGE", "HOURLY":
		if budget.MinKopecks == nil || budget.MaxKopecks == nil || *budget.MinKopecks <= 0 || *budget.MaxKopecks < *budget.MinKopecks {
			return invalid("budget range is invalid")
		}
	}
	return nil
}
func ValidateFilter(filter Filter) error {
	if len([]rune(filter.Q)) > 120 || len(filter.Category) > 120 || (filter.Category != "" && !validUUID(filter.Category) && !slugPattern.MatchString(filter.Category)) ||
		(filter.BudgetType != "" && !oneOf(filter.BudgetType, "FIXED", "RANGE", "NEGOTIABLE", "HOURLY")) ||
		(filter.ExperienceLevel != "" && !oneOf(filter.ExperienceLevel, "BEGINNER", "INTERMEDIATE", "ADVANCED", "EXPERT")) ||
		(filter.MinBudgetKopecks != nil && (*filter.MinBudgetKopecks < 0 || *filter.MinBudgetKopecks > 1_000_000_000_000_000)) {
		return invalid("invalid project filter")
	}
	return nil
}

func itemFromCreate(actorID, id string, input CreateRequest, now time.Time) Item {
	item := Item{ID: id, CustomerID: actorID, Title: strings.TrimSpace(input.Title), Slug: strings.ToLower(strings.TrimSpace(input.Slug)), Description: strings.TrimSpace(input.Description), Budget: normalizeBudget(input.Budget), DeadlineAt: utcTime(input.DeadlineAt), ExperienceLevel: strings.ToUpper(strings.TrimSpace(input.ExperienceLevel)), Visibility: strings.ToUpper(strings.TrimSpace(input.Visibility)), Status: "DRAFT", SourceType: "MANUAL", Skills: skillRefs(input.SkillIDs), Media: mediaRefs(input.MediaObjectIDs), CreatedAt: now, UpdatedAt: now}
	if id := normalizeID(input.CategoryID); id != "" {
		item.Category = &Reference{ID: id}
	}
	return item
}
func mergePatch(item Item, patch PatchRequest, now time.Time) Item {
	if patch.CategoryID != nil {
		id := normalizeID(*patch.CategoryID)
		if id == "" {
			item.Category = nil
		} else {
			item.Category = &Reference{ID: id}
		}
	}
	if patch.Title != nil {
		item.Title = strings.TrimSpace(*patch.Title)
	}
	if patch.Slug != nil {
		item.Slug = strings.ToLower(strings.TrimSpace(*patch.Slug))
	}
	if patch.Description != nil {
		item.Description = strings.TrimSpace(*patch.Description)
	}
	if patch.Budget != nil {
		item.Budget = normalizeBudget(*patch.Budget)
	}
	if patch.DeadlineAt != nil {
		item.DeadlineAt = utcTime(patch.DeadlineAt)
	}
	if patch.ExperienceLevel != nil {
		item.ExperienceLevel = strings.ToUpper(strings.TrimSpace(*patch.ExperienceLevel))
	}
	if patch.Visibility != nil {
		item.Visibility = strings.ToUpper(strings.TrimSpace(*patch.Visibility))
	}
	if patch.SkillIDs != nil {
		item.Skills = skillRefs(*patch.SkillIDs)
	}
	if patch.MediaObjectIDs != nil {
		item.Media = mediaRefs(*patch.MediaObjectIDs)
	}
	item.UpdatedAt = now
	return item
}
func (s *Store) resolveReferences(actorID string, item *Item, requireCategory bool) error {
	if item.Category != nil {
		reference, ok := s.Categories[normalizeID(item.Category.ID)]
		if !ok {
			return fmt.Errorf("%w: category is not active", ErrInvalidReference)
		}
		item.Category = &reference
	} else if requireCategory {
		return invalid("category is required to publish")
	}
	seen := map[string]struct{}{}
	skills := make([]Skill, 0, len(item.Skills))
	for _, skill := range item.Skills {
		id := normalizeID(skill.ID)
		if !validUUID(id) {
			return invalid("invalid skill id")
		}
		if _, ok := seen[id]; ok {
			return invalid("duplicate skill id")
		}
		seen[id] = struct{}{}
		reference, ok := s.Skills[id]
		if !ok {
			return fmt.Errorf("%w: skill is not active", ErrInvalidReference)
		}
		skills = append(skills, Skill{Reference: reference, Importance: 100})
	}
	item.Skills = skills
	seen = map[string]struct{}{}
	media := make([]Media, 0, len(item.Media))
	for index, attached := range item.Media {
		id := normalizeID(attached.ID)
		if !validUUID(id) {
			return invalid("invalid media id")
		}
		if _, ok := seen[id]; ok {
			return invalid("duplicate media id")
		}
		seen[id] = struct{}{}
		object, ok := s.Media[id]
		if !ok || object.OwnerID != actorID || object.Purpose != "PROJECT" || !object.Uploaded || object.ScanStatus != "CLEAN" || object.Deleted {
			return fmt.Errorf("%w: media is not attachable", ErrInvalidReference)
		}
		media = append(media, Media{ID: id, OriginalFilename: object.OriginalFilename, MIMEType: object.MIMEType, SizeBytes: object.SizeBytes, SortOrder: index})
	}
	item.Media = media
	return nil
}

func transition(status, action string) (string, bool) {
	switch action {
	case "publish":
		return "OPEN", status == "DRAFT"
	case "make-public":
		return status, oneOf(status, "OPEN", "MATCHING", "IN_PROGRESS", "COMPLETED")
	case "cancel":
		return "CANCELLED", status == "DRAFT" || status == "OPEN" || status == "MATCHING"
	case "complete":
		return "COMPLETED", status == "IN_PROGRESS"
	default:
		return "", false
	}
}
func eventType(action string) string {
	switch action {
	case "publish":
		return "PROJECT_PUBLISHED"
	case "make-public":
		return "PROJECT_VISIBILITY_CHANGED"
	case "cancel":
		return "PROJECT_CANCELLED"
	case "complete":
		return "PROJECT_COMPLETED"
	default:
		return "PROJECT_CHANGED"
	}
}
func matches(item Item, filter Filter) bool {
	q := strings.ToLower(filter.Q)
	if q != "" && !strings.Contains(strings.ToLower(item.Title+" "+item.Description), q) {
		return false
	}
	if filter.Category != "" && (item.Category == nil || item.Category.ID != filter.Category && item.Category.Slug != filter.Category) {
		return false
	}
	if filter.BudgetType != "" && item.Budget.Type != filter.BudgetType {
		return false
	}
	if filter.ExperienceLevel != "" && item.ExperienceLevel != filter.ExperienceLevel {
		return false
	}
	if filter.MinBudgetKopecks != nil {
		amount := item.Budget.MaxKopecks
		if amount == nil {
			amount = item.Budget.MinKopecks
		}
		if amount == nil || *amount < *filter.MinBudgetKopecks {
			return false
		}
	}
	if filter.DeadlineBefore != nil && item.DeadlineAt != nil && item.DeadlineAt.After(*filter.DeadlineBefore) {
		return false
	}
	return true
}
func paginate(items []Item, limit int, timestamp func(Item) time.Time) Page {
	if limit < 1 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	sort.Slice(items, func(i, j int) bool {
		left, right := timestamp(items[i]), timestamp(items[j])
		if !left.Equal(right) {
			return left.After(right)
		}
		return items[i].ID > items[j].ID
	})
	page := Page{Items: items}
	if len(items) > limit {
		last := items[limit-1]
		page.Items = items[:limit]
		page.NextCursor = &Cursor{At: timestamp(last), ID: last.ID}
	}
	return page
}
func after(at time.Time, id string, cursor *Cursor) bool {
	return cursor == nil || at.Before(cursor.At) || at.Equal(cursor.At) && id < cursor.ID
}
func normalizeBudget(value Budget) Budget {
	value.Type = strings.ToUpper(strings.TrimSpace(value.Type))
	value.Currency = strings.ToUpper(strings.TrimSpace(value.Currency))
	return value
}
func utcTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}
func skillRefs(ids []string) []Skill {
	values := make([]Skill, len(ids))
	for i, id := range ids {
		values[i] = Skill{Reference: Reference{ID: normalizeID(id)}, Importance: 100}
	}
	return values
}
func mediaRefs(ids []string) []Media {
	values := make([]Media, len(ids))
	for i, id := range ids {
		values[i] = Media{ID: normalizeID(id), SortOrder: i}
	}
	return values
}
func normalizeID(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
func validUUID(value string) bool     { return uuidPattern.MatchString(normalizeID(value)) }
func oneOf(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
func invalid(message string) error { return fmt.Errorf("%w: %s", ErrInvalidInput, message) }
func slugExists(items map[string]Item, actorID, slug, except string) bool {
	for _, item := range items {
		if item.CustomerID == actorID && item.Slug == slug && item.ID != except && item.DeletedAt == nil {
			return true
		}
	}
	return false
}
func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func newUUIDv7() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	milliseconds := uint64(time.Now().UTC().UnixMilli())
	value[0], value[1], value[2] = byte(milliseconds>>40), byte(milliseconds>>32), byte(milliseconds>>24)
	value[3], value[4], value[5] = byte(milliseconds>>16), byte(milliseconds>>8), byte(milliseconds)
	value[6], value[8] = (value[6]&0x0f)|0x70, (value[8]&0x3f)|0x80
	var encoded [36]byte
	hex.Encode(encoded[0:8], value[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], value[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], value[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], value[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], value[10:16])
	return string(encoded[:]), nil
}
