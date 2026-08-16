package profiles

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"freelance/apps/api/internal/auth"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
)

type Handler struct{ Repository Repository }

func (h Handler) Public(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	username := strings.TrimPrefix(r.URL.Path, "/api/v1/profiles/")
	p, err := h.Repository.Public(r.Context(), username)
	if errors.Is(err, ErrNotFound) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "profile not found")
		return
	}
	if err != nil {
		writeRepositoryError(w, r, err)
		return
	}
	write(w, http.StatusOK, map[string]any{"data": p})
}

func (h Handler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(query)) > 120 {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid search query")
		return
	}
	cursor, limit, valid := publicPageInput(w, r)
	if !valid {
		return
	}
	page, err := h.Repository.PublicList(r.Context(), query, cursor, limit)
	if err != nil {
		writeRepositoryError(w, r, err)
		return
	}
	var next *string
	if page.NextCursor != nil {
		raw, _ := json.Marshal(page.NextCursor)
		encoded := base64.RawURLEncoding.EncodeToString(raw)
		next = &encoded
	}
	write(w, http.StatusOK, map[string]any{"data": page.Items, "page": map[string]any{"next_cursor": next, "has_more": next != nil}})
}

func publicPageInput(w http.ResponseWriter, r *http.Request) (*PublicCursor, int, bool) {
	limit := 20
	var err error
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 50 {
			writeError(w, r, 400, "VALIDATION_ERROR", "invalid limit")
			return nil, 0, false
		}
	}
	raw := r.URL.Query().Get("cursor")
	if raw == "" {
		return nil, limit, true
	}
	if len(raw) > 1024 {
		writeError(w, r, 400, "VALIDATION_ERROR", "invalid cursor")
		return nil, 0, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	var cursor PublicCursor
	if err != nil || json.Unmarshal(payload, &cursor) != nil || cursor.Username == "" || !uuidPattern.MatchString(strings.ToLower(cursor.ID)) {
		writeError(w, r, 400, "VALIDATION_ERROR", "invalid cursor")
		return nil, 0, false
	}
	cursor.Username = strings.ToLower(cursor.Username)
	cursor.ID = strings.ToLower(cursor.ID)
	return &cursor, limit, true
}

func (h Handler) Update(w http.ResponseWriter, r *http.Request) {
	actorID, ok := auth.ActorID(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		profile, err := h.Repository.Current(r.Context(), actorID)
		if writeProfileError(w, r, err) {
			return
		}
		write(w, http.StatusOK, map[string]any{"data": profile})
	case http.MethodPatch:
		var input UpdateRequest
		if !decodeBody(w, r, &input) {
			return
		}
		profile, err := h.Repository.Update(r.Context(), actorID, input)
		if writeProfileError(w, r, err) {
			return
		}
		write(w, http.StatusOK, map[string]any{"data": profile})
	default:
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (h Handler) ReplaceCategories(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Categories *[]CategorySelection `json:"categories"`
	}
	h.replace(w, r, &input, func() error {
		if input.Categories == nil {
			return invalid("categories are required")
		}
		return nil
	}, func(ctx context.Context, actorID string) (Profile, error) {
		return h.Repository.ReplaceCategories(ctx, actorID, *input.Categories)
	})
}

func (h Handler) ReplaceSkills(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Skills *[]SkillSelection `json:"skills"`
	}
	h.replace(w, r, &input, func() error {
		if input.Skills == nil {
			return invalid("skills are required")
		}
		return nil
	}, func(ctx context.Context, actorID string) (Profile, error) {
		return h.Repository.ReplaceSkills(ctx, actorID, *input.Skills)
	})
}

func (h Handler) ReplaceLanguages(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Languages *[]Language `json:"languages"`
	}
	h.replace(w, r, &input, func() error {
		if input.Languages == nil {
			return invalid("languages are required")
		}
		return nil
	}, func(ctx context.Context, actorID string) (Profile, error) {
		return h.Repository.ReplaceLanguages(ctx, actorID, *input.Languages)
	})
}

func (h Handler) replace(w http.ResponseWriter, r *http.Request, input any, validate func() error, update func(context.Context, string) (Profile, error)) {
	if r.Method != http.MethodPut {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	actorID, ok := auth.ActorID(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return
	}
	if !decodeBody(w, r, input) {
		return
	}
	if err := validate(); err != nil {
		writeProfileError(w, r, err)
		return
	}
	profile, err := update(r.Context(), actorID)
	if writeProfileError(w, r, err) {
		return
	}
	write(w, http.StatusOK, map[string]any{"data": profile})
}

func decodeBody(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
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

func writeProfileError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "profile not found")
	case errors.Is(err, ErrInvalidInput), errors.Is(err, ErrInvalidReference):
		message := strings.TrimPrefix(err.Error(), ErrInvalidInput.Error()+": ")
		message = strings.TrimPrefix(message, ErrInvalidReference.Error()+": ")
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", message)
	default:
		writeRepositoryError(w, r, err)
	}
	return true
}

func writeRepositoryError(w http.ResponseWriter, r *http.Request, err error) {
	requestID := w.Header().Get("X-Request-ID")
	if requestID == "" {
		requestID = r.Header.Get("X-Request-ID")
	}
	log.Printf("profile repository failure request_id=%s error_type=%T", requestID, err)
	writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}

func writeDecodeError(w http.ResponseWriter, r *http.Request, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, r, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "profile payload is too large")
		return
	}
	writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid profile payload")
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
