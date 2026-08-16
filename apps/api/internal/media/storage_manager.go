package media

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"freelance/apps/api/internal/platform/objectstorage"
)

type S3PublicConfig struct {
	Endpoint            string `json:"endpoint"`
	Region              string `json:"region"`
	Bucket              string `json:"bucket"`
	AccessKey           string `json:"access_key"`
	SecretKeyConfigured bool   `json:"secret_key_configured"`
	SecretKeyMasked     string `json:"secret_key_masked"`
	UseSSL              bool   `json:"use_ssl"`
	PathStyle           bool   `json:"path_style"`
	PublicURL           string `json:"public_url"`
}

type S3UpdateConfig struct {
	Endpoint  string `json:"endpoint"`
	Region    string `json:"region"`
	Bucket    string `json:"bucket"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key,omitempty"`
	UseSSL    bool   `json:"use_ssl"`
	PathStyle bool   `json:"path_style"`
	PublicURL string `json:"public_url"`
}

type StorageSettings struct {
	Provider string         `json:"provider"`
	S3       S3PublicConfig `json:"s3"`
}

type StorageSettingsUpdate struct {
	Provider string         `json:"provider"`
	S3       S3UpdateConfig `json:"s3"`
	Reason   string         `json:"reason,omitempty"`
}

type BackendRecord struct {
	ID                 string
	Provider           string
	Endpoint           string
	Region             string
	Bucket             string
	AccessKeyID        string
	SecretKeyEncrypted string
	UseSSL             bool
	ForcePathStyle     bool
	PublicURL          string
	IsActive           bool
}

type StorageManager struct {
	db            *sql.DB
	encryptionKey [32]byte
	mu            sync.RWMutex

	localStorage Storage
	localBucket  string

	activeBackendID string
	activeType      string // "local" or "s3"
	activeBucket    string

	s3Storage Storage
	s3Config  S3UpdateConfig

	backends       map[string]Storage
	backendRecords map[string]BackendRecord
}

type s3Adapter struct {
	client *objectstorage.S3
}

func newS3Adapter(client *objectstorage.S3) Storage {
	if client == nil {
		return nil
	}
	return &s3Adapter{client: client}
}

func (a *s3Adapter) PresignPut(ctx context.Context, key, mime string, size int64, ttl time.Duration) (string, map[string]string, time.Time, error) {
	return a.client.PresignPut(ctx, key, mime, size, ttl)
}

func (a *s3Adapter) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, time.Time, error) {
	return a.client.PresignGet(ctx, key, ttl)
}

func (a *s3Adapter) Inspect(ctx context.Context, key string) (StoredObject, error) {
	obj, err := a.client.Inspect(ctx, key)
	if err != nil {
		if errors.Is(err, objectstorage.ErrStorageMissing) {
			return StoredObject{}, ErrStorageMissing
		}
		return StoredObject{}, err
	}
	return StoredObject{
		SizeBytes:    obj.SizeBytes,
		MIMEType:     obj.MIMEType,
		DetectedMIME: obj.DetectedMIME,
		Data:         obj.Data,
	}, nil
}

func (a *s3Adapter) Delete(ctx context.Context, key string) error {
	err := a.client.Delete(ctx, key)
	if errors.Is(err, objectstorage.ErrStorageMissing) {
		return ErrStorageMissing
	}
	return err
}

func (a *s3Adapter) WithBucket(bucket string) Storage {
	if a.client == nil || bucket == "" || bucket == a.client.Bucket() {
		return a
	}
	return &s3Adapter{client: a.client.WithBucket(bucket)}
}

