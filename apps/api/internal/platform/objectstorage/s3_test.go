package objectstorage

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestS3PresignContract(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	storage, err := NewS3(S3Config{Endpoint: "https://storage.example.com", Region: "ru-1", Bucket: "private-media",
		AccessKey: "access", SecretKey: "secret", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	value, headers, expiresAt, err := storage.PresignPut(context.Background(), "portfolio/user/object.png", "image/png", 100, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(value)
	query := parsed.Query()
	if parsed.Scheme != "https" || !strings.HasSuffix(parsed.Path, "/private-media/portfolio/user/object.png") ||
		query.Get("X-Amz-Signature") == "" || query.Get("X-Amz-SignedHeaders") != "content-type;host" ||
		headers["Content-Type"] != "image/png" || !expiresAt.Equal(now.Add(15*time.Minute)) || strings.Contains(value, "secret") {
		t.Fatalf("url = %s, headers = %#v, expires = %s", value, headers, expiresAt)
	}
}

func TestS3InspectDetectsSignature(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("x", 20))
	client := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r.Header.Get("Authorization") == "" {
				t.Error("authorization header is missing")
			}
			switch r.Method {
			case http.MethodHead:
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewReader(nil)),
				}
				resp.Header.Set("Content-Length", "28")
				resp.Header.Set("Content-Type", "image/png")
				return resp, nil
			case http.MethodGet:
				resp := &http.Response{
					StatusCode: http.StatusPartialContent,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewReader(png)),
				}
				return resp, nil
			case http.MethodDelete:
				return &http.Response{
					StatusCode: http.StatusNoContent,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewReader(nil)),
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewReader(nil)),
				}, nil
			}
		}),
	}

	storage, err := NewS3(S3Config{
		Endpoint:   "https://s3.mock.example.com",
		Region:     "test",
		Bucket:     "bucket",
		AccessKey:  "access",
		SecretKey:  "secret",
		HTTPClient: client,
		Now:        func() time.Time { return time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	object, err := storage.Inspect(context.Background(), "portfolio/user/object.png")
	if err != nil || object.SizeBytes != 28 || object.MIMEType != "image/png" || object.DetectedMIME != "image/png" {
		t.Fatalf("object = %#v, error = %v", object, err)
	}
	if err := storage.Delete(context.Background(), "portfolio/user/object.png"); err != nil {
		t.Fatal(err)
	}
}

func TestS3ProbeAndPut(t *testing.T) {
	var mu sync.Mutex
	objects := make(map[string][]byte)

	client := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			mu.Lock()
			defer mu.Unlock()

			key := r.URL.Path
			switch r.Method {
			case http.MethodPut:
				body, _ := io.ReadAll(r.Body)
				objects[key] = body
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
				resp.Header.Set("Content-Type", "text/plain")
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

	storage, err := NewS3(S3Config{
		Endpoint:   "https://s3.mock.example.com",
		Region:     "ru-central1",
		Bucket:     "test-bucket",
		AccessKey:  "access",
		SecretKey:  "secret",
		HTTPClient: client,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := storage.Probe(context.Background()); err != nil {
		t.Fatalf("Probe failed: %v", err)
	}
}

func TestS3RejectsUnsafeEndpoint(t *testing.T) {
	_, err := NewS3(S3Config{Endpoint: "http://storage.example.com", Region: "test", Bucket: "bucket", AccessKey: "access", SecretKey: "secret"})
	if err == nil {
		t.Fatal("insecure endpoint was accepted")
	}
}

