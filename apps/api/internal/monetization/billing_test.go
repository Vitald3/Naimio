package monetization

import (
	"context"
	"errors"
	"testing"
	"time"

	"freelance/apps/api/internal/payments"
)

type billingFakeRepo struct {
	*fakeRepo
	pending      Subscription
	attempts     []payments.Attempt
	activated    int
	renewed      int
	pastDue      int
	failed       int
	cancelled    int
	savedMethod  string
	renewalItems []Subscription
	plansByID    map[string]Plan
}

func (r *billingFakeRepo) CreatePendingSubscription(_ context.Context, userID string, plan Plan, provider payments.ProviderName, start, end time.Time) (Subscription, error) {
	if r.pending.ID != "" {
		return r.pending, nil
	}
	r.pending = Subscription{ID: "11111111-1111-1111-1111-111111111111", UserID: userID, PlanID: plan.ID, PlanName: plan.Name, Status: "PENDING", StartsAt: start, CurrentPeriodStart: start, CurrentPeriodEnd: end, Provider: string(provider)}
	return r.pending, nil
}
func (r *billingFakeRepo) SavePaymentMethod(_ context.Context, _ string, _ string, method string) error {
	r.savedMethod = method
	r.pending.PaymentMethodRef = method
	return nil
}
func (r *billingFakeRepo) ActivatePaidSubscription(_ context.Context, id, _ string, start, end time.Time, provider string) (Subscription, error) {
	r.activated++
	r.pending.ID, r.pending.Status, r.pending.StartsAt, r.pending.CurrentPeriodStart, r.pending.CurrentPeriodEnd, r.pending.Provider = id, "ACTIVE", start, start, end, provider
	return r.pending, nil
}
func (r *billingFakeRepo) RenewPaidSubscription(_ context.Context, _ string, _ string, start, end time.Time) (Subscription, error) {
	r.renewed++
	r.pending.Status, r.pending.CurrentPeriodStart, r.pending.CurrentPeriodEnd = "ACTIVE", start, end
	return r.pending, nil
}
func (r *billingFakeRepo) MarkSubscriptionPastDue(context.Context, string, string) error {
	r.pastDue++
	r.pending.Status = "PAST_DUE"
	return nil
}
func (r *billingFakeRepo) FailInitialSubscription(context.Context, string, string, string) error {
	r.failed++
	r.pending.Status = "CANCELED"
	return nil
}
func (r *billingFakeRepo) SetCancelAtPeriodEnd(_ context.Context, userID string, value bool) (Subscription, error) {
	r.cancelled++
	r.pending.UserID = userID
	r.pending.CancelAtPeriodEnd = value
	return r.pending, nil
}
func (r *billingFakeRepo) ListBillingAttempts(context.Context, string, int) ([]payments.Attempt, error) {
	return r.attempts, nil
}
func (r *billingFakeRepo) GetSubscriptionForBilling(context.Context, string) (Subscription, error) {
	return r.pending, nil
}
func (r *billingFakeRepo) CurrentSubscription(context.Context, string) (*Subscription, error) {
	if r.pending.ID == "" {
		return nil, nil
	}
	v := r.pending
	return &v, nil
}
func (r *billingFakeRepo) UserOwnsSubscription(_ context.Context, userID, subscriptionID string) (bool, error) {
	return r.pending.UserID == userID && r.pending.ID == subscriptionID, nil
}
func (r *billingFakeRepo) ClaimDueRenewals(context.Context, int, time.Time) ([]Subscription, error) {
	return append([]Subscription(nil), r.renewalItems...), nil
}
func (r *billingFakeRepo) ReleaseRenewalClaim(context.Context, string) error { return nil }
func (r *billingFakeRepo) ExpireDueSubscriptions(context.Context, int, time.Time) (int, error) {
	return 0, nil
}
func (r *billingFakeRepo) GetPlan(_ context.Context, id string) (Plan, error) {
	if p, ok := r.plansByID[id]; ok {
		return p, nil
	}
	return r.fakeRepo.GetPlan(context.Background(), id)
}

type billingRoutingRepo struct{ route payments.Route }

func (r billingRoutingRepo) GetRoute(context.Context, payments.Domain) (payments.Route, error) {
	return r.route, nil
}

type billingProvider struct {
	create         payments.PurchaseResult
	recur          payments.PurchaseResult
	recurringCalls int
}

