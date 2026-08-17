package media

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Store struct {
	mu      sync.RWMutex
	Objects map[string]Object
}

func (s *Store) Create(_ context.Context, object Object) (Object, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Objects == nil {
		s.Objects = make(map[string]Object)
	}
	if object.StorageProvider == "" {
		object.StorageProvider = "local"
	}
	s.Objects[object.ID] = object
	return object, nil
}

func (s *Store) GetOwned(_ context.Context, actorID, mediaID string) (Object, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	object, ok := s.Objects[mediaID]
	if !ok || object.OwnerID != actorID || object.DeletedAt != nil {
		return Object{}, ErrNotFound
	}
	return object, nil
}

func (s *Store) GetPublic(_ context.Context, mediaID string) (Object, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	object, ok := s.Objects[mediaID]
	if !ok || object.DeletedAt != nil || object.UploadedAt == nil || object.ScanStatus != ScanClean {
		return Object{}, ErrNotFound
	}
	return object, nil
}

func (s *Store) MarkUploaded(_ context.Context, actorID, mediaID string, at time.Time) (Object, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	object, ok := s.Objects[mediaID]
	if !ok || object.OwnerID != actorID || object.DeletedAt != nil {
		return Object{}, ErrNotFound
	}
	if object.UploadedAt == nil {
		at = at.UTC()
		object.UploadedAt = &at
		object.UpdatedAt = at
		s.Objects[mediaID] = object
	}
	return object, nil
}

func (s *Store) MarkScanResult(_ context.Context, mediaID, status string) error {
	if status != ScanClean && status != ScanInfected && status != ScanFailed {
		return invalid("invalid scan status")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	object, ok := s.Objects[mediaID]
	if !ok || object.DeletedAt != nil || object.UploadedAt == nil {
		return ErrNotFound
	}
	object.ScanStatus = status
	object.UpdatedAt = time.Now().UTC()
	s.Objects[mediaID] = object
	return nil
}

func (s *Store) Delete(_ context.Context, actorID, mediaID string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	object, ok := s.Objects[mediaID]
	if !ok || object.OwnerID != actorID || object.DeletedAt != nil {
		return ErrNotFound
	}
	at = at.UTC()
	object.DeletedAt, object.UpdatedAt = &at, at
	s.Objects[mediaID] = object
	return nil
}

type MemoryStorage struct {
	mu      sync.RWMutex
	Objects map[string]StoredObject
	BaseURL string
	Now     func() time.Time
}

func (s *MemoryStorage) PresignPut(_ context.Context, key, mime string, size int64, ttl time.Duration) (string, map[string]string, time.Time, error) {
	return s.url("put", key), map[string]string{"Content-Type": mime}, s.now().Add(ttl), nil
}

func (s *MemoryStorage) Inspect(_ context.Context, key string) (StoredObject, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	object, ok := s.Objects[key]
	if !ok {
		return StoredObject{}, ErrStorageMissing
	}
	return object, nil
}

func (s *MemoryStorage) PresignGet(_ context.Context, key string, ttl time.Duration) (string, time.Time, error) {
	return s.url("get", key), s.now().Add(ttl), nil
}

func (s *MemoryStorage) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Objects[key]; !ok {
		return ErrStorageMissing
	}
	delete(s.Objects, key)
	return nil
}

func (s *MemoryStorage) Open(_ context.Context, key string) (io.ReadCloser, int64, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	object, ok := s.Objects[key]
	if !ok {
		return nil, 0, "", ErrStorageMissing
	}
	return io.NopCloser(bytes.NewReader(object.Data)), object.SizeBytes, object.MIMEType, nil
}

func (s *MemoryStorage) url(action, key string) string {
	base := strings.TrimRight(s.BaseURL, "/")
	if base == "" {
		base = "/api/v1/dev-storage"
	}
	return base + "?action=" + url.QueryEscape(action) + "&key=" + url.QueryEscape(key)
}

// ServeHTTP provides an in-memory object-storage transport for local development.
// It is registered by cmd/api only outside production; production always uses S3.
func (s *MemoryStorage) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	action := r.URL.Query().Get("action")
	key := r.URL.Query().Get("key")
	if key == "" || (action != "put" && action != "get") {
		http.NotFound(w, r)
		return
	}
	switch action {
	case "put":
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		reader := http.MaxBytesReader(w, r.Body, 25<<20)
		data, err := io.ReadAll(reader)
		if err != nil {
			http.Error(w, "upload too large", http.StatusRequestEntityTooLarge)
			return
		}
		mime := strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0])
		s.mu.Lock()
		if s.Objects == nil {
			s.Objects = map[string]StoredObject{}
		}
		s.Objects[key] = StoredObject{SizeBytes: int64(len(data)), MIMEType: mime, Data: append([]byte(nil), data...)}
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case "get":
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		s.mu.RLock()
		object, ok := s.Objects[key]
		s.mu.RUnlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		if object.MIMEType != "" {
			w.Header().Set("Content-Type", object.MIMEType)
		}
		w.Header().Set("Content-Length", fmtInt64(object.SizeBytes))
		if r.Method == http.MethodGet {
			_, _ = w.Write(object.Data)
		}
	}
}

func fmtInt64(value int64) string {
	if value == 0 {
		return "0"
	}
	buf := make([]byte, 0, 20)
	for value > 0 {
		buf = append(buf, byte('0'+value%10))
		value /= 10
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}

func (s *MemoryStorage) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
