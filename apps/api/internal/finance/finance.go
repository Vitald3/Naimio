package finance

// Administrative configuration of the payment economics.
//
// This module lets an administrator configure the two independent fee classes
// described in internal/safedeal.economics:
//
//   - PLATFORM COMMISSION      — the marketplace's own revenue, plus who pays
//     each fee class (the provider payer mode is carried here, next to the
//     platform payer mode, because the pricing table below holds pure cost).
//   - PROVIDER / SAFE-DEAL COST — the pure cost structure of a payment
//     provider + payment method (percent + fixed, clamped), versioned
//     independently so a new provider can be priced without touching
//     commission policy.
//
// Every mutation is a governed action: it requires ADMIN (or SUPER_ADMIN)
// authorization — never an ordinary moderator — an explicit confirmation, a
// human-readable reason, and it writes an audit log entry. Crucially it never
// mutates a historical rule: it inserts a NEW VERSION and deactivates the
// previously-active one, so existing Safe Deals (which snapshot their own
// economics) are never affected.

import (
	"context"
	"errors"
	"strings"
	"time"

	"freelance/apps/api/internal/safedeal"
)

var (
	// ErrForbidden is returned when the actor lacks ADMIN/SUPER_ADMIN.
	ErrForbidden = errors.New("finance admin permission required")
	// ErrInvalidInput is returned when the proposed rule fails validation.
	ErrInvalidInput = errors.New("invalid finance input")
	// ErrConfirmationRequired is returned when the caller did not confirm.
	ErrConfirmationRequired = errors.New("finance change confirmation required")
	// ErrReasonRequired is returned when no adequate reason was supplied.
	ErrReasonRequired = errors.New("finance change reason required")
)

// paymentMethods mirrors the CHECK constraint on safe_deal_provider_pricing.
var paymentMethods = map[string]bool{
	safedeal.MethodCard:    true,
	safedeal.MethodSBP:     true,
	safedeal.MethodTPay:    true,
	safedeal.MethodSberPay: true,
	safedeal.MethodOther:   true,
}

// FeeRule is one versioned platform-commission rule. It also carries the
// provider payer mode (who pays the provider cost), because the pricing table
// stores only the pure cost. It maps 1:1 to a row of safe_deal_fee_rules.
type FeeRule struct {
	Version                          int       `json:"version"`
	CommissionBasisPoints            int       `json:"commission_basis_points"`
	MinimumFeeKopecks                int64     `json:"minimum_fee_kopecks"`
	MaximumFeeKopecks                *int64    `json:"maximum_fee_kopecks,omitempty"`
	PlatformFeePayerMode             string    `json:"platform_fee_payer_mode"`
	PlatformCustomerShareBasisPoints int       `json:"platform_customer_share_basis_points"`
	ProviderFeePayerMode             string    `json:"provider_fee_payer_mode"`
	ProviderCustomerShareBasisPoints int       `json:"provider_customer_share_basis_points"`
	Enabled                          bool      `json:"enabled"`
	EffectiveFrom                    time.Time `json:"effective_from"`
	CreatedAt                        time.Time `json:"created_at"`
}

// normalize upper-cases the payer modes, zeroes the SPLIT shares that do not
// apply, and validates the rule using the SAME validators the domain uses when
// it prices a real deal, so admin config can never store a rule the deal
// calculation would reject.
func (in *FeeRule) normalize() error {
	in.PlatformFeePayerMode = strings.ToUpper(strings.TrimSpace(in.PlatformFeePayerMode))
	in.ProviderFeePayerMode = strings.ToUpper(strings.TrimSpace(in.ProviderFeePayerMode))
	if in.PlatformFeePayerMode != safedeal.PayerSplit {
		in.PlatformCustomerShareBasisPoints = 0
	}
	if in.ProviderFeePayerMode != safedeal.PayerSplit {
		in.ProviderCustomerShareBasisPoints = 0
	}
	platform := safedeal.FeePolicy{
		CommissionBasisPoints:    in.CommissionBasisPoints,
		MinimumFeeKopecks:        in.MinimumFeeKopecks,
		MaximumFeeKopecks:        in.MaximumFeeKopecks,
		PayerMode:                in.PlatformFeePayerMode,
		CustomerShareBasisPoints: in.PlatformCustomerShareBasisPoints,
	}
	if err := platform.Validate(); err != nil {
		return ErrInvalidInput
	}
	// The provider payer mode is validated with the shared provider validator;
	// its cost fields default to zero here, which is valid — only the payer
	// mode and its share are being checked.
	provider := safedeal.ProviderPricing{
		PayerMode:                in.ProviderFeePayerMode,
		CustomerShareBasisPoints: in.ProviderCustomerShareBasisPoints,
	}
	if err := provider.Validate(); err != nil {
		return ErrInvalidInput
	}
	return nil
}

