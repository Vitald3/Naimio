package portfolio

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

type Handler struct{ Repository Repository }

func (h Handler) PublicList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/profiles/")
	username, found := strings.CutSuffix(path, "/portfolio")
	if !found || username == "" || strings.Contains(username, "/") {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "portfolio not found")
		return
	}
	cursor, err := decodeCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid cursor")
		return
	}
	limit := 20
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		limit, err = strconv.Atoi(rawLimit)
		if err != nil || limit < 1 || limit > 50 {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid limit")
			return
		}
	}
	page, err := h.Repository.ListPublic(r.Context(), username, cursor, limit)
	if writeDomainError(w, r, err) {
		return
	}
	var nextCursor *string
	if page.NextCursor != nil {
		encoded := encodeCursor(*page.NextCursor)
		nextCursor = &encoded
	}
	write(w, http.StatusOK, map[string]any{"data": page.Items, "page": map[string]any{"next_cursor": nextCursor, "has_more": nextCursor != nil}})
}

func (h Handler) Collection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	actorID, ok := auth.ActorID(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		items, err := h.Repository.ListOwned(r.Context(), actorID)
		if writeDomainError(w, r, err) {
			return
		}
		write(w, http.StatusOK, map[string]any{"data": items})
		return
	}
	var input WriteRequest
	if !decodeBody(w, r, &input) {
		return
	}
	item, err := h.Repository.Create(r.Context(), actorID, input)
	if writeDomainError(w, r, err) {
		return
	}
	write(w, http.StatusCreated, map[string]any{"data": item})
}

func (h Handler) Item(w http.ResponseWriter, r *http.Request) {
	actorID, ok := auth.ActorID(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/me/portfolio/"), "/"), "/")
	if len(parts) == 1 && validUUID(parts[0]) {
		h.item(w, r, actorID, parts[0])
		return
	}
	if len(parts) == 2 && validUUID(parts[0]) && parts[1] == "media" && r.Method == http.MethodPost {
		h.attachMedia(w, r, actorID, parts[0])
		return
	}
	if len(parts) == 3 && validUUID(parts[0]) && parts[1] == "media" && validUUID(parts[2]) && r.Method == http.MethodDelete {
		if writeDomainError(w, r, h.Repository.DetachMedia(r.Context(), actorID, parts[0], parts[2])) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeError(w, r, http.StatusNotFound, "NOT_FOUND", "portfolio item not found")
}

func (h Handler) item(w http.ResponseWriter, r *http.Request, actorID, itemID string) {
	switch r.Method {
	case http.MethodGet:
		item, err := h.Repository.GetOwned(r.Context(), actorID, itemID)
		if writeDomainError(w, r, err) {
			return
		}
		write(w, http.StatusOK, map[string]any{"data": item})
	case http.MethodPatch:
		var input WriteRequest
		if !decodeBody(w, r, &input) {
			return
		}
		item, err := h.Repository.Update(r.Context(), actorID, itemID, input)
		if writeDomainError(w, r, err) {
			return
		}
		write(w, http.StatusOK, map[string]any{"data": item})
	case http.MethodDelete:
		if writeDomainError(w, r, h.Repository.Delete(r.Context(), actorID, itemID)) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (h Handler) attachMedia(w http.ResponseWriter, r *http.Request, actorID, itemID string) {
	var input struct {
		MediaObjectID string `json:"media_object_id"`
		SortOrder     int    `json:"sort_order"`
	}
	if !decodeBody(w, r, &input) {
		return
	}
	item, err := h.Repository.AttachMedia(r.Context(), actorID, itemID, input.MediaObjectID, input.SortOrder)
	if writeDomainError(w, r, err) {
		return
	}
	write(w, http.StatusOK, map[string]any{"data": item})
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

func writeDecodeError(w http.ResponseWriter, r *http.Request, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, r, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "portfolio payload is too large")
		return
	}
	writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid portfolio payload")
}

func writeDomainError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "portfolio item not found")
	case errors.Is(err, ErrConflict):
		writeError(w, r, http.StatusConflict, "CONFLICT", "portfolio slug already exists")
	case errors.Is(err, ErrItemLimit):
		writeError(w, r, http.StatusConflict, "PLAN_LIMIT_REACHED", "Достигнут лимит работ в портфолио для текущего плана. Удалите кейс или оформите PRO для расширенного лимита.")
	case errors.Is(err, ErrMediaLimit):
		writeError(w, r, http.StatusConflict, "PLAN_LIMIT_REACHED", "Достигнут лимит медиафайлов в одной работе для текущего плана. Уберите лишние файлы или оформите PRO.")
	case errors.Is(err, ErrLimit):
		writeError(w, r, http.StatusConflict, "PLAN_LIMIT_REACHED", "Достигнут лимит портфолио для текущего плана")
	case errors.Is(err, ErrInvalidInput), errors.Is(err, ErrInvalidReference):
		message := strings.TrimPrefix(err.Error(), ErrInvalidInput.Error()+": ")
		message = strings.TrimPrefix(message, ErrInvalidReference.Error()+": ")
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", message)
	default:
		requestID := w.Header().Get("X-Request-ID")
		if requestID == "" {
			requestID = r.Header.Get("X-Request-ID")
		}
		log.Printf("portfolio repository failure request_id=%s error_type=%T", requestID, err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
	return true
}

type cursorPayload struct {
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

func encodeCursor(cursor Cursor) string {
	payload, _ := json.Marshal(cursorPayload{SortOrder: cursor.SortOrder, CreatedAt: cursor.CreatedAt.UTC(), ID: cursor.ID})
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
	if err := decoder.Decode(&cursor); err != nil || !validUUID(cursor.ID) || cursor.CreatedAt.IsZero() || cursor.SortOrder < 0 || cursor.SortOrder > 10000 {
		return nil, errors.New("invalid cursor")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, errors.New("invalid cursor")
	}
	return &Cursor{SortOrder: cursor.SortOrder, CreatedAt: cursor.CreatedAt.UTC(), ID: cursor.ID}, nil
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
