package services

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
	ErrUnauthorized     = errors.New("unauthenticated")
	ErrNotFound         = errors.New("service not found")
	ErrInvalidInput     = errors.New("invalid service input")
	ErrInvalidReference = errors.New("invalid service reference")
	ErrConflict         = errors.New("service conflict")
	ErrInvalidState     = errors.New("invalid service state transition")
	ErrSellerIneligible = errors.New("seller is not eligible")
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type Money struct {
	AmountKopecks int64  `json:"amount_kopecks"`
	Currency      string `json:"currency"`
}

type Reference struct {
	ID   string `json:"id"`
	Slug string `json:"slug,omitempty"`
	Name string `json:"name,omitempty"`
}

type Media struct {
	ID        string `json:"id"`
	MIMEType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
	SortOrder int    `json:"sort_order"`
}

type MediaObject struct {
	ID         string
	OwnerID    string
	Purpose    string
	MIMEType   string
	SizeBytes  int64
	Uploaded   bool
	ScanStatus string
	Deleted    bool
}

type EducationDetails struct {
	Format          string `json:"format"`
	DurationMinutes *int   `json:"duration_minutes,omitempty"`
	SessionsCount   *int   `json:"sessions_count,omitempty"`
	AudienceType    string `json:"audience_type,omitempty"`
	GroupSizeMax    *int   `json:"group_size_max,omitempty"`
}

