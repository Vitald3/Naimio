package objectstorage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

var ErrStorageMissing = errors.New("stored object not found")

type StoredObject struct {
	SizeBytes    int64
	MIMEType     string
	DetectedMIME string
	Data         []byte
}

type S3Config struct {
	Endpoint          string
	Region            string
	Bucket            string
	AccessKey         string
	SecretKey         string
	SessionToken      string
	UseSSL            bool
	PathStyle         bool
	PublicURL         string
	AllowInsecureHTTP bool
	HTTPClient        *http.Client
	Now               func() time.Time
}

type S3 struct {
	endpoint          *url.URL
	region            string
	bucket            string
	accessKey         string
	secretKey         string
	sessionToken      string
	useSSL            bool
	pathStyle         bool
	publicURL         *url.URL
	client            *http.Client
	now               func() time.Time
}

func NewS3(config S3Config) (*S3, error) {
	rawEndpoint := strings.TrimSpace(config.Endpoint)
	if rawEndpoint == "" {
		return nil, errors.New("object storage endpoint is required")
	}
	if !strings.HasPrefix(rawEndpoint, "http://") && !strings.HasPrefix(rawEndpoint, "https://") {
		if config.UseSSL || !config.AllowInsecureHTTP {
			rawEndpoint = "https://" + rawEndpoint
		} else {
			rawEndpoint = "http://" + rawEndpoint
		}
	}
	endpoint, err := url.Parse(strings.TrimRight(rawEndpoint, "/"))
	if err != nil || endpoint.Host == "" || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil, errors.New("OBJECT_STORAGE_ENDPOINT must be a valid origin")
	}
	if endpoint.Scheme != "https" && !config.AllowInsecureHTTP {
		return nil, errors.New("OBJECT_STORAGE_ENDPOINT must be an HTTPS origin")
	}
	if config.Region == "" || config.Bucket == "" || config.AccessKey == "" || config.SecretKey == "" || strings.Contains(config.Bucket, "/") {
		return nil, errors.New("object storage configuration is incomplete")
	}
	var pubURL *url.URL
	if strings.TrimSpace(config.PublicURL) != "" {
		p, err := url.Parse(strings.TrimRight(strings.TrimSpace(config.PublicURL), "/"))
		if err == nil && p.Host != "" {
			pubURL = p
		}
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &S3{
		endpoint:     endpoint,
		region:       config.Region,
		bucket:       config.Bucket,
		accessKey:    config.AccessKey,
		secretKey:    config.SecretKey,
		sessionToken: config.SessionToken,
		useSSL:       endpoint.Scheme == "https",
		pathStyle:    config.PathStyle,
		publicURL:    pubURL,
		client:       client,
		now:          now,
	}, nil
}

func (s *S3) Bucket() string {
	return s.bucket
}

func (s *S3) WithBucket(bucket string) *S3 {
	if bucket == "" || bucket == s.bucket {
		return s
	}
	cp := *s
	cp.bucket = bucket
	return &cp
}

func (s *S3) PresignPut(_ context.Context, key, mime string, _ int64, ttl time.Duration) (string, map[string]string, time.Time, error) {
	value, expiresAt, err := s.presign(http.MethodPut, key, ttl, map[string]string{"content-type": mime})
	return value, map[string]string{"Content-Type": mime}, expiresAt, err
}

func (s *S3) PresignGet(_ context.Context, key string, ttl time.Duration) (string, time.Time, error) {
	return s.presign(http.MethodGet, key, ttl, nil)
}

