package monetization

import (
	"context"
	"errors"
	"fmt"
	"time"

	"freelance/apps/api/internal/payments"
)

var ErrBillingUnavailable = errors.New("billing unavailable")

type BillingRepository interface {
	Repository
	CreatePendingSubscription(context.Context, string, Plan, payments.ProviderName, time.Time, time.Time) (Subscription, error)
	SavePaymentMethod(context.Context, string, string, string) error
	ActivatePaidSubscription(context.Context, string, string, time.Time, time.Time, string) (Subscription, error)
	RenewPaidSubscription(context.Context, string, string, time.Time, time.Time) (Subscription, error)
	MarkSubscriptionPastDue(context.Context, string, string) error
	FailInitialSubscription(context.Context, string, string, string) error
	SetCancelAtPeriodEnd(context.Context, string, bool) (Subscription, error)
	ListBillingAttempts(context.Context, string, int) ([]payments.Attempt, error)
	GetSubscriptionForBilling(context.Context, string) (Subscription, error)
	UserOwnsSubscription(context.Context, string, string) (bool, error)
	ClaimDueRenewals(context.Context, int, time.Time) ([]Subscription, error)
	ReleaseRenewalClaim(context.Context, string) error
	ExpireDueSubscriptions(context.Context, int, time.Time) (int, error)
}

type BillingService struct {
	Repository    BillingRepository
	Payments      payments.Service
	Routing       payments.RoutingService
	Providers     payments.ProviderSet
	PublicBaseURL string
	Now           func() time.Time
}

type Checkout struct {
	Attempt         payments.Attempt `json:"attempt"`
	Subscription    Subscription     `json:"subscription"`
	ConfirmationURL string           `json:"confirmation_url,omitempty"`
}

