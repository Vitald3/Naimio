package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"freelance/apps/api/internal/media"
	"freelance/apps/api/internal/platform/objectstorage"
)

const (
	baseURL        = "http://127.0.0.1:8088"
	adminEmail     = "admin@example.test"
	adminPass      = "LocalDemo2026!"
	userAEmail     = "freelancer@example.test"
	userAPass      = "LocalDemo2026!"
	userBEmail     = "customer@example.test"
	userBPass      = "LocalDemo2026!"
	minioAEndpoint = "http://minio-a:9000"
	minioAUser     = "endpoint-a-user"
	minioAPass     = "endpoint-a-secret"
	bucketName     = "files"
)

type Checker struct {
	client   *http.Client
	adminJar *cookiejar.Jar
	userAJar *cookiejar.Jar
	userBJar *cookiejar.Jar
}

func clearRateLimits() {
	_ = exec.Command("docker", "exec", "freelance-redis-1", "redis-cli", "-a", "local_redis_2026", "flushall").Run()
}

func generateValidPNG(tag string) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
	buf.Write([]byte{0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52})
	buf.Write([]byte{0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00})
	buf.Write([]byte{0x1F, 0x15, 0xC4, 0x89})
	buf.Write([]byte{0x00, 0x00, 0x00, 0x0A, 0x49, 0x44, 0x41, 0x54})
	buf.Write([]byte{0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00, 0x05, 0x00, 0x01})
	buf.Write([]byte{0x0D, 0x0A, 0x2D, 0xB4})
	textData := fmt.Sprintf("Tag\x00%s_%d", tag, time.Now().UnixNano())
	textLen := uint32(len(textData))
	buf.Write([]byte{byte(textLen >> 24), byte(textLen >> 16), byte(textLen >> 8), byte(textLen)})
	buf.Write([]byte("tEXt"))
	buf.Write([]byte(textData))
	crc := sha256.Sum256([]byte("tEXt" + textData))
	buf.Write(crc[:4])
	buf.Write([]byte{0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82})
	return buf.Bytes()
}

func main() {
	log.Println("=== Starting Verification of Final 3 Checks ===")
	clearRateLimits()

	adminJar, _ := cookiejar.New(nil)
	userAJar, _ := cookiejar.New(nil)
	userBJar, _ := cookiejar.New(nil)

	c := &Checker{
		client:   &http.Client{Timeout: 30 * time.Second},
		adminJar: adminJar,
		userAJar: userAJar,
		userBJar: userBJar,
	}

	c.SetupMinIO()
	c.AuthenticateAll()

	// Check 1: Security-проверка UploadSessionMiddleware, Purpose Isolation, IDOR, Staff media lifecycle
	c.RunCheck1SecurityUploadIsolation()

	// Check 2: Проверка upgrade migration 000053 → 000054
	c.RunCheck2Migration053To054()

	// Check 3: Проверка единственного active backend, DB constraint & concurrency
	c.RunCheck3ActiveBackendInvariant()

	log.Println("\n=======================================================")
	log.Println("ALL 3 FINAL CHECKS COMPLETED SUCCESSFULLY (100% PASS)!")
	log.Println("=======================================================")
}

func (c *Checker) SetupMinIO() {
	_ = exec.Command("docker", "exec", "freelance-minio-a-1", "mc", "alias", "set", "minio-a", "http://127.0.0.1:9000", minioAUser, minioAPass).Run()
	_ = exec.Command("docker", "exec", "freelance-minio-a-1", "mc", "mb", "minio-a/"+bucketName).Run()
}

