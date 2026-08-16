package media

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"freelance/apps/api/internal/auth"
	"freelance/apps/api/internal/platform/objectstorage"
)

type mockRoundTripper func(*http.Request) (*http.Response, error)

func (m mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m(req)
}

func createMockS3Storage(t *testing.T) (Storage, map[string][]byte) {
	var mu sync.Mutex
	objects := make(map[string][]byte)

	client := &http.Client{
		Transport: mockRoundTripper(func(r *http.Request) (*http.Response, error) {
			mu.Lock()
			defer mu.Unlock()

			key := strings.TrimPrefix(r.URL.Path, "/test-bucket/")
			switch r.Method {
			case http.MethodPut:
				data, _ := io.ReadAll(r.Body)
				objects[key] = data
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewReader(nil)),
				}, nil
			case http.MethodHead:
				data, ok := objects[key]
				if !ok {
					return &http.Response{
						StatusCode: http.StatusNotFound,
						Header:     make(http.Header),
						Body:       io.NopCloser(bytes.NewReader(nil)),
					}, nil
				}
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewReader(nil)),
				}
				resp.Header.Set("Content-Length", strconv.Itoa(len(data)))
				resp.Header.Set("Content-Type", "image/png")
				return resp, nil
			case http.MethodGet:
				data, ok := objects[key]
				if !ok {
					return &http.Response{
						StatusCode: http.StatusNotFound,
						Header:     make(http.Header),
						Body:       io.NopCloser(bytes.NewReader(nil)),
					}, nil
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewReader(data)),
				}, nil
			case http.MethodDelete:
				delete(objects, key)
				return &http.Response{
					StatusCode: http.StatusNoContent,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewReader(nil)),
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusMethodNotAllowed,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewReader(nil)),
				}, nil
			}
		}),
	}

	s3Instance, err := objectstorage.NewS3(objectstorage.S3Config{
		Endpoint:   "https://s3.mock.test",
		Region:     "ru-central1",
		Bucket:     "test-bucket",
		AccessKey:  "test-access",
		SecretKey:  "test-secret",
		HTTPClient: client,
	})
	if err != nil {
		t.Fatalf("failed to create mock s3: %v", err)
	}

	return newS3Adapter(s3Instance), objects
}

