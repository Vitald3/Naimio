package safedeal

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"freelance/apps/api/internal/auth"
)

const (
	dealID       = "11111111-1111-4111-8111-111111111111"
	projectID    = "22222222-2222-4222-8222-222222222222"
	assignmentID = "33333333-3333-4333-8333-333333333333"
	customerID   = "44444444-4444-4444-8444-444444444444"
	freelancerID = "55555555-5555-4555-8555-555555555555"
	adminID      = "66666666-6666-4666-8666-666666666666"
	otherID      = "77777777-7777-4777-8777-777777777777"
)

func fixture() (Service, *Store, *SandboxProvider) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	store := &Store{Deals: map[string]Deal{dealID: {ID: dealID, ProjectID: projectID, AssignmentID: assignmentID, CustomerUserID: customerID, FreelancerUserID: freelancerID, Currency: "RUB", GrossAmountKopecks: 1000000, PlatformFeeKopecks: 100000, FreelancerAmountKopecks: 900000, Status: "AWAITING_FUNDING", CreatedAt: now, UpdatedAt: now}}, Admins: map[string]bool{adminID: true}, Now: func() time.Time { return now }}
	provider := NewSandboxProvider("sandbox-contract-secret")
	provider.Now = func() time.Time { return now }
	return Service{Repository: store, Provider: provider}, store, provider
}
func webhook(t *testing.T, s Service, payment, eventType, state, eventID string, amount ...int64) (Deal, bool, error) {
	t.Helper()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	value := int64(1000000)
	if len(amount) > 0 {
		value = amount[0]
	}
	body := []byte(fmt.Sprintf(`{"id":%q,"payment_id":%q,"type":%q,"state":%q,"amount_kopecks":%d,"currency":"RUB"}`, eventID, payment, eventType, state, value))
	stamp := fmt.Sprint(now.Unix())
	mac := hmac.New(sha256.New, []byte("sandbox-contract-secret"))
	_, _ = mac.Write([]byte(stamp + "."))
	_, _ = mac.Write(body)
	return s.Webhook(context.Background(), map[string][]string{"X-Sandbox-Timestamp": {stamp}, "X-Sandbox-Signature": {hex.EncodeToString(mac.Sum(nil))}}, body)
}
func TestSafeDealFundingRevisionReleaseEndToEnd(t *testing.T) {
	s, store, _ := fixture()
	ctx := context.Background()
	d, e := s.Funding(ctx, customerID, dealID, "funding-command-1")
	if e != nil || d.Payment == nil {
		t.Fatalf("fund=%+v %v", d, e)
	}
	payment := d.Payment.ProviderPaymentID
	if _, applied, e := webhook(t, s, payment, "FUNDING_CONFIRMED", "FUNDED", "provider-event-funded"); e != nil || !applied {
		t.Fatalf("fund webhook applied=%v err=%v", applied, e)
	}
	if _, e = s.Start(ctx, customerID, dealID, "start-wrong-actor"); !errors.Is(e, ErrNotFound) {
		t.Fatalf("wrong start=%v", e)
	}
	if d, e = s.Start(ctx, freelancerID, dealID, "start-command-1"); e != nil || d.Status != "IN_PROGRESS" {
		t.Fatalf("start=%+v %v", d, e)
	}
	if d, e = s.Submit(ctx, freelancerID, dealID, "submit-command-1", SubmitInput{Summary: "Работа готова"}); e != nil || d.Status != "SUBMITTED" {
		t.Fatalf("submit=%+v %v", d, e)
	}
	if d, e = s.Revision(ctx, customerID, dealID, "revision-command-1", "Нужно исправить согласованный пункт"); e != nil || d.Status != "REVISION_REQUESTED" {
		t.Fatalf("revision=%+v %v", d, e)
	}
	_, e = s.Submit(ctx, freelancerID, dealID, "submit-command-2", SubmitInput{Summary: "Правки готовы"})
	if e != nil {
		t.Fatal(e)
	}
	d, e = s.Accept(ctx, customerID, dealID, "accept-command-1")
	if e != nil || d.Status != "RELEASE_PENDING" {
		t.Fatalf("accept=%+v %v", d, e)
	}
	d, applied, e := webhook(t, s, payment, "RELEASE_CONFIRMED", "RELEASED", "provider-event-released", 900000)
	if e != nil || !applied || d.Status != "COMPLETED" || store.ProjectStatus[projectID] != "COMPLETED" {
		t.Fatalf("complete=%+v applied=%v project=%s err=%v", d, applied, store.ProjectStatus[projectID], e)
	}
	if _, applied, e = webhook(t, s, payment, "RELEASE_CONFIRMED", "RELEASED", "provider-event-released", 900000); e != nil || applied {
		t.Fatalf("replay applied=%v err=%v", applied, e)
	}
}
func TestDisputeEvidenceAndAdminRefund(t *testing.T) {
	s, store, _ := fixture()
	ctx := context.Background()
	d, _ := s.Funding(ctx, customerID, dealID, "funding-dispute")
	payment := d.Payment.ProviderPaymentID
	_, _, _ = webhook(t, s, payment, "FUNDING_CONFIRMED", "FUNDED", "funded-dispute")
	d, e := s.Dispute(ctx, freelancerID, dealID, "dispute-command-1", DisputeInput{ReasonCode: "CUSTOMER_UNRESPONSIVE", Description: "Нет ответа по согласованной работе"})
	if e != nil || d.Status != "DISPUTED" {
		t.Fatalf("dispute=%+v %v", d, e)
	}
	if _, e = s.Evidence(ctx, otherID, d.Dispute.ID, "evidence-command-x", EvidenceInput{Kind: "COMMENT", Body: "private"}); !errors.Is(e, ErrNotFound) {
		t.Fatalf("evidence bola=%v", e)
	}
	if _, e = s.Evidence(ctx, customerID, d.Dispute.ID, "evidence-command-1", EvidenceInput{Kind: "COMMENT", Body: "Ответ заказчика"}); e != nil {
		t.Fatal(e)
	}
	d, e = s.Resolve(ctx, adminID, dealID, "resolution-command-1", ResolutionInput{Outcome: "CUSTOMER", Reason: "Возврат по предоставленным доказательствам"})
	if e != nil || d.Status != "REFUND_PENDING" {
		t.Fatalf("resolve=%+v %v", d, e)
	}
	d, _, e = webhook(t, s, payment, "REFUND_CONFIRMED", "REFUNDED", "provider-event-refund")
	if e != nil || d.Status != "REFUNDED" || store.ProjectStatus[projectID] != "CANCELLED" {
		t.Fatalf("refund=%+v %v", d, e)
	}
}
func TestMoneyProviderFailureAndWebhookSecurity(t *testing.T) {
	max := int64(150000)
	fee, net, e := CalculateFee(2000000, 1000, 50000, &max)
	if e != nil || fee != 150000 || net != 1850000 {
		t.Fatalf("money=%d %d %v", fee, net, e)
	}
	if _, _, e = CalculateFee(100, 10000, 0, nil); !errors.Is(e, ErrInvalid) {
		t.Fatalf("fee equal to gross must fail: %v", e)
	}
	s, _, _ := fixture()
	s.Provider = nil
	if _, e = s.Funding(context.Background(), customerID, dealID, "provider-missing"); !errors.Is(e, ErrProvider) {
		t.Fatalf("provider=%v", e)
	}
	s, _, _ = fixture()
	if _, _, e = s.Webhook(context.Background(), map[string][]string{}, []byte(`{}`)); !errors.Is(e, ErrForbidden) {
		t.Fatalf("signature=%v", e)
	}
	if _, _, e = webhook(t, s, "unknown-payment", "FUNDING_CONFIRMED", "RELEASED", "mismatched-event"); !errors.Is(e, ErrForbidden) {
		t.Fatalf("mismatched provider event=%v", e)
	}
}
func TestPendingFundingCancelAndLostWebhookReconciliation(t *testing.T) {
	s, store, _ := fixture()
	d, e := s.Funding(context.Background(), customerID, dealID, "cancel-funding-intent")
	if e != nil {
		t.Fatal(e)
	}
	payment := d.Payment.ProviderPaymentID
	d, e = s.Cancel(context.Background(), customerID, dealID, "cancel-pending-intent")
	if e != nil || d.Status != "CANCEL_PENDING" {
		t.Fatalf("cancel=%+v %v", d, e)
	}
	if d, _, e = webhook(t, s, payment, "CANCEL_CONFIRMED", "CANCELED", "cancel-confirmed"); e != nil || d.Status != "CANCELED" {
		t.Fatalf("canceled=%+v %v", d, e)
	}
	s, store, _ = fixture()
	store.Payments = map[string]Payment{dealID: {ID: "88888888-8888-4888-8888-888888888888", DealID: dealID, Provider: "reconcile", ProviderPaymentID: "lost-payment", ProviderStatus: "PENDING", AmountKopecks: 1000000, Currency: "RUB"}}
	store.ProviderPayments = map[string]string{"lost-payment": dealID}
	s.Provider = reconcileProvider{}
	count, e := s.Reconcile(context.Background())
	if e != nil || count != 1 || store.Deals[dealID].Status != "FUNDED" {
		t.Fatalf("reconcile=%d status=%s err=%v", count, store.Deals[dealID].Status, e)
	}
}

