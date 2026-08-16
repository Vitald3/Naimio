package ai

import (
	"context"
	"errors"
	"time"
)

type Capability string

const (
	ProjectBrief       Capability = "project_brief"
	ProjectImport      Capability = "project_import"
	ProjectEstimate    Capability = "estimate"
	OfferAnalysis      Capability = "offer_analysis"
	TaxonomySuggestion Capability = "taxonomy_suggestion"
	MatchRerank        Capability = "match_rerank"
)

var (
	ErrInvalidInput  = errors.New("invalid ai input")
	ErrInvalidOutput = errors.New("invalid ai output")
	ErrNotFound      = errors.New("draft not found")
	ErrForbidden     = errors.New("draft access forbidden")
	ErrUnavailable   = errors.New("ai temporarily unavailable")
)

type Usage struct {
	InputTokens  int `json:"-"`
	OutputTokens int `json:"-"`
}
type CategoryCandidate struct {
	ID         string  `json:"id"`
	Slug       string  `json:"slug"`
	Name       string  `json:"name"`
	Confidence float64 `json:"confidence"`
}
type SkillCandidate struct {
	ID         string  `json:"id"`
	Slug       string  `json:"slug"`
	Name       string  `json:"name"`
	Confidence float64 `json:"confidence"`
}
type Range struct {
	MinKopecks int64  `json:"min_kopecks"`
	MaxKopecks int64  `json:"max_kopecks"`
	Currency   string `json:"currency"`
	Confidence string `json:"confidence"`
}
type DurationRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}
type ProjectBriefResult struct {
	Title        string              `json:"title"`
	Summary      string              `json:"summary"`
	Requirements []string            `json:"requirements"`
	Questions    []string            `json:"questions"`
	Assumptions  []string            `json:"assumptions"`
	Categories   []CategoryCandidate `json:"category_candidates"`
	Skills       []SkillCandidate    `json:"skills"`
	Budget       Range               `json:"budget"`
	DurationDays DurationRange       `json:"duration_days"`
	Usage        Usage               `json:"-"`
}
type ProjectImportResult struct {
	Title                     string              `json:"title"`
	Goal                      string              `json:"goal"`
	Scope                     string              `json:"scope"`
	FunctionalRequirements    []string            `json:"functional_requirements"`
	NonFunctionalRequirements []string            `json:"non_functional_requirements"`
	Integrations              []string            `json:"integrations"`
	Deliverables              []string            `json:"deliverables"`
	Questions                 []string            `json:"questions"`
	Assumptions               []string            `json:"assumptions"`
	ProvidedBudget            *Range              `json:"provided_budget,omitempty"`
	ProvidedDeadline          string              `json:"provided_deadline,omitempty"`
	Categories                []CategoryCandidate `json:"category_candidates"`
	Skills                    []SkillCandidate    `json:"skills"`
	Usage                     Usage               `json:"-"`
}
type EstimateResult struct {
	Range        Range         `json:"range"`
	DurationDays DurationRange `json:"duration_days"`
	Drivers      []string      `json:"drivers"`
	Assumptions  []string      `json:"assumptions"`
	Usage        Usage         `json:"-"`
}
type OfferAnalysisResult struct {
	Deliverables     []string `json:"deliverables"`
	PricingBreakdown []string `json:"pricing_breakdown"`
	MissingScope     []string `json:"missing_scope"`
	RiskQuestions    []string `json:"risk_questions"`
	BenchmarkNotes   []string `json:"benchmark_notes"`
	Benchmark        Range    `json:"benchmark"`
	Confidence       string   `json:"confidence"`
	Usage            Usage    `json:"-"`
}
type TaxonomyResult struct {
	Categories []CategoryCandidate `json:"categories"`
	Skills     []SkillCandidate    `json:"skills"`
	Usage      Usage               `json:"-"`
}

type BriefRequest struct {
	Text     string
	Existing map[string]any
}
type ImportRequest struct{ Materials []Material }
type Material struct{ Name, Text string }
type EstimateRequest struct {
	Text, CategorySlug   string
	SkillSlugs, Features []string
}
type OfferRequest struct{ Text, Goal string }
type TaxonomyRequest struct{ Text string }
type MatchCandidate struct {
	ID                                               string
	DeterministicScore, SkillMatches, RequiredSkills int
	Availability                                     string
	NativeRating                                     *float64
	Reviews                                          int
}
type MatchRerankRequest struct {
	ProjectTitle, ProjectDescription string
	Candidates                       []MatchCandidate
}
type MatchRerankResult struct {
	OrderedIDs  []string
	ReasonCodes map[string][]string
	Usage       Usage
}

// AIProvider is the replaceable provider boundary. Provider SDK values never cross it.
type AIProvider interface {
	GenerateProjectBrief(context.Context, BriefRequest) (ProjectBriefResult, error)
	ExtractProject(context.Context, ImportRequest) (ProjectImportResult, error)
	EstimateProject(context.Context, EstimateRequest) (EstimateResult, error)
	AnalyzeCommercialOffer(context.Context, OfferRequest) (OfferAnalysisResult, error)
	SuggestTaxonomy(context.Context, TaxonomyRequest) (TaxonomyResult, error)
	RerankCandidates(context.Context, MatchRerankRequest) (MatchRerankResult, error)
}

type RequestMetric struct {
	UserID                             string
	Capability                         Capability
	Provider, Model, Status, ErrorCode string
	InputTokens, OutputTokens          int
	CostMicrounits                     int64
	Latency                            time.Duration
}
type MetricRecorder interface {
	Record(context.Context, RequestMetric) error
}

type Draft struct {
	ID             string         `json:"id"`
	OwnerUserID    string         `json:"-"`
	SourceType     string         `json:"source_type"`
	RawInput       map[string]any `json:"raw_input"`
	NormalizedData map[string]any `json:"normalized_data"`
	ExpiresAt      time.Time      `json:"expires_at"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}
type DraftRepository interface {
	Create(context.Context, string, string, map[string]any) (Draft, string, error)
	Get(context.Context, string, string) (Draft, error)
	Update(context.Context, string, string, map[string]any, map[string]any) (Draft, error)
	Claim(context.Context, string, string) (Draft, error)
}

type TaxonomyRepository interface {
	Candidates(context.Context) ([]CategoryCandidate, []SkillCandidate, error)
}
