package notification

import (
	"context"
	"testing"
)

type fakeDigest struct{ called int }

func (f *fakeDigest) ProcessDigest(context.Context) (bool, error) { f.called++; return true, nil }

func TestDigestProcessorRunsOneBoundedBatch(t *testing.T) {
	f := &fakeDigest{}
	if err := (DigestProcessor{Repository: f}).Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.called != 1 {
		t.Fatalf("calls=%d", f.called)
	}
}

func TestDigestPresentationCoversMarketplaceCatalogs(t *testing.T) {
	for event, want := range map[string]string{"PROJECT_PUBLISHED": "/projects", "VACANCY_PUBLISHED": "/vacancies", "SERVICE_PUBLISHED": "/services"} {
		kind, path := digestPresentation(event)
		if kind == "" || path != want {
			t.Fatalf("event=%s kind=%s path=%s", event, kind, path)
		}
	}
}