func (s *S3) Inspect(ctx context.Context, key string) (StoredObject, error) {
	headResponse, err := s.do(ctx, http.MethodHead, key, nil)
	if err != nil {
		return StoredObject{}, err
	}
	defer headResponse.Body.Close()
	if headResponse.StatusCode == http.StatusNotFound {
		return StoredObject{}, ErrStorageMissing
	}
	if headResponse.StatusCode < 200 || headResponse.StatusCode >= 300 {
		return StoredObject{}, fmt.Errorf("object storage HEAD returned %d", headResponse.StatusCode)
	}
	size, err := strconv.ParseInt(headResponse.Header.Get("Content-Length"), 10, 64)
	if err != nil || size < 0 {
		return StoredObject{}, errors.New("object storage returned invalid content length")
	}

	getResponse, err := s.do(ctx, http.MethodGet, key, map[string]string{"Range": "bytes=0-511"})
	if err != nil {
		return StoredObject{}, err
	}
	defer getResponse.Body.Close()
	if getResponse.StatusCode != http.StatusOK && getResponse.StatusCode != http.StatusPartialContent {
		return StoredObject{}, fmt.Errorf("object storage range GET returned %d", getResponse.StatusCode)
	}
	sample, err := io.ReadAll(io.LimitReader(getResponse.Body, 512))
	if err != nil {
		return StoredObject{}, err
	}
	return StoredObject{SizeBytes: size, MIMEType: headResponse.Header.Get("Content-Type"), DetectedMIME: http.DetectContentType(sample)}, nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	response, err := s.do(ctx, http.MethodDelete, key, nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return ErrStorageMissing
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("object storage DELETE returned %d", response.StatusCode)
	}
	return nil
}

func (s *S3) Put(ctx context.Context, key, mime string, data []byte) error {
	now := s.now().UTC()
	timestamp, date := now.Format("20060102T150405Z"), now.Format("20060102")
	payloadSum := sha256.Sum256(data)
	payloadHash := hex.EncodeToString(payloadSum[:])
	objectURL := s.objectURL(key)
	signed := map[string]string{
		"host":                 objectURL.Host,
		"x-amz-content-sha256": payloadHash,
		"x-amz-date":           timestamp,
		"content-type":         mime,
		"content-length":       strconv.Itoa(len(data)),
	}
	if s.sessionToken != "" {
		signed["x-amz-security-token"] = s.sessionToken
	}
	canonical, names := canonicalHeaders(signed)
	canonicalRequest := "PUT\n" + objectURL.EscapedPath() + "\n\n" + canonical + "\n" + names + "\n" + payloadHash
	requestHash := sha256.Sum256([]byte(canonicalRequest))
	scope := date + "/" + s.region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + timestamp + "\n" + scope + "\n" + hex.EncodeToString(requestHash[:])
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, objectURL.String(), bytes.NewReader(data))
	if err != nil {
		return err
	}
	for name, value := range signed {
		if name != "host" {
			request.Header.Set(name, value)
		}
	}
	request.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+s.accessKey+"/"+scope+", SignedHeaders="+names+", Signature="+hex.EncodeToString(s.signature(date, stringToSign)))
	resp, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("S3 PUT returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (s *S3) Probe(ctx context.Context) error {
	probeKey := fmt.Sprintf("_naimio_storage_probe_%d.tmp", time.Now().UnixNano())
	probeData := []byte("storage_connection_probe")
	if err := s.Put(ctx, probeKey, "text/plain", probeData); err != nil {
		return fmt.Errorf("write test failed: %w", err)
	}
	defer func() {
		_ = s.Delete(context.Background(), probeKey)
	}()
	obj, err := s.Inspect(ctx, probeKey)
	if err != nil {
		return fmt.Errorf("read test failed: %w", err)
	}
	if obj.SizeBytes != int64(len(probeData)) {
		return fmt.Errorf("read test failed: expected %d bytes, got %d", len(probeData), obj.SizeBytes)
	}
	if err := s.Delete(ctx, probeKey); err != nil {
		return fmt.Errorf("delete test failed: %w", err)
	}
	return nil
}

