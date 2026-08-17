package media_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"freelance/apps/api/internal/auth"
	"freelance/apps/api/internal/blog"
	"freelance/apps/api/internal/media"
)

// generateValidJPEG creates a valid binary JPEG image for testing
func generateValidJPEG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 100, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
	return buf.Bytes()
}

type mockAuthorizer struct{}

func (mockAuthorizer) CanUsePurpose(_ context.Context, _, _ string) (bool, error) {
	return true, nil
}

func TestE2EMediaServingAndNextImageCompatibility(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	repo := &media.Store{Objects: map[string]media.Object{}}
	storage := &media.MemoryStorage{Objects: map[string]media.StoredObject{}, Now: func() time.Time { return now }}
	mediaService := media.Service{
		Repository:        repo,
		Storage:           storage,
		PurposeAuthorizer: mockAuthorizer{},
		Bucket:            "test-bucket",
		Now:               func() time.Time { return now },
		AutoClean:         true,
	}

	mediaHandler := media.Handler{Service: mediaService}
	blogHandler := blog.Handler{
		MediaHandler: http.HandlerFunc(mediaHandler.ServeMedia),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/uploads/presign", mediaHandler.Presign)
	mux.HandleFunc("/api/v1/uploads/", mediaHandler.Item)
	mux.HandleFunc("/api/v1/media/", mediaHandler.ServeMedia)
	mux.HandleFunc("/api/v1/blog/media/", blogHandler.Public)
	mux.Handle("/api/v1/dev-storage", storage)

	actorID := "11111111-1111-4111-8111-111111111111"
	jpegData := generateValidJPEG()

	// Step 1: Create Presign for BLOG_COVER
	presignPayload := map[string]any{
		"purpose":    "BLOG_COVER",
		"filename":   "test_cover.jpg",
		"mime_type":  "image/jpeg",
		"size_bytes": len(jpegData),
	}
	payloadBytes, _ := json.Marshal(presignPayload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/presign", bytes.NewReader(payloadBytes))
	req = req.WithContext(auth.WithAdminSession(auth.WithActorID(req.Context(), actorID), true))
	req.Header.Set("Content-Type", "application/json")

	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)

	if res.Code != http.StatusCreated {
		t.Fatalf("presign failed with status %d", res.Code)
	}

	var presignRes struct {
		Data struct {
			MediaID   string            `json:"media_id"`
			UploadURL string            `json:"upload_url"`
			Headers   map[string]string `json:"headers"`
		} `json:"data"`
	}
	_ = json.NewDecoder(res.Body).Decode(&presignRes)

	mediaID := presignRes.Data.MediaID
	if mediaID == "" {
		t.Fatal("empty mediaID from presign")
	}

	// Step 2: Upload JPEG binary via PUT to /api/v1/dev-storage
	uploadURL := presignRes.Data.UploadURL
	putReq := httptest.NewRequest(http.MethodPut, uploadURL, bytes.NewReader(jpegData))
	for k, v := range presignRes.Data.Headers {
		putReq.Header.Set(k, v)
	}
	putRes := httptest.NewRecorder()
	mux.ServeHTTP(putRes, putReq)

	if putRes.Code != http.StatusNoContent && putRes.Code != http.StatusOK {
		t.Fatalf("upload PUT failed with status %d", putRes.Code)
	}

	// Step 3: Complete upload
	completeReq := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/"+mediaID+"/complete", strings.NewReader("{}"))
	completeReq = completeReq.WithContext(auth.WithActorID(completeReq.Context(), actorID))
	completeReq.Header.Set("Content-Type", "application/json")

	completeRes := httptest.NewRecorder()
	mux.ServeHTTP(completeRes, completeReq)

	if completeRes.Code != http.StatusOK {
		t.Fatalf("complete failed with status %d", completeRes.Code)
	}

	// Step 4: Verify Universal Media Endpoint GET /api/v1/media/{id}
	mediaReq := httptest.NewRequest(http.MethodGet, "/api/v1/media/"+mediaID, nil)
	mediaRes := httptest.NewRecorder()
	mux.ServeHTTP(mediaRes, mediaReq)

	if mediaRes.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/media/%s returned status %d, expected 200", mediaID, mediaRes.Code)
	}

	contentType := mediaRes.Header().Get("Content-Type")
	if contentType != "image/jpeg" {
		t.Fatalf("expected Content-Type image/jpeg, got %s", contentType)
	}

	cacheControl := mediaRes.Header().Get("Cache-Control")
	if cacheControl != "public, max-age=86400" {
		t.Fatalf("expected Cache-Control 'public, max-age=86400', got %s", cacheControl)
	}

	contentLength := mediaRes.Header().Get("Content-Length")
	if contentLength == "" {
		t.Fatal("expected Content-Length header to be set")
	}

	bodyBytes := mediaRes.Body.Bytes()
	if !bytes.Equal(bodyBytes, jpegData) {
		t.Fatalf("returned image body does not match uploaded JPEG! got %d bytes, want %d bytes", len(bodyBytes), len(jpegData))
	}

	// Step 5: Verify Legacy Blog Media Endpoint GET /api/v1/blog/media/{id} delegates cleanly without redirect
	legacyReq := httptest.NewRequest(http.MethodGet, "/api/v1/blog/media/"+mediaID, nil)
	legacyRes := httptest.NewRecorder()
	mux.ServeHTTP(legacyRes, legacyReq)

	if legacyRes.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/blog/media/%s returned status %d (expected 200 without redirect)", mediaID, legacyRes.Code)
	}
	if legacyRes.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("expected Content-Type image/jpeg on legacy endpoint, got %s", legacyRes.Header().Get("Content-Type"))
	}

	legacyBody := legacyRes.Body.Bytes()
	if !bytes.Equal(legacyBody, jpegData) {
		t.Fatalf("legacy endpoint body mismatch")
	}

	// Step 6: Verify image decoding directly from the HTTP 200 response body (as Next.js Image does)
	img, format, err := image.Decode(bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("failed to decode image from media response body: %v", err)
	}
	if format != "jpeg" {
		t.Fatalf("decoded image format is %s, expected jpeg", format)
	}
	bounds := img.Bounds()
	if bounds.Dx() != 100 || bounds.Dy() != 100 {
		t.Fatalf("image dimensions mismatch: %dx%d, expected 100x100", bounds.Dx(), bounds.Dy())
	}
}
