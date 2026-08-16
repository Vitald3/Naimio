package profiles

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

var ErrUnauthorized = errors.New("unauthenticated")
var ErrNotFound = errors.New("profile not found")
var ErrInvalidInput = errors.New("invalid profile input")
var ErrInvalidReference = errors.New("invalid profile reference")
var languageCodePattern = regexp.MustCompile(`^[a-z]{2,3}(-[a-z0-9]{2,6})?$`)

// Catalog seed IDs are deterministic UUID-shaped identifiers and are not all
// RFC-versioned UUIDs. PostgreSQL still stores them as uuid, so accept the
// complete database representation instead of inspecting version bits.
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var countryCodePattern = regexp.MustCompile(`^[A-Z]{2}$`)

type Category struct {
	ID        string `json:"id"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	IsPrimary bool   `json:"is_primary"`
}

type Skill struct {
	ID         string `json:"id"`
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	Level      string `json:"level,omitempty"`
	Years      *int   `json:"years,omitempty"`
	IsFeatured bool   `json:"is_featured"`
}

type Language struct {
	Code  string `json:"code"`
	Level string `json:"level"`
}

type CategorySelection struct {
	ID        string `json:"id"`
	IsPrimary bool   `json:"is_primary"`
}

type SkillSelection struct {
	ID         string `json:"id"`
	Level      string `json:"level,omitempty"`
	Years      *int   `json:"years,omitempty"`
	IsFeatured bool   `json:"is_featured"`
}

type Profile struct {
	UserID              string     `json:"-"`
	ID                  string     `json:"id,omitempty"`
	Username            string     `json:"username"`
	DisplayName         string     `json:"display_name"`
	ProfessionalTitle   string     `json:"professional_title,omitempty"`
	Bio                 string     `json:"bio,omitempty"`
	LocationText        string     `json:"location_text,omitempty"`
	CountryCode         string     `json:"country_code,omitempty"`
	ExperienceYears     *int       `json:"experience_years,omitempty"`
	HourlyRateKopecks   *int64     `json:"hourly_rate_kopecks,omitempty"`
	MinimumOrderKopecks *int64     `json:"minimum_order_kopecks,omitempty"`
	Availability        string     `json:"availability"`
	ProfileVisibility   string     `json:"profile_visibility"`
	ResponseTimeMinutes *int       `json:"response_time_minutes,omitempty"`
	NativeRating        *float64   `json:"native_rating,omitempty"`
	ReviewsCount        int        `json:"reviews_count"`
	CompletedProjects   int        `json:"completed_projects_count"`
	EffectivePro        bool       `json:"effective_pro"`
	Categories          []Category `json:"categories"`
	Skills              []Skill    `json:"skills"`
	CustomSkills        []string   `json:"custom_skills"`
	Languages           []Language `json:"languages"`
}

type UpdateRequest struct {
	ProfessionalTitle   string              `json:"professional_title"`
	Bio                 string              `json:"bio"`
	LocationText        string              `json:"location_text"`
	CountryCode         string              `json:"country_code"`
	ExperienceYears     *int                `json:"experience_years"`
	HourlyRateKopecks   *int64              `json:"hourly_rate_kopecks"`
	MinimumOrderKopecks *int64              `json:"minimum_order_kopecks"`
	Availability        string              `json:"availability"`
	ProfileVisibility   string              `json:"profile_visibility"`
	Categories          []CategorySelection `json:"categories"`
	Skills              []SkillSelection    `json:"skills"`
	CustomSkills        []string            `json:"custom_skills"`
	Languages           []Language          `json:"languages"`
}

type Store struct {
	mu    sync.RWMutex
	Items map[string]Profile
}
type PublicCursor struct {
	Score    float64 `json:"score"`
	Username string  `json:"username"`
	ID       string  `json:"id"`
}
type PublicPage struct {
	Items      []Profile
	NextCursor *PublicCursor
}

type Repository interface {
	Public(context.Context, string) (Profile, error)
	PublicList(context.Context, string, *PublicCursor, int) (PublicPage, error)
	Current(context.Context, string) (Profile, error)
	Update(context.Context, string, UpdateRequest) (Profile, error)
	ReplaceCategories(context.Context, string, []CategorySelection) (Profile, error)
	ReplaceSkills(context.Context, string, []SkillSelection) (Profile, error)
	ReplaceLanguages(context.Context, string, []Language) (Profile, error)
}

func Validate(p Profile) error {
	if len([]rune(p.Bio)) > 5000 || len([]rune(p.ProfessionalTitle)) > 160 || len([]rune(p.LocationText)) > 160 {
		return invalid("profile field is too long")
	}
	switch p.Availability {
	case "AVAILABLE", "PARTIALLY_BUSY", "BUSY", "UNAVAILABLE":
	default:
		return invalid("invalid availability")
	}
	if p.ProfileVisibility != "PUBLIC" && p.ProfileVisibility != "PRIVATE" {
		return invalid("invalid visibility")
	}
	if p.CountryCode != "" && !countryCodePattern.MatchString(p.CountryCode) {
		return invalid("invalid country code")
	}
	if p.ExperienceYears != nil && (*p.ExperienceYears < 0 || *p.ExperienceYears > 80) {
		return invalid("invalid experience years")
	}
	if p.HourlyRateKopecks != nil && *p.HourlyRateKopecks < 0 {
		return invalid("invalid hourly rate")
	}
	if p.MinimumOrderKopecks != nil && *p.MinimumOrderKopecks < 0 {
		return invalid("invalid minimum order")
	}
	if len(p.Categories) > 10 || len(p.Skills)+len(p.CustomSkills) > 50 || len(p.Languages) > 20 {
		return invalid("too many profile associations")
	}
	if err := validateCategories(p.Categories); err != nil {
		return err
	}
	if err := validateSkills(p.Skills); err != nil {
		return err
	}
	if _, err := normalizeCustomSkills(p.CustomSkills); err != nil {
		return err
	}
	if err := validateLanguages(p.Languages); err != nil {
		return err
	}
	return nil
}

func normalizeCustomSkills(items []string) ([]string, error) {
	result := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, raw := range items {
		value := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
		key := strings.ToLower(value)
		if len([]rune(value)) < 2 || len([]rune(value)) > 80 {
			return nil, invalid("invalid custom skill")
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func validateCategories(items []Category) error {
	seen := make(map[string]struct{}, len(items))
	primaryCount := 0
	for _, item := range items {
		if !uuidPattern.MatchString(strings.ToLower(strings.TrimSpace(item.ID))) {
			return invalid("invalid category id")
		}
		if _, ok := seen[item.ID]; ok {
			return invalid("duplicate category")
		}
		seen[item.ID] = struct{}{}
		if item.IsPrimary {
			primaryCount++
		}
	}
	if primaryCount > 1 {
		return invalid("multiple primary categories")
	}
	return nil
}

func validateSkills(items []Skill) error {
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if !uuidPattern.MatchString(strings.ToLower(strings.TrimSpace(item.ID))) {
			return invalid("invalid skill id")
		}
		if _, ok := seen[item.ID]; ok {
			return invalid("duplicate skill")
		}
		seen[item.ID] = struct{}{}
		switch item.Level {
		case "", "BEGINNER", "INTERMEDIATE", "ADVANCED", "EXPERT":
		default:
			return invalid("invalid skill level")
		}
		if item.Years != nil && (*item.Years < 0 || *item.Years > 80) {
			return invalid("invalid skill years")
		}
	}
	return nil
}

func validateLanguages(items []Language) error {
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		item.Code = strings.ToLower(strings.TrimSpace(item.Code))
		if !languageCodePattern.MatchString(item.Code) {
			return invalid("invalid language code")
		}
		if _, ok := seen[item.Code]; ok {
			return invalid("duplicate language")
		}
		seen[item.Code] = struct{}{}
		switch item.Level {
		case "BASIC", "CONVERSATIONAL", "FLUENT", "NATIVE":
		default:
			return invalid("invalid language level")
		}
	}
	return nil
}

func invalid(message string) error { return fmt.Errorf("%w: %s", ErrInvalidInput, message) }

func (s *Store) Public(_ context.Context, username string) (Profile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.Items {
		if strings.EqualFold(p.Username, username) && p.ProfileVisibility == "PUBLIC" {
			p.ID = p.UserID
			p.UserID = ""
			return p, nil
		}
	}
	return Profile{}, ErrNotFound
}

func (s *Store) PublicList(_ context.Context, query string, cursor *PublicCursor, limit int) (PublicPage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	type ranked struct {
		profile Profile
		score   float64
	}
	out := make([]ranked, 0)
	for _, p := range s.Items {
		q := strings.ToLower(strings.TrimSpace(query))
		if p.ProfileVisibility != "PUBLIC" {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(p.DisplayName+" "+p.ProfessionalTitle+" "+p.Bio), q) {
			continue
		}
		item := p
		item.ID = item.UserID
		relevance := TextRelevanceScore(query, item.DisplayName, item.ProfessionalTitle, item.Bio)
		quality := QualityScore(item.NativeRating, item.CompletedProjects)
		boost := 0.0
		if item.EffectivePro {
			boost = BoundedProBoost(true, 1.08)
		}
		score := DiscoveryScore(relevance, quality, boost)
		item.UserID = ""
		item.Bio = ""
		item.LocationText = ""
		item.MinimumOrderKopecks = nil
		item.Categories = []Category{}
		item.Languages = []Language{}
		if len(item.Skills) > 5 {
			item.Skills = append([]Skill(nil), item.Skills[:5]...)
		}
		out = append(out, ranked{profile: item, score: score})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		left, right := strings.ToLower(out[i].profile.Username), strings.ToLower(out[j].profile.Username)
		if left != right {
			return left < right
		}
		return out[i].profile.ID < out[j].profile.ID
	})
	if cursor != nil {
		kept := out[:0]
		for _, item := range out {
			username := strings.ToLower(item.profile.Username)
			if item.score < cursor.Score ||
				(item.score == cursor.Score && username > cursor.Username) ||
				(item.score == cursor.Score && username == cursor.Username && item.profile.ID > cursor.ID) {
				kept = append(kept, item)
			}
		}
		out = kept
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	items := make([]Profile, 0, len(out))
	for _, item := range out {
		items = append(items, item.profile)
	}
	page := PublicPage{Items: items}
	if len(items) > limit {
		last := out[limit-1]
		page.Items = items[:limit]
		page.NextCursor = &PublicCursor{Score: last.score, Username: strings.ToLower(last.profile.Username), ID: last.profile.ID}
	}
	return page, nil
}

func (s *Store) Current(_ context.Context, actorID string) (Profile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.Items {
		if p.UserID == actorID {
			p.ID = p.UserID
			return p, nil
		}
	}
	return Profile{}, ErrNotFound
}

func (s *Store) Update(_ context.Context, actorID string, input UpdateRequest) (Profile, error) {
	if actorID == "" {
		return Profile{}, ErrUnauthorized
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	profile, ok := s.Items[actorID]
	if !ok {
		return Profile{}, ErrNotFound
	}
	profile.ProfessionalTitle = strings.TrimSpace(input.ProfessionalTitle)
	profile.Bio = strings.TrimSpace(input.Bio)
	profile.LocationText = strings.TrimSpace(input.LocationText)
	profile.CountryCode = strings.ToUpper(strings.TrimSpace(input.CountryCode))
	profile.ExperienceYears = input.ExperienceYears
	profile.HourlyRateKopecks = input.HourlyRateKopecks
	profile.MinimumOrderKopecks = input.MinimumOrderKopecks
	profile.Availability = input.Availability
	profile.ProfileVisibility = input.ProfileVisibility
	if input.Categories != nil {
		profile.Categories = categoriesFromSelections(input.Categories)
	}
	if input.Skills != nil {
		profile.Skills = skillsFromSelections(input.Skills)
	}
	if input.CustomSkills != nil {
		profile.CustomSkills, _ = normalizeCustomSkills(input.CustomSkills)
	}
	if input.Languages != nil {
		profile.Languages = normalizeLanguages(input.Languages)
	}
	if err := Validate(profile); err != nil {
		return Profile{}, err
	}
	s.Items[actorID] = profile
	return profile, nil
}

func (s *Store) ReplaceCategories(_ context.Context, actorID string, items []CategorySelection) (Profile, error) {
	if actorID == "" {
		return Profile{}, ErrUnauthorized
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	profile, ok := s.Items[actorID]
	if !ok {
		return Profile{}, ErrNotFound
	}
	profile.Categories = categoriesFromSelections(items)
	if err := Validate(profile); err != nil {
		return Profile{}, err
	}
	s.Items[actorID] = profile
	return profile, nil
}

func (s *Store) ReplaceSkills(_ context.Context, actorID string, items []SkillSelection) (Profile, error) {
	if actorID == "" {
		return Profile{}, ErrUnauthorized
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	profile, ok := s.Items[actorID]
	if !ok {
		return Profile{}, ErrNotFound
	}
	profile.Skills = skillsFromSelections(items)
	if err := Validate(profile); err != nil {
		return Profile{}, err
	}
	s.Items[actorID] = profile
	return profile, nil
}

func (s *Store) ReplaceLanguages(_ context.Context, actorID string, items []Language) (Profile, error) {
	if actorID == "" {
		return Profile{}, ErrUnauthorized
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	profile, ok := s.Items[actorID]
	if !ok {
		return Profile{}, ErrNotFound
	}
	profile.Languages = normalizeLanguages(items)
	if err := Validate(profile); err != nil {
		return Profile{}, err
	}
	s.Items[actorID] = profile
	return profile, nil
}

func categoriesFromSelections(items []CategorySelection) []Category {
	out := make([]Category, len(items))
	for index, item := range items {
		out[index] = Category{ID: strings.ToLower(strings.TrimSpace(item.ID)), IsPrimary: item.IsPrimary}
	}
	return out
}

func skillsFromSelections(items []SkillSelection) []Skill {
	out := make([]Skill, len(items))
	for index, item := range items {
		out[index] = Skill{ID: strings.ToLower(strings.TrimSpace(item.ID)), Level: item.Level, Years: item.Years, IsFeatured: item.IsFeatured}
	}
	return out
}

func normalizeLanguages(items []Language) []Language {
	out := append([]Language(nil), items...)
	for index := range out {
		out[index].Code = strings.ToLower(strings.TrimSpace(out[index].Code))
	}
	return out
}
