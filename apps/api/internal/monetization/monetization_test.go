package monetization

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepo struct {
	enabled, admin bool
	plans          []Plan
	current        *Subscription
	features       map[string][]Entitlement
	grants         int
}

func (f *fakeRepo) FeatureEnabled(context.Context, string) (bool, error) { return f.enabled, nil }
func (f *fakeRepo) IsAdmin(context.Context, string) (bool, error)        { return f.admin, nil }
func (f *fakeRepo) ListPlans(context.Context, bool) ([]Plan, error)      { return f.plans, nil }
func (f *fakeRepo) GetPlan(_ context.Context, id string) (Plan, error) {
	for _, p := range f.plans {
		if p.ID == id {
			return p, nil
		}
	}
	return Plan{}, ErrNotFound
}
func (f *fakeRepo) PlanEntitlements(_ context.Context, id string) ([]Entitlement, error) {
	return f.features[id], nil
}
func (f *fakeRepo) CurrentSubscription(context.Context, string) (*Subscription, error) {
	return f.current, nil
}
func (f *fakeRepo) SubscriptionHistory(context.Context, string) ([]Subscription, error) {
	return nil, nil
}
func (f *fakeRepo) SubscriptionEvents(context.Context, string) ([]Event, error) { return nil, nil }
func (f *fakeRepo) Overview(context.Context) (Overview, error)                  { return Overview{}, nil }
func (f *fakeRepo) ListSubscriptions(context.Context, string, int) ([]Subscription, error) {
	return nil, nil
}
func (f *fakeRepo) Grant(_ context.Context, actor, user, plan string, start, end time.Time, reason, request string) (Subscription, error) {
	f.grants++
	return Subscription{ID: "s", UserID: user, PlanID: plan, Status: "ACTIVE", StartsAt: start, CurrentPeriodStart: start, CurrentPeriodEnd: end}, nil
}
func (f *fakeRepo) Transition(context.Context, string, string, string, string, string) (Subscription, error) {
	return Subscription{}, nil
}
func (f *fakeRepo) UpdatePlan(context.Context, string, Plan, string, string) (Plan, error) {
	return Plan{}, nil
}
func (f *fakeRepo) SetEntitlement(context.Context, string, string, Entitlement, string, string) (Entitlement, error) {
	return Entitlement{}, nil
}
func limit(v int64) Entitlement {
	return Entitlement{FeatureKey: FeaturePortfolioItems, Kind: "LIMIT", Enabled: true, LimitValue: &v, Config: map[string]any{}}
}
func TestResolveUsesEffectiveActiveSubscription(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	free := Plan{ID: "free", Tier: "FREE", Active: true}
	pro := Plan{ID: "pro", Tier: "PRO", Active: true}
	repo := &fakeRepo{enabled: true, plans: []Plan{free, pro}, features: map[string][]Entitlement{"free": {limit(8)}, "pro": {{FeatureKey: FeatureProBadge, Kind: "BOOLEAN", Enabled: true}, limit(40)}}, current: &Subscription{PlanID: "pro", Status: "ACTIVE", StartsAt: now.Add(-time.Hour), CurrentPeriodEnd: now.Add(time.Hour)}}
	caps, err := (Service{Repository: repo, Now: func() time.Time { return now }}).Resolve(context.Background(), "u")
	if err != nil || !caps.EffectivePro || !caps.Has(FeatureProBadge) {
		t.Fatalf("caps=%#v err=%v", caps, err)
	}
	got, _, _ := caps.Limit(FeaturePortfolioItems)
	if got != 40 {
		t.Fatalf("limit=%d", got)
	}
}
func TestResolveExpiredAndDisabledFallBackToFree(t *testing.T) {
	now := time.Now().UTC()
	free := Plan{ID: "free", Tier: "FREE", Active: true}
	pro := Plan{ID: "pro", Tier: "PRO", Active: true}
	repo := &fakeRepo{enabled: true, plans: []Plan{free, pro}, features: map[string][]Entitlement{"free": {limit(8)}}, current: &Subscription{PlanID: "pro", Status: "ACTIVE", StartsAt: now.Add(-2 * time.Hour), CurrentPeriodEnd: now.Add(-time.Hour)}}
	service := Service{Repository: repo, Now: func() time.Time { return now }}
	caps, err := service.Resolve(context.Background(), "u")
	if err != nil || caps.EffectivePro || caps.Plan.ID != "free" {
		t.Fatalf("caps=%#v err=%v", caps, err)
	}
	repo.enabled = false
	repo.current.CurrentPeriodEnd = now.Add(time.Hour)
	caps, _ = service.Resolve(context.Background(), "u")
	if caps.EffectivePro || caps.ProSystemEnabled {
		t.Fatalf("disabled caps=%#v", caps)
	}
}
func TestAdminGrantCannotBeSelfActivated(t *testing.T) {
	repo := &fakeRepo{plans: []Plan{{ID: "pro", Tier: "PRO", Active: true}}}
	service := Service{Repository: repo}
	_, err := service.Grant(context.Background(), "ordinary", "u", "pro", time.Now(), time.Now().Add(time.Hour), "reason", "r")
	if !errors.Is(err, ErrForbidden) || repo.grants != 0 {
		t.Fatalf("err=%v grants=%d", err, repo.grants)
	}
	repo.admin = true
	_, err = service.Grant(context.Background(), "admin", "u", "pro", time.Now(), time.Now().Add(time.Hour), "reason", "r")
	if err != nil || repo.grants != 1 {
		t.Fatalf("err=%v grants=%d", err, repo.grants)
	}
}
func TestDisabledProviderNeverFakesCheckout(t *testing.T) {
	_, err := (DisabledProvider{}).CreateCheckout(context.Background(), CheckoutRequest{})
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("err=%v", err)
	}
}

func TestRankingMultiplierAndDiscoveryBoost(t *testing.T) {
	caps := Capabilities{Features: map[string]Entitlement{
		FeatureSearchPriority: {FeatureKey: FeatureSearchPriority, Kind: "BOOLEAN", Enabled: true, Config: map[string]any{"ranking_multiplier": 1.08}},
	}}
	mult, ok := caps.RankingMultiplier()
	if !ok || mult != 1.08 {
		t.Fatalf("mult=%v ok=%v", mult, ok)
	}
	if got := DiscoveryBoostPoints(true, mult); got != 8 {
		t.Fatalf("boost=%v", got)
	}
	if got := DiscoveryBoostPoints(false, mult); got != 0 {
		t.Fatalf("disabled boost=%v", got)
	}
}
