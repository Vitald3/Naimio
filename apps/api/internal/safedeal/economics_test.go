package safedeal

import (
	"errors"
	"testing"
)

func kptr(v int64) *int64 { return &v }

// alloc is the expected three-way split of one fee.
type alloc struct{ total, customer, freelancer, platform int64 }

type quoteExp struct {
	platform, provider              alloc
	customerTotal, freelancerPayout int64
	gross, providerCost             int64
	subsidy, net                    int64
}

func assertAlloc(t *testing.T, label string, got FeeAllocation, want alloc) {
	t.Helper()
	if got.TotalKopecks != want.total || got.CustomerKopecks != want.customer || got.FreelancerKopecks != want.freelancer || got.PlatformKopecks != want.platform {
		t.Errorf("%s allocation = {total:%d cust:%d free:%d plat:%d}, want {total:%d cust:%d free:%d plat:%d}",
			label, got.TotalKopecks, got.CustomerKopecks, got.FreelancerKopecks, got.PlatformKopecks,
			want.total, want.customer, want.freelancer, want.platform)
	}
}

func TestCalculateDealQuote(t *testing.T) {
	// Base: 10% platform commission, 2% provider fee, both on 100000 kopecks.
	base := func() QuoteInput {
		return QuoteInput{
			WorkAmountKopecks: 100000,
			Currency:          "RUB",
			Fee:               FeePolicy{Version: 3, CommissionBasisPoints: 1000, PayerMode: PayerCustomer},
			Provider:          ProviderPricing{Version: 7, Provider: "sandbox", PaymentMethod: MethodCard, PercentBasisPoints: 200, PayerMode: PayerCustomer},
		}
	}
	withFee := func(mode string, share int) func(*QuoteInput) {
		return func(in *QuoteInput) { in.Fee.PayerMode = mode; in.Fee.CustomerShareBasisPoints = share }
	}
	withProvider := func(mode string, share int) func(*QuoteInput) {
		return func(in *QuoteInput) { in.Provider.PayerMode = mode; in.Provider.CustomerShareBasisPoints = share }
	}

	cases := []struct {
		name  string
		build func() QuoteInput
		mods  []func(*QuoteInput)
		exp   quoteExp
	}{
		// ---- Platform commission payer matrix (provider fee = CUSTOMER, 2000) ----
		{
			name:  "platform commission CUSTOMER",
			build: base, mods: []func(*QuoteInput){withFee(PayerCustomer, 0)},
			exp: quoteExp{
				platform: alloc{10000, 10000, 0, 0}, provider: alloc{2000, 2000, 0, 0},
				customerTotal: 112000, freelancerPayout: 100000, gross: 10000, providerCost: 0, subsidy: 0, net: 10000,
			},
		},
		{
			name:  "platform commission FREELANCER",
			build: base, mods: []func(*QuoteInput){withFee(PayerFreelancer, 0)},
			exp: quoteExp{
				platform: alloc{10000, 0, 10000, 0}, provider: alloc{2000, 2000, 0, 0},
				customerTotal: 102000, freelancerPayout: 90000, gross: 10000, providerCost: 0, subsidy: 0, net: 10000,
			},
		},
		{
			name:  "platform commission SPLIT 50-50 + provider CUSTOMER (canonical)",
			build: base, mods: []func(*QuoteInput){withFee(PayerSplit, 5000), withProvider(PayerCustomer, 0)},
			exp: quoteExp{
				platform: alloc{10000, 5000, 5000, 0}, provider: alloc{2000, 2000, 0, 0},
				customerTotal: 107000, freelancerPayout: 95000, gross: 10000, providerCost: 0, subsidy: 0, net: 10000,
			},
		},
		{
			name:  "platform commission SPLIT 30-70",
			build: base, mods: []func(*QuoteInput){withFee(PayerSplit, 3000)},
			exp: quoteExp{
				platform: alloc{10000, 3000, 7000, 0}, provider: alloc{2000, 2000, 0, 0},
				customerTotal: 105000, freelancerPayout: 93000, gross: 10000, providerCost: 0, subsidy: 0, net: 10000,
			},
		},
		{
			name:  "platform commission PLATFORM",
			build: base, mods: []func(*QuoteInput){withFee(PayerPlatform, 0)},
			exp: quoteExp{
				platform: alloc{10000, 0, 0, 10000}, provider: alloc{2000, 2000, 0, 0},
				customerTotal: 102000, freelancerPayout: 100000, gross: 10000, providerCost: 0, subsidy: 10000, net: 0,
			},
		},

		// ---- Provider fee payer matrix (platform commission = CUSTOMER, 10000) ----
		{
			name:  "provider fee FREELANCER",
			build: base, mods: []func(*QuoteInput){withProvider(PayerFreelancer, 0)},
			exp: quoteExp{
				platform: alloc{10000, 10000, 0, 0}, provider: alloc{2000, 0, 2000, 0},
				customerTotal: 110000, freelancerPayout: 98000, gross: 10000, providerCost: 0, subsidy: 0, net: 10000,
			},
		},
		{
			name:  "provider fee SPLIT 50-50",
			build: base, mods: []func(*QuoteInput){withProvider(PayerSplit, 5000)},
			exp: quoteExp{
				platform: alloc{10000, 10000, 0, 0}, provider: alloc{2000, 1000, 1000, 0},
				customerTotal: 111000, freelancerPayout: 99000, gross: 10000, providerCost: 0, subsidy: 0, net: 10000,
			},
		},
		{
			name:  "provider fee PLATFORM",
			build: base, mods: []func(*QuoteInput){withProvider(PayerPlatform, 0)},
			exp: quoteExp{
				platform: alloc{10000, 10000, 0, 0}, provider: alloc{2000, 0, 0, 2000},
				customerTotal: 110000, freelancerPayout: 100000, gross: 10000, providerCost: 2000, subsidy: 2000, net: 8000,
			},
		},

		// ---- Combinations ----
		{
			name:  "platform FREELANCER + provider CUSTOMER",
			build: base, mods: []func(*QuoteInput){withFee(PayerFreelancer, 0), withProvider(PayerCustomer, 0)},
			exp: quoteExp{
				platform: alloc{10000, 0, 10000, 0}, provider: alloc{2000, 2000, 0, 0},
				customerTotal: 102000, freelancerPayout: 90000, gross: 10000, providerCost: 0, subsidy: 0, net: 10000,
			},
		},
		{
			name:  "platform SPLIT 50-50 + provider SPLIT 50-50",
			build: base, mods: []func(*QuoteInput){withFee(PayerSplit, 5000), withProvider(PayerSplit, 5000)},
			exp: quoteExp{
				platform: alloc{10000, 5000, 5000, 0}, provider: alloc{2000, 1000, 1000, 0},
				customerTotal: 106000, freelancerPayout: 94000, gross: 10000, providerCost: 0, subsidy: 0, net: 10000,
			},
		},
		{
			name:  "platform PLATFORM + provider PLATFORM (fully subsidized)",
			build: base, mods: []func(*QuoteInput){withFee(PayerPlatform, 0), withProvider(PayerPlatform, 0)},
			exp: quoteExp{
				platform: alloc{10000, 0, 0, 10000}, provider: alloc{2000, 0, 0, 2000},
				customerTotal: 100000, freelancerPayout: 100000, gross: 10000, providerCost: 2000, subsidy: 12000, net: -2000,
			},
		},

		// ---- Minimum fee ----
		{
			name: "platform minimum fee floor",
			build: func() QuoteInput {
				in := base()
				in.WorkAmountKopecks = 1000
				in.Fee = FeePolicy{Version: 1, CommissionBasisPoints: 1000, MinimumFeeKopecks: 500, PayerMode: PayerCustomer}
				in.Provider = ProviderPricing{Version: 1, Provider: "sandbox", PercentBasisPoints: 0, PayerMode: PayerCustomer}
				return in
			},
			exp: quoteExp{
				platform: alloc{500, 500, 0, 0}, provider: alloc{0, 0, 0, 0},
				customerTotal: 1500, freelancerPayout: 1000, gross: 500, providerCost: 0, subsidy: 0, net: 500,
			},
		},

		// ---- Maximum fee ----
		{
			name: "platform maximum fee cap",
			build: func() QuoteInput {
				in := base()
				in.WorkAmountKopecks = 1000000
				in.Fee = FeePolicy{Version: 1, CommissionBasisPoints: 1000, MaximumFeeKopecks: kptr(50000), PayerMode: PayerCustomer}
				in.Provider = ProviderPricing{Version: 1, Provider: "sandbox", PercentBasisPoints: 0, PayerMode: PayerCustomer}
				return in
			},
			exp: quoteExp{
				platform: alloc{50000, 50000, 0, 0}, provider: alloc{0, 0, 0, 0},
				customerTotal: 1050000, freelancerPayout: 1000000, gross: 50000, providerCost: 0, subsidy: 0, net: 50000,
			},
		},

		// ---- Rounding (odd fee split, remainder to freelancer) ----
		{
			name: "rounding remainder to freelancer",
			build: func() QuoteInput {
				in := base()
				in.WorkAmountKopecks = 100010 // 10% -> 10001, odd
				in.Fee = FeePolicy{Version: 1, CommissionBasisPoints: 1000, PayerMode: PayerSplit, CustomerShareBasisPoints: 5000}
				in.Provider = ProviderPricing{Version: 1, Provider: "sandbox", PercentBasisPoints: 0, PayerMode: PayerCustomer}
				return in
			},
			exp: quoteExp{
				platform: alloc{10001, 5000, 5001, 0}, provider: alloc{0, 0, 0, 0},
				customerTotal: 105010, freelancerPayout: 95009, gross: 10001, providerCost: 0, subsidy: 0, net: 10001,
			},
		},

		// ---- Provider fixed fee + percent ----
		{
			name: "provider percent plus fixed fee",
			build: func() QuoteInput {
				in := base()
				in.Fee = FeePolicy{Version: 1, CommissionBasisPoints: 1000, PayerMode: PayerCustomer}
				in.Provider = ProviderPricing{Version: 2, Provider: "sandbox", PaymentMethod: MethodSBP, PercentBasisPoints: 100, FixedFeeKopecks: 1500, PayerMode: PayerCustomer}
				return in
			},
			exp: quoteExp{
				platform: alloc{10000, 10000, 0, 0}, provider: alloc{2500, 2500, 0, 0}, // 1% of 100000 = 1000 + 1500 fixed
				customerTotal: 112500, freelancerPayout: 100000, gross: 10000, providerCost: 0, subsidy: 0, net: 10000,
			},
		},

		// ---- Zero provider fee ----
		{
			name: "zero provider fee",
			build: func() QuoteInput {
				in := base()
				in.Fee = FeePolicy{Version: 1, CommissionBasisPoints: 1000, PayerMode: PayerCustomer}
				in.Provider = ProviderPricing{Version: 1, Provider: "sandbox", PercentBasisPoints: 0, FixedFeeKopecks: 0, PayerMode: PayerFreelancer}
				return in
			},
			exp: quoteExp{
				platform: alloc{10000, 10000, 0, 0}, provider: alloc{0, 0, 0, 0},
				customerTotal: 110000, freelancerPayout: 100000, gross: 10000, providerCost: 0, subsidy: 0, net: 10000,
			},
		},

		// ---- Discounts / subsidies (explicit adjustments) ----
		{
			name: "customer discount and freelancer bonus",
			build: func() QuoteInput {
				in := base()
				in.Fee = FeePolicy{Version: 1, CommissionBasisPoints: 1000, PayerMode: PayerCustomer}
				in.Provider = ProviderPricing{Version: 1, Provider: "sandbox", PercentBasisPoints: 200, PayerMode: PayerCustomer}
				in.Adjustments = Adjustments{CustomerDiscountKopecks: 3000, FreelancerBonusKopecks: 1000, Reason: "referral"}
				return in
			},
			exp: quoteExp{
				platform: alloc{10000, 10000, 0, 0}, provider: alloc{2000, 2000, 0, 0},
				// customer: 100000 + 10000 + 2000 - 3000 = 109000; payout: 100000 + 1000 = 101000
				customerTotal: 109000, freelancerPayout: 101000, gross: 10000, providerCost: 0, subsidy: 4000, net: 6000,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := tc.build()
			for _, m := range tc.mods {
				m(&in)
			}
			q, err := CalculateDealQuote(in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertAlloc(t, "platform", q.PlatformFee, tc.exp.platform)
			assertAlloc(t, "provider", q.ProviderFee, tc.exp.provider)
			if q.CustomerTotalKopecks != tc.exp.customerTotal {
				t.Errorf("customer total = %d, want %d", q.CustomerTotalKopecks, tc.exp.customerTotal)
			}
			if q.FreelancerPayoutKopecks != tc.exp.freelancerPayout {
				t.Errorf("freelancer payout = %d, want %d", q.FreelancerPayoutKopecks, tc.exp.freelancerPayout)
			}
			if q.PlatformGrossRevenueKopecks != tc.exp.gross {
				t.Errorf("gross revenue = %d, want %d", q.PlatformGrossRevenueKopecks, tc.exp.gross)
			}
			if q.PlatformProviderCostKopecks != tc.exp.providerCost {
				t.Errorf("provider cost = %d, want %d", q.PlatformProviderCostKopecks, tc.exp.providerCost)
			}
			if q.PlatformSubsidyKopecks != tc.exp.subsidy {
				t.Errorf("subsidy = %d, want %d", q.PlatformSubsidyKopecks, tc.exp.subsidy)
			}
			if q.PlatformNetRevenueKopecks != tc.exp.net {
				t.Errorf("net revenue = %d, want %d", q.PlatformNetRevenueKopecks, tc.exp.net)
			}
			// The constructor already runs validateInvariants; re-run defensively.
			if err := q.validateInvariants(); err != nil {
				t.Errorf("invariants failed: %v", err)
			}
		})
	}
}

func TestCalculateDealQuoteRejectsBadInput(t *testing.T) {
	cases := []struct {
		name string
		in   QuoteInput
	}{
		{"zero work", QuoteInput{WorkAmountKopecks: 0, Currency: "RUB", Fee: FeePolicy{CommissionBasisPoints: 1000, PayerMode: PayerCustomer}, Provider: ProviderPricing{Provider: "sandbox", PayerMode: PayerCustomer}}},
		{"negative work", QuoteInput{WorkAmountKopecks: -1, Currency: "RUB", Fee: FeePolicy{CommissionBasisPoints: 1000, PayerMode: PayerCustomer}, Provider: ProviderPricing{Provider: "sandbox", PayerMode: PayerCustomer}}},
		{"non-RUB currency", QuoteInput{WorkAmountKopecks: 100000, Currency: "USD", Fee: FeePolicy{CommissionBasisPoints: 1000, PayerMode: PayerCustomer}, Provider: ProviderPricing{Provider: "sandbox", PayerMode: PayerCustomer}}},
		{"bad payer mode", QuoteInput{WorkAmountKopecks: 100000, Currency: "RUB", Fee: FeePolicy{CommissionBasisPoints: 1000, PayerMode: "NOBODY"}, Provider: ProviderPricing{Provider: "sandbox", PayerMode: PayerCustomer}}},
		{"commission over 100%", QuoteInput{WorkAmountKopecks: 100000, Currency: "RUB", Fee: FeePolicy{CommissionBasisPoints: 10001, PayerMode: PayerCustomer}, Provider: ProviderPricing{Provider: "sandbox", PayerMode: PayerCustomer}}},
		{"negative adjustment", QuoteInput{WorkAmountKopecks: 100000, Currency: "RUB", Fee: FeePolicy{CommissionBasisPoints: 1000, PayerMode: PayerCustomer}, Provider: ProviderPricing{Provider: "sandbox", PayerMode: PayerCustomer}, Adjustments: Adjustments{CustomerDiscountKopecks: -1}}},
		// Fee entirely on freelancer wipes out the payout -> must be rejected, never negative.
		{"payout would be non-positive", QuoteInput{WorkAmountKopecks: 1000, Currency: "RUB", Fee: FeePolicy{CommissionBasisPoints: 10000, PayerMode: PayerFreelancer}, Provider: ProviderPricing{Provider: "sandbox", PayerMode: PayerCustomer}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := CalculateDealQuote(tc.in); !errors.Is(err, ErrInvalid) {
				t.Fatalf("expected ErrInvalid, got %v", err)
			}
		})
	}
}

// A quote is a value snapshot: mutating the source policy after the fact must
// not change an already-computed quote, and a different rule version must
// produce a distinguishable quote. This is the historical-immutability
// guarantee at the calculation layer.
func TestCalculateDealQuoteSnapshotImmutability(t *testing.T) {
	in := QuoteInput{
		WorkAmountKopecks: 100000,
		Currency:          "RUB",
		Fee:               FeePolicy{Version: 1, CommissionBasisPoints: 1000, PayerMode: PayerFreelancer},
		Provider:          ProviderPricing{Version: 1, Provider: "sandbox", PercentBasisPoints: 0, PayerMode: PayerCustomer},
	}
	first, err := CalculateDealQuote(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first.FeeRuleVersion != 1 || first.FreelancerPayoutKopecks != 90000 {
		t.Fatalf("unexpected first quote: version=%d payout=%d", first.FeeRuleVersion, first.FreelancerPayoutKopecks)
	}
	// Admin publishes a new rule version with a higher commission.
	in.Fee.Version = 2
	in.Fee.CommissionBasisPoints = 2000
	second, err := CalculateDealQuote(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The previously captured snapshot is unchanged...
	if first.FeeRuleVersion != 1 || first.FreelancerPayoutKopecks != 90000 || first.PlatformFee.TotalKopecks != 10000 {
		t.Errorf("first quote mutated: %+v", first)
	}
	// ...and the new version yields a different, correctly-versioned result.
	if second.FeeRuleVersion != 2 || second.PlatformFee.TotalKopecks != 20000 || second.FreelancerPayoutKopecks != 80000 {
		t.Errorf("second quote wrong: version=%d fee=%d payout=%d", second.FeeRuleVersion, second.PlatformFee.TotalKopecks, second.FreelancerPayoutKopecks)
	}
}
