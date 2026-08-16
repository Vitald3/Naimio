package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"freelance/apps/api/internal/media"
	"freelance/apps/api/internal/platform/objectstorage"
)

func TestAdminStorageSettingsEndpoints(t *testing.T) {
	repo := &fakeRepo{roles: map[string][]string{
		"11111111-1111-4111-8111-111111111111": {"ADMIN"},
		"22222222-2222-4222-8222-222222222222": {"USER"},
	}}
	service := Service{Repository: repo}

	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	localStorage := &media.MemoryStorage{Objects: map[string]media.StoredObject{}, Now: func() time.Time { return now }}
	manager, err := media.NewStorageManager(nil, "master-test-key-32-bytes-long!!", localStorage, objectstorage.S3Config{})
	if err != nil {
		t.Fatal(err)
	}

	handler := Handler{
		Service: service,
		ActorID: func(ctx context.Context) (string, bool) {
			val, ok := ctx.Value("actor_id").(string)
			return val, ok
		},
	}
	storageHandler := handler.StorageSettingsHandler(manager)

	// 1. Unauthorized request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/storage-settings", nil)
	w := httptest.NewRecorder()
	storageHandler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	// 2. Forbidden request (regular user)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/storage-settings", nil)
	req = req.WithContext(context.WithValue(req.Context(), "actor_id", "22222222-2222-4222-8222-222222222222"))
	w = httptest.NewRecorder()
	storageHandler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}

	// 3. Authorized GET settings
	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/storage-settings", nil)
	req = req.WithContext(context.WithValue(req.Context(), "actor_id", "11111111-1111-4111-8111-111111111111"))
	w = httptest.NewRecorder()
	storageHandler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var getResp struct {
		Data media.StorageSettings `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &getResp); err != nil {
		t.Fatal(err)
	}
	if getResp.Data.Provider != "local" {
		t.Fatalf("expected provider=local, got %s", getResp.Data.Provider)
	}

	// 4. Update to Local settings (valid)
	updatePayload := media.StorageSettingsUpdate{
		Provider: "local",
		S3: media.S3UpdateConfig{
			Endpoint:  "https://s3.yandexcloud.net",
			Region:    "ru-central1",
			Bucket:    "naimio-bucket",
			AccessKey: "YC123",
			SecretKey: "secret123",
			UseSSL:    true,
			PathStyle: true,
		},
	}
	body, _ := json.Marshal(updatePayload)
	req = httptest.NewRequest(http.MethodPut, "/api/v1/admin/storage-settings", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), "actor_id", "11111111-1111-4111-8111-111111111111"))
	w = httptest.NewRecorder()
	storageHandler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on update, got %d: %s", w.Code, w.Body.String())
	}

	// 5. Test connection failure with unreachable host
	testPayload := media.S3UpdateConfig{
		Endpoint:  "https://unreachable.s3.invalid",
		Region:    "ru-central1",
		Bucket:    "test-bucket",
		AccessKey: "key",
		SecretKey: "secret",
		UseSSL:    true,
	}
	testBody, _ := json.Marshal(testPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/storage-settings/test", bytes.NewReader(testBody))
	req = req.WithContext(context.WithValue(req.Context(), "actor_id", "11111111-1111-4111-8111-111111111111"))
	w = httptest.NewRecorder()
	storageHandler.ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for unreachable test connection, got %d", w.Code)
	}
}