// ProviderPricingRule is one versioned provider-cost rule, keyed by provider +
// payment method. It maps 1:1 to a row of safe_deal_provider_pricing. It holds
// pure cost only; who pays it is decided by the fee rule's provider payer mode.
type ProviderPricingRule struct {
	Version            int       `json:"version"`
	Provider           string    `json:"provider"`
	PaymentMethod      string    `json:"payment_method"`
	PercentBasisPoints int       `json:"percent_basis_points"`
	FixedFeeKopecks    int64     `json:"fixed_fee_kopecks"`
	MinimumFeeKopecks  int64     `json:"minimum_fee_kopecks"`
	MaximumFeeKopecks  *int64    `json:"maximum_fee_kopecks,omitempty"`
	Enabled            bool      `json:"enabled"`
	EffectiveFrom      time.Time `json:"effective_from"`
	CreatedAt          time.Time `json:"created_at"`
}

// normalize trims + validates the pricing using the shared provider validator.
func (in *ProviderPricingRule) normalize() error {
	in.Provider = strings.TrimSpace(in.Provider)
	in.PaymentMethod = strings.ToUpper(strings.TrimSpace(in.PaymentMethod))
	if in.Provider == "" || len(in.Provider) > 80 || !paymentMethods[in.PaymentMethod] {
		return ErrInvalidInput
	}
	// PayerCustomer is a sentinel that satisfies the shared validator; the real
	// payer mode for the provider cost lives on the fee rule, not here.
	pricing := safedeal.ProviderPricing{
		Provider:           in.Provider,
		PaymentMethod:      in.PaymentMethod,
		PercentBasisPoints: in.PercentBasisPoints,
		FixedFeeKopecks:    in.FixedFeeKopecks,
		MinimumFeeKopecks:  in.MinimumFeeKopecks,
		MaximumFeeKopecks:  in.MaximumFeeKopecks,
		PayerMode:          safedeal.PayerCustomer,
	}
	if err := pricing.Validate(); err != nil {
		return ErrInvalidInput
	}
	return nil
}

// Repository persists versioned fee and provider-pricing rules and reads the
// actor's roles for authorization.
type Repository interface {
	Roles(ctx context.Context, actor string) ([]string, error)
	ListFeeRules(ctx context.Context) ([]FeeRule, error)
	CreateFeeRule(ctx context.Context, actor string, in FeeRule, reason, requestID string) (FeeRule, error)
	ListProviderPricing(ctx context.Context) ([]ProviderPricingRule, error)
	CreateProviderPricing(ctx context.Context, actor string, in ProviderPricingRule, reason, requestID string) (ProviderPricingRule, error)
}

type Service struct{ Repository Repository }

// require enforces that the actor is an ADMIN or SUPER_ADMIN. An ordinary
// MODERATOR is intentionally NOT permitted to change payment economics.
func (s Service) require(ctx context.Context, actor string) error {
	if actor == "" || s.Repository == nil {
		return ErrForbidden
	}
	roles, err := s.Repository.Roles(ctx, actor)
	if err != nil {
		return err
	}
	for _, role := range roles {
		switch strings.ToUpper(strings.TrimSpace(role)) {
		case "ADMIN", "SUPER_ADMIN":
			return nil
		}
	}
	return ErrForbidden
}

// guard enforces authorization, confirmation and a reason for every mutation.
func (s Service) guard(ctx context.Context, actor string, confirm bool, reason string) (string, error) {
	if err := s.require(ctx, actor); err != nil {
		return "", err
	}
	if !confirm {
		return "", ErrConfirmationRequired
	}
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) < 3 || len([]rune(reason)) > 2000 {
		return "", ErrReasonRequired
	}
	return reason, nil
}

func (s Service) ListFeeRules(ctx context.Context, actor string) ([]FeeRule, error) {
	if err := s.require(ctx, actor); err != nil {
		return nil, err
	}
	return s.Repository.ListFeeRules(ctx)
}

func (s Service) CreateFeeRule(ctx context.Context, actor string, in FeeRule, confirm bool, reason, requestID string) (FeeRule, error) {
	reason, err := s.guard(ctx, actor, confirm, reason)
	if err != nil {
		return FeeRule{}, err
	}
	if err := in.normalize(); err != nil {
		return FeeRule{}, err
	}
	return s.Repository.CreateFeeRule(ctx, actor, in, reason, requestID)
}

func (s Service) ListProviderPricing(ctx context.Context, actor string) ([]ProviderPricingRule, error) {
	if err := s.require(ctx, actor); err != nil {
		return nil, err
	}
	return s.Repository.ListProviderPricing(ctx)
}

func (s Service) CreateProviderPricing(ctx context.Context, actor string, in ProviderPricingRule, confirm bool, reason, requestID string) (ProviderPricingRule, error) {
	reason, err := s.guard(ctx, actor, confirm, reason)
	if err != nil {
		return ProviderPricingRule{}, err
	}
	if err := in.normalize(); err != nil {
		return ProviderPricingRule{}, err
	}
	return s.Repository.CreateProviderPricing(ctx, actor, in, reason, requestID)
}
