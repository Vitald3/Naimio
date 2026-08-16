package payments

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"time"
)

var ErrWebhookInvalid = errors.New("invalid payment webhook")

// VerifiedEvent is the normalized, authenticity-checked provider event. Raw
// bodies never leave the adapter verifier or enter this persistence layer.
type VerifiedEvent struct {
	Provider                                 ProviderName
	ID, ExternalOperationID, Type, RawStatus string
	SavedMethodRef                           string
	Status                                   Status
	OccurredAt                               time.Time
}
type WebhookVerifier interface {
	VerifyWebhook(context.Context, []byte, map[string][]string) (VerifiedEvent, error)
}
type WebhookRepository interface {
	PersistWebhookEvent(context.Context, WebhookEvent) (bool, error)
	MarkWebhookVerified(context.Context, ProviderName, string) error
	AttachWebhookAttempt(context.Context, ProviderName, string, string) error
	MarkWebhookProcessed(context.Context, ProviderName, string, string) error
	FindByProviderExternalID(context.Context, ProviderName, string) (Attempt, bool, error)
	Update(context.Context, Attempt) error
}
type WebhookService struct {
	Repository              WebhookRepository
	Verifiers               map[ProviderName]WebhookVerifier
	Providers               ProviderSet
	AfterTransition         func(context.Context, Attempt) error
	AfterVerifiedTransition func(context.Context, Attempt, VerifiedEvent) error
	Now                     func() time.Time
}

func (s WebhookService) Handle(ctx context.Context, provider ProviderName, body []byte, headers map[string][]string) (bool, error) {
	var v WebhookVerifier
	var err error
	if s.Providers.Runtime != nil {
		v, err = s.Providers.Webhook(provider)
	} else {
		v = s.Verifiers[provider]
		if v == nil {
			err = ErrProviderUnavailable
		}
	}
	if err != nil || v == nil {
		return false, ErrWebhookInvalid
	}
	e, err := v.VerifyWebhook(ctx, body, headers)
	if err != nil || e.Provider != provider || e.ID == "" || e.ExternalOperationID == "" {
		return false, ErrWebhookInvalid
	}
	created, err := s.Repository.PersistWebhookEvent(ctx, WebhookEvent{Provider: provider, EventID: e.ID, Type: e.Type, ExternalReference: e.ExternalOperationID, PayloadHash: hash(body)})
	if err != nil || !created {
		return false, err
	}
	if err = s.Repository.MarkWebhookVerified(ctx, provider, e.ID); err != nil {
		return false, err
	}
	a, found, err := s.Repository.FindByProviderExternalID(ctx, provider, e.ExternalOperationID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, s.Repository.MarkWebhookProcessed(ctx, provider, e.ID, "UNMATCHED")
	}
	if err = s.Repository.AttachWebhookAttempt(ctx, provider, e.ID, a.ID); err != nil {
		return false, err
	}
	a.ProviderRawStatus = e.RawStatus
	if e.SavedMethodRef != "" {
		a.ProviderPaymentMethodRef = e.SavedMethodRef
	}
	// A retry can arrive after the attempt transition was persisted but before
	// the owning domain callback completed. Re-run that callback for the same
	// authoritative status instead of permanently losing domain synchronization.
	if a.Status == e.Status {
		a.UpdatedAt = s.now()
	} else if err = a.Transition(e.Status, s.now()); err != nil {
		return false, s.Repository.MarkWebhookProcessed(ctx, provider, e.ID, "IGNORED")
	}
	if err = s.Repository.Update(ctx, a); err != nil {
		return false, err
	}
	if s.AfterVerifiedTransition != nil {
		if err = s.AfterVerifiedTransition(ctx, a, e); err != nil {
			return false, err
		}
	} else if s.AfterTransition != nil {
		if err = s.AfterTransition(ctx, a); err != nil {
			return false, err
		}
	}
	return true, s.Repository.MarkWebhookProcessed(ctx, provider, e.ID, "PROCESSED")
}
func (s WebhookService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func hash(value []byte) string { v := sha256.Sum256(value); return hex.EncodeToString(v[:]) }
func constantEqual(a, b string) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
