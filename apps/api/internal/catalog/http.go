package catalog

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"freelance/apps/api/internal/auth"
)

type Handler struct{ Repository Repository }

func (h Handler) Categories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	items, err := h.Repository.CategoryTree(r.Context())
	if handleError(w, r, err) {
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	write(w, 200, map[string]any{"data": items})
}
func (h Handler) Category(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	slug := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/categories/"), "/")
	if slug == "" || strings.Contains(slug, "/") {
		writeError(w, r, 404, "NOT_FOUND", "category not found")
		return
	}
	item, err := h.Repository.Category(r.Context(), slug)
	if handleError(w, r, err) {
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	write(w, 200, map[string]any{"data": item})
}
func (h Handler) Skills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	items, err := h.Repository.SearchSkills(r.Context(), r.URL.Query().Get("q"))
	if handleError(w, r, err) {
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	write(w, 200, map[string]any{"data": items})
}
func (h Handler) AdminCategories(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.ActorID(r.Context())
	if !ok {
		writeError(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		items, err := h.Repository.AdminCategories(r.Context(), actor)
		if handleError(w, r, err) {
			return
		}
		write(w, 200, map[string]any{"data": items})
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	var in CategoryInput
	if !decode(w, r, &in) {
		return
	}
	item, err := h.Repository.CreateCategory(r.Context(), actor, in)
	if handleError(w, r, err) {
		return
	}
	write(w, 201, map[string]any{"data": item})
}
func (h Handler) AdminCategory(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.ActorID(r.Context())
	if !ok {
		writeError(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/categories/"), "/")
	var in CategoryInput
	if !uuidPattern.MatchString(strings.ToLower(id)) {
		writeError(w, r, 404, "NOT_FOUND", "catalog item not found")
		return
	}
	if r.Method == http.MethodDelete {
		if handleError(w, r, h.Repository.DeleteCategory(r.Context(), actor, id)) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPut {
		writeError(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if !decode(w, r, &in) {
		return
	}
	item, err := h.Repository.UpdateCategory(r.Context(), actor, id, in)
	if handleError(w, r, err) {
		return
	}
	write(w, 200, map[string]any{"data": item})
}
func (h Handler) AdminSkills(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.ActorID(r.Context())
	if !ok {
		writeError(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		items, err := h.Repository.AdminSkills(r.Context(), actor)
		if handleError(w, r, err) {
			return
		}
		write(w, 200, map[string]any{"data": items})
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	var in SkillInput
	if !decode(w, r, &in) {
		return
	}
	item, err := h.Repository.CreateSkill(r.Context(), actor, in)
	if handleError(w, r, err) {
		return
	}
	write(w, 201, map[string]any{"data": item})
}
func (h Handler) AdminSkill(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.ActorID(r.Context())
	if !ok {
		writeError(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/skills/"), "/")
	var in SkillInput
	if !uuidPattern.MatchString(strings.ToLower(id)) {
		writeError(w, r, 404, "NOT_FOUND", "catalog item not found")
		return
	}
	if r.Method == http.MethodDelete {
		if handleError(w, r, h.Repository.DeleteSkill(r.Context(), actor, id)) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPut {
		writeError(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if !decode(w, r, &in) {
		return
	}
	item, err := h.Repository.UpdateSkill(r.Context(), actor, id, in)
	if handleError(w, r, err) {
		return
	}
	write(w, 200, map[string]any{"data": item})
}
func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		writeError(w, r, 400, "VALIDATION_ERROR", "invalid catalog payload")
		return false
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		writeError(w, r, 400, "VALIDATION_ERROR", "invalid catalog payload")
		return false
	}
	return true
}
func handleError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, r, 404, "NOT_FOUND", "catalog item not found")
	case errors.Is(err, ErrForbidden):
		writeError(w, r, 404, "NOT_FOUND", "catalog item not found")
	case errors.Is(err, ErrInvalid):
		writeError(w, r, 422, "VALIDATION_ERROR", "invalid catalog input")
	default:
		writeError(w, r, 500, "INTERNAL_ERROR", "internal server error")
	}
	return true
}
func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	id := w.Header().Get("X-Request-ID")
	if id == "" {
		id = r.Header.Get("X-Request-ID")
	}
	write(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "request_id": id}})
}
func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
