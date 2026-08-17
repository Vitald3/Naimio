package media

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"freelance/apps/api/internal/auth"
)

var (
	ErrUnauthorized   = errors.New("unauthenticated")
	ErrNotFound       = errors.New("media object not found")
	ErrInvalidInput   = errors.New("invalid upload input")
	ErrInvalidObject  = errors.New("uploaded object is invalid")
	ErrStorageMissing = errors.New("stored object not found")
)

const (
	PurposePortfolio   = "PORTFOLIO"
	PurposeService     = "SERVICE"
	PurposeProject     = "PROJECT"
	PurposeChat        = "CHAT"
	PurposeAvatar      = "AVATAR"
	PurposeBlogCover   = "BLOG_COVER"
	PurposeBlogContent = "BLOG_CONTENT"
	ScanPending        = "PENDING"
	ScanClean          = "CLEAN"
	ScanInfected       = "INFECTED"
	ScanFailed         = "FAILED"
)

type Object struct {
	ID               string     `json:"id"`
	OwnerID          string     `json:"-"`
	Purpose          string     `json:"purpose"`
	StorageProvider  string     `json:"storage_provider"`
	StorageBackendID string     `json:"storage_backend_id,omitempty"`
	ObjectKey        string     `json:"-"`
	Bucket           string     `json:"-"`
	OriginalFilename string     `json:"original_filename"`
	MIMEType         string     `json:"mime_type"`
	SizeBytes        int64      `json:"size_bytes"`
	ScanStatus       string     `json:"scan_status"`
	UploadedAt       *time.Time `json:"uploaded_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	DeletedAt        *time.Time `json:"-"`
}

type PresignInput struct {
	Purpose  string `json:"purpose"`
	Filename string `json:"filename"`
	MIMEType string `json:"mime_type"`
	Size     int64  `json:"size_bytes"`
}

type Presign struct {
	MediaID   string            `json:"media_id"`
	UploadURL string            `json:"upload_url"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt time.Time         `json:"expires_at"`
}