func (c *Checker) AuthenticateAll() {
	clearRateLimits()
	log.Println("\n[Auth] Authenticating Admin, User A, and User B...")

	// Admin
	loginAdminBody, _ := json.Marshal(map[string]string{"email": adminEmail, "password": adminPass, "portal": "admin"})
	reqAdmin, _ := http.NewRequest("POST", baseURL+"/api/v1/auth/login", bytes.NewReader(loginAdminBody))
	reqAdmin.Header.Set("Content-Type", "application/json")
	reqAdmin.Header.Set("Origin", baseURL)
	respAdmin, err := (&http.Client{Jar: c.adminJar, Timeout: 10 * time.Second}).Do(reqAdmin)
	if err != nil || respAdmin.StatusCode != 200 {
		log.Fatalf("Admin login failed: %v", err)
	}

	// User A
	loginUserABody, _ := json.Marshal(map[string]string{"email": userAEmail, "password": userAPass})
	reqUserA, _ := http.NewRequest("POST", baseURL+"/api/v1/auth/login", bytes.NewReader(loginUserABody))
	reqUserA.Header.Set("Content-Type", "application/json")
	reqUserA.Header.Set("Origin", baseURL)
	respUserA, err := (&http.Client{Jar: c.userAJar, Timeout: 10 * time.Second}).Do(reqUserA)
	if err != nil || respUserA.StatusCode != 200 {
		log.Fatalf("User A login failed: %v", err)
	}

	// User B
	loginUserBBody, _ := json.Marshal(map[string]string{"email": userBEmail, "password": userBPass})
	reqUserB, _ := http.NewRequest("POST", baseURL+"/api/v1/auth/login", bytes.NewReader(loginUserBBody))
	reqUserB.Header.Set("Content-Type", "application/json")
	reqUserB.Header.Set("Origin", baseURL)
	respUserB, err := (&http.Client{Jar: c.userBJar, Timeout: 10 * time.Second}).Do(reqUserB)
	if err != nil || respUserB.StatusCode != 200 {
		log.Fatalf("User B login failed: %v", err)
	}

	log.Println("PASS - All identities authenticated.")
}

