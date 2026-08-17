package blog

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSanitizeRichContentRemovesStoredXSS(t *testing.T) {
	raw := `<h2 onclick="x()">Title</h2><script>alert(1)</script><p><a href="javascript:alert(1)">bad</a><a href="https://example.com">good</a></p><img src="/api/v1/media/11111111-1111-4111-8111-111111111111" onerror="x()"><img src="/api/v1/blog/media/22222222-2222-4222-8222-222222222222">`
	clean, err := Sanitize(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"script", "onclick", "javascript:", "onerror"} {
		if strings.Contains(clean, bad) {
			t.Fatalf("unsafe %q in %s", bad, clean)
		}
	}
	if !strings.Contains(clean, `rel="noopener noreferrer nofollow"`) || !strings.Contains(clean, "/api/v1/media/") || !strings.Contains(clean, "/api/v1/blog/media/") {
		t.Fatalf("missing safe content: %s", clean)
	}
}
func TestValidateWritePublicationStatesAndSlug(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour)
	valid := WriteRequest{Title: "Статья", Slug: "valid-slug", Excerpt: "Лид", ContentHTML: "<p>Текст</p>", Status: "SCHEDULED", ScheduledAt: &future}
	if err := validateWrite(&valid); err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Hour)
	valid.ScheduledAt = &past
	if err := validateWrite(&valid); err == nil {
		t.Fatal("past schedule accepted")
	}
	valid.Status = "DRAFT"
	valid.Slug = "../../bad"
	if err := validateWrite(&valid); err == nil {
		t.Fatal("unsafe slug accepted")
	}
}
func TestSanitizeOnlyAllowsOwnedMediaRouteShape(t *testing.T) {
	clean, _ := Sanitize(`<img src="https://tracker.invalid/x.png"><img src="/api/v1/blog/media/not-a-uuid">`)
	if strings.Contains(clean, "img") {
		t.Fatalf("external or malformed image survived: %s", clean)
	}
}

func TestBlogMediaHandlerDelegatesToMediaHandler(t *testing.T) {
	called := false
	mediaHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "image/jpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("image-data"))
	})
	h := Handler{MediaHandler: mediaHandler}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/blog/media/11111111-1111-4111-8111-111111111111", nil)
	res := httptest.NewRecorder()
	h.Public(res, req)

	if !called {
		t.Fatal("expected MediaHandler to be called")
	}
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}
	if res.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("expected image/jpeg, got %s", res.Header().Get("Content-Type"))
	}
}