type View struct {
	Object
	DownloadURL string     `json:"download_url,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

type StoredObject struct {
	SizeBytes    int64
	MIMEType     string
	DetectedMIME string
	Data         []byte
}

type Storage interface {
	PresignPut(context.Context, string, string, int64, time.Duration) (string, map[string]string, time.Time, error)
	Inspect(context.Context, string) (StoredObject, error)
	PresignGet(context.Context, string, time.Duration) (string, time.Time, error)
	Delete(context.Context, string) error
	Open(context.Context, string) (io.ReadCloser, int64, string, error)
}

type StorageProviderResolver interface {
	ActiveStorage(context.Context) (Storage, string, string, error)
	ResolveStorage(string) (Storage, error)
}

type StorageBackendResolver interface {
	ActiveStorageBackend(context.Context) (Storage, string, string, string, error)
	ResolveStorageBackend(ctx context.Context, backendID, provider, bucket string) (Storage, error)
}

type Repository interface {
	Create(context.Context, Object) (Object, error)
	GetOwned(context.Context, string, string) (Object, error)
	GetPublic(context.Context, string) (Object, error)
	MarkUploaded(context.Context, string, string, time.Time) (Object, error)
	MarkScanResult(context.Context, string, string) error
	Delete(context.Context, string, string, time.Time) error
}

type Service struct {
	Repository        Repository
	Storage           Storage
	Resolver          StorageProviderResolver
	PurposeAuthorizer interface {
		CanUsePurpose(context.Context, string, string) (bool, error)
	}
	Bucket    string
	Now       func() time.Time
	AutoClean bool
}

func (s Service) storageFor(ctx context.Context, provider, bucket, backendID string) Storage {
	if s.Resolver != nil {
		if backendResolver, ok := s.Resolver.(StorageBackendResolver); ok && backendID != "" {
			if st, err := backendResolver.ResolveStorageBackend(ctx, backendID, provider, bucket); err == nil && st != nil {
				return st
			}
		}
		if st, err := s.Resolver.ResolveStorage(provider); err == nil && st != nil {
			if bucketAware, ok := st.(interface{ WithBucket(string) Storage }); ok && bucket != "" {
				return bucketAware.WithBucket(bucket)
			}
			return st
		}
	}
	return s.Storage
}

func (s Service) CreatePresign(ctx context.Context, actorID string, input PresignInput) (Presign, error) {
	if actorID == "" {
		return Presign{}, ErrUnauthorized
	}
	if auth.IsAdminSession(ctx) {
		if input.Purpose != PurposeBlogCover && input.Purpose != PurposeBlogContent {
			return Presign{}, ErrUnauthorized
		}
	}
	input = normalizeInput(input)
	if err := ValidatePresign(input); err != nil {
		return Presign{}, err
	}
	if input.Purpose == PurposeBlogCover || input.Purpose == PurposeBlogContent {
		if s.PurposeAuthorizer == nil {
			return Presign{}, ErrInvalidInput
		}
		allowed, err := s.PurposeAuthorizer.CanUsePurpose(ctx, actorID, input.Purpose)
		if err != nil {
			return Presign{}, err
		}
		if !allowed {
			return Presign{}, ErrInvalidInput
		}
	}
	id, err := newUUIDv7()
	if err != nil {
		return Presign{}, err
	}
	extension := canonicalExtension(input.MIMEType)
	key := strings.ToLower(input.Purpose) + "/" + actorID + "/" + id + extension

	st := s.Storage
	provider := "local"
	bucket := s.Bucket
	backendID := ""
	if s.Resolver != nil {
		if backendResolver, ok := s.Resolver.(StorageBackendResolver); ok {
			activeSt, activeProv, activeBucket, activeBackendID, err := backendResolver.ActiveStorageBackend(ctx)
			if err != nil {
				return Presign{}, err
			}
			st = activeSt
			provider = activeProv
			bucket = activeBucket
			backendID = activeBackendID
		} else {
			activeSt, activeProv, activeBucket, err := s.Resolver.ActiveStorage(ctx)
			if err != nil {
				return Presign{}, err
			}
			st = activeSt
			provider = activeProv
			bucket = activeBucket
		}
	}

	url, headers, expiresAt, err := st.PresignPut(ctx, key, input.MIMEType, input.Size, 15*time.Minute)
	if err != nil {
		return Presign{}, err
	}
	now := s.now()
	_, err = s.Repository.Create(ctx, Object{ID: id, OwnerID: actorID, Purpose: input.Purpose, StorageProvider: provider, StorageBackendID: backendID, ObjectKey: key,
		Bucket: bucket, OriginalFilename: input.Filename, MIMEType: input.MIMEType, SizeBytes: input.Size,
		ScanStatus: ScanPending, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		return Presign{}, err
	}
	return Presign{MediaID: id, UploadURL: url, Headers: headers, ExpiresAt: expiresAt.UTC()}, nil
}

func (s Service) Complete(ctx context.Context, actorID, mediaID string) (Object, error) {
	if actorID == "" {
		return Object{}, ErrUnauthorized
	}
	if !validUUID(mediaID) {
		return Object{}, ErrNotFound
	}
	object, err := s.Repository.GetOwned(ctx, actorID, mediaID)
	if err != nil {
		return Object{}, err
	}
	if auth.IsAdminSession(ctx) && object.Purpose != PurposeBlogCover && object.Purpose != PurposeBlogContent {
		return Object{}, ErrUnauthorized
	}
	if object.UploadedAt != nil {
		if object.ScanStatus == ScanFailed || object.ScanStatus == ScanInfected {
			return Object{}, ErrInvalidObject
		}
		if s.AutoClean && object.ScanStatus == ScanPending {
			if err := s.Repository.MarkScanResult(ctx, mediaID, ScanClean); err != nil {
				return Object{}, err
			}
			return s.Repository.GetOwned(ctx, actorID, mediaID)
		}
		return object, nil
	}
	st := s.storageFor(ctx, object.StorageProvider, object.Bucket, object.StorageBackendID)
	stored, err := st.Inspect(ctx, object.ObjectKey)
	if err != nil {
		if errors.Is(err, ErrStorageMissing) {
			return Object{}, ErrInvalidObject
		}
		return Object{}, err
	}
	if stored.SizeBytes != object.SizeBytes || normalizeMIME(stored.MIMEType) != object.MIMEType ||
		(stored.DetectedMIME != "" && normalizeMIME(stored.DetectedMIME) != object.MIMEType) {
		_, _ = s.Repository.MarkUploaded(ctx, actorID, mediaID, s.now())
		_ = s.Repository.MarkScanResult(ctx, object.ID, ScanFailed)
		return Object{}, ErrInvalidObject
	}
	uploaded, err := s.Repository.MarkUploaded(ctx, actorID, mediaID, s.now())
	if err != nil {
		return Object{}, err
	}
	if s.AutoClean {
		if err := s.Repository.MarkScanResult(ctx, mediaID, ScanClean); err != nil {
			return Object{}, err
		}
		return s.Repository.GetOwned(ctx, actorID, mediaID)
	}
	return uploaded, nil
}

func (s Service) Get(ctx context.Context, actorID, mediaID string) (View, error) {
	if actorID == "" {
		return View{}, ErrUnauthorized
	}
	if !validUUID(mediaID) {
		return View{}, ErrNotFound
	}
	object, err := s.Repository.GetOwned(ctx, actorID, mediaID)
	if err != nil {
		return View{}, err
	}
	if auth.IsAdminSession(ctx) && object.Purpose != PurposeBlogCover && object.Purpose != PurposeBlogContent {
		return View{}, ErrUnauthorized
	}
	view := View{Object: object}
	if object.UploadedAt != nil && object.ScanStatus == ScanClean {
		st := s.storageFor(ctx, object.StorageProvider, object.Bucket, object.StorageBackendID)
		url, expiresAt, err := st.PresignGet(ctx, object.ObjectKey, 5*time.Minute)
		if err != nil {
			return View{}, err
		}
		view.DownloadURL, view.ExpiresAt = url, &expiresAt
	}
	return view, nil
}

func (s Service) Delete(ctx context.Context, actorID, mediaID string) error {
	if actorID == "" {
		return ErrUnauthorized
	}
	if !validUUID(mediaID) {
		return ErrNotFound
	}
	object, err := s.Repository.GetOwned(ctx, actorID, mediaID)
	if err != nil {
		return err
	}
	if auth.IsAdminSession(ctx) && object.Purpose != PurposeBlogCover && object.Purpose != PurposeBlogContent {
		return ErrUnauthorized
	}
	st := s.storageFor(ctx, object.StorageProvider, object.Bucket, object.StorageBackendID)
	if err := st.Delete(ctx, object.ObjectKey); err != nil && !errors.Is(err, ErrStorageMissing) {
		return err
	}
	return s.Repository.Delete(ctx, actorID, mediaID, s.now())
}

func ValidatePresign(input PresignInput) error {
	if input.Purpose != PurposePortfolio && input.Purpose != PurposeService && input.Purpose != PurposeProject && input.Purpose != PurposeChat && input.Purpose != PurposeAvatar && input.Purpose != PurposeBlogCover && input.Purpose != PurposeBlogContent {
		return invalid("unsupported upload purpose")
	}
	if len([]rune(input.Filename)) < 1 || len([]rune(input.Filename)) > 255 || strings.ContainsAny(input.Filename, `/\\`) {
		return invalid("invalid filename")
	}
	for _, character := range input.Filename {
		if unicode.IsControl(character) {
			return invalid("invalid filename")
		}
	}
	maximum, ok := allowedMIME[input.MIMEType]
	if input.Purpose == PurposeAvatar && (!strings.HasPrefix(input.MIMEType, "image/") || input.Size > 5<<20) {
		return invalid("avatar must be a JPG, PNG or WebP image up to 5 MB")
	}
	if (input.Purpose == PurposeBlogCover || input.Purpose == PurposeBlogContent) && (!strings.HasPrefix(input.MIMEType, "image/") || input.Size > 10<<20) {
		return invalid("blog media must be a JPG, PNG or WebP image up to 10 MB")
	}
	if !ok || input.Size < 1 || input.Size > maximum {
		return invalid("unsupported file type or size")
	}
	if !slices.Contains(allowedExtensions(input.MIMEType), strings.ToLower(filepath.Ext(input.Filename))) {
		return invalid("filename extension does not match mime type")
	}
	return nil
}

var allowedMIME = map[string]int64{
	"image/jpeg":         10 << 20,
	"image/png":          10 << 20,
	"image/webp":         10 << 20,
	"application/pdf":    20 << 20,
	"application/msword": 20 << 20,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": 20 << 20,
	"text/plain":  5 << 20,
	"audio/webm":  20 << 20,
	"audio/ogg":   20 << 20,
	"audio/mpeg":  20 << 20,
	"audio/mp4":   20 << 20,
	"audio/x-m4a": 20 << 20,
}

func normalizeInput(input PresignInput) PresignInput {
	input.Purpose = strings.ToUpper(strings.TrimSpace(input.Purpose))
	input.Filename = strings.TrimSpace(input.Filename)
	input.MIMEType = normalizeMIME(input.MIMEType)
	return input
}

func normalizeMIME(value string) string {
	if before, _, found := strings.Cut(strings.ToLower(strings.TrimSpace(value)), ";"); found {
		return strings.TrimSpace(before)
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func canonicalExtension(mime string) string {
	switch mime {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "application/pdf":
		return ".pdf"
	case "application/msword":
		return ".doc"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return ".docx"
	case "text/plain":
		return ".txt"
	case "audio/webm":
		return ".webm"
	case "audio/ogg":
		return ".ogg"
	case "audio/mpeg":
		return ".mp3"
	case "audio/mp4", "audio/x-m4a":
		return ".m4a"
	default:
		return ""
	}
}

func allowedExtensions(mime string) []string {
	switch mime {
	case "image/jpeg":
		return []string{".jpg", ".jpeg"}
	case "audio/mp4", "audio/x-m4a":
		return []string{".m4a", ".mp4"}
	default:
		return []string{canonicalExtension(mime)}
	}
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func invalid(message string) error { return fmt.Errorf("%w: %s", ErrInvalidInput, message) }

func newUUIDv7() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	milliseconds := uint64(time.Now().UTC().UnixMilli())
	value[0], value[1], value[2] = byte(milliseconds>>40), byte(milliseconds>>32), byte(milliseconds>>24)
	value[3], value[4], value[5] = byte(milliseconds>>16), byte(milliseconds>>8), byte(milliseconds)
	value[6], value[8] = (value[6]&0x0f)|0x70, (value[8]&0x3f)|0x80
	var encoded [36]byte
	hex.Encode(encoded[0:8], value[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], value[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], value[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], value[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], value[10:16])
	return string(encoded[:]), nil
}
