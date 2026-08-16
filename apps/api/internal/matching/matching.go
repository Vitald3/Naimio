package matching

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrUnauthorized = errors.New("authentication required")
	ErrNotFound     = errors.New("matching resource not found")
	ErrForbidden    = errors.New("matching forbidden")
	ErrInvalid      = errors.New("invalid matching input")
	ErrInvalidState = errors.New("project is not matchable")
)

const AlgorithmVersion = "deterministic-v1"

type Constraints struct {
	RequireImmediateAvailability bool   `json:"require_immediate_availability"`
	RequireCategoryMatch         bool   `json:"require_category_match"`
	MaxMinimumOrderKopecks       *int64 `json:"max_minimum_order_kopecks,omitempty"`
}
type Project struct {
	ID, CustomerID, Title, Description, CategoryID string
	SkillIDs                                       []string
	BudgetMin, BudgetMax                           *int64
	DeadlineAt                                     *time.Time
	Status                                         string
}
type Candidate struct {
	UserID, Username, DisplayName, ProfessionalTitle, Availability, PrimaryCategoryID                                                                                        string
	HourlyRate, MinimumOrder                                                                                                                                                 *int64
	ResponseMinutes                                                                                                                                                          *int
	ProfileCompletion, SkillMatches, FeaturedSkillMatches, RequiredSkills, RelevantPortfolio, SimilarCompleted, Reviews, CompletedProjects, VerifiedExternal, RepeatProjects int
	NativeRating, CompletionRate, OnTimeRate, RecommendationRate                                                                                                             *float64
}
type Reason struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}
type Recommendation struct {
	FreelancerID       string   `json:"freelancer_user_id"`
	Username           string   `json:"username,omitempty"`
	DisplayName        string   `json:"display_name"`
	ProfessionalTitle  string   `json:"professional_title,omitempty"`
	Availability       string   `json:"availability"`
	HourlyRate         *int64   `json:"hourly_rate_kopecks,omitempty"`
	NativeRating       *float64 `json:"native_rating,omitempty"`
	ReviewsCount       int      `json:"reviews_count"`
	CompletedProjects  int      `json:"completed_projects_count"`
	Score              int      `json:"score"`
	DeterministicScore int      `json:"-"`
	Rank               int      `json:"rank"`
	Reasons            []Reason `json:"reasons"`
	Manual             bool     `json:"platform_recommended"`
}
type Run struct {
	ID               string           `json:"id"`
	ProjectID        string           `json:"project_id"`
	AlgorithmVersion string           `json:"algorithm_version"`
	AIUsed           bool             `json:"ai_used"`
	CandidateCount   int              `json:"candidate_count"`
	CreatedAt        time.Time        `json:"created_at"`
	Recommendations  []Recommendation `json:"recommendations"`
}
type RerankCandidate struct {
	ID                                               string
	DeterministicScore, SkillMatches, RequiredSkills int
	Availability                                     string
	NativeRating                                     *float64
	Reviews                                          int
}
type RerankRequest struct {
	ProjectTitle, ProjectDescription string
	Candidates                       []RerankCandidate
}
type RerankResult struct {
	OrderedIDs  []string
	ReasonCodes map[string][]string
}
type Reranker interface {
	Rerank(context.Context, string, RerankRequest) (RerankResult, error)
}
type Metrics struct {
	Runs              int64   `json:"runs"`
	Candidates        int64   `json:"candidates"`
	AIUsed            int64   `json:"ai_used_runs"`
	Impressions       int64   `json:"impressions"`
	ProfileOpens      int64   `json:"profile_opens"`
	Invites           int64   `json:"invites"`
	Shortlists        int64   `json:"shortlists"`
	Proposals         int64   `json:"proposals"`
	Acceptances       int64   `json:"acceptances"`
	Completed         int64   `json:"completed"`
	Repeats           int64   `json:"repeats"`
	AverageCandidates float64 `json:"average_candidates"`
	AIRequests        int64   `json:"ai_requests"`
	AIFailures        int64   `json:"ai_failures"`
	AICostMicrounits  int64   `json:"ai_cost_microunits"`
	AIP95LatencyMS    float64 `json:"ai_p95_latency_ms"`
}

