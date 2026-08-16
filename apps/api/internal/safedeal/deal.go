package safedeal

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
)

var (
	ErrNotFound       = errors.New("safe deal not found")
	ErrInvalid        = errors.New("invalid safe deal input")
	ErrInvalidState   = errors.New("invalid safe deal state")
	ErrForbidden      = errors.New("safe deal action forbidden")
	ErrConflict       = errors.New("safe deal idempotency conflict")
	ErrProvider       = errors.New("payment provider unavailable")
	ErrUnsupported    = errors.New("payment provider operation unsupported")
	validID           = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	validIdempotency  = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,160}$`)
	disputeReasonCode = map[string]bool{"WORK_NOT_DELIVERED": true, "WORK_DOES_NOT_MATCH_SCOPE": true, "DEADLINE_MISSED": true, "CUSTOMER_UNRESPONSIVE": true, "UNREASONABLE_REVISION": true, "FRAUD_SUSPECTED": true, "MUTUAL_CANCELLATION": true, "OTHER": true}
)

type Deal struct {
	ID               string `json:"id"`
	ProjectID        string `json:"project_id"`
	ProjectTitle     string `json:"project_title,omitempty"`
	AssignmentID     string `json:"assignment_id"`
	CustomerUserID   string `json:"-"`
	FreelancerUserID string `json:"-"`
	CustomerName     string `json:"-"`
	FreelancerName   string `json:"-"`
	CounterpartyName string `json:"counterparty_name,omitempty"`
	Currency         string `json:"currency"`
	// Legacy aliases kept for API compatibility. They are derived from Quote:
	//   GrossAmountKopecks      == Quote.CustomerTotalKopecks (the funding charge)
	//   PlatformFeeKopecks      == Quote.PlatformGrossRevenueKopecks (total commission)
	//   FreelancerAmountKopecks == Quote.FreelancerPayoutKopecks (the release amount)
	GrossAmountKopecks       int64         `json:"gross_amount_kopecks"`
	PlatformFeeKopecks       int64         `json:"platform_fee_kopecks"`
	FreelancerAmountKopecks  int64         `json:"freelancer_amount_kopecks"`
	Quote                    DealQuote     `json:"quote"`
	Status                   string        `json:"status"`
	RevisionCount            int           `json:"revision_count"`
	Payment                  *Payment      `json:"payment,omitempty"`
	Submission               *Submission   `json:"submission,omitempty"`
	Dispute                  *Dispute      `json:"dispute,omitempty"`
	FundedAt                 *time.Time    `json:"funded_at,omitempty"`
	WorkStartedAt            *time.Time    `json:"work_started_at,omitempty"`
	SubmittedAt              *time.Time    `json:"submitted_at,omitempty"`
	AcceptedAt               *time.Time    `json:"accepted_at,omitempty"`
	CompletedAt              *time.Time    `json:"completed_at,omitempty"`
	CreatedAt                time.Time     `json:"created_at"`
	UpdatedAt                time.Time     `json:"updated_at"`
	Events                   []DomainEvent `json:"events,omitempty"`
	ConversationID           string        `json:"conversation_id,omitempty"`
	Evidence                 []Evidence    `json:"evidence,omitempty"`
	ProviderOperational      bool          `json:"provider_operational"`
	ProviderCapabilityNotice string        `json:"provider_capability_notice,omitempty"`
	ProviderCapabilities     *Capabilities `json:"provider_capabilities,omitempty"`
	ViewerRole               string        `json:"viewer_role,omitempty"`
}

// ApplyQuote stores the authoritative quote snapshot on the deal and keeps the
// legacy money fields in sync. Funding, release and refund all read the legacy
// fields, so this is the single place that binds the economic model to the
// money-movement flow.
func (d *Deal) ApplyQuote(q DealQuote) {
	d.Quote = q
	d.Currency = q.Currency
	d.GrossAmountKopecks = q.CustomerTotalKopecks
	d.PlatformFeeKopecks = q.PlatformGrossRevenueKopecks
	d.FreelancerAmountKopecks = q.FreelancerPayoutKopecks
}

type Payment struct {
	ID                string    `json:"id"`
	DealID            string    `json:"deal_id"`
	Provider          string    `json:"provider"`
	ProviderPaymentID string    `json:"provider_payment_id"`
	ProviderStatus    string    `json:"provider_status"`
	CheckoutURL       string    `json:"checkout_url,omitempty"`
	Currency          string    `json:"currency"`
	IdempotencyKey    string    `json:"-"`
	AmountKopecks     int64     `json:"amount_kopecks"`
	UpdatedAt         time.Time `json:"updated_at"`
}
type Submission struct {
	ID                string    `json:"id"`
	DealID            string    `json:"deal_id"`
	SubmittedByUserID string    `json:"submitted_by_user_id"`
	Summary           string    `json:"summary"`
	MessageID         string    `json:"message_id,omitempty"`
	RevisionNumber    int       `json:"revision_number"`
	CreatedAt         time.Time `json:"created_at"`
}
type Dispute struct {
	ID               string     `json:"id"`
	DealID           string     `json:"deal_id"`
	OpenedByUserID   string     `json:"-"`
	ReasonCode       string     `json:"reason_code"`
	Description      string     `json:"description"`
	Status           string     `json:"status"`
	Resolution       string     `json:"resolution,omitempty"`
	ResolutionReason string     `json:"resolution_reason,omitempty"`
	OpenedAt         time.Time  `json:"opened_at"`
	ResolvedAt       *time.Time `json:"resolved_at,omitempty"`
	Evidence         []Evidence `json:"evidence,omitempty"`
}
type Evidence struct {
	ID           string    `json:"id"`
	DisputeID    string    `json:"dispute_id"`
	AuthorUserID string    `json:"author_user_id"`
	Kind         string    `json:"kind"`
	Body         string    `json:"body"`
	ReferenceID  string    `json:"reference_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}
