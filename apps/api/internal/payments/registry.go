// Package payments contains provider-neutral routing metadata. It deliberately
// contains no business-state transitions: Safe Deal and subscriptions retain
// ownership of their own state machines.
package payments

import (
	"errors"
	"sort"
	"strings"
)

type ProviderName string

const (
	ProviderDisabled      ProviderName = "disabled"
	ProviderYooKassa      ProviderName = "yookassa"
	ProviderTBank         ProviderName = "tbank"
	ProviderYandexPay     ProviderName = "yandex_pay"
	ProviderCloudPayments ProviderName = "cloudpayments"
	ProviderRobokassa     ProviderName = "robokassa"
)

type Domain string

const (
	DomainSafeDeal        Domain = "SAFE_DEAL"
	DomainProSubscription Domain = "PRO_SUBSCRIPTION"
	DomainPlatformPayment Domain = "OTHER_PLATFORM_PAYMENT"
)

type Capability string

const (
	CapabilityOneTimePayment              Capability = "ONE_TIME_PAYMENT"
	CapabilityMerchantManagedRecurring    Capability = "MERCHANT_MANAGED_RECURRING"
	CapabilityProviderManagedSubscription Capability = "PROVIDER_MANAGED_SUBSCRIPTION"
	CapabilityCard                        Capability = "CARD"
	CapabilitySBP                         Capability = "SBP"
	CapabilityTPay                        Capability = "T_PAY"
	CapabilityYandexPay                   Capability = "YANDEX_PAY"
	CapabilityTwoStage                    Capability = "TWO_STAGE_PAYMENT"
	CapabilityCapture                     Capability = "CAPTURE"
	CapabilityVoid                        Capability = "VOID"
	CapabilityFullRefund                  Capability = "FULL_REFUND"
	CapabilityPartialRefund               Capability = "PARTIAL_REFUND"
	CapabilitySafeDeal                    Capability = "SAFE_DEAL"
	CapabilityPayoutCard                  Capability = "PAYOUT_CARD"
	CapabilityPayoutSBP                   Capability = "PAYOUT_SBP"
	CapabilityPayoutSelfEmployed          Capability = "PAYOUT_SELF_EMPLOYED"
	CapabilitySavedPaymentMethod          Capability = "SAVED_PAYMENT_METHOD"
	CapabilityFiscalization               Capability = "FISCALIZATION"
	CapabilityWebhooks                    Capability = "WEBHOOKS"
	CapabilityReconciliation              Capability = "RECONCILIATION"
	CapabilitySandbox                     Capability = "SANDBOX"
)

var (
	ErrUnknownProvider     = errors.New("unknown payment provider")
	ErrProviderUnavailable = errors.New("payment provider is not enabled and configured")
	ErrUnsupportedRoute    = errors.New("payment provider lacks required capability")
)

type Environment string

const (
	EnvironmentSandbox    Environment = "sandbox"
	EnvironmentProduction Environment = "production"
)

type Descriptor struct {
	Name         ProviderName
	Capabilities map[Capability]bool
}

func (d Descriptor) Supports(required ...Capability) bool {
	for _, capability := range required {
		if !d.Capabilities[capability] {
			return false
		}
	}
	return true
}

// Registry is the one routing source for new operations. Existing operations
// always persist their provider and must not be rerouted by this type.
type Registry struct{ providers map[ProviderName]Descriptor }

func NewRegistry(descriptors ...Descriptor) Registry {
	providers := make(map[ProviderName]Descriptor, len(descriptors)+1)
	providers[ProviderDisabled] = Descriptor{Name: ProviderDisabled, Capabilities: map[Capability]bool{}}
	for _, descriptor := range descriptors {
		providers[descriptor.Name] = descriptor
	}
	return Registry{providers: providers}
}

func DefaultRegistry() Registry {
	return NewRegistry(
		// Capabilities describe code that is actually callable in this release,
		// not commercial products that a provider may offer. This makes routing a
		// fail-closed boundary while remaining adapters are completed.
		Descriptor{Name: ProviderYooKassa, Capabilities: set(CapabilityOneTimePayment, CapabilityMerchantManagedRecurring, CapabilityCard, CapabilitySBP, CapabilityTwoStage, CapabilityCapture, CapabilityVoid, CapabilityFullRefund, CapabilityPartialRefund, CapabilitySafeDeal, CapabilityPayoutCard, CapabilitySavedPaymentMethod, CapabilityFiscalization, CapabilityWebhooks, CapabilityReconciliation, CapabilitySandbox)},
		Descriptor{Name: ProviderTBank, Capabilities: set(CapabilityOneTimePayment, CapabilityMerchantManagedRecurring, CapabilityCard, CapabilitySBP, CapabilityTPay, CapabilityFullRefund, CapabilitySavedPaymentMethod, CapabilityWebhooks, CapabilityReconciliation)},
		Descriptor{Name: ProviderYandexPay, Capabilities: set(CapabilityOneTimePayment, CapabilityProviderManagedSubscription, CapabilityCard, CapabilitySBP, CapabilityYandexPay, CapabilityFullRefund, CapabilityWebhooks, CapabilityReconciliation, CapabilitySandbox)},
		Descriptor{Name: ProviderCloudPayments, Capabilities: set(CapabilityOneTimePayment, CapabilityMerchantManagedRecurring, CapabilityCard, CapabilityTwoStage, CapabilityCapture, CapabilityVoid, CapabilityFullRefund, CapabilityPartialRefund, CapabilitySavedPaymentMethod, CapabilityWebhooks, CapabilityReconciliation, CapabilitySandbox)},
		Descriptor{Name: ProviderRobokassa, Capabilities: set(CapabilityOneTimePayment, CapabilityMerchantManagedRecurring, CapabilityCard, CapabilityFullRefund, CapabilitySavedPaymentMethod, CapabilityWebhooks, CapabilityReconciliation, CapabilitySandbox)},
	)
}

func set(values ...Capability) map[Capability]bool {
	m := make(map[Capability]bool, len(values))
	for _, v := range values {
		m[v] = true
	}
	return m
}

func (r Registry) SupportsRecurring(name ProviderName) bool {
	d, ok := r.Provider(name)
	return ok && (d.Capabilities[CapabilityMerchantManagedRecurring] || d.Capabilities[CapabilityProviderManagedSubscription])
}

func (r Registry) Provider(name ProviderName) (Descriptor, bool) {
	d, ok := r.providers[name]
	return d, ok
}

func (r Registry) Providers() []Descriptor {
	items := make([]Descriptor, 0, len(r.providers))
	for _, item := range r.providers {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

// ValidateRoute checks a configured provider before a routing setting can be
// saved. It does not choose fallbacks and thus cannot cause a cross-provider
// retry after an unknown financial result.
func (r Registry) ValidateRoute(name ProviderName, enabled, configured bool, required ...Capability) error {
	descriptor, ok := r.Provider(name)
	if !ok {
		return ErrUnknownProvider
	}
	if name == ProviderDisabled || !enabled || !configured {
		return ErrProviderUnavailable
	}
	if !descriptor.Supports(required...) {
		return ErrUnsupportedRoute
	}
	return nil
}

func ParseProvider(value string) ProviderName {
	return ProviderName(strings.ToLower(strings.TrimSpace(value)))
}
