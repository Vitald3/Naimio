package payments

import (
	"context"
	"testing"
)

type reconcileRepo struct{ values []Attempt }

func (f *reconcileRepo) ListPendingReconciliation(context.Context, int) ([]Attempt, error) {
	return f.values, nil
}
func (f *reconcileRepo) Update(_ context.Context, a Attempt) error { f.values[0] = a; return nil }

type statusProvider struct{ status Status }

func (p statusProvider) GetStatus(context.Context, string) (Status, string, error) {
	return p.status, "succeeded", nil
}
func TestReconcilerKeepsProviderPinned(t *testing.T) {
	r := &reconcileRepo{values: []Attempt{{Provider: ProviderYooKassa, ProviderOperationID: "p", Status: StatusUnknownReconciliation, ReconciliationState: ReconciliationRequired}}}
	n, e := (Reconciler{Repository: r, Providers: map[ProviderName]StatusProvider{ProviderYooKassa: statusProvider{StatusSucceeded}}, Service: Service{Now: fixedTime}}).Run(context.Background(), 10)
	if e != nil || n != 1 || r.values[0].Status != StatusSucceeded {
		t.Fatalf("%d %v %+v", n, e, r.values[0])
	}
}

func TestReconcilerFallsBackToExplicitStatusProvider(t *testing.T) {
	provider := ProviderName("sandbox")
	r := &reconcileRepo{values: []Attempt{{Provider: provider, ProviderOperationID: "sb_pay_1", Status: StatusUnknownReconciliation, ReconciliationState: ReconciliationRequired}}}
	reconciler := Reconciler{
		Repository:  r,
		Providers:   map[ProviderName]StatusProvider{provider: statusProvider{StatusSucceeded}},
		ProviderSet: ProviderSet{Runtime: NewProviderRuntime()},
		Service:     Service{Now: fixedTime},
	}
	n, err := reconciler.Run(context.Background(), 10)
	if err != nil || n != 1 || r.values[0].Status != StatusSucceeded {
		t.Fatalf("processed=%d err=%v attempt=%+v", n, err, r.values[0])
	}
}
