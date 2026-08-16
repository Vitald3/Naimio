package acquisition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"freelance/apps/api/internal/ai"
)

var (
	ErrNotFound = errors.New("calculator not found")
	ErrInvalid  = errors.New("invalid acquisition input")
)
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var questionKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}
type Question struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Min      *int     `json:"min,omitempty"`
	Max      *int     `json:"max,omitempty"`
	Options  []Option `json:"options,omitempty"`
}
type Definition struct {
	ID        string        `json:"-"`
	Slug      string        `json:"slug"`
	Title     string        `json:"title"`
	Intro     string        `json:"intro"`
	Version   int           `json:"version"`
	Questions []Question    `json:"questions"`
	UpdatedAt time.Time     `json:"updated_at"`
	Pricing   PricingConfig `json:"-"`
}
type AdminDefinition struct {
	Slug      string        `json:"slug"`
	Title     string        `json:"title"`
	Intro     string        `json:"intro"`
	Version   int           `json:"version"`
	Enabled   bool          `json:"enabled"`
	Questions []Question    `json:"questions"`
	Pricing   PricingConfig `json:"pricing"`
}
type AdminDefinitionInput struct {
	Slug      string        `json:"slug"`
	Title     string        `json:"title"`
	Intro     string        `json:"intro"`
	Enabled   bool          `json:"enabled"`
	Questions []Question    `json:"questions"`
	Pricing   PricingConfig `json:"pricing"`
	Reason    string        `json:"reason"`
}
type PricingConfig struct {
	BaselineMin  int64                     `json:"baseline_min_kopecks"`
	BaselineMax  int64                     `json:"baseline_max_kopecks"`
	DurationMin  int                       `json:"duration_min_days"`
	DurationMax  int                       `json:"duration_max_days"`
	OptionBasis  map[string]map[string]int `json:"option_basis_points"`
	NumberBasis  map[string]int            `json:"number_basis_points"`
	BooleanBasis map[string]int            `json:"boolean_basis_points"`
	CategorySlug string                    `json:"category_slug"`
	SkillSlugs   []string                  `json:"skill_slugs"`
	Assumptions  []string                  `json:"assumptions"`
}
type Taxonomy struct {
	Category *ai.CategoryCandidate
	Skills   []ai.SkillCandidate
}
type Estimate struct {
	EstimatedMinKopecks int64                 `json:"estimated_min_kopecks"`
	EstimatedMaxKopecks int64                 `json:"estimated_max_kopecks"`
	Currency            string                `json:"currency"`
	Duration            ai.DurationRange      `json:"estimated_duration_days"`
	Category            *ai.CategoryCandidate `json:"recommended_category,omitempty"`
	Skills              []ai.SkillCandidate   `json:"recommended_skills"`
	Assumptions         []string              `json:"assumptions"`
	Confidence          string                `json:"confidence"`
	DraftToken          string                `json:"draft_token"`
}
type Attribution struct {
	AnonymousID  string `json:"anonymous_id"`
	LandingPath  string `json:"landing_path"`
	UTMSource    string `json:"utm_source"`
	UTMMedium    string `json:"utm_medium"`
	UTMCampaign  string `json:"utm_campaign"`
	UTMContent   string `json:"utm_content"`
	ReferralCode string `json:"referral_code"`
}
type Event struct {
	Attribution
	Type     string            `json:"event_type"`
	Metadata map[string]string `json:"metadata"`
	UserID   string            `json:"-"`
}
type SitemapItem struct {
	Path      string    `json:"path"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Repository interface {
	Definition(context.Context, string) (Definition, error)
	Definitions(context.Context) ([]Definition, error)
	Resolve(context.Context, string, []string) (Taxonomy, error)
	Record(context.Context, Event) error
	Sitemap(context.Context) ([]SitemapItem, error)
	AdminDefinitions(context.Context) ([]AdminDefinition, error)
	CreateAdminDefinition(context.Context, string, AdminDefinitionInput) (AdminDefinition, error)
	UpdateAdminDefinition(context.Context, string, string, AdminDefinitionInput) (AdminDefinition, error)
}
type Service struct {
	Repository Repository
	Drafts     ai.Service
}

func (s Service) Definition(ctx context.Context, slug string) (Definition, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if !slugPattern.MatchString(slug) || len(slug) > 160 {
		return Definition{}, ErrNotFound
	}
	return s.Repository.Definition(ctx, slug)
}
func (s Service) Definitions(ctx context.Context) ([]Definition, error) {
	return s.Repository.Definitions(ctx)
}
func (s Service) AdminDefinitions(ctx context.Context) ([]AdminDefinition, error) {
	return s.Repository.AdminDefinitions(ctx)
}
func (s Service) CreateAdminDefinition(ctx context.Context, actor string, in AdminDefinitionInput) (AdminDefinition, error) {
	if err := validateAdminInput(actor, &in, true); err != nil {
		return AdminDefinition{}, err
	}
	return s.Repository.CreateAdminDefinition(ctx, actor, in)
}
func (s Service) UpdateAdminDefinition(ctx context.Context, actor, slug string, in AdminDefinitionInput) (AdminDefinition, error) {
	in.Slug = slug
	if err := validateAdminInput(actor, &in, false); err != nil {
		return AdminDefinition{}, err
	}
	return s.Repository.UpdateAdminDefinition(ctx, actor, slug, in)
}
func validateAdminInput(actor string, in *AdminDefinitionInput, creating bool) error {
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	in.Title = strings.TrimSpace(in.Title)
	in.Intro = strings.TrimSpace(in.Intro)
	in.Reason = strings.TrimSpace(in.Reason)
	d := Definition{Slug: in.Slug, Title: in.Title, Intro: in.Intro, Version: 1, Questions: in.Questions, Pricing: in.Pricing}
	if actor == "" || !slugPattern.MatchString(in.Slug) || len(in.Slug) > 160 || len([]rune(in.Title)) < 1 || len([]rune(in.Title)) > 220 || len([]rune(in.Intro)) < 1 || len([]rune(in.Intro)) > 600 || len([]rune(in.Reason)) < 3 || len([]rune(in.Reason)) > 500 || validateDefinition(d) != nil {
		return ErrInvalid
	}
	for _, byOption := range in.Pricing.OptionBasis {
		for _, points := range byOption {
			if points < 0 || points > 50000 {
				return ErrInvalid
			}
		}
	}
	for _, points := range in.Pricing.NumberBasis {
		if points < 0 || points > 10000 {
			return ErrInvalid
		}
	}
	for _, points := range in.Pricing.BooleanBasis {
		if points < 0 || points > 50000 {
			return ErrInvalid
		}
	}
	if len(in.Pricing.SkillSlugs) > 30 || len(in.Pricing.Assumptions) > 20 {
		return ErrInvalid
	}
	_ = creating
	return nil
}
func (s Service) Estimate(ctx context.Context, actor, slug string, answers map[string]any, a Attribution) (Estimate, error) {
	d, e := s.Definition(ctx, slug)
	if e != nil {
		return Estimate{}, e
	}
	points, e := validateAnswers(d, answers)
	if e != nil {
		return Estimate{}, e
	}
	if e = validateAttribution(&a, actor); e != nil {
		return Estimate{}, e
	}
	taxonomy, e := s.Repository.Resolve(ctx, d.Pricing.CategorySlug, d.Pricing.SkillSlugs)
	if e != nil {
		return Estimate{}, e
	}
	min := d.Pricing.BaselineMin * (10000 + int64(points)) / 10000
	max := d.Pricing.BaselineMax * (10000 + int64(points)) / 10000
	if min < 1 || max < min || max > 10_000_000_000 {
		return Estimate{}, ErrInvalid
	}
	result := Estimate{EstimatedMinKopecks: min, EstimatedMaxKopecks: max, Currency: "RUB", Duration: ai.DurationRange{Min: d.Pricing.DurationMin, Max: d.Pricing.DurationMax}, Category: taxonomy.Category, Skills: taxonomy.Skills, Assumptions: append([]string(nil), d.Pricing.Assumptions...), Confidence: "ESTIMATE"}
	raw := map[string]any{"calculator_slug": d.Slug, "calculator_version": d.Version, "answers": answers, "attribution": a}
	draft, token, e := s.Drafts.CreateDraft(ctx, actor, "CALCULATOR", raw)
	if e != nil {
		return Estimate{}, e
	}
	normalized := map[string]any{"title": d.Title, "summary": fmt.Sprintf("Расчёт по калькулятору «%s». Ответы сохранены в черновике.", d.Title), "budget": map[string]any{"min_kopecks": min, "max_kopecks": max, "currency": "RUB", "confidence": "ESTIMATE"}, "duration_days": result.Duration, "assumptions": result.Assumptions, "calculator_slug": d.Slug, "calculator_version": d.Version}
	if taxonomy.Category != nil {
		normalized["category_candidates"] = []ai.CategoryCandidate{*taxonomy.Category}
	}
	normalized["skills"] = taxonomy.Skills
	if _, e = s.Drafts.UpdateDraft(ctx, actor, token, raw, normalized); e != nil {
		return Estimate{}, e
	}
	result.DraftToken = token
	_ = s.Repository.Record(ctx, Event{Attribution: a, Type: "CALCULATOR_COMPLETED", Metadata: map[string]string{"calculator_slug": d.Slug}, UserID: actor})
	_ = draft
	return result, nil
}
func (s Service) Record(ctx context.Context, actor string, e Event) error {
	e.UserID = actor
	e.Type = strings.ToUpper(strings.TrimSpace(e.Type))
	if !allowedEvent(e.Type) || validateAttribution(&e.Attribution, actor) != nil {
		return ErrInvalid
	}
	if len(e.Metadata) > 5 {
		return ErrInvalid
	}
	safe := map[string]string{}
	for k, v := range e.Metadata {
		if !oneOf(k, "calculator_slug", "source", "draft_source") || len(v) > 160 {
			return ErrInvalid
		}
		safe[k] = strings.TrimSpace(v)
	}
	e.Metadata = safe
	return s.Repository.Record(ctx, e)
}
func validateAnswers(d Definition, answers map[string]any) (int, error) {
	encoded, _ := json.Marshal(answers)
	if len(encoded) > 32<<10 || len(answers) != len(d.Questions) {
		return 0, ErrInvalid
	}
	points := 0
	for _, q := range d.Questions {
		v, ok := answers[q.Key]
		if !ok && q.Required {
			return 0, ErrInvalid
		}
		switch q.Type {
		case "SELECT":
			text, ok := v.(string)
			if !ok {
				return 0, ErrInvalid
			}
			valid := false
			for _, o := range q.Options {
				if text == o.Value {
					valid = true
				}
			}
			if !valid {
				return 0, ErrInvalid
			}
			points += d.Pricing.OptionBasis[q.Key][text]
		case "NUMBER":
			number, ok := v.(float64)
			if !ok || number != float64(int(number)) || (q.Min != nil && int(number) < *q.Min) || (q.Max != nil && int(number) > *q.Max) {
				return 0, ErrInvalid
			}
			points += int(number) * d.Pricing.NumberBasis[q.Key]
		case "BOOLEAN":
			flag, ok := v.(bool)
			if !ok {
				return 0, ErrInvalid
			}
			if flag {
				points += d.Pricing.BooleanBasis[q.Key]
			}
		default:
			return 0, ErrInvalid
		}
	}
	if points < 0 || points > 50000 {
		return 0, ErrInvalid
	}
	return points, nil
}
func validateDefinition(d Definition) error {
	if !slugPattern.MatchString(d.Slug) || d.Version < 1 || len(d.Questions) < 1 || len(d.Questions) > 20 || d.Pricing.BaselineMin < 100 || d.Pricing.BaselineMax < d.Pricing.BaselineMin || d.Pricing.BaselineMax > 10_000_000_000 || d.Pricing.DurationMin < 1 || d.Pricing.DurationMax < d.Pricing.DurationMin || d.Pricing.DurationMax > 3650 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, q := range d.Questions {
		if seen[q.Key] || !questionKeyPattern.MatchString(q.Key) || !oneOf(q.Type, "SELECT", "NUMBER", "BOOLEAN") || len([]rune(q.Label)) < 1 || len([]rune(q.Label)) > 200 {
			return ErrInvalid
		}
		seen[q.Key] = true
		switch q.Type {
		case "SELECT":
			if len(q.Options) < 2 || len(q.Options) > 30 {
				return ErrInvalid
			}
			optionSeen := map[string]bool{}
			for _, option := range q.Options {
				if !questionKeyPattern.MatchString(option.Value) || optionSeen[option.Value] || len([]rune(option.Label)) < 1 || len([]rune(option.Label)) > 120 {
					return ErrInvalid
				}
				optionSeen[option.Value] = true
			}
			if len(d.Pricing.OptionBasis[q.Key]) != len(q.Options) {
				return ErrInvalid
			}
		case "NUMBER":
			if q.Min == nil || q.Max == nil || *q.Min < 0 || *q.Max < *q.Min || *q.Max > 100000 {
				return ErrInvalid
			}
		case "BOOLEAN":
			if len(q.Options) != 0 {
				return ErrInvalid
			}
		}
	}
	return nil
}
func validateAttribution(a *Attribution, actor string) error {
	a.AnonymousID = strings.ToLower(strings.TrimSpace(a.AnonymousID))
	if actor == "" && !uuidPattern.MatchString(a.AnonymousID) {
		return ErrInvalid
	}
	if a.AnonymousID != "" && !uuidPattern.MatchString(a.AnonymousID) {
		return ErrInvalid
	}
	a.LandingPath = strings.TrimSpace(a.LandingPath)
	if a.LandingPath != "" && (!strings.HasPrefix(a.LandingPath, "/") || strings.HasPrefix(a.LandingPath, "//") || strings.ContainsAny(a.LandingPath, "?#") || len(a.LandingPath) > 500) {
		return ErrInvalid
	}
	for _, v := range []*string{&a.UTMSource, &a.UTMMedium, &a.UTMCampaign, &a.UTMContent, &a.ReferralCode} {
		*v = strings.TrimSpace(*v)
		if len(*v) > 200 {
			return ErrInvalid
		}
	}
	return nil
}
func allowedEvent(v string) bool {
	return oneOf(v, "LANDING_VIEW", "HOMEPAGE_TASK_STARTED", "GUEST_PROJECT_ANALYSIS_COMPLETED", "CALCULATOR_STARTED", "CALCULATOR_COMPLETED", "COMMERCIAL_OFFER_ANALYZED", "REGISTRATION_COMPLETED", "PROJECT_DRAFT_CLAIMED", "PROJECT_PUBLISHED", "PROPOSAL_RECEIVED", "PROPOSAL_ACCEPTED", "PROJECT_COMPLETED")
}
func oneOf(v string, values ...string) bool {
	for _, x := range values {
		if v == x {
			return true
		}
	}
	return false
}

type Store struct {
	mu           sync.Mutex
	Items        map[string]Definition
	Events       []Event
	Taxonomy     map[string]Taxonomy
	SitemapItems []SitemapItem
}

func (s *Store) Definition(_ context.Context, slug string) (Definition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.Items[slug]
	if !ok {
		return Definition{}, ErrNotFound
	}
	return v, nil
}
func (s *Store) Definitions(_ context.Context) ([]Definition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Definition, 0, len(s.Items))
	for _, value := range s.Items {
		value.Pricing = PricingConfig{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Title < result[j].Title })
	return result, nil
}
func (s *Store) Resolve(_ context.Context, category string, skills []string) (Taxonomy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.Taxonomy[category]; ok {
		return v, nil
	}
	return Taxonomy{Skills: []ai.SkillCandidate{}}, nil
}
func (s *Store) Record(_ context.Context, e Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Events = append(s.Events, e)
	return nil
}
func (s *Store) Sitemap(_ context.Context) ([]SitemapItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]SitemapItem(nil), s.SitemapItems...)
	for slug, d := range s.Items {
		out = append(out, SitemapItem{Path: "/price/" + slug, UpdatedAt: d.UpdatedAt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}
func (s *Store) AdminDefinitions(_ context.Context) ([]AdminDefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]AdminDefinition, 0, len(s.Items))
	for _, d := range s.Items {
		result = append(result, adminDefinition(d, true))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Title < result[j].Title })
	return result, nil
}
func (s *Store) CreateAdminDefinition(_ context.Context, _ string, in AdminDefinitionInput) (AdminDefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.Items[in.Slug]; exists {
		return AdminDefinition{}, ErrInvalid
	}
	d := Definition{Slug: in.Slug, Title: in.Title, Intro: in.Intro, Version: 1, Questions: in.Questions, Pricing: in.Pricing, UpdatedAt: time.Now().UTC()}
	s.Items[in.Slug] = d
	return adminDefinition(d, in.Enabled), nil
}
func (s *Store) UpdateAdminDefinition(_ context.Context, _ string, slug string, in AdminDefinitionInput) (AdminDefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.Items[slug]
	if !ok {
		return AdminDefinition{}, ErrNotFound
	}
	d.Title, d.Intro, d.Questions, d.Pricing = in.Title, in.Intro, in.Questions, in.Pricing
	d.Version++
	d.UpdatedAt = time.Now().UTC()
	s.Items[slug] = d
	return adminDefinition(d, in.Enabled), nil
}
func adminDefinition(d Definition, enabled bool) AdminDefinition {
	return AdminDefinition{Slug: d.Slug, Title: d.Title, Intro: d.Intro, Version: d.Version, Enabled: enabled, Questions: d.Questions, Pricing: d.Pricing}
}
