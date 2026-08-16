package projects

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"freelance/apps/api/internal/auth"
)

type Handler struct {
	Repository Repository
	Search     SearchEngine
}

func (h Handler) PublicCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	cursor, limit, ok := pageInput(w, r)
	if !ok {
		return
	}
	filter := Filter{Q: strings.TrimSpace(r.URL.Query().Get("q")), Category: strings.ToLower(strings.TrimSpace(r.URL.Query().Get("category"))), BudgetType: strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("budget_type"))), ExperienceLevel: strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("experience_level")))}
	if raw := strings.TrimSpace(r.URL.Query().Get("min_budget_kopecks")); raw != "" {
		value, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || value < 0 {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid minimum budget")
			return
		}
		filter.MinBudgetKopecks = &value
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("deadline_before")); raw != "" {
		value, parseErr := time.Parse("2006-01-02", raw)
		if parseErr != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid deadline filter")
			return
		}
		endOfDay := value.UTC().Add(24*time.Hour - time.Nanosecond)
		filter.DeadlineBefore = &endOfDay
	}
	page, err := h.Search.ListPublic(r.Context(), filter, cursor, limit)
	if writeDomainError(w, r, err) {
		return
	}
	writePage(w, page)
}

func (h Handler) PublicItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	reference := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/projects/"), "/")
	if reference == "" || strings.Contains(reference, "/") || len(reference) > 240 || (!validUUID(reference) && !slugPattern.MatchString(reference)) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "project not found")
		return
	}
	item, err := h.Search.GetPublic(r.Context(), strings.ToLower(reference))
	if writeDomainError(w, r, err) {
		return
	}
	write(w, http.StatusOK, map[string]any{"data": item})
}

func (h Handler) OwnerCollection(w http.ResponseWriter, r *http.Request) {
	actorID, ok := auth.ActorID(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		cursor, limit, valid := pageInput(w, r)
		if !valid {
			return
		}
		page, err := h.Repository.ListOwned(r.Context(), actorID, cursor, limit)
		if writeDomainError(w, r, err) {
			return
		}
		writePage(w, page)
	case http.MethodPost:
		var input CreateRequest
		if !decodeBody(w, r, &input) {
			return
		}
		item, err := h.Repository.Create(r.Context(), actorID, input)
		if writeDomainError(w, r, err) {
			return
		}
		write(w, http.StatusCreated, map[string]any{"data": item})
	default:
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (h Handler) OwnerItem(w http.ResponseWriter, r *http.Request) {
	actorID, ok := auth.ActorID(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/me/projects/"), "/"), "/")
	if len(parts) < 1 || !validUUID(parts[0]) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "project not found")
		return
	}
	if len(parts) == 2 && r.Method == http.MethodPost && oneOf(parts[1], "publish", "make-public", "cancel", "complete") {
		if !emptyBody(w, r) {
			return
		}
		if parts[1] == "complete" && len(r.Header.Get("Idempotency-Key")) > 128 {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid idempotency key")
			return
		}
		item, err := h.Repository.Transition(r.Context(), actorID, parts[0], parts[1])
		if writeDomainError(w, r, err) {
			return
		}
		write(w, http.StatusOK, map[string]any{"data": item})
		return
	}
	if len(parts) != 1 {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "project not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		item, err := h.Repository.GetOwned(r.Context(), actorID, parts[0])
		if writeDomainError(w, r, err) {
			return
		}
		write(w, http.StatusOK, map[string]any{"data": item})
	case http.MethodPatch:
		var patch PatchRequest
		if !decodeBody(w, r, &patch) {
			return
		}
		item, err := h.Repository.Update(r.Context(), actorID, parts[0], patch)
		if writeDomainError(w, r, err) {
			return
		}
		write(w, http.StatusOK, map[string]any{"data": item})
	case http.MethodDelete:
		if writeDomainError(w, r, h.Repository.Delete(r.Context(), actorID, parts[0])) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func decodeBody(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
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
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "action payload must be empty")
		return false
	}
	return true
}
func pageInput(w http.ResponseWriter, r *http.Request) (*Cursor, int, bool) {
	cursor, err := decodeCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid cursor")
		return nil, 0, false
	}
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 50 {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid limit")
			return nil, 0, false
		}
	}
	return cursor, limit, true
}

type cursorPayload struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

func encodeCursor(cursor Cursor) string {
	payload, _ := json.Marshal(cursorPayload{At: cursor.At.UTC(), ID: cursor.ID})
	return base64.RawURLEncoding.EncodeToString(payload)
}
func decodeCursor(value string) (*Cursor, error) {
	if value == "" {
		return nil, nil
	}
	if len(value) > 1024 {
		return nil, errors.New("invalid cursor")
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	var cursor cursorPayload
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil || cursor.At.IsZero() || !validUUID(cursor.ID) {
		return nil, errors.New("invalid cursor")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, errors.New("invalid cursor")
	}
	return &Cursor{At: cursor.At.UTC(), ID: normalizeID(cursor.ID)}, nil
}
func writePage(w http.ResponseWriter, page Page) {
	var next *string
	if page.NextCursor != nil {
		encoded := encodeCursor(*page.NextCursor)
		next = &encoded
	}
	write(w, http.StatusOK, map[string]any{"data": page.Items, "page": map[string]any{"next_cursor": next, "has_more": next != nil}})
}
func writeDecodeError(w http.ResponseWriter, r *http.Request, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, r, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "project payload is too large")
		return
	}
	writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid project payload")
}
func writeDomainError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "project not found")
	case errors.Is(err, ErrConflict):
		writeError(w, r, http.StatusConflict, "CONFLICT", "project slug already exists")
	case errors.Is(err, ErrInvalidState):
		writeError(w, r, http.StatusConflict, "INVALID_STATE", "project state transition is not allowed")
	case errors.Is(err, ErrCustomerIneligible):
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "customer capability is required")
	case errors.Is(err, ErrInvalidInput), errors.Is(err, ErrInvalidReference):
		message := strings.TrimPrefix(strings.TrimPrefix(err.Error(), ErrInvalidInput.Error()+": "), ErrInvalidReference.Error()+": ")
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", message)
	default:
		requestID := w.Header().Get("X-Request-ID")
		if requestID == "" {
			requestID = r.Header.Get("X-Request-ID")
		}
		log.Printf("project operation failure request_id=%s error_type=%T", requestID, err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
	return true
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