func TestStorageManagerSwitchingAndCrossStorage(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

	localStorage := &MemoryStorage{Objects: map[string]StoredObject{}, Now: func() time.Time { return now }}
	s3Storage, s3Objects := createMockS3Storage(t)

	manager, err := NewStorageManager(nil, "master-test-key-32-bytes-long!!", localStorage, objectstorage.S3Config{})
	if err != nil {
		t.Fatalf("NewStorageManager failed: %v", err)
	}
	manager.s3Storage = s3Storage

	repository := &Store{Objects: map[string]Object{}}
	service := Service{
		Repository: repository,
		Storage:    manager,
		Resolver:   manager,
		Bucket:     "local-media",
		Now:        func() time.Time { return now },
		AutoClean:  true,
	}

	// 1. Initial mode is LOCAL
	st, prov, bucket, err := manager.ActiveStorage(ctx)
	if err != nil || prov != "local" || st != localStorage || bucket != "local-media" {
		t.Fatalf("expected local storage, got prov=%s, bucket=%s, err=%v", prov, bucket, err)
	}

	// 2. Upload file in LOCAL mode
	presignLocal, err := service.CreatePresign(ctx, ownerID, PresignInput{
		Purpose:  PurposeAvatar,
		Filename: "local_avatar.png",
		MIMEType: "image/png",
		Size:     128,
	})
	if err != nil {
		t.Fatalf("CreatePresign (local) failed: %v", err)
	}

	localObj, err := repository.GetOwned(ctx, ownerID, presignLocal.MediaID)
	if err != nil || localObj.StorageProvider != "local" {
		t.Fatalf("expected StorageProvider=local, got %#v, err=%v", localObj, err)
	}

	// Put data into local storage and complete
	localStorage.Objects[localObj.ObjectKey] = StoredObject{SizeBytes: 128, MIMEType: "image/png", DetectedMIME: "image/png"}
	completedLocal, err := service.Complete(ctx, ownerID, presignLocal.MediaID)
	if err != nil || completedLocal.ScanStatus != ScanClean {
		t.Fatalf("Complete (local) failed: %v, obj=%#v", err, completedLocal)
	}

	// 3. Switch to S3 mode
	manager.mu.Lock()
	manager.activeType = "s3"
	manager.s3Config = S3UpdateConfig{
		Endpoint:  "https://s3.mock.test",
		Region:    "ru-central1",
		Bucket:    "test-bucket",
		AccessKey: "test-access",
		SecretKey: "test-secret",
	}
	manager.mu.Unlock()

	activeSt, activeProv, activeBucket, err := manager.ActiveStorage(ctx)
	if err != nil || activeProv != "s3" || activeSt != s3Storage || activeBucket != "test-bucket" {
		t.Fatalf("expected S3 storage, got prov=%s, bucket=%s, err=%v", activeProv, activeBucket, err)
	}

	// 4. Old local file must still be viewable while S3 is active
	viewLocal, err := service.Get(ctx, ownerID, presignLocal.MediaID)
	if err != nil || viewLocal.DownloadURL == "" {
		t.Fatalf("Get old local file while S3 active failed: %v, view=%#v", err, viewLocal)
	}

	// 5. Upload new file in S3 mode
	presignS3, err := service.CreatePresign(ctx, ownerID, PresignInput{
		Purpose:  PurposeAvatar,
		Filename: "s3_avatar.png",
		MIMEType: "image/png",
		Size:     256,
	})
	if err != nil {
		t.Fatalf("CreatePresign (S3) failed: %v", err)
	}

	s3Obj, err := repository.GetOwned(ctx, ownerID, presignS3.MediaID)
	if err != nil || s3Obj.StorageProvider != "s3" || s3Obj.Bucket != "test-bucket" {
		t.Fatalf("expected StorageProvider=s3 and bucket=test-bucket, got %#v, err=%v", s3Obj, err)
	}

	// Put valid PNG data into mock S3 and complete
	s3Data := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte("a"), 248)...)
	s3Objects[s3Obj.ObjectKey] = s3Data
	completedS3, err := service.Complete(ctx, ownerID, presignS3.MediaID)
	if err != nil || completedS3.ScanStatus != ScanClean {
		t.Fatalf("Complete (S3) failed: %v, obj=%#v", err, completedS3)
	}

	// 6. Cross-storage replacement: delete old local avatar
	if err := service.Delete(ctx, ownerID, presignLocal.MediaID); err != nil {
		t.Fatalf("Delete old local avatar failed while S3 active: %v", err)
	}
	if _, exists := localStorage.Objects[localObj.ObjectKey]; exists {
		t.Fatal("expected local object to be deleted from localStorage")
	}

	// S3 object still exists and readable
	viewS3, err := service.Get(ctx, ownerID, presignS3.MediaID)
	if err != nil || viewS3.DownloadURL == "" {
		t.Fatalf("Get S3 object failed: %v", err)
	}

	// 7. Switch back from S3 to LOCAL mode
	manager.mu.Lock()
	manager.activeType = "local"
	manager.mu.Unlock()

	// S3 object must still be viewable and deletable while LOCAL is active
	viewS3AfterSwitch, err := service.Get(ctx, ownerID, presignS3.MediaID)
	if err != nil || viewS3AfterSwitch.DownloadURL == "" {
		t.Fatalf("Get S3 object while Local active failed: %v", err)
	}

	if err := service.Delete(ctx, ownerID, presignS3.MediaID); err != nil {
		t.Fatalf("Delete S3 object while Local active failed: %v", err)
	}
	if _, exists := s3Objects[s3Obj.ObjectKey]; exists {
		t.Fatal("expected S3 object to be deleted from S3 storage")
	}
}

