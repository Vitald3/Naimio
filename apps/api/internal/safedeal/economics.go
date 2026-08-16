package safedeal

// Provider-independent payment economics.
//
// Two distinct fee classes are modelled and never merged into one opaque
// number:
//
//   - PLATFORM COMMISSION — the marketplace's own revenue.
//   - PROVIDER / SAFE-DEAL COST — the cost charged by the payment or
//     safe-deal provider (a pass-through cost, not marketplace revenue).
//
// Each class independently decides who pays it (the "payer mode"). All money
// is integer kopecks; there is no float anywhere in this file by design.
// CalculateDealQuote is the single authoritative money function — the
// frontend must never compute authoritative amounts.

// Fee payer modes. Both fee classes accept the same set.
const (
	// PayerCustomer adds the whole fee on top of what the customer pays.
	PayerCustomer = "CUSTOMER"
	// PayerFreelancer subtracts the whole fee from the freelancer payout.
	PayerFreelancer = "FREELANCER"
	// PayerSplit divides the fee between customer and freelancer by
	// CustomerShareBasisPoints; the platform takes no part in SPLIT.
	PayerSplit = "SPLIT"
	// PayerPlatform makes the platform absorb the whole fee (subsidy).
	PayerPlatform = "PLATFORM"
)

// Payment methods recognised for provider pricing. OTHER is the catch-all so
// the domain never has to special-case a specific bank product.
const (
	MethodCard    = "CARD"
	MethodSBP     = "SBP"
	MethodTPay    = "T_PAY"
	MethodSberPay = "SBER_PAY"
	MethodOther   = "OTHER"
)

func validPayerMode(mode string) bool {
	switch mode {
	case PayerCustomer, PayerFreelancer, PayerSplit, PayerPlatform:
		return true
	}
	return false
}

func validPaymentMethod(method string) bool {
	switch method {
	case MethodCard, MethodSBP, MethodTPay, MethodSberPay, MethodOther:
		return true
	}
	return false
}

// FeePolicy is the platform-commission half of a versioned fee rule. It is a
// snapshot: once copied onto a deal it must never change, even if an admin
// edits the live rule afterwards.
type FeePolicy struct {
	Version                  int    `json:"version"`
	CommissionBasisPoints    int    `json:"commission_basis_points"`
	MinimumFeeKopecks        int64  `json:"minimum_fee_kopecks"`
	MaximumFeeKopecks        *int64 `json:"maximum_fee_kopecks,omitempty"`
	PayerMode                string `json:"payer_mode"`
	CustomerShareBasisPoints int    `json:"customer_share_basis_points"`
}

// ProviderPricing is the provider-cost half. It is configured per provider and
// payment method and versioned independently of the platform commission, so a
// new provider can be priced without touching commission policy.
type ProviderPricing struct {
	Version                  int    `json:"version"`
	Provider                 string `json:"provider"`
	PaymentMethod            string `json:"payment_method"`
	PercentBasisPoints       int    `json:"percent_basis_points"`
	FixedFeeKopecks          int64  `json:"fixed_fee_kopecks"`
	MinimumFeeKopecks        int64  `json:"minimum_fee_kopecks"`
	MaximumFeeKopecks        *int64 `json:"maximum_fee_kopecks,omitempty"`
	PayerMode                string `json:"payer_mode"`
	CustomerShareBasisPoints int    `json:"customer_share_basis_points"`
}

// Adjustments are explicit, auditable discounts/subsidies. They are the
// extension point for referral discounts, first-deal promos and platform
// subsidies: nothing is hardcoded here, callers pass concrete kopecks. A
// customer discount lowers the customer total; a freelancer bonus raises the
// payout. Both are funded by the platform and therefore reduce net revenue.
type Adjustments struct {
	CustomerDiscountKopecks int64  `json:"customer_discount_kopecks"`
	FreelancerBonusKopecks  int64  `json:"freelancer_bonus_kopecks"`
	Reason                  string `json:"reason,omitempty"`
}

