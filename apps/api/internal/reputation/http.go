package reputation

import (
	"encoding/json"
	"errors"
	"freelance/apps/api/internal/auth"
	"io"
	"log"
	"net/http"
	"strings"
)

type Handler struct{ Service Service }

func (h Handler) OwnerCollection(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.ActorID(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		items, err := h.Service.ListOwned(r.Context(), actor)
		if handleError(w, r, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": items})
	case http.MethodPost:
		var input CreateRequest
		if !decode(w, r, &input) {
			return
		}
		item, err := h.Service.Create(r.Context(), actor, input)
		if handleError(w, r, err) {
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"data": item})
	default:
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (h Handler) OwnerItem(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.ActorID(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return
	}
	remainder := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/me/external-reputations/"), "/")
	parts := strings.Split(remainder, "/")
	id := parts[0]
	if !validUUID(id) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "external reputation not found")
		return
	}
	if len(parts) == 2 && parts[1] == "verification" {
		switch r.Method {
		case http.MethodPost:
			var input StartVerificationRequest
			if !decode(w, r, &input) {
				return
			}
			value, err := h.Service.StartVerification(r.Context(), actor, id, input)
			if handleError(w, r, err) {
				return
			}
			writeJSON(w, http.StatusCreated, map[string]any{"data": value})
		case http.MethodGet:
			value, err := h.Service.GetVerification(r.Context(), actor, id)
			if handleError(w, r, err) {
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"data": value})
		default:
			writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		}
		return
	}
	if len(parts) != 1 {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "external reputation not found")
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var input PatchRequest
		if !decode(w, r, &input) {
			return
		}
		item, err := h.Service.Update(r.Context(), actor, id, input)
		if handleError(w, r, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": item})
	case http.MethodDelete:
		if handleError(w, r, h.Service.Delete(r.Context(), actor, id)) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (h Handler) Admin(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.ActorID(r.Context())
	if !ok {
		writeError(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	remainder := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/external-reputations"), "/")
	if remainder == "" {
		if r.Method != http.MethodGet {
			writeError(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		if status := r.URL.Query().Get("status"); status != "" && status != "PENDING" {
			writeError(w, r, 422, "VALIDATION_ERROR", "invalid status")
			return
		}
		items, err := h.Service.ListPending(r.Context(), actor)
		if handleError(w, r, err) {
			return
		}
		writeJSON(w, 200, map[string]any{"data": items})
		return
	}
	parts := strings.Split(remainder, "/")
	if len(parts) != 2 || !validUUID(parts[0]) || (parts[1] != "verify" && parts[1] != "reject") {
		writeError(w, r, 404, "NOT_FOUND", "external reputation not found")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	var input DecisionRequest
	if !decode(w, r, &input) {
		return
	}
	item, err := h.Service.Decide(r.Context(), actor, parts[0], parts[1], input)
	if handleError(w, r, err) {
		return
	}
	writeJSON(w, 200, map[string]any{"data": item})
}

func (h Handler) Public(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/profiles/")
	username := strings.TrimSuffix(path, "/external-reputations")
	items, err := h.Service.ListPublic(r.Context(), username)
	if handleError(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
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

func writeDecodeError(w http.ResponseWriter, r *http.Request, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, r, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "external reputation payload is too large")
		return
	}
	writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid external reputation payload")
}

func handleError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ErrUnauthorized):
		writeError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
	case errors.Is(err, ErrNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "external reputation not found")
	case errors.Is(err, ErrConflict):
		writeError(w, r, http.StatusConflict, "CONFLICT", "external reputation already exists")
	case errors.Is(err, ErrInvalid):
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid external reputation")
	case errors.Is(err, ErrForbidden):
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "moderator role required")
	case errors.Is(err, ErrInvalidState):
		writeError(w, r, http.StatusConflict, "INVALID_STATE", "invalid external reputation state")
	default:
		log.Printf("external reputation repository failure request_id=%s error_type=%T", requestID(w, r), err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
	return true
}

func requestID(w http.ResponseWriter, r *http.Request) string {
	if value := w.Header().Get("X-Request-ID"); value != "" {
		return value
	}
	return r.Header.Get("X-Request-ID")
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "request_id": requestID(w, r)}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