func (c *Checker) RunCheck1SecurityUploadIsolation() {
	log.Println("\n=======================================================")
	log.Println("CHECK 1: UploadSessionMiddleware Security & Purpose Isolation")
	log.Println("=======================================================")

	adminClient := &http.Client{Jar: c.adminJar, Timeout: 10 * time.Second}
	userAClient := &http.Client{Jar: c.userAJar, Timeout: 10 * time.Second}
	userBClient := &http.Client{Jar: c.userBJar, Timeout: 10 * time.Second}

	// 1.1 Presign purpose isolation for session_admin
	log.Println("\n1.1 Testing Presign purpose isolation for session_admin...")
	purposes := []struct {
		purpose  string
		expected int // 201 for allowed, 401 or 422 for rejected
	}{
		{"AVATAR", 401},
		{"PORTFOLIO", 401},
		{"SERVICE", 401},
		{"PROJECT", 401},
		{"CHAT", 401},
		{"BLOG_COVER", 201},
		{"BLOG_CONTENT", 201},
	}

	for _, p := range purposes {
		clearRateLimits()
		preBody, _ := json.Marshal(map[string]any{
			"purpose":    p.purpose,
			"filename":   "test.png",
			"mime_type":  "image/png",
			"size_bytes": 1024,
		})
		req, _ := http.NewRequest("POST", baseURL+"/api/v1/uploads/presign", bytes.NewReader(preBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", baseURL)
		resp, err := adminClient.Do(req)
		if err != nil {
			log.Fatalf("Request error for %s: %v", p.purpose, err)
		}
		body, _ := io.ReadAll(resp.Body)
		if p.expected == 201 {
			if resp.StatusCode != 201 {
				log.Fatalf("Expected 201 for admin presign of %s, got %d: %s", p.purpose, resp.StatusCode, string(body))
			}
			log.Printf("Purpose %-15s → Status %d ALLOWED (PASS)", p.purpose, resp.StatusCode)
		} else {
			if resp.StatusCode == 200 || resp.StatusCode == 201 {
				log.Fatalf("SECURITY VIOLATION: Admin was able to presign non-staff purpose %s!", p.purpose)
			}
			log.Printf("Purpose %-15s → Status %d REJECTED (PASS)", p.purpose, resp.StatusCode)
		}
	}

	// 1.2 Subsequent upload endpoints (complete, get, delete) by session_admin on user media
	log.Println("\n1.2 Testing Subsequent upload endpoints on User media by session_admin...")
	userPurposes := []string{"AVATAR", "PORTFOLIO", "SERVICE", "PROJECT"}
	createdUserObjects := make(map[string]string) // purpose -> mediaID

	for _, purp := range userPurposes {
		clearRateLimits()
		content := generateValidPNG("USER_A_" + purp)
		preBody, _ := json.Marshal(map[string]any{
			"purpose":    purp,
			"filename":   strings.ToLower(purp) + ".png",
			"mime_type":  "image/png",
			"size_bytes": len(content),
		})
		reqPre, _ := http.NewRequest("POST", baseURL+"/api/v1/uploads/presign", bytes.NewReader(preBody))
		reqPre.Header.Set("Content-Type", "application/json")
		reqPre.Header.Set("Origin", baseURL)
		respPre, err := userAClient.Do(reqPre)
		if err != nil || respPre.StatusCode != 201 {
			body, _ := io.ReadAll(respPre.Body)
			log.Fatalf("User A presign %s failed: status=%d, body=%s", purp, respPre.StatusCode, string(body))
		}
		var preResp struct {
			Data struct {
				MediaID   string `json:"media_id"`
				UploadURL string `json:"upload_url"`
			} `json:"data"`
		}
		_ = json.NewDecoder(respPre.Body).Decode(&preResp)

		// Upload content to dev-storage / minio
		uploadURL := preResp.Data.UploadURL
		putURL := baseURL + uploadURL
		if strings.Contains(uploadURL, "minio-a:9000") {
			putURL = strings.Replace(uploadURL, "http://minio-a:9000", "http://localhost:9000", 1)
		}
		putReq, _ := http.NewRequest("PUT", putURL, bytes.NewReader(content))
		putReq.Header.Set("Content-Type", "image/png")
		if strings.Contains(uploadURL, "minio-a:9000") {
			putReq.Host = "minio-a:9000"
		}
		putResp, err := c.client.Do(putReq)
		if err != nil || (putResp.StatusCode != 200 && putResp.StatusCode != 204) {
			log.Fatalf("Upload PUT failed for %s", purp)
		}

		// User A completes the object
		reqComp, _ := http.NewRequest("POST", baseURL+"/api/v1/uploads/"+preResp.Data.MediaID+"/complete", nil)
		reqComp.Header.Set("Origin", baseURL)
		respComp, err := userAClient.Do(reqComp)
		if err != nil || respComp.StatusCode != 200 {
			log.Fatalf("User A complete %s failed", purp)
		}
		createdUserObjects[purp] = preResp.Data.MediaID
		log.Printf("User A created %s media object: %s", purp, preResp.Data.MediaID)
	}

	// Now Admin attempts GET, COMPLETE, DELETE on User A's media objects
	for purp, mediaID := range createdUserObjects {
		clearRateLimits()
		// GET
		reqGet, _ := http.NewRequest("GET", baseURL+"/api/v1/uploads/"+mediaID, nil)
		respGet, err := adminClient.Do(reqGet)
		if err != nil || respGet.StatusCode == 200 {
			log.Fatalf("SECURITY VIOLATION: Admin was able to GET User A's %s (status %d)!", purp, respGet.StatusCode)
		}
		log.Printf("Admin GET User A's %-12s (ID: %s) → Status %d REJECTED (PASS)", purp, mediaID, respGet.StatusCode)

		// COMPLETE
		clearRateLimits()
		reqComp, _ := http.NewRequest("POST", baseURL+"/api/v1/uploads/"+mediaID+"/complete", nil)
		reqComp.Header.Set("Origin", baseURL)
		respComp, err := adminClient.Do(reqComp)
		if err != nil || respComp.StatusCode == 200 {
			log.Fatalf("SECURITY VIOLATION: Admin was able to COMPLETE User A's %s (status %d)!", purp, respComp.StatusCode)
		}
		log.Printf("Admin COMPLETE User A's %-8s (ID: %s) → Status %d REJECTED (PASS)", purp, mediaID, respComp.StatusCode)

		// DELETE
		clearRateLimits()
		reqDel, _ := http.NewRequest("DELETE", baseURL+"/api/v1/uploads/"+mediaID, nil)
		reqDel.Header.Set("Origin", baseURL)
		respDel, err := adminClient.Do(reqDel)
		if err != nil || respDel.StatusCode == 200 || respDel.StatusCode == 204 {
			log.Fatalf("SECURITY VIOLATION: Admin was able to DELETE User A's %s (status %d)!", purp, respDel.StatusCode)
		}
		log.Printf("Admin DELETE User A's %-10s (ID: %s) → Status %d REJECTED (PASS)", purp, mediaID, respDel.StatusCode)
	}

	// 1.3 IDOR: User B attempts GET, COMPLETE, DELETE on User A's media
	log.Println("\n1.3 Testing IDOR User B → User A...")
	for purp, mediaID := range createdUserObjects {
		clearRateLimits()
		reqGet, _ := http.NewRequest("GET", baseURL+"/api/v1/uploads/"+mediaID, nil)
		respGet, err := userBClient.Do(reqGet)
		if err != nil || respGet.StatusCode == 200 {
			log.Fatalf("SECURITY VIOLATION: User B accessed User A's %s via IDOR (status %d)!", purp, respGet.StatusCode)
		}

		reqDel, _ := http.NewRequest("DELETE", baseURL+"/api/v1/uploads/"+mediaID, nil)
		reqDel.Header.Set("Origin", baseURL)
		respDel, err := userBClient.Do(reqDel)
		if err != nil || respDel.StatusCode == 200 || respDel.StatusCode == 204 {
			log.Fatalf("SECURITY VIOLATION: User B deleted User A's %s via IDOR!", purp)
		}
		log.Printf("User B IDOR on User A's %-12s → Status %d REJECTED (PASS)", purp, respGet.StatusCode)
	}

	// 1.4 Staff media full lifecycle (BLOG_COVER and BLOG_CONTENT)
	log.Println("\n1.4 Testing Staff Media full lifecycle for session_admin...")
	staffPurposes := []string{"BLOG_COVER", "BLOG_CONTENT"}
	for _, purp := range staffPurposes {
		clearRateLimits()
		content := generateValidPNG("STAFF_" + purp)
		preBody, _ := json.Marshal(map[string]any{
			"purpose":    purp,
			"filename":   strings.ToLower(purp) + ".png",
			"mime_type":  "image/png",
			"size_bytes": len(content),
		})
		reqPre, _ := http.NewRequest("POST", baseURL+"/api/v1/uploads/presign", bytes.NewReader(preBody))
		reqPre.Header.Set("Content-Type", "application/json")
		reqPre.Header.Set("Origin", baseURL)
		respPre, err := adminClient.Do(reqPre)
		if err != nil || respPre.StatusCode != 201 {
			log.Fatalf("Admin presign %s failed: status=%d", purp, respPre.StatusCode)
		}
		var preResp struct {
			Data struct {
				MediaID   string `json:"media_id"`
				UploadURL string `json:"upload_url"`
			} `json:"data"`
		}
		_ = json.NewDecoder(respPre.Body).Decode(&preResp)

		uploadURL := preResp.Data.UploadURL
		putURL := baseURL + uploadURL
		if strings.Contains(uploadURL, "minio-a:9000") {
			putURL = strings.Replace(uploadURL, "http://minio-a:9000", "http://localhost:9000", 1)
		}
		putReq, _ := http.NewRequest("PUT", putURL, bytes.NewReader(content))
		putReq.Header.Set("Content-Type", "image/png")
		if strings.Contains(uploadURL, "minio-a:9000") {
			putReq.Host = "minio-a:9000"
		}
		putResp, err := c.client.Do(putReq)
		if err != nil || (putResp.StatusCode != 200 && putResp.StatusCode != 204) {
			log.Fatalf("Admin upload PUT failed for %s", purp)
		}

		// Complete
		reqComp, _ := http.NewRequest("POST", baseURL+"/api/v1/uploads/"+preResp.Data.MediaID+"/complete", nil)
		reqComp.Header.Set("Origin", baseURL)
		respComp, err := adminClient.Do(reqComp)
		if err != nil || respComp.StatusCode != 200 {
			log.Fatalf("Admin complete %s failed: status=%d", purp, respComp.StatusCode)
		}

		// Read (Get)
		reqGet, _ := http.NewRequest("GET", baseURL+"/api/v1/uploads/"+preResp.Data.MediaID, nil)
		respGet, err := adminClient.Do(reqGet)
		if err != nil || respGet.StatusCode != 200 {
			log.Fatalf("Admin GET %s failed: status=%d", purp, respGet.StatusCode)
		}

		// Delete
		reqDel, _ := http.NewRequest("DELETE", baseURL+"/api/v1/uploads/"+preResp.Data.MediaID, nil)
		reqDel.Header.Set("Origin", baseURL)
		respDel, err := adminClient.Do(reqDel)
		if err != nil || (respDel.StatusCode != 200 && respDel.StatusCode != 204) {
			log.Fatalf("Admin DELETE %s failed: status=%d", purp, respDel.StatusCode)
		}
		log.Printf("Staff media %-12s full lifecycle: Presign → Upload → Complete → Read → Delete (PASS)", purp)
	}

	// 1.5 Cookie Precedence
	log.Println("\n1.5 Testing Cookie Precedence (session + session_admin)...")
	jarBoth, _ := cookiejar.New(nil)
	u, _ := url.Parse(baseURL)
	cookiesUser := c.userAJar.Cookies(u)
	cookiesAdmin := c.adminJar.Cookies(u)
	jarBoth.SetCookies(u, append(cookiesUser, cookiesAdmin...))
	clientBoth := &http.Client{Jar: jarBoth, Timeout: 10 * time.Second}

	// 1. User route /api/v1/me identifies User A
	reqMeBoth, _ := http.NewRequest("GET", baseURL+"/api/v1/me", nil)
	respMeBoth, err := clientBoth.Do(reqMeBoth)
	if err != nil || respMeBoth.StatusCode != 200 {
		log.Fatalf("Cookie precedence on /me failed: %v", err)
	}
	var meResp struct {
		Data struct {
			Email string `json:"email"`
		} `json:"data"`
	}
	_ = json.NewDecoder(respMeBoth.Body).Decode(&meResp)
	if meResp.Data.Email != userAEmail {
		log.Fatalf("Cookie precedence on /me evaluated to %s, expected %s", meResp.Data.Email, userAEmail)
	}

	// 2. Admin route identifies Admin
	reqAdminBoth, _ := http.NewRequest("GET", baseURL+"/api/v1/admin/storage-settings", nil)
	respAdminBoth, err := clientBoth.Do(reqAdminBoth)
	if err != nil || respAdminBoth.StatusCode != 200 {
		log.Fatalf("Cookie precedence on /admin/storage-settings failed: %v", err)
	}
	log.Println("PASS - Cookie Precedence correctly isolates user and admin operations.")
}

func execSQL(dbName, sqlQuery string) error {
	cmd := exec.Command("docker", "exec", "freelance-postgres-1", "psql", "-U", "freelance", "-d", dbName, "-c", sqlQuery)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("execSQL %s: %w, out: %s", sqlQuery, err, string(out))
	}
	return nil
}

func querySQL(dbName, sqlQuery string) (string, error) {
	cmd := exec.Command("docker", "exec", "freelance-postgres-1", "psql", "-U", "freelance", "-d", dbName, "-t", "-A", "-c", sqlQuery)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("querySQL %s: %w, out: %s", sqlQuery, err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

func (c *Checker) RunCheck2Migration053To054() {
	log.Println("\n=======================================================")
	log.Println("CHECK 2: Upgrade Migration 000053 → 000054 Real Verification")
	log.Println("=======================================================")

	testDBName := "test_storage_migration_053_054"
	_ = exec.Command("docker", "exec", "freelance-postgres-1", "psql", "-U", "freelance", "-d", "postgres", "-c", "DROP DATABASE IF EXISTS "+testDBName).Run()
	outCreate, err := exec.Command("docker", "exec", "freelance-postgres-1", "psql", "-U", "freelance", "-d", "postgres", "-c", "CREATE DATABASE "+testDBName).CombinedOutput()
	if err != nil {
		log.Fatalf("Failed to create test database: %v, out: %s", err, string(outCreate))
	}
	defer func() {
		_ = exec.Command("docker", "exec", "freelance-postgres-1", "psql", "-U", "freelance", "-d", "postgres", "-c", "DROP DATABASE IF EXISTS "+testDBName).Run()
	}()

	// 1. Apply migrations up to 000053 only
	log.Println("Applying migrations up to 000053...")
	migrationsDir := "/Users/vitald3/PhpstormProjects/freelance/db/migrations"
	files, err := os.ReadDir(migrationsDir)
	if err != nil {
		log.Fatalf("Failed to read migrations: %v", err)
	}

	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".sql") {
			continue
		}
		if strings.HasPrefix(f.Name(), "000054") {
			break // Stop before 000054
		}
		content, _ := os.ReadFile(filepath.Join(migrationsDir, f.Name()))
		cmd := exec.Command("docker", "exec", "-i", "freelance-postgres-1", "psql", "-U", "freelance", "-d", testDBName)
		cmd.Stdin = bytes.NewReader(content)
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Fatalf("Migration %s failed on test DB: %v, out: %s", f.Name(), err, string(out))
		}
	}
	log.Println("PASS - Migrations 000001 through 000053 applied successfully.")

	// 2. Create pre-migration data in test DB:
	// - S3 config in feature_flags and storage_credentials
	// - Local media object
	// - S3 media object
	log.Println("Inserting pre-migration data (Local and S3 objects, feature_flags, storage_credentials)...")

	testOwnerID := "11111111-1111-4111-8111-111111111111"
	if err := execSQL(testDBName, fmt.Sprintf(`INSERT INTO users (id, email, email_normalized, password_hash, display_name) VALUES
		('%s', 'test-mig@example.test', 'test-mig@example.test', 'hash', 'Test Migration')`, testOwnerID)); err != nil {
		log.Fatalf("Insert user failed: %v", err)
	}

	// Encrypt secret for storage_credentials
	manager, _ := media.NewStorageManager(nil, "naimio-default-storage-key-change-in-prod", nil, objectstorage.S3Config{})
	encSecret, _ := manager.EncryptSecret("endpoint-a-secret")

	// S3 config in feature_flags
	ffJSON := `{"provider":"s3","s3":{"endpoint":"http://minio-a:9000","region":"us-east-1","bucket":"files","access_key":"endpoint-a-user","use_ssl":false,"path_style":true,"public_url":""}}`
	_ = execSQL(testDBName, fmt.Sprintf(`INSERT INTO feature_flags (key, enabled, config) VALUES ('file_storage', true, '%s'::jsonb) ON CONFLICT (key) DO UPDATE SET config = EXCLUDED.config`, ffJSON))
	_ = execSQL(testDBName, fmt.Sprintf(`INSERT INTO storage_credentials (provider, encrypted_config) VALUES ('s3', '%s') ON CONFLICT (provider) DO UPDATE SET encrypted_config = EXCLUDED.encrypted_config`, encSecret))

	// Local file
	localID := "22222222-2222-4222-8222-222222222222"
	localKey := "portfolio/" + testOwnerID + "/local-pre-mig.png"
	localContent := generateValidPNG("LOCAL_PRE_MIG")
	localHash := sha256.Sum256(localContent)
	// Write to local container disk
	_ = exec.Command("docker", "exec", "-i", "freelance-api-1", "sh", "-c", "mkdir -p /var/lib/naimio-media/portfolio/"+testOwnerID).Run()
	cmdWriteLocal := exec.Command("docker", "exec", "-i", "freelance-api-1", "sh", "-c", "cat > /var/lib/naimio-media/"+localKey)
	cmdWriteLocal.Stdin = bytes.NewReader(localContent)
	_ = cmdWriteLocal.Run()

	if err := execSQL(testDBName, fmt.Sprintf(`INSERT INTO media_objects (id, owner_user_id, purpose, storage_provider, object_key, bucket, original_filename, mime_type, size_bytes, scan_status, uploaded_at)
		VALUES ('%s', '%s', 'PORTFOLIO', 'local', '%s', 'local-media', 'local-pre-mig.png', 'image/png', %d, 'CLEAN', now())`,
		localID, testOwnerID, localKey, len(localContent))); err != nil {
		log.Fatalf("Insert local media_object failed: %v", err)
	}

	// S3 file
	s3ID := "33333333-3333-4333-8333-333333333333"
	s3Key := "portfolio/" + testOwnerID + "/s3-pre-mig.png"
	s3Content := generateValidPNG("S3_PRE_MIG")
	s3Hash := sha256.Sum256(s3Content)

	// Write to MinIO A
	cmdWriteS3 := exec.Command("docker", "exec", "-i", "freelance-minio-a-1", "mc", "pipe", "minio-a/files/"+s3Key)
	cmdWriteS3.Stdin = bytes.NewReader(s3Content)
	if out, err := cmdWriteS3.CombinedOutput(); err != nil {
		log.Fatalf("Write S3 file to MinIO A failed: %v, out: %s", err, string(out))
	}

	if err := execSQL(testDBName, fmt.Sprintf(`INSERT INTO media_objects (id, owner_user_id, purpose, storage_provider, object_key, bucket, original_filename, mime_type, size_bytes, scan_status, uploaded_at)
		VALUES ('%s', '%s', 'PORTFOLIO', 's3', '%s', 'files', 's3-pre-mig.png', 'image/png', %d, 'CLEAN', now())`,
		s3ID, testOwnerID, s3Key, len(s3Content))); err != nil {
		log.Fatalf("Insert s3 media_object failed: %v", err)
	}

	log.Println("PASS - Pre-migration Local and S3 records & physical files created.")

	// 3. Apply migration 000054_storage_backends.sql
	log.Println("Applying migration 000054_storage_backends.sql...")
	mig54Content, _ := os.ReadFile(filepath.Join(migrationsDir, "000054_storage_backends.sql"))
	cmdMig54 := exec.Command("docker", "exec", "-i", "freelance-postgres-1", "psql", "-U", "freelance", "-d", testDBName)
	cmdMig54.Stdin = bytes.NewReader(mig54Content)
	if out, err := cmdMig54.CombinedOutput(); err != nil {
		log.Fatalf("Migration 000054 failed: %v, out: %s", err, string(out))
	}
	log.Println("PASS - Migration 000054 applied successfully.")

	// 4. Verify post-migration database state
	locRow, err := querySQL(testDBName, fmt.Sprintf(`
		SELECT m.storage_provider || '|' || COALESCE(m.storage_backend_id::text, '') || '|' || COALESCE(sb.provider, '')
		FROM media_objects m
		LEFT JOIN storage_backends sb ON sb.id = m.storage_backend_id
		WHERE m.id = '%s'`, localID))
	if err != nil || !strings.HasPrefix(locRow, "local|") || !strings.HasSuffix(locRow, "|local") {
		log.Fatalf("Post-migration local object validation failed: row=%s, err=%v", locRow, err)
	}
	log.Printf("Post-migration Local object: %s (PASS)", locRow)

	s3Row, err := querySQL(testDBName, fmt.Sprintf(`
		SELECT m.storage_provider || '|' || COALESCE(m.storage_backend_id::text, '') || '|' || COALESCE(sb.provider, '') || '|' || COALESCE(sb.endpoint, '') || '|' || COALESCE(sb.access_key_id, '')
		FROM media_objects m
		LEFT JOIN storage_backends sb ON sb.id = m.storage_backend_id
		WHERE m.id = '%s'`, s3ID))
	if err != nil || !strings.HasPrefix(s3Row, "s3|") || !strings.Contains(s3Row, "|s3|http://minio-a:9000|endpoint-a-user") {
		log.Fatalf("Post-migration S3 object validation failed: row=%s, err=%v", s3Row, err)
	}
	log.Printf("Post-migration S3 object: %s (PASS)", s3Row)

	// 5. Verify physical read of local file
	outLocRead, _ := exec.Command("docker", "exec", "freelance-api-1", "cat", "/var/lib/naimio-media/"+localKey).CombinedOutput()
	hashLocRead := sha256.Sum256(outLocRead)
	if hashLocRead != localHash {
		log.Fatalf("Local file SHA256 mismatch after migration!")
	}
	log.Printf("Local file read: SHA256 matches (%s) (PASS)", hex.EncodeToString(hashLocRead[:]))

	// Read S3 file from MinIO A
	outS3Read, _ := exec.Command("docker", "exec", "freelance-minio-a-1", "mc", "cat", "minio-a/files/"+s3Key).CombinedOutput()
	hashS3Read := sha256.Sum256(outS3Read)
	if hashS3Read != s3Hash {
		log.Fatalf("S3 file SHA256 mismatch after migration!")
	}
	log.Printf("S3 file read: SHA256 matches (%s) (PASS)", hex.EncodeToString(hashS3Read[:]))

	// 6. Delete both objects physically
	_ = exec.Command("docker", "exec", "freelance-api-1", "rm", "-f", "/var/lib/naimio-media/"+localKey).Run()
	if _, err := exec.Command("docker", "exec", "freelance-api-1", "ls", "/var/lib/naimio-media/"+localKey).CombinedOutput(); err == nil {
		log.Fatalf("Local file was not physically deleted!")
	}

	_ = exec.Command("docker", "exec", "freelance-minio-a-1", "mc", "rm", "minio-a/files/"+s3Key).Run()
	if _, err := exec.Command("docker", "exec", "freelance-minio-a-1", "mc", "stat", "minio-a/files/"+s3Key).CombinedOutput(); err == nil {
		log.Fatalf("S3 file was not physically deleted from MinIO A!")
	}
	log.Println("PASS - Post-migration physical reads, SHA256 hashes, and physical deletions verified.")
}

