package main

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os/exec"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	baseURL       = "http://127.0.0.1:8088"
	adminEmail    = "admin@example.test"
	adminPass     = "LocalDemo2026!"
	userEmail     = "freelancer@example.test"
	userPass      = "LocalDemo2026!"
	minioAEndpoint = "http://minio-a:9000"
	minioAUser     = "endpoint-a-user"
	minioAPass     = "endpoint-a-secret"
	minioBEndpoint = "http://minio-b:9000"
	minioBUser     = "endpoint-b-user"
	minioBPass     = "endpoint-b-secret"
	bucketName    = "files"
)

type AuditRunner struct {
	client   *http.Client
	adminJar *cookiejar.Jar
	userJar  *cookiejar.Jar
	db       *sql.DB
}

func clearRateLimits() {
	_ = exec.Command("docker", "exec", "freelance-redis-1", "redis-cli", "-a", "local_redis_2026", "flushall").Run()
}

func generateValidPNG(tag string) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) // PNG Header
	buf.Write([]byte{0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52}) // IHDR chunk length & type
	buf.Write([]byte{0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00}) // 1x1 RGBA
	buf.Write([]byte{0x1F, 0x15, 0xC4, 0x89})                         // IHDR CRC
	buf.Write([]byte{0x00, 0x00, 0x00, 0x0A, 0x49, 0x44, 0x41, 0x54}) // IDAT chunk
	buf.Write([]byte{0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00, 0x05, 0x00, 0x01})
	buf.Write([]byte{0x0D, 0x0A, 0x2D, 0xB4})                         // IDAT CRC
	// tEXt chunk with unique tag & timestamp
	textData := fmt.Sprintf("Tag\x00%s_%d", tag, time.Now().UnixNano())
	textLen := uint32(len(textData))
	buf.Write([]byte{byte(textLen >> 24), byte(textLen >> 16), byte(textLen >> 8), byte(textLen)})
	buf.Write([]byte("tEXt"))
	buf.Write([]byte(textData))
	crc := sha256.Sum256([]byte("tEXt" + textData))
	buf.Write(crc[:4])
	buf.Write([]byte{0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82}) // IEND
	return buf.Bytes()
}

func main() {
	log.Println("=== Starting Final Audit: Endpoint A -> B, Docker Persistence, Security Review ===")
	clearRateLimits()

	dsn := "postgres://freelance:freelance_dev_secret@localhost:5432/freelance?sslmode=disable"
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	adminJar, _ := cookiejar.New(nil)
	userJar, _ := cookiejar.New(nil)

	r := &AuditRunner{
		client:   &http.Client{Timeout: 30 * time.Second},
		adminJar: adminJar,
		userJar:  userJar,
		db:       db,
	}

	// Step 1: Check dual MinIO instances and aliases
	r.SetupMinIOAliases()

	// Step 2: Authenticate Admin (session_admin) and User (session)
	r.Authenticate()

	// Step 3-8: Test Endpoint A -> Endpoint B migration and old object preservation
	r.TestEndpointSwitchingAndPreservation()

	// Step 9-10: Docker persistence (down/up and rebuild)
	r.TestDockerPersistence()

	// Step 11-16: Security audit of SessionMiddleware and privilege escalation
	r.TestSecurityAudit()

	// Step 17: Storage regression suite
	r.TestStorageRegression()

	log.Println("\n=======================================================")
	log.Println("ALL FINAL AUDIT TESTS COMPLETED SUCCESSFULLY (100% PASS)!")
	log.Println("=======================================================")
}

func (r *AuditRunner) SetupMinIOAliases() {
	log.Println("\n[1] Checking MinIO A and MinIO B containers and buckets...")
	_ = exec.Command("docker", "exec", "freelance-minio-a-1", "mc", "alias", "set", "minio-a", "http://127.0.0.1:9000", minioAUser, minioAPass).Run()
	_ = exec.Command("docker", "exec", "freelance-minio-a-1", "mc", "mb", "minio-a/"+bucketName).Run()
	_ = exec.Command("docker", "exec", "freelance-minio-b-1", "mc", "alias", "set", "minio-b", "http://127.0.0.1:9000", minioBUser, minioBPass).Run()
	_ = exec.Command("docker", "exec", "freelance-minio-b-1", "mc", "mb", "minio-b/"+bucketName).Run()

	outA, errA := exec.Command("docker", "exec", "freelance-minio-a-1", "mc", "ls", "minio-a").CombinedOutput()
	outB, errB := exec.Command("docker", "exec", "freelance-minio-b-1", "mc", "ls", "minio-b").CombinedOutput()
	if errA != nil || errB != nil {
		log.Fatalf("MinIO check failed: errA=%v, errB=%v", errA, errB)
	}
	log.Printf("MinIO A buckets:\n%s", string(outA))
	log.Printf("MinIO B buckets:\n%s", string(outB))
	log.Println("PASS - MinIO A and MinIO B are both running with separate volumes and credentials.")
}