type reconcileProvider struct{}

func (reconcileProvider) Capabilities() Capabilities {
	return Capabilities{Funding: true, Release: true, Refund: true, Webhooks: true, Reconciliation: true}
}
func (reconcileProvider) CreateFunding(context.Context, CreateFundingRequest) (CreateFundingResult, error) {
	return CreateFundingResult{}, ErrUnsupported
}
func (reconcileProvider) GetPayment(context.Context, string) (PaymentState, error) {
	return PaymentState{Status: "FUNDED"}, nil
}
func (reconcileProvider) CancelPayment(context.Context, CancelPaymentRequest) error {
	return ErrUnsupported
}
func (reconcileProvider) Refund(context.Context, RefundRequest) (RefundResult, error) {
	return RefundResult{}, ErrUnsupported
}
func (reconcileProvider) Release(context.Context, ReleaseRequest) (ReleaseResult, error) {
	return ReleaseResult{}, ErrUnsupported
}
func (reconcileProvider) VerifyWebhook(context.Context, map[string][]string, []byte) (VerifiedProviderEvent, error) {
	return VerifiedProviderEvent{}, ErrUnsupported
}
func TestSafeDealHTTPRejectsClientIdentityAndRequiresIdempotency(t *testing.T) {
	s, _, _ := fixture()
	h := Handler{Service: s}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/me/safe-deals/"+dealID+"/fund", strings.NewReader(`{"user_id":"`+otherID+`"}`))
	r = r.WithContext(auth.WithActorID(r.Context(), customerID))
	w := httptest.NewRecorder()
	h.Mine(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing key=%d %s", w.Code, w.Body.String())
	}
	r = httptest.NewRequest(http.MethodGet, "/api/v1/me/safe-deals/"+dealID, nil)
	r = r.WithContext(auth.WithActorID(r.Context(), otherID))
	w = httptest.NewRecorder()
	h.Mine(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("BOLA=%d", w.Code)
	}
}