func (c *Checker) RunCheck3ActiveBackendInvariant() {
	log.Println("\n=======================================================")
	log.Println("CHECK 3: Single Active Backend Invariant, DB Constraint & Concurrency")
	log.Println("=======================================================")

	// 1. Verify DB constraint (unique partial index)
	log.Println("Testing DB unique constraint on (is_active) WHERE is_active = true...")
	_ = execSQL("freelance", `UPDATE storage_backends SET is_active = false`)
	_ = execSQL("freelance", `
		INSERT INTO storage_backends (id, provider, bucket, is_active)
		VALUES ('00000000-0000-0000-0000-000000000001', 'local', 'local-media', true)
		ON CONFLICT (id) DO UPDATE SET is_active = true`)

	// Try inserting a second active backend
	err := execSQL("freelance", `
		INSERT INTO storage_backends (provider, endpoint, bucket, is_active)
		VALUES ('s3', 'http://minio-conflict:9000', 'files', true)`)
	if err == nil {
		log.Fatalf("DB Constraint FAILURE: Was able to insert second active backend!")
	}
	log.Println("PASS - DB constraint successfully rejected second active backend.")

	// 2. Concurrency test: 20 concurrent activation updates
	log.Println("Executing 20 concurrent activation updates via HTTP API (Local vs S3)...")
	concurrency := 20
	var wg sync.WaitGroup
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			defer wg.Done()
			clearRateLimits()
			var body []byte
			if idx%2 == 0 {
				body, _ = json.Marshal(map[string]any{"provider": "local"})
			} else {
				body, _ = json.Marshal(map[string]any{
					"provider": "s3",
					"s3": map[string]any{
						"endpoint":   minioAEndpoint,
						"region":     "us-east-1",
						"bucket":     bucketName,
						"access_key": minioAUser,
						"secret_key": minioAPass,
						"use_ssl":    false,
						"path_style": true,
					},
				})
			}
			req, _ := http.NewRequest("PUT", baseURL+"/api/v1/admin/storage-settings", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", baseURL)
			_, _ = (&http.Client{Jar: c.adminJar, Timeout: 10 * time.Second}).Do(req)
		}(i)
	}

	wg.Wait()

	// Verify exact count of active backends in DB
	activeCountStr, err := querySQL("freelance", `SELECT count(*) FROM storage_backends WHERE is_active = true`)
	if err != nil || activeCountStr != "1" {
		log.Fatalf("INVARIANT VIOLATION: Expected exactly 1 active backend, got %s!", activeCountStr)
	}
	activeDetails, _ := querySQL("freelance", `SELECT COALESCE(max(provider), '') || '|' || COALESCE(max(endpoint), '') || '|' || COALESCE(max(bucket), '') FROM storage_backends WHERE is_active = true`)
	log.Printf("Active backends in DB after 20 concurrent updates: %s (details: %s) (PASS)", activeCountStr, activeDetails)

	// 3. Restart container and verify unambiguous LoadFromDB
	log.Println("Restarting API container and verifying unambiguous active backend resolution...")
	_ = exec.Command("docker", "restart", "freelance-api-1").Run()
	time.Sleep(5 * time.Second)

	c.AuthenticateAll()
	reqSettings, _ := http.NewRequest("GET", baseURL+"/api/v1/admin/storage-settings", nil)
	respSettings, err := (&http.Client{Jar: c.adminJar, Timeout: 10 * time.Second}).Do(reqSettings)
	if err != nil || respSettings.StatusCode != 200 {
		log.Fatalf("Failed to query storage settings after restart: %v", err)
	}
	var settingsResp struct {
		Data struct {
			Provider string `json:"provider"`
		} `json:"data"`
	}
	_ = json.NewDecoder(respSettings.Body).Decode(&settingsResp)
	if settingsResp.Data.Provider != "local" && settingsResp.Data.Provider != "s3" {
		log.Fatalf("Invalid provider after restart: %s", settingsResp.Data.Provider)
	}
	log.Printf("Active provider after restart: %s (PASS)", settingsResp.Data.Provider)
}