type DomainEvent struct {
	Type        string            `json:"type"`
	ActorUserID string            `json:"actor_user_id,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}
type SubmitInput struct{ Summary, MessageID string }
type DisputeInput struct{ ReasonCode, Description string }
type EvidenceInput struct{ Kind, Body, ReferenceID string }
type ResolutionInput struct{ Outcome, Reason string }

type Repository interface {
	List(context.Context, string, string) ([]Deal, error)
	Get(context.Context, string, string, bool) (Deal, error)
	SaveFunding(context.Context, string, string, string, CreateFundingResult) (Deal, error)
	Start(context.Context, string, string, string) (Deal, error)
	Submit(context.Context, string, string, string, SubmitInput) (Deal, error)
	Revision(context.Context, string, string, string, string) (Deal, error)
	OpenDispute(context.Context, string, string, string, DisputeInput) (Deal, error)
	AddEvidence(context.Context, string, string, string, EvidenceInput) (Deal, error)
	PrepareAccept(context.Context, string, string, string) (Deal, error)
	MarkReleasePending(context.Context, string, string, ReleaseResult, bool) (Deal, error)
	CancelUnfunded(context.Context, string, string, string) (Deal, error)
	MarkCancelPending(context.Context, string, string) (Deal, error)
	MarkRefundPending(context.Context, string, string, RefundResult, bool) (Deal, error)
	ApplyProviderEvent(context.Context, VerifiedProviderEvent) (Deal, bool, error)
	AdminList(context.Context, string) ([]Deal, error)
	AdminDeleteUnfunded(context.Context, string, string, string) error
	PrepareResolution(context.Context, string, string, string, ResolutionInput) (Deal, error)
	Unresolved(context.Context) ([]Payment, error)
	// ActiveEconomics returns the currently enabled platform fee policy and the
	// provider pricing for the given payment method. The provider payer mode is
	// carried on the fee rule (the pricing table holds pure cost), so the two are
	// returned together. When no pricing row exists for the method the provider
	// cost is zero, mirroring the assignment trigger.
	ActiveEconomics(ctx context.Context, paymentMethod string) (FeePolicy, ProviderPricing, error)
}

type Service struct {
	Repository Repository
	Provider   PaymentProvider
}

func (s Service) List(ctx context.Context, actor, project string) ([]Deal, error) {
	if actor == "" || project != "" && !validID.MatchString(project) {
		return nil, ErrInvalid
	}
	items, err := s.Repository.List(ctx, actor, project)
	for i := range items {
		if items[i].CustomerUserID == actor {
			items[i].ViewerRole = "CUSTOMER"
			items[i].CounterpartyName = items[i].FreelancerName
		} else if items[i].FreelancerUserID == actor {
			items[i].ViewerRole = "FREELANCER"
			items[i].CounterpartyName = items[i].CustomerName
		}
		s.attachCapabilities(&items[i])
	}
	return items, err
}
func (s Service) Get(ctx context.Context, actor, id string) (Deal, error) {
	if actor == "" || !validID.MatchString(id) {
		return Deal{}, ErrNotFound
	}
	d, err := s.Repository.Get(ctx, actor, id, false)
	if d.CustomerUserID == actor {
		d.ViewerRole = "CUSTOMER"
		d.CounterpartyName = d.FreelancerName
	} else if d.FreelancerUserID == actor {
		d.ViewerRole = "FREELANCER"
		d.CounterpartyName = d.CustomerName
	}
	s.attachCapabilities(&d)
	return d, err
}

// attachCapabilities exposes the configured provider's capability flags on the
// deal so the UI can explain what is possible without hardcoding a provider.
func (s Service) attachCapabilities(d *Deal) {
	if s.Provider == nil {
		return
	}
	caps := s.Provider.Capabilities()
	d.ProviderCapabilities = &caps
}

// maxQuoteWorkKopecks bounds the work amount a quote may be requested for. It
// is far above any realistic project (10 billion RUB) yet low enough that
// work*basisPoints can never overflow int64, so CalculateDealQuote is always safe.
const maxQuoteWorkKopecks = 1_000_000_000_000

// Quote returns the authoritative money breakdown for a hypothetical deal of the
// given work amount and payment method, using the currently enabled fee policy
// and provider pricing. It is the single source of truth the customer and
// freelancer UIs read before a deal exists — the frontend never computes money.
func (s Service) Quote(ctx context.Context, workAmountKopecks int64, paymentMethod string) (DealQuote, error) {
	if workAmountKopecks <= 0 || workAmountKopecks > maxQuoteWorkKopecks {
		return DealQuote{}, ErrInvalid
	}
	paymentMethod = strings.ToUpper(strings.TrimSpace(paymentMethod))
	if paymentMethod == "" {
		paymentMethod = MethodCard
	}
	if !validPaymentMethod(paymentMethod) {
		return DealQuote{}, ErrInvalid
	}
	fee, pricing, err := s.Repository.ActiveEconomics(ctx, paymentMethod)
	if err != nil {
		return DealQuote{}, err
	}
	return CalculateDealQuote(QuoteInput{WorkAmountKopecks: workAmountKopecks, Currency: "RUB", Fee: fee, Provider: pricing})
}

func (s Service) Funding(ctx context.Context, actor, id, key string) (Deal, error) {
	if !commandInput(actor, id, key) {
		return Deal{}, ErrInvalid
	}
	d, err := s.Repository.Get(ctx, actor, id, false)
	if err != nil {
		return Deal{}, err
	}
	if d.CustomerUserID != actor {
		return Deal{}, ErrNotFound
	}
	if d.Status != "AWAITING_FUNDING" {
		return Deal{}, ErrInvalidState
	}
	if s.Provider == nil {
		return Deal{}, ErrProvider
	}
	if !s.Provider.Capabilities().Funding {
		return Deal{}, ErrUnsupported
	}
	result, err := s.Provider.CreateFunding(ctx, CreateFundingRequest{DealID: id, AmountKopecks: d.GrossAmountKopecks, PayoutKopecks: d.FreelancerAmountKopecks, Currency: d.Currency, IdempotencyKey: key})
	if err != nil {
		return Deal{}, ErrProvider
	}
	saved, err := s.Repository.SaveFunding(ctx, actor, id, key, result)
	if err != nil {
		return Deal{}, err
	}
	if result.Provider == "sandbox" && result.Status == "FUNDED" {
		eventID := "sandbox-funding-" + result.ProviderPaymentID
		confirmed, _, applyErr := s.Repository.ApplyProviderEvent(ctx, VerifiedProviderEvent{Provider: "sandbox", ProviderEventID: eventID, ProviderPaymentID: result.ProviderPaymentID, Type: "FUNDING_CONFIRMED", State: "FUNDED", Currency: d.Currency, AmountKopecks: d.GrossAmountKopecks, Verified: true, OccurredAt: time.Now().UTC(), Payload: map[string]any{"source": "sandbox_auto_confirm"}})
		if applyErr != nil && !errors.Is(applyErr, ErrInvalidState) {
			return Deal{}, applyErr
		}
		if confirmed.ID != "" {
			return confirmed, nil
		}
	}
	return saved, nil
}
func (s Service) Start(ctx context.Context, actor, id, key string) (Deal, error) {
	if !commandInput(actor, id, key) {
		return Deal{}, ErrInvalid
	}
	return s.Repository.Start(ctx, actor, id, key)
}
func (s Service) Submit(ctx context.Context, actor, id, key string, in SubmitInput) (Deal, error) {
	in.Summary = strings.TrimSpace(in.Summary)
	if !commandInput(actor, id, key) || len([]rune(in.Summary)) < 1 || len([]rune(in.Summary)) > 5000 || in.MessageID != "" && !validID.MatchString(in.MessageID) {
		return Deal{}, ErrInvalid
	}
	return s.Repository.Submit(ctx, actor, id, key, in)
}
func (s Service) Revision(ctx context.Context, actor, id, key, reason string) (Deal, error) {
	reason = strings.TrimSpace(reason)
	if !commandInput(actor, id, key) || len([]rune(reason)) < 3 || len([]rune(reason)) > 2000 {
		return Deal{}, ErrInvalid
	}
	return s.Repository.Revision(ctx, actor, id, key, reason)
}
func (s Service) Dispute(ctx context.Context, actor, id, key string, in DisputeInput) (Deal, error) {
	in.ReasonCode = strings.ToUpper(strings.TrimSpace(in.ReasonCode))
	in.Description = strings.TrimSpace(in.Description)
	if !commandInput(actor, id, key) || !disputeReasonCode[in.ReasonCode] || len([]rune(in.Description)) < 3 || len([]rune(in.Description)) > 5000 {
		return Deal{}, ErrInvalid
	}
	return s.Repository.OpenDispute(ctx, actor, id, key, in)
}
func (s Service) Evidence(ctx context.Context, actor, dispute, key string, in EvidenceInput) (Deal, error) {
	in.Kind = strings.ToUpper(strings.TrimSpace(in.Kind))
	in.Body = strings.TrimSpace(in.Body)
	if actor == "" || !validID.MatchString(dispute) || !validIdempotency.MatchString(key) || !oneOf(in.Kind, "COMMENT", "MESSAGE_REFERENCE", "SUBMISSION_REFERENCE") || len([]rune(in.Body)) < 1 || len([]rune(in.Body)) > 5000 || in.Kind != "COMMENT" && !validID.MatchString(in.ReferenceID) {
		return Deal{}, ErrInvalid
	}
	return s.Repository.AddEvidence(ctx, actor, dispute, key, in)
}
func (s Service) Accept(ctx context.Context, actor, id, key string) (Deal, error) {
	if !commandInput(actor, id, key) {
		return Deal{}, ErrInvalid
	}
	d, err := s.Repository.PrepareAccept(ctx, actor, id, key)
	if err != nil {
		return Deal{}, err
	}
	if d.Status == "RELEASE_PENDING" || d.Status == "COMPLETED" {
		return d, nil
	}
	if s.Provider == nil || d.Payment == nil {
		return Deal{}, ErrProvider
	}
	if !s.Provider.Capabilities().Release {
		return Deal{}, ErrUnsupported
	}
	out, err := s.Provider.Release(ctx, ReleaseRequest{ProviderPaymentID: d.Payment.ProviderPaymentID, AmountKopecks: d.FreelancerAmountKopecks, Currency: d.Currency, IdempotencyKey: key})
	if err != nil {
		return Deal{}, ErrProvider
	}
	return s.Repository.MarkReleasePending(ctx, id, key, out, false)
}
func (s Service) Cancel(ctx context.Context, actor, id, key string) (Deal, error) {
	if !commandInput(actor, id, key) {
		return Deal{}, ErrInvalid
	}
	d, err := s.Repository.Get(ctx, actor, id, false)
	if err != nil {
		return Deal{}, err
	}
	if d.CustomerUserID != actor {
		return Deal{}, ErrNotFound
	}
	if d.Status == "AWAITING_FUNDING" && (d.Payment == nil || strings.TrimSpace(d.Payment.ProviderPaymentID) == "") {
		// No provider-side operation exists, so there are no funds to reverse.
		// Treat this as an unfunded cancellation instead of calling a PSP with
		// an empty external id and surfacing PAYMENT_PROVIDER_UNAVAILABLE.
		return s.Repository.CancelUnfunded(ctx, actor, id, key)
	}
	if d.Status == "AWAITING_FUNDING" && d.Payment != nil {
		if s.Provider == nil {
			return Deal{}, ErrProvider
		}
		if !s.Provider.Capabilities().Funding {
			return Deal{}, ErrUnsupported
		}
		if err := s.Provider.CancelPayment(ctx, CancelPaymentRequest{ProviderPaymentID: d.Payment.ProviderPaymentID, IdempotencyKey: key}); err != nil {
			return Deal{}, ErrProvider
		}
		return s.Repository.MarkCancelPending(ctx, id, key)
	}
	if d.Status != "FUNDED" || d.Payment == nil {
		return Deal{}, ErrInvalidState
	}
	if s.Provider == nil {
		return Deal{}, ErrProvider
	}
	if !s.Provider.Capabilities().Refund {
		return Deal{}, ErrUnsupported
	}
	out, err := s.Provider.Refund(ctx, RefundRequest{ProviderPaymentID: d.Payment.ProviderPaymentID, AmountKopecks: d.GrossAmountKopecks, Currency: d.Currency, IdempotencyKey: key})
	if err != nil {
		return Deal{}, ErrProvider
	}
	return s.Repository.MarkRefundPending(ctx, id, key, out, false)
}
func (s Service) Webhook(ctx context.Context, headers map[string][]string, body []byte) (Deal, bool, error) {
	if s.Provider == nil {
		return Deal{}, false, ErrProvider
	}
	if !s.Provider.Capabilities().Webhooks {
		return Deal{}, false, ErrUnsupported
	}
	event, err := s.Provider.VerifyWebhook(ctx, headers, body)
	if err != nil {
		return Deal{}, false, ErrForbidden
	}
	return s.Repository.ApplyProviderEvent(ctx, event)
}
func (s Service) AdminList(ctx context.Context, actor string) ([]Deal, error) {
	if actor == "" {
		return nil, ErrForbidden
	}
	return s.Repository.AdminList(ctx, actor)
}
func (s Service) AdminDeleteUnfunded(ctx context.Context, actor, id, reason string) error {
	reason = strings.TrimSpace(reason)
	if actor == "" || !validID.MatchString(id) || len([]rune(reason)) < 3 || len([]rune(reason)) > 2000 {
		return ErrInvalid
	}
	return s.Repository.AdminDeleteUnfunded(ctx, actor, id, reason)
}

func (s Service) Resolve(ctx context.Context, actor, id, key string, in ResolutionInput) (Deal, error) {
	in.Outcome = strings.ToUpper(strings.TrimSpace(in.Outcome))
	in.Reason = strings.TrimSpace(in.Reason)
	if !commandInput(actor, id, key) || !oneOf(in.Outcome, "CUSTOMER", "FREELANCER") || len([]rune(in.Reason)) < 3 || len([]rune(in.Reason)) > 2000 {
		return Deal{}, ErrInvalid
	}
	d, err := s.Repository.PrepareResolution(ctx, actor, id, key, in)
	if err != nil {
		return Deal{}, err
	}
	if d.Payment == nil || s.Provider == nil {
		return Deal{}, ErrProvider
	}
	if in.Outcome == "CUSTOMER" {
		if !s.Provider.Capabilities().Refund {
			return Deal{}, ErrUnsupported
		}
		out, e := s.Provider.Refund(ctx, RefundRequest{ProviderPaymentID: d.Payment.ProviderPaymentID, AmountKopecks: d.GrossAmountKopecks, Currency: d.Currency, IdempotencyKey: key})
		if e != nil {
			return Deal{}, ErrProvider
		}
		return s.Repository.MarkRefundPending(ctx, id, key, out, true)
	}
	if !s.Provider.Capabilities().Release {
		return Deal{}, ErrUnsupported
	}
	out, e := s.Provider.Release(ctx, ReleaseRequest{ProviderPaymentID: d.Payment.ProviderPaymentID, AmountKopecks: d.FreelancerAmountKopecks, Currency: d.Currency, IdempotencyKey: key})
	if e != nil {
		return Deal{}, ErrProvider
	}
	return s.Repository.MarkReleasePending(ctx, id, key, out, true)
}
func (s Service) Reconcile(ctx context.Context) (int, error) {
	if s.Provider == nil {
		return 0, ErrProvider
	}
	if !s.Provider.Capabilities().Reconciliation {
		return 0, ErrUnsupported
	}
	payments, err := s.Repository.Unresolved(ctx)
	if err != nil {
		return 0, err
	}
	changed := 0
	for _, p := range payments {
		state, e := s.Provider.GetPayment(ctx, p.ProviderPaymentID)
		if e != nil {
			continue
		}
		eventType := stateEvent(state.Status)
		if eventType == "" {
			continue
		}
		// A zero amount asks the repository to derive the expected authoritative
		// amount for the event type (gross for funding/refund/cancel, net for release).
		event := VerifiedProviderEvent{Provider: p.Provider, ProviderEventID: "reconcile:" + p.ID + ":" + state.Status, ProviderPaymentID: p.ProviderPaymentID, Type: eventType, State: state.Status, AmountKopecks: 0, Currency: p.Currency, Verified: true, OccurredAt: time.Now().UTC()}
		_, applied, e := s.Repository.ApplyProviderEvent(ctx, event)
		if e != nil {
			// A single non-actionable payment (already resolved via a webhook,
			// duplicate, or amount/currency mismatch) must not abort the sweep;
			// only genuine infrastructure errors are fatal.
			if errors.Is(e, ErrInvalidState) || errors.Is(e, ErrInvalid) || errors.Is(e, ErrConflict) || errors.Is(e, ErrNotFound) {
				continue
			}
			return changed, e
		}
		if applied {
			changed++
		}
	}
	return changed, nil
}

func CalculateFee(gross int64, basisPoints int, minimum int64, maximum *int64) (int64, int64, error) {
	if gross <= 0 || basisPoints < 0 || basisPoints > 10000 || minimum < 0 {
		return 0, 0, ErrInvalid
	}
	fee := gross * int64(basisPoints) / 10000
	if fee < minimum {
		fee = minimum
	}
	if maximum != nil && fee > *maximum {
		fee = *maximum
	}
	if fee >= gross {
		return 0, 0, ErrInvalid
	}
	return fee, gross - fee, nil
}
func commandInput(actor, id, key string) bool {
	return actor != "" && validID.MatchString(id) && validIdempotency.MatchString(key)
}
func oneOf(v string, values ...string) bool {
	for _, x := range values {
		if v == x {
			return true
		}
	}
	return false
}
func stateEvent(v string) string {
	switch v {
	case "FUNDED":
		return "FUNDING_CONFIRMED"
	case "RELEASED":
		return "RELEASE_CONFIRMED"
	case "REFUNDED":
		return "REFUND_CONFIRMED"
	case "CANCELED":
		return "CANCEL_CONFIRMED"
	}
	return ""
}
