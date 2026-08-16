package email

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type repo struct {
	job     *Job
	sent    bool
	attempt int
	final   bool
}

func (r *repo) Claim(context.Context) (*Job, error) { j := r.job; r.job = nil; return j, nil }
func (r *repo) Sent(context.Context, string) error  { r.sent = true; return nil }
func (r *repo) Retry(_ context.Context, _ string, a int, _ string, f bool) error {
	r.attempt = a
	r.final = f
	return nil
}

type provider struct {
	err           error
	subject, body string
}

func (p *provider) Send(_ context.Context, _, s, b string) error {
	p.subject, p.body = s, b
	return p.err
}
func TestProcessorSendsSafeTemplate(t *testing.T) {
	r := &repo{job: &Job{ID: "1", Template: "new_message", Address: "u@example.test"}}
	p := &provider{}
	if err := (Processor{Repository: r, Provider: p, PublicBaseURL: "https://example.test"}).Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !r.sent || strings.Contains(p.body, "private body") || !strings.Contains(p.body, "/settings/notifications") {
		t.Fatalf("unsafe or missing template: %q", p.body)
	}
}
func TestProcessorBoundsRetries(t *testing.T) {
	r := &repo{job: &Job{ID: "1", Template: "new_message", Address: "u@example.test", Attempts: 4}}
	p := &provider{err: errors.New("temporary")}
	if err := (Processor{Repository: r, Provider: p, MaxAttempts: 5}).Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.attempt != 5 || !r.final {
		t.Fatalf("retry not bounded: %d %v", r.attempt, r.final)
	}
}

func TestVerificationEmailUsesOnlyOpaqueToken(t *testing.T) {
	r := &repo{job: &Job{ID: "1", Template: "verify_email", Address: "u@example.test", Payload: map[string]any{"token": "opaque-token"}}}
	p := &provider{}
	if err := (Processor{Repository: r, Provider: p, PublicBaseURL: "https://example.test"}).Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !r.sent || !strings.Contains(p.body, "/verify-email?token=opaque-token") || strings.Contains(p.body, "password") {
		t.Fatalf("invalid verification template: %q", p.body)
	}
}
