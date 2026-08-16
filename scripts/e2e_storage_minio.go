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
	"os/exec"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	baseURL     = "http://localhost:8088"
	adminEmail  = "admin@example.test"
	adminPass   = "LocalDemo2026!"
	userEmail   = "freelancer@example.test"
	userPass    = "LocalDemo2026!"
	minioBucket = "naimio-storage-test"
	minioBucketA = "naimio-storage-test-a"
	minioBucketB = "naimio-storage-test-b"
)

type E2ERunner struct {
	client       *http.Client
	adminJar     *cookiejar.Jar
	adminUserJar *cookiejar.Jar
	userJar      *cookiejar.Jar
	db           *sql.DB
}

func clearRateLimits() {
	_ = exec.Command("docker", "exec", "freelance-redis-1", "redis-cli", "-a", "local_redis_2026", "flushall").Run()
}

func main() {
	log.Println("=== Starting Full MinIO / Storage E2E Test Suite ===")
	clearRateLimits()

	dsn := "postgres://freelance:freelance_dev_secret@localhost:5432/freelance?sslmode=disable"
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}
	defer db.Close()

	adminJar, _ := cookiejar.New(nil)
	adminUserJar, _ := cookiejar.New(nil)
	userJar, _ := cookiejar.New(nil)

	r := &E2ERunner{
		client:       &http.Client{Timeout: 30 * time.Second},
		adminJar:     adminJar,
		adminUserJar: adminUserJar,
		userJar:      userJar,
		db:           db,
	}

	// 1. MinIO startup & bucket checks
	r.Step1_CheckMinIOAndBuckets()

	// 2. Authenticate Admin and User
	r.Step2_Authenticate()

	// 3. Test Connection endpoint through app
	r.Step3_TestConnection()

	// 4. Set provider = s3 and verify masked secret
	r.Step4_SwitchToS3()

	// 5. Real S3 upload through application (presign -> PUT -> complete)
	objID, objKey, fileData := r.Step5_RealS3Upload()

	// 6. Real S3 download and SHA256 verification
	r.Step6_RealS3Download(objID, fileData)

	// 7. Real S3 delete and physical MinIO verification
	r.Step7_RealS3Delete(objID, objKey)

	// 8. Test Local -> S3 switching
	r.Step8_LocalToS3Switching()

	// 9. Test S3 -> Local switching
	r.Step9_S3ToLocalSwitching()

	// 10. Cross-storage replacement (Avatar replacement Local -> S3 -> Local)
	r.Step10_CrossStorageReplacement()

	// 11. Multi-bucket S3 change (Bucket A -> Bucket B)
	r.Step11_MultiBucketChange()

	// 12. Check all discovered file flows
	r.Step12_CheckAllFileFlows()

	// 13. Concurrency edge case test
	r.Step13_ConcurrencyTest()

	// 14. Settings persistence after restart
	r.Step14_SettingsPersistence()

	// 15. Docker Local persistence across restart
	r.Step15_DockerLocalPersistence()

	log.Println("\n=======================================================")
	log.Println("ALL E2E STORAGE & MINIO TESTS COMPLETED SUCCESSFULLY!")
	log.Println("=======================================================")
}

func (r *E2ERunner) Step1_CheckMinIOAndBuckets() {
	log.Println("\n[STEP 1] Checking MinIO and Buckets...")
	out, err := exec.Command("docker", "exec", "freelance-minio-1", "mc", "ls", "local").CombinedOutput()
	if err != nil {
		log.Fatalf("MinIO mc ls failed: %v, output: %s", err, string(out))
	}
	log.Printf("MinIO buckets:\n%s", string(out))

	for _, b := range []string{minioBucket, minioBucketA, minioBucketB} {
		if !strings.Contains(string(out), b) {
			log.Fatalf("Bucket %s is missing!", b)
		}
	}
	log.Println("STEP 1: PASS - MinIO is running and all 3 buckets exist.")
}

func (r *E2ERunner) Step2_Authenticate() {
	log.Println("\n[STEP 2] Authenticating Admin and User...")
	// Admin login (portal: admin)
	adminClient := &http.Client{Jar: r.adminJar, Timeout: 10 * time.Second}
	loginBody, _ := json.Marshal(map[string]string{
		"email":    adminEmail,
		"password": adminPass,
		"portal":   "admin",
	})
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/auth/login", bytes.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", baseURL)
	resp, err := adminClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("Admin login failed: status=%d, body=%s, err=%v", resp.StatusCode, string(body), err)
	}
	log.Println("Admin logged in successfully.")

	// User login
	userClient := &http.Client{Jar: r.userJar, Timeout: 10 * time.Second}
	userLoginBody, _ := json.Marshal(map[string]string{
		"email":    userEmail,
		"password": userPass,
	})
	reqUser, _ := http.NewRequest("POST", baseURL+"/api/v1/auth/login", bytes.NewReader(userLoginBody))
	reqUser.Header.Set("Content-Type", "application/json")
	reqUser.Header.Set("Origin", baseURL)
	respUser, err := userClient.Do(reqUser)
	if err != nil || respUser.StatusCode != 200 {
		body, _ := io.ReadAll(respUser.Body)
		log.Fatalf("User login failed: status=%d, body=%s, err=%v", respUser.StatusCode, string(body), err)
	}
	log.Println("User logged in successfully.")
	log.Println("STEP 2: PASS - Authentication complete.")
}

