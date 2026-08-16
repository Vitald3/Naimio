package ai

import (
	"context"
	"regexp"
	"sort"
	"strings"
)

type DeterministicProvider struct {
	Taxonomy  TaxonomyRepository
	Baselines map[string]Range
}

func (p DeterministicProvider) GenerateProjectBrief(ctx context.Context, req BriefRequest) (ProjectBriefResult, error) {
	text := clean(req.Text)
	if text == "" {
		return ProjectBriefResult{}, ErrInvalidInput
	}
	taxonomy, err := p.SuggestTaxonomy(ctx, TaxonomyRequest{Text: text})
	if err != nil {
		return ProjectBriefResult{}, err
	}
	title := firstSentence(text, 120)
	result := ProjectBriefResult{Title: title, Summary: text, Requirements: sentences(text, 8), Questions: missingQuestions(text), Categories: taxonomy.Categories, Skills: taxonomy.Skills, Budget: p.baseline(firstCategory(taxonomy.Categories)), DurationDays: DurationRange{Min: 7, Max: 21}, Usage: estimateUsage(text)}
	if len(text) > 1000 {
		result.DurationDays = DurationRange{Min: 21, Max: 60}
		result.Assumptions = []string{"Объём уточняется после ответов на вопросы"}
	}
	return result, validateBrief(result)
}

func (p DeterministicProvider) ExtractProject(ctx context.Context, req ImportRequest) (ProjectImportResult, error) {
	parts := make([]string, 0, len(req.Materials))
	for _, material := range req.Materials {
		if value := clean(material.Text); value != "" {
			parts = append(parts, value)
		}
	}
	text := strings.Join(parts, "\n")
	if text == "" {
		return ProjectImportResult{}, ErrInvalidInput
	}
	taxonomy, err := p.SuggestTaxonomy(ctx, TaxonomyRequest{Text: text})
	if err != nil {
		return ProjectImportResult{}, err
	}
	result := ProjectImportResult{Title: firstSentence(text, 120), Goal: firstSentence(text, 500), Scope: text, FunctionalRequirements: sentences(text, 12), Questions: missingQuestions(text), Categories: taxonomy.Categories, Skills: taxonomy.Skills, Usage: estimateUsage(text)}
	return result, validateImport(result)
}

func (p DeterministicProvider) EstimateProject(_ context.Context, req EstimateRequest) (EstimateResult, error) {
	text := clean(req.Text)
	if text == "" {
		return EstimateResult{}, ErrInvalidInput
	}
	value := p.baseline(req.CategorySlug)
	multiplier := int64(100)
	lower := strings.ToLower(text + " " + strings.Join(req.Features, " "))
	drivers := []string{"Базовый диапазон категории"}
	for _, item := range []struct {
		word, label string
		add         int64
	}{{"интеграц", "Интеграции", 20}, {"мобиль", "Мобильные клиенты", 25}, {"миграц", "Миграция данных", 15}, {"срочно", "Сжатые сроки", 20}} {
		if strings.Contains(lower, item.word) {
			multiplier += item.add
			drivers = append(drivers, item.label)
		}
	}
	if len([]rune(text)) > 2500 {
		multiplier += 30
		drivers = append(drivers, "Большой объём описания")
	}
	value.MinKopecks = round(value.MinKopecks * multiplier / 100)
	value.MaxKopecks = round(value.MaxKopecks * multiplier / 100)
	confidence := "LOW"
	if req.CategorySlug != "" && len(req.SkillSlugs) > 0 {
		confidence = "MEDIUM"
	}
	value.Confidence = confidence
	duration := DurationRange{Min: 10, Max: 30}
	if multiplier >= 140 {
		duration = DurationRange{Min: 25, Max: 60}
	}
	result := EstimateResult{Range: value, DurationDays: duration, Drivers: drivers, Assumptions: []string{"Оценка ориентировочная; финальную цену предлагают специалисты"}, Usage: estimateUsage(text)}
	return result, validateEstimate(result)
}

func (p DeterministicProvider) AnalyzeCommercialOffer(ctx context.Context, req OfferRequest) (OfferAnalysisResult, error) {
	text := clean(req.Text)
	if text == "" {
		return OfferAnalysisResult{}, ErrInvalidInput
	}
	estimate, err := p.EstimateProject(ctx, EstimateRequest{Text: text})
	if err != nil {
		return OfferAnalysisResult{}, err
	}
	pricing := moneyLines(text)
	missing := []string{}
	lower := strings.ToLower(text)
	for _, item := range []string{"срок", "приёмк", "поддержк"} {
		if !strings.Contains(lower, item) {
			missing = append(missing, "В предложении не найдено: "+item)
		}
	}
	result := OfferAnalysisResult{Deliverables: sentences(text, 10), PricingBreakdown: pricing, MissingScope: missing, RiskQuestions: missingQuestions(text), Benchmark: estimate.Range, Confidence: "LOW", Usage: estimateUsage(text)}
	return result, validateOffer(result)
}

