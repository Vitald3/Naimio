package media

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"freelance/apps/api/internal/auth"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Handler struct {
	Service  Service
	Database *sql.DB
}

func (h Handler) Avatar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	identity := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/v1/avatars/"))
	if identity == "" || len(identity) > 120 || strings.Contains(identity, "/") {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "avatar not found")
		return
	}
	defaultURL := func(gender string) string {
		hash := fnv.New32a()
		_, _ = hash.Write([]byte(strings.ToLower(identity)))
		// Standard avatars are stable and gender-aware. Keeping the fallback on
		// bundled assets avoids storage/network work for users without uploads.
		base, count := uint32(1), uint32(12)
		switch strings.ToUpper(gender) {
		case "MALE":
			base, count = 1, 6
		case "FEMALE":
			base, count = 7, 6
		}
		return fmt.Sprintf("/media/avatars/avatar-%02d.svg", base+hash.Sum32()%count)
	}
	if h.Database == nil {
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, defaultURL(""), http.StatusTemporaryRedirect)
		return
	}
	var objectKey sql.NullString
	var storageProvider sql.NullString
	var storageBackendID sql.NullString
	var bucket sql.NullString
	var gender string
	err := h.Database.QueryRowContext(r.Context(), `SELECT m.object_key, COALESCE(m.storage_provider, 'local'), COALESCE(m.storage_backend_id::text, ''), COALESCE(m.bucket, ''), u.gender FROM users u LEFT JOIN media_objects m ON m.id=u.avatar_media_object_id AND m.owner_user_id=u.id AND m.purpose='AVATAR' AND m.scan_status='CLEAN' AND m.uploaded_at IS NOT NULL AND m.deleted_at IS NULL WHERE (u.id::text=$1 OR u.username_normalized=lower($1)) AND u.status='ACTIVE' AND u.deleted_at IS NULL`, identity).Scan(&objectKey, &storageProvider, &storageBackendID, &bucket, &gender)
	if errors.Is(err, sql.ErrNoRows) {
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, defaultURL(""), http.StatusTemporaryRedirect)
		return
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	if !objectKey.Valid || strings.TrimSpace(objectKey.String) == "" {
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, defaultURL(gender), http.StatusTemporaryRedirect)
		return
	}
	st := h.Service.storageFor(r.Context(), storageProvider.String, bucket.String, storageBackendID.String)
	// A DB row can outlive a development file created before persistent media
	// storage was introduced. Validate the object before issuing a download URL
	// so catalog cards never make a second request that ends in /dev-storage 404.
	if _, inspectErr := st.Inspect(r.Context(), objectKey.String); inspectErr != nil {
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, defaultURL(gender), http.StatusTemporaryRedirect)
		return
	}
	url, _, err := st.PresignGet(r.Context(), objectKey.String, 5*time.Minute)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func (h Handler) Presign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	actorID, ok := auth.ActorID(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return
	}
	var input PresignInput
	if !decodeBody(w, r, &input) {
		return
	}
	result, err := h.Service.CreatePresign(r.Context(), actorID, input)
	if writeDomainError(w, r, err) {
		return
	}
	write(w, http.StatusCreated, map[string]any{"data": result})
}

func (h Handler) Item(w http.ResponseWriter, r *http.Request) {
	actorID, ok := auth.ActorID(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/uploads/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) == 2 && validUUID(parts[0]) && parts[1] == "complete" && r.Method == http.MethodPost {
		if !emptyBody(w, r) {
			return
		}
		object, err := h.Service.Complete(r.Context(), actorID, parts[0])
		if writeDomainError(w, r, err) {
			return
		}
		write(w, http.StatusOK, map[string]any{"data": object})
		return
	}
	if len(parts) != 1 || !validUUID(parts[0]) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "upload not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		view, err := h.Service.Get(r.Context(), actorID, parts[0])
		if writeDomainError(w, r, err) {
			return
		}
		write(w, http.StatusOK, map[string]any{"data": view})
	case http.MethodDelete:
		if writeDomainError(w, r, h.Service.Delete(r.Context(), actorID, parts[0])) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func decodeBody(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeDecodeError(w, r, err)
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeDecodeError(w, r, err)
		return false
	}
	return true
}

func emptyBody(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	content, err := io.ReadAll(r.Body)
	if err != nil {
		writeDecodeError(w, r, err)
		return false
	}
	if value := strings.TrimSpace(string(content)); value != "" && value != "{}" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "complete payload must be empty")
		return false
	}
	return true
}

func writeDecodeError(w http.ResponseWriter, r *http.Request, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, r, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "upload payload is too large")
		return
	}
	writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid upload payload")
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "upload not found")
	case errors.Is(err, ErrInvalidInput):
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", strings.TrimPrefix(err.Error(), ErrInvalidInput.Error()+": "))
	case errors.Is(err, ErrInvalidObject):
		writeError(w, r, http.StatusUnprocessableEntity, "UPLOAD_VALIDATION_FAILED", "uploaded object does not match the presign request")
	default:
		requestID := w.Header().Get("X-Request-ID")
		if requestID == "" {
			requestID = r.Header.Get("X-Request-ID")
		}
		log.Printf("media operation failure request_id=%s error_type=%T", requestID, err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
	return true
}

func validUUID(value string) bool {
	return uuidPattern.MatchString(strings.ToLower(strings.TrimSpace(value)))
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	requestID := w.Header().Get("X-Request-ID")
	if requestID == "" {
		requestID = r.Header.Get("X-Request-ID")
	}
	write(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "request_id": requestID}})
}

func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