type Item struct {
	ID                 string            `json:"id"`
	SellerID           string            `json:"seller_user_id,omitempty"`
	SellerUsername     string            `json:"seller_username,omitempty"`
	SellerDisplayName  string            `json:"seller_display_name,omitempty"`
	SellerNativeRating *float64          `json:"seller_native_rating,omitempty"`
	SellerReviewsCount int               `json:"seller_reviews_count"`
	Category           Reference         `json:"category"`
	ServiceType        string            `json:"service_type"`
	Title              string            `json:"title"`
	Slug               string            `json:"slug"`
	ShortDescription   string            `json:"short_description,omitempty"`
	Description        string            `json:"description"`
	PriceType          string            `json:"price_type"`
	PriceFrom          *Money            `json:"price_from,omitempty"`
	DeliveryDays       *int              `json:"delivery_days,omitempty"`
	IncludedRevisions  *int              `json:"included_revisions,omitempty"`
	Status             string            `json:"status"`
	ModerationStatus   string            `json:"moderation_status,omitempty"`
	ModerationReason   string            `json:"moderation_reason,omitempty"`
	Visibility         string            `json:"visibility"`
	PublishedAt        *time.Time        `json:"published_at,omitempty"`
	Skills             []Reference       `json:"skills"`
	Media              []Media           `json:"media"`
	Education          *EducationDetails `json:"education_details,omitempty"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
	DeletedAt          *time.Time        `json:"-"`
}

type CreateRequest struct {
	CategoryID        string            `json:"category_id"`
	ServiceType       string            `json:"service_type"`
	Title             string            `json:"title"`
	Slug              string            `json:"slug"`
	ShortDescription  string            `json:"short_description"`
	Description       string            `json:"description"`
	PriceType         string            `json:"price_type"`
	PriceFrom         *Money            `json:"price_from"`
	DeliveryDays      *int              `json:"delivery_days"`
	IncludedRevisions *int              `json:"included_revisions"`
	Visibility        string            `json:"visibility"`
	SkillIDs          []string          `json:"skill_ids"`
	MediaObjectIDs    []string          `json:"media_object_ids"`
	Education         *EducationDetails `json:"education_details"`
}

type PatchRequest struct {
	CategoryID        *string           `json:"category_id"`
	ServiceType       *string           `json:"service_type"`
	Title             *string           `json:"title"`
	Slug              *string           `json:"slug"`
	ShortDescription  *string           `json:"short_description"`
	Description       *string           `json:"description"`
	PriceType         *string           `json:"price_type"`
	PriceFrom         *Money            `json:"price_from"`
	DeliveryDays      *int              `json:"delivery_days"`
	IncludedRevisions *int              `json:"included_revisions"`
	Visibility        *string           `json:"visibility"`
	SkillIDs          *[]string         `json:"skill_ids"`
	MediaObjectIDs    *[]string         `json:"media_object_ids"`
	Education         *EducationDetails `json:"education_details"`
}

type Filter struct {
	Q                  string
	Category           string
	ServiceType        string
	PriceType          string
	Format             string
	Audience           string
	MaxDurationMinutes *int
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
	Create(context.Context, string, CreateRequest) (Item, error)
	ListOwned(context.Context, string, *Cursor, int) (Page, error)
	GetOwned(context.Context, string, string) (Item, error)
	Update(context.Context, string, string, PatchRequest) (Item, error)
	Delete(context.Context, string, string) error
	Transition(context.Context, string, string, string) (Item, error)
	Moderate(context.Context, string, string, string, string) (Item, error)
}

type SearchEngine interface {
	ListPublic(context.Context, Filter, *Cursor, int) (Page, error)
	GetPublic(context.Context, string) (Item, error)
}

type Store struct {
	mu             sync.RWMutex
	Items          map[string]Item
	Categories     map[string]Reference
	Skills         map[string]Reference
	Media          map[string]MediaObject
	SellerEligible map[string]bool
	Publishable    map[string]bool
	Admins         map[string]bool
	Now            func() time.Time
}

func (s *Store) Create(_ context.Context, actorID string, input CreateRequest) (Item, error) {
	if actorID == "" {
		return Item{}, ErrUnauthorized
	}
	id, err := newUUIDv7()
	if err != nil {
		return Item{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.SellerEligible != nil && !s.SellerEligible[actorID] {
		return Item{}, ErrSellerIneligible
	}
	item := itemFromCreate(actorID, id, input, s.now())
	if err := Validate(item); err != nil {
		return Item{}, err
	}
	if err := s.resolveReferences(actorID, &item, input.CategoryID, input.SkillIDs, input.MediaObjectIDs); err != nil {
		return Item{}, err
	}
	if slugExists(s.Items, actorID, item.Slug, "") {
		return Item{}, ErrConflict
	}
	if s.Items == nil {
		s.Items = make(map[string]Item)
	}
	s.Items[id] = item
	return item, nil
}

func (s *Store) ListOwned(_ context.Context, actorID string, cursor *Cursor, limit int) (Page, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]Item, 0)
	for _, item := range s.Items {
		if item.SellerID == actorID && item.DeletedAt == nil && after(item.CreatedAt, item.ID, cursor) {
			items = append(items, item)
		}
	}
	return paginate(items, limit, func(item Item) time.Time { return item.CreatedAt }), nil
}

func (s *Store) GetOwned(_ context.Context, actorID, serviceID string) (Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.Items[serviceID]
	if !ok || item.SellerID != actorID || item.DeletedAt != nil {
		return Item{}, ErrNotFound
	}
	return item, nil
}

func (s *Store) Update(_ context.Context, actorID, serviceID string, patch PatchRequest) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.Items[serviceID]
	if !ok || existing.SellerID != actorID || existing.DeletedAt != nil {
		return Item{}, ErrNotFound
	}
	if existing.Status != "DRAFT" && existing.Status != "PAUSED" && existing.Status != "REJECTED" {
		return Item{}, ErrInvalidState
	}
	updated := mergePatch(existing, patch, s.now())
	if err := Validate(updated); err != nil {
		return Item{}, err
	}
	categoryID, skillIDs, mediaIDs := updated.Category.ID, referenceIDs(updated.Skills), mediaIDs(updated.Media)
	if err := s.resolveReferences(actorID, &updated, categoryID, skillIDs, mediaIDs); err != nil {
		return Item{}, err
	}
	if slugExists(s.Items, actorID, updated.Slug, serviceID) {
		return Item{}, ErrConflict
	}
	s.Items[serviceID] = updated
	return updated, nil
}

func (s *Store) Delete(_ context.Context, actorID, serviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.Items[serviceID]
	if !ok || item.SellerID != actorID || item.DeletedAt != nil {
		return ErrNotFound
	}
	now := s.now()
	item.Status, item.DeletedAt, item.UpdatedAt = "ARCHIVED", &now, now
	s.Items[serviceID] = item
	return nil
}

func (s *Store) Transition(_ context.Context, actorID, serviceID, action string) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.Items[serviceID]
	if !ok || item.SellerID != actorID || item.DeletedAt != nil {
		return Item{}, ErrNotFound
	}
	target, allowed := transition(item.Status, action)
	if !allowed {
		return Item{}, ErrInvalidState
	}
	if (action == "publish" || action == "resume") && s.Publishable != nil && !s.Publishable[actorID] {
		return Item{}, ErrSellerIneligible
	}
	if action == "publish" || action == "resume" {
		if err := s.resolveReferences(actorID, &item, item.Category.ID, referenceIDs(item.Skills), mediaIDs(item.Media)); err != nil {
			return Item{}, err
		}
	}
	now := s.now()
	item.Status, item.UpdatedAt = target, now
	if action == "publish" && item.PublishedAt == nil {
		item.PublishedAt = &now
	}
	s.Items[serviceID] = item
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
		_, categoryActive := s.Categories[item.Category.ID]
		sellerPublic := s.Publishable == nil || s.Publishable[item.SellerID]
		if item.Status != "ACTIVE" || item.Visibility != "PUBLIC" || item.DeletedAt != nil || item.PublishedAt == nil ||
			(item.ModerationStatus != "" && item.ModerationStatus != "VISIBLE") ||
			!categoryActive || !sellerPublic || !after(*item.PublishedAt, item.ID, cursor) || !matches(item, filter) {
			continue
		}
		public := s.publicItem(item)
		items = append(items, public)
	}
	return paginate(items, limit, func(item Item) time.Time { return *item.PublishedAt }), nil
}

func (s *Store) GetPublic(_ context.Context, reference string) (Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if validUUID(reference) {
		item, ok := s.Items[normalizeID(reference)]
		_, categoryActive := s.Categories[item.Category.ID]
		sellerPublic := s.Publishable == nil || s.Publishable[item.SellerID]
		if !ok || !categoryActive || !sellerPublic || item.Status != "ACTIVE" || (item.ModerationStatus != "" && item.ModerationStatus != "VISIBLE") || item.Visibility != "PUBLIC" || item.DeletedAt != nil {
			return Item{}, ErrNotFound
		}
		return s.publicItem(item), nil
	}
	var found *Item
	for _, item := range s.Items {
		_, categoryActive := s.Categories[item.Category.ID]
		sellerPublic := s.Publishable == nil || s.Publishable[item.SellerID]
		if item.Status != "ACTIVE" || (item.ModerationStatus != "" && item.ModerationStatus != "VISIBLE") || item.Visibility != "PUBLIC" || item.DeletedAt != nil || item.Slug != reference {
			continue
		}
		if !categoryActive || !sellerPublic {
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

func (s *Store) publicItem(item Item) Item {
	skills := make([]Reference, 0, len(item.Skills))
	for _, skill := range item.Skills {
		if current, ok := s.Skills[skill.ID]; ok {
			skills = append(skills, current)
		}
	}
	item.Skills = skills
	media := make([]Media, 0, len(item.Media))
	for _, attached := range item.Media {
		current, ok := s.Media[attached.ID]
		if !ok || current.Deleted || current.Purpose != "SERVICE" || !current.Uploaded || current.ScanStatus != "CLEAN" {
			continue
		}
		media = append(media, attached)
	}
	item.Media = media
	return item
}

func Validate(item Item) error {
	if len([]rune(strings.TrimSpace(item.Title))) < 1 || len([]rune(item.Title)) > 180 || !slugPattern.MatchString(item.Slug) || len(item.Slug) > 220 {
		return invalid("invalid title or slug")
	}
	if len([]rune(item.ShortDescription)) > 320 || len([]rune(strings.TrimSpace(item.Description))) < 1 || len([]rune(item.Description)) > 10000 {
		return invalid("invalid service description")
	}
	if !oneOf(item.ServiceType, "PROFESSIONAL_SERVICE", "CONSULTATION", "EDUCATION", "MENTORING") || !oneOf(item.PriceType, "FIXED", "FROM", "HOURLY", "NEGOTIABLE") {
		return invalid("invalid service or price type")
	}
	if item.PriceType == "NEGOTIABLE" {
		if item.PriceFrom != nil {
			return invalid("negotiable service cannot set a price")
		}
	} else if item.PriceFrom == nil || item.PriceFrom.AmountKopecks <= 0 || item.PriceFrom.Currency != "RUB" {
		return invalid("a positive RUB price is required")
	}
	if item.DeliveryDays != nil && (*item.DeliveryDays < 1 || *item.DeliveryDays > 365) ||
		item.IncludedRevisions != nil && (*item.IncludedRevisions < 0 || *item.IncludedRevisions > 100) {
		return invalid("invalid delivery terms")
	}
	if item.Visibility != "PUBLIC" && item.Visibility != "PRIVATE" {
		return invalid("invalid visibility")
	}
	if item.ServiceType == "PROFESSIONAL_SERVICE" && item.Education != nil {
		return invalid("education details are not allowed for this service type")
	}
	if (item.ServiceType == "EDUCATION" || item.ServiceType == "MENTORING") && item.Education == nil {
		return invalid("education details are required")
	}
	if item.Education != nil {
		if err := validateEducation(*item.Education); err != nil {
			return err
		}
	}
	if len(item.Skills) > 30 || len(item.Media) > 12 {
		return invalid("too many service references")
	}
	return nil
}

func (s *Store) Moderate(_ context.Context, actorID, serviceID, action, reason string) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Admins[actorID] {
		return Item{}, ErrUnauthorized
	}
	item, ok := s.Items[serviceID]
	action, reason = strings.ToUpper(strings.TrimSpace(action)), strings.TrimSpace(reason)
	if !ok || item.DeletedAt != nil {
		return Item{}, ErrNotFound
	}
	if !oneOf(action, "HIDE", "RESTORE") || len([]rune(reason)) < 3 || len([]rune(reason)) > 1000 {
		return Item{}, ErrInvalidInput
	}
	if action == "HIDE" {
		item.ModerationStatus = "HIDDEN"
	} else {
		item.ModerationStatus = "VISIBLE"
	}
	item.UpdatedAt = s.now()
	s.Items[serviceID] = item
	return item, nil
}

func ValidateFilter(filter Filter) error {
	if len([]rune(filter.Q)) > 120 || len(filter.Category) > 120 || (filter.Category != "" && !validUUID(filter.Category) && !slugPattern.MatchString(filter.Category)) ||
		(filter.ServiceType != "" && !oneOf(filter.ServiceType, "PROFESSIONAL_SERVICE", "CONSULTATION", "EDUCATION", "MENTORING")) ||
		(filter.PriceType != "" && !oneOf(filter.PriceType, "FIXED", "FROM", "HOURLY", "NEGOTIABLE")) ||
		(filter.Format != "" && !oneOf(filter.Format, "ONLINE", "OFFLINE", "ASYNC", "HYBRID")) ||
		(filter.Audience != "" && !oneOf(filter.Audience, "INDIVIDUAL", "GROUP", "BOTH")) ||
		(filter.MaxDurationMinutes != nil && (*filter.MaxDurationMinutes < 1 || *filter.MaxDurationMinutes > 10080)) {
		return invalid("invalid service filter")
	}
	return nil
}

func validateEducation(details EducationDetails) error {
	if !oneOf(details.Format, "ONLINE", "OFFLINE", "ASYNC", "HYBRID") ||
		details.DurationMinutes != nil && (*details.DurationMinutes < 15 || *details.DurationMinutes > 10080) ||
		details.SessionsCount != nil && (*details.SessionsCount < 1 || *details.SessionsCount > 365) ||
		(details.AudienceType != "" && !oneOf(details.AudienceType, "INDIVIDUAL", "GROUP", "BOTH")) ||
		details.GroupSizeMax != nil && (*details.GroupSizeMax < 2 || *details.GroupSizeMax > 1000) {
		return invalid("invalid education details")
	}
	if details.GroupSizeMax != nil && details.AudienceType != "GROUP" && details.AudienceType != "BOTH" {
		return invalid("group size requires a group audience")
	}
	return nil
}

func itemFromCreate(actorID, id string, input CreateRequest, now time.Time) Item {
	return Item{ID: id, SellerID: actorID, Category: Reference{ID: normalizeID(input.CategoryID)}, ServiceType: strings.ToUpper(strings.TrimSpace(input.ServiceType)),
		Title: strings.TrimSpace(input.Title), Slug: strings.ToLower(strings.TrimSpace(input.Slug)), ShortDescription: strings.TrimSpace(input.ShortDescription),
		Description: strings.TrimSpace(input.Description), PriceType: strings.ToUpper(strings.TrimSpace(input.PriceType)), PriceFrom: normalizeMoney(input.PriceFrom),
		DeliveryDays: input.DeliveryDays, IncludedRevisions: input.IncludedRevisions, Status: "DRAFT", ModerationStatus: "VISIBLE", Visibility: strings.ToUpper(strings.TrimSpace(input.Visibility)),
		Skills: references(input.SkillIDs), Media: mediaReferences(input.MediaObjectIDs), Education: normalizeEducation(input.Education), CreatedAt: now, UpdatedAt: now}
}

func mergePatch(item Item, patch PatchRequest, now time.Time) Item {
	if patch.CategoryID != nil {
		item.Category.ID = normalizeID(*patch.CategoryID)
	}
	if patch.ServiceType != nil {
		item.ServiceType = strings.ToUpper(strings.TrimSpace(*patch.ServiceType))
	}
	if patch.Title != nil {
		item.Title = strings.TrimSpace(*patch.Title)
	}
	if patch.Slug != nil {
		item.Slug = strings.ToLower(strings.TrimSpace(*patch.Slug))
	}
	if patch.ShortDescription != nil {
		item.ShortDescription = strings.TrimSpace(*patch.ShortDescription)
	}
	if patch.Description != nil {
		item.Description = strings.TrimSpace(*patch.Description)
	}
	if patch.PriceType != nil {
		item.PriceType = strings.ToUpper(strings.TrimSpace(*patch.PriceType))
		if item.PriceType == "NEGOTIABLE" {
			item.PriceFrom = nil
		}
	}
	if patch.PriceFrom != nil {
		item.PriceFrom = normalizeMoney(patch.PriceFrom)
	}
	if patch.DeliveryDays != nil {
		item.DeliveryDays = patch.DeliveryDays
	}
	if patch.IncludedRevisions != nil {
		item.IncludedRevisions = patch.IncludedRevisions
	}
	if patch.Visibility != nil {
		item.Visibility = strings.ToUpper(strings.TrimSpace(*patch.Visibility))
	}
	if patch.SkillIDs != nil {
		item.Skills = references(*patch.SkillIDs)
	}
	if patch.MediaObjectIDs != nil {
		item.Media = mediaReferences(*patch.MediaObjectIDs)
	}
	if patch.Education != nil {
		item.Education = normalizeEducation(patch.Education)
	}
	if item.ServiceType == "PROFESSIONAL_SERVICE" {
		item.Education = nil
	}
	item.UpdatedAt = now
	return item
}

func (s *Store) resolveReferences(actorID string, item *Item, categoryID string, skillIDs []string, objectIDs []string) error {
	category, ok := s.Categories[normalizeID(categoryID)]
	if !ok || !validUUID(category.ID) {
		return fmt.Errorf("%w: category is not active", ErrInvalidReference)
	}
	item.Category = category
	seen := map[string]struct{}{}
	item.Skills = make([]Reference, 0, len(skillIDs))
	for _, id := range skillIDs {
		id = normalizeID(id)
		if !validUUID(id) {
			return invalid("invalid skill id")
		}
		if _, duplicate := seen[id]; duplicate {
			return invalid("duplicate skill id")
		}
		seen[id] = struct{}{}
		reference, exists := s.Skills[id]
		if !exists {
			return fmt.Errorf("%w: skill is not active", ErrInvalidReference)
		}
		item.Skills = append(item.Skills, reference)
	}
	seen = map[string]struct{}{}
	item.Media = make([]Media, 0, len(objectIDs))
	for index, id := range objectIDs {
		id = normalizeID(id)
		if !validUUID(id) {
			return invalid("invalid media id")
		}
		if _, duplicate := seen[id]; duplicate {
			return invalid("duplicate media id")
		}
		seen[id] = struct{}{}
		object, exists := s.Media[id]
		if !exists || object.OwnerID != actorID || object.Purpose != "SERVICE" || !object.Uploaded || object.ScanStatus != "CLEAN" || object.Deleted {
			return fmt.Errorf("%w: media is not attachable", ErrInvalidReference)
		}
		item.Media = append(item.Media, Media{ID: id, MIMEType: object.MIMEType, SizeBytes: object.SizeBytes, SortOrder: index})
	}
	return Validate(*item)
}

func transition(status, action string) (string, bool) {
	switch action {
	case "publish":
		return "ACTIVE", status == "DRAFT" || status == "REJECTED"
	case "pause":
		return "PAUSED", status == "ACTIVE"
	case "resume":
		return "ACTIVE", status == "PAUSED"
	default:
		return "", false
	}
}

func matches(item Item, filter Filter) bool {
	q := strings.ToLower(filter.Q)
	if q != "" && !strings.Contains(strings.ToLower(item.Title+" "+item.ShortDescription+" "+item.Description), q) {
		return false
	}
	if filter.Category != "" && item.Category.ID != filter.Category && item.Category.Slug != filter.Category {
		return false
	}
	if filter.ServiceType != "" && item.ServiceType != filter.ServiceType || filter.PriceType != "" && item.PriceType != filter.PriceType {
		return false
	}
	if filter.Format != "" && (item.Education == nil || item.Education.Format != filter.Format) {
		return false
	}
	if filter.Audience != "" && (item.Education == nil || item.Education.AudienceType != filter.Audience && item.Education.AudienceType != "BOTH") {
		return false
	}
	if filter.MaxDurationMinutes != nil && item.Education != nil && item.Education.DurationMinutes != nil && *item.Education.DurationMinutes > *filter.MaxDurationMinutes {
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

func references(ids []string) []Reference {
	values := make([]Reference, len(ids))
	for index, id := range ids {
		values[index] = Reference{ID: normalizeID(id)}
	}
	return values
}
func mediaReferences(ids []string) []Media {
	values := make([]Media, len(ids))
	for index, id := range ids {
		values[index] = Media{ID: normalizeID(id), SortOrder: index}
	}
	return values
}
func referenceIDs(values []Reference) []string {
	ids := make([]string, len(values))
	for i := range values {
		ids[i] = values[i].ID
	}
	return ids
}
func mediaIDs(values []Media) []string {
	ids := make([]string, len(values))
	for i := range values {
		ids[i] = values[i].ID
	}
	return ids
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
func normalizeMoney(value *Money) *Money {
	if value == nil {
		return nil
	}
	copy := *value
	copy.Currency = strings.ToUpper(strings.TrimSpace(copy.Currency))
	return &copy
}
func normalizeEducation(value *EducationDetails) *EducationDetails {
	if value == nil {
		return nil
	}
	copy := *value
	copy.Format = strings.ToUpper(strings.TrimSpace(copy.Format))
	copy.AudienceType = strings.ToUpper(strings.TrimSpace(copy.AudienceType))
	return &copy
}
func slugExists(items map[string]Item, actorID, slug, exceptID string) bool {
	for _, item := range items {
		if item.SellerID == actorID && item.Slug == slug && item.ID != exceptID && item.DeletedAt == nil {
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