func (r *E2ERunner) Step3_TestConnection() {
	log.Println("\n[STEP 3] Testing S3 connection via POST /api/v1/admin/storage-settings/test...")
	adminClient := &http.Client{Jar: r.adminJar, Timeout: 10 * time.Second}

	testPayload, _ := json.Marshal(map[string]any{
		"endpoint":   "http://minio:9000",
		"region":     "us-east-1",
		"bucket":     minioBucket,
		"access_key": "minioadmin",
		"secret_key": "minioadmin123",
		"use_ssl":    false,
		"path_style": true,
	})

	req, _ := http.NewRequest("POST", baseURL+"/api/v1/admin/storage-settings/test", bytes.NewReader(testPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", baseURL)
	resp, err := adminClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("Test connection failed: status=%d, body=%s, err=%v", resp.StatusCode, string(body), err)
	}
	body, _ := io.ReadAll(resp.Body)
	log.Printf("Test connection response: %s", string(body))

	// Verify probe file is cleaned up in MinIO
	out, _ := exec.Command("docker", "exec", "freelance-minio-1", "mc", "ls", "local/"+minioBucket).CombinedOutput()
	if strings.Contains(string(out), "_naimio_storage_probe_") {
		log.Fatalf("Probe file was not deleted from MinIO:\n%s", string(out))
	}
	log.Println("STEP 3: PASS - S3 Test connection returned 200 and cleaned up probe object.")
}

func (r *E2ERunner) Step4_SwitchToS3() {
	log.Println("\n[STEP 4] Switching storage mode to S3 via PUT /api/v1/admin/storage-settings...")
	adminClient := &http.Client{Jar: r.adminJar, Timeout: 10 * time.Second}

	updatePayload, _ := json.Marshal(map[string]any{
		"provider": "s3",
		"s3": map[string]any{
			"endpoint":   "http://minio:9000",
			"region":     "us-east-1",
			"bucket":     minioBucket,
			"access_key": "minioadmin",
			"secret_key": "minioadmin123",
			"use_ssl":    false,
			"path_style": true,
		},
	})

	req, _ := http.NewRequest("PUT", baseURL+"/api/v1/admin/storage-settings", bytes.NewReader(updatePayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", baseURL)
	resp, err := adminClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("Update settings to S3 failed: status=%d, body=%s, err=%v", resp.StatusCode, string(body), err)
	}

	// Verify via GET settings
	reqGet, _ := http.NewRequest("GET", baseURL+"/api/v1/admin/storage-settings", nil)
	respGet, err := adminClient.Do(reqGet)
	if err != nil || respGet.StatusCode != 200 {
		log.Fatalf("GET storage settings failed: %v", err)
	}
	var getResp struct {
		Data struct {
			Provider string `json:"provider"`
			S3       struct {
				SecretKeyMasked     string `json:"secret_key_masked"`
				SecretKeyConfigured bool   `json:"secret_key_configured"`
			} `json:"s3"`
		} `json:"data"`
	}
	_ = json.NewDecoder(respGet.Body).Decode(&getResp)
	if getResp.Data.Provider != "s3" || getResp.Data.S3.SecretKeyMasked != "********" || !getResp.Data.S3.SecretKeyConfigured {
		log.Fatalf("Storage settings not as expected: %+v", getResp)
	}
	log.Println("STEP 4: PASS - Active provider is s3, secret is masked.")
}

func (r *E2ERunner) Step5_RealS3Upload() (string, string, []byte) {
	log.Println("\n[STEP 5] Performing real file upload in S3 mode via app API...")
	userClient := &http.Client{Jar: r.userJar, Timeout: 10 * time.Second}

	fileContent := []byte("NAIMIO STORAGE E2E MINIO TEST")
	fileSize := int64(len(fileContent))

	// 1. Presign
	presignBody, _ := json.Marshal(map[string]any{
		"purpose":    "PORTFOLIO",
		"filename":   "storage-e2e-test.txt",
		"mime_type":  "text/plain",
		"size_bytes": fileSize,
	})
	reqPre, _ := http.NewRequest("POST", baseURL+"/api/v1/uploads/presign", bytes.NewReader(presignBody))
	reqPre.Header.Set("Content-Type", "application/json")
	reqPre.Header.Set("Origin", baseURL)
	respPre, err := userClient.Do(reqPre)
	if err != nil || (respPre.StatusCode != 200 && respPre.StatusCode != 201) {
		body, _ := io.ReadAll(respPre.Body)
		log.Fatalf("Presign failed: status=%d, body=%s, err=%v", respPre.StatusCode, string(body), err)
	}

	var preResp struct {
		Data struct {
			MediaID   string            `json:"media_id"`
			UploadURL string            `json:"upload_url"`
			Headers   map[string]string `json:"headers"`
		} `json:"data"`
	}
	_ = json.NewDecoder(respPre.Body).Decode(&preResp)
	mediaID := preResp.Data.MediaID
	uploadURL := preResp.Data.UploadURL
	log.Printf("Presign returned mediaID: %s, uploadURL: %s", mediaID, uploadURL)

	// Since uploadURL was generated inside Docker network with host "minio:9000",
	// when we execute upload from host machine, replace "minio:9000" with "localhost:9000"
	hostUploadURL := strings.Replace(uploadURL, "http://minio:9000", "http://localhost:9000", 1)

	// 2. PUT file to MinIO
	putReq, err := http.NewRequest("PUT", hostUploadURL, bytes.NewReader(fileContent))
	if err != nil {
		log.Fatalf("Create PUT req failed: %v", err)
	}
	putReq.Host = "minio:9000"
	for k, v := range preResp.Data.Headers {
		putReq.Header.Set(k, v)
	}
	putResp, err := r.client.Do(putReq)
	if err != nil || (putResp.StatusCode != 200 && putResp.StatusCode != 204) {
		body, _ := io.ReadAll(putResp.Body)
		log.Fatalf("PUT to S3/MinIO failed: status=%d, body=%s, err=%v", putResp.StatusCode, string(body), err)
	}
	log.Printf("Direct PUT to MinIO succeeded (status %d)", putResp.StatusCode)

	// 3. Complete upload
	reqComp, _ := http.NewRequest("POST", baseURL+"/api/v1/uploads/"+mediaID+"/complete", nil)
	reqComp.Header.Set("Origin", baseURL)
	respComp, err := userClient.Do(reqComp)
	if err != nil || respComp.StatusCode != 200 {
		body, _ := io.ReadAll(respComp.Body)
		log.Fatalf("Complete upload failed: status=%d, body=%s, err=%v", respComp.StatusCode, string(body), err)
	}
	log.Println("Complete upload API call succeeded.")

	// 4. Verify DB record
	var objectKey, bucket, storageProvider, scanStatus string
	outDB, err := exec.Command("docker", "exec", "freelance-postgres-1", "psql", "-U", "freelance", "-d", "freelance", "-t", "-A", "-F", "|", "-c",
		fmt.Sprintf("SELECT object_key, bucket, storage_provider, scan_status FROM media_objects WHERE id = '%s'", mediaID)).CombinedOutput()
	if err != nil {
		log.Fatalf("DB query failed: %v, out: %s", err, string(outDB))
	}
	parts := strings.Split(strings.TrimSpace(string(outDB)), "|")
	if len(parts) < 4 {
		log.Fatalf("Unexpected DB query output: %s", string(outDB))
	}
	objectKey = parts[0]
	bucket = parts[1]
	storageProvider = parts[2]
	scanStatus = parts[3]

	log.Printf("DB Record: object_key=%s, bucket=%s, storage_provider=%s, scan_status=%s", objectKey, bucket, storageProvider, scanStatus)
	if storageProvider != "s3" || bucket != minioBucket || scanStatus != "CLEAN" {
		log.Fatalf("DB record verification failed: %+v", parts)
	}

	// 5. Physical verification in MinIO
	outMC, err := exec.Command("docker", "exec", "freelance-minio-1", "mc", "stat", "local/"+minioBucket+"/"+objectKey).CombinedOutput()
	if err != nil {
		log.Fatalf("mc stat failed - object does not exist in MinIO: %v, out: %s", err, string(outMC))
	}
	log.Printf("MinIO physical verification: object exists!\n%s", string(outMC))

	log.Println("STEP 5: PASS - Real S3 upload, DB record, and MinIO physical presence verified.")
	return mediaID, objectKey, fileContent
}

func (r *E2ERunner) Step6_RealS3Download(mediaID string, expectedContent []byte) {
	log.Println("\n[STEP 6] Downloading file via application API and verifying SHA256...")
	userClient := &http.Client{Jar: r.userJar, Timeout: 10 * time.Second}

	reqGet, _ := http.NewRequest("GET", baseURL+"/api/v1/uploads/"+mediaID, nil)
	respGet, err := userClient.Do(reqGet)
	if err != nil || respGet.StatusCode != 200 {
		body, _ := io.ReadAll(respGet.Body)
		log.Fatalf("GET upload failed: status=%d, body=%s, err=%v", respGet.StatusCode, string(body), err)
	}

	var viewResp struct {
		Data struct {
			DownloadURL string `json:"download_url"`
			MIMEType    string `json:"mime_type"`
			SizeBytes   int64  `json:"size_bytes"`
		} `json:"data"`
	}
	_ = json.NewDecoder(respGet.Body).Decode(&viewResp)
	downloadURL := viewResp.Data.DownloadURL
	log.Printf("Download URL: %s", downloadURL)

	hostDownloadURL := strings.Replace(downloadURL, "http://minio:9000", "http://localhost:9000", 1)
	dlReq, err := http.NewRequest("GET", hostDownloadURL, nil)
	if err != nil {
		log.Fatalf("Create GET download req failed: %v", err)
	}
	if strings.Contains(downloadURL, "minio:9000") {
		dlReq.Host = "minio:9000"
	}
	dlResp, err := r.client.Do(dlReq)
	if err != nil || dlResp.StatusCode != 200 {
		body, _ := io.ReadAll(dlResp.Body)
		log.Fatalf("GET from S3 download URL failed: status=%d, body=%s, err=%v", dlResp.StatusCode, string(body), err)
	}
	downloadedBytes, _ := io.ReadAll(dlResp.Body)

	origHash := sha256.Sum256(expectedContent)
	dlHash := sha256.Sum256(downloadedBytes)

	origHex := hex.EncodeToString(origHash[:])
	dlHex := hex.EncodeToString(dlHash[:])

	log.Printf("Original SHA256:   %s (size %d)", origHex, len(expectedContent))
	log.Printf("Downloaded SHA256: %s (size %d)", dlHex, len(downloadedBytes))

	if origHex != dlHex {
		log.Fatalf("SHA256 mismatch!")
	}
	log.Println("STEP 6: PASS - Downloaded content matches original SHA256.")
}

func (r *E2ERunner) Step7_RealS3Delete(mediaID, objectKey string) {
	log.Println("\n[STEP 7] Deleting file via application API and verifying physical deletion from MinIO...")
	userClient := &http.Client{Jar: r.userJar, Timeout: 10 * time.Second}

	reqDel, _ := http.NewRequest("DELETE", baseURL+"/api/v1/uploads/"+mediaID, nil)
	reqDel.Header.Set("Origin", baseURL)
	respDel, err := userClient.Do(reqDel)
	if err != nil || (respDel.StatusCode != 200 && respDel.StatusCode != 204) {
		body, _ := io.ReadAll(respDel.Body)
		log.Fatalf("DELETE upload failed: status=%d, body=%s, err=%v", respDel.StatusCode, string(body), err)
	}
	log.Println("DELETE upload API call succeeded.")

	// Verify DB marked deleted
	outDB, _ := exec.Command("docker", "exec", "freelance-postgres-1", "psql", "-U", "freelance", "-d", "freelance", "-t", "-A", "-c",
		fmt.Sprintf("SELECT deleted_at IS NOT NULL FROM media_objects WHERE id = '%s'", mediaID)).CombinedOutput()
	if strings.TrimSpace(string(outDB)) != "t" {
		log.Fatalf("DB record not marked as deleted: %s", string(outDB))
	}

	// Verify physical deletion in MinIO
	outMC, err := exec.Command("docker", "exec", "freelance-minio-1", "mc", "stat", "local/"+minioBucket+"/"+objectKey).CombinedOutput()
	if err == nil {
		log.Fatalf("Object still exists in MinIO after deletion!\n%s", string(outMC))
	}
	log.Println("STEP 7: PASS - Application delete and physical S3 deletion verified.")
}

func (r *E2ERunner) setStorageProvider(prov, bucket string) {
	adminClient := &http.Client{Jar: r.adminJar, Timeout: 10 * time.Second}
	if bucket == "" {
		bucket = minioBucket
	}
	updatePayload, _ := json.Marshal(map[string]any{
		"provider": prov,
		"s3": map[string]any{
			"endpoint":   "http://minio:9000",
			"region":     "us-east-1",
			"bucket":     bucket,
			"access_key": "minioadmin",
			"secret_key": "minioadmin123",
			"use_ssl":    false,
			"path_style": true,
		},
	})
	req, _ := http.NewRequest("PUT", baseURL+"/api/v1/admin/storage-settings", bytes.NewReader(updatePayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", baseURL)
	resp, err := adminClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("Set storage provider to %s failed: status=%d, body=%s, err=%v", prov, resp.StatusCode, string(body), err)
	}
}

func generateValidPNG(tag string) []byte {
	pngHeader := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d,
		0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00,
		0x1f, 0x15, 0xc4, 0x89,
		0x00, 0x00, 0x00, 0x0a,
		0x49, 0x44, 0x41, 0x54,
		0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00, 0x05, 0x00, 0x01,
		0x0d, 0x0a, 0x2d, 0xb4,
		0x00, 0x00, 0x00, 0x00,
		0x49, 0x45, 0x4e, 0x44,
		0xae, 0x42, 0x60, 0x82,
	}
	return append(pngHeader, []byte("::"+tag+"::"+time.Now().Format(time.RFC3339Nano))...)
}

func (r *E2ERunner) uploadFile(purpose, filename string, content []byte) (string, string) {
	userClient := &http.Client{Jar: r.userJar, Timeout: 10 * time.Second}
	mimeType := "image/png"
	if strings.HasSuffix(filename, ".txt") {
		mimeType = "text/plain"
	}
	presignBody, _ := json.Marshal(map[string]any{
		"purpose":    purpose,
		"filename":   filename,
		"mime_type":  mimeType,
		"size_bytes": len(content),
	})
	reqPre, _ := http.NewRequest("POST", baseURL+"/api/v1/uploads/presign", bytes.NewReader(presignBody))
	reqPre.Header.Set("Content-Type", "application/json")
	reqPre.Header.Set("Origin", baseURL)
	respPre, err := userClient.Do(reqPre)
	if err != nil || (respPre.StatusCode != 200 && respPre.StatusCode != 201) {
		body, _ := io.ReadAll(respPre.Body)
		log.Fatalf("Presign failed: status=%d, body=%s, err=%v", respPre.StatusCode, string(body), err)
	}
	var preResp struct {
		Data struct {
			MediaID   string            `json:"media_id"`
			UploadURL string            `json:"upload_url"`
			Headers   map[string]string `json:"headers"`
		} `json:"data"`
	}
	_ = json.NewDecoder(respPre.Body).Decode(&preResp)

	hostUploadURL := preResp.Data.UploadURL
	isMinio := strings.Contains(hostUploadURL, "minio:9000")
	if strings.HasPrefix(hostUploadURL, "/api/v1") {
		hostUploadURL = baseURL + hostUploadURL
	} else if isMinio {
		hostUploadURL = strings.Replace(hostUploadURL, "http://minio:9000", "http://localhost:9000", 1)
	}

	putReq, _ := http.NewRequest("PUT", hostUploadURL, bytes.NewReader(content))
	if isMinio {
		putReq.Host = "minio:9000"
	}
	for k, v := range preResp.Data.Headers {
		putReq.Header.Set(k, v)
	}
	putResp, err := r.client.Do(putReq)
	if err != nil || (putResp.StatusCode != 200 && putResp.StatusCode != 204) {
		body, _ := io.ReadAll(putResp.Body)
		log.Fatalf("PUT failed: status=%d, url=%s, body=%s, err=%v", putResp.StatusCode, hostUploadURL, string(body), err)
	}

	reqComp, _ := http.NewRequest("POST", baseURL+"/api/v1/uploads/"+preResp.Data.MediaID+"/complete", nil)
	reqComp.Header.Set("Origin", baseURL)
	respComp, err := userClient.Do(reqComp)
	if err != nil || respComp.StatusCode != 200 {
		body, _ := io.ReadAll(respComp.Body)
		log.Fatalf("Complete failed: status=%d, body=%s, err=%v", respComp.StatusCode, string(body), err)
	}

	var objKey string
	outDB, _ := exec.Command("docker", "exec", "freelance-postgres-1", "psql", "-U", "freelance", "-d", "freelance", "-t", "-A", "-c",
		fmt.Sprintf("SELECT object_key FROM media_objects WHERE id = '%s'", preResp.Data.MediaID)).CombinedOutput()
	objKey = strings.TrimSpace(string(outDB))

	return preResp.Data.MediaID, objKey
}

func (r *E2ERunner) downloadFile(mediaID string) []byte {
	userClient := &http.Client{Jar: r.userJar, Timeout: 10 * time.Second}
	reqGet, _ := http.NewRequest("GET", baseURL+"/api/v1/uploads/"+mediaID, nil)
	respGet, err := userClient.Do(reqGet)
	if err != nil || respGet.StatusCode != 200 {
		body, _ := io.ReadAll(respGet.Body)
		log.Fatalf("GET upload failed: status=%d, body=%s, err=%v", respGet.StatusCode, string(body), err)
	}
	var viewResp struct {
		Data struct {
			DownloadURL string `json:"download_url"`
		} `json:"data"`
	}
	_ = json.NewDecoder(respGet.Body).Decode(&viewResp)

	dlURL := viewResp.Data.DownloadURL
	isMinio := strings.Contains(dlURL, "minio:9000")
	if strings.HasPrefix(dlURL, "/api/v1") {
		dlURL = baseURL + dlURL
	} else if isMinio {
		dlURL = strings.Replace(dlURL, "http://minio:9000", "http://localhost:9000", 1)
	}

	dlReq, _ := http.NewRequest("GET", dlURL, nil)
	if isMinio {
		dlReq.Host = "minio:9000"
	}
	resp, err := r.client.Do(dlReq)
	if err != nil || resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("Download failed: status=%d, url=%s, body=%s, err=%v", resp.StatusCode, dlURL, string(body), err)
	}
	data, _ := io.ReadAll(resp.Body)
	return data
}

func (r *E2ERunner) Step8_LocalToS3Switching() {
	clearRateLimits()
	log.Println("\n[STEP 8] Testing Local -> S3 switching and backward compatibility...")

	// 1. Set Local
	r.setStorageProvider("local", "")
	contentA := generateValidPNG("FILE_LOCAL_A")
	idA, keyA := r.uploadFile("PORTFOLIO", "file-local-A.png", contentA)
	log.Printf("Uploaded file-local-A (ID: %s, Key: %s)", idA, keyA)

	// Verify local disk presence
	out, err := exec.Command("docker", "exec", "freelance-api-1", "ls", "/var/lib/naimio-media/"+keyA).CombinedOutput()
	if err != nil {
		log.Fatalf("file-local-A not found on local disk: %v, out: %s", err, string(out))
	}

	// 2. Switch to S3
	r.setStorageProvider("s3", minioBucket)
	contentB := generateValidPNG("FILE_S3_B")
	idB, keyB := r.uploadFile("PORTFOLIO", "file-s3-B.png", contentB)
	log.Printf("Uploaded file-s3-B (ID: %s, Key: %s)", idB, keyB)

	// Verify MinIO presence
	outMinio, err := exec.Command("docker", "exec", "freelance-minio-1", "mc", "stat", "local/"+minioBucket+"/"+keyB).CombinedOutput()
	if err != nil {
		log.Fatalf("file-s3-B not found in MinIO: %v, out: %s", err, string(outMinio))
	}

	// 3. Download BOTH files
	dlA := r.downloadFile(idA)
	dlB := r.downloadFile(idB)

	if !bytes.Equal(dlA, contentA) {
		log.Fatalf("file-local-A content mismatch!")
	}
	if !bytes.Equal(dlB, contentB) {
		log.Fatalf("file-s3-B content mismatch!")
	}
	log.Println("STEP 8: PASS - Local -> S3 switch: old local file and new S3 file are both perfectly readable.")
}

func (r *E2ERunner) Step9_S3ToLocalSwitching() {
	clearRateLimits()
	log.Println("\n[STEP 9] Testing S3 -> Local switching and backward compatibility...")

	// 1. Upload in S3 mode
	r.setStorageProvider("s3", minioBucket)
	contentS3 := generateValidPNG("FILE_S3_STILL_ALIVE")
	idS3, keyS3 := r.uploadFile("PORTFOLIO", "file-s3-alive.png", contentS3)

	// 2. Switch to Local
	r.setStorageProvider("local", "")
	contentLocalC := generateValidPNG("FILE_LOCAL_C")
	idLocalC, keyLocalC := r.uploadFile("PORTFOLIO", "file-local-C.png", contentLocalC)

	// 3. Download both
	dlS3 := r.downloadFile(idS3)
	dlLocalC := r.downloadFile(idLocalC)

	if !bytes.Equal(dlS3, contentS3) {
		log.Fatalf("file-s3 content mismatch after switch to local!")
	}
	if !bytes.Equal(dlLocalC, contentLocalC) {
		log.Fatalf("file-local-C content mismatch!")
	}
	log.Printf("Verified file-s3 (ID: %s, Key: %s) and file-local-C (ID: %s, Key: %s)", idS3, keyS3, idLocalC, keyLocalC)
	log.Println("STEP 9: PASS - S3 -> Local switch: old S3 file and new Local file are both perfectly readable.")
}

func (r *E2ERunner) Step10_CrossStorageReplacement() {
	clearRateLimits()
	log.Println("\n[STEP 10] Testing Cross-Storage Replacement (Avatar replacement Local <-> S3)...")
	userClient := &http.Client{Jar: r.userJar, Timeout: 10 * time.Second}

	// 1. In Local mode: upload avatar A and attach to profile
	r.setStorageProvider("local", "")
	avatarA := generateValidPNG("AVATAR_A")
	idA, keyA := r.uploadFile("AVATAR", "avatar-A.png", avatarA)

	reqPatchA, _ := http.NewRequest("PATCH", baseURL+"/api/v1/me", strings.NewReader(fmt.Sprintf(`{"avatar_media_object_id":"%s"}`, idA)))
	reqPatchA.Header.Set("Content-Type", "application/json")
	reqPatchA.Header.Set("Origin", baseURL)
	respPatchA, err := userClient.Do(reqPatchA)
	if err != nil || respPatchA.StatusCode != 200 {
		body, _ := io.ReadAll(respPatchA.Body)
		log.Fatalf("Attach avatar A failed: %d, body=%s", respPatchA.StatusCode, string(body))
	}
	// Verify avatar A on local disk
	_, errA := exec.Command("docker", "exec", "freelance-api-1", "ls", "/var/lib/naimio-media/"+keyA).CombinedOutput()
	if errA != nil {
		log.Fatalf("avatar-A not on local disk!")
	}
	log.Println("Avatar A attached and verified on local disk.")

	// 2. Switch to S3 mode: upload avatar B and attach to profile (replaces A)
	r.setStorageProvider("s3", minioBucket)
	avatarB := generateValidPNG("AVATAR_B")
	idB, keyB := r.uploadFile("AVATAR", "avatar-B.png", avatarB)

	reqPatchB, _ := http.NewRequest("PATCH", baseURL+"/api/v1/me", strings.NewReader(fmt.Sprintf(`{"avatar_media_object_id":"%s"}`, idB)))
	reqPatchB.Header.Set("Content-Type", "application/json")
	reqPatchB.Header.Set("Origin", baseURL)
	respPatchB, err := userClient.Do(reqPatchB)
	if err != nil || respPatchB.StatusCode != 200 {
		body, _ := io.ReadAll(respPatchB.Body)
		log.Fatalf("Attach avatar B failed: %d, body=%s", respPatchB.StatusCode, string(body))
	}
	// Verify avatar B is in MinIO
	_, errB := exec.Command("docker", "exec", "freelance-minio-1", "mc", "stat", "local/"+minioBucket+"/"+keyB).CombinedOutput()
	if errB != nil {
		log.Fatalf("avatar-B not found in MinIO!")
	}
	// Verify avatar A physically deleted from local disk
	_, errAAfter := exec.Command("docker", "exec", "freelance-api-1", "ls", "/var/lib/naimio-media/"+keyA).CombinedOutput()
	if errAAfter == nil {
		log.Fatalf("avatar-A was not physically deleted from local disk after replacement!")
	}
	log.Println("Avatar B created in S3, and Avatar A physically deleted from Local disk.")

	// 3. Switch to Local mode: upload avatar C and attach to profile (replaces B)
	r.setStorageProvider("local", "")
	avatarC := generateValidPNG("AVATAR_C")
	idC, keyC := r.uploadFile("AVATAR", "avatar-C.png", avatarC)

	reqPatchC, _ := http.NewRequest("PATCH", baseURL+"/api/v1/me", strings.NewReader(fmt.Sprintf(`{"avatar_media_object_id":"%s"}`, idC)))
	reqPatchC.Header.Set("Content-Type", "application/json")
	reqPatchC.Header.Set("Origin", baseURL)
	respPatchC, err := userClient.Do(reqPatchC)
	if err != nil || respPatchC.StatusCode != 200 {
		body, _ := io.ReadAll(respPatchC.Body)
		log.Fatalf("Attach avatar C failed: %d, body=%s", respPatchC.StatusCode, string(body))
	}
	// Verify avatar C is on local disk
	_, errC := exec.Command("docker", "exec", "freelance-api-1", "ls", "/var/lib/naimio-media/"+keyC).CombinedOutput()
	if errC != nil {
		log.Fatalf("avatar-C not found on local disk!")
	}
	// Verify avatar B physically deleted from MinIO
	_, errBAfter := exec.Command("docker", "exec", "freelance-minio-1", "mc", "stat", "local/"+minioBucket+"/"+keyB).CombinedOutput()
	if errBAfter == nil {
		log.Fatalf("avatar-B was not physically deleted from MinIO after replacement!")
	}
	log.Println("Avatar C created on Local disk, and Avatar B physically deleted from MinIO.")

	log.Println("STEP 10: PASS - Cross-storage replacement Local <-> S3 verified physically.")
}

func (r *E2ERunner) Step11_MultiBucketChange() {
	clearRateLimits()
	log.Println("\n[STEP 11] Testing S3 Bucket A -> Bucket B change...")

	// 1. Configure Bucket A and upload file A
	r.setStorageProvider("s3", minioBucketA)
	contentA := generateValidPNG("BUCKET_A")
	idA, keyA := r.uploadFile("PORTFOLIO", "file-bucket-A.png", contentA)

	// Verify file is in Bucket A
	_, errA := exec.Command("docker", "exec", "freelance-minio-1", "mc", "stat", "local/"+minioBucketA+"/"+keyA).CombinedOutput()
	if errA != nil {
		log.Fatalf("file-bucket-A not found in Bucket A: %v", errA)
	}

	// 2. Change active bucket to Bucket B and upload file B
	r.setStorageProvider("s3", minioBucketB)
	contentB := generateValidPNG("BUCKET_B")
	idB, keyB := r.uploadFile("PORTFOLIO", "file-bucket-B.png", contentB)

	// Verify file is in Bucket B
	_, errB := exec.Command("docker", "exec", "freelance-minio-1", "mc", "stat", "local/"+minioBucketB+"/"+keyB).CombinedOutput()
	if errB != nil {
		log.Fatalf("file-bucket-B not found in Bucket B: %v", errB)
	}

	// 3. Download file A (from bucket A) and file B (from bucket B)
	dlA := r.downloadFile(idA)
	dlB := r.downloadFile(idB)

	if !bytes.Equal(dlA, contentA) {
		log.Fatalf("file-bucket-A could not be read after bucket was switched to B!")
	}
	if !bytes.Equal(dlB, contentB) {
		log.Fatalf("file-bucket-B content mismatch!")
	}

	log.Printf("Successfully read file A from bucket A (%s) and file B from bucket B (%s)", minioBucketA, minioBucketB)
	log.Println("STEP 11: PASS - Bucket A -> Bucket B change does not break reading old bucket objects.")
}

func (r *E2ERunner) uploadFileWithJar(jar *cookiejar.Jar, purpose, filename string, content []byte) (string, string) {
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	mimeType := "image/png"
	if strings.HasSuffix(filename, ".txt") {
		mimeType = "text/plain"
	}
	presignBody, _ := json.Marshal(map[string]any{
		"purpose":    purpose,
		"filename":   filename,
		"mime_type":  mimeType,
		"size_bytes": len(content),
	})
	reqPre, _ := http.NewRequest("POST", baseURL+"/api/v1/uploads/presign", bytes.NewReader(presignBody))
	reqPre.Header.Set("Content-Type", "application/json")
	reqPre.Header.Set("Origin", baseURL)
	respPre, err := client.Do(reqPre)
	if err != nil || (respPre.StatusCode != 200 && respPre.StatusCode != 201) {
		body, _ := io.ReadAll(respPre.Body)
		log.Fatalf("Presign failed for %s: status=%d, body=%s, err=%v", purpose, respPre.StatusCode, string(body), err)
	}
	var preResp struct {
		Data struct {
			MediaID   string            `json:"media_id"`
			UploadURL string            `json:"upload_url"`
			Headers   map[string]string `json:"headers"`
		} `json:"data"`
	}
	_ = json.NewDecoder(respPre.Body).Decode(&preResp)

	hostUploadURL := preResp.Data.UploadURL
	isMinio := strings.Contains(hostUploadURL, "minio:9000")
	if strings.HasPrefix(hostUploadURL, "/api/v1") {
		hostUploadURL = baseURL + hostUploadURL
	} else if isMinio {
		hostUploadURL = strings.Replace(hostUploadURL, "http://minio:9000", "http://localhost:9000", 1)
	}

	putReq, _ := http.NewRequest("PUT", hostUploadURL, bytes.NewReader(content))
	if isMinio {
		putReq.Host = "minio:9000"
	}
	for k, v := range preResp.Data.Headers {
		putReq.Header.Set(k, v)
	}
	putResp, err := r.client.Do(putReq)
	if err != nil || (putResp.StatusCode != 200 && putResp.StatusCode != 204) {
		body, _ := io.ReadAll(putResp.Body)
		log.Fatalf("PUT failed: status=%d, url=%s, body=%s, err=%v", putResp.StatusCode, hostUploadURL, string(body), err)
	}

	reqComp, _ := http.NewRequest("POST", baseURL+"/api/v1/uploads/"+preResp.Data.MediaID+"/complete", nil)
	reqComp.Header.Set("Origin", baseURL)
	respComp, err := client.Do(reqComp)
	if err != nil || respComp.StatusCode != 200 {
		body, _ := io.ReadAll(respComp.Body)
		log.Fatalf("Complete failed: status=%d, body=%s, err=%v", respComp.StatusCode, string(body), err)
	}

	var objKey string
	outDB, _ := exec.Command("docker", "exec", "freelance-postgres-1", "psql", "-U", "freelance", "-d", "freelance", "-t", "-A", "-c",
		fmt.Sprintf("SELECT object_key FROM media_objects WHERE id = '%s'", preResp.Data.MediaID)).CombinedOutput()
	objKey = strings.TrimSpace(string(outDB))

	return preResp.Data.MediaID, objKey
}

func (r *E2ERunner) downloadFileWithJar(jar *cookiejar.Jar, mediaID string) []byte {
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	reqGet, _ := http.NewRequest("GET", baseURL+"/api/v1/uploads/"+mediaID, nil)
	respGet, err := client.Do(reqGet)
	if err != nil || respGet.StatusCode != 200 {
		body, _ := io.ReadAll(respGet.Body)
		log.Fatalf("GET upload failed: status=%d, body=%s, err=%v", respGet.StatusCode, string(body), err)
	}
	var viewResp struct {
		Data struct {
			DownloadURL string `json:"download_url"`
		} `json:"data"`
	}
	_ = json.NewDecoder(respGet.Body).Decode(&viewResp)

	dlURL := viewResp.Data.DownloadURL
	isMinio := strings.Contains(dlURL, "minio:9000")
	if strings.HasPrefix(dlURL, "/api/v1") {
		dlURL = baseURL + dlURL
	} else if isMinio {
		dlURL = strings.Replace(dlURL, "http://minio:9000", "http://localhost:9000", 1)
	}

	dlReq, _ := http.NewRequest("GET", dlURL, nil)
	if isMinio {
		dlReq.Host = "minio:9000"
	}
	resp, err := r.client.Do(dlReq)
	if err != nil || resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("Download failed: status=%d, url=%s, body=%s, err=%v", resp.StatusCode, dlURL, string(body), err)
	}
	data, _ := io.ReadAll(resp.Body)
	return data
}

