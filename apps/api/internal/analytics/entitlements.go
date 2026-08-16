package analytics

import (
	"context"

	"freelance/apps/api/internal/monetization"
)

type EntitlementBridge struct {
	Service monetization.Service
}

func (b EntitlementBridge) HasAnalytics(ctx context.Context, userID string) (bool, bool, error) {
	caps, err := b.Service.Resolve(ctx, userID)
	if err != nil {
		return false, false, err
	}
	return caps.ProSystemEnabled, caps.Has(monetization.FeatureProfileAnalytics), nil
}