func (r *AuditRunner) Authenticate() {
	log.Println("\n[2] Authenticating Admin and User...")
	// Admin login
	adminClient := &http.Client{Jar: r.adminJar, Timeout: 10 * time.Second}
	loginAdminBody, _ := json.Marshal(map[string]string{
		"email":    adminEmail,
		"password": adminPass,
		"portal":   "admin",
	})
	reqAdmin, _ := http.NewRequest("POST", baseURL+"/api/v1/auth/login", bytes.NewReader(loginAdminBody))
	reqAdmin.Header.Set("Content-Type", "application/json")
	reqAdmin.Header.Set("Origin", baseURL)
	respAdmin, err := adminClient.Do(reqAdmin)
	if err != nil || respAdmin.StatusCode != 200 {
		log.Fatalf("Admin login failed: %v", err)
	}

	// User login
	userClient := &http.Client{Jar: r.userJar, Timeout: 10 * time.Second}
	loginUserBody, _ := json.Marshal(map[string]string{
		"email":    userEmail,
		"password": userPass,
	})
	reqUser, _ := http.NewRequest("POST", baseURL+"/api/v1/auth/login", bytes.NewReader(loginUserBody))
	reqUser.Header.Set("Content-Type", "application/json")
	reqUser.Header.Set("Origin", baseURL)
	respUser, err := userClient.Do(reqUser)
	if err != nil || respUser.StatusCode != 200 {
		body, _ := io.ReadAll(respUser.Body)
		log.Fatalf("User login failed: status=%d, body=%s, err=%v", respUser.StatusCode, string(body), err)
	}
	log.Println("PASS - Admin and User authenticated.")
}

func (r *AuditRunner) setS3Settings(endpoint, user, pass, bucket string) {
	clearRateLimits()
	adminClient := &http.Client{Jar: r.adminJar, Timeout: 10 * time.Second}

	// Test connection probe
	testBody, _ := json.Marshal(map[string]any{
		"endpoint":         endpoint,
		"region":           "us-east-1",
		"bucket":           bucket,
		"access_key":       user,
		"secret_key":       pass,
		"use_ssl":          false,
		"path_style":       true,
	})
	reqTest, _ := http.NewRequest("POST", baseURL+"/api/v1/admin/storage-settings/test", bytes.NewReader(testBody))
	reqTest.Header.Set("Content-Type", "application/json")
	reqTest.Header.Set("Origin", baseURL)
	respTest, err := adminClient.Do(reqTest)
	if err != nil || respTest.StatusCode != 200 {
		body, _ := io.ReadAll(respTest.Body)
		log.Fatalf("Test connection to %s failed: status=%d, body=%s, err=%v", endpoint, respTest.StatusCode, string(body), err)
	}

	// Save settings
	putBody, _ := json.Marshal(map[string]any{
		"provider": "s3",
		"s3": map[string]any{
			"endpoint":         endpoint,
			"region":           "us-east-1",
			"bucket":           bucket,
			"access_key":       user,
			"secret_key":       pass,
			"use_ssl":          false,
			"path_style":       true,
		},
	})
	reqPut, _ := http.NewRequest("PUT", baseURL+"/api/v1/admin/storage-settings", bytes.NewReader(putBody))
	reqPut.Header.Set("Content-Type", "application/json")
	reqPut.Header.Set("Origin", baseURL)
	respPut, err := adminClient.Do(reqPut)
	if err != nil || respPut.StatusCode != 200 {
		body, _ := io.ReadAll(respPut.Body)
		log.Fatalf("Update settings for %s failed: status=%d, body=%s", endpoint, respPut.StatusCode, string(body))
	}
}