// TestQuoteCanonicalExample verifies the spec's canonical breakdown: work
// 100000, platform commission 10% split 50/50, provider cost 2% on the customer
// => customer total 107000, freelancer payout 95000, gross commission 10000.
func TestQuoteCanonicalExample(t *testing.T) {
	s, store, _ := fixture()
	store.Fee = FeePolicy{Version: 2, CommissionBasisPoints: 1000, PayerMode: PayerSplit, CustomerShareBasisPoints: 5000}
	store.Pricing = ProviderPricing{Version: 3, Provider: "sandbox", PaymentMethod: MethodCard, PercentBasisPoints: 200, PayerMode: PayerCustomer}
	q, err := s.Quote(context.Background(), 100000, "CARD")
	if err != nil {
		t.Fatal(err)
	}
	if q.CustomerTotalKopecks != 107000 || q.FreelancerPayoutKopecks != 95000 || q.PlatformGrossRevenueKopecks != 10000 {
		t.Fatalf("canonical quote wrong: %+v", q)
	}
	if q.ProviderFee.TotalKopecks != 2000 || q.ProviderFee.CustomerKopecks != 2000 {
		t.Fatalf("provider fee wrong: %+v", q.ProviderFee)
	}
	if q.PlatformFee.CustomerKopecks != 5000 || q.PlatformFee.FreelancerKopecks != 5000 {
		t.Fatalf("platform split wrong: %+v", q.PlatformFee)
	}
	if q.FeeRuleVersion != 2 || q.ProviderPricingVersion != 3 {
		t.Fatalf("provenance wrong: %+v", q)
	}
}

