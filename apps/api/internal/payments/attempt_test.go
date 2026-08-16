package payments

import (
	"context"
	"testing"
	"time"
)

func TestAttemptUsesExactKopecksAndRequiresReconciliationForUnknown(t *testing.T) {
	a := Attempt{Domain: DomainProSubscription, InternalReferenceID: "11111111-1111-4111-8111-111111111111", Provider: ProviderYooKassa, OperationType: OperationPayment, Status: StatusCreated, AmountKopecks: 199900, Currency: "rub", IdempotencyKey: "pro-payment-unique"}
	if err := a.Normalize(); err != nil {
		t.Fatal(err)
	}
	if a.AmountKopecks != 199900 || a.Currency != "RUB" {
		t.Fatalf("money was not preserved: %+v", a)
	}
	if err := a.Transition(StatusUnknownReconciliation, fixedTime()); err != nil {
		t.Fatal(err)
	}
	if a.ReconciliationState != ReconciliationRequired {
		t.Fatalf("unknown result must reconcile: %+v", a)
	}
}
func TestTerminalAttemptCannotBeReassignedOrRetried(t *testing.T) {
	a := Attempt{Status: StatusProcessing}
	if err := a.Transition(StatusSucceeded, fixedTime()); err != nil {
		t.Fatal(err)
	}
	if err := a.Transition(StatusProcessing, fixedTime()); err != ErrInvalidTransition {
		t.Fatalf("terminal attempt transitioned: %v", err)
	}
}

func TestServicePreventsDuplicateBusinessAttempt(t *testing.T) {
	s := Service{Repository: &Store{}, Now: fixedTime}
	in := Attempt{Domain: DomainProSubscription, InternalReferenceID: "22222222-2222-4222-8222-222222222222", Provider: ProviderYooKassa, OperationType: OperationPayment, AmountKopecks: 10000, Currency: "RUB", IdempotencyKey: "stable-attempt-key"}
	first, err := s.Create(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := s.Create(context.Background(), in); err != nil || again.ID != first.ID {
		t.Fatalf("idempotency failed: %+v %v", again, err)
	}
	in.AmountKopecks++
	if _, err := s.Create(context.Background(), in); err != ErrAttemptConflict {
		t.Fatalf("mutated duplicate accepted: %v", err)
	}
}
func fixedTime() (v time.Time) { return time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC) }
