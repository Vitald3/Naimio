package finance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeRepo records the last created rules and lets tests drive role checks.
type fakeRepo struct {
	roles       map[string][]string
	feeRules    []FeeRule
	pricing     []ProviderPricingRule
	lastFee     FeeRule
	lastPricing ProviderPricingRule
}

func (f *fakeRepo) Roles(_ context.Context, id string) ([]string, error) { return f.roles[id], nil }
func (f *fakeRepo) ListFeeRules(context.Context) ([]FeeRule, error)      { return f.feeRules, nil }
func (f *fakeRepo) CreateFeeRule(_ context.Context, _ string, in FeeRule, _, _ string) (FeeRule, error) {
	in.Version = len(f.feeRules) + 2 // simulate MAX(version)+1 over a seeded v1
	in.Enabled = true
	in.EffectiveFrom = time.Unix(0, 0).UTC()
	in.CreatedAt = time.Unix(0, 0).UTC()
	f.lastFee = in
	f.feeRules = append([]FeeRule{in}, f.feeRules...)
	return in, nil
}
func (f *fakeRepo) ListProviderPricing(context.Context) ([]ProviderPricingRule, error) {
	return f.pricing, nil
}
func (f *fakeRepo) CreateProviderPricing(_ context.Context, _ string, in ProviderPricingRule, _, _ string) (ProviderPricingRule, error) {
	in.Version = len(f.pricing) + 2
	in.Enabled = true
	f.lastPricing = in
	f.pricing = append([]ProviderPricingRule{in}, f.pricing...)
	return in, nil
}

func canonicalFee() FeeRule {
	return FeeRule{CommissionBasisPoints: 1000, PlatformFeePayerMode: "SPLIT", PlatformCustomerShareBasisPoints: 5000, ProviderFeePayerMode: "CUSTOMER"}
}

// TestOnlyAdminMayConfigure verifies moderators are refused and admins allowed.
func TestOnlyAdminMayConfigure(t *testing.T) {
	repo := &fakeRepo{roles: map[string][]string{"mod": {"MODERATOR"}, "admin": {"ADMIN"}, "super": {"SUPER_ADMIN"}, "user": {}}}
	s := Service{Repository: repo}
	ctx := context.Background()

	if _, err := s.ListFeeRules(ctx, "mod"); err != ErrForbidden {
		t.Fatalf("moderator must not read finance config: %v", err)
	}
	if _, err := s.CreateFeeRule(ctx, "mod", canonicalFee(), true, "adjust commission", "req"); err != ErrForbidden {
		t.Fatalf("moderator must not change finance config: %v", err)
	}
	if _, err := s.CreateFeeRule(ctx, "user", canonicalFee(), true, "adjust commission", "req"); err != ErrForbidden {
		t.Fatalf("ordinary user must not change finance config: %v", err)
	}
	if _, err := s.CreateFeeRule(ctx, "admin", canonicalFee(), true, "adjust commission", "req"); err != nil {
		t.Fatalf("admin must be allowed: %v", err)
	}
	if _, err := s.CreateFeeRule(ctx, "super", canonicalFee(), true, "adjust commission", "req"); err != nil {
		t.Fatalf("super admin must be allowed: %v", err)
	}
}

// TestConfirmationAndReasonRequired verifies the governance guards.
func TestConfirmationAndReasonRequired(t *testing.T) {
	repo := &fakeRepo{roles: map[string][]string{"admin": {"ADMIN"}}}
	s := Service{Repository: repo}
	ctx := context.Background()

	if _, err := s.CreateFeeRule(ctx, "admin", canonicalFee(), false, "adjust commission", "req"); err != ErrConfirmationRequired {
		t.Fatalf("missing confirmation must be rejected: %v", err)
	}
	if _, err := s.CreateFeeRule(ctx, "admin", canonicalFee(), true, "  ", "req"); err != ErrReasonRequired {
		t.Fatalf("blank reason must be rejected: %v", err)
	}
	if _, err := s.CreateFeeRule(ctx, "admin", canonicalFee(), true, "ok", "req"); err != ErrReasonRequired {
		t.Fatalf("too-short reason must be rejected: %v", err)
	}
}

