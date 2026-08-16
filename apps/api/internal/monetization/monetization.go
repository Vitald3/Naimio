package monetization

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
)

var (
	ErrForbidden           = errors.New("monetization admin permission required")
	ErrNotFound            = errors.New("monetization resource not found")
	ErrInvalid             = errors.New("invalid monetization input")
	ErrConflict            = errors.New("monetization conflict")
	ErrProviderUnavailable = errors.New("subscription payments are unavailable")
)

const (
	FeatureProBadge         = "profile.pro_badge"
	FeaturePortfolioItems   = "portfolio.item_limit"
	FeaturePortfolioMedia   = "portfolio.media_limit"
	FeatureProfileAnalytics = "profile.analytics"
	FeatureSearchPriority   = "search.priority_visibility"
)

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(?:\.[a-z][a-z0-9_]*)+$`)

type Plan struct {
	ID            string        `json:"id"`
	Code          string        `json:"code"`
	Name          string        `json:"name"`
	Description   string        `json:"description"`
	Tier          string        `json:"tier"`
	BillingPeriod string        `json:"billing_period"`
	Currency      string        `json:"currency"`
	AmountKopecks int64         `json:"amount_kopecks"`
	Active        bool          `json:"active"`
	DisplayOrder  int           `json:"display_order"`
	Entitlements  []Entitlement `json:"entitlements,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
}

type Entitlement struct {
	FeatureKey string         `json:"feature_key"`
	Kind       string         `json:"kind"`
	Enabled    bool           `json:"enabled"`
	LimitValue *int64         `json:"limit_value,omitempty"`
	Unlimited  bool           `json:"unlimited"`
	Config     map[string]any `json:"config"`
}

type Subscription struct {
	ID                     string     `json:"id"`
	UserID                 string     `json:"user_id"`
	UserName               string     `json:"user_name,omitempty"`
	PlanID                 string     `json:"plan_id"`
	PlanCode               string     `json:"plan_code"`
	PlanName               string     `json:"plan_name"`
	Status                 string     `json:"status"`
	StartsAt               time.Time  `json:"starts_at"`
	CurrentPeriodStart     time.Time  `json:"current_period_start"`
	CurrentPeriodEnd       time.Time  `json:"current_period_end"`
	CancelAtPeriodEnd      bool       `json:"cancel_at_period_end"`
	CanceledAt             *time.Time `json:"canceled_at,omitempty"`
	Provider               string     `json:"provider,omitempty"`
	ProviderCustomerID     string     `json:"-"`
	ProviderSubscriptionID string     `json:"-"`
	PaymentMethodRef       string     `json:"-"`
	NextRetryAt            *time.Time `json:"-"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

func (s Subscription) Effective(at time.Time) bool {
	return (s.Status == "ACTIVE" || s.Status == "PAST_DUE") && !at.Before(s.StartsAt) && at.Before(s.CurrentPeriodEnd)
}

type Event struct {
	ID             string    `json:"id"`
	SubscriptionID string    `json:"subscription_id"`
	EventType      string    `json:"event_type"`
	FromStatus     string    `json:"from_status,omitempty"`
	ToStatus       string    `json:"to_status,omitempty"`
	ActorUserID    string    `json:"actor_user_id,omitempty"`
	Reason         string    `json:"reason,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type Capabilities struct {
	ProSystemEnabled bool                   `json:"pro_system_enabled"`
	EffectivePro     bool                   `json:"effective_pro"`
	Plan             *Plan                  `json:"plan,omitempty"`
	Subscription     *Subscription          `json:"subscription,omitempty"`
	Features         map[string]Entitlement `json:"features"`
}

func (c Capabilities) Has(key string) bool {
	v, ok := c.Features[key]
	return ok && v.Enabled && v.Kind == "BOOLEAN"
}

func (c Capabilities) Limit(key string) (value int64, unlimited bool, ok bool) {
	v, ok := c.Features[key]
	if !ok || !v.Enabled || v.Kind != "LIMIT" {
		return 0, false, false
	}
	if v.Unlimited {
		return 0, true, true
	}
	if v.LimitValue == nil {
		return 0, false, false
	}
	return *v.LimitValue, false, true
}

