package safedeal

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"time"
)

type Store struct {
	mu               sync.Mutex
	Deals            map[string]Deal
	Payments         map[string]Payment
	ProviderPayments map[string]string
	Disputes         map[string]Dispute
	Operations       map[string][]byte
	Admins           map[string]bool
	ProjectStatus    map[string]string
	Now              func() time.Time
	// Fee and Pricing are the active economic configuration used when a deal
	// is created. When left zero-valued a sensible default is used that
	// reproduces the legacy model (10% platform commission borne by the
	// freelancer, no separate provider fee).
	Fee     FeePolicy
	Pricing ProviderPricing
}

// feeRule returns the configured platform fee policy or the legacy default.
func (s *Store) feeRule() FeePolicy {
	if s.Fee.PayerMode == "" {
		return FeePolicy{Version: 1, CommissionBasisPoints: 1000, PayerMode: PayerFreelancer}
	}
	return s.Fee
}

// pricingRule returns the configured provider pricing or a zero-cost default.
func (s *Store) pricingRule() ProviderPricing {
	if s.Pricing.PayerMode == "" {
		return ProviderPricing{Version: 1, Provider: "sandbox", PaymentMethod: MethodCard, PayerMode: PayerCustomer}
	}
	return s.Pricing
}

// ActiveEconomics exposes the configured fee policy and provider pricing for the
// quote preview. The in-memory store keeps a single active configuration; the
// requested payment method is echoed onto the returned pricing.
func (s *Store) ActiveEconomics(_ context.Context, paymentMethod string) (FeePolicy, ProviderPricing, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pricing := s.pricingRule()
	if paymentMethod != "" {
		pricing.PaymentMethod = paymentMethod
	}
	return s.feeRule(), pricing, nil
}

func (s *Store) CreateFromAssignment(_ context.Context, project, assignment, customer, freelancer string, gross int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.init()
	for _, d := range s.Deals {
		if d.AssignmentID == assignment || d.ProjectID == project && !oneOf(d.Status, "COMPLETED", "CANCELED", "REFUNDED", "FAILED") {
			return nil
		}
	}
	quote, e := CalculateDealQuote(QuoteInput{WorkAmountKopecks: gross, Currency: "RUB", Fee: s.feeRule(), Provider: s.pricingRule()})
	if e != nil {
		return e
	}
	now := s.now()
	id := newID()
	d := Deal{ID: id, ProjectID: project, AssignmentID: assignment, CustomerUserID: customer, FreelancerUserID: freelancer, Status: "AWAITING_FUNDING", CreatedAt: now, UpdatedAt: now, Events: []DomainEvent{{Type: "DEAL_FUNDING_REQUIRED", ActorUserID: customer, CreatedAt: now}}}
	d.ApplyQuote(quote)
	s.Deals[id] = d
	s.ProjectStatus[project] = "AWAITING_FUNDING"
	return nil
}

