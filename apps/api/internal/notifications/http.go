package notifications

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"freelance/apps/api/internal/auth"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Handler struct{ Service Service }

func (h Handler) Collection(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.ActorID(r.Context())
	if !ok {
		writeError(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	c, l, valid := pageInput(r)
	if !valid {
		writeError(w, r, 400, "VALIDATION_ERROR", "invalid notification cursor")
		return
	}
	v, e := h.Service.List(r.Context(), u, c, l)
	if fail(w, r, e) {
		return
	}
	var next *string
	if v.Next != nil {
		raw, _ := json.Marshal(struct {
			At time.Time `json:"at"`
			ID string    `json:"id"`
		}{v.Next.At, v.Next.ID})
		value := base64.RawURLEncoding.EncodeToString(raw)
		next = &value
	}
	write(w, 200, map[string]any{"data": v.Items, "page": map[string]any{"next_cursor": next, "has_more": next != nil}})
}
func pageInput(r *http.Request) (*Cursor, int, bool) {
	l := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 1 || n > 100 {
			return nil, 0, false
		}
		l = n
	}
	raw := r.URL.Query().Get("before")
	if raw == "" {
		return nil, l, true
	}
	if len(raw) > 1024 {
		return nil, 0, false
	}
	b, e := base64.RawURLEncoding.DecodeString(raw)
	var v struct {
		At time.Time `json:"at"`
		ID string    `json:"id"`
	}
	if e != nil || json.Unmarshal(b, &v) != nil || v.At.IsZero() || !validID(v.ID) {
		return nil, 0, false
	}
	return &Cursor{At: v.At.UTC(), ID: v.ID}, l, true
}
func (h Handler) Item(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.ActorID(r.Context())
	if !ok {
		writeError(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/notifications/"), "/")
	if path == "read-all" {
		if fail(w, r, h.Service.ReadAll(r.Context(), u)) {
			return
		}
		w.WriteHeader(204)
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[1] != "read" {
		writeError(w, r, 404, "NOT_FOUND", "notification not found")
		return
	}
	if fail(w, r, h.Service.Read(r.Context(), u, parts[0])) {
		return
	}
	w.WriteHeader(204)
}
func (h Handler) Preferences(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.ActorID(r.Context())
	if !ok {
		writeError(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		v, e := h.Service.Preferences(r.Context(), u)
		if fail(w, r, e) {
			return
		}
		write(w, 200, map[string]any{"data": v})
	case http.MethodPut:
		var in struct {
			Preferences []Preference `json:"preferences"`
		}
		if !decode(w, r, &in) {
			return
		}
		v, e := h.Service.ReplacePreferences(r.Context(), u, in.Preferences)
		if fail(w, r, e) {
			return
		}
		write(w, 200, map[string]any{"data": v})
	default:
		writeError(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(&struct{}{}) != io.EOF {
		writeError(w, r, 400, "VALIDATION_ERROR", "invalid notification preferences")
		return false
	}
	return true
}
func fail(w http.ResponseWriter, r *http.Request, e error) bool {
	if e == nil {
		return false
	}
	if errors.Is(e, ErrNotFound) {
		writeError(w, r, 404, "NOT_FOUND", "notification not found")
	} else if e.Error() == "invalid preferences" {
		writeError(w, r, 422, "VALIDATION_ERROR", "invalid notification preferences")
	} else {
		writeError(w, r, 500, "INTERNAL_ERROR", "internal server error")
	}
	return true
}
func writeError(w http.ResponseWriter, r *http.Request, s int, c, m string) {
	write(w, s, map[string]any{"error": map[string]any{"code": c, "message": m, "request_id": r.Header.Get("X-Request-ID")}})
}
func write(w http.ResponseWriter, s int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(s)
	_ = json.NewEncoder(w).Encode(v)
}