// QuoteInput is the complete, authoritative input to a deal quote.
type QuoteInput struct {
	WorkAmountKopecks int64
	Currency          string
	Fee               FeePolicy
	Provider          ProviderPricing
	Adjustments       Adjustments
}

// FeeAllocation splits one fee three ways. The three parts always sum exactly
// to TotalKopecks.
type FeeAllocation struct {
	TotalKopecks      int64 `json:"total_kopecks"`
	CustomerKopecks   int64 `json:"customer_kopecks"`
	FreelancerKopecks int64 `json:"freelancer_kopecks"`
	PlatformKopecks   int64 `json:"platform_kopecks"`
}

// DealQuote is the authoritative money breakdown for a deal. Every field is
// integer kopecks. Callers persist this verbatim as the immutable per-deal
// economic snapshot.
type DealQuote struct {
	Currency          string `json:"currency"`
	WorkAmountKopecks int64  `json:"work_amount_kopecks"`

	// The two fee classes, each with who pays each part.
	PlatformFee FeeAllocation `json:"platform_fee"`
	ProviderFee FeeAllocation `json:"provider_fee"`

	// Explicit adjustments applied (echoed for audit).
	CustomerDiscountKopecks int64 `json:"customer_discount_kopecks"`
	FreelancerBonusKopecks  int64 `json:"freelancer_bonus_kopecks"`

	// What each side actually moves.
	CustomerTotalKopecks    int64 `json:"customer_total_kopecks"`
	FreelancerPayoutKopecks int64 `json:"freelancer_payout_kopecks"`

	// Platform economics.
	PlatformGrossRevenueKopecks int64 `json:"platform_gross_revenue_kopecks"`
	PlatformProviderCostKopecks int64 `json:"platform_provider_cost_kopecks"`
	PlatformSubsidyKopecks      int64 `json:"platform_subsidy_kopecks"`
	PlatformNetRevenueKopecks   int64 `json:"platform_net_revenue_kopecks"`

	// Provenance: which versioned configs produced this quote.
	FeeRuleVersion         int `json:"fee_rule_version"`
	ProviderPricingVersion int `json:"provider_pricing_version"`
}

// Validate checks a platform fee policy in isolation (used by admin config
// before a new version is stored).
func (p FeePolicy) Validate() error {
	if p.CommissionBasisPoints < 0 || p.CommissionBasisPoints > 10000 {
		return ErrInvalid
	}
	if p.MinimumFeeKopecks < 0 {
		return ErrInvalid
	}
	if p.MaximumFeeKopecks != nil && (*p.MaximumFeeKopecks < 0 || *p.MaximumFeeKopecks < p.MinimumFeeKopecks) {
		return ErrInvalid
	}
	if !validPayerMode(p.PayerMode) {
		return ErrInvalid
	}
	if p.PayerMode == PayerSplit && (p.CustomerShareBasisPoints < 0 || p.CustomerShareBasisPoints > 10000) {
		return ErrInvalid
	}
	return nil
}

// Validate checks a provider pricing rule in isolation.
func (p ProviderPricing) Validate() error {
	if p.PercentBasisPoints < 0 || p.PercentBasisPoints > 10000 {
		return ErrInvalid
	}
	if p.FixedFeeKopecks < 0 || p.MinimumFeeKopecks < 0 {
		return ErrInvalid
	}
	if p.MaximumFeeKopecks != nil && (*p.MaximumFeeKopecks < 0 || *p.MaximumFeeKopecks < p.MinimumFeeKopecks) {
		return ErrInvalid
	}
	if !validPayerMode(p.PayerMode) {
		return ErrInvalid
	}
	if p.PayerMode == PayerSplit && (p.CustomerShareBasisPoints < 0 || p.CustomerShareBasisPoints > 10000) {
		return ErrInvalid
	}
	if p.PaymentMethod != "" && !validPaymentMethod(p.PaymentMethod) {
		return ErrInvalid
	}
	return nil
}