type Repository interface {
	OwnedProject(context.Context, string, string) (Project, error)
	Retrieve(context.Context, Project, Constraints, int) ([]Candidate, error)
	SaveRun(context.Context, string, Project, Constraints, []Recommendation, bool) (Run, error)
	Run(context.Context, string, string, string) (Run, error)
	Latest(context.Context, string, string) (Run, error)
	ManualPut(context.Context, string, string, string, string) error
	ManualDelete(context.Context, string, string, string) error
	RecordEvent(context.Context, string, string, string, string, string, string) error
	Metrics(context.Context, string) (Metrics, error)
}

type Weights struct{ Skills, Category, Portfolio, Trust, External, Availability, Budget, Deadline, Response, Repeat int }

func DefaultWeights() Weights {
	return Weights{Skills: 2500, Category: 1500, Portfolio: 1500, Trust: 1200, External: 600, Availability: 700, Budget: 700, Deadline: 500, Response: 400, Repeat: 400}
}
func (w Weights) Validate() error {
	values := []int{w.Skills, w.Category, w.Portfolio, w.Trust, w.External, w.Availability, w.Budget, w.Deadline, w.Response, w.Repeat}
	sum := 0
	for _, v := range values {
		if v < 0 || v > 5000 {
			return ErrInvalid
		}
		sum += v
	}
	if sum != 10000 {
		return fmt.Errorf("%w: weights must sum to 10000", ErrInvalid)
	}
	return nil
}

type Service struct {
	Repository                     Repository
	Reranker                       Reranker
	Weights                        Weights
	RetrievalLimit, ShortlistLimit int
}

func (s Service) Run(ctx context.Context, actor, projectID string, constraints Constraints) (Run, error) {
	if actor == "" {
		return Run{}, ErrUnauthorized
	}
	if constraints.MaxMinimumOrderKopecks != nil && *constraints.MaxMinimumOrderKopecks < 0 {
		return Run{}, ErrInvalid
	}
	weights := s.Weights
	if weights == (Weights{}) {
		weights = DefaultWeights()
	}
	if err := weights.Validate(); err != nil {
		return Run{}, err
	}
	limit := s.RetrievalLimit
	if limit < 50 || limit > 200 {
		limit = 100
	}
	shortlist := s.ShortlistLimit
	if shortlist < 1 || shortlist > 20 {
		shortlist = 20
	}
	project, err := s.Repository.OwnedProject(ctx, actor, projectID)
	if err != nil {
		return Run{}, err
	}
	if project.Status != "DRAFT" && project.Status != "OPEN" && project.Status != "MATCHING" {
		return Run{}, ErrInvalidState
	}
	candidates, err := s.Repository.Retrieve(ctx, project, constraints, limit)
	if err != nil {
		return Run{}, err
	}
	recommendations := make([]Recommendation, 0, len(candidates))
	byID := map[string]Candidate{}
	for _, candidate := range candidates {
		recommendation := score(project, candidate, weights)
		recommendations = append(recommendations, recommendation)
		byID[candidate.UserID] = candidate
	}
	sort.SliceStable(recommendations, func(i, j int) bool {
		if recommendations[i].Score != recommendations[j].Score {
			return recommendations[i].Score > recommendations[j].Score
		}
		return recommendations[i].FreelancerID < recommendations[j].FreelancerID
	})
	applyColdStart(recommendations, byID, shortlist)
	aiUsed := false
	if s.Reranker != nil && len(recommendations) > 1 {
		count := min(shortlist, len(recommendations))
		request := RerankRequest{ProjectTitle: project.Title, ProjectDescription: truncate(project.Description, 1000), Candidates: make([]RerankCandidate, 0, count)}
		for _, item := range recommendations[:count] {
			candidate := byID[item.FreelancerID]
			request.Candidates = append(request.Candidates, RerankCandidate{ID: item.FreelancerID, DeterministicScore: item.Score, SkillMatches: candidate.SkillMatches, RequiredSkills: candidate.RequiredSkills, Availability: candidate.Availability, NativeRating: candidate.NativeRating, Reviews: candidate.Reviews})
		}
		if reranked, rerankErr := s.Reranker.Rerank(ctx, actor, request); rerankErr == nil && validRerank(request, reranked) {
			recommendations = applyRerank(recommendations, count, reranked)
			aiUsed = true
		}
	}
	for index := range recommendations {
		recommendations[index].Rank = index + 1
	}
	return s.Repository.SaveRun(ctx, actor, project, constraints, recommendations, aiUsed)
}
func (s Service) RunByID(ctx context.Context, actor, projectID, runID string) (Run, error) {
	return s.Repository.Run(ctx, actor, projectID, runID)
}
func (s Service) Latest(ctx context.Context, actor, projectID string) (Run, error) {
	return s.Repository.Latest(ctx, actor, projectID)
}
func (s Service) PutManual(ctx context.Context, admin, projectID, freelancerID, reason string) error {
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) < 3 || len([]rune(reason)) > 1000 {
		return ErrInvalid
	}
	return s.Repository.ManualPut(ctx, admin, projectID, freelancerID, reason)
}
func (s Service) DeleteManual(ctx context.Context, admin, projectID, freelancerID string) error {
	return s.Repository.ManualDelete(ctx, admin, projectID, freelancerID)
}
func (s Service) Event(ctx context.Context, actor, projectID, runID, freelancerID, eventType, key string) error {
	allowed := map[string]bool{"IMPRESSION": true, "PROFILE_OPEN": true, "INVITE": true, "SHORTLIST": true, "PROPOSAL": true, "ACCEPTANCE": true, "COMPLETED": true, "REPEAT": true}
	eventType = strings.ToUpper(strings.TrimSpace(eventType))
	if !allowed[eventType] || len(key) < 8 || len(key) > 100 {
		return ErrInvalid
	}
	return s.Repository.RecordEvent(ctx, actor, projectID, runID, freelancerID, eventType, key)
}
func (s Service) Metrics(ctx context.Context, admin string) (Metrics, error) {
	return s.Repository.Metrics(ctx, admin)
}

