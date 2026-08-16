package media

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DiskStorage is the durable development transport. Docker mounts RootDir on
// a named volume, so avatars and attachments survive API image rebuilds.
type DiskStorage struct {
	RootDir string
	BaseURL string
}

func (s *DiskStorage) path(key string) (string, bool) {
	clean := filepath.Clean("/" + key)
	if strings.Contains(clean, "..") {
		return "", false
	}
	return filepath.Join(s.RootDir, clean), true
}
func (s *DiskStorage) PresignPut(_ context.Context, key, mime string, size int64, ttl time.Duration) (string, map[string]string, time.Time, error) {
	return s.url("put", key), map[string]string{"Content-Type": mime}, time.Now().UTC().Add(ttl), nil
}
func (s *DiskStorage) Inspect(_ context.Context, key string) (StoredObject, error) {
	path, ok := s.path(key)
	if !ok {
		return StoredObject{}, ErrStorageMissing
	}
	info, err := os.Stat(path)
	if err != nil {
		return StoredObject{}, ErrStorageMissing
	}
	mimeBytes, _ := os.ReadFile(path + ".mime")
	return StoredObject{SizeBytes: info.Size(), MIMEType: strings.TrimSpace(string(mimeBytes))}, nil
}
func (s *DiskStorage) PresignGet(_ context.Context, key string, ttl time.Duration) (string, time.Time, error) {
	return s.url("get", key), time.Now().UTC().Add(ttl), nil
}
func (s *DiskStorage) Delete(_ context.Context, key string) error {
	path, ok := s.path(key)
	if !ok {
		return ErrStorageMissing
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	_ = os.Remove(path + ".mime")
	return nil
}
func (s *DiskStorage) Put(_ context.Context, key, mime string, data []byte) error {
	path, ok := s.path(key)
	if !ok {
		return ErrStorageMissing
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	if mime != "" {
		_ = os.WriteFile(path+".mime", []byte(mime), 0600)
	}
	return nil
}
func (s *DiskStorage) url(action, key string) string {
	base := strings.TrimRight(s.BaseURL, "/")
	if base == "" {
		base = "/api/v1/dev-storage"
	}
	return base + "?action=" + url.QueryEscape(action) + "&key=" + url.QueryEscape(key)
}
func (s *DiskStorage) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	action, key := r.URL.Query().Get("action"), r.URL.Query().Get("key")
	path, ok := s.path(key)
	if !ok || key == "" {
		http.NotFound(w, r)
		return
	}
	switch action {
	case "put":
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
			http.Error(w, "storage unavailable", 500)
			return
		}
		f, err := os.Create(path)
		if err != nil {
			http.Error(w, "storage unavailable", 500)
			return
		}
		_, copyErr := io.Copy(f, http.MaxBytesReader(w, r.Body, 25<<20))
		closeErr := f.Close()
		if copyErr != nil || closeErr != nil {
			_ = os.Remove(path)
			http.Error(w, "upload failed", 400)
			return
		}
		mime := strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0])
		_ = os.WriteFile(path+".mime", []byte(mime), 0600)
		w.WriteHeader(http.StatusNoContent)
	case "get":
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if _, err := os.Stat(path); err != nil {
			// Old development rows can reference files created before durable
			// media volumes were introduced. Do not leave a broken avatar URL in
			// every catalog card; avatar requests degrade to the bundled fallback.
			if strings.HasPrefix(key, "avatar/") {
				http.Redirect(w, r, "/media/avatars/avatar-01.svg", http.StatusTemporaryRedirect)
				return
			}
			http.NotFound(w, r)
			return
		}
		mime, _ := os.ReadFile(path + ".mime")
		if len(mime) > 0 {
			w.Header().Set("Content-Type", strings.TrimSpace(string(mime)))
		}
		http.ServeFile(w, r, path)
	default:
		http.NotFound(w, r)
	}
}
