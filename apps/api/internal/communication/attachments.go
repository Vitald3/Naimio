package communication

import (
	"context"
	"database/sql"
	"errors"
	"freelance/apps/api/internal/media"
	"time"
)

type AttachmentViewer interface {
	View(context.Context, string, string, string) (media.View, error)
}
type PostgresAttachmentViewer struct {
	DB      *sql.DB
	Storage media.Storage
}

func (v PostgresAttachmentViewer) View(ctx context.Context, actor, messageID, mediaID string) (media.View, error) {
	var object media.Object
	err := v.DB.QueryRowContext(ctx, `SELECT mo.id::text,mo.owner_user_id::text,mo.purpose,COALESCE(mo.storage_provider, 'local'),mo.object_key,mo.bucket,mo.original_filename,mo.mime_type,mo.size_bytes,mo.scan_status,mo.uploaded_at,mo.created_at,mo.updated_at FROM message_media mm JOIN messages m ON m.id=mm.message_id JOIN conversation_members cm ON cm.conversation_id=m.conversation_id AND cm.user_id=$1 JOIN media_objects mo ON mo.id=mm.media_object_id WHERE mm.message_id=$2 AND mm.media_object_id=$3 AND mo.purpose='CHAT' AND mo.scan_status='CLEAN' AND mo.deleted_at IS NULL`, actor, messageID, mediaID).Scan(&object.ID, &object.OwnerID, &object.Purpose, &object.StorageProvider, &object.ObjectKey, &object.Bucket, &object.OriginalFilename, &object.MIMEType, &object.SizeBytes, &object.ScanStatus, &object.UploadedAt, &object.CreatedAt, &object.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return media.View{}, ErrNotFound
	}
	if err != nil {
		return media.View{}, err
	}
	url, expires, err := v.Storage.PresignGet(ctx, object.ObjectKey, 5*time.Minute)
	if err != nil {
		return media.View{}, err
	}
	return media.View{Object: object, DownloadURL: url, ExpiresAt: &expires}, nil
}