func NewStorageManager(db *sql.DB, masterKey string, localStorage Storage, envS3Config objectstorage.S3Config) (*StorageManager, error) {
	if localStorage == nil {
		localStorage = &DiskStorage{RootDir: "/var/lib/naimio-media", BaseURL: "/api/v1/dev-storage"}
	}
	if masterKey == "" {
		masterKey = "naimio-default-storage-key-change-in-prod"
	}
	key := sha256.Sum256([]byte(masterKey))

	m := &StorageManager{
		db:             db,
		encryptionKey:  key,
		localStorage:   localStorage,
		localBucket:    "local-media",
		activeType:     "local",
		backends:       make(map[string]Storage),
		backendRecords: make(map[string]BackendRecord),
	}

	// Initialize fallback S3 from env if available
	if envS3Config.Endpoint != "" && envS3Config.Bucket != "" && envS3Config.AccessKey != "" && envS3Config.SecretKey != "" {
		m.s3Config = S3UpdateConfig{
			Endpoint:  envS3Config.Endpoint,
			Region:    envS3Config.Region,
			Bucket:    envS3Config.Bucket,
			AccessKey: envS3Config.AccessKey,
			SecretKey: envS3Config.SecretKey,
			UseSSL:    envS3Config.UseSSL || strings.HasPrefix(envS3Config.Endpoint, "https://"),
			PathStyle: envS3Config.PathStyle,
			PublicURL: envS3Config.PublicURL,
		}
		s3Instance, err := objectstorage.NewS3(envS3Config)
		if err == nil {
			m.s3Storage = newS3Adapter(s3Instance)
		}
	}

	return m, nil
}

func (m *StorageManager) LoadFromDB(ctx context.Context) error {
	if m.db == nil {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. Try loading from storage_backends table
	rows, err := m.db.QueryContext(ctx, `
		SELECT id::text, provider, endpoint, region, bucket, access_key_id, secret_key_encrypted, use_ssl, force_path_style, public_url, is_active
		FROM storage_backends
		ORDER BY is_active DESC, updated_at DESC`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var rec BackendRecord
			if scanErr := rows.Scan(&rec.ID, &rec.Provider, &rec.Endpoint, &rec.Region, &rec.Bucket, &rec.AccessKeyID, &rec.SecretKeyEncrypted, &rec.UseSSL, &rec.ForcePathStyle, &rec.PublicURL, &rec.IsActive); scanErr != nil {
				continue
			}
			m.backendRecords[rec.ID] = rec
			if rec.Provider == "local" {
				m.backends[rec.ID] = m.localStorage
				if rec.IsActive {
					m.activeBackendID = rec.ID
					m.activeType = "local"
					m.activeBucket = m.localBucket
				}
			} else if rec.Provider == "s3" && rec.Endpoint != "" && rec.AccessKeyID != "" && rec.SecretKeyEncrypted != "" {
				decryptedSecret, decErr := m.decryptSecret(rec.SecretKeyEncrypted)
				if decErr == nil && decryptedSecret != "" {
					s3Instance, s3Err := objectstorage.NewS3(objectstorage.S3Config{
						Endpoint:          rec.Endpoint,
						Region:            rec.Region,
						Bucket:            rec.Bucket,
						AccessKey:         rec.AccessKeyID,
						SecretKey:         decryptedSecret,
						UseSSL:            rec.UseSSL,
						PathStyle:         rec.ForcePathStyle,
						PublicURL:         rec.PublicURL,
						AllowInsecureHTTP: !rec.UseSSL,
					})
					if s3Err == nil {
						adapter := newS3Adapter(s3Instance)
						m.backends[rec.ID] = adapter
						if rec.IsActive {
							m.activeBackendID = rec.ID
							m.activeType = "s3"
							m.activeBucket = rec.Bucket
							m.s3Storage = adapter
							m.s3Config = S3UpdateConfig{
								Endpoint:  rec.Endpoint,
								Region:    rec.Region,
								Bucket:    rec.Bucket,
								AccessKey: rec.AccessKeyID,
								SecretKey: decryptedSecret,
								UseSSL:    rec.UseSSL,
								PathStyle: rec.ForcePathStyle,
								PublicURL: rec.PublicURL,
							}
						}
					}
				}
			}
		}
		if len(m.backendRecords) > 0 {
			return nil
		}
	}

	// 2. Legacy fallback to feature_flags and storage_credentials
	var enabled bool
	var configRaw []byte
	err = m.db.QueryRowContext(ctx, `SELECT enabled, config FROM feature_flags WHERE key = 'file_storage'`).Scan(&enabled, &configRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load storage feature flag: %w", err)
	}

	type flagPayload struct {
		Provider string         `json:"provider"`
		S3       S3UpdateConfig `json:"s3"`
	}
	var payload flagPayload
	if err := json.Unmarshal(configRaw, &payload); err == nil {
		if payload.Provider == "s3" || payload.Provider == "local" {
			m.activeType = payload.Provider
		}
		if payload.S3.Endpoint != "" {
			m.s3Config.Endpoint = payload.S3.Endpoint
			m.s3Config.Region = payload.S3.Region
			m.s3Config.Bucket = payload.S3.Bucket
			m.s3Config.AccessKey = payload.S3.AccessKey
			m.s3Config.UseSSL = payload.S3.UseSSL
			m.s3Config.PathStyle = payload.S3.PathStyle
			m.s3Config.PublicURL = payload.S3.PublicURL
		}
	}

	var encryptedSecret string
	err = m.db.QueryRowContext(ctx, `SELECT encrypted_config FROM storage_credentials WHERE provider = 's3'`).Scan(&encryptedSecret)
	if err == nil && encryptedSecret != "" {
		decrypted, decErr := m.decryptSecret(encryptedSecret)
		if decErr == nil && decrypted != "" {
			m.s3Config.SecretKey = decrypted
		}
	}

	if m.s3Config.Endpoint != "" && m.s3Config.Bucket != "" && m.s3Config.AccessKey != "" && m.s3Config.SecretKey != "" {
		s3Instance, err := objectstorage.NewS3(objectstorage.S3Config{
			Endpoint:          m.s3Config.Endpoint,
			Region:            m.s3Config.Region,
			Bucket:            m.s3Config.Bucket,
			AccessKey:         m.s3Config.AccessKey,
			SecretKey:         m.s3Config.SecretKey,
			UseSSL:            m.s3Config.UseSSL,
			PathStyle:         m.s3Config.PathStyle,
			PublicURL:         m.s3Config.PublicURL,
			AllowInsecureHTTP: !m.s3Config.UseSSL,
		})
		if err == nil {
			m.s3Storage = newS3Adapter(s3Instance)
		} else {
			log.Printf("storage manager s3 init warning: %v", err)
		}
	}

	return nil
}