func TestStorageSecretEncryption(t *testing.T) {
	manager, err := NewStorageManager(nil, "my-super-secret-master-key-here!", nil, objectstorage.S3Config{})
	if err != nil {
		t.Fatal(err)
	}

	rawSecret := "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	encrypted, err := manager.encryptSecret(rawSecret)
	if err != nil {
		t.Fatalf("encryptSecret failed: %v", err)
	}

	if encrypted == rawSecret || strings.Contains(encrypted, rawSecret) {
		t.Fatal("encrypted secret contains plaintext")
	}

	decrypted, err := manager.decryptSecret(encrypted)
	if err != nil {
		t.Fatalf("decryptSecret failed: %v", err)
	}

	if decrypted != rawSecret {
		t.Fatalf("expected %s, got %s", rawSecret, decrypted)
	}
}

func TestStorageSettingsMasking(t *testing.T) {
	ctx := context.Background()
	manager, err := NewStorageManager(nil, "master-key", nil, objectstorage.S3Config{})
	if err != nil {
		t.Fatal(err)
	}

	manager.s3Config = S3UpdateConfig{
		Endpoint:  "https://s3.yandexcloud.net",
		Region:    "ru-central1",
		Bucket:    "naimio-bucket",
		AccessKey: "YC1234567890",
		SecretKey: "super-secret-aws-key",
	}

	settings, err := manager.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if settings.S3.SecretKeyMasked != "********" {
		t.Fatalf("expected secret_key_masked to be '********', got %q", settings.S3.SecretKeyMasked)
	}
	if !settings.S3.SecretKeyConfigured {
		t.Fatal("expected secret_key_configured to be true")
	}
}

type fakeAuthorizer struct {
	allowed map[string]bool
}

func (f fakeAuthorizer) CanUsePurpose(_ context.Context, actor, purpose string) (bool, error) {
	return f.allowed[actor+":"+purpose], nil
}

func TestAdminUploadPurposeIsolation(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	repo := &Store{Objects: map[string]Object{}}
	localStorage := &MemoryStorage{Objects: map[string]StoredObject{}}
	authorizer := fakeAuthorizer{
		allowed: map[string]bool{
			"admin-1:BLOG_COVER":   true,
			"admin-1:BLOG_CONTENT": true,
		},
	}
	service := Service{
		Repository:        repo,
		Storage:           localStorage,
		PurposeAuthorizer: authorizer,
		Now:               func() time.Time { return now },
		AutoClean:         true,
	}

	adminCtx := auth.WithAdminSession(auth.WithActorID(context.Background(), "admin-1"), true)
	userCtx := auth.WithActorID(context.Background(), "user-1")

	purposes := []struct {
		purpose       string
		adminExpected bool
		userExpected  bool
	}{
		{PurposeAvatar, false, true},
		{PurposePortfolio, false, true},
		{PurposeService, false, true},
		{PurposeProject, false, true},
		{PurposeChat, false, true},
		{PurposeBlogCover, true, false},
		{PurposeBlogContent, true, false},
	}

	for _, p := range purposes {
		t.Run("admin_"+p.purpose, func(t *testing.T) {
			_, err := service.CreatePresign(adminCtx, "admin-1", PresignInput{
				Purpose:  p.purpose,
				Filename: "test.png",
				MIMEType: "image/png",
				Size:     1024,
			})
			if p.adminExpected && err != nil {
				t.Fatalf("expected admin to presign %s, got err: %v", p.purpose, err)
			}
			if !p.adminExpected && err == nil {
				t.Fatalf("expected admin presign for %s to be REJECTED", p.purpose)
			}
		})

		t.Run("user_"+p.purpose, func(t *testing.T) {
			_, err := service.CreatePresign(userCtx, "user-1", PresignInput{
				Purpose:  p.purpose,
				Filename: "test.png",
				MIMEType: "image/png",
				Size:     1024,
			})
			if p.userExpected && err != nil {
				t.Fatalf("expected user to presign %s, got err: %v", p.purpose, err)
			}
			if !p.userExpected && err == nil {
				t.Fatalf("expected user presign for %s to be REJECTED", p.purpose)
			}
		})
	}
}

