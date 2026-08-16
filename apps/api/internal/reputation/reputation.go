package reputation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrUnauthorized = errors.New("authentication required")
	ErrNotFound     = errors.New("external reputation not found")
	ErrConflict     = errors.New("external reputation already exists")
	ErrInvalid      = errors.New("invalid external reputation")
	ErrForbidden    = errors.New("moderator role required")
	ErrInvalidState = errors.New("invalid external reputation state")
)

const StatusUnverified = "UNVERIFIED"

type Item struct {
	ID                   string     `json:"id"`
	UserID               string     `json:"-"`
	Platform             string     `json:"platform"`
	DisplayName          string     `json:"display_name"`
	ProfileURL           string     `json:"profile_url"`
	ExternalUsername     string     `json:"external_username,omitempty"`
	Rating               *float64   `json:"rating,omitempty"`
	ReviewsCount         *int       `json:"reviews_count,omitempty"`
	CompletedOrdersCount *int       `json:"completed_orders_count,omitempty"`
	AccountSince         *string    `json:"account_since,omitempty"`
	VerificationStatus   string     `json:"verification_status"`
	VerificationMethod   string     `json:"verification_method,omitempty"`
	VerifiedAt           *time.Time `json:"verified_at,omitempty"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty"`
	LastCheckedAt        *time.Time `json:"last_checked_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type PublicItem struct {
	Platform             string     `json:"platform"`
	DisplayName          string     `json:"display_name"`
	ProfileURL           string     `json:"profile_url"`
	Verified             bool       `json:"verified"`
	Rating               *float64   `json:"rating,omitempty"`
	ReviewsCount         *int       `json:"reviews_count,omitempty"`
	CompletedOrdersCount *int       `json:"completed_orders_count,omitempty"`
	AccountSince         *string    `json:"account_since,omitempty"`
	VerifiedAt           *time.Time `json:"verified_at,omitempty"`
}

type CreateRequest struct {
	Platform         string `json:"platform"`
	ProfileURL       string `json:"profile_url"`
	ExternalUsername string `json:"external_username"`
}

type PatchRequest struct {
	Platform         *string `json:"platform"`
	ProfileURL       *string `json:"profile_url"`
	ExternalUsername *string `json:"external_username"`
}

type Repository interface {
	ListOwned(context.Context, string) ([]Item, error)
	Create(context.Context, string, CreateRequest) (Item, error)
	Update(context.Context, string, string, PatchRequest) (Item, error)
	Delete(context.Context, string, string) error
	ListPublic(context.Context, string) ([]PublicItem, error)
	StartVerification(context.Context, string, string, StartVerificationRequest, []byte, time.Time) (Challenge, error)
	GetVerification(context.Context, string, string, time.Time) (Challenge, error)
	ListPending(context.Context, string) ([]ModerationItem, error)
	Decide(context.Context, string, string, DecisionRequest, time.Time) (Item, error)
}

type StartVerificationRequest struct {
	Method   string         `json:"method"`
	Evidence map[string]any `json:"evidence,omitempty"`
}

type Challenge struct {
	ID                   string     `json:"id"`
	ExternalReputationID string     `json:"external_reputation_id"`
	Method               string     `json:"method"`
	Code                 string     `json:"code,omitempty"`
	ExpiresAt            time.Time  `json:"expires_at"`
	Attempts             int        `json:"attempts"`
	Status               string     `json:"status"`
	CreatedAt            time.Time  `json:"created_at"`
	VerifiedAt           *time.Time `json:"verified_at,omitempty"`
}

type ModerationItem struct {
	Item
	Evidence  map[string]any `json:"evidence"`
	Challenge *Challenge     `json:"challenge,omitempty"`
}

type DecisionRequest struct {
	ReasonCode string `json:"reason_code,omitempty"`
	Note       string `json:"note,omitempty"`
}

type Service struct{ Repository Repository }

func (s Service) StartVerification(ctx context.Context, actor, id string, input StartVerificationRequest) (Challenge, error) {
	if actor == "" {
		return Challenge{}, ErrUnauthorized
	}
	if !validUUID(id) || input.Method != "PROFILE_CODE" && input.Method != "MANUAL" || !validEvidence(input.Evidence) {
		return Challenge{}, ErrInvalid
	}
	if input.Evidence == nil {
		input.Evidence = map[string]any{}
	}
	var code string
	var hash []byte
	if input.Method == "PROFILE_CODE" {
		code = verificationCode()
		sum := sha256.Sum256([]byte(code))
		hash = sum[:]
	}
	now := time.Now().UTC()
	challenge, err := s.Repository.StartVerification(ctx, actor, strings.ToLower(id), input, hash, now)
	if err == nil {
		challenge.Code = code
	}
	return challenge, err
}

func (s Service) GetVerification(ctx context.Context, actor, id string) (Challenge, error) {
	if actor == "" {
		return Challenge{}, ErrUnauthorized
	}
	if !validUUID(id) {
		return Challenge{}, ErrInvalid
	}
	return s.Repository.GetVerification(ctx, actor, strings.ToLower(id), time.Now().UTC())
}

func (s Service) ListPending(ctx context.Context, actor string) ([]ModerationItem, error) {
	if actor == "" {
		return nil, ErrUnauthorized
	}
	return s.Repository.ListPending(ctx, actor)
}

func (s Service) Decide(ctx context.Context, actor, id, action string, input DecisionRequest) (Item, error) {
	if actor == "" {
		return Item{}, ErrUnauthorized
	}
	if !validUUID(id) || action != "verify" && action != "reject" || len([]rune(input.Note)) > 2000 {
		return Item{}, ErrInvalid
	}
	if action == "reject" && !validReason(input.ReasonCode) {
		return Item{}, ErrInvalid
	}
	if action == "verify" {
		input.ReasonCode = ""
	}
	return s.Repository.Decide(ctx, actor, strings.ToLower(id), inputWithAction(input, action), time.Now().UTC())
}

func (s Service) ListOwned(ctx context.Context, actor string) ([]Item, error) {
	if actor == "" {
		return nil, ErrUnauthorized
	}
	items, err := s.Repository.ListOwned(ctx, actor)
	decorate(items)
	return items, err
}

func (s Service) Create(ctx context.Context, actor string, input CreateRequest) (Item, error) {
	if actor == "" {
		return Item{}, ErrUnauthorized
	}
	normalized, err := normalizeCreate(input)
	if err != nil {
		return Item{}, err
	}
	item, err := s.Repository.Create(ctx, actor, normalized)
	item.DisplayName = platformName(item.Platform)
	return item, err
}

func (s Service) Update(ctx context.Context, actor, id string, patch PatchRequest) (Item, error) {
	if actor == "" {
		return Item{}, ErrUnauthorized
	}
	if !validUUID(id) || patch.Platform == nil && patch.ProfileURL == nil && patch.ExternalUsername == nil {
		return Item{}, ErrInvalid
	}
	items, err := s.Repository.ListOwned(ctx, actor)
	if err != nil {
		return Item{}, err
	}
	var current *Item
	for i := range items {
		if items[i].ID == strings.ToLower(id) {
			current = &items[i]
			break
		}
	}
	if current == nil || current.VerificationStatus != StatusUnverified {
		return Item{}, ErrNotFound
	}
	effective := CreateRequest{Platform: current.Platform, ProfileURL: current.ProfileURL, ExternalUsername: current.ExternalUsername}
	if patch.Platform != nil {
		effective.Platform = *patch.Platform
	}
	if patch.ProfileURL != nil {
		effective.ProfileURL = *patch.ProfileURL
	}
	if patch.ExternalUsername != nil {
		effective.ExternalUsername = *patch.ExternalUsername
	}
	normalized, err := normalizeCreate(effective)
	if err != nil {
		return Item{}, err
	}
	platform, profileURL, externalUsername := normalized.Platform, normalized.ProfileURL, normalized.ExternalUsername
	item, err := s.Repository.Update(ctx, actor, strings.ToLower(id), PatchRequest{Platform: &platform, ProfileURL: &profileURL, ExternalUsername: &externalUsername})
	item.DisplayName = platformName(item.Platform)
	return item, err
}

func (s Service) Delete(ctx context.Context, actor, id string) error {
	if actor == "" {
		return ErrUnauthorized
	}
	if !validUUID(id) {
		return ErrInvalid
	}
	return s.Repository.Delete(ctx, actor, strings.ToLower(id))
}

func (s Service) ListPublic(ctx context.Context, username string) ([]PublicItem, error) {
	username = strings.TrimSpace(username)
	if username == "" || len(username) > 40 {
		return nil, ErrNotFound
	}
	items, err := s.Repository.ListPublic(ctx, username)
	for i := range items {
		items[i].DisplayName = platformName(items[i].Platform)
		items[i].Verified = true
	}
	return items, err
}

var platforms = map[string]struct {
	Name  string
	Hosts []string
}{
	"KWORK":       {Name: "Kwork", Hosts: []string{"kwork.ru"}},
	"FL_RU":       {Name: "FL.ru", Hosts: []string{"fl.ru"}},
	"GITHUB":      {Name: "GitHub", Hosts: []string{"github.com"}},
	"BEHANCE":     {Name: "Behance", Hosts: []string{"behance.net"}},
	"DRIBBBLE":    {Name: "Dribbble", Hosts: []string{"dribbble.com"}},
	"HABR_CAREER": {Name: "Хабр Карьера", Hosts: []string{"career.habr.com"}},
	"OTHER":       {Name: "Другая платформа"},
}

func normalizeCreate(input CreateRequest) (CreateRequest, error) {
	input.Platform = strings.ToUpper(strings.TrimSpace(input.Platform))
	input.ExternalUsername = strings.TrimSpace(input.ExternalUsername)
	if _, ok := platforms[input.Platform]; !ok || len([]rune(input.ExternalUsername)) > 160 {
		return CreateRequest{}, ErrInvalid
	}
	normalizedURL, err := normalizeURL(input.Platform, input.ProfileURL)
	if err != nil {
		return CreateRequest{}, err
	}
	input.ProfileURL = normalizedURL
	return input, nil
}

func normalizeURL(platform, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2048 {
		return "", ErrInvalid
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || !parsed.IsAbs() || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Port() != "" {
		return "", ErrInvalid
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "" || host == "localhost" || net.ParseIP(host) != nil {
		return "", ErrInvalid
	}
	definition := platforms[platform]
	if len(definition.Hosts) > 0 {
		valid := false
		for _, allowed := range definition.Hosts {
			if host == allowed || host == "www."+allowed {
				valid = true
				break
			}
		}
		if !valid || parsed.EscapedPath() == "" || parsed.EscapedPath() == "/" {
			return "", ErrInvalid
		}
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = host
	parsed.Fragment = ""
	if parsed.Path != "/" {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	}
	return parsed.String(), nil
}

func platformName(code string) string {
	if value, ok := platforms[code]; ok {
		return value.Name
	}
	return code
}

func validEvidence(value map[string]any) bool {
	if len(value) > 10 {
		return false
	}
	encoded, err := json.Marshal(value)
	return err == nil && len(encoded) <= 8192
}

func verificationCode() string {
	var value [5]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic("secure random source unavailable")
	}
	return "VERIFY-FR-" + strings.ToUpper(hex.EncodeToString(value[:]))
}

func validReason(value string) bool {
	switch value {
	case "PROFILE_NOT_ACCESSIBLE", "OWNERSHIP_NOT_PROVEN", "METRICS_NOT_VERIFIABLE", "PROFILE_MISMATCH", "INVALID_EVIDENCE", "OTHER":
		return true
	}
	return false
}

func inputWithAction(input DecisionRequest, action string) DecisionRequest {
	input.Note = strings.TrimSpace(input.Note)
	input.ReasonCode = action + ":" + input.ReasonCode
	return input
}

func decorate(items []Item) {
	for i := range items {
		items[i].DisplayName = platformName(items[i].Platform)
	}
}

func validUUID(value string) bool {
	value = strings.ToLower(value)
	if len(value) != 36 {
		return false
	}
	var parts = []int{8, 13, 18, 23}
	for _, index := range parts {
		if value[index] != '-' {
			return false
		}
	}
	for index, char := range value {
		if value[index] == '-' {
			continue
		}
		if char < '0' || char > '9' && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

type Store struct {
	mu          sync.Mutex
	Items       map[string]Item
	PublicUsers map[string]string
	Now         func() time.Time
	Challenges  map[string]Challenge
	Evidence    map[string]map[string]any
	Admins      map[string]bool
	Audits      []string
}

func (s *Store) StartVerification(_ context.Context, actor, id string, input StartVerificationRequest, _ []byte, now time.Time) (Challenge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.Items[id]
	if !ok || item.UserID != actor {
		return Challenge{}, ErrNotFound
	}
	if item.VerificationStatus != StatusUnverified && item.VerificationStatus != "REJECTED" && item.VerificationStatus != "EXPIRED" {
		return Challenge{}, ErrInvalidState
	}
	for _, challenge := range s.Challenges {
		if challenge.ExternalReputationID == id && challenge.Status == "PENDING" && challenge.ExpiresAt.After(now) {
			return Challenge{}, ErrConflict
		}
	}
	challenge := Challenge{ID: newUUID(), ExternalReputationID: id, Method: input.Method, ExpiresAt: now.Add(24 * time.Hour), Status: "PENDING", CreatedAt: now}
	if s.Challenges == nil {
		s.Challenges = map[string]Challenge{}
	}
	if s.Evidence == nil {
		s.Evidence = map[string]map[string]any{}
	}
	s.Challenges[challenge.ID], s.Evidence[id] = challenge, input.Evidence
	item.VerificationStatus, item.VerificationMethod, item.UpdatedAt = "PENDING", input.Method, now
	s.Items[id] = item
	return challenge, nil
}

func (s *Store) GetVerification(_ context.Context, actor, id string, now time.Time) (Challenge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.Items[id]
	if !ok || item.UserID != actor {
		return Challenge{}, ErrNotFound
	}
	for key, challenge := range s.Challenges {
		if challenge.ExternalReputationID == id && challenge.Status == "PENDING" {
			if !challenge.ExpiresAt.After(now) {
				challenge.Status = "EXPIRED"
				s.Challenges[key] = challenge
				item.VerificationStatus = "EXPIRED"
				s.Items[id] = item
			}
			return challenge, nil
		}
	}
	return Challenge{}, ErrNotFound
}

func (s *Store) ListPending(_ context.Context, actor string) ([]ModerationItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Admins[actor] {
		return nil, ErrForbidden
	}
	now := s.now()
	for key, challenge := range s.Challenges {
		if challenge.Status == "PENDING" && !challenge.ExpiresAt.After(now) {
			challenge.Status = "EXPIRED"
			s.Challenges[key] = challenge
			item := s.Items[challenge.ExternalReputationID]
			item.VerificationStatus, item.UpdatedAt = "EXPIRED", now
			s.Items[item.ID] = item
		}
	}
	items := []ModerationItem{}
	for _, item := range s.Items {
		if item.VerificationStatus == "PENDING" {
			value := ModerationItem{Item: item, Evidence: s.Evidence[item.ID]}
			for _, c := range s.Challenges {
				if c.ExternalReputationID == item.ID && c.Status == "PENDING" {
					cc := c
					value.Challenge = &cc
				}
			}
			items = append(items, value)
		}
	}
	return items, nil
}

func (s *Store) Decide(_ context.Context, actor, id string, input DecisionRequest, now time.Time) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Admins[actor] {
		return Item{}, ErrForbidden
	}
	item, ok := s.Items[id]
	if !ok || item.VerificationStatus != "PENDING" {
		return Item{}, ErrNotFound
	}
	action := strings.SplitN(input.ReasonCode, ":", 2)[0]
	if action == "verify" {
		for _, candidate := range s.Items {
			if candidate.ID != id && candidate.Platform == item.Platform && candidate.ProfileURL == item.ProfileURL && candidate.VerificationStatus == "VERIFIED" {
				return Item{}, ErrConflict
			}
		}
		item.VerificationStatus = "VERIFIED"
		item.VerifiedAt = &now
	} else {
		item.VerificationStatus = "REJECTED"
	}
	item.UpdatedAt = now
	s.Items[id] = item
	for key, c := range s.Challenges {
		if c.ExternalReputationID == id && c.Status == "PENDING" {
			if action == "verify" {
				c.Status = "VERIFIED"
				c.VerifiedAt = &now
			} else {
				c.Status = "REJECTED"
			}
			s.Challenges[key] = c
		}
	}
	s.Audits = append(s.Audits, actor+":"+action+":"+id)
	return item, nil
}

func (s *Store) ListOwned(_ context.Context, actor string) ([]Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]Item, 0)
	for _, item := range s.Items {
		if item.UserID == actor {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}

func (s *Store) Create(_ context.Context, actor string, input CreateRequest) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.Items {
		if item.UserID == actor && item.Platform == input.Platform && item.ProfileURL == input.ProfileURL {
			return Item{}, ErrConflict
		}
	}
	now := s.now()
	item := Item{ID: newUUID(), UserID: actor, Platform: input.Platform, ProfileURL: input.ProfileURL, ExternalUsername: input.ExternalUsername, VerificationStatus: StatusUnverified, CreatedAt: now, UpdatedAt: now}
	if s.Items == nil {
		s.Items = map[string]Item{}
	}
	s.Items[item.ID] = item
	return item, nil
}

func (s *Store) Update(_ context.Context, actor, id string, patch PatchRequest) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.Items[id]
	if !ok || item.UserID != actor || item.VerificationStatus != StatusUnverified {
		return Item{}, ErrNotFound
	}
	for _, candidate := range s.Items {
		if candidate.ID != id && candidate.UserID == actor && candidate.Platform == *patch.Platform && candidate.ProfileURL == *patch.ProfileURL {
			return Item{}, ErrConflict
		}
	}
	item.Platform, item.ProfileURL, item.ExternalUsername = *patch.Platform, *patch.ProfileURL, *patch.ExternalUsername
	item.UpdatedAt = s.now()
	s.Items[id] = item
	return item, nil
}

func (s *Store) Delete(_ context.Context, actor, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.Items[id]
	if !ok || item.UserID != actor {
		return ErrNotFound
	}
	delete(s.Items, id)
	return nil
}

func (s *Store) ListPublic(_ context.Context, username string) ([]PublicItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	userID, ok := s.PublicUsers[strings.ToLower(username)]
	if !ok {
		return nil, ErrNotFound
	}
	items := make([]PublicItem, 0)
	for _, item := range s.Items {
		if item.UserID == userID && item.VerificationStatus == "VERIFIED" {
			items = append(items, PublicItem{Platform: item.Platform, ProfileURL: item.ProfileURL, Verified: true, Rating: item.Rating, ReviewsCount: item.ReviewsCount, CompletedOrdersCount: item.CompletedOrdersCount, AccountSince: item.AccountSince, VerifiedAt: item.VerifiedAt})
		}
	}
	return items, nil
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func newUUID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic("secure random source unavailable")
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}