func (p *billingProvider) CreatePurchase(context.Context, payments.PurchaseRequest) (payments.PurchaseResult, error) {
	return p.create, nil
}
func (p *billingProvider) GetStatus(context.Context, string) (payments.Status, string, error) {
	return p.create.Status, p.create.RawStatus, nil
}
func (p *billingProvider) RefundPurchase(context.Context, string, string, int64) (string, error) {
	return "refund-1", nil
}
func (p *billingProvider) ChargeRecurring(context.Context, payments.PurchaseRequest) (payments.PurchaseResult, error) {
	p.recurringCalls++
	return p.recur, nil
}

func TestBillingPurchaseRequiresAuthoritativeSuccessBeforeActivation(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	plan := Plan{ID: "22222222-2222-2222-2222-222222222222", Name: "PRO месяц", Tier: "PRO", BillingPeriod: "MONTH", Currency: "RUB", AmountKopecks: 99900, Active: true}
	repo := &billingFakeRepo{fakeRepo: &fakeRepo{enabled: true, plans: []Plan{plan}}, plansByID: map[string]Plan{plan.ID: plan}}
	provider := &billingProvider{create: payments.PurchaseResult{ExternalID: "provider-payment-1", Status: payments.StatusPendingUserAction, RawStatus: "pending", ConfirmationURL: "https://provider.test/pay", SavedMethodRef: ""}}
	store := &payments.Store{}
	service := BillingService{
		Repository:    repo,
		Payments:      payments.Service{Repository: store, Now: func() time.Time { return now }},
		Routing:       payments.RoutingService{Repository: billingRoutingRepo{route: payments.Route{Domain: payments.DomainProSubscription, Provider: payments.ProviderYooKassa, Enabled: true, Configured: true, Environment: payments.EnvironmentSandbox}}, Registry: payments.DefaultRegistry(), ApplicationEnvironment: payments.EnvironmentSandbox},
		Providers:     payments.ProviderSet{Purchases: map[payments.ProviderName]payments.PurchaseProvider{payments.ProviderYooKassa: provider}, Recurring: map[payments.ProviderName]payments.RecurringProvider{payments.ProviderYooKassa: provider}},
		PublicBaseURL: "https://naimio.test", Now: func() time.Time { return now },
	}
	out, err := service.StartPurchase(context.Background(), "user-1", plan.ID, "purchase-123456")
	if err != nil {
		t.Fatal(err)
	}
	if out.Attempt.Status != payments.StatusPendingUserAction || repo.activated != 0 {
		t.Fatalf("attempt=%s activated=%d", out.Attempt.Status, repo.activated)
	}
	attempt, err := store.Get(context.Background(), out.Attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := attempt.Transition(payments.StatusSucceeded, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	attempt.ProviderPaymentMethodRef = "saved-method-1"
	if err := store.Update(context.Background(), attempt); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ApplyAttempt(context.Background(), attempt, attempt.ProviderPaymentMethodRef); err != nil {
		t.Fatal(err)
	}
	if repo.activated != 1 || repo.pending.Status != "ACTIVE" || repo.savedMethod != "saved-method-1" {
		t.Fatalf("activated=%d status=%s method=%q", repo.activated, repo.pending.Status, repo.savedMethod)
	}
}

func TestBillingRenewalUsesExistingProviderAndOneAttemptKey(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	plan := Plan{ID: "22222222-2222-2222-2222-222222222222", Name: "PRO месяц", Tier: "PRO", BillingPeriod: "MONTH", Currency: "RUB", AmountKopecks: 99900, Active: true}
	sub := Subscription{ID: "11111111-1111-1111-1111-111111111111", UserID: "user-1", PlanID: plan.ID, Status: "ACTIVE", Provider: string(payments.ProviderYooKassa), PaymentMethodRef: "saved-method", CurrentPeriodStart: now.AddDate(0, -1, 0), CurrentPeriodEnd: now}
	repo := &billingFakeRepo{fakeRepo: &fakeRepo{enabled: true, plans: []Plan{plan}}, pending: sub, renewalItems: []Subscription{sub}, plansByID: map[string]Plan{plan.ID: plan}}
	provider := &billingProvider{recur: payments.PurchaseResult{ExternalID: "renewal-payment-1", Status: payments.StatusSucceeded, RawStatus: "succeeded", SavedMethodRef: "saved-method"}}
	store := &payments.Store{}
	service := BillingService{Repository: repo, Payments: payments.Service{Repository: store, Now: func() time.Time { return now }}, Providers: payments.ProviderSet{Recurring: map[payments.ProviderName]payments.RecurringProvider{payments.ProviderYooKassa: provider}}, Now: func() time.Time { return now }}
	processed, err := service.RunRenewals(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 || provider.recurringCalls != 1 || repo.renewed != 1 {
		t.Fatalf("processed=%d calls=%d renewed=%d", processed, provider.recurringCalls, repo.renewed)
	}
	// Simulate the same due item being observed again. The provider must not be called twice.
	repo.renewalItems = []Subscription{sub}
	processed, err = service.RunRenewals(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 0 || provider.recurringCalls != 1 {
		t.Fatalf("second processed=%d calls=%d", processed, provider.recurringCalls)
	}
}

func TestBillingRecoveryUsesPinnedProviderAndExistingSubscription(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	plan := Plan{ID: "22222222-2222-2222-2222-222222222222", Name: "PRO месяц", Tier: "PRO", BillingPeriod: "MONTH", Currency: "RUB", AmountKopecks: 99900, Active: true}
	sub := Subscription{ID: "11111111-1111-1111-1111-111111111111", UserID: "user-1", PlanID: plan.ID, PlanName: plan.Name, Status: "PAST_DUE", Provider: string(payments.ProviderYooKassa), CurrentPeriodStart: now.AddDate(0, -1, 0), CurrentPeriodEnd: now.Add(-time.Hour)}
	repo := &billingFakeRepo{fakeRepo: &fakeRepo{enabled: true, plans: []Plan{plan}}, pending: sub, plansByID: map[string]Plan{plan.ID: plan}}
	provider := &billingProvider{create: payments.PurchaseResult{ExternalID: "recovery-payment-1", Status: payments.StatusPendingUserAction, RawStatus: "pending", ConfirmationURL: "https://provider.test/recover"}}
	store := &payments.Store{}
	service := BillingService{
		Repository: repo,
		Payments:   payments.Service{Repository: store, Now: func() time.Time { return now }},
		Routing: payments.RoutingService{
			Registry:               payments.DefaultRegistry(),
			ApplicationEnvironment: payments.EnvironmentSandbox,
		},
		Providers: payments.ProviderSet{
			Purchases: map[payments.ProviderName]payments.PurchaseProvider{payments.ProviderYooKassa: provider},
			Recurring: map[payments.ProviderName]payments.RecurringProvider{payments.ProviderYooKassa: provider},
		},
		PublicBaseURL: "https://naimio.test",
		Now:           func() time.Time { return now },
	}
	out, err := service.RecoverPurchase(context.Background(), "user-1", "recovery-123456")
	if err != nil {
		t.Fatal(err)
	}
	if out.Subscription.ID != sub.ID || out.Attempt.InternalReferenceID != sub.ID {
		t.Fatalf("recovery created a different subscription: sub=%s attempt_ref=%s", out.Subscription.ID, out.Attempt.InternalReferenceID)
	}
	if out.Attempt.Provider != payments.ProviderYooKassa || out.Attempt.OperationType != payments.OperationRenewal {
		t.Fatalf("recovery was not pinned to the subscription provider: provider=%s operation=%s", out.Attempt.Provider, out.Attempt.OperationType)
	}
	if out.Attempt.Status != payments.StatusPendingUserAction || repo.renewed != 0 {
		t.Fatalf("browser checkout must not activate renewal: status=%s renewed=%d", out.Attempt.Status, repo.renewed)
	}
}

func TestBillingRecoveryRejectsNonPastDueSubscription(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	plan := Plan{ID: "22222222-2222-2222-2222-222222222222", Name: "PRO месяц", Tier: "PRO", BillingPeriod: "MONTH", Currency: "RUB", AmountKopecks: 99900, Active: true}
	repo := &billingFakeRepo{fakeRepo: &fakeRepo{enabled: true, plans: []Plan{plan}}, pending: Subscription{ID: "11111111-1111-1111-1111-111111111111", UserID: "user-1", PlanID: plan.ID, Status: "ACTIVE", Provider: string(payments.ProviderYooKassa)}, plansByID: map[string]Plan{plan.ID: plan}}
	service := BillingService{Repository: repo, Routing: payments.RoutingService{Registry: payments.DefaultRegistry()}, Now: func() time.Time { return now }}
	if _, err := service.RecoverPurchase(context.Background(), "user-1", "recovery-123456"); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict for ACTIVE subscription, got %v", err)
	}
}