func (p DeterministicProvider) SuggestTaxonomy(ctx context.Context, req TaxonomyRequest) (TaxonomyResult, error) {
	text := strings.ToLower(clean(req.Text))
	if text == "" {
		return TaxonomyResult{}, ErrInvalidInput
	}
	if p.Taxonomy == nil {
		return TaxonomyResult{Categories: []CategoryCandidate{}, Skills: []SkillCandidate{}, Usage: estimateUsage(text)}, nil
	}
	categories, skills, err := p.Taxonomy.Candidates(ctx)
	if err != nil {
		return TaxonomyResult{}, err
	}
	score := func(slug, name string) float64 {
		words := strings.Fields(strings.NewReplacer("-", " ", "_", " ").Replace(strings.ToLower(slug + " " + name)))
		hits := 0
		for _, w := range words {
			if len([]rune(w)) >= 2 && strings.Contains(text, w) {
				hits++
			}
		}
		if hits == 0 {
			return 0
		}
		return min(0.99, 0.55+float64(hits)*0.12)
	}
	for i := range categories {
		categories[i].Confidence = score(categories[i].Slug, categories[i].Name)
	}
	for i := range skills {
		skills[i].Confidence = score(skills[i].Slug, skills[i].Name)
	}
	categories = topCategories(categories, 5)
	skills = topSkills(skills, 10)
	return TaxonomyResult{Categories: categories, Skills: skills, Usage: estimateUsage(text)}, nil
}
func (p DeterministicProvider) RerankCandidates(_ context.Context, req MatchRerankRequest) (MatchRerankResult, error) {
	if len(req.Candidates) < 1 || len(req.Candidates) > 20 {
		return MatchRerankResult{}, ErrInvalidInput
	}
	ids := make([]string, 0, len(req.Candidates))
	for _, candidate := range req.Candidates {
		if candidate.ID == "" {
			return MatchRerankResult{}, ErrInvalidInput
		}
		ids = append(ids, candidate.ID)
	}
	return MatchRerankResult{OrderedIDs: ids, ReasonCodes: map[string][]string{}, Usage: estimateUsage(req.ProjectTitle + req.ProjectDescription)}, nil
}

func (p DeterministicProvider) baseline(slug string) Range {
	if value, ok := p.Baselines[slug]; ok {
		return value
	}
	if value, ok := p.Baselines["default"]; ok {
		return value
	}
	return Range{MinKopecks: 5_000_000, MaxKopecks: 12_000_000, Currency: "RUB", Confidence: "LOW"}
}
func clean(value string) string { return strings.Join(strings.Fields(strings.TrimSpace(value)), " ") }
func firstSentence(value string, max int) string {
	parts := sentences(value, 1)
	if len(parts) == 0 {
		return "Новый проект"
	}
	r := []rune(parts[0])
	if len(r) > max {
		r = r[:max]
	}
	return strings.TrimSpace(string(r))
}
func sentences(value string, limit int) []string {
	split := regexp.MustCompile(`[.!?\n]+`).Split(value, -1)
	out := []string{}
	for _, v := range split {
		v = clean(v)
		if v != "" {
			out = append(out, v)
			if len(out) == limit {
				break
			}
		}
	}
	return out
}
func missingQuestions(value string) []string {
	lower := strings.ToLower(value)
	out := []string{}
	if !strings.Contains(lower, "срок") {
		out = append(out, "Какой желаемый срок?")
	}
	if !strings.Contains(lower, "бюджет") && !strings.Contains(lower, "₽") {
		out = append(out, "Какой ориентир по бюджету?")
	}
	return out
}
func moneyLines(value string) []string {
	out := []string{}
	for _, s := range sentences(value, 20) {
		if strings.Contains(s, "₽") || strings.Contains(strings.ToLower(s), "руб") {
			out = append(out, s)
		}
	}
	return out
}
func estimateUsage(value string) Usage {
	return Usage{InputTokens: max(1, len([]rune(value))/4), OutputTokens: max(1, len([]rune(value))/12)}
}
func round(value int64) int64 { const step int64 = 100_000; return ((value + step/2) / step) * step }
func firstCategory(values []CategoryCandidate) string {
	if len(values) > 0 {
		return values[0].Slug
	}
	return "default"
}
func topCategories(values []CategoryCandidate, n int) []CategoryCandidate {
	sort.Slice(values, func(i, j int) bool { return values[i].Confidence > values[j].Confidence })
	out := []CategoryCandidate{}
	for _, v := range values {
		if v.Confidence > 0 {
			out = append(out, v)
			if len(out) == n {
				break
			}
		}
	}
	return out
}
func topSkills(values []SkillCandidate, n int) []SkillCandidate {
	sort.Slice(values, func(i, j int) bool { return values[i].Confidence > values[j].Confidence })
	out := []SkillCandidate{}
	for _, v := range values {
		if v.Confidence > 0 {
			out = append(out, v)
			if len(out) == n {
				break
			}
		}
	}
	return out
}
func validateBrief(v ProjectBriefResult) error {
	if clean(v.Title) == "" || clean(v.Summary) == "" || validateRange(v.Budget) != nil || v.DurationDays.Min < 1 || v.DurationDays.Max < v.DurationDays.Min {
		return ErrInvalidOutput
	}
	return nil
}
func validateImport(v ProjectImportResult) error {
	if clean(v.Title) == "" || clean(v.Scope) == "" {
		return ErrInvalidOutput
	}
	return nil
}
func validateEstimate(v EstimateResult) error {
	if validateRange(v.Range) != nil || v.DurationDays.Min < 1 || v.DurationDays.Max < v.DurationDays.Min {
		return ErrInvalidOutput
	}
	return nil
}
func validateOffer(v OfferAnalysisResult) error {
	if validateRange(v.Benchmark) != nil || v.Confidence == "" {
		return ErrInvalidOutput
	}
	return nil
}
func validateRange(v Range) error {
	if v.MinKopecks <= 0 || v.MaxKopecks < v.MinKopecks || v.Currency != "RUB" || (v.Confidence != "LOW" && v.Confidence != "MEDIUM" && v.Confidence != "HIGH") {
		return ErrInvalidOutput
	}
	return nil
}