func (s *S3) presign(method, key string, ttl time.Duration, signed map[string]string) (string, time.Time, error) {
	if ttl <= 0 || ttl > 24*time.Hour {
		return "", time.Time{}, errors.New("invalid presign ttl")
	}
	now := s.now().UTC()
	date, timestamp := now.Format("20060102"), now.Format("20060102T150405Z")
	scope := date + "/" + s.region + "/s3/aws4_request"
	objectURL := s.objectURL(key)
	query := objectURL.Query()
	query.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	query.Set("X-Amz-Credential", s.accessKey+"/"+scope)
	query.Set("X-Amz-Date", timestamp)
	query.Set("X-Amz-Expires", strconv.FormatInt(int64(ttl/time.Second), 10))
	if s.sessionToken != "" {
		query.Set("X-Amz-Security-Token", s.sessionToken)
	}
	headers := map[string]string{"host": objectURL.Host}
	for name, value := range signed {
		headers[strings.ToLower(name)] = strings.TrimSpace(value)
	}
	canonicalHeaders, signedHeaders := canonicalHeaders(headers)
	query.Set("X-Amz-SignedHeaders", signedHeaders)
	objectURL.RawQuery = query.Encode()
	canonicalRequest := method + "\n" + objectURL.EscapedPath() + "\n" + objectURL.RawQuery + "\n" + canonicalHeaders + "\n" + signedHeaders + "\nUNSIGNED-PAYLOAD"
	requestHash := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := "AWS4-HMAC-SHA256\n" + timestamp + "\n" + scope + "\n" + hex.EncodeToString(requestHash[:])
	query.Set("X-Amz-Signature", hex.EncodeToString(s.signature(date, stringToSign)))
	objectURL.RawQuery = query.Encode()
	return objectURL.String(), now.Add(ttl), nil
}

func (s *S3) do(ctx context.Context, method, key string, headers map[string]string) (*http.Response, error) {
	now := s.now().UTC()
	timestamp, date := now.Format("20060102T150405Z"), now.Format("20060102")
	objectURL := s.objectURL(key)
	payloadHash := hex.EncodeToString(sha256.New().Sum(nil))
	signed := map[string]string{"host": objectURL.Host, "x-amz-content-sha256": payloadHash, "x-amz-date": timestamp}
	if s.sessionToken != "" {
		signed["x-amz-security-token"] = s.sessionToken
	}
	canonical, names := canonicalHeaders(signed)
	canonicalRequest := method + "\n" + objectURL.EscapedPath() + "\n\n" + canonical + "\n" + names + "\n" + payloadHash
	requestHash := sha256.Sum256([]byte(canonicalRequest))
	scope := date + "/" + s.region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + timestamp + "\n" + scope + "\n" + hex.EncodeToString(requestHash[:])
	request, err := http.NewRequestWithContext(ctx, method, objectURL.String(), nil)
	if err != nil {
		return nil, err
	}
	for name, value := range signed {
		if name != "host" {
			request.Header.Set(name, value)
		}
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	request.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+s.accessKey+"/"+scope+", SignedHeaders="+names+", Signature="+hex.EncodeToString(s.signature(date, stringToSign)))
	return s.client.Do(request)
}

func (s *S3) objectURL(key string) *url.URL {
	copy := *s.endpoint
	segments := strings.Split(strings.Trim(key, "/"), "/")
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	copy.RawPath = ""
	copy.Path = strings.TrimRight(copy.Path, "/") + "/" + url.PathEscape(s.bucket) + "/" + strings.Join(segments, "/")
	return &copy
}

func (s *S3) signature(date, value string) []byte {
	dateKey := hmacSHA256([]byte("AWS4"+s.secretKey), date)
	regionKey := hmacSHA256(dateKey, s.region)
	serviceKey := hmacSHA256(regionKey, "s3")
	signingKey := hmacSHA256(serviceKey, "aws4_request")
	return hmacSHA256(signingKey, value)
}

func canonicalHeaders(headers map[string]string) (string, string) {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, strings.ToLower(name))
	}
	slices.Sort(names)
	var canonical strings.Builder
	for _, name := range names {
		canonical.WriteString(name)
		canonical.WriteByte(':')
		canonical.WriteString(strings.Join(strings.Fields(headers[name]), " "))
		canonical.WriteByte('\n')
	}
	return canonical.String(), strings.Join(names, ";")
}

func hmacSHA256(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}