func (m *StorageManager) ActiveStorage(_ context.Context) (Storage, string, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.activeType == "s3" && m.s3Storage != nil {
		return m.s3Storage, "s3", m.s3Config.Bucket, nil
	}
	return m.localStorage, "local", m.localBucket, nil
}

func (m *StorageManager) ActiveStorageBackend(_ context.Context) (Storage, string, string, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.activeType == "s3" && m.s3Storage != nil {
		return m.s3Storage, "s3", m.s3Config.Bucket, m.activeBackendID, nil
	}
	return m.localStorage, "local", m.localBucket, m.activeBackendID, nil
}

func (m *StorageManager) ResolveStorage(provider string) (Storage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "s3":
		if m.s3Storage != nil {
			return m.s3Storage, nil
		}
		return nil, errors.New("S3 storage is requested but not configured")
	case "local", "":
		return m.localStorage, nil
	default:
		return m.localStorage, nil
	}
}

func (m *StorageManager) ResolveStorageBackend(ctx context.Context, backendID, provider, bucket string) (Storage, error) {
	backendID = strings.TrimSpace(backendID)
	if backendID == "" {
		st, err := m.ResolveStorage(provider)
		if err != nil {
			return nil, err
		}
		if bucketAware, ok := st.(interface{ WithBucket(string) Storage }); ok && bucket != "" {
			return bucketAware.WithBucket(bucket), nil
		}
		return st, nil
	}

	m.mu.RLock()
	cached, exists := m.backends[backendID]
	m.mu.RUnlock()
	if exists && cached != nil {
		if bucketAware, ok := cached.(interface{ WithBucket(string) Storage }); ok && bucket != "" {
			return bucketAware.WithBucket(bucket), nil
		}
		return cached, nil
	}

	// Not in memory cache, query DB
	if m.db != nil {
		var rec BackendRecord
		err := m.db.QueryRowContext(ctx, `
			SELECT id::text, provider, endpoint, region, bucket, access_key_id, secret_key_encrypted, use_ssl, force_path_style, public_url, is_active
			FROM storage_backends WHERE id = $1`, backendID).Scan(
			&rec.ID, &rec.Provider, &rec.Endpoint, &rec.Region, &rec.Bucket, &rec.AccessKeyID, &rec.SecretKeyEncrypted, &rec.UseSSL, &rec.ForcePathStyle, &rec.PublicURL, &rec.IsActive)
		if err == nil {
			if rec.Provider == "local" {
				m.mu.Lock()
				m.backends[backendID] = m.localStorage
				m.backendRecords[backendID] = rec
				m.mu.Unlock()
				return m.localStorage, nil
			} else if rec.Provider == "s3" {
				decryptedSecret, decErr := m.decryptSecret(rec.SecretKeyEncrypted)
				if decErr == nil {
					targetBucket := rec.Bucket
					if bucket != "" {
						targetBucket = bucket
					}
					s3Instance, s3Err := objectstorage.NewS3(objectstorage.S3Config{
						Endpoint:          rec.Endpoint,
						Region:            rec.Region,
						Bucket:            targetBucket,
						AccessKey:         rec.AccessKeyID,
						SecretKey:         decryptedSecret,
						UseSSL:            rec.UseSSL,
						PathStyle:         rec.ForcePathStyle,
						PublicURL:         rec.PublicURL,
						AllowInsecureHTTP: !rec.UseSSL,
					})
					if s3Err == nil {
						adapter := newS3Adapter(s3Instance)
						m.mu.Lock()
						m.backends[backendID] = adapter
						m.backendRecords[backendID] = rec
						m.mu.Unlock()
						return adapter, nil
					}
				}
			}
		}
	}

	// Fallback to active storage
	st, err := m.ResolveStorage(provider)
	if err != nil {
		return nil, err
	}
	if bucketAware, ok := st.(interface{ WithBucket(string) Storage }); ok && bucket != "" {
		return bucketAware.WithBucket(bucket), nil
	}
	return st, nil
}

