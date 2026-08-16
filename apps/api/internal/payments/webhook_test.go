package payments

import (
	"context"
	"errors"
	"testing"
)

type webhookFake struct {
	Attempt
	events          map[string]bool
	updates         int
	attachedAttempt string
}

func (f *webhookFake) PersistWebhookEvent(_ context.Context, e WebhookEvent) (bool, error) {
	if f.events == nil {
		f.events = map[string]bool{}
	}
	if f.events[e.EventID] {
		return false, nil
	}
	f.events[e.EventID] = true
	return true, nil
}
func (*webhookFake) MarkWebhookVerified(context.Context, ProviderName, string) error { return nil }
func (f *webhookFake) AttachWebhookAttempt(_ context.Context, _ ProviderName, _ string, attemptID string) error {
	f.attachedAttempt = attemptID
	return nil
}
func (*webhookFake) MarkWebhookProcessed(context.Context, ProviderName, string, string) error {
	return nil
}
func (f *webhookFake) FindByProviderExternalID(_ context.Context, p ProviderName, id string) (Attempt, bool, error) {
	return f.Attempt, f.Provider == p && f.ProviderOperationID == id, nil
}
func (f *webhookFake) Update(_ context.Context, a Attempt) error {
	f.Attempt = a
	f.updates++
	return nil
}

type verifier struct {
	event VerifiedEvent
	err   error
}

func (v verifier) VerifyWebhook(context.Context, []byte, map[string][]string) (VerifiedEvent, error) {
	return v.event, v.err
}
func TestWebhookRejectsInvalidAndDeduplicatesVerifiedEvent(t *testing.T) {
	f := &webhookFake{Attempt: Attempt{ID: "11111111-1111-1111-1111-111111111111", Provider: ProviderYooKassa, ProviderOperationID: "pay-1", Status: StatusProcessing}}
	s := WebhookService{Repository: f, Verifiers: map[ProviderName]WebhookVerifier{ProviderYooKassa: verifier{event: VerifiedEvent{Provider: ProviderYooKassa, ID: "evt-1", ExternalOperationID: "pay-1", Status: StatusSucceeded}}}, Now: fixedTime}
	if ok, err := s.Handle(context.Background(), ProviderYooKassa, []byte(`{"safe":true}`), nil); err != nil || !ok {
		t.Fatalf("valid webhook: %v %v", ok, err)
	}
	if f.attachedAttempt != f.Attempt.ID {
		t.Fatalf("verified webhook not linked to payment attempt: got %q want %q", f.attachedAttempt, f.Attempt.ID)
	}
	if ok, err := s.Handle(context.Background(), ProviderYooKassa, []byte(`{"safe":true}`), nil); err != nil || ok || f.updates != 1 {
		t.Fatalf("replay mutated: %v %v updates=%d", ok, err, f.updates)
	}
	s.Verifiers[ProviderYooKassa] = verifier{err: errors.New("bad signature")}
	if _, err := s.Handle(context.Background(), ProviderYooKassa, []byte(`x`), nil); !errors.Is(err, ErrWebhookInvalid) {
		t.Fatalf("invalid webhook accepted: %v", err)
	}
}