// RankingMultiplier returns the configured PRO discovery multiplier when the
// priority-visibility entitlement is enabled. Callers must still apply a
// bounded conversion; raw multipliers never become hard rank overrides.
func (c Capabilities) RankingMultiplier() (float64, bool) {
	v, ok := c.Features[FeatureSearchPriority]
	if !ok || !v.Enabled || v.Kind != "BOOLEAN" {
		return 1, false
	}
	raw, exists := v.Config["ranking_multiplier"]
	if !exists {
		return 1.08, true
	}
	switch n := raw.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return 1.08, true
		}
		return f, true
	default:
		return 1.08, true
	}
}

type Overview struct {
	ProSystemEnabled  bool  `json:"pro_system_enabled"`
	ActiveCount       int64 `json:"active_count"`
	New30Days         int64 `json:"new_30_days"`
	Expiring7Days     int64 `json:"expiring_7_days"`
	ProviderConnected bool  `json:"provider_connected"`
}

type Repository interface {
	FeatureEnabled(context.Context, string) (bool, error)
	IsAdmin(context.Context, string) (bool, error)
	ListPlans(context.Context, bool) ([]Plan, error)
	GetPlan(context.Context, string) (Plan, error)
	PlanEntitlements(context.Context, string) ([]Entitlement, error)
	CurrentSubscription(context.Context, string) (*Subscription, error)
	SubscriptionHistory(context.Context, string) ([]Subscription, error)
	SubscriptionEvents(context.Context, string) ([]Event, error)
	Overview(context.Context) (Overview, error)
	ListSubscriptions(context.Context, string, int) ([]Subscription, error)
	Grant(context.Context, string, string, string, time.Time, time.Time, string, string) (Subscription, error)
	Transition(context.Context, string, string, string, string, string) (Subscription, error)
	UpdatePlan(context.Context, string, Plan, string, string) (Plan, error)
	SetEntitlement(context.Context, string, string, Entitlement, string, string) (Entitlement, error)
}

type Service struct {
	Repository Repository
	Now        func() time.Time
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s Service) PublicPlans(ctx context.Context) ([]Plan, bool, error) {
	enabled, err := s.Repository.FeatureEnabled(ctx, "pro_subscriptions_enabled")
	if err != nil {
		return nil, false, err
	}
	if !enabled {
		return []Plan{}, false, nil
	}
	plans, err := s.Repository.ListPlans(ctx, true)
	if err != nil {
		return nil, false, err
	}
	return plans, true, nil
}

func (s Service) Resolve(ctx context.Context, userID string) (Capabilities, error) {
	enabled, err := s.Repository.FeatureEnabled(ctx, "pro_subscriptions_enabled")
	if err != nil {
		return Capabilities{}, err
	}
	plans, err := s.Repository.ListPlans(ctx, true)
	if err != nil {
		return Capabilities{}, err
	}
	var selected *Plan
	for i := range plans {
		if plans[i].Tier == "FREE" {
			p := plans[i]
			selected = &p
			break
		}
	}
	if selected == nil {
		return Capabilities{}, ErrNotFound
	}
	var current *Subscription
	effective := false
	if userID != "" {
		current, err = s.Repository.CurrentSubscription(ctx, userID)
		if err != nil {
			return Capabilities{}, err
		}
		if enabled && current != nil && current.Effective(s.now()) {
			p, getErr := s.Repository.GetPlan(ctx, current.PlanID)
			if getErr != nil {
				return Capabilities{}, getErr
			}
			if p.Active && p.Tier == "PRO" {
				selected = &p
				effective = true
			}
		}
	}
	features, err := s.Repository.PlanEntitlements(ctx, selected.ID)
	if err != nil {
		return Capabilities{}, err
	}
	resolved := make(map[string]Entitlement, len(features))
	for _, feature := range features {
		resolved[feature.FeatureKey] = feature
	}
	return Capabilities{ProSystemEnabled: enabled, EffectivePro: effective, Plan: selected, Subscription: current, Features: resolved}, nil
}