func (m *StorageManager) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, time.Time, error) {
	m.mu.RLock()
	active := m.activeType
	localSt := m.localStorage
	s3St := m.s3Storage
	m.mu.RUnlock()

	// If the file exists on local storage, always use local storage
	if localSt != nil {
		if _, err := localSt.Inspect(ctx, key); err == nil {
			return localSt.PresignGet(ctx, key, ttl)
		}
	}

	// If S3 is configured and active or local file not found
	if s3St != nil {
		return s3St.PresignGet(ctx, key, ttl)
	}

	if active == "s3" && s3St != nil {
		return s3St.PresignGet(ctx, key, ttl)
	}
	return localSt.PresignGet(ctx, key, ttl)
}

func (m *StorageManager) PresignGetProvider(ctx context.Context, provider, key string, ttl time.Duration) (string, time.Time, error) {
	st, err := m.ResolveStorage(provider)
	if err != nil {
		return "", time.Time{}, err
	}
	return st.PresignGet(ctx, key, ttl)
}

func (m *StorageManager) PresignPut(ctx context.Context, key, mime string, size int64, ttl time.Duration) (string, map[string]string, time.Time, error) {
	st, _, _, err := m.ActiveStorage(ctx)
	if err != nil {
		return "", nil, time.Time{}, err
	}
	return st.PresignPut(ctx, key, mime, size, ttl)
}

func (m *StorageManager) Inspect(ctx context.Context, key string) (StoredObject, error) {
	m.mu.RLock()
	localSt := m.localStorage
	s3St := m.s3Storage
	m.mu.RUnlock()

	if localSt != nil {
		if obj, err := localSt.Inspect(ctx, key); err == nil {
			return obj, nil
		}
	}
	if s3St != nil {
		return s3St.Inspect(ctx, key)
	}
	return StoredObject{}, ErrStorageMissing
}

