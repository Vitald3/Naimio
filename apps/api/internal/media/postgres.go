package media

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type PostgresRepository struct{ DB *sql.DB }

func (r PostgresRepository) Create(ctx context.Context, object Object) (Object, error) {
	provider := object.StorageProvider
	if provider == "" {
		provider = "local"
	}
	var backendID sql.NullString
	if object.StorageBackendID != "" {
		backendID.String = object.StorageBackendID
		backendID.Valid = true
	}
	return scanObject(r.DB.QueryRowContext(ctx, `
INSERT INTO media_objects
  (id, owner_user_id, object_key, bucket, storage_provider, storage_backend_id, original_filename, mime_type, size_bytes, purpose, scan_status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, NULLIF($6, '')::uuid, $7, $8, $9, $10, 'PENDING', $11, $11)
RETURNING id::text, owner_user_id::text, purpose, COALESCE(storage_provider, 'local'), COALESCE(storage_backend_id::text, ''), object_key, bucket, COALESCE(original_filename, ''),
  mime_type, size_bytes, scan_status, uploaded_at, created_at, updated_at, deleted_at`, object.ID, object.OwnerID,
		object.ObjectKey, object.Bucket, provider, backendID.String, object.OriginalFilename, object.MIMEType, object.SizeBytes, object.Purpose, object.CreatedAt))
}

func (r PostgresRepository) GetOwned(ctx context.Context, actorID, mediaID string) (Object, error) {
	return scanObject(r.DB.QueryRowContext(ctx, `
SELECT id::text, owner_user_id::text, purpose, COALESCE(storage_provider, 'local'), COALESCE(storage_backend_id::text, ''), object_key, bucket, COALESCE(original_filename, ''),
  mime_type, size_bytes, scan_status, uploaded_at, created_at, updated_at, deleted_at
FROM media_objects WHERE id = $1 AND owner_user_id = $2 AND deleted_at IS NULL`, mediaID, actorID))
}

func (r PostgresRepository) GetPublic(ctx context.Context, mediaID string) (Object, error) {
	return scanObject(r.DB.QueryRowContext(ctx, `
SELECT id::text, owner_user_id::text, purpose, COALESCE(storage_provider, 'local'), COALESCE(storage_backend_id::text, ''), object_key, bucket, COALESCE(original_filename, ''),
  mime_type, size_bytes, scan_status, uploaded_at, created_at, updated_at, deleted_at
FROM media_objects
WHERE id = $1 AND deleted_at IS NULL AND uploaded_at IS NOT NULL AND scan_status = 'CLEAN'`, mediaID))
}

func (r PostgresRepository) MarkUploaded(ctx context.Context, actorID, mediaID string, at time.Time) (Object, error) {
	return scanObject(r.DB.QueryRowContext(ctx, `
UPDATE media_objects SET uploaded_at = COALESCE(uploaded_at, $3), updated_at = $3
WHERE id = $1 AND owner_user_id = $2 AND deleted_at IS NULL
RETURNING id::text, owner_user_id::text, purpose, COALESCE(storage_provider, 'local'), COALESCE(storage_backend_id::text, ''), object_key, bucket, COALESCE(original_filename, ''),
  mime_type, size_bytes, scan_status, uploaded_at, created_at, updated_at, deleted_at`, mediaID, actorID, at))
}

func (r PostgresRepository) MarkScanResult(ctx context.Context, mediaID, status string) error {
	if status != ScanClean && status != ScanInfected && status != ScanFailed {
		return invalid("invalid scan status")
	}
	result, err := r.DB.ExecContext(ctx, `UPDATE media_objects SET scan_status = $2, updated_at = now()
WHERE id = $1 AND uploaded_at IS NOT NULL AND deleted_at IS NULL`, mediaID, status)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrNotFound
	}
	return nil
}

func (r PostgresRepository) Delete(ctx context.Context, actorID, mediaID string, at time.Time) error {
	result, err := r.DB.ExecContext(ctx, `UPDATE media_objects SET deleted_at = $3, updated_at = $3
WHERE id = $1 AND owner_user_id = $2 AND deleted_at IS NULL`, mediaID, actorID, at)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrNotFound
	}
	return nil
}

type rowScanner interface{ Scan(...any) error }

func scanObject(row rowScanner) (Object, error) {
	var object Object
	var backendID sql.NullString
	err := row.Scan(&object.ID, &object.OwnerID, &object.Purpose, &object.StorageProvider, &backendID, &object.ObjectKey, &object.Bucket,
		&object.OriginalFilename, &object.MIMEType, &object.SizeBytes, &object.ScanStatus, &object.UploadedAt,
		&object.CreatedAt, &object.UpdatedAt, &object.DeletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Object{}, ErrNotFound
	}
	if backendID.Valid {
		object.StorageBackendID = backendID.String
	}
	return object, err
}
