package media

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"freelance/apps/api/internal/auth"
)

const (
	ownerID = "11111111-1111-4111-8111-111111111111"
	otherID = "22222222-2222-4222-8222-222222222222"
	mediaID = "33333333-3333-4333-8333-333333333333"
)

func testService() (Service, *Store, *MemoryStorage) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	repository := &Store{Objects: map[string]Object{}}
	storage := &MemoryStorage{Objects: map[string]StoredObject{}, Now: func() time.Time { return now }}
	return Service{Repository: repository, Storage: storage, Bucket: "test", Now: func() time.Time { return now }}, repository, storage
}

func validPresign() PresignInput {
	return PresignInput{Purpose: PurposePortfolio, Filename: "screen.png", MIMEType: "image/png", Size: 128}
}

func TestPresignValidationRejectsMaliciousFiles(t *testing.T) {
	for name, input := range map[string]PresignInput{
		"path traversal": {Purpose: PurposePortfolio, Filename: "../screen.png", MIMEType: "image/png", Size: 10},
		"mime mismatch":  {Purpose: PurposePortfolio, Filename: "screen.jpg", MIMEType: "image/png", Size: 10},
		"svg":            {Purpose: PurposePortfolio, Filename: "screen.svg", MIMEType: "image/svg+xml", Size: 10},
		"archive":        {Purpose: PurposePortfolio, Filename: "files.zip", MIMEType: "application/zip", Size: 10},
		"empty":          {Purpose: PurposePortfolio, Filename: "screen.png", MIMEType: "image/png", Size: 0},
		"oversized":      {Purpose: PurposePortfolio, Filename: "screen.png", MIMEType: "image/png", Size: (10 << 20) + 1},
		"future purpose": {Purpose: "MESSAGE", Filename: "screen.png", MIMEType: "image/png", Size: 10},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidatePresign(normalizeInput(input)); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestFeaturePurposePresign(t *testing.T) {
	for _, purpose := range []string{PurposeService, PurposeProject, PurposeAvatar} {
		t.Run(purpose, func(t *testing.T) {
			service, repository, _ := testService()
			input := validPresign()
			input.Purpose = purpose
			presign, err := service.CreatePresign(context.Background(), ownerID, input)
			if err != nil {
				t.Fatal(err)
			}
			object, err := repository.GetOwned(context.Background(), ownerID, presign.MediaID)
			if err != nil || object.Purpose != purpose || !strings.HasPrefix(object.ObjectKey, strings.ToLower(purpose)+"/") {
				t.Fatalf("object = %#v, error = %v", object, err)
			}
		})
	}
}

func TestAvatarUploadAcceptsOnlySmallRasterImages(t *testing.T) {
	valid := validPresign()
	valid.Purpose = PurposeAvatar
	if err := ValidatePresign(valid); err != nil {
		t.Fatal(err)
	}
	for _, input := range []PresignInput{{Purpose: PurposeAvatar, Filename: "avatar.pdf", MIMEType: "application/pdf", Size: 100}, {Purpose: PurposeAvatar, Filename: "avatar.png", MIMEType: "image/png", Size: (5 << 20) + 1}} {
		if err := ValidatePresign(input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("input=%#v error=%v", input, err)
		}
	}
}

func TestAvatarHandlerFallsBackToDeterministicAsset(t *testing.T) {
	service, _, _ := testService()
	h := Handler{Service: service}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/avatars/demo-user", nil)
	res := httptest.NewRecorder()
	h.Avatar(res, req)
	if res.Code != http.StatusTemporaryRedirect || !strings.HasPrefix(res.Header().Get("Location"), "/media/avatars/avatar-") {
		t.Fatalf("status=%d location=%q", res.Code, res.Header().Get("Location"))
	}
}

func TestPresignCompleteScanBoundaryAndDelete(t *testing.T) {
	service, repository, storage := testService()
	presign, err := service.CreatePresign(context.Background(), ownerID, validPresign())
	if err != nil || presign.MediaID == "" || presign.Headers["Content-Type"] != "image/png" || strings.Contains(presign.UploadURL, "screen.png") {
		t.Fatalf("presign = %#v, error = %v", presign, err)
	}
	object, err := repository.GetOwned(context.Background(), ownerID, presign.MediaID)
	if err != nil || object.ScanStatus != ScanPending || object.UploadedAt != nil || !strings.HasPrefix(object.ObjectKey, "portfolio/"+ownerID+"/") {
		t.Fatalf("object = %#v, error = %v", object, err)
	}
	if _, err := service.Complete(context.Background(), otherID, presign.MediaID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user complete error = %v", err)
	}
	storage.Objects[object.ObjectKey] = StoredObject{SizeBytes: object.SizeBytes, MIMEType: object.MIMEType, DetectedMIME: "image/png"}
	completed, err := service.Complete(context.Background(), ownerID, presign.MediaID)
	if err != nil || completed.UploadedAt == nil || completed.ScanStatus != ScanPending {
		t.Fatalf("completed = %#v, error = %v", completed, err)
	}
	pending, err := service.Get(context.Background(), ownerID, presign.MediaID)
	if err != nil || pending.DownloadURL != "" {
		t.Fatalf("pending view = %#v, error = %v", pending, err)
	}
	if err := repository.MarkScanResult(context.Background(), presign.MediaID, ScanClean); err != nil {
		t.Fatal(err)
	}
	clean, err := service.Get(context.Background(), ownerID, presign.MediaID)
	if err != nil || clean.DownloadURL == "" || clean.ExpiresAt == nil || clean.ScanStatus != ScanClean {
		t.Fatalf("clean view = %#v, error = %v", clean, err)
	}
	if err := service.Delete(context.Background(), otherID, presign.MediaID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user delete error = %v", err)
	}
	if err := service.Delete(context.Background(), ownerID, presign.MediaID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(context.Background(), ownerID, presign.MediaID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted object error = %v", err)
	}
}

func TestCompleteRejectsProviderMetadataAndSignatureMismatch(t *testing.T) {
	for name, stored := range map[string]StoredObject{
		"size":      {SizeBytes: 127, MIMEType: "image/png", DetectedMIME: "image/png"},
		"declared":  {SizeBytes: 128, MIMEType: "image/jpeg", DetectedMIME: "image/png"},
		"signature": {SizeBytes: 128, MIMEType: "image/png", DetectedMIME: "application/pdf"},
	} {
		t.Run(name, func(t *testing.T) {
			service, repository, storage := testService()
			presign, err := service.CreatePresign(context.Background(), ownerID, validPresign())
			if err != nil {
				t.Fatal(err)
			}
			object, _ := repository.GetOwned(context.Background(), ownerID, presign.MediaID)
			storage.Objects[object.ObjectKey] = stored
			if _, err := service.Complete(context.Background(), ownerID, presign.MediaID); !errors.Is(err, ErrInvalidObject) {
				t.Fatalf("error = %v", err)
			}
			failed, _ := repository.GetOwned(context.Background(), ownerID, presign.MediaID)
			if failed.ScanStatus != ScanFailed || failed.UploadedAt == nil {
				t.Fatalf("failed object = %#v", failed)
			}
		})
	}
}

func TestUploadHandlersAuthorizationAndBodyLimits(t *testing.T) {
	service, _, _ := testService()
	handler := Handler{Service: service}
	res := httptest.NewRecorder()
	handler.Presign(res, httptest.NewRequest(http.MethodPost, "/api/v1/uploads/presign", strings.NewReader(`{}`)))
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", res.Code)
	}

	res = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/presign", strings.NewReader(strings.Repeat(" ", (32<<10)+1)+`{}`))
	req = req.WithContext(auth.WithActorID(req.Context(), ownerID))
	handler.Presign(res, req)
	if res.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized status = %d", res.Code)
	}

	res = httptest.NewRecorder()
	body, _ := json.Marshal(map[string]any{"purpose": PurposePortfolio, "filename": "screen.png", "mime_type": "image/png", "size_bytes": 128, "user_id": otherID})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/uploads/presign", strings.NewReader(string(body)))
	req = req.WithContext(auth.WithActorID(req.Context(), ownerID))
	handler.Presign(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("client identity status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestPendingObjectCannotBeMarkedClean(t *testing.T) {
	_, repository, _ := testService()
	repository.Objects[mediaID] = Object{ID: mediaID, OwnerID: ownerID, ScanStatus: ScanPending}
	if err := repository.MarkScanResult(context.Background(), mediaID, ScanClean); !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v", err)
	}
}