func (m *StorageManager) Delete(ctx context.Context, key string) error {
	m.mu.RLock()
	localSt := m.localStorage
	s3St := m.s3Storage
	m.mu.RUnlock()

	var lastErr error
	deleted := false
	if localSt != nil {
		if err := localSt.Delete(ctx, key); err == nil {
			deleted = true
		} else if !errors.Is(err, ErrStorageMissing) {
			lastErr = err
		}
	}
	if s3St != nil {
		if err := s3St.Delete(ctx, key); err == nil {
			deleted = true
		} else if !errors.Is(err, ErrStorageMissing) {
			lastErr = err
		}
	}
	if deleted {
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return ErrStorageMissing
}

func (m *StorageManager) GetSettings(_ context.Context) (StorageSettings, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	masked := ""
	if m.s3Config.SecretKey != "" {
		masked = "********"
	}

	return StorageSettings{
		Provider: m.activeType,
		S3: S3PublicConfig{
			Endpoint:            m.s3Config.Endpoint,
			Region:              m.s3Config.Region,
			Bucket:              m.s3Config.Bucket,
			AccessKey:           m.s3Config.AccessKey,
			SecretKeyConfigured: m.s3Config.SecretKey != "",
			SecretKeyMasked:     masked,
			UseSSL:              m.s3Config.UseSSL,
			PathStyle:           m.s3Config.PathStyle,
			PublicURL:           m.s3Config.PublicURL,
		},
	}, nil
}

func (m *StorageManager) UpdateSettings(ctx context.Context, actorID string, update StorageSettingsUpdate) (StorageSettings, error) {
	update.Provider = strings.ToLower(strings.TrimSpace(update.Provider))
	if update.Provider != "local" && update.Provider != "s3" {
		return StorageSettings{}, errors.New("provider must be 'local' or 's3'")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	endpoint := strings.TrimSpace(update.S3.Endpoint)
	region := strings.TrimSpace(update.S3.Region)
	bucket := strings.TrimSpace(update.S3.Bucket)
	accessKey := strings.TrimSpace(update.S3.AccessKey)
	secretKey := strings.TrimSpace(update.S3.SecretKey)

	// If secret key not provided in update or masked, keep current secret
	if secretKey == "" || secretKey == "********" {
		secretKey = m.s3Config.SecretKey
	}

	var activeBackendID string
	var newS3AdapterInstance Storage

	if update.Provider == "s3" {
		if endpoint == "" {
			return StorageSettings{}, errors.New("S3 endpoint is required")
		}
		if bucket == "" {
			return StorageSettings{}, errors.New("S3 bucket name is required")
		}
		if accessKey == "" {
			return StorageSettings{}, errors.New("S3 access key is required")
		}
		if secretKey == "" {
			return StorageSettings{}, errors.New("S3 secret key is required")
		}
		if region == "" {
			region = "ru-central1"
		}

		s3Client, err := objectstorage.NewS3(objectstorage.S3Config{
			Endpoint:          endpoint,
			Region:            region,
			Bucket:            bucket,
			AccessKey:         accessKey,
			SecretKey:         secretKey,
			UseSSL:            update.S3.UseSSL,
			PathStyle:         update.S3.PathStyle,
			PublicURL:         update.S3.PublicURL,
			AllowInsecureHTTP: !update.S3.UseSSL,
		})
		if err != nil {
			return StorageSettings{}, fmt.Errorf("invalid S3 configuration: %w", err)
		}

		// Verify connection probe
		if err := s3Client.Probe(ctx); err != nil {
			return StorageSettings{}, fmt.Errorf("S3 connection verification failed: %w", err)
		}

		newS3AdapterInstance = newS3Adapter(s3Client)
	}

	// Persist to Postgres if available
	if m.db != nil {
		tx, err := m.db.BeginTx(ctx, nil)
		if err != nil {
			return StorageSettings{}, fmt.Errorf("begin transaction: %w", err)
		}
		defer func() { _ = tx.Rollback() }()

		// Deactivate all backends
		if _, err := tx.ExecContext(ctx, `UPDATE storage_backends SET is_active = false, updated_at = now()`); err != nil {
			return StorageSettings{}, fmt.Errorf("deactivate backends: %w", err)
		}

		if update.Provider == "local" {
			var localID string
			err := tx.QueryRowContext(ctx, `
				INSERT INTO storage_backends (provider, bucket, is_active)
				VALUES ('local', 'local-media', true)
				ON CONFLICT DO NOTHING
				RETURNING id::text`).Scan(&localID)
			if errors.Is(err, sql.ErrNoRows) || err != nil {
				_ = tx.QueryRowContext(ctx, `
					UPDATE storage_backends SET is_active = true, updated_at = now()
					WHERE provider = 'local'
					RETURNING id::text`).Scan(&localID)
			}
			activeBackendID = localID
			m.backends[localID] = m.localStorage
		} else if update.Provider == "s3" {
			encrypted, err := m.encryptSecret(secretKey)
			if err != nil {
				return StorageSettings{}, fmt.Errorf("encrypt s3 secret: %w", err)
			}

			var s3ID string
			err = tx.QueryRowContext(ctx, `
				SELECT id::text FROM storage_backends
				WHERE provider = 's3' AND endpoint = $1 AND bucket = $2 AND access_key_id = $3 AND use_ssl = $4 AND force_path_style = $5
				LIMIT 1`, endpoint, bucket, accessKey, update.S3.UseSSL, update.S3.PathStyle).Scan(&s3ID)
			if err == nil && s3ID != "" {
				_, err = tx.ExecContext(ctx, `
					UPDATE storage_backends
					SET secret_key_encrypted = $1, region = $2, public_url = $3, is_active = true, updated_at = now()
					WHERE id = $4`, encrypted, region, update.S3.PublicURL, s3ID)
				if err != nil {
					return StorageSettings{}, fmt.Errorf("update storage backend: %w", err)
				}
				activeBackendID = s3ID
			} else {
				err = tx.QueryRowContext(ctx, `
					INSERT INTO storage_backends
					  (provider, endpoint, region, bucket, access_key_id, secret_key_encrypted, use_ssl, force_path_style, public_url, is_active)
					VALUES ('s3', $1, $2, $3, $4, $5, $6, $7, $8, true)
					RETURNING id::text`, endpoint, region, bucket, accessKey, encrypted, update.S3.UseSSL, update.S3.PathStyle, update.S3.PublicURL).Scan(&activeBackendID)
				if err != nil {
					return StorageSettings{}, fmt.Errorf("insert storage backend: %w", err)
				}
			}
			m.backends[activeBackendID] = newS3AdapterInstance
		}

		// Update feature_flags and storage_credentials for legacy backward compatibility
		configMap := map[string]any{
			"provider": update.Provider,
			"s3": map[string]any{
				"endpoint":   endpoint,
				"region":     region,
				"bucket":     bucket,
				"access_key": accessKey,
				"use_ssl":    update.S3.UseSSL,
				"path_style": update.S3.PathStyle,
				"public_url": update.S3.PublicURL,
			},
		}
		configJSON, _ := json.Marshal(configMap)
		_, _ = tx.ExecContext(ctx, `
			INSERT INTO feature_flags (key, enabled, description, config, updated_by, updated_at)
			VALUES ('file_storage', true, 'Настройки файлового хранилища (Локальный сервер / S3)', $1, NULLIF($2, '')::uuid, now())
			ON CONFLICT (key) DO UPDATE SET config = EXCLUDED.config, updated_by = EXCLUDED.updated_by, updated_at = now()`,
			configJSON, actorID)

		if secretKey != "" && update.Provider == "s3" {
			encrypted, _ := m.encryptSecret(secretKey)
			_, _ = tx.ExecContext(ctx, `
				INSERT INTO storage_credentials (provider, encrypted_config, updated_by, updated_at)
				VALUES ('s3', $1, NULLIF($2, '')::uuid, now())
				ON CONFLICT (provider) DO UPDATE SET encrypted_config = EXCLUDED.encrypted_config, updated_by = EXCLUDED.updated_by, updated_at = now()`,
				encrypted, actorID)
		}

		if err := tx.Commit(); err != nil {
			return StorageSettings{}, fmt.Errorf("commit storage settings: %w", err)
		}
	}

	// Update in-memory state
	m.activeBackendID = activeBackendID
	m.activeType = update.Provider
	if update.Provider == "s3" {
		m.s3Storage = newS3AdapterInstance
		m.s3Config = S3UpdateConfig{
			Endpoint:  endpoint,
			Region:    region,
			Bucket:    bucket,
			AccessKey: accessKey,
			SecretKey: secretKey,
			UseSSL:    update.S3.UseSSL,
			PathStyle: update.S3.PathStyle,
			PublicURL: strings.TrimSpace(update.S3.PublicURL),
		}
	}

	masked := ""
	if m.s3Config.SecretKey != "" {
		masked = "********"
	}

	return StorageSettings{
		Provider: m.activeType,
		S3: S3PublicConfig{
			Endpoint:            m.s3Config.Endpoint,
			Region:              m.s3Config.Region,
			Bucket:              m.s3Config.Bucket,
			AccessKey:           m.s3Config.AccessKey,
			SecretKeyConfigured: m.s3Config.SecretKey != "",
			SecretKeyMasked:     masked,
			UseSSL:              m.s3Config.UseSSL,
			PathStyle:           m.s3Config.PathStyle,
			PublicURL:           m.s3Config.PublicURL,
		},
	}, nil
}

func (m *StorageManager) TestConnection(ctx context.Context, config S3UpdateConfig) error {
	endpoint := strings.TrimSpace(config.Endpoint)
	region := strings.TrimSpace(config.Region)
	bucket := strings.TrimSpace(config.Bucket)
	accessKey := strings.TrimSpace(config.AccessKey)
	secretKey := strings.TrimSpace(config.SecretKey)

	m.mu.RLock()
	if secretKey == "" || secretKey == "********" {
		secretKey = m.s3Config.SecretKey
	}
	m.mu.RUnlock()

	if endpoint == "" {
		return errors.New("S3 endpoint is required")
	}
	if bucket == "" {
		return errors.New("S3 bucket name is required")
	}
	if accessKey == "" {
		return errors.New("S3 access key is required")
	}
	if secretKey == "" {
		return errors.New("S3 secret key is required")
	}
	if region == "" {
		region = "ru-central1"
	}

	s3Client, err := objectstorage.NewS3(objectstorage.S3Config{
		Endpoint:          endpoint,
		Region:            region,
		Bucket:            bucket,
		AccessKey:         accessKey,
		SecretKey:         secretKey,
		UseSSL:            config.UseSSL,
		PathStyle:         config.PathStyle,
		PublicURL:         config.PublicURL,
		AllowInsecureHTTP: !config.UseSSL,
	})
	if err != nil {
		return fmt.Errorf("invalid S3 parameters: %w", err)
	}

	return s3Client.Probe(ctx)
}

func (m *StorageManager) encryptSecret(secret string) (string, error) {
	block, err := aes.NewCipher(m.encryptionKey[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(secret), nil)
	return base64.RawStdEncoding.EncodeToString(sealed), nil
}

func (m *StorageManager) decryptSecret(raw string) (string, error) {
	blob, err := base64.RawStdEncoding.DecodeString(raw)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(m.encryptionKey[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(blob) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := blob[:gcm.NonceSize()], blob[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (m *StorageManager) EncryptSecret(secret string) (string, error) {
	return m.encryptSecret(secret)
}

func (m *StorageManager) DecryptSecret(encrypted string) (string, error) {
	return m.decryptSecret(encrypted)
}