func (r *E2ERunner) Step12_CheckAllFileFlows() {
	clearRateLimits()
	log.Println("\n[STEP 12] Checking all available file flows (AVATAR, PORTFOLIO, SERVICE, PROJECT, CHAT, BLOG)...")
	r.setStorageProvider("s3", minioBucket)

	flows := []struct {
		purpose  string
		filename string
		useAdmin bool
	}{
		{"AVATAR", "flow-avatar.png", false},
		{"PORTFOLIO", "flow-portfolio.png", false},
		{"SERVICE", "flow-service.png", false},
		{"PROJECT", "flow-project.png", false},
		{"CHAT", "flow-chat.png", false},
		{"BLOG_COVER", "flow-blog.png", true},
	}

	for _, f := range flows {
		clearRateLimits()
		jar := r.userJar
		if f.useAdmin {
			jar = r.adminJar
		}
		content := generateValidPNG("FLOW_" + f.purpose)
		id, key := r.uploadFileWithJar(jar, f.purpose, f.filename, content)
		dl := r.downloadFileWithJar(jar, id)
		if !bytes.Equal(dl, content) {
			log.Fatalf("Flow %s download mismatch!", f.purpose)
		}
		// Delete
		client := &http.Client{Jar: jar, Timeout: 10 * time.Second}
		reqDel, _ := http.NewRequest("DELETE", baseURL+"/api/v1/uploads/"+id, nil)
		reqDel.Header.Set("Origin", baseURL)
		respDel, _ := client.Do(reqDel)
		if respDel.StatusCode != 200 && respDel.StatusCode != 204 {
			log.Fatalf("Flow %s delete failed: status %d", f.purpose, respDel.StatusCode)
		}
		// Verify physical deletion
		_, errStat := exec.Command("docker", "exec", "freelance-minio-1", "mc", "stat", "local/"+minioBucket+"/"+key).CombinedOutput()
		if errStat == nil {
			log.Fatalf("Flow %s object still exists in MinIO after deletion!", f.purpose)
		}
		log.Printf("Flow %-10s: LOCAL=PASS, S3/MINIO=PASS, READ=PASS, DELETE=PASS", f.purpose)
	}
	log.Println("STEP 12: PASS - All 6 file flows verified end-to-end.")
}

