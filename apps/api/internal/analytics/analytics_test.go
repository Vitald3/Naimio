package analytics

import (
	"context"
	"testing"
	"time"

	"freelance/apps/api/internal/monetization"
)

type fakeEntitlements struct {
	enabled, unlocked bool
}

func (f fakeEntitlements) HasAnalytics(context.Context, string) (bool, bool, error) {
	return f.enabled, f.unlocked, nil
}

func TestTrackSkipsSelfViewsAndDedupes(t *testing.T) {
	repo := &MemoryRepository{}
	service := Service{Repository: repo, Now: func() time.Time { return time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC) }}
	subject := "11111111-1111-4111-8111-111111111111"
	viewer := "22222222-2222-4222-8222-222222222222"
	if err := service.Track(context.Background(), EventInput{SubjectUserID: subject, ViewerUserID: subject, EventType: EventProfileView}); err != nil {
		t.Fatal(err)
	}
	if len(repo.Events) != 0 {
		t.Fatalf("self view recorded: %#v", repo.Events)
	}
	in := EventInput{SubjectUserID: subject, ViewerUserID: viewer, EventType: EventProfileView}
	if err := service.Track(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if err := service.Track(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if len(repo.Events) != 1 {
		t.Fatalf("dedupe failed: %#v", repo.Events)
	}
}

func TestMineLocksAdvancedForFree(t *testing.T) {
	subject := "11111111-1111-4111-8111-111111111111"
	repo := &MemoryRepository{
		Events:    []EventInput{{SubjectUserID: subject, EventType: EventProfileView, DayKey: "2026-08-14"}},
		Proposals: map[string]int64{subject: 2},
	}
	service := Service{Repository: repo, Entitlements: fakeEntitlements{enabled: true, unlocked: false}}
	metrics, err := service.Mine(context.Background(), subject)
	if err != nil || metrics.AdvancedUnlocked || metrics.ProfileViews != nil || metrics.ProposalsSent != 2 {
		t.Fatalf("metrics=%#v err=%v", metrics, err)
	}
	if len(metrics.LockedAdvancedMetrics) == 0 {
		t.Fatal("expected locked metrics")
	}
}

func TestMineUnlocksAdvancedForPRO(t *testing.T) {
	subject := "11111111-1111-4111-8111-111111111111"
	repo := &MemoryRepository{
		Events: []EventInput{
			{SubjectUserID: subject, EventType: EventProfileView, DayKey: "a"},
			{SubjectUserID: subject, EventType: EventProfileView, DayKey: "b"},
			{SubjectUserID: subject, EventType: EventPortfolioView, DayKey: "a"},
		},
		Proposals: map[string]int64{subject: 1},
	}
	service := Service{Repository: repo, Entitlements: fakeEntitlements{enabled: true, unlocked: true}}
	metrics, err := service.Mine(context.Background(), subject)
	if err != nil || !metrics.AdvancedUnlocked || metrics.ProfileViews == nil || *metrics.ProfileViews != 2 {
		t.Fatalf("metrics=%#v err=%v", metrics, err)
	}
	if metrics.ProfileToProposalRate == nil || *metrics.ProfileToProposalRate != 0.5 {
		t.Fatalf("rate=%v", metrics.ProfileToProposalRate)
	}
}

func TestEntitlementBridgeUsesResolvedCapabilities(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	free := monetization.Plan{ID: "free", Tier: "FREE", Active: true}
	pro := monetization.Plan{ID: "pro", Tier: "PRO", Active: true}
	repo := &stubMono{
		enabled: true,
		plans:   []monetization.Plan{free, pro},
		features: map[string][]monetization.Entitlement{
			"free": {{FeatureKey: monetization.FeatureProfileAnalytics, Kind: "BOOLEAN", Enabled: false, Config: map[string]any{}}},
			"pro":  {{FeatureKey: monetization.FeatureProfileAnalytics, Kind: "BOOLEAN", Enabled: true, Config: map[string]any{}}},
		},
		current: &monetization.Subscription{PlanID: "pro", Status: "ACTIVE", StartsAt: now.Add(-time.Hour), CurrentPeriodEnd: now.Add(time.Hour)},
	}
	bridge := EntitlementBridge{Service: monetization.Service{Repository: repo, Now: func() time.Time { return now }}}
	enabled, unlocked, err := bridge.HasAnalytics(context.Background(), "u")
	if err != nil || !enabled || !unlocked {
		t.Fatalf("enabled=%v unlocked=%v err=%v", enabled, unlocked, err)
	}
}

type stubMono struct {
	enabled  bool
	plans    []monetization.Plan
	features map[string][]monetization.Entitlement
	current  *monetization.Subscription
}

func (s *stubMono) FeatureEnabled(context.Context, string) (bool, error) { return s.enabled, nil }
func (s *stubMono) IsAdmin(context.Context, string) (bool, error)        { return false, nil }
func (s *stubMono) ListPlans(context.Context, bool) ([]monetization.Plan, error) {
	return s.plans, nil
}
func (s *stubMono) GetPlan(_ context.Context, id string) (monetization.Plan, error) {
	for _, plan := range s.plans {
		if plan.ID == id {
			return plan, nil
		}
	}
	return monetization.Plan{}, monetization.ErrNotFound
}
func (s *stubMono) PlanEntitlements(_ context.Context, id string) ([]monetization.Entitlement, error) {
	return s.features[id], nil
}
func (s *stubMono) CurrentSubscription(context.Context, string) (*monetization.Subscription, error) {
	return s.current, nil
}
func (s *stubMono) SubscriptionHistory(context.Context, string) ([]monetization.Subscription, error) {
	return nil, nil
}
func (s *stubMono) SubscriptionEvents(context.Context, string) ([]monetization.Event, error) {
	return nil, nil
}
func (s *stubMono) Overview(context.Context) (monetization.Overview, error) {
	return monetization.Overview{}, nil
}
func (s *stubMono) ListSubscriptions(context.Context, string, int) ([]monetization.Subscription, error) {
	return nil, nil
}
func (s *stubMono) Grant(context.Context, string, string, string, time.Time, time.Time, string, string) (monetization.Subscription, error) {
	return monetization.Subscription{}, nil
}
func (s *stubMono) Transition(context.Context, string, string, string, string, string) (monetization.Subscription, error) {
	return monetization.Subscription{}, nil
}
func (s *stubMono) UpdatePlan(context.Context, string, monetization.Plan, string, string) (monetization.Plan, error) {
	return monetization.Plan{}, nil
}
func (s *stubMono) SetEntitlement(context.Context, string, string, monetization.Entitlement, string, string) (monetization.Entitlement, error) {
	return monetization.Entitlement{}, nil
}
