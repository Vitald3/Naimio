package payments

import (
	"errors"
	"strings"
	"time"
)

type Status string

const (
	StatusCreated               Status = "CREATED"
	StatusPendingUserAction     Status = "PENDING_USER_ACTION"
	StatusProcessing            Status = "PROCESSING"
	StatusAuthorized            Status = "AUTHORIZED"
	StatusSucceeded             Status = "SUCCEEDED"
	StatusFailed                Status = "FAILED"
	StatusCanceled              Status = "CANCELED"
	StatusRefunded              Status = "REFUNDED"
	StatusPartiallyRefunded     Status = "PARTIALLY_REFUNDED"
	StatusUnknownReconciliation Status = "UNKNOWN_REQUIRES_RECONCILIATION"
)

type OperationType string

const (
	OperationPayment OperationType = "PAYMENT"
	OperationRenewal OperationType = "RENEWAL"
	OperationRefund  OperationType = "REFUND"
	OperationPayout  OperationType = "PAYOUT"
	OperationCapture OperationType = "CAPTURE"
	OperationVoid    OperationType = "VOID"
)

type ReconciliationState string

const (
	ReconciliationNotRequired ReconciliationState = "NOT_REQUIRED"
	ReconciliationRequired    ReconciliationState = "REQUIRED"
	ReconciliationInProgress  ReconciliationState = "IN_PROGRESS"
	ReconciliationReconciled  ReconciliationState = "RECONCILED"
	ReconciliationFailed      ReconciliationState = "FAILED"
)

var (
	ErrInvalidAttempt    = errors.New("invalid payment attempt")
	ErrInvalidTransition = errors.New("invalid payment attempt transition")
)

type Attempt struct {
	ID                       string              `json:"id"`
	Domain                   Domain              `json:"domain"`
	InternalReferenceID      string              `json:"-"`
	Provider                 ProviderName        `json:"provider"`
	OperationType            OperationType       `json:"operation_type"`
	Status                   Status              `json:"status"`
	AmountKopecks            int64               `json:"amount_kopecks"`
	Currency                 string              `json:"currency"`
	IdempotencyKey           string              `json:"-"`
	PaymentMethod            string              `json:"payment_method,omitempty"`
	ProviderOperationID      string              `json:"-"`
	ProviderPaymentMethodRef string              `json:"-"`
	ConfirmationURL          string              `json:"confirmation_url,omitempty"`
	ProviderRawStatus        string              `json:"-"`
	ErrorCategory            string              `json:"error_category,omitempty"`
	ReconciliationState      ReconciliationState `json:"reconciliation_state"`
	CreatedAt                time.Time           `json:"created_at"`
	UpdatedAt                time.Time           `json:"updated_at"`
	TerminalAt               *time.Time          `json:"terminal_at,omitempty"`
}

func (a *Attempt) Normalize() error {
	a.Currency = strings.ToUpper(strings.TrimSpace(a.Currency))
	a.PaymentMethod = strings.ToUpper(strings.TrimSpace(a.PaymentMethod))
	if a.Domain != DomainSafeDeal && a.Domain != DomainProSubscription && a.Domain != DomainPlatformPayment || a.InternalReferenceID == "" || a.Provider == "" || a.Provider == ProviderDisabled || a.AmountKopecks <= 0 || a.Currency != "RUB" || len(a.IdempotencyKey) < 8 {
		return ErrInvalidAttempt
	}
	return nil
}
func (a Attempt) Terminal() bool {
	return a.Status == StatusSucceeded || a.Status == StatusFailed || a.Status == StatusCanceled || a.Status == StatusRefunded || a.Status == StatusPartiallyRefunded
}
func (a *Attempt) Transition(to Status, now time.Time) error {
	if a.Terminal() || !allowed[a.Status][to] {
		return ErrInvalidTransition
	}
	a.Status = to
	a.UpdatedAt = now.UTC()
	if to == StatusUnknownReconciliation {
		a.ReconciliationState = ReconciliationRequired
	}
	if a.Terminal() {
		a.ReconciliationState = ReconciliationNotRequired
		a.TerminalAt = &a.UpdatedAt
	}
	return nil
}

var allowed = map[Status]map[Status]bool{StatusCreated: {StatusPendingUserAction: true, StatusProcessing: true, StatusSucceeded: true, StatusFailed: true, StatusCanceled: true, StatusRefunded: true, StatusPartiallyRefunded: true, StatusUnknownReconciliation: true}, StatusPendingUserAction: {StatusProcessing: true, StatusAuthorized: true, StatusSucceeded: true, StatusFailed: true, StatusCanceled: true, StatusRefunded: true, StatusPartiallyRefunded: true, StatusUnknownReconciliation: true}, StatusProcessing: {StatusAuthorized: true, StatusSucceeded: true, StatusFailed: true, StatusCanceled: true, StatusRefunded: true, StatusPartiallyRefunded: true, StatusUnknownReconciliation: true}, StatusAuthorized: {StatusSucceeded: true, StatusCanceled: true, StatusRefunded: true, StatusPartiallyRefunded: true, StatusUnknownReconciliation: true}, StatusUnknownReconciliation: {StatusProcessing: true, StatusAuthorized: true, StatusSucceeded: true, StatusFailed: true, StatusCanceled: true, StatusRefunded: true, StatusPartiallyRefunded: true}}
