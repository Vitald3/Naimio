package ai

import (
	"context"
	"encoding/json"
	"strings"
)

type Service struct {
	Runner   Runner
	Fallback AIProvider
	Drafts   DraftRepository
	Taxonomy TaxonomyRepository
}

func (s Service) CreateDraft(ctx context.Context, actor, source string, raw map[string]any) (Draft, string, error) {
	if source != "AI_BRIEF" && source != "IMPORT" && source != "COMMERCIAL_OFFER" && source != "CALCULATOR" && source != "MANUAL" {
		return Draft{}, "", ErrInvalidInput
	}
	if jsonSize(raw) > 128<<10 {
		return Draft{}, "", ErrInvalidInput
	}
	return s.Drafts.Create(ctx, actor, source, raw)
}
func (s Service) GetDraft(ctx context.Context, actor, token string) (Draft, error) {
	if !validToken(token) {
		return Draft{}, ErrNotFound
	}
	return s.Drafts.Get(ctx, actor, token)
}
func (s Service) UpdateDraft(ctx context.Context, actor, token string, raw, normalized map[string]any) (Draft, error) {
	if !validToken(token) || jsonSize(raw)+jsonSize(normalized) > 128<<10 {
		return Draft{}, ErrInvalidInput
	}
	return s.Drafts.Update(ctx, actor, token, raw, normalized)
}
func (s Service) ClaimDraft(ctx context.Context, actor, token string) (Draft, error) {
	if !validToken(token) {
		return Draft{}, ErrNotFound
	}
	return s.Drafts.Claim(ctx, actor, token)
}

