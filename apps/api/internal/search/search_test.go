package search

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeWhitelistsSortAndBoundsQuery(t *testing.T) {
	in, err := Normalize("  api ", "newest", "NEWEST", "NEWEST")
	if err != nil || in.Query != "api" || in.Sort != "NEWEST" {
		t.Fatalf("input=%#v err=%v", in, err)
	}
	if _, err = Normalize("api", "DROP TABLE projects", "NEWEST", "NEWEST"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("sort=%v", err)
	}
	if _, err = Normalize(strings.Repeat("x", 121), "", "NEWEST", "NEWEST"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("query=%v", err)
	}
}