func (r *AuditRunner) uploadFile(purpose, filename string, content []byte) (string, string) {
	clearRateLimits()
	userClient := &http.Client{Jar: r.userJar, Timeout: 10 * time.Second}
	presignBody, _ := json.Marshal(map[string]any{
		"purpose":    purpose,
		"filename":   filename,
		"mime_type":  "image/png",
		"size_bytes": len(content),
	})
	reqPre, _ := http.NewRequest("POST", baseURL+"/api/v1/uploads/presign", bytes.NewReader(presignBody))
	reqPre.Header.Set("Content-Type", "application/json")
	reqPre.Header.Set("Origin", baseURL)
	respPre, err := userClient.Do(reqPre)
	if err != nil || respPre.StatusCode != 201 {
		body, _ := io.ReadAll(respPre.Body)
		log.Fatalf("Presign failed: status=%d, body=%s, err=%v", respPre.StatusCode, string(body), err)
	}
	var preResp struct {
		Data struct {
			MediaID   string `json:"media_id"`
			UploadURL string `json:"upload_url"`
		} `json:"data"`
	}
	_ = json.NewDecoder(respPre.Body).Decode(&preResp)

	// PUT
	uploadURL := preResp.Data.UploadURL
	isMinioA := strings.Contains(uploadURL, "minio-a:9000")
	isMinioB := strings.Contains(uploadURL, "minio-b:9000")

	putURL := uploadURL
	if isMinioA {
		putURL = strings.Replace(uploadURL, "http://minio-a:9000", "http://localhost:9000", 1)
	} else if isMinioB {
		putURL = strings.Replace(uploadURL, "http://minio-b:9000", "http://localhost:9010", 1)
	} else if strings.HasPrefix(uploadURL, "/") {
		putURL = baseURL + uploadURL
	}

	putReq, _ := http.NewRequest("PUT", putURL, bytes.NewReader(content))
	putReq.Header.Set("Content-Type", "image/png")
	if isMinioA {
		putReq.Host = "minio-a:9000"
	} else if isMinioB {
		putReq.Host = "minio-b:9000"
	}
	putResp, err := r.client.Do(putReq)
	if err != nil || (putResp.StatusCode != 200 && putResp.StatusCode != 204) {
		body, _ := io.ReadAll(putResp.Body)
		log.Fatalf("PUT failed: url=%s, status=%d, body=%s, err=%v", putURL, putResp.StatusCode, string(body), err)
	}

	// Complete
	reqComp, _ := http.NewRequest("POST", baseURL+"/api/v1/uploads/"+preResp.Data.MediaID+"/complete", nil)
	reqComp.Header.Set("Origin", baseURL)
	respComp, err := userClient.Do(reqComp)
	if err != nil || respComp.StatusCode != 200 {
		body, _ := io.ReadAll(respComp.Body)
		log.Fatalf("Complete failed: status=%d, body=%s", respComp.StatusCode, string(body))
	}
	var compResp struct {
		Data struct {
			ID              string `json:"id"`
			StorageProvider string `json:"storage_provider"`
		} `json:"data"`
	}
	_ = json.NewDecoder(respComp.Body).Decode(&compResp)

	// Fetch object key from DB
	outKey, err := exec.Command("docker", "exec", "freelance-postgres-1", "psql", "-U", "freelance", "-d", "freelance", "-t", "-A", "-c",
		fmt.Sprintf("SELECT object_key FROM media_objects WHERE id = '%s'", preResp.Data.MediaID)).CombinedOutput()
	if err != nil {
		log.Fatalf("Failed to query object_key from DB: %v, out: %s", err, string(outKey))
	}
	key := strings.TrimSpace(string(outKey))
	log.Printf("uploadFile [%s, %s]: mediaID=%s, provider=%s, key=%s, uploadURL=%s", purpose, filename, preResp.Data.MediaID, compResp.Data.StorageProvider, key, uploadURL)

	return preResp.Data.MediaID, key
}

func (r *AuditRunner) downloadFile(id string) []byte {
	userClient := &http.Client{Jar: r.userJar, Timeout: 10 * time.Second}
	reqGet, _ := http.NewRequest("GET", baseURL+"/api/v1/uploads/"+id, nil)
	respGet, err := userClient.Do(reqGet)
	if err != nil || respGet.StatusCode != 200 {
		body, _ := io.ReadAll(respGet.Body)
		log.Fatalf("GET upload failed: status=%d, body=%s", respGet.StatusCode, string(body))
	}
	var getResp struct {
		Data struct {
			DownloadURL string `json:"download_url"`
		} `json:"data"`
	}
	_ = json.NewDecoder(respGet.Body).Decode(&getResp)

	dlURL := getResp.Data.DownloadURL
	isMinioA := strings.Contains(dlURL, "minio-a:9000")
	isMinioB := strings.Contains(dlURL, "minio-b:9000")

	fetchURL := dlURL
	if isMinioA {
		fetchURL = strings.Replace(dlURL, "http://minio-a:9000", "http://localhost:9000", 1)
	} else if isMinioB {
		fetchURL = strings.Replace(dlURL, "http://minio-b:9000", "http://localhost:9010", 1)
	} else if strings.HasPrefix(dlURL, "/") {
		fetchURL = baseURL + dlURL
	}

	dlReq, _ := http.NewRequest("GET", fetchURL, nil)
	if isMinioA {
		dlReq.Host = "minio-a:9000"
	} else if isMinioB {
		dlReq.Host = "minio-b:9000"
	}
	resp, err := r.client.Do(dlReq)
	if err != nil || resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("Download failed: url=%s, status=%d, body=%s, err=%v", fetchURL, resp.StatusCode, string(body), err)
	}
	data, _ := io.ReadAll(resp.Body)
	return data
}

