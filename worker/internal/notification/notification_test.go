package notification

import (
	"context"
	"testing"
)

type fake struct{ called int }

func (f *fake) ProcessOne(context.Context) (bool, error) { f.called++; return true, nil }
func TestProcessorDispatchesOneTransactionalEvent(t *testing.T) {
	f := &fake{}
	if err := (Processor{Repository: f}).Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.called != 1 {
		t.Fatalf("calls=%d", f.called)
	}
}
