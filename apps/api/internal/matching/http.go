package matching

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"

	"freelance/apps/api/internal/auth"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Handler struct{ Service Service }
type eventInput struct {
	RunID        string `json:"run_id"`
	FreelancerID string `json:"freelancer_user_id"`
	EventType    string `json:"event_type"`
}
type manualInput struct {
	FreelancerID   string `json:"freelancer_user_id"`
	InternalReason string `json:"internal_reason"`
}

func (h Handler) Customer(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.ActorID(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/me/projects/"), "/"), "/")
	if len(parts) < 2 || !validUUID(parts[0]) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "matching resource not found")
		return
	}
	projectID := parts[0]
	switch {
	case len(parts) == 2 && parts[1] == "matching-runs" && r.Method == http.MethodPost:
		var input Constraints
		if !decode(w, r, &input) {
			return
		}
		run, err := h.Service.Run(r.Context(), actor, projectID, input)
		if domainError(w, r, err) {
			return
		}
		write(w, http.StatusCreated, map[string]any{"data": run})
	case len(parts) == 3 && parts[1] == "matching-runs" && r.Method == http.MethodGet && validUUID(parts[2]):
		run, err := h.Service.RunByID(r.Context(), actor, projectID, parts[2])
		if domainError(w, r, err) {
			return
		}
		write(w, http.StatusOK, map[string]any{"data": run})
	case len(parts) == 2 && parts[1] == "recommendations" && r.Method == http.MethodGet:
		run, err := h.Service.Latest(r.Context(), actor, projectID)
		if domainError(w, r, err) {
			return
		}
		write(w, http.StatusOK, map[string]any{"data": run.Recommendations, "run": map[string]any{"id": run.ID, "algorithm_version": run.AlgorithmVersion, "ai_used": run.AIUsed, "created_at": run.CreatedAt}})
	case len(parts) == 2 && parts[1] == "matching-events" && r.Method == http.MethodPost:
		var input eventInput
		if !decode(w, r, &input) {
			return
		}
		key := r.Header.Get("Idempotency-Key")
		if !validUUID(input.RunID) || !validUUID(input.FreelancerID) {
			writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid matching event")
			return
		}
		if domainError(w, r, h.Service.Event(r.Context(), actor, projectID, input.RunID, input.FreelancerID, input.EventType, key)) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "matching resource not found")
	}
}
func (h Handler) Admin(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.ActorID(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/projects/"), "/"), "/")
	if len(parts) < 2 || !validUUID(parts[0]) || parts[1] != "recommendations" {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "matching resource not found")
		return
	}
	if len(parts) == 2 && r.Method == http.MethodPost {
		var input manualInput
		if !decode(w, r, &input) {
			return
		}
		if !validUUID(input.FreelancerID) {
			writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid recommendation")
			return
		}
		if domainError(w, r, h.Service.PutManual(r.Context(), actor, parts[0], input.FreelancerID, input.InternalReason)) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(parts) == 3 && r.Method == http.MethodDelete && validUUID(parts[2]) {
		if domainError(w, r, h.Service.DeleteManual(r.Context(), actor, parts[0], parts[2])) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
}
func (h Handler) AdminMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	actor, ok := auth.ActorID(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return
	}
	metrics, err := h.Service.Metrics(r.Context(), actor)
	if domainError(w, r, err) {
		return
	}
	write(w, http.StatusOK, map[string]any{"data": metrics, "window": "30d"})
}
func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, r, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "matching payload is too large")
		} else {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid matching payload")
		}
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid matching payload")
		return false
	}
	return true
}
func domainError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ErrUnauthorized):
		writeError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
	case errors.Is(err, ErrForbidden):
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "permission denied")
	case errors.Is(err, ErrNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "matching resource not found")
	case errors.Is(err, ErrInvalid):
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid matching input")
	case errors.Is(err, ErrInvalidState):
		writeError(w, r, http.StatusConflict, "INVALID_STATE", "project is not matchable")
	default:
		log.Printf("matching failure request_id=%s error_type=%T", r.Header.Get("X-Request-ID"), err)
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
	write(w, status, map[string]any{"error": map[string]string{"code": code, "message": message, "request_id": requestID}})
}
func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