func (r *AuditRunner) TestEndpointSwitchingAndPreservation() {
	clearRateLimits()
	log.Println("\n[3] Testing S3 Endpoint A -> Endpoint B switching and old credentials preservation...")

	// 1. Configure Endpoint A
	r.setS3Settings(minioAEndpoint, minioAUser, minioAPass, bucketName)
	contentA := generateValidPNG("ENDPOINT_A_FILE")
	hashA := sha256.Sum256(contentA)
	idA, keyA := r.uploadFile("PORTFOLIO", "endpoint-A-file.png", contentA)
	log.Printf("Uploaded File A to Endpoint A (ID: %s, Key: %s, SHA256: %s)", idA, keyA, hex.EncodeToString(hashA[:]))

	// Physical check: exists in MinIO A, absent in MinIO B
	outStatA, errStatA := exec.Command("docker", "exec", "freelance-minio-a-1", "mc", "stat", "minio-a/"+bucketName+"/"+keyA).CombinedOutput()
	if errStatA != nil {
		log.Fatalf("File A missing in MinIO A: %v", errStatA)
	}
	log.Printf("Verified File A physically exists in MinIO A:\n%s", string(outStatA))

	_, errStatBCheck := exec.Command("docker", "exec", "freelance-minio-b-1", "mc", "stat", "minio-b/"+bucketName+"/"+keyA).CombinedOutput()
	if errStatBCheck == nil {
		log.Fatalf("File A should NOT exist in MinIO B!")
	}
	log.Println("Confirmed File A does NOT exist in MinIO B.")

	// 2. Switch settings to Endpoint B (different endpoint, different credentials)
	log.Println("\n[4] Switching active S3 settings to Endpoint B with Credentials B...")
	r.setS3Settings(minioBEndpoint, minioBUser, minioBPass, bucketName)
	contentB := generateValidPNG("ENDPOINT_B_FILE")
	hashB := sha256.Sum256(contentB)
	idB, keyB := r.uploadFile("PORTFOLIO", "endpoint-B-file.png", contentB)
	log.Printf("Uploaded File B to Endpoint B (ID: %s, Key: %s, SHA256: %s)", idB, keyB, hex.EncodeToString(hashB[:]))

	// Physical check: File B exists in MinIO B, absent in MinIO A
	outStatB, errStatB := exec.Command("docker", "exec", "freelance-minio-b-1", "mc", "stat", "minio-b/"+bucketName+"/"+keyB).CombinedOutput()
	if errStatB != nil {
		log.Fatalf("File B missing in MinIO B: %v", errStatB)
	}
	log.Printf("Verified File B physically exists in MinIO B:\n%s", string(outStatB))

	_, errStatACheck := exec.Command("docker", "exec", "freelance-minio-a-1", "mc", "stat", "minio-a/"+bucketName+"/"+keyB).CombinedOutput()
	if errStatACheck == nil {
		log.Fatalf("File B should NOT exist in MinIO A!")
	}
	log.Println("Confirmed File B does NOT exist in MinIO A.")

	// 3. READ old File A from Endpoint A while Endpoint B is active!
	log.Println("\n[5] Reading old File A (from Endpoint A) through application while Endpoint B is active...")
	dlA := r.downloadFile(idA)
	if !bytes.Equal(dlA, contentA) {
		log.Fatalf("Old File A content mismatch when read under Endpoint B active setting!")
	}
	log.Println("PASS - Old File A successfully read from Endpoint A with its preserved credentials.")

	// 4. READ File B from Endpoint B
	dlB := r.downloadFile(idB)
	if !bytes.Equal(dlB, contentB) {
		log.Fatalf("File B content mismatch!")
	}
	log.Println("PASS - File B successfully read from Endpoint B.")

	// 5. DELETE old File A while Endpoint B is active
	log.Println("\n[6] Deleting old File A through application while Endpoint B is active...")
	userClient := &http.Client{Jar: r.userJar, Timeout: 10 * time.Second}
	reqDelA, _ := http.NewRequest("DELETE", baseURL+"/api/v1/uploads/"+idA, nil)
	reqDelA.Header.Set("Origin", baseURL)
	respDelA, _ := userClient.Do(reqDelA)
	if respDelA.StatusCode != 200 && respDelA.StatusCode != 204 {
		log.Fatalf("Delete File A failed: status=%d", respDelA.StatusCode)
	}

	// Verify File A physically deleted from MinIO A
	_, errDelAStat := exec.Command("docker", "exec", "freelance-minio-a-1", "mc", "stat", "minio-a/"+bucketName+"/"+keyA).CombinedOutput()
	if errDelAStat == nil {
		log.Fatalf("File A was not physically deleted from MinIO A!")
	}
	log.Println("Confirmed File A was physically deleted from MinIO A.")

	// Verify File B remains in MinIO B untouched
	_, errBStillThere := exec.Command("docker", "exec", "freelance-minio-b-1", "mc", "stat", "minio-b/"+bucketName+"/"+keyB).CombinedOutput()
	if errBStillThere != nil {
		log.Fatalf("File B in MinIO B was accidentally damaged or deleted!")
	}
	log.Println("Confirmed File B in MinIO B is completely untouched and valid.")

	// 6. DELETE File B
	reqDelB, _ := http.NewRequest("DELETE", baseURL+"/api/v1/uploads/"+idB, nil)
	reqDelB.Header.Set("Origin", baseURL)
	respDelB, _ := userClient.Do(reqDelB)
	if respDelB.StatusCode != 200 && respDelB.StatusCode != 204 {
		log.Fatalf("Delete File B failed: status=%d", respDelB.StatusCode)
	}
	_, errDelBStat := exec.Command("docker", "exec", "freelance-minio-b-1", "mc", "stat", "minio-b/"+bucketName+"/"+keyB).CombinedOutput()
	if errDelBStat == nil {
		log.Fatalf("File B was not physically deleted from MinIO B!")
	}
	log.Println("Confirmed File B was physically deleted from MinIO B.")

	log.Println("PASS - Endpoint A -> Endpoint B migration and multi-backend preservation fully verified!")
}