func (s *Store) ResolveByProject(_ context.Context, project string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, d := range s.Deals {
		if d.ProjectID == project && !oneOf(d.Status, "COMPLETED", "CANCELED", "REFUNDED", "FAILED") {
			return id, nil
		}
	}
	return "", fmt.Errorf("safe deal not found")
}
func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s *Store) init() {
	if s.Deals == nil {
		s.Deals = map[string]Deal{}
	}
	if s.Payments == nil {
		s.Payments = map[string]Payment{}
	}
	if s.ProviderPayments == nil {
		s.ProviderPayments = map[string]string{}
	}
	if s.Disputes == nil {
		s.Disputes = map[string]Dispute{}
	}
	if s.Operations == nil {
		s.Operations = map[string][]byte{}
	}
	if s.ProjectStatus == nil {
		s.ProjectStatus = map[string]string{}
	}
}
func (s *Store) List(_ context.Context, actor, project string) ([]Deal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.init()
	out := []Deal{}
	for _, d := range s.Deals {
		if (d.CustomerUserID == actor || d.FreelancerUserID == actor) && (project == "" || d.ProjectID == project) {
			out = append(out, s.enrich(d))
		}
	}
	return out, nil
}
func (s *Store) Get(_ context.Context, actor, id string, admin bool) (Deal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.init()
	d, ok := s.Deals[id]
	if !ok || !admin && actor != d.CustomerUserID && actor != d.FreelancerUserID || admin && !s.Admins[actor] {
		return Deal{}, ErrNotFound
	}
	return s.enrich(d), nil
}
func (s *Store) SaveFunding(_ context.Context, actor, id, key string, r CreateFundingResult) (Deal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.init()
	d, e := s.party(id, actor, "CUSTOMER")
	if e != nil {
		return Deal{}, e
	}
	op := id + ":fund:" + key
	if _, ok := s.Operations[op]; ok {
		return s.enrich(d), nil
	}
	if d.Status != "AWAITING_FUNDING" {
		return Deal{}, ErrInvalidState
	}
	p := Payment{ID: newID(), DealID: id, Provider: r.Provider, ProviderPaymentID: r.ProviderPaymentID, ProviderStatus: r.Status, CheckoutURL: r.CheckoutURL, AmountKopecks: d.GrossAmountKopecks, Currency: d.Currency, IdempotencyKey: key, UpdatedAt: s.now()}
	s.Payments[id] = p
	s.ProviderPayments[p.ProviderPaymentID] = id
	s.Operations[op] = nil
	return s.enrich(d), nil
}
func (s *Store) Start(_ context.Context, actor, id, key string) (Deal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, e := s.party(id, actor, "FREELANCER")
	if e != nil {
		return Deal{}, e
	}
	if s.done(id, "start", key) {
		return s.enrich(d), nil
	}
	if d.Status != "FUNDED" {
		return Deal{}, ErrInvalidState
	}
	now := s.now()
	d.Status = "IN_PROGRESS"
	d.WorkStartedAt = &now
	s.save(d, "DEAL_STARTED", actor, key)
	s.ProjectStatus[d.ProjectID] = "IN_PROGRESS"
	return s.enrich(d), nil
}
func (s *Store) Submit(_ context.Context, actor, id, key string, in SubmitInput) (Deal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, e := s.party(id, actor, "FREELANCER")
	if e != nil {
		return Deal{}, e
	}
	if s.done(id, "submit", key) {
		return s.enrich(d), nil
	}
	if d.Status != "IN_PROGRESS" && d.Status != "REVISION_REQUESTED" {
		return Deal{}, ErrInvalidState
	}
	now := s.now()
	d.Status = "SUBMITTED"
	d.SubmittedAt = &now
	d.Submission = &Submission{ID: newID(), DealID: id, SubmittedByUserID: actor, Summary: in.Summary, MessageID: in.MessageID, RevisionNumber: d.RevisionCount, CreatedAt: now}
	s.save(d, "DEAL_SUBMITTED", actor, key)
	return s.enrich(d), nil
}
func (s *Store) Revision(_ context.Context, actor, id, key, reason string) (Deal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, e := s.party(id, actor, "CUSTOMER")
	if e != nil {
		return Deal{}, e
	}
	if s.done(id, "revision", key) {
		return s.enrich(d), nil
	}
	if d.Status != "SUBMITTED" || d.RevisionCount >= 2 {
		return Deal{}, ErrInvalidState
	}
	d.Status = "REVISION_REQUESTED"
	d.RevisionCount++
	s.saveMeta(d, "DEAL_REVISION_REQUESTED", actor, key, map[string]string{"reason": reason})
	return s.enrich(d), nil
}
func (s *Store) OpenDispute(_ context.Context, actor, id, key string, in DisputeInput) (Deal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, e := s.party(id, actor, "")
	if e != nil {
		return Deal{}, e
	}
	if s.done(id, "dispute", key) {
		return s.enrich(d), nil
	}
	if !oneOf(d.Status, "FUNDED", "IN_PROGRESS", "SUBMITTED", "REVISION_REQUESTED") {
		return Deal{}, ErrInvalidState
	}
	v := Dispute{ID: newID(), DealID: id, OpenedByUserID: actor, ReasonCode: in.ReasonCode, Description: in.Description, Status: "OPEN", OpenedAt: s.now()}
	s.Disputes[id] = v
	d.Status = "DISPUTED"
	s.save(d, "DISPUTE_OPENED", actor, key)
	return s.enrich(d), nil
}
func (s *Store) AddEvidence(_ context.Context, actor, dispute, key string, in EvidenceInput) (Deal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.init()
	var d Deal
	var found bool
	for dealID, v := range s.Disputes {
		if v.ID == dispute {
			d = s.Deals[dealID]
			found = true
			if actor != d.CustomerUserID && actor != d.FreelancerUserID && !s.Admins[actor] {
				return Deal{}, ErrNotFound
			}
			if s.done(d.ID, "evidence", key) {
				return s.enrich(d), nil
			}
			ev := Evidence{ID: newID(), DisputeID: v.ID, AuthorUserID: actor, Kind: in.Kind, Body: in.Body, ReferenceID: in.ReferenceID, CreatedAt: s.now()}
			v.Evidence = append(v.Evidence, ev)
			s.Disputes[dealID] = v
			s.Operations[d.ID+":evidence:"+key] = nil
			break
		}
	}
	if !found {
		return Deal{}, ErrNotFound
	}
	return s.enrich(d), nil
}
func (s *Store) PrepareAccept(_ context.Context, actor, id, key string) (Deal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, e := s.party(id, actor, "CUSTOMER")
	if e != nil {
		return Deal{}, e
	}
	if d.Status == "RELEASE_PENDING" || d.Status == "COMPLETED" || s.done(id, "accept", key) {
		return s.enrich(d), nil
	}
	if d.Status != "SUBMITTED" {
		return Deal{}, ErrInvalidState
	}
	now := s.now()
	d.Status = "ACCEPTED"
	d.AcceptedAt = &now
	s.save(d, "DEAL_ACCEPTED", actor, key)
	return s.enrich(d), nil
}
func (s *Store) MarkReleasePending(_ context.Context, id, key string, _ ReleaseResult, _ bool) (Deal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.Deals[id]
	if !ok {
		return Deal{}, ErrNotFound
	}
	if d.Status == "RELEASE_PENDING" || d.Status == "COMPLETED" {
		return s.enrich(d), nil
	}
	if d.Status != "ACCEPTED" && d.Status != "DISPUTED" {
		return Deal{}, ErrInvalidState
	}
	d.Status = "RELEASE_PENDING"
	p := s.Payments[id]
	p.ProviderStatus = "RELEASE_PENDING"
	p.UpdatedAt = s.now()
	s.Payments[id] = p
	s.save(d, "DEAL_RELEASE_PENDING", "", key)
	return s.enrich(d), nil
}
func (s *Store) CancelUnfunded(_ context.Context, actor, id, key string) (Deal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, e := s.party(id, actor, "CUSTOMER")
	if e != nil {
		return Deal{}, e
	}
	if s.done(id, "cancel", key) || d.Status == "CANCELED" {
		return s.enrich(d), nil
	}
	if d.Status != "AWAITING_FUNDING" {
		return Deal{}, ErrInvalidState
	}
	now := s.now()
	d.Status = "CANCELED"
	d.CompletedAt = &now
	s.save(d, "DEAL_CANCELED", actor, key)
	s.ProjectStatus[d.ProjectID] = "CANCELLED"
	return s.enrich(d), nil
}
func (s *Store) MarkCancelPending(_ context.Context, id, key string) (Deal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.Deals[id]
	if !ok {
		return Deal{}, ErrNotFound
	}
	if d.Status == "CANCEL_PENDING" || d.Status == "CANCELED" {
		return s.enrich(d), nil
	}
	if d.Status != "AWAITING_FUNDING" {
		return Deal{}, ErrInvalidState
	}
	d.Status = "CANCEL_PENDING"
	p := s.Payments[id]
	p.ProviderStatus = "CANCEL_PENDING"
	p.UpdatedAt = s.now()
	s.Payments[id] = p
	s.save(d, "DEAL_CANCEL_PENDING", "", key)
	return s.enrich(d), nil
}
func (s *Store) MarkRefundPending(_ context.Context, id, key string, _ RefundResult, _ bool) (Deal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.Deals[id]
	if !ok {
		return Deal{}, ErrNotFound
	}
	if d.Status == "REFUND_PENDING" || d.Status == "REFUNDED" {
		return s.enrich(d), nil
	}
	if d.Status != "FUNDED" && d.Status != "DISPUTED" {
		return Deal{}, ErrInvalidState
	}
	d.Status = "REFUND_PENDING"
	p := s.Payments[id]
	p.ProviderStatus = "REFUND_PENDING"
	p.UpdatedAt = s.now()
	s.Payments[id] = p
	s.save(d, "DEAL_REFUND_PENDING", "", key)
	return s.enrich(d), nil
}
func (s *Store) ApplyProviderEvent(_ context.Context, event VerifiedProviderEvent) (Deal, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.init()
	if !event.Verified {
		return Deal{}, false, ErrForbidden
	}
	op := "provider:" + event.Provider + ":" + event.ProviderEventID
	if _, ok := s.Operations[op]; ok {
		dealID := s.ProviderPayments[event.ProviderPaymentID]
		return s.enrich(s.Deals[dealID]), false, nil
	}
	dealID, ok := s.ProviderPayments[event.ProviderPaymentID]
	if !ok {
		return Deal{}, false, ErrNotFound
	}
	d := s.Deals[dealID]
	p := s.Payments[dealID]
	expected := p.AmountKopecks
	if event.Type == "RELEASE_CONFIRMED" {
		expected = d.FreelancerAmountKopecks
	}
	if event.AmountKopecks == 0 {
		event.AmountKopecks = expected
	}
	if expected != event.AmountKopecks || p.Currency != event.Currency {
		return Deal{}, false, ErrConflict
	}
	now := s.now()
	switch event.Type {
	case "FUNDING_CONFIRMED":
		if d.Status != "AWAITING_FUNDING" {
			return Deal{}, false, ErrInvalidState
		}
		d.Status = "FUNDED"
		d.FundedAt = &now
		p.ProviderStatus = "FUNDED"
	case "RELEASE_CONFIRMED":
		if d.Status != "RELEASE_PENDING" {
			return Deal{}, false, ErrInvalidState
		}
		d.Status = "COMPLETED"
		d.CompletedAt = &now
		p.ProviderStatus = "RELEASED"
		s.ProjectStatus[d.ProjectID] = "COMPLETED"
	case "REFUND_CONFIRMED":
		if d.Status != "REFUND_PENDING" {
			return Deal{}, false, ErrInvalidState
		}
		d.Status = "REFUNDED"
		d.CompletedAt = &now
		p.ProviderStatus = "REFUNDED"
		s.ProjectStatus[d.ProjectID] = "CANCELLED"
	case "CANCEL_CONFIRMED":
		d.Status = "CANCELED"
		d.CompletedAt = &now
		p.ProviderStatus = "CANCELED"
		s.ProjectStatus[d.ProjectID] = "CANCELLED"
	default:
		return Deal{}, false, ErrInvalid
	}
	p.UpdatedAt = now
	s.Payments[dealID] = p
	s.Operations[op] = nil
	s.save(d, "DEAL_"+event.State, "", op)
	return s.enrich(d), true, nil
}
func (s *Store) AdminList(_ context.Context, actor string) ([]Deal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.init()
	if !s.Admins[actor] {
		return nil, ErrForbidden
	}
	out := []Deal{}
	for _, d := range s.Deals {
		out = append(out, s.enrich(d))
	}
	return out, nil
}
func (s *Store) AdminDeleteUnfunded(context.Context, string, string, string) error {
	return ErrUnsupported
}
func (s *Store) PrepareResolution(_ context.Context, actor, id, key string, in ResolutionInput) (Deal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.init()
	if !s.Admins[actor] {
		return Deal{}, ErrForbidden
	}
	d, ok := s.Deals[id]
	if !ok {
		return Deal{}, ErrNotFound
	}
	v, ok := s.Disputes[id]
	if !ok || d.Status != "DISPUTED" || !oneOf(v.Status, "OPEN", "EVIDENCE_COLLECTION", "UNDER_REVIEW") {
		return Deal{}, ErrInvalidState
	}
	if s.done(id, "resolve", key) {
		return s.enrich(d), nil
	}
	v.Status = "RESOLVED_" + in.Outcome
	v.Resolution = in.Outcome
	v.ResolutionReason = in.Reason
	now := s.now()
	v.ResolvedAt = &now
	s.Disputes[id] = v
	s.Operations[id+":resolve:"+key] = nil
	return s.enrich(d), nil
}
func (s *Store) Unresolved(_ context.Context) ([]Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.init()
	out := []Payment{}
	for _, p := range s.Payments {
		if oneOf(p.ProviderStatus, "PENDING", "FUNDED", "RELEASE_PENDING", "REFUND_PENDING", "CANCEL_PENDING") {
			out = append(out, p)
		}
	}
	return out, nil
}
func (s *Store) party(id, actor, role string) (Deal, error) {
	s.init()
	d, ok := s.Deals[id]
	if !ok {
		return Deal{}, ErrNotFound
	}
	if role == "CUSTOMER" && actor != d.CustomerUserID || role == "FREELANCER" && actor != d.FreelancerUserID || role == "" && actor != d.CustomerUserID && actor != d.FreelancerUserID {
		return Deal{}, ErrNotFound
	}
	return d, nil
}
func (s *Store) done(id, action, key string) bool {
	_, ok := s.Operations[id+":"+action+":"+key]
	return ok
}
func (s *Store) save(d Deal, event, actor, key string) { s.saveMeta(d, event, actor, key, nil) }
func (s *Store) saveMeta(d Deal, event, actor, key string, meta map[string]string) {
	d.UpdatedAt = s.now()
	d.Events = append(d.Events, DomainEvent{Type: event, ActorUserID: actor, Metadata: meta, CreatedAt: d.UpdatedAt})
	s.Deals[d.ID] = d
	s.Operations[d.ID+":"+operationAction(event)+":"+key] = nil
}
func (s *Store) enrich(d Deal) Deal {
	if p, ok := s.Payments[d.ID]; ok {
		copy := p
		d.Payment = &copy
	}
	if v, ok := s.Disputes[d.ID]; ok {
		copy := v
		d.Dispute = &copy
		d.Evidence = append([]Evidence{}, v.Evidence...)
	}
	d.ProviderOperational = true
	d.ProviderCapabilityNotice = "Финансовые операции подтверждаются платёжным провайдером."
	return d
}
func operationAction(event string) string {
	m := map[string]string{"DEAL_STARTED": "start", "DEAL_SUBMITTED": "submit", "DEAL_REVISION_REQUESTED": "revision", "DISPUTE_OPENED": "dispute", "DEAL_ACCEPTED": "accept", "DEAL_CANCELED": "cancel"}
	if v := m[event]; v != "" {
		return v
	}
	return event
}
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&15 | 64
	b[8] = b[8]&63 | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
