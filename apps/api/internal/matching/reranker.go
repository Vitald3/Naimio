package matching

import (
	"context"
	"freelance/apps/api/internal/ai"
)

type AIReranker struct{ Service ai.Service }

func (r AIReranker) Rerank(ctx context.Context, actor string, req RerankRequest) (RerankResult, error) {
	candidates := make([]ai.MatchCandidate, 0, len(req.Candidates))
	for _, item := range req.Candidates {
		candidates = append(candidates, ai.MatchCandidate{ID: item.ID, DeterministicScore: item.DeterministicScore, SkillMatches: item.SkillMatches, RequiredSkills: item.RequiredSkills, Availability: item.Availability, NativeRating: item.NativeRating, Reviews: item.Reviews})
	}
	result, err := r.Service.Rerank(ctx, actor, ai.MatchRerankRequest{ProjectTitle: req.ProjectTitle, ProjectDescription: req.ProjectDescription, Candidates: candidates})
	return RerankResult{OrderedIDs: result.OrderedIDs, ReasonCodes: result.ReasonCodes}, err
}