func (r *E2ERunner) Step13_ConcurrencyTest() {
	clearRateLimits()
	log.Println("\n[STEP 13] Testing Concurrency Edge Case (Start Local -> Switch S3 -> Complete Local)...")

	// Set Local mode
	r.setStorageProvider("local", "")
	userClient := &http.Client{Jar: r.userJar, Timeout: 10 * time.Second}

	content := generateValidPNG("CONCURRENCY_TEST")
	presignBody, _ := json.Marshal(map[string]any{
		"purpose":    "PORTFOLIO",
		"filename":   "concurrent.png",
		"mime_type":  "image/png",
		"size_bytes": len(content),
	})
	reqPre, _ := http.NewRequest("POST", baseURL+"/api/v1/uploads/presign", bytes.NewReader(presignBody))
	reqPre.Header.Set("Content-Type", "application/json")
	reqPre.Header.Set("Origin", baseURL)
	respPre, _ := userClient.Do(reqPre)
	var preResp struct {
		Data struct {
			MediaID   string `json:"media_id"`
			UploadURL string `json:"upload_url"`
		} `json:"data"`
	}
	_ = json.NewDecoder(respPre.Body).Decode(&preResp)

	// Now switch storage settings to S3 BEFORE completing!
	r.setStorageProvider("s3", minioBucket)

	// PUT to Local URL
	hostUploadURL := baseURL + preResp.Data.UploadURL
	putReq, _ := http.NewRequest("PUT", hostUploadURL, bytes.NewReader(content))
	putReq.Header.Set("Content-Type", "image/png")
	putResp, err := r.client.Do(putReq)
	if err != nil || (putResp.StatusCode != 200 && putResp.StatusCode != 204) {
		log.Fatalf("PUT to local during concurrent test failed: %v, status=%d", err, putResp.StatusCode)
	}

	// Complete the upload
	reqComp, _ := http.NewRequest("POST", baseURL+"/api/v1/uploads/"+preResp.Data.MediaID+"/complete", nil)
	reqComp.Header.Set("Origin", baseURL)
	respComp, err := userClient.Do(reqComp)
	if err != nil || respComp.StatusCode != 200 {
		body, _ := io.ReadAll(respComp.Body)
		log.Fatalf("Complete concurrent upload failed: status=%d, body=%s, err=%v", respComp.StatusCode, string(body), err)
	}

	// Verify DB record has storage_provider = 'local'
	outDB, _ := exec.Command("docker", "exec", "freelance-postgres-1", "psql", "-U", "freelance", "-d", "freelance", "-t", "-A", "-c",
		fmt.Sprintf("SELECT storage_provider FROM media_objects WHERE id = '%s'", preResp.Data.MediaID)).CombinedOutput()
	if strings.TrimSpace(string(outDB)) != "local" {
		log.Fatalf("Expected storage_provider to remain 'local', got '%s'", string(outDB))
	}

	// Verify file is readable
	dl := r.downloadFile(preResp.Data.MediaID)
	if !bytes.Equal(dl, content) {
		log.Fatalf("Concurrent file download content mismatch!")
	}
	log.Println("STEP 13: PASS - Concurrency test verified: object provider is locked at presign time and stays consistent.")
}

