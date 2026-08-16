package payments

import (
	"context"
	"errors"
)

// StatusProvider is intentionally narrow. Reconciliation is always sent to the
// provider pinned on the attempt; it does not perform provider selection.
type StatusProvider interface {
	GetStatus(context.Context, string) (Status, string, error)
}

// OperationStatusProvider lets adapters distinguish a payment from a refund or
// payout when the provider exposes separate status resources.
type OperationStatusProvider interface {
	GetOperationStatus(context.Context, OperationType, string) (Status, string, error)
}
type StatusDetails struct {
	Status         Status
	RawStatus      string
	SavedMethodRef string
}
type StatusDetailsProvider interface {
	GetStatusDetails(context.Context, string) (StatusDetails, error)
}
type OperationStatusDetailsProvider interface {
	GetOperationStatusDetails(context.Context, OperationType, string) (StatusDetails, error)
}
type ReconciliationRepository interface {
	ListPendingReconciliation(context.Context, int) ([]Attempt, error)
	Update(context.Context, Attempt) error
}
type Reconciler struct {
	Repository      ReconciliationRepository
	Providers       map[ProviderName]StatusProvider
	ProviderSet     ProviderSet
	Service         Service
	AfterTransition func(context.Context, Attempt) error
}

func (r Reconciler) statusProvider(name ProviderName) StatusProvider {
	if r.ProviderSet.Runtime != nil {
		if p, err := r.ProviderSet.Status(name); err == nil && p != nil {
			return p
		}
	}
	return r.Providers[name]
}

func (r Reconciler) Run(ctx context.Context, limit int) (int, error) {
	items, err := r.Repository.ListPendingReconciliation(ctx, limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, a := range items {
		p := r.statusProvider(a.Provider)
		if p == nil {
			continue
		}
		details, e := QueryProviderStatus(ctx, p, a.OperationType, a.ProviderOperationID)
		status, raw := details.Status, details.RawStatus
		if e != nil {
			a.ReconciliationState = ReconciliationRequired
			a.ErrorCategory = "STATUS_QUERY_RETRYABLE"
			_ = r.Repository.Update(ctx, a)
			continue
		}
		if status == StatusUnknownReconciliation {
			a.ReconciliationState = ReconciliationRequired
			a.ProviderRawStatus = raw
			_ = r.Repository.Update(ctx, a)
			continue
		}
		if details.SavedMethodRef != "" {
			a.ProviderPaymentMethodRef = details.SavedMethodRef
		}
		if a.Status == status {
			a.ProviderRawStatus = raw
			if a.Terminal() {
				a.ReconciliationState = ReconciliationNotRequired
			} else {
				a.ReconciliationState = ReconciliationRequired
			}
			a.UpdatedAt = r.Service.now()
		} else {
			if e = a.Transition(status, r.Service.now()); e != nil {
				continue
			}
			a.ProviderRawStatus = raw
			if !a.Terminal() {
				a.ReconciliationState = ReconciliationRequired
			}
		}
		if e = r.Repository.Update(ctx, a); e != nil {
			return processed, e
		}
		if r.AfterTransition != nil {
			if e = r.AfterTransition(ctx, a); e != nil {
				return processed, e
			}
		}
		processed++
	}
	return processed, nil
}
func (r Reconciler) One(ctx context.Context, a Attempt) (Attempt, error) {
	p := r.statusProvider(a.Provider)
	if p == nil {
		return Attempt{}, ErrUnknownProvider
	}
	if a.ProviderOperationID == "" {
		return Attempt{}, errors.New("payment attempt has no provider operation")
	}
	details, err := QueryProviderStatus(ctx, p, a.OperationType, a.ProviderOperationID)
	s, raw := details.Status, details.RawStatus
	if err != nil {
		return Attempt{}, err
	}
	if s == StatusUnknownReconciliation {
		return a, nil
	}
	if details.SavedMethodRef != "" {
		a.ProviderPaymentMethodRef = details.SavedMethodRef
	}
	if a.Status == s {
		a.ProviderRawStatus = raw
		a.UpdatedAt = r.Service.now()
		if a.Terminal() {
			a.ReconciliationState = ReconciliationNotRequired
		} else {
			a.ReconciliationState = ReconciliationRequired
		}
	} else {
		if err = a.Transition(s, r.Service.now()); err != nil {
			return Attempt{}, err
		}
		a.ProviderRawStatus = raw
		if !a.Terminal() {
			a.ReconciliationState = ReconciliationRequired
		}
	}
	if err = r.Repository.Update(ctx, a); err != nil {
		return Attempt{}, err
	}
	if r.AfterTransition != nil {
		if err = r.AfterTransition(ctx, a); err != nil {
			return Attempt{}, err
		}
	}
	return a, nil
}

func QueryProviderStatus(ctx context.Context, provider StatusProvider, operation OperationType, id string) (StatusDetails, error) {
	if specialized, ok := provider.(OperationStatusDetailsProvider); ok {
		return specialized.GetOperationStatusDetails(ctx, operation, id)
	}
	if operation == OperationPayment || operation == OperationRenewal {
		if detailed, ok := provider.(StatusDetailsProvider); ok {
			return detailed.GetStatusDetails(ctx, id)
		}
	}
	if specialized, ok := provider.(OperationStatusProvider); ok {
		status, raw, err := specialized.GetOperationStatus(ctx, operation, id)
		return StatusDetails{Status: status, RawStatus: raw}, err
	}
	status, raw, err := provider.GetStatus(ctx, id)
	return StatusDetails{Status: status, RawStatus: raw}, err
}
