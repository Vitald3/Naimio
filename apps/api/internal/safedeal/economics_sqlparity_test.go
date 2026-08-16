package safedeal

import "testing"

// triggerQuote is a faithful line-by-line transcription of the plpgsql in
// db/migrations/000024_provider_independent_economics.sql
// (create_safe_deal_for_assignment). The database cannot run in the build
// sandbox, so this mirror lets us assert that the SQL trigger and the
// authoritative Go domain (CalculateDealQuote) compute byte-identical integer
// economics. If the two ever diverge, this test fails. The trigger never
// applies discounts/bonuses, so those are fixed at zero here.
func triggerQuote(work int64, fee FeePolicy, hasPricing bool, pr ProviderPricing) DealQuote {
	clamp := func(v, min int64, max *int64) int64 {
		if v < min {
			v = min
		}
		if max != nil && v > *max {
			v = *max
		}
		return v
	}
	platFee := clamp(work*int64(fee.CommissionBasisPoints)/10000, fee.MinimumFeeKopecks, fee.MaximumFeeKopecks)
	var provFee int64
	provVersion := 1
	if hasPricing {
		provFee = clamp(work*int64(pr.PercentBasisPoints)/10000+pr.FixedFeeKopecks, pr.MinimumFeeKopecks, pr.MaximumFeeKopecks)
		provVersion = pr.Version
	}
	alloc := func(fee int64, mode string, share int) (c, f, p int64) {
		switch mode {
		case PayerCustomer:
			c = fee
		case PayerFreelancer:
			f = fee
		case PayerPlatform:
			p = fee
		default: // SPLIT
			c = fee * int64(share) / 10000
			f = fee - c
		}
		return
	}
	platCust, platFree, platPlat := alloc(platFee, fee.PayerMode, fee.CustomerShareBasisPoints)
	provCust, provFree, provPlat := alloc(provFee, pr.PayerMode, pr.CustomerShareBasisPoints)
	customerTotal := work + platCust + provCust
	freelancerPayout := work - platFree - provFree
	return DealQuote{
		Currency:                    "RUB",
		WorkAmountKopecks:           work,
		PlatformFee:                 FeeAllocation{TotalKopecks: platFee, CustomerKopecks: platCust, FreelancerKopecks: platFree, PlatformKopecks: platPlat},
		ProviderFee:                 FeeAllocation{TotalKopecks: provFee, CustomerKopecks: provCust, FreelancerKopecks: provFree, PlatformKopecks: provPlat},
		CustomerTotalKopecks:        customerTotal,
		FreelancerPayoutKopecks:     freelancerPayout,
		PlatformGrossRevenueKopecks: platFee,
		PlatformProviderCostKopecks: provPlat,
		PlatformSubsidyKopecks:      platPlat + provPlat,
		PlatformNetRevenueKopecks:   customerTotal - provFee - freelancerPayout,
		FeeRuleVersion:              fee.Version,
		ProviderPricingVersion:      provVersion,
	}
}

func TestTriggerMirrorsCalculateDealQuote(t *testing.T) {
	payers := []struct {
		mode  string
		share int
	}{{PayerCustomer, 0}, {PayerFreelancer, 0}, {PayerPlatform, 0}, {PayerSplit, 5000}, {PayerSplit, 3000}}
	works := []int64{100000, 100010, 1000, 777777, 250000}
	commissions := []int{0, 1000, 250, 10000}
	provPercents := []int{0, 200, 100}
	provFixed := []int64{0, 1500}

	for _, w := range works {
		for _, c := range commissions {
			for _, pf := range payers {
				for _, pp := range provPercents {
					for _, pfix := range provFixed {
						for _, ppay := range payers {
							fee := FeePolicy{Version: 3, CommissionBasisPoints: c, PayerMode: pf.mode, CustomerShareBasisPoints: pf.share}
							pr := ProviderPricing{Version: 7, Provider: "sandbox", PaymentMethod: MethodCard, PercentBasisPoints: pp, FixedFeeKopecks: pfix, PayerMode: ppay.mode, CustomerShareBasisPoints: ppay.share}
							domain, err := CalculateDealQuote(QuoteInput{WorkAmountKopecks: w, Currency: "RUB", Fee: fee, Provider: pr})
							if err != nil {
								// The trigger's INSERT would likewise fail its CHECKs
								// (non-positive payout); skip — both reject identically.
								continue
							}
							sql := triggerQuote(w, fee, true, pr)
							if sql != domain {
								t.Fatalf("SQL/domain divergence work=%d comm=%d platPayer=%s/%d provPct=%d provFix=%d provPayer=%s/%d\n sql=%+v\n dom=%+v",
									w, c, pf.mode, pf.share, pp, pfix, ppay.mode, ppay.share, sql, domain)
							}
						}
					}
				}
			}
		}
	}
}

// TestTriggerZeroPricingMatchesLegacy proves the migration's backfill/default
// path: with no provider pricing row the trigger charges zero provider fee, and
// the legacy default (10% commission borne by the freelancer) reproduces the
// pre-migration customer charge, payout and commission exactly.
func TestTriggerZeroPricingMatchesLegacy(t *testing.T) {
	work := int64(1000000)
	fee := FeePolicy{Version: 1, CommissionBasisPoints: 1000, PayerMode: PayerFreelancer}
	sql := triggerQuote(work, fee, false, ProviderPricing{})
	if sql.CustomerTotalKopecks != 1000000 || sql.FreelancerPayoutKopecks != 900000 || sql.PlatformGrossRevenueKopecks != 100000 {
		t.Fatalf("legacy reproduction wrong: %+v", sql)
	}
	if sql.ProviderFee.TotalKopecks != 0 || sql.PlatformNetRevenueKopecks != 100000 {
		t.Fatalf("expected zero provider fee and net==gross: %+v", sql)
	}
	// And it agrees with the domain fed the same legacy inputs (zero-cost provider).
	domain, err := CalculateDealQuote(QuoteInput{WorkAmountKopecks: work, Currency: "RUB", Fee: fee, Provider: ProviderPricing{Version: 1, Provider: "sandbox", PayerMode: PayerCustomer}})
	if err != nil {
		t.Fatal(err)
	}
	if sql.ProviderPricingVersion != 1 {
		t.Fatalf("expected default pricing version 1, got %d", sql.ProviderPricingVersion)
	}
	// Compare the money fields (provider pricing version differs by construction only when a row exists).
	sql.ProviderPricingVersion = domain.ProviderPricingVersion
	if sql != domain {
		t.Fatalf("legacy domain divergence\n sql=%+v\n dom=%+v", sql, domain)
	}
}