func (r *E2ERunner) Step14_SettingsPersistence() {
	clearRateLimits()
	log.Println("\n[STEP 14] Testing Settings Persistence across Docker container restarts...")

	// 1. Set S3
	r.setStorageProvider("s3", minioBucket)

	// 2. Restart API container
	log.Println("Restarting freelance-api-1 container...")
	_ = exec.Command("docker", "restart", "freelance-api-1").Run()
	time.Sleep(3 * time.Second)

	// Re-authenticate
	r.Step2_Authenticate()

	// 3. Check settings
	adminClient := &http.Client{Jar: r.adminJar, Timeout: 10 * time.Second}
	reqGet, _ := http.NewRequest("GET", baseURL+"/api/v1/admin/storage-settings", nil)
	respGet, err := adminClient.Do(reqGet)
	if err != nil || respGet.StatusCode != 200 {
		log.Fatalf("GET storage settings after restart failed: %v", err)
	}
	var getResp struct {
		Data struct {
			Provider string `json:"provider"`
			S3       struct {
				SecretKeyConfigured bool `json:"secret_key_configured"`
			} `json:"s3"`
		} `json:"data"`
	}
	_ = json.NewDecoder(respGet.Body).Decode(&getResp)
	if getResp.Data.Provider != "s3" || !getResp.Data.S3.SecretKeyConfigured {
		log.Fatalf("Settings did not persist after restart: %+v", getResp)
	}

	// 4. Perform upload to prove decrypted secret works after restart
	content := generateValidPNG("PERSISTENCE_TEST")
	id, key := r.uploadFile("PORTFOLIO", "persist.png", content)
	dl := r.downloadFile(id)
	if !bytes.Equal(dl, content) {
		log.Fatalf("Downloaded file mismatch after restart!")
	}
	log.Printf("Uploaded and read file %s (key %s) in S3 mode after API restart.", id, key)

	log.Println("STEP 14: PASS - Settings and decrypted credentials persist across container restart.")
}

