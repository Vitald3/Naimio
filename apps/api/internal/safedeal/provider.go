package safedeal

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"freelance/apps/api/internal/payments"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CreateFundingRequest struct {
	DealID                   string
	AmountKopecks            int64
	PayoutKopecks            int64
	Currency, IdempotencyKey string
}
type CreateFundingResult struct{ Provider, ProviderPaymentID, Status, CheckoutURL string }
type PaymentState struct{ Status string }
type ReleaseRequest struct {
	ProviderPaymentID        string
	AmountKopecks            int64
	PayoutKopecks            int64
	Currency, IdempotencyKey string
}
type ReleaseResult struct{ Status, ProviderOperationID string }
type RefundRequest struct {
	ProviderPaymentID        string
	AmountKopecks            int64
	PayoutKopecks            int64
	Currency, IdempotencyKey string
}
type RefundResult struct{ Status, ProviderOperationID string }
type CancelPaymentRequest struct{ ProviderPaymentID, IdempotencyKey string }
type VerifiedProviderEvent struct {
	Provider, ProviderEventID, ProviderPaymentID, Type, State, Currency string
	AmountKopecks                                                       int64
	Verified                                                            bool
	OccurredAt                                                          time.Time
	Payload                                                             map[string]any
}

// Capabilities describes what a payment/safe-deal provider can do. The domain
// consults these flags before attempting an operation and fails safely with
// ErrUnsupported instead of assuming a provider can do something it cannot.
// This keeps the business logic provider-independent: adding TBank, YooKassa
// or any other provider later only changes which flags are true, never the
// domain flow.
type Capabilities struct {
	Funding         bool     `json:"funding"`          // can create/hold incoming funding
	SplitSettlement bool     `json:"split_settlement"` // can settle to multiple payees natively
	HoldFunds       bool     `json:"hold_funds"`       // supports deferred capture / escrow hold
	Release         bool     `json:"release"`          // can release held funds to the freelancer
	Refund          bool     `json:"refund"`           // can refund the customer
	PartialRefund   bool     `json:"partial_refund"`   // can refund a partial amount
	Payout          bool     `json:"payout"`           // can push a payout to an external account
	Webhooks        bool     `json:"webhooks"`         // emits verifiable webhooks
	Reconciliation  bool     `json:"reconciliation"`   // supports polling payment state
	PaymentMethods  []string `json:"payment_methods"`  // methods the provider accepts
}

type PaymentProvider interface {
	Capabilities() Capabilities
	CreateFunding(context.Context, CreateFundingRequest) (CreateFundingResult, error)
	GetPayment(context.Context, string) (PaymentState, error)
	CancelPayment(context.Context, CancelPaymentRequest) error
	Refund(context.Context, RefundRequest) (RefundResult, error)
	Release(context.Context, ReleaseRequest) (ReleaseResult, error)
	VerifyWebhook(context.Context, map[string][]string, []byte) (VerifiedProviderEvent, error)
}

// DisabledProvider keeps Safe Deal visible but makes every money-moving action
// fail closed until an audited real provider adapter is enabled.
type DisabledProvider struct{}

func (DisabledProvider) Capabilities() Capabilities { return Capabilities{} }
func (DisabledProvider) CreateFunding(context.Context, CreateFundingRequest) (CreateFundingResult, error) {
	return CreateFundingResult{}, ErrUnsupported
}
func (DisabledProvider) GetPayment(context.Context, string) (PaymentState, error) {
	return PaymentState{}, ErrUnsupported
}
func (DisabledProvider) CancelPayment(context.Context, CancelPaymentRequest) error {
	return ErrUnsupported
}
func (DisabledProvider) Refund(context.Context, RefundRequest) (RefundResult, error) {
	return RefundResult{}, ErrUnsupported
}
func (DisabledProvider) Release(context.Context, ReleaseRequest) (ReleaseResult, error) {
	return ReleaseResult{}, ErrUnsupported
}
func (DisabledProvider) VerifyWebhook(context.Context, map[string][]string, []byte) (VerifiedProviderEvent, error) {
	return VerifiedProviderEvent{}, ErrUnsupported
}

type SandboxProvider struct {
	Secret          []byte
	Now             func() time.Time
	AutoConfirm     bool
	DB              *sql.DB
	PaymentAttempts *payments.Service
	mu              sync.Mutex
	states          map[string]string
}

func NewSandboxProvider(secret string) *SandboxProvider {
	return &SandboxProvider{Secret: []byte(secret), states: map[string]string{}}
}