func (s Service) Brief(ctx context.Context, actor, token, text string) (ProjectBriefResult, Draft, error) {
	if !validText(text, s.maxInput(ProjectBrief, 30000)) {
		return ProjectBriefResult{}, Draft{}, ErrInvalidInput
	}
	value, err := s.Runner.Run(ctx, actor, ProjectBrief, func(ctx context.Context, p AIProvider) (callResult, error) {
		result, err := p.GenerateProjectBrief(ctx, BriefRequest{Text: text})
		if err == nil && !s.outputAllowed(ProjectBrief, result) {
			err = ErrInvalidOutput
		}
		if err == nil {
			err = validateBrief(result)
		}
		if err == nil {
			err = s.validateTaxonomy(ctx, result.Categories, result.Skills)
		}
		return callResult{Value: result, Usage: result.Usage}, err
	})
	result, ok := value.(ProjectBriefResult)
	if err != nil || !ok {
		result, err = s.Fallback.GenerateProjectBrief(ctx, BriefRequest{Text: text})
	}
	if err == nil {
		err = validateBrief(result)
	}
	if err == nil {
		err = s.validateTaxonomy(ctx, result.Categories, result.Skills)
	}
	if err == nil && !s.outputAllowed(ProjectBrief, result) {
		err = ErrInvalidOutput
	}
	if err != nil {
		return ProjectBriefResult{}, Draft{}, err
	}
	draft, err := s.save(ctx, actor, token, map[string]any{"text": text}, result)
	return result, draft, err
}
func (s Service) Import(ctx context.Context, actor, token string, materials []Material) (ProjectImportResult, Draft, error) {
	if !validMaterials(materials, s.maxInput(ProjectImport, 100000)) {
		return ProjectImportResult{}, Draft{}, ErrInvalidInput
	}
	value, err := s.Runner.Run(ctx, actor, ProjectImport, func(ctx context.Context, p AIProvider) (callResult, error) {
		result, err := p.ExtractProject(ctx, ImportRequest{Materials: materials})
		if err == nil && !s.outputAllowed(ProjectImport, result) {
			err = ErrInvalidOutput
		}
		if err == nil {
			err = validateImport(result)
		}
		if err == nil {
			err = s.validateTaxonomy(ctx, result.Categories, result.Skills)
		}
		return callResult{Value: result, Usage: result.Usage}, err
	})
	result, ok := value.(ProjectImportResult)
	if err != nil || !ok {
		result, err = s.Fallback.ExtractProject(ctx, ImportRequest{Materials: materials})
	}
	if err == nil {
		err = validateImport(result)
	}
	if err == nil {
		err = s.validateTaxonomy(ctx, result.Categories, result.Skills)
	}
	if err == nil && !s.outputAllowed(ProjectImport, result) {
		err = ErrInvalidOutput
	}
	if err != nil {
		return ProjectImportResult{}, Draft{}, err
	}
	draft, err := s.save(ctx, actor, token, map[string]any{"materials": materials}, result)
	return result, draft, err
}
func (s Service) Estimate(ctx context.Context, actor, token string, req EstimateRequest) (EstimateResult, Draft, error) {
	if !validText(req.Text, s.maxInput(ProjectEstimate, 30000)) || len(req.SkillSlugs) > 30 || len(req.Features) > 30 {
		return EstimateResult{}, Draft{}, ErrInvalidInput
	}
	baseline, baselineErr := s.Fallback.EstimateProject(ctx, req)
	if baselineErr != nil {
		return EstimateResult{}, Draft{}, baselineErr
	}
	value, err := s.Runner.Run(ctx, actor, ProjectEstimate, func(ctx context.Context, p AIProvider) (callResult, error) {
		result, err := p.EstimateProject(ctx, req)
		if err == nil && !s.outputAllowed(ProjectEstimate, result) {
			err = ErrInvalidOutput
		}
		if err == nil {
			err = validateEstimate(result)
		}
		return callResult{Value: result, Usage: result.Usage}, err
	})
	result, ok := value.(EstimateResult)
	if err != nil || !ok {
		result, err = baseline, nil
	} else {
		result = combineEstimate(baseline, result)
	}
	if err == nil {
		err = validateEstimate(result)
	}
	if err == nil && !s.outputAllowed(ProjectEstimate, result) {
		err = ErrInvalidOutput
	}
	if err != nil {
		return EstimateResult{}, Draft{}, err
	}
	draft, err := s.save(ctx, actor, token, map[string]any{"text": req.Text, "category_slug": req.CategorySlug, "skill_slugs": req.SkillSlugs, "features": req.Features}, result)
	return result, draft, err
}
func (s Service) AnalyzeOffer(ctx context.Context, actor, token, text, goal string) (OfferAnalysisResult, Draft, error) {
	if !validText(text, s.maxInput(OfferAnalysis, 50000)) || len([]rune(goal)) > 2000 {
		return OfferAnalysisResult{}, Draft{}, ErrInvalidInput
	}
	req := OfferRequest{Text: text, Goal: goal}
	value, err := s.Runner.Run(ctx, actor, OfferAnalysis, func(ctx context.Context, p AIProvider) (callResult, error) {
		result, err := p.AnalyzeCommercialOffer(ctx, req)
		if err == nil && !s.outputAllowed(OfferAnalysis, result) {
			err = ErrInvalidOutput
		}
		if err == nil {
			err = validateOffer(result)
		}
		return callResult{Value: result, Usage: result.Usage}, err
	})
	result, ok := value.(OfferAnalysisResult)
	if err != nil || !ok {
		result, err = s.Fallback.AnalyzeCommercialOffer(ctx, req)
	}
	if err == nil {
		err = validateOffer(result)
	}
	if err == nil && !s.outputAllowed(OfferAnalysis, result) {
		err = ErrInvalidOutput
	}
	if err != nil {
		return OfferAnalysisResult{}, Draft{}, err
	}
	draft, err := s.save(ctx, actor, token, map[string]any{"text": text, "goal": goal}, result)
	return result, draft, err
}
func (s Service) Suggest(ctx context.Context, actor, text string) (TaxonomyResult, error) {
	if !validText(text, s.maxInput(TaxonomySuggestion, 30000)) {
		return TaxonomyResult{}, ErrInvalidInput
	}
	value, err := s.Runner.Run(ctx, actor, TaxonomySuggestion, func(ctx context.Context, p AIProvider) (callResult, error) {
		result, err := p.SuggestTaxonomy(ctx, TaxonomyRequest{Text: text})
		if err == nil && !s.outputAllowed(TaxonomySuggestion, result) {
			err = ErrInvalidOutput
		}
		if err == nil {
			err = s.validateTaxonomy(ctx, result.Categories, result.Skills)
		}
		return callResult{Value: result, Usage: result.Usage}, err
	})
	result, ok := value.(TaxonomyResult)
	if err != nil || !ok {
		result, err = s.Fallback.SuggestTaxonomy(ctx, TaxonomyRequest{Text: text})
	}
	if err == nil {
		err = s.validateTaxonomy(ctx, result.Categories, result.Skills)
	}
	if err == nil && !s.outputAllowed(TaxonomySuggestion, result) {
		err = ErrInvalidOutput
	}
	return result, err
}
func (s Service) Rerank(ctx context.Context, actor string, req MatchRerankRequest) (MatchRerankResult, error) {
	if len(req.Candidates) < 1 || len(req.Candidates) > 20 || len([]rune(req.ProjectDescription)) > 1000 {
		return MatchRerankResult{}, ErrInvalidInput
	}
	value, err := s.Runner.Run(ctx, actor, MatchRerank, func(ctx context.Context, p AIProvider) (callResult, error) {
		result, err := p.RerankCandidates(ctx, req)
		if err == nil {
			err = validateMatchRerank(req, result)
		}
		return callResult{Value: result, Usage: result.Usage}, err
	})
	result, ok := value.(MatchRerankResult)
	if err != nil || !ok {
		return MatchRerankResult{}, err
	}
	return result, nil
}
func validateMatchRerank(req MatchRerankRequest, result MatchRerankResult) error {
	if len(result.OrderedIDs) != len(req.Candidates) {
		return ErrInvalidOutput
	}
	known := map[string]bool{}
	for _, candidate := range req.Candidates {
		known[candidate.ID] = true
	}
	seen := map[string]bool{}
	for _, id := range result.OrderedIDs {
		if !known[id] || seen[id] {
			return ErrInvalidOutput
		}
		seen[id] = true
		for _, code := range result.ReasonCodes[id] {
			if code != "STRONG_SCOPE_FIT" {
				return ErrInvalidOutput
			}
		}
	}
	return nil
}
func (s Service) save(ctx context.Context, actor, token string, raw map[string]any, result any) (Draft, error) {
	if !validToken(token) {
		return Draft{}, ErrInvalidInput
	}
	encoded, _ := json.Marshal(result)
	normalized := map[string]any{}
	_ = json.Unmarshal(encoded, &normalized)
	if current, err := s.Drafts.Get(ctx, actor, token); err == nil {
		merged := make(map[string]any, len(current.RawInput)+len(raw))
		for key, value := range current.RawInput {
			merged[key] = value
		}
		for key, value := range raw {
			merged[key] = value
		}
		raw = merged
	}
	return s.Drafts.Update(ctx, actor, token, raw, normalized)
}
func (s Service) validateTaxonomy(ctx context.Context, categories []CategoryCandidate, skills []SkillCandidate) error {
	if s.Taxonomy == nil && len(categories)+len(skills) == 0 {
		return nil
	}
	if s.Taxonomy == nil {
		return ErrInvalidOutput
	}
	knownCategories, knownSkills, err := s.Taxonomy.Candidates(ctx)
	if err != nil {
		return err
	}
	categoryIDs, skillIDs := map[string]bool{}, map[string]bool{}
	for _, item := range knownCategories {
		categoryIDs[item.ID] = true
	}
	for _, item := range knownSkills {
		skillIDs[item.ID] = true
	}
	if len(categories) > 5 || len(skills) > 10 {
		return ErrInvalidOutput
	}
	for _, item := range categories {
		if !categoryIDs[item.ID] || item.Confidence < 0 || item.Confidence > 1 {
			return ErrInvalidOutput
		}
	}
	for _, item := range skills {
		if !skillIDs[item.ID] || item.Confidence < 0 || item.Confidence > 1 {
			return ErrInvalidOutput
		}
	}
	return nil
}
func (s Service) maxInput(capability Capability, fallback int) int {
	if value, ok := s.Runner.Config[capability]; ok && value.MaxInputChars > 0 {
		return value.MaxInputChars
	}
	return fallback
}
func (s Service) outputAllowed(capability Capability, value any) bool {
	config, ok := s.Runner.Config[capability]
	return !ok || config.MaxOutputTokens <= 0 || jsonSize(value) <= config.MaxOutputTokens*8
}
func combineEstimate(baseline, classified EstimateResult) EstimateResult {
	clamp := func(value, minimum, maximum int64) int64 {
		if value < minimum {
			return minimum
		}
		if value > maximum {
			return maximum
		}
		return value
	}
	classified.Range.MinKopecks = clamp(classified.Range.MinKopecks, baseline.Range.MinKopecks/2, baseline.Range.MinKopecks*2)
	classified.Range.MaxKopecks = clamp(classified.Range.MaxKopecks, baseline.Range.MaxKopecks/2, baseline.Range.MaxKopecks*2)
	baseline.Range.MinKopecks = round((baseline.Range.MinKopecks + classified.Range.MinKopecks) / 2)
	baseline.Range.MaxKopecks = round((baseline.Range.MaxKopecks + classified.Range.MaxKopecks) / 2)
	if baseline.Range.MaxKopecks < baseline.Range.MinKopecks {
		baseline.Range.MaxKopecks = baseline.Range.MinKopecks
	}
	baseline.DurationDays = DurationRange{Min: max(1, (baseline.DurationDays.Min+classified.DurationDays.Min)/2), Max: max(1, (baseline.DurationDays.Max+classified.DurationDays.Max)/2)}
	baseline.Drivers = append(baseline.Drivers, classified.Drivers...)
	baseline.Assumptions = append(baseline.Assumptions, classified.Assumptions...)
	baseline.Usage = classified.Usage
	return baseline
}
func validToken(value string) bool {
	return len(value) == 64 && strings.IndexFunc(value, func(r rune) bool { return !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') }) == -1
}
func validText(value string, max int) bool {
	n := len([]rune(strings.TrimSpace(value)))
	return n >= 3 && n <= max
}
func validMaterials(values []Material, maxTotal int) bool {
	if len(values) < 1 || len(values) > 10 {
		return false
	}
	total := 0
	for _, value := range values {
		if len([]rune(value.Name)) > 200 || !validText(value.Text, 50000) {
			return false
		}
		total += len([]rune(value.Text))
	}
	return total <= maxTotal
}
func jsonSize(value any) int { encoded, _ := json.Marshal(value); return len(encoded) }