func TestAdminCannotAccessNonStaffMediaByID(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	repo := &Store{Objects: map[string]Object{}}
	localStorage := &MemoryStorage{Objects: map[string]StoredObject{}}
	service := Service{
		Repository: repo,
		Storage:    localStorage,
		Now:        func() time.Time { return now },
		AutoClean:  true,
	}

	userACtx := auth.WithActorID(context.Background(), "user-a")
	userBCtx := auth.WithActorID(context.Background(), "user-b")
	adminCtx := auth.WithAdminSession(auth.WithActorID(context.Background(), "admin-1"), true)

	// User A creates and completes an AVATAR object
	pre, err := service.CreatePresign(userACtx, "user-a", PresignInput{
		Purpose:  PurposeAvatar,
		Filename: "avatar.png",
		MIMEType: "image/png",
		Size:     100,
	})
	if err != nil {
		t.Fatal(err)
	}
	localStorage.Objects[repo.Objects[pre.MediaID].ObjectKey] = StoredObject{SizeBytes: 100, MIMEType: "image/png"}
	_, err = service.Complete(userACtx, "user-a", pre.MediaID)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Admin attempts to complete, get, or delete User A's avatar
	if _, err := service.Complete(adminCtx, "admin-1", pre.MediaID); err == nil {
		t.Fatal("expected admin Complete of user avatar to fail")
	}
	if _, err := service.Get(adminCtx, "admin-1", pre.MediaID); err == nil {
		t.Fatal("expected admin Get of user avatar to fail")
	}
	if err := service.Delete(adminCtx, "admin-1", pre.MediaID); err == nil {
		t.Fatal("expected admin Delete of user avatar to fail")
	}

	// 2. IDOR: User B attempts to complete, get, or delete User A's avatar
	if _, err := service.Complete(userBCtx, "user-b", pre.MediaID); err == nil {
		t.Fatal("expected User B Complete of User A avatar to fail (IDOR)")
	}
	if _, err := service.Get(userBCtx, "user-b", pre.MediaID); err == nil {
		t.Fatal("expected User B Get of User A avatar to fail (IDOR)")
	}
	if err := service.Delete(userBCtx, "user-b", pre.MediaID); err == nil {
		t.Fatal("expected User B Delete of User A avatar to fail (IDOR)")
	}
}

func TestStaffMediaFullLifecycle(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	repo := &Store{Objects: map[string]Object{}}
	localStorage := &MemoryStorage{Objects: map[string]StoredObject{}}
	authorizer := fakeAuthorizer{
		allowed: map[string]bool{
			"admin-1:BLOG_COVER":   true,
			"admin-1:BLOG_CONTENT": true,
		},
	}
	service := Service{
		Repository:        repo,
		Storage:           localStorage,
		PurposeAuthorizer: authorizer,
		Now:               func() time.Time { return now },
		AutoClean:         true,
	}

	adminCtx := auth.WithAdminSession(auth.WithActorID(context.Background(), "admin-1"), true)

	for _, purpose := range []string{PurposeBlogCover, PurposeBlogContent} {
		t.Run(purpose, func(t *testing.T) {
			// 1. Presign
			pre, err := service.CreatePresign(adminCtx, "admin-1", PresignInput{
				Purpose:  purpose,
				Filename: "image.png",
				MIMEType: "image/png",
				Size:     2048,
			})
			if err != nil {
				t.Fatalf("Presign failed: %v", err)
			}

			// 2. Upload (mock)
			key := repo.Objects[pre.MediaID].ObjectKey
			localStorage.Objects[key] = StoredObject{SizeBytes: 2048, MIMEType: "image/png"}

			// 3. Complete
			comp, err := service.Complete(adminCtx, "admin-1", pre.MediaID)
			if err != nil || comp.ScanStatus != ScanClean {
				t.Fatalf("Complete failed: %v", err)
			}

			// 4. Read (Get)
			view, err := service.Get(adminCtx, "admin-1", pre.MediaID)
			if err != nil || view.DownloadURL == "" {
				t.Fatalf("Get failed: %v", err)
			}

			// 5. Delete
			if err := service.Delete(adminCtx, "admin-1", pre.MediaID); err != nil {
				t.Fatalf("Delete failed: %v", err)
			}

			if _, exists := localStorage.Objects[key]; exists {
				t.Fatal("expected file to be deleted from storage")
			}
		})
	}
}