// computeFee applies percent + fixed then clamps to [minimum, maximum].
// Integer division truncates toward zero, matching Postgres integer division
// so the SQL trigger can mirror this exactly.
func computeFee(work int64, basisPoints int, fixed, minimum int64, maximum *int64) (int64, error) {
	if work <= 0 || basisPoints < 0 || basisPoints > 10000 || fixed < 0 || minimum < 0 {
		return 0, ErrInvalid
	}
	fee := work*int64(basisPoints)/10000 + fixed
	if fee < minimum {
		fee = minimum
	}
	if maximum != nil && fee > *maximum {
		fee = *maximum
	}
	if fee < 0 {
		return 0, ErrInvalid
	}
	return fee, nil
}

// allocate divides a fee among customer / freelancer / platform according to
// the payer mode. The rounding remainder in SPLIT goes to the freelancer, so
// the parts always sum back to the total exactly.
func allocate(fee int64, mode string, customerShareBasisPoints int) (FeeAllocation, error) {
	a := FeeAllocation{TotalKopecks: fee}
	switch mode {
	case PayerCustomer:
		a.CustomerKopecks = fee
	case PayerFreelancer:
		a.FreelancerKopecks = fee
	case PayerPlatform:
		a.PlatformKopecks = fee
	case PayerSplit:
		if customerShareBasisPoints < 0 || customerShareBasisPoints > 10000 {
			return FeeAllocation{}, ErrInvalid
		}
		customer := fee * int64(customerShareBasisPoints) / 10000
		a.CustomerKopecks = customer
		a.FreelancerKopecks = fee - customer
	default:
		return FeeAllocation{}, ErrInvalid
	}
	return a, nil
}

// CalculateDealQuote is the authoritative money calculation for a Safe Deal.
// Given the work amount, a platform fee policy, a provider pricing rule and
// optional explicit adjustments it returns the full breakdown with strong
// arithmetic invariants enforced before returning.
func CalculateDealQuote(in QuoteInput) (DealQuote, error) {
	if in.WorkAmountKopecks <= 0 || in.Currency != "RUB" {
		return DealQuote{}, ErrInvalid
	}
	if err := in.Fee.Validate(); err != nil {
		return DealQuote{}, err
	}
	if err := in.Provider.Validate(); err != nil {
		return DealQuote{}, err
	}
	if in.Adjustments.CustomerDiscountKopecks < 0 || in.Adjustments.FreelancerBonusKopecks < 0 {
		return DealQuote{}, ErrInvalid
	}

	platformFee, err := computeFee(in.WorkAmountKopecks, in.Fee.CommissionBasisPoints, 0, in.Fee.MinimumFeeKopecks, in.Fee.MaximumFeeKopecks)
	if err != nil {
		return DealQuote{}, err
	}
	providerFee, err := computeFee(in.WorkAmountKopecks, in.Provider.PercentBasisPoints, in.Provider.FixedFeeKopecks, in.Provider.MinimumFeeKopecks, in.Provider.MaximumFeeKopecks)
	if err != nil {
		return DealQuote{}, err
	}

	platformAlloc, err := allocate(platformFee, in.Fee.PayerMode, in.Fee.CustomerShareBasisPoints)
	if err != nil {
		return DealQuote{}, err
	}
	providerAlloc, err := allocate(providerFee, in.Provider.PayerMode, in.Provider.CustomerShareBasisPoints)
	if err != nil {
		return DealQuote{}, err
	}

	discount := in.Adjustments.CustomerDiscountKopecks
	bonus := in.Adjustments.FreelancerBonusKopecks

	customerTotal := in.WorkAmountKopecks + platformAlloc.CustomerKopecks + providerAlloc.CustomerKopecks - discount
	freelancerPayout := in.WorkAmountKopecks - platformAlloc.FreelancerKopecks - providerAlloc.FreelancerKopecks + bonus
	if customerTotal <= 0 || freelancerPayout <= 0 {
		return DealQuote{}, ErrInvalid
	}

	// The platform always remits the full provider fee to the provider, but
	// recovers the customer/freelancer shares; the part it genuinely bears is
	// providerAlloc.Platform. Its gross commission revenue is the full
	// platform fee regardless of who funds it; subsidy is everything the
	// platform gives back (its own fee shares, provider shares, adjustments).
	grossRevenue := platformFee
	providerCost := providerAlloc.PlatformKopecks
	subsidy := platformAlloc.PlatformKopecks + providerAlloc.PlatformKopecks + discount + bonus
	netRevenue := customerTotal - providerFee - freelancerPayout

	q := DealQuote{
		Currency:                    in.Currency,
		WorkAmountKopecks:           in.WorkAmountKopecks,
		PlatformFee:                 platformAlloc,
		ProviderFee:                 providerAlloc,
		CustomerDiscountKopecks:     discount,
		FreelancerBonusKopecks:      bonus,
		CustomerTotalKopecks:        customerTotal,
		FreelancerPayoutKopecks:     freelancerPayout,
		PlatformGrossRevenueKopecks: grossRevenue,
		PlatformProviderCostKopecks: providerCost,
		PlatformSubsidyKopecks:      subsidy,
		PlatformNetRevenueKopecks:   netRevenue,
		FeeRuleVersion:              in.Fee.Version,
		ProviderPricingVersion:      in.Provider.Version,
	}
	if err := q.validateInvariants(); err != nil {
		return DealQuote{}, err
	}
	return q, nil
}