func score(project Project, c Candidate, w Weights) Recommendation {
	parts := map[string]int{}
	if c.RequiredSkills == 0 {
		parts["skills"] = w.Skills / 2
	} else {
		coverage := min(c.SkillMatches, c.RequiredSkills)
		parts["skills"] = w.Skills * coverage / c.RequiredSkills
		parts["skills"] += min(w.Skills/5, w.Skills*c.FeaturedSkillMatches/max(1, c.RequiredSkills)/5)
	}
	if project.CategoryID != "" && c.PrimaryCategoryID == project.CategoryID {
		parts["category"] = w.Category
	}
	parts["portfolio"] = w.Portfolio * min(3, c.RelevantPortfolio+c.SimilarCompleted) / 3
	trustBase := 0
	if c.NativeRating != nil {
		trustBase += int(*c.NativeRating * 120)
	}
	trustBase += min(250, c.Reviews*25)
	if c.CompletionRate != nil {
		trustBase += int(*c.CompletionRate * 2)
	}
	if c.OnTimeRate != nil {
		trustBase += int(*c.OnTimeRate)
	}
	if c.RecommendationRate != nil {
		trustBase += int(*c.RecommendationRate)
	}
	if c.Reviews == 0 {
		trustBase = max(trustBase, c.ProfileCompletion*4)
	}
	parts["trust"] = w.Trust * min(1000, trustBase) / 1000
	parts["external"] = w.External * min(2, c.VerifiedExternal) / 2
	switch c.Availability {
	case "AVAILABLE":
		parts["availability"] = w.Availability
	case "PARTIALLY_BUSY":
		parts["availability"] = w.Availability * 2 / 3
	case "BUSY":
		parts["availability"] = w.Availability / 4
	}
	if project.BudgetMax == nil || c.MinimumOrder == nil || *c.MinimumOrder <= *project.BudgetMax {
		parts["budget"] = w.Budget
	}
	if project.DeadlineAt == nil || c.Availability == "AVAILABLE" {
		parts["deadline"] = w.Deadline
	} else if c.Availability == "PARTIALLY_BUSY" {
		parts["deadline"] = w.Deadline / 2
	}
	if c.ResponseMinutes == nil {
		parts["response"] = w.Response / 3
	} else if *c.ResponseMinutes <= 60 {
		parts["response"] = w.Response
	} else if *c.ResponseMinutes <= 1440 {
		parts["response"] = w.Response * 2 / 3
	} else {
		parts["response"] = w.Response / 3
	}
	parts["repeat"] = w.Repeat * min(2, c.RepeatProjects) / 2
	total := 0
	for _, value := range parts {
		total += value
	}
	total = min(10000, total)
	reasons := []Reason{}
	if c.RequiredSkills > 0 && c.SkillMatches > 0 {
		reasons = append(reasons, Reason{Code: "SKILL_COVERAGE", Label: fmt.Sprintf("%d из %d ключевых навыков", c.SkillMatches, c.RequiredSkills)})
	}
	if c.PrimaryCategoryID == project.CategoryID && project.CategoryID != "" {
		reasons = append(reasons, Reason{Code: "CATEGORY_MATCH", Label: "Работает в нужной категории"})
	}
	if c.RelevantPortfolio+c.SimilarCompleted > 0 {
		reasons = append(reasons, Reason{Code: "SIMILAR_WORK", Label: fmt.Sprintf("%d похожих работ", c.RelevantPortfolio+c.SimilarCompleted)})
	}
	if c.Availability == "AVAILABLE" {
		reasons = append(reasons, Reason{Code: "AVAILABLE_NOW", Label: "Свободен сейчас"})
	}
	if c.CompletionRate != nil && *c.CompletionRate >= 80 {
		reasons = append(reasons, Reason{Code: "NATIVE_TRUST", Label: fmt.Sprintf("%.0f%% завершённых проектов", *c.CompletionRate)})
	}
	if c.VerifiedExternal > 0 {
		reasons = append(reasons, Reason{Code: "VERIFIED_EXTERNAL", Label: "Есть подтверждённая внешняя репутация"})
	}
	if c.RepeatProjects > 0 {
		reasons = append(reasons, Reason{Code: "REPEAT_COLLABORATION", Label: "Уже успешно работал с вами"})
	}
	if c.Reviews == 0 && c.ProfileCompletion >= 70 {
		reasons = append(reasons, Reason{Code: "COLD_START_POTENTIAL", Label: "Новый специалист с заполненным профилем"})
	}
	if len(reasons) > 5 {
		reasons = reasons[:5]
	}
	return Recommendation{FreelancerID: c.UserID, Username: c.Username, DisplayName: c.DisplayName, ProfessionalTitle: c.ProfessionalTitle, Availability: c.Availability, HourlyRate: c.HourlyRate, NativeRating: c.NativeRating, ReviewsCount: c.Reviews, CompletedProjects: c.CompletedProjects, Score: total, DeterministicScore: total, Reasons: reasons}
}
func applyColdStart(items []Recommendation, byID map[string]Candidate, limit int) {
	if len(items) <= 1 || limit < 2 {
		return
	}
	top := min(limit, len(items))
	for i := 0; i < top; i++ {
		if byID[items[i].FreelancerID].Reviews == 0 {
			return
		}
	}
	for i := top; i < len(items); i++ {
		candidate := byID[items[i].FreelancerID]
		if candidate.Reviews == 0 && candidate.ProfileCompletion >= 70 && items[i].Score+1500 >= items[top-1].Score {
			value := items[i]
			copy(items[top:], items[top-1:i])
			items[top-1] = value
			return
		}
	}
}
func validRerank(request RerankRequest, result RerankResult) bool {
	if len(result.OrderedIDs) != len(request.Candidates) {
		return false
	}
	known := map[string]bool{}
	for _, candidate := range request.Candidates {
		known[candidate.ID] = true
	}
	seen := map[string]bool{}
	for _, id := range result.OrderedIDs {
		if !known[id] || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}
func applyRerank(items []Recommendation, count int, result RerankResult) []Recommendation {
	byID := map[string]Recommendation{}
	for _, item := range items[:count] {
		byID[item.FreelancerID] = item
	}
	prefix := make([]Recommendation, 0, count)
	for index, id := range result.OrderedIDs {
		item := byID[id]
		adjustment := (count - index) * 10
		item.Score = min(10000, item.Score+adjustment)
		for _, code := range result.ReasonCodes[id] {
			if code == "STRONG_SCOPE_FIT" {
				item.Reasons = append(item.Reasons, Reason{Code: code, Label: "Хорошо соответствует описанному объёму"})
			}
		}
		if len(item.Reasons) > 5 {
			item.Reasons = item.Reasons[:5]
		}
		prefix = append(prefix, item)
	}
	return append(prefix, items[count:]...)
}
func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}
