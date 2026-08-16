package payments

import (
	"context"
	"errors"
	"fmt"
)

// PurchaseRequest is the provider-neutral boundary used by PRO and other
// platform purchases. Card data never crosses this boundary: SavedMethodRef is
// an opaque provider token/reference only.
type PurchaseRequest struct {
	AmountKopecks       int64
	Currency            string
	InternalReferenceID string
	IdempotencyKey      string
	Description         string
	ReturnURL           string
	FailURL             string
	SavedMethodRef      string
	SavePaymentMethod   bool
	CustomerID          string
	BillingPeriod       string
}

type PurchaseResult struct {
	ExternalID      string
	Status          Status
	RawStatus       string
	ConfirmationURL string
	SavedMethodRef  string
}

type PurchaseProvider interface {
	CreatePurchase(context.Context, PurchaseRequest) (PurchaseResult, error)
	GetStatus(context.Context, string) (Status, string, error)
	RefundPurchase(context.Context, string, string, int64) (string, error)
}

type RecurringProvider interface {
	ChargeRecurring(context.Context, PurchaseRequest) (PurchaseResult, error)
}

// CancellationProvider is optional because not every PSP exposes a safe
// server-side cancellation primitive for every payment state. Implementations
// must return an authoritative normalized state; callers must not infer
// cancellation from a browser redirect or from a transport-level 2xx alone.
type CancellationProvider interface {
	CancelPurchase(context.Context, string, string, int64) (Status, string, error)
}

type ProviderSet struct {
	Purchases map[ProviderName]PurchaseProvider
	Recurring map[ProviderName]RecurringProvider
	Webhooks  map[ProviderName]WebhookVerifier
	Statuses  map[ProviderName]StatusProvider
	Runtime   *ProviderRuntime
}

func (s ProviderSet) Purchase(name ProviderName) (PurchaseProvider, error) {
	if s.Runtime != nil {
		return s.Runtime.Purchase(name)
	}
	p := s.Purchases[name]
	if p == nil {
		return nil, ErrProviderUnavailable
	}
	return p, nil
}
func (s ProviderSet) RecurringCharge(name ProviderName) (RecurringProvider, error) {
	if s.Runtime != nil {
		return s.Runtime.Recurring(name)
	}
	p := s.Recurring[name]
	if p == nil {
		return nil, ErrUnsupportedRoute
	}
	return p, nil
}

func (s ProviderSet) Status(name ProviderName) (StatusProvider, error) {
	if s.Runtime != nil {
		return s.Runtime.Status(name)
	}
	p := s.Statuses[name]
	if p == nil {
		return nil, ErrProviderUnavailable
	}
	return p, nil
}

func (s ProviderSet) Webhook(name ProviderName) (WebhookVerifier, error) {
	if s.Runtime != nil {
		return s.Runtime.Webhook(name)
	}
	p := s.Webhooks[name]
	if p == nil {
		return nil, ErrProviderUnavailable
	}
	return p, nil
}

func NormalizeProviderStatus(status Status) error {
	switch status {
	case StatusCreated, StatusPendingUserAction, StatusProcessing, StatusAuthorized,
		StatusSucceeded, StatusFailed, StatusCanceled, StatusRefunded,
		StatusPartiallyRefunded, StatusUnknownReconciliation:
		return nil
	default:
		return errors.New("unsupported normalized provider status")
	}
}

func purchaseResult(external string, status Status, raw, url, method string) (PurchaseResult, error) {
	if external == "" {
		return PurchaseResult{}, fmt.Errorf("provider returned empty operation id")
	}
	if err := NormalizeProviderStatus(status); err != nil {
		return PurchaseResult{}, err
	}
	return PurchaseResult{ExternalID: external, Status: status, RawStatus: raw, ConfirmationURL: url, SavedMethodRef: method}, nil
}