func (s Service) requireAdmin(ctx context.Context, actor string) error {
	if actor == "" {
		return ErrForbidden
	}
	ok, err := s.Repository.IsAdmin(ctx, actor)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return nil
}

func (s Service) AdminData(ctx context.Context, actor, status string) (Overview, []Plan, []Subscription, error) {
	if err := s.requireAdmin(ctx, actor); err != nil {
		return Overview{}, nil, nil, err
	}
	o, err := s.Repository.Overview(ctx)
	if err != nil {
		return o, nil, nil, err
	}
	p, err := s.Repository.ListPlans(ctx, false)
	if err != nil {
		return o, nil, nil, err
	}
	items, err := s.Repository.ListSubscriptions(ctx, strings.ToUpper(strings.TrimSpace(status)), 100)
	return o, p, items, err
}

func (s Service) Grant(ctx context.Context, actor, userID, planID string, startsAt, endsAt time.Time, reason, requestID string) (Subscription, error) {
	if err := s.requireAdmin(ctx, actor); err != nil {
		return Subscription{}, err
	}
	if strings.TrimSpace(reason) == "" || !endsAt.After(startsAt) || startsAt.IsZero() || endsAt.IsZero() {
		return Subscription{}, ErrInvalid
	}
	p, err := s.Repository.GetPlan(ctx, planID)
	if err != nil {
		return Subscription{}, err
	}
	if !p.Active || p.Tier != "PRO" {
		return Subscription{}, ErrInvalid
	}
	return s.Repository.Grant(ctx, actor, userID, planID, startsAt.UTC(), endsAt.UTC(), strings.TrimSpace(reason), requestID)
}

func (s Service) Transition(ctx context.Context, actor, id, status, reason, requestID string) (Subscription, error) {
	if err := s.requireAdmin(ctx, actor); err != nil {
		return Subscription{}, err
	}
	status = strings.ToUpper(strings.TrimSpace(status))
	if (status != "CANCELED" && status != "EXPIRED") || strings.TrimSpace(reason) == "" {
		return Subscription{}, ErrInvalid
	}
	return s.Repository.Transition(ctx, actor, id, status, strings.TrimSpace(reason), requestID)
}

func (s Service) UpdatePlan(ctx context.Context, actor string, plan Plan, reason, requestID string) (Plan, error) {
	if err := s.requireAdmin(ctx, actor); err != nil {
		return Plan{}, err
	}
	plan.Name, plan.Description, plan.Currency = strings.TrimSpace(plan.Name), strings.TrimSpace(plan.Description), strings.ToUpper(strings.TrimSpace(plan.Currency))
	if plan.ID == "" || plan.Name == "" || len(plan.Name) > 120 || plan.AmountKopecks < 0 || plan.Currency != "RUB" || plan.DisplayOrder < 0 || strings.TrimSpace(reason) == "" {
		return Plan{}, ErrInvalid
	}
	return s.Repository.UpdatePlan(ctx, actor, plan, strings.TrimSpace(reason), requestID)
}

func (s Service) SetEntitlement(ctx context.Context, actor, planID string, v Entitlement, reason, requestID string) (Entitlement, error) {
	if err := s.requireAdmin(ctx, actor); err != nil {
		return Entitlement{}, err
	}
	v.FeatureKey, v.Kind = strings.ToLower(strings.TrimSpace(v.FeatureKey)), strings.ToUpper(strings.TrimSpace(v.Kind))
	if !keyPattern.MatchString(v.FeatureKey) || strings.TrimSpace(reason) == "" || (v.Kind != "BOOLEAN" && v.Kind != "LIMIT") {
		return Entitlement{}, ErrInvalid
	}
	if v.Kind == "BOOLEAN" {
		v.LimitValue = nil
		v.Unlimited = false
	} else if !v.Unlimited && v.LimitValue == nil {
		return Entitlement{}, ErrInvalid
	}
	if v.Config == nil {
		v.Config = map[string]any{}
	}
	return s.Repository.SetEntitlement(ctx, actor, planID, v, strings.TrimSpace(reason), requestID)
}