// TestQuoteDefaultReproducesLegacy verifies the zero-configured store falls back
// to the legacy model (10% freelancer-borne commission, no provider fee).
func TestQuoteDefaultReproducesLegacy(t *testing.T) {
	s, _, _ := fixture()
	q, err := s.Quote(context.Background(), 100000, "")
	if err != nil {
		t.Fatal(err)
	}
	if q.CustomerTotalKopecks != 100000 || q.FreelancerPayoutKopecks != 90000 || q.PlatformGrossRevenueKopecks != 10000 || q.ProviderFee.TotalKopecks != 0 {
		t.Fatalf("legacy default quote wrong: %+v", q)
	}
}

func TestQuoteRejectsInvalidInput(t *testing.T) {
	s, _, _ := fixture()
	if _, err := s.Quote(context.Background(), 0, "CARD"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid for zero work, got %v", err)
	}
	if _, err := s.Quote(context.Background(), 100000, "BITCOIN"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid for unknown method, got %v", err)
	}
	if _, err := s.Quote(context.Background(), maxQuoteWorkKopecks+1, "CARD"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid for overflow-risk work, got %v", err)
	}
}

func TestQuoteHTTPEndpoint(t *testing.T) {
	s, store, _ := fixture()
	store.Fee = FeePolicy{Version: 2, CommissionBasisPoints: 1000, PayerMode: PayerSplit, CustomerShareBasisPoints: 5000}
	store.Pricing = ProviderPricing{Version: 3, Provider: "sandbox", PaymentMethod: MethodCard, PercentBasisPoints: 200, PayerMode: PayerCustomer}
	h := Handler{Service: s}
	// Authenticated POST returns the authoritative breakdown.
	r := httptest.NewRequest(http.MethodPost, "/api/v1/me/safe-deals/quote", strings.NewReader(`{"work_amount_kopecks":100000,"payment_method":"CARD"}`))
	r = r.WithContext(auth.WithActorID(r.Context(), freelancerID))
	w := httptest.NewRecorder()
	h.Mine(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("quote status=%d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"customer_total_kopecks":107000`) || !strings.Contains(w.Body.String(), `"freelancer_payout_kopecks":95000`) {
		t.Fatalf("quote body wrong: %s", w.Body.String())
	}
	// Unauthenticated requests are rejected.
	r = httptest.NewRequest(http.MethodPost, "/api/v1/me/safe-deals/quote", strings.NewReader(`{"work_amount_kopecks":100000}`))
	w = httptest.NewRecorder()
	h.Mine(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without actor, got %d", w.Code)
	}
	// Invalid work amount is a validation error.
	r = httptest.NewRequest(http.MethodPost, "/api/v1/me/safe-deals/quote", strings.NewReader(`{"work_amount_kopecks":0}`))
	r = r.WithContext(auth.WithActorID(r.Context(), customerID))
	w = httptest.NewRecorder()
	h.Mine(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for zero work, got %d %s", w.Code, w.Body.String())
	}
}

func TestSandboxConfirmationSurvivesProviderStateReset(t *testing.T) {
	provider := NewSandboxProvider("sandbox-contract-secret")
	if err := provider.ConfirmPayment("sb_pay_persisted_after_restart"); err != nil {
		t.Fatalf("confirm persisted sandbox payment: %v", err)
	}
	state, err := provider.GetPayment(context.Background(), "sb_pay_persisted_after_restart")
	if err != nil || state.Status != "FUNDED" {
		t.Fatalf("sandbox state=%+v err=%v", state, err)
	}
}
