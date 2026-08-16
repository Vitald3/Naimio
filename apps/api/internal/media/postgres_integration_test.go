//go:build integration

package media

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresMediaRepository(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is required for integration tests")
	}
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()
	owner := "12121212-1212-4212-8212-121212121212"
	other := "34343434-3434-4434-8434-343434343434"
	objectID := "56565656-5656-4656-8656-565656565656"
	_, err = database.ExecContext(ctx, `INSERT INTO users (id, email, email_normalized, password_hash, display_name) VALUES
($1, 'media-owner@example.invalid', 'media-owner@example.invalid', 'test-only', 'Media Owner'),
($2, 'media-other@example.invalid', 'media-other@example.invalid', 'test-only', 'Media Other')`, owner, other)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.ExecContext(context.Background(), "DELETE FROM media_objects WHERE owner_user_id IN ($1, $2)", owner, other)
		_, _ = database.ExecContext(context.Background(), "DELETE FROM users WHERE id IN ($1, $2)", owner, other)
	})

	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	repository := PostgresRepository{DB: database}
	created, err := repository.Create(ctx, Object{ID: objectID, OwnerID: owner, Purpose: PurposePortfolio,
		ObjectKey: "portfolio/owner/object.png", Bucket: "test", OriginalFilename: "screen.png",
		MIMEType: "image/png", SizeBytes: 128, ScanStatus: ScanPending, CreatedAt: now, UpdatedAt: now})
	if err != nil || created.Purpose != PurposePortfolio || created.UploadedAt != nil {
		t.Fatalf("created = %#v, error = %v", created, err)
	}
	if _, err := repository.GetOwned(ctx, other, objectID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user get error = %v", err)
	}
	if err := repository.MarkScanResult(ctx, objectID, ScanClean); !errors.Is(err, ErrNotFound) {
		t.Fatalf("pending scan error = %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		completed, completeErr := repository.MarkUploaded(ctx, owner, objectID, now.Add(time.Minute))
		if completeErr != nil || completed.UploadedAt == nil {
			t.Fatalf("attempt %d object = %#v, error = %v", attempt, completed, completeErr)
		}
	}
	if err := repository.MarkScanResult(ctx, objectID, ScanClean); err != nil {
		t.Fatal(err)
	}
	if err := repository.Delete(ctx, other, objectID, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user delete error = %v", err)
	}
	if err := repository.Delete(ctx, owner, objectID, now); err != nil {
		t.Fatal(err)
	}
}

func TestOnlyOneStorageBackendCanBeActive(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is required for integration tests")
	}
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()

	// Ensure at least one active row
	_, _ = database.ExecContext(ctx, `UPDATE storage_backends SET is_active = false`)
	_, _ = database.ExecContext(ctx, `
		INSERT INTO storage_backends (id, provider, bucket, is_active)
		VALUES ('00000000-0000-0000-0000-000000000001', 'local', 'local-media', true)
		ON CONFLICT (id) DO UPDATE SET is_active = true`)

	// Attempt to insert a second active backend - should fail due to unique index
	_, err = database.ExecContext(ctx, `
		INSERT INTO storage_backends (provider, endpoint, bucket, is_active)
		VALUES ('s3', 'http://minio-conflict:9000', 'files', true)`)
	if err == nil {
		t.Fatal("expected insert of second active backend to fail with unique violation")
	}

	// Verify only 1 active backend exists in DB
	var activeCount int
	err = database.QueryRowContext(ctx, `SELECT count(*) FROM storage_backends WHERE is_active = true`).Scan(&activeCount)
	if err != nil || activeCount != 1 {
		t.Fatalf("expected exactly 1 active backend, got %d, err: %v", activeCount, err)
	}
}

func TestConcurrentStorageBackendActivation(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is required for integration tests")
	}
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()

	manager, err := NewStorageManager(database, "test-master-key", &DiskStorage{RootDir: "/tmp/media"}, objectstorage.S3Config{})
	if err != nil {
		t.Fatal(err)
	}

	// Run concurrent activations between Local and S3
	concurrency := 10
	errChan := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			var updateErr error
			if idx%2 == 0 {
				_, updateErr = manager.UpdateSettings(ctx, "00000000-0000-0000-0000-000000000001", StorageSettingsUpdate{
					Provider: "local",
				})
			} else {
				_, updateErr = manager.UpdateSettings(ctx, "00000000-0000-0000-0000-000000000001", StorageSettingsUpdate{
					Provider: "s3",
					S3: S3UpdateConfig{
						Endpoint:  "http://minio-a:9000",
						Bucket:    "files",
						AccessKey: "endpoint-a-user",
						SecretKey: "endpoint-a-secret",
					},
				})
			}
			errChan <- updateErr
		}(i)
	}

	for i := 0; i < concurrency; i++ {
		<-errChan
	}

	// Verify invariant: exactly 1 active backend exists in DB
	var activeCount int
	err = database.QueryRowContext(ctx, `SELECT count(*) FROM storage_backends WHERE is_active = true`).Scan(&activeCount)
	if err != nil || activeCount != 1 {
		t.Fatalf("expected exactly 1 active backend after concurrent updates, got %d, err: %v", activeCount, err)
	}

	// Restart manager and verify unambiguous resolution
	newManager, err := NewStorageManager(database, "test-master-key", &DiskStorage{RootDir: "/tmp/media"}, objectstorage.S3Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := newManager.LoadFromDB(ctx); err != nil {
		t.Fatalf("LoadFromDB failed: %v", err)
	}

	_, activeProv, _, err := newManager.ActiveStorage(ctx)
	if err != nil || (activeProv != "local" && activeProv != "s3") {
		t.Fatalf("unambiguous active storage resolution failed: prov=%s, err=%v", activeProv, err)
	}
}