func NewAutoConfirmSandboxProvider(secret string) *SandboxProvider {
	p := NewSandboxProvider(secret)
	p.AutoConfirm = true
	return p
}

// Capabilities reports the sandbox adapter's abilities. It emulates a
// single-account escrow provider: it can fund, hold, release and refund, and
// emits signed webhooks, but does not natively split-settle, partial-refund or
// push external payouts. Split economics are therefore computed by the domain
// (CalculateDealQuote), not delegated to the provider.
func (p *SandboxProvider) Capabilities() Capabilities {
	return Capabilities{
		Funding:        true,
		HoldFunds:      true,
		Release:        true,
		Refund:         true,
		Webhooks:       true,
		Reconciliation: true,
		PaymentMethods: []string{MethodCard, MethodSBP},
	}
}
func (p *SandboxProvider) CreateFunding(ctx context.Context, r CreateFundingRequest) (CreateFundingResult, error) {
	if r.AmountKopecks <= 0 || r.Currency != "RUB" || len(r.IdempotencyKey) < 8 {
		return CreateFundingResult{}, ErrInvalid
	}

	// Debug uses a real Phase 11 payment attempt too. Older development builds
	// could leave an unstarted real-provider attempt (CREATED with no external
	// operation) for the same Safe Deal. It is safe to close only that exact
	// no-side-effect state before creating the local sandbox operation.
	if p.DB != nil {
		_, _ = p.DB.ExecContext(ctx, `UPDATE payment_attempts
SET status='CANCELED', error_category='DEBUG_REPLACED_UNSTARTED_ATTEMPT', reconciliation_state='NOT_REQUIRED', terminal_at=now(), updated_at=now()
WHERE domain='SAFE_DEAL' AND internal_reference_id=$1::uuid AND operation_type='PAYMENT'
  AND status='CREATED' AND provider_operation_id IS NULL AND provider<>'sandbox'`, r.DealID)
	}

	providerKey := r.IdempotencyKey
	var attempt payments.Attempt
	if p.PaymentAttempts != nil {
		created, err := p.PaymentAttempts.Create(ctx, payments.Attempt{
			Domain:              payments.DomainSafeDeal,
			InternalReferenceID: r.DealID,
			Provider:            payments.ProviderName("sandbox"),
			OperationType:       payments.OperationPayment,
			AmountKopecks:       r.AmountKopecks,
			Currency:            r.Currency,
			IdempotencyKey:      r.IdempotencyKey,
			PaymentMethod:       MethodCard,
		})
		if errors.Is(err, payments.ErrAttemptConflict) && p.DB != nil {
			existing, ok, lookupErr := (payments.PostgresRepository{DB: p.DB}).FindOpenSafeDealOperation(ctx, r.DealID, payments.OperationPayment)
			if lookupErr == nil && ok && existing.Provider == payments.ProviderName("sandbox") && existing.AmountKopecks == r.AmountKopecks {
				if existing.ProviderOperationID != "" {
					return CreateFundingResult{Provider: "sandbox", ProviderPaymentID: existing.ProviderOperationID, Status: existing.ProviderRawStatus, CheckoutURL: existing.ConfirmationURL}, nil
				}
				// A previous attempt may have claimed the database row and then died
				// before the local provider operation was persisted. Reuse that exact
				// attempt instead of creating a second financial operation.
				attempt = existing
				providerKey = existing.IdempotencyKey
				err = nil
			}
		}
		if err != nil {
			return CreateFundingResult{}, err
		}
		if attempt.ID == "" {
			attempt = created
		}
		if attempt.ProviderOperationID != "" {
			return CreateFundingResult{Provider: "sandbox", ProviderPaymentID: attempt.ProviderOperationID, Status: attempt.ProviderRawStatus, CheckoutURL: attempt.ConfirmationURL}, nil
		}
	}

	id := "sb_pay_" + digest(r.DealID + ":" + providerKey)[:24]
	status := "PENDING"
	if p.AutoConfirm {
		status = "FUNDED"
	}
	checkoutURL := "/sandbox/payments/" + id
	if p.AutoConfirm {
		checkoutURL = ""
	}

	p.mu.Lock()
	if _, ok := p.states[id]; !ok {
		p.states[id] = status
	}
	p.mu.Unlock()

	if p.PaymentAttempts != nil {
		attempt.ProviderOperationID = id
		attempt.ConfirmationURL = checkoutURL
		attempt.ProviderRawStatus = status
		target := payments.StatusPendingUserAction
		if status == "FUNDED" {
			target = payments.StatusSucceeded
		}
		if attempt.Status != target {
			if err := attempt.Transition(target, time.Now().UTC()); err != nil {
				return CreateFundingResult{}, err
			}
		}
		if err := p.PaymentAttempts.Repository.Update(ctx, attempt); err != nil {
			return CreateFundingResult{}, err
		}
	}

	return CreateFundingResult{Provider: "sandbox", ProviderPaymentID: id, Status: status, CheckoutURL: checkoutURL}, nil
}
func (p *SandboxProvider) ConfirmPayment(id string) error {
	if id == "" {
		return errors.New("not found")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	// The debug checkout survives API restarts because its payment record is
	// persisted in PostgreSQL, while this provider intentionally keeps only an
	// in-memory state map. The dev confirmation endpoint validates that record
	// before calling us, so recreating a missing in-memory entry here is safe and
	// makes local checkout idempotent across rebuilds/restarts.
	p.states[id] = "FUNDED"
	return nil
}

func (p *SandboxProvider) GetPayment(_ context.Context, id string) (PaymentState, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	v, ok := p.states[id]
	if !ok {
		return PaymentState{}, errors.New("not found")
	}
	return PaymentState{Status: v}, nil
}

func (p *SandboxProvider) GetStatus(ctx context.Context, id string) (payments.Status, string, error) {
	state, err := p.GetPayment(ctx, id)
	raw := ""
	if err == nil {
		raw = state.Status
	} else if p.DB != nil {
		// The in-memory emulator is intentionally ephemeral. payment_records is
		// the durable debug source of truth after an API rebuild/restart.
		if dbErr := p.DB.QueryRowContext(ctx, `SELECT provider_status FROM payment_records WHERE provider='sandbox' AND provider_payment_id=$1 ORDER BY updated_at DESC LIMIT 1`, id).Scan(&raw); dbErr != nil {
			return payments.StatusUnknownReconciliation, "", err
		}
	} else {
		return payments.StatusUnknownReconciliation, "", err
	}
	raw = strings.ToUpper(strings.TrimSpace(raw))
	switch raw {
	case "FUNDED", "RELEASED":
		return payments.StatusSucceeded, raw, nil
	case "CANCELED":
		return payments.StatusCanceled, raw, nil
	case "REFUNDED":
		return payments.StatusRefunded, raw, nil
	case "FAILED":
		return payments.StatusFailed, raw, nil
	case "PENDING":
		return payments.StatusPendingUserAction, raw, nil
	case "RELEASE_PENDING", "REFUND_PENDING", "CANCEL_PENDING":
		return payments.StatusProcessing, raw, nil
	default:
		return payments.StatusUnknownReconciliation, raw, nil
	}
}

func (p *SandboxProvider) persistedState(ctx context.Context, id string) (string, bool) {
	p.mu.Lock()
	state, ok := p.states[id]
	p.mu.Unlock()
	if ok {
		return state, true
	}
	if p.DB == nil || strings.TrimSpace(id) == "" {
		return "", false
	}
	if err := p.DB.QueryRowContext(ctx, `SELECT provider_status FROM payment_records WHERE provider='sandbox' AND provider_payment_id=$1 ORDER BY updated_at DESC LIMIT 1`, id).Scan(&state); err != nil {
		return "", false
	}
	p.mu.Lock()
	p.states[id] = state
	p.mu.Unlock()
	return state, true
}

func (p *SandboxProvider) persistState(ctx context.Context, id, state string) error {
	p.mu.Lock()
	p.states[id] = state
	p.mu.Unlock()
	if p.DB == nil {
		return nil
	}
	_, err := p.DB.ExecContext(ctx, `UPDATE payment_records SET provider_status=$2,updated_at=now() WHERE provider='sandbox' AND provider_payment_id=$1`, id, state)
	return err
}

func (p *SandboxProvider) CancelPayment(ctx context.Context, r CancelPaymentRequest) error {
	state, ok := p.persistedState(ctx, r.ProviderPaymentID)
	if !ok {
		return errors.New("not found")
	}
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "CANCELED", "CANCEL_PENDING":
		return nil
	case "PENDING", "FUNDED":
		return p.persistState(ctx, r.ProviderPaymentID, "CANCEL_PENDING")
	default:
		return ErrInvalidState
	}
}
func (p *SandboxProvider) Refund(ctx context.Context, r RefundRequest) (RefundResult, error) {
	if r.AmountKopecks <= 0 || r.Currency != "RUB" {
		return RefundResult{}, ErrInvalid
	}
	state, ok := p.persistedState(ctx, r.ProviderPaymentID)
	if !ok {
		return RefundResult{}, errors.New("not found")
	}
	opID := "sb_ref_" + digest(r.ProviderPaymentID + ":" + r.IdempotencyKey)[:24]
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "REFUNDED", "REFUND_PENDING":
		return RefundResult{Status: "PENDING", ProviderOperationID: opID}, nil
	case "FUNDED", "RELEASE_PENDING":
		if err := p.persistState(ctx, r.ProviderPaymentID, "REFUND_PENDING"); err != nil {
			return RefundResult{}, err
		}
		return RefundResult{Status: "PENDING", ProviderOperationID: opID}, nil
	default:
		return RefundResult{}, ErrInvalidState
	}
}
func (p *SandboxProvider) Release(ctx context.Context, r ReleaseRequest) (ReleaseResult, error) {
	if r.AmountKopecks < 0 || r.Currency != "RUB" {
		return ReleaseResult{}, ErrInvalid
	}
	state, ok := p.persistedState(ctx, r.ProviderPaymentID)
	if !ok {
		return ReleaseResult{}, errors.New("not found")
	}
	opID := "sb_rel_" + digest(r.ProviderPaymentID + ":" + r.IdempotencyKey)[:24]
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "RELEASED", "RELEASE_PENDING":
		return ReleaseResult{Status: "PENDING", ProviderOperationID: opID}, nil
	case "FUNDED":
		if err := p.persistState(ctx, r.ProviderPaymentID, "RELEASE_PENDING"); err != nil {
			return ReleaseResult{}, err
		}
		return ReleaseResult{Status: "PENDING", ProviderOperationID: opID}, nil
	default:
		return ReleaseResult{}, ErrInvalidState
	}
}
func (p *SandboxProvider) VerifyWebhook(_ context.Context, h map[string][]string, body []byte) (VerifiedProviderEvent, error) {
	if len(body) > 256<<10 || len(p.Secret) < 16 {
		return VerifiedProviderEvent{}, ErrForbidden
	}
	timestamp := first(h["X-Sandbox-Timestamp"])
	signature := first(h["X-Sandbox-Signature"])
	seconds, e := strconv.ParseInt(timestamp, 10, 64)
	if e != nil {
		return VerifiedProviderEvent{}, ErrForbidden
	}
	now := time.Now().UTC()
	if p.Now != nil {
		now = p.Now().UTC()
	}
	occurred := time.Unix(seconds, 0).UTC()
	if occurred.Before(now.Add(-5*time.Minute)) || occurred.After(now.Add(5*time.Minute)) {
		return VerifiedProviderEvent{}, ErrForbidden
	}
	mac := hmac.New(sha256.New, p.Secret)
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return VerifiedProviderEvent{}, ErrForbidden
	}
	var in struct {
		ID            string `json:"id"`
		PaymentID     string `json:"payment_id"`
		Type          string `json:"type"`
		State         string `json:"state"`
		Currency      string `json:"currency"`
		AmountKopecks int64  `json:"amount_kopecks"`
	}
	if json.Unmarshal(body, &in) != nil || in.ID == "" || in.PaymentID == "" || !oneOf(in.Type, "FUNDING_CONFIRMED", "RELEASE_CONFIRMED", "REFUND_CONFIRMED", "CANCEL_CONFIRMED") || !oneOf(in.State, "FUNDED", "RELEASED", "REFUNDED", "CANCELED") || stateEvent(in.State) != in.Type || in.Currency != "RUB" || in.AmountKopecks <= 0 {
		return VerifiedProviderEvent{}, ErrForbidden
	}
	var safe map[string]any
	_ = json.Unmarshal(body, &safe)
	p.mu.Lock()
	p.states[in.PaymentID] = in.State
	p.mu.Unlock()
	return VerifiedProviderEvent{Provider: "sandbox", ProviderEventID: in.ID, ProviderPaymentID: in.PaymentID, Type: in.Type, State: in.State, Currency: in.Currency, AmountKopecks: in.AmountKopecks, Verified: true, OccurredAt: occurred, Payload: safe}, nil
}
func digest(v string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(v))) }
func first(v []string) string {
	if len(v) == 0 {
		return ""
	}
	return v[0]
}
