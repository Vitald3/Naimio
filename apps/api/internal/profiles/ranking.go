package profiles

import (
	"math"
	"strings"
)

// MaxProBoostPoints caps the additive PRO visibility signal so relevance can
// still dominate when candidates differ materially.
const MaxProBoostPoints = 12.0

// BrowseRelevanceBaseline is used when the catalog is browsed without a query.
const BrowseRelevanceBaseline = 40.0

// DiscoveryScore combines relevance, quality/reputation and an optional bounded
// PRO visibility boost. PRO is never a hard rank override.
func DiscoveryScore(relevance, quality, proBoost float64) float64 {
	if relevance < 0 {
		relevance = 0
	}
	if quality < 0 {
		quality = 0
	}
	if proBoost < 0 {
		proBoost = 0
	}
	if proBoost > MaxProBoostPoints {
		proBoost = MaxProBoostPoints
	}
	return relevance + quality + proBoost
}

// BoundedProBoost converts a configurable ranking_multiplier into additive
// points. Multiplier 1.08 → 8 points. Values at or below 1 disable the boost.
func BoundedProBoost(enabled bool, rankingMultiplier float64) float64 {
	if !enabled || rankingMultiplier <= 1 {
		return 0
	}
	boost := (rankingMultiplier - 1) * 100
	if boost > MaxProBoostPoints {
		return MaxProBoostPoints
	}
	return math.Round(boost)
}

// QualityScore maps native trust signals into a bounded quality component.
// Rating 0–5 contributes up to 40; completed projects contribute up to 10.
func QualityScore(nativeRating *float64, completedProjects int) float64 {
	rating := 0.0
	if nativeRating != nil {
		rating = math.Max(0, math.Min(5, *nativeRating))
	}
	completed := float64(completedProjects)
	if completed < 0 {
		completed = 0
	}
	if completed > 20 {
		completed = 20
	}
	return rating*8 + completed*0.5
}

// TextRelevanceScore estimates text-match strength without corrupting full-text
// filters. Strong title/name matches outrank weak bio-only matches.
func TextRelevanceScore(query, displayName, title, bio string) float64 {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return BrowseRelevanceBaseline
	}
	score := 20.0
	name := strings.ToLower(displayName)
	t := strings.ToLower(title)
	b := strings.ToLower(bio)
	if name == q || t == q {
		score += 70
	} else if strings.HasPrefix(name, q) || strings.HasPrefix(t, q) {
		score += 55
	} else if strings.Contains(name, q) || strings.Contains(t, q) {
		score += 45
	} else if strings.Contains(b, q) {
		score += 25
	}
	if score > 100 {
		return 100
	}
	return score
}