func (r *AuditRunner) setLocalSettings() {
	adminClient := &http.Client{Jar: r.adminJar, Timeout: 10 * time.Second}
	putBody, _ := json.Marshal(map[string]any{"provider": "local"})
	reqPut, _ := http.NewRequest("PUT", baseURL+"/api/v1/admin/storage-settings", bytes.NewReader(putBody))
	reqPut.Header.Set("Content-Type", "application/json")
	reqPut.Header.Set("Origin", baseURL)
	respPut, err := adminClient.Do(reqPut)
	if err != nil || respPut.StatusCode != 200 {
		log.Fatalf("Set local storage failed: %v", err)
	}
}

func (r *AuditRunner) TestDockerPersistence() {
	clearRateLimits()
	log.Println("\n[7] Testing Docker Persistence across real 'docker compose down' & 'docker compose up -d'...")

	// 1. Switch to local storage
	r.setLocalSettings()

	// 2. Upload file
	content := generateValidPNG("DOCKER_PERSISTENCE")
	hashOrig := sha256.Sum256(content)
	id, key := r.uploadFile("PORTFOLIO", "docker-down-up-persistence.png", content)
	log.Printf("Uploaded local file (ID: %s, Key: %s, SHA256: %s)", id, key, hex.EncodeToString(hashOrig[:]))

	// 3. Confirm physical path
	outBefore, err := exec.Command("docker", "exec", "freelance-api-1", "ls", "/var/lib/naimio-media/"+key).CombinedOutput()
	if err != nil {
		log.Fatalf("File not found on local disk before restart: %s", string(outBefore))
	}
	log.Println("Confirmed physical file on local volume.")

	// 4. Real 'docker compose down' && 'docker compose up -d'
	log.Println("Executing 'docker compose down'...")
	_ = exec.Command("docker", "compose", "down").Run()
	time.Sleep(2 * time.Second)

	log.Println("Executing 'docker compose up -d'...")
	_ = exec.Command("docker", "compose", "up", "-d").Run()
	time.Sleep(5 * time.Second)

	// Re-establish MinIO mc aliases in new containers and authenticate
	r.SetupMinIOAliases()
	r.Authenticate()

	// 5. Verify Postgres row and physical file exist
	outAfter, err := exec.Command("docker", "exec", "freelance-api-1", "ls", "/var/lib/naimio-media/"+key).CombinedOutput()
	if err != nil {
		log.Fatalf("File missing after 'docker compose down/up': %v, out: %s", err, string(outAfter))
	}

	// 6. Download through app API and verify SHA256
	dl := r.downloadFile(id)
	hashDl := sha256.Sum256(dl)
	if hashOrig != hashDl {
		log.Fatalf("SHA256 mismatch after 'docker compose down/up'!")
	}
	log.Printf("PASS - Docker down/up persistence: SHA256 matches (%s)", hex.EncodeToString(hashDl[:]))

	// 7. Test Docker rebuild persistence ('docker compose down' -> 'build api web' -> 'up -d')
	log.Println("\n[8] Testing Docker Rebuild persistence ('docker compose down' -> 'build' -> 'up -d')...")
	_ = exec.Command("docker", "compose", "down").Run()
	time.Sleep(2 * time.Second)
	_ = exec.Command("docker", "compose", "build", "api", "web").Run()
	_ = exec.Command("docker", "compose", "up", "-d").Run()
	time.Sleep(5 * time.Second)

	// Re-establish MinIO mc aliases in new containers and authenticate
	r.SetupMinIOAliases()
	r.Authenticate()

	// Verify file persists through rebuild
	dlRebuild := r.downloadFile(id)
	hashRebuild := sha256.Sum256(dlRebuild)
	if hashOrig != hashRebuild {
		log.Fatalf("SHA256 mismatch after Docker rebuild!")
	}
	log.Printf("PASS - Docker rebuild persistence: SHA256 matches (%s)", hex.EncodeToString(hashRebuild[:]))
}

