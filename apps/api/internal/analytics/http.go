package analytics

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"freelance/apps/api/internal/auth"
)

type Handler struct {
	Service Service
}

func (h Handler) Track(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	var body struct {
		EventType     string `json:"event_type"`
		SubjectUserID string `json:"subject_user_id"`
		EntityID      string `json:"entity_id"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid analytics payload")
		return
	}
	viewer, _ := auth.ActorID(r.Context())
	err := h.Service.Track(r.Context(), EventInput{
		SubjectUserID: body.SubjectUserID,
		ViewerUserID:  viewer,
		EventType:     body.EventType,
		EntityID:      body.EntityID,
	})
	if writeDomainError(w, r, err) {
		return
	}
	write(w, http.StatusAccepted, map[string]any{"data": map[string]any{"recorded": true}})
}

func (h Handler) Mine(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	actor, ok := auth.ActorID(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return
	}
	metrics, err := h.Service.Mine(r.Context(), actor)
	if writeDomainError(w, r, err) {
		return
	}
	write(w, http.StatusOK, map[string]any{"data": metrics})
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ErrUnauthorized):
		writeError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
	case errors.Is(err, ErrForbidden):
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "analytics entitlement required")
	case errors.Is(err, ErrInvalid):
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid analytics input")
	case errors.Is(err, ErrNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "analytics subject not found")
	default:
		requestID := w.Header().Get("X-Request-ID")
		if requestID == "" {
			requestID = r.Header.Get("X-Request-ID")
		}
		log.Printf("analytics failure request_id=%s error_type=%T", requestID, err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
	return true
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