func (s BillingService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func periodEnd(start time.Time, p Plan) time.Time {
	if p.BillingPeriod == "YEAR" {
		return start.AddDate(1, 0, 0)
	}
	return start.AddDate(0, 1, 0)
}

func (s BillingService) StartPurchase(ctx context.Context, userID, planID, idempotencyKey string) (Checkout, error) {
	if userID == "" || planID == "" || len(idempotencyKey) < 8 {
		return Checkout{}, ErrInvalid
	}
	enabled, err := s.Repository.FeatureEnabled(ctx, "pro_subscriptions_enabled")
	if err != nil || !enabled {
		return Checkout{}, ErrProviderUnavailable
	}
	plan, err := s.Repository.GetPlan(ctx, planID)
	if err != nil {
		return Checkout{}, err
	}
	if !plan.Active || plan.Tier != "PRO" || plan.AmountKopecks <= 0 || plan.Currency != "RUB" {
		return Checkout{}, ErrInvalid
	}
	provider, err := s.Routing.Select(ctx, payments.DomainProSubscription, payments.CapabilityOneTimePayment)
	if err != nil || !s.Routing.Registry.SupportsRecurring(provider) {
		return Checkout{}, ErrProviderUnavailable
	}
	if _, err := s.Providers.RecurringCharge(provider); err != nil {
		return Checkout{}, ErrProviderUnavailable
	}
	now := s.now()
	sub, err := s.Repository.CreatePendingSubscription(ctx, userID, plan, provider, now, periodEnd(now, plan))
	if err != nil {
		return Checkout{}, err
	}
	if sub.Status == "ACTIVE" || sub.PlanID != plan.ID {
		return Checkout{}, ErrConflict
	}
	attempt, err := s.Payments.Create(ctx, payments.Attempt{Domain: payments.DomainProSubscription, InternalReferenceID: sub.ID, Provider: provider, OperationType: payments.OperationPayment, AmountKopecks: plan.AmountKopecks, Currency: "RUB", IdempotencyKey: idempotencyKey, PaymentMethod: "CARD"})
	if err != nil {
		return Checkout{}, err
	}
	if attempt.ProviderOperationID != "" {
		return Checkout{Attempt: attempt, Subscription: sub, ConfirmationURL: attempt.ConfirmationURL}, nil
	}
	p, err := s.Providers.Purchase(provider)
	if err != nil {
		return Checkout{}, ErrProviderUnavailable
	}
	base := s.PublicBaseURL
	if base == "" {
		base = "http://localhost:8088"
	}
	out, callErr := p.CreatePurchase(ctx, payments.PurchaseRequest{AmountKopecks: plan.AmountKopecks, Currency: "RUB", InternalReferenceID: sub.ID, IdempotencyKey: idempotencyKey, Description: "Naimio PRO — " + plan.Name, ReturnURL: base + "/pro/payment-return?attempt_id=" + attempt.ID, FailURL: base + "/pro/payment-return?attempt_id=" + attempt.ID, SavePaymentMethod: true, CustomerID: userID, BillingPeriod: plan.BillingPeriod})
	if callErr != nil {
		_, _ = s.Payments.MarkUnknown(ctx, attempt.ID)
		return Checkout{}, fmt.Errorf("%w: %v", ErrBillingUnavailable, callErr)
	}
	attempt.ProviderOperationID = out.ExternalID
	attempt.ConfirmationURL = out.ConfirmationURL
	attempt.ProviderRawStatus = out.RawStatus
	attempt.PaymentMethod = "CARD"
	if out.Status != payments.StatusCreated {
		if e := attempt.Transition(out.Status, s.now()); e != nil && out.Status != attempt.Status {
			return Checkout{}, e
		}
	}
	if err = s.Payments.Repository.Update(ctx, attempt); err != nil {
		return Checkout{}, err
	}
	if out.SavedMethodRef != "" {
		_ = s.Repository.SavePaymentMethod(ctx, sub.ID, string(provider), out.SavedMethodRef)
	}
	if attempt.Status == payments.StatusSucceeded {
		if _, err = s.ApplyAttempt(ctx, attempt, out.SavedMethodRef); err != nil {
			return Checkout{}, err
		}
	}
	return Checkout{Attempt: attempt, Subscription: sub, ConfirmationURL: out.ConfirmationURL}, nil
}

func (s BillingService) RecoverPurchase(ctx context.Context, userID, idempotencyKey string) (Checkout, error) {
	if userID == "" || len(idempotencyKey) < 8 {
		return Checkout{}, ErrInvalid
	}
	enabled, err := s.Repository.FeatureEnabled(ctx, "pro_subscriptions_enabled")
	if err != nil || !enabled {
		return Checkout{}, ErrProviderUnavailable
	}
	sub, err := s.Repository.CurrentSubscription(ctx, userID)
	if err != nil {
		return Checkout{}, err
	}
	if sub == nil {
		return Checkout{}, ErrNotFound
	}
	if sub.Status != "PAST_DUE" || sub.CancelAtPeriodEnd {
		return Checkout{}, ErrConflict
	}
	provider := payments.ParseProvider(sub.Provider)
	if provider == "" || !s.Routing.Registry.SupportsRecurring(provider) {
		return Checkout{}, ErrProviderUnavailable
	}
	purchase, err := s.Providers.Purchase(provider)
	if err != nil {
		return Checkout{}, ErrProviderUnavailable
	}
	if _, err := s.Providers.RecurringCharge(provider); err != nil {
		return Checkout{}, ErrProviderUnavailable
	}
	plan, err := s.Repository.GetPlan(ctx, sub.PlanID)
	if err != nil {
		return Checkout{}, err
	}
	if !plan.Active || plan.Tier != "PRO" || plan.AmountKopecks <= 0 || plan.Currency != "RUB" {
		return Checkout{}, ErrInvalid
	}
	attempt, err := s.Payments.Create(ctx, payments.Attempt{
		Domain:              payments.DomainProSubscription,
		InternalReferenceID: sub.ID,
		Provider:            provider,
		OperationType:       payments.OperationRenewal,
		AmountKopecks:       plan.AmountKopecks,
		Currency:            plan.Currency,
		IdempotencyKey:      idempotencyKey,
		PaymentMethod:       "CARD",
	})
	if err != nil {
		return Checkout{}, err
	}
	if attempt.ProviderOperationID != "" {
		return Checkout{Attempt: attempt, Subscription: *sub, ConfirmationURL: attempt.ConfirmationURL}, nil
	}
	base := s.PublicBaseURL
	if base == "" {
		base = "http://localhost:8088"
	}
	out, callErr := purchase.CreatePurchase(ctx, payments.PurchaseRequest{
		AmountKopecks:       plan.AmountKopecks,
		Currency:            plan.Currency,
		InternalReferenceID: sub.ID,
		IdempotencyKey:      idempotencyKey,
		Description:         "Naimio PRO payment recovery — " + plan.Name,
		ReturnURL:           base + "/pro/payment-return?attempt_id=" + attempt.ID,
		FailURL:             base + "/pro/payment-return?attempt_id=" + attempt.ID,
		SavePaymentMethod:   true,
		CustomerID:          userID,
		BillingPeriod:       plan.BillingPeriod,
	})
	if callErr != nil {
		_, _ = s.Payments.MarkUnknown(ctx, attempt.ID)
		return Checkout{}, fmt.Errorf("%w: %v", ErrBillingUnavailable, callErr)
	}
	attempt.ProviderOperationID = out.ExternalID
	attempt.ConfirmationURL = out.ConfirmationURL
	attempt.ProviderRawStatus = out.RawStatus
	if out.SavedMethodRef != "" {
		attempt.ProviderPaymentMethodRef = out.SavedMethodRef
	}
	if out.Status != payments.StatusCreated {
		if e := attempt.Transition(out.Status, s.now()); e != nil && out.Status != attempt.Status {
			return Checkout{}, e
		}
	}
	if err = s.Payments.Repository.Update(ctx, attempt); err != nil {
		return Checkout{}, err
	}
	if out.SavedMethodRef != "" {
		_ = s.Repository.SavePaymentMethod(ctx, sub.ID, string(provider), out.SavedMethodRef)
	}
	if attempt.Status == payments.StatusSucceeded || attempt.Status == payments.StatusFailed || attempt.Status == payments.StatusCanceled {
		updated, applyErr := s.ApplyAttempt(ctx, attempt, out.SavedMethodRef)
		if applyErr != nil {
			return Checkout{}, applyErr
		}
		*sub = updated
	}
	return Checkout{Attempt: attempt, Subscription: *sub, ConfirmationURL: out.ConfirmationURL}, nil
}

func (s BillingService) ApplyAttempt(ctx context.Context, a payments.Attempt, savedMethod string) (Subscription, error) {
	if a.Domain != payments.DomainProSubscription {
		return Subscription{}, ErrInvalid
	}
	sub, err := s.Repository.GetSubscriptionForBilling(ctx, a.InternalReferenceID)
	if err != nil {
		return Subscription{}, err
	}
	if savedMethod != "" {
		_ = s.Repository.SavePaymentMethod(ctx, sub.ID, string(a.Provider), savedMethod)
	}
	if a.Status == payments.StatusSucceeded {
		if a.OperationType == payments.OperationRenewal {
			start := sub.CurrentPeriodEnd
			if start.Before(s.now()) {
				start = s.now()
			}
			plan, err := s.Repository.GetPlan(ctx, sub.PlanID)
			if err != nil {
				return Subscription{}, err
			}
			return s.Repository.RenewPaidSubscription(ctx, sub.ID, a.ID, start, periodEnd(start, plan))
		}
		start, end := sub.CurrentPeriodStart, sub.CurrentPeriodEnd
		if !end.After(s.now()) {
			start = s.now()
			plan, err := s.Repository.GetPlan(ctx, sub.PlanID)
			if err != nil {
				return Subscription{}, err
			}
			end = periodEnd(start, plan)
		}
		return s.Repository.ActivatePaidSubscription(ctx, sub.ID, a.ID, start, end, string(a.Provider))
	}
	if a.OperationType == payments.OperationRenewal && (a.Status == payments.StatusFailed || a.Status == payments.StatusCanceled) {
		_ = s.Repository.MarkSubscriptionPastDue(ctx, sub.ID, a.ID)
		return s.Repository.GetSubscriptionForBilling(ctx, sub.ID)
	}
	if a.OperationType == payments.OperationPayment && (a.Status == payments.StatusFailed || a.Status == payments.StatusCanceled) && sub.Status == "PENDING" {
		reason := "authoritative provider payment failed"
		if a.Status == payments.StatusCanceled {
			reason = "authoritative provider payment canceled"
		}
		if err := s.Repository.FailInitialSubscription(ctx, sub.ID, a.ID, reason); err != nil {
			return Subscription{}, err
		}
		return s.Repository.GetSubscriptionForBilling(ctx, sub.ID)
	}
	return sub, nil
}

func (s BillingService) Available(ctx context.Context) bool {
	provider, err := s.Routing.Select(ctx, payments.DomainProSubscription, payments.CapabilityOneTimePayment)
	if err != nil || !s.Routing.Registry.SupportsRecurring(provider) {
		return false
	}
	if _, err := s.Providers.Purchase(provider); err != nil {
		return false
	}
	if _, err := s.Providers.RecurringCharge(provider); err != nil {
		return false
	}
	return true
}

func (s BillingService) Status(ctx context.Context, userID, attemptID string) (payments.Attempt, error) {
	a, err := s.Payments.Repository.Get(ctx, attemptID)
	if err != nil {
		return payments.Attempt{}, err
	}
	if a.Domain != payments.DomainProSubscription {
		return payments.Attempt{}, ErrNotFound
	}
	if ok, err := s.Repository.UserOwnsSubscription(ctx, userID, a.InternalReferenceID); err != nil || !ok {
		return payments.Attempt{}, ErrForbidden
	}
	// The browser return is never authoritative. While an operation is still
	// pending, ask the provider pinned to this attempt and persist only a valid
	// provider transition. A status lookup failure leaves the existing state in
	// place for the reconciliation worker instead of creating a second charge.
	if !a.Terminal() && a.ProviderOperationID != "" {
		if provider, providerErr := s.Providers.Status(a.Provider); providerErr == nil && provider != nil {
			details, queryErr := payments.QueryProviderStatus(ctx, provider, a.OperationType, a.ProviderOperationID)
			if queryErr == nil && details.Status != payments.StatusUnknownReconciliation {
				if details.SavedMethodRef != "" {
					a.ProviderPaymentMethodRef = details.SavedMethodRef
				}
				if details.Status != a.Status {
					if transitionErr := a.Transition(details.Status, s.now()); transitionErr == nil {
						a.ProviderRawStatus = details.RawStatus
						if err := s.Payments.Repository.Update(ctx, a); err != nil {
							return payments.Attempt{}, err
						}
						if _, err := s.ApplyAttempt(ctx, a, details.SavedMethodRef); err != nil {
							return payments.Attempt{}, err
						}
					}
				} else if details.SavedMethodRef != "" || details.RawStatus != "" {
					a.ProviderRawStatus = details.RawStatus
					if err := s.Payments.Repository.Update(ctx, a); err != nil {
						return payments.Attempt{}, err
					}
				}
			}
		}
	}
	return a, nil
}

func (s BillingService) History(ctx context.Context, userID string) ([]payments.Attempt, error) {
	return s.Repository.ListBillingAttempts(ctx, userID, 100)
}
func (s BillingService) CancelAutoRenew(ctx context.Context, userID string) (Subscription, error) {
	return s.Repository.SetCancelAtPeriodEnd(ctx, userID, true)
}

func (s BillingService) RunRenewals(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	now := s.now()
	if _, err := s.Repository.ExpireDueSubscriptions(ctx, limit, now); err != nil {
		return 0, err
	}
	items, err := s.Repository.ClaimDueRenewals(ctx, limit, now)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, sub := range items {
		err := func() error {
			defer func() { _ = s.Repository.ReleaseRenewalClaim(context.WithoutCancel(ctx), sub.ID) }()
			if sub.CancelAtPeriodEnd || sub.PaymentMethodRef == "" {
				return nil
			}
			provider := payments.ParseProvider(sub.Provider)
			recurring, err := s.Providers.RecurringCharge(provider)
			if err != nil {
				return nil
			}
			plan, err := s.Repository.GetPlan(ctx, sub.PlanID)
			if err != nil {
				return err
			}
			start := sub.CurrentPeriodEnd.UTC()
			if start.Before(now) {
				start = now
			}
			// The first renewal key is anchored to the paid billing period, not to
			// wall-clock worker execution. Otherwise an unknown PSP result followed
			// by another scheduler tick could generate a new key and double-charge.
			// Once an authoritative failure moved the subscription to PAST_DUE,
			// next_retry_at deliberately creates a new retry generation.
			generation := sub.CurrentPeriodEnd.UTC()
			if sub.Status == "PAST_DUE" && sub.NextRetryAt != nil {
				generation = sub.NextRetryAt.UTC()
			}
			key := fmt.Sprintf("pro-renewal:%s:%s", sub.ID, generation.Format("20060102T150405Z"))
			attempt, err := s.Payments.Create(ctx, payments.Attempt{Domain: payments.DomainProSubscription, InternalReferenceID: sub.ID, Provider: provider, OperationType: payments.OperationRenewal, AmountKopecks: plan.AmountKopecks, Currency: plan.Currency, IdempotencyKey: key, PaymentMethod: "CARD"})
			if err != nil {
				return nil
			}
			// Call the PSP only from the pristine CREATED state. If an earlier
			// provider call became unknown/pending without a durable external id,
			// reconciliation/manual recovery must resolve it; a retry must never
			// create a second cross-request charge.
			if attempt.ProviderOperationID != "" || attempt.Status != payments.StatusCreated {
				return nil
			}
			out, callErr := recurring.ChargeRecurring(ctx, payments.PurchaseRequest{AmountKopecks: plan.AmountKopecks, Currency: plan.Currency, InternalReferenceID: sub.ID, IdempotencyKey: key, Description: "Naimio PRO renewal — " + plan.Name, SavedMethodRef: sub.PaymentMethodRef, CustomerID: sub.UserID, BillingPeriod: plan.BillingPeriod})
			if callErr != nil {
				_, _ = s.Payments.MarkUnknown(ctx, attempt.ID)
				return nil
			}
			attempt.ProviderOperationID, attempt.ProviderRawStatus = out.ExternalID, out.RawStatus
			if out.SavedMethodRef != "" {
				attempt.ProviderPaymentMethodRef = out.SavedMethodRef
			}
			if out.Status != payments.StatusCreated {
				_ = attempt.Transition(out.Status, now)
			}
			if err := s.Payments.Repository.Update(ctx, attempt); err != nil {
				return err
			}
			if out.Status == payments.StatusSucceeded || out.Status == payments.StatusFailed {
				if _, err := s.ApplyAttempt(ctx, attempt, out.SavedMethodRef); err != nil {
					return err
				}
			}
			processed++
			return nil
		}()
		if err != nil {
			return processed, err
		}
	}
	return processed, nil
}