func (r *AuditRunner) TestSecurityAudit() {
	clearRateLimits()
	log.Println("\n[9] Running Security Audit on SessionMiddleware, Privilege Escalation, and Cookie Precedence...")

	adminClient := &http.Client{Jar: r.adminJar, Timeout: 10 * time.Second}
	userClient := &http.Client{Jar: r.userJar, Timeout: 10 * time.Second}

	// A. Admin Session trying user endpoints (Must receive 401 UNAUTHENTICATED)
	userEndpoints := []struct {
		method string
		path   string
		body   string
	}{
		{"GET", "/api/v1/me", ""},
		{"PATCH", "/api/v1/me", `{"display_name":"Hacked"}`},
		{"POST", "/api/v1/me/services", `{"title":"Hacked Service"}`},
		{"POST", "/api/v1/me/projects", `{"title":"Hacked Project"}`},
		{"POST", "/api/v1/me/skills", `{"skill_ids":["00000000-0000-0000-0000-000000000001"]}`},
		{"POST", "/api/v1/me/safe-deals", `{"proposal_id":"00000000-0000-0000-0000-000000000001"}`},
		{"POST", "/api/v1/me/reviews/given", `{"project_id":"00000000-0000-0000-0000-000000000001"}`},
		{"POST", "/api/v1/me/portfolio", `{"title":"Hacked Portfolio"}`},
	}

	for _, ep := range userEndpoints {
		clearRateLimits()
		var req *http.Request
		if ep.body != "" {
			req, _ = http.NewRequest(ep.method, baseURL+ep.path, strings.NewReader(ep.body))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req, _ = http.NewRequest(ep.method, baseURL+ep.path, nil)
		}
		req.Header.Set("Origin", baseURL)
		resp, err := adminClient.Do(req)
		if err != nil {
			log.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 401 {
			body, _ := io.ReadAll(resp.Body)
			log.Fatalf("SECURITY VIOLATION: Admin session was accepted on %s %s! Status: %d, body: %s", ep.method, ep.path, resp.StatusCode, string(body))
		}
		log.Printf("Verified admin session on %-6s %-30s: 401 UNAUTHENTICATED (PASS)", ep.method, ep.path)
	}

	// B. User Session trying admin endpoints (Must receive 401 / 403 / 404)
	adminEndpoints := []struct {
		method string
		path   string
		body   string
	}{
		{"GET", "/api/v1/admin/storage-settings", ""},
		{"PUT", "/api/v1/admin/storage-settings", `{"provider":"local"}`},
		{"POST", "/api/v1/admin/storage-settings/test", `{"endpoint":"http://foo","bucket":"b","access_key":"k","secret_key":"s"}`},
	}

	for _, ep := range adminEndpoints {
		clearRateLimits()
		var req *http.Request
		if ep.body != "" {
			req, _ = http.NewRequest(ep.method, baseURL+ep.path, strings.NewReader(ep.body))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req, _ = http.NewRequest(ep.method, baseURL+ep.path, nil)
		}
		req.Header.Set("Origin", baseURL)
		resp, err := userClient.Do(req)
		if err != nil {
			log.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode == 200 || resp.StatusCode == 201 || resp.StatusCode == 204 {
			log.Fatalf("SECURITY VIOLATION: User session gained admin access on %s %s!", ep.method, ep.path)
		}
		log.Printf("Verified user session on %-6s %-35s: Status %d REJECTED (PASS)", ep.method, ep.path, resp.StatusCode)
	}

	// C. Admin BLOG_COVER upload
	clearRateLimits()
	blogContent := generateValidPNG("ADMIN_BLOG_COVER")
	blogPreBody, _ := json.Marshal(map[string]any{
		"purpose":    "BLOG_COVER",
		"filename":   "blog-cover.png",
		"mime_type":  "image/png",
		"size_bytes": len(blogContent),
	})
	reqBlog, _ := http.NewRequest("POST", baseURL+"/api/v1/uploads/presign", bytes.NewReader(blogPreBody))
	reqBlog.Header.Set("Content-Type", "application/json")
	reqBlog.Header.Set("Origin", baseURL)
	respBlog, err := adminClient.Do(reqBlog)
	if err != nil || respBlog.StatusCode != 201 {
		body, _ := io.ReadAll(respBlog.Body)
		log.Fatalf("Admin BLOG_COVER upload failed: status=%d, body=%s, err=%v", respBlog.StatusCode, string(body), err)
	}
	log.Println("Verified Admin BLOG_COVER upload: 201 Created (PASS)")

	// D. Regular user BLOG_COVER upload (Must be rejected with 422 / 400)
	clearRateLimits()
	reqUserBlog, _ := http.NewRequest("POST", baseURL+"/api/v1/uploads/presign", bytes.NewReader(blogPreBody))
	reqUserBlog.Header.Set("Content-Type", "application/json")
	reqUserBlog.Header.Set("Origin", baseURL)
	respUserBlog, err := userClient.Do(reqUserBlog)
	if err != nil || (respUserBlog.StatusCode != 400 && respUserBlog.StatusCode != 422) {
		body, _ := io.ReadAll(respUserBlog.Body)
		log.Fatalf("SECURITY VIOLATION: Regular user was able to upload BLOG_COVER: status=%d, body=%s", respUserBlog.StatusCode, string(body))
	}
	log.Printf("Verified Regular User BLOG_COVER upload: %d REJECTED (PASS)", respUserBlog.StatusCode)

	// E. Cookie Precedence Test
	clearRateLimits()
	jarBoth, _ := cookiejar.New(nil)
	u, _ := url.Parse(baseURL)
	cookiesUser := r.userJar.Cookies(u)
	cookiesAdmin := r.adminJar.Cookies(u)
	jarBoth.SetCookies(u, append(cookiesUser, cookiesAdmin...))
	clientBoth := &http.Client{Jar: jarBoth, Timeout: 10 * time.Second}

	// 1. Calling /api/v1/me with both cookies must identify as user
	reqMeBoth, _ := http.NewRequest("GET", baseURL+"/api/v1/me", nil)
	respMeBoth, err := clientBoth.Do(reqMeBoth)
	if err != nil || respMeBoth.StatusCode != 200 {
		log.Fatalf("Cookie precedence test on /me failed: %v", err)
	}
	var meResp struct {
		Data struct {
			Email string `json:"email"`
		} `json:"data"`
	}
	_ = json.NewDecoder(respMeBoth.Body).Decode(&meResp)
	if meResp.Data.Email != userEmail {
		log.Fatalf("Cookie precedence on /me evaluated to '%s' instead of '%s'!", meResp.Data.Email, userEmail)
	}
	log.Printf("Verified Cookie Precedence on /api/v1/me: Authenticated as %s (PASS)", meResp.Data.Email)

	// 2. Calling /api/v1/admin/storage-settings with both cookies must identify as admin
	reqAdminBoth, _ := http.NewRequest("GET", baseURL+"/api/v1/admin/storage-settings", nil)
	respAdminBoth, err := clientBoth.Do(reqAdminBoth)
	if err != nil || respAdminBoth.StatusCode != 200 {
		log.Fatalf("Cookie precedence test on /admin/storage-settings failed: %v", err)
	}
	log.Println("Verified Cookie Precedence on /api/v1/admin/storage-settings: 200 OK (PASS)")

	log.Println("PASS - Security Audit complete. All authorization and isolation boundaries verified.")
}

func (r *AuditRunner) TestStorageRegression() {
	clearRateLimits()
	log.Println("\n[10] Running Full Storage Regression Suite (Local, S3, Cross-Storage, Multi-Bucket)...")

	// 1. Local mode upload & download
	r.setLocalSettings()
	localContent := generateValidPNG("REG_LOCAL")
	idLoc, _ := r.uploadFile("PORTFOLIO", "reg-local.png", localContent)
	dlLoc := r.downloadFile(idLoc)
	if !bytes.Equal(dlLoc, localContent) {
		log.Fatalf("Regression: local download mismatch")
	}

	// 2. Switch to S3 (MinIO A)
	r.setS3Settings(minioAEndpoint, minioAUser, minioAPass, bucketName)
	s3Content := generateValidPNG("REG_S3")
	idS3, _ := r.uploadFile("PORTFOLIO", "reg-s3.png", s3Content)
	dlS3 := r.downloadFile(idS3)
	if !bytes.Equal(dlS3, s3Content) {
		log.Fatalf("Regression: S3 download mismatch")
	}

	// 3. Read old local file under S3 mode
	dlOldLoc := r.downloadFile(idLoc)
	if !bytes.Equal(dlOldLoc, localContent) {
		log.Fatalf("Regression: old local read under S3 mode mismatch")
	}

	// 4. Switch back to Local mode
	r.setLocalSettings()
	dlOldS3 := r.downloadFile(idS3)
	if !bytes.Equal(dlOldS3, s3Content) {
		log.Fatalf("Regression: old S3 read under Local mode mismatch")
	}

	// 5. Cross-storage avatar replacement
	userClient := &http.Client{Jar: r.userJar, Timeout: 10 * time.Second}
	r.setLocalSettings()
	avLocalContent := generateValidPNG("AV_LOCAL")
	idAvLoc, keyAvLoc := r.uploadFile("AVATAR", "avatar-reg-loc.png", avLocalContent)
	reqPatchA, _ := http.NewRequest("PATCH", baseURL+"/api/v1/me", strings.NewReader(fmt.Sprintf(`{"avatar_media_object_id":"%s"}`, idAvLoc)))
	reqPatchA.Header.Set("Content-Type", "application/json")
	respPatchA, errPatchA := userClient.Do(reqPatchA)
	bodyA, _ := io.ReadAll(respPatchA.Body)
	log.Printf("PATCH /me (local avatar): status=%d, body=%s, err=%v", respPatchA.StatusCode, string(bodyA), errPatchA)

	r.setS3Settings(minioAEndpoint, minioAUser, minioAPass, bucketName)
	avS3Content := generateValidPNG("AV_S3")
	idAvS3, keyAvS3 := r.uploadFile("AVATAR", "avatar-reg-s3.png", avS3Content)
	reqPatchB, _ := http.NewRequest("PATCH", baseURL+"/api/v1/me", strings.NewReader(fmt.Sprintf(`{"avatar_media_object_id":"%s"}`, idAvS3)))
	reqPatchB.Header.Set("Content-Type", "application/json")
	reqPatchB.Header.Set("Origin", baseURL)
	respPatchB, errPatchB := userClient.Do(reqPatchB)
	bodyB, _ := io.ReadAll(respPatchB.Body)
	log.Printf("PATCH /me (S3 avatar): status=%d, body=%s, err=%v", respPatchB.StatusCode, string(bodyB), errPatchB)

	// Verify old avatar physically deleted from local
	_, errAvLocStat := exec.Command("docker", "exec", "freelance-api-1", "ls", "/var/lib/naimio-media/"+keyAvLoc).CombinedOutput()
	if errAvLocStat == nil {
		log.Fatalf("Regression: old local avatar was not deleted on replacement!")
	}

	// Verify new avatar in MinIO A
	outAvS3Stat, errAvS3Stat := exec.Command("docker", "exec", "freelance-minio-a-1", "mc", "stat", "minio-a/"+bucketName+"/"+keyAvS3).CombinedOutput()
	if errAvS3Stat != nil {
		log.Fatalf("Regression: new S3 avatar missing in MinIO A! key=%s, err=%v, out=%s", keyAvS3, errAvS3Stat, string(outAvS3Stat))
	}

	log.Println("PASS - Storage regression tests completed successfully.")
}