// TestFeeRuleValidation verifies invalid economics are refused before storage.
func TestFeeRuleValidation(t *testing.T) {
	repo := &fakeRepo{roles: map[string][]string{"admin": {"ADMIN"}}}
	s := Service{Repository: repo}
	ctx := context.Background()

	bad := []FeeRule{
		{CommissionBasisPoints: 10001, PlatformFeePayerMode: "CUSTOMER", ProviderFeePayerMode: "CUSTOMER"}, // commission out of range
		{CommissionBasisPoints: 1000, PlatformFeePayerMode: "BITCOIN", ProviderFeePayerMode: "CUSTOMER"},   // bad platform payer
		{CommissionBasisPoints: 1000, PlatformFeePayerMode: "CUSTOMER", ProviderFeePayerMode: "NOPE"},      // bad provider payer
		{CommissionBasisPoints: 1000, PlatformFeePayerMode: "SPLIT", PlatformCustomerShareBasisPoints: 20000, ProviderFeePayerMode: "CUSTOMER"},
	}
	for i, in := range bad {
		if _, err := s.CreateFeeRule(ctx, "admin", in, true, "attempted change", "req"); err != ErrInvalidInput {
			t.Fatalf("bad fee rule %d must be rejected, got %v", i, err)
		}
	}
	// A non-SPLIT platform mode must have its share zeroed by normalization.
	if _, err := s.CreateFeeRule(ctx, "admin", FeeRule{CommissionBasisPoints: 1000, PlatformFeePayerMode: "CUSTOMER", PlatformCustomerShareBasisPoints: 5000, ProviderFeePayerMode: "CUSTOMER"}, true, "customer pays commission", "req"); err != nil {
		t.Fatalf("valid fee rule rejected: %v", err)
	}
	if repo.lastFee.PlatformCustomerShareBasisPoints != 0 {
		t.Fatalf("non-split share must be zeroed, got %d", repo.lastFee.PlatformCustomerShareBasisPoints)
	}
}

// TestProviderPricingValidation verifies pricing constraints.
func TestProviderPricingValidation(t *testing.T) {
	repo := &fakeRepo{roles: map[string][]string{"admin": {"ADMIN"}}}
	s := Service{Repository: repo}
	ctx := context.Background()

	if _, err := s.CreateProviderPricing(ctx, "admin", ProviderPricingRule{Provider: "sandbox", PaymentMethod: "BITCOIN"}, true, "add method", "req"); err != ErrInvalidInput {
		t.Fatalf("unknown payment method must be rejected: %v", err)
	}
	if _, err := s.CreateProviderPricing(ctx, "admin", ProviderPricingRule{Provider: "", PaymentMethod: "CARD"}, true, "add method", "req"); err != ErrInvalidInput {
		t.Fatalf("empty provider must be rejected: %v", err)
	}
	max := int64(100)
	if _, err := s.CreateProviderPricing(ctx, "admin", ProviderPricingRule{Provider: "sandbox", PaymentMethod: "card", PercentBasisPoints: 200, MinimumFeeKopecks: 500, MaximumFeeKopecks: &max}, true, "add method", "req"); err != ErrInvalidInput {
		t.Fatalf("max<min must be rejected: %v", err)
	}
	v, err := s.CreateProviderPricing(ctx, "admin", ProviderPricingRule{Provider: "sandbox", PaymentMethod: "card", PercentBasisPoints: 200, FixedFeeKopecks: 1500}, true, "sandbox card pricing", "req")
	if err != nil {
		t.Fatalf("valid pricing rejected: %v", err)
	}
	if v.PaymentMethod != "CARD" {
		t.Fatalf("payment method must be upper-cased, got %q", v.PaymentMethod)
	}
	if v.Version < 2 {
		t.Fatalf("create must produce a new version, got %d", v.Version)
	}
}

// TestHTTPRejectsOrdinaryUser verifies the handler rejects a non-admin actor.
func TestHTTPRejectsOrdinaryUser(t *testing.T) {
	repo := &fakeRepo{roles: map[string][]string{"mod": {"MODERATOR"}}}
	h := Handler{Service: Service{Repository: repo}, ActorID: func(context.Context) (string, bool) { return "mod", true }}
	r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/finance/fees", nil)
	w := httptest.NewRecorder()
	h.Fees(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("moderator must get 403, got %d %s", w.Code, w.Body.String())
	}
}

// TestHTTPCreateFeeRule verifies the full POST path for an admin.
func TestHTTPCreateFeeRule(t *testing.T) {
	repo := &fakeRepo{roles: map[string][]string{"admin": {"ADMIN"}}}
	h := Handler{Service: Service{Repository: repo}, ActorID: func(context.Context) (string, bool) { return "admin", true }}
	body := `{"commission_basis_points":1000,"platform_fee_payer_mode":"SPLIT","platform_customer_share_basis_points":5000,"provider_fee_payer_mode":"CUSTOMER","confirm":true,"reason":"launch split commission"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/finance/fees", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Fees(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create fee rule status=%d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"platform_fee_payer_mode":"SPLIT"`) {
		t.Fatalf("response missing rule: %s", w.Body.String())
	}

	// Missing confirmation is a 422 CONFIRMATION_REQUIRED.
	r = httptest.NewRequest(http.MethodPost, "/api/v1/admin/finance/fees", strings.NewReader(`{"commission_basis_points":1000,"platform_fee_payer_mode":"CUSTOMER","provider_fee_payer_mode":"CUSTOMER","reason":"no confirm"}`))
	w = httptest.NewRecorder()
	h.Fees(w, r)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "CONFIRMATION_REQUIRED") {
		t.Fatalf("missing confirm must be 422 CONFIRMATION_REQUIRED, got %d %s", w.Code, w.Body.String())
	}
}