// validateInvariants enforces the arithmetic that must hold for every quote.
// A violation means an internal calculation bug, so it fails closed.
func (q DealQuote) validateInvariants() error {
	nonNegative := []int64{
		q.PlatformFee.CustomerKopecks, q.PlatformFee.FreelancerKopecks, q.PlatformFee.PlatformKopecks,
		q.ProviderFee.CustomerKopecks, q.ProviderFee.FreelancerKopecks, q.ProviderFee.PlatformKopecks,
		q.CustomerDiscountKopecks, q.FreelancerBonusKopecks,
		q.PlatformGrossRevenueKopecks, q.PlatformProviderCostKopecks,
	}
	for _, v := range nonNegative {
		if v < 0 {
			return ErrInvalid
		}
	}
	// Each fee's parts sum to its total.
	if q.PlatformFee.CustomerKopecks+q.PlatformFee.FreelancerKopecks+q.PlatformFee.PlatformKopecks != q.PlatformFee.TotalKopecks {
		return ErrInvalid
	}
	if q.ProviderFee.CustomerKopecks+q.ProviderFee.FreelancerKopecks+q.ProviderFee.PlatformKopecks != q.ProviderFee.TotalKopecks {
		return ErrInvalid
	}
	// Customer and freelancer legs.
	if q.CustomerTotalKopecks != q.WorkAmountKopecks+q.PlatformFee.CustomerKopecks+q.ProviderFee.CustomerKopecks-q.CustomerDiscountKopecks {
		return ErrInvalid
	}
	if q.FreelancerPayoutKopecks != q.WorkAmountKopecks-q.PlatformFee.FreelancerKopecks-q.ProviderFee.FreelancerKopecks+q.FreelancerBonusKopecks {
		return ErrInvalid
	}
	if q.CustomerTotalKopecks <= 0 || q.FreelancerPayoutKopecks <= 0 {
		return ErrInvalid
	}
	// Platform revenue provenance.
	if q.PlatformGrossRevenueKopecks != q.PlatformFee.TotalKopecks {
		return ErrInvalid
	}
	if q.PlatformProviderCostKopecks != q.ProviderFee.PlatformKopecks {
		return ErrInvalid
	}
	if q.PlatformSubsidyKopecks != q.PlatformFee.PlatformKopecks+q.ProviderFee.PlatformKopecks+q.CustomerDiscountKopecks+q.FreelancerBonusKopecks {
		return ErrInvalid
	}
	// Master invariant: money in equals money out.
	if q.CustomerTotalKopecks != q.FreelancerPayoutKopecks+q.ProviderFee.TotalKopecks+q.PlatformNetRevenueKopecks {
		return ErrInvalid
	}
	// Cross-check net via gross minus subsidy.
	if q.PlatformNetRevenueKopecks != q.PlatformGrossRevenueKopecks-q.PlatformSubsidyKopecks {
		return ErrInvalid
	}
	return nil
}
