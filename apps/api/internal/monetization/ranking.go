package monetization

import "math"

// MaxDiscoveryBoostPoints mirrors profiles.MaxProBoostPoints so entitlement
// configuration stays bounded even when plan config is edited aggressively.
const MaxDiscoveryBoostPoints = 12.0

// DiscoveryBoostPoints converts search.priority_visibility config into an
// additive ranking signal. Disabled entitlements and multipliers <= 1 yield 0.
func DiscoveryBoostPoints(enabled bool, rankingMultiplier float64) float64 {
	if !enabled || rankingMultiplier <= 1 {
		return 0
	}
	boost := (rankingMultiplier - 1) * 100
	if boost > MaxDiscoveryBoostPoints {
		return MaxDiscoveryBoostPoints
	}
	return math.Round(boost)
}
