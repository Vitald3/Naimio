package monetization

import "context"

type CheckoutRequest struct{ UserID, PlanID, ReturnURL string }
type ProviderCheckout struct{ ID, URL string }
type ProviderSubscription struct{ ID, Status string }
type WebhookEvent struct {
	ID, Type, SubscriptionID string
	Payload                  []byte
}

// SubscriptionPaymentProvider is intentionally separate from Safe Deal payments.
// A future adapter owns checkout, recurring billing and verified webhook translation.
type SubscriptionPaymentProvider interface {
	CreateCheckout(context.Context, CheckoutRequest) (ProviderCheckout, error)
	CancelSubscription(context.Context, string) error
	GetSubscription(context.Context, string) (ProviderSubscription, error)
	VerifyWebhook(context.Context, []byte, map[string]string) error
	HandleWebhook(context.Context, []byte) (WebhookEvent, error)
}

type DisabledProvider struct{}

func (DisabledProvider) CreateCheckout(context.Context, CheckoutRequest) (ProviderCheckout, error) {
	return ProviderCheckout{}, ErrProviderUnavailable
}
func (DisabledProvider) CancelSubscription(context.Context, string) error {
	return ErrProviderUnavailable
}
func (DisabledProvider) GetSubscription(context.Context, string) (ProviderSubscription, error) {
	return ProviderSubscription{}, ErrProviderUnavailable
}
func (DisabledProvider) VerifyWebhook(context.Context, []byte, map[string]string) error {
	return ErrProviderUnavailable
}
func (DisabledProvider) HandleWebhook(context.Context, []byte) (WebhookEvent, error) {
	return WebhookEvent{}, ErrProviderUnavailable
}