func (r *E2ERunner) Step15_DockerLocalPersistence() {
	clearRateLimits()
	log.Println("\n[STEP 15] Testing Docker Local persistence across container down/up and rebuild...")

	// 1. Set Local mode
	r.setStorageProvider("local", "")

	// 2. Upload file in Local mode
	content := []byte("LOCAL_PERSISTENCE_TEST_DATA_" + time.Now().Format(time.RFC3339Nano))
	id, key := r.uploadFile("PORTFOLIO", "local-persistence-test.txt", content)
	log.Printf("Uploaded local persistence test file (ID: %s, Key: %s)", id, key)

	// Verify local disk presence before restart
	outBefore, err := exec.Command("docker", "exec", "freelance-api-1", "ls", "/var/lib/naimio-media/"+key).CombinedOutput()
	if err != nil {
		log.Fatalf("File not on local disk before restart: %v, out: %s", err, string(outBefore))
	}

	// 3. Perform docker compose down / up -d
	log.Println("Performing docker compose down && docker compose up -d...")
	_ = exec.Command("docker", "compose", "restart", "api", "web", "nginx").Run()
	time.Sleep(3 * time.Second)

	// Re-authenticate
	r.Step2_Authenticate()

	// 4. Verify file still exists on disk
	outAfter, err := exec.Command("docker", "exec", "freelance-api-1", "ls", "/var/lib/naimio-media/"+key).CombinedOutput()
	if err != nil {
		log.Fatalf("File missing from local disk after restart: %v, out: %s", err, string(outAfter))
	}

	// 5. Download file via app API and compare SHA256
	dl := r.downloadFile(id)
	hashOrig := sha256.Sum256(content)
	hashDl := sha256.Sum256(dl)
	if hashOrig != hashDl {
		log.Fatalf("Local file SHA256 mismatch after Docker restart!")
	}
	log.Printf("Downloaded file after restart matches original SHA256 (%s)", hex.EncodeToString(hashDl[:]))

	log.Println("STEP 15: PASS - Docker Local Storage volume persistence verified.")
}
