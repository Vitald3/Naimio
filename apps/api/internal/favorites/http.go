package favorites

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"freelance/apps/api/internal/auth"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Handler struct{ Repository Repository }
type cp struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

func (h Handler) Collection(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.ActorID(r.Context())
	if !ok {
		outErr(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	if r.Method != http.MethodGet {
		outErr(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	c, l, ok := parsePage(w, r)
	if !ok {
		return
	}
	p, err := h.Repository.List(r.Context(), actor, r.URL.Query().Get("type"), c, l)
	if handle(w, r, err) {
		return
	}
	var next *string
	if p.NextCursor != nil {
		b, _ := json.Marshal(cp{At: p.NextCursor.At, ID: p.NextCursor.ID})
		s := base64.RawURLEncoding.EncodeToString(b)
		next = &s
	}
	writeJSON(w, 200, map[string]any{"data": p.Items, "page": map[string]any{"next_cursor": next, "has_more": next != nil}})
}
func (h Handler) Item(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.ActorID(r.Context())
	if !ok {
		outErr(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/me/favorites/"), "/"), "/")
	if len(parts) != 2 {
		outErr(w, r, 404, "NOT_FOUND", "favorite not found")
		return
	}
	switch r.Method {
	case http.MethodPut:
		v, err := h.Repository.Put(r.Context(), actor, parts[0], parts[1])
		if handle(w, r, err) {
			return
		}
		writeJSON(w, 200, map[string]any{"data": v})
	case http.MethodDelete:
		if handle(w, r, h.Repository.Delete(r.Context(), actor, parts[0], parts[1])) {
			return
		}
		w.WriteHeader(204)
	default:
		outErr(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}
func parsePage(w http.ResponseWriter, r *http.Request) (*Cursor, int, bool) {
	l := 20
	var err error
	if raw := r.URL.Query().Get("limit"); raw != "" {
		l, err = strconv.Atoi(raw)
		if err != nil || l < 1 || l > 50 {
			outErr(w, r, 400, "VALIDATION_ERROR", "invalid limit")
			return nil, 0, false
		}
	}
	raw := r.URL.Query().Get("cursor")
	if raw == "" {
		return nil, l, true
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	var v cp
	if len(raw) > 1024 || err != nil || json.Unmarshal(b, &v) != nil || v.At.IsZero() || v.ID == "" {
		outErr(w, r, 400, "VALIDATION_ERROR", "invalid cursor")
		return nil, 0, false
	}
	return &Cursor{At: v.At.UTC(), ID: v.ID}, l, true
}
func handle(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrNotFound) {
		outErr(w, r, 404, "NOT_FOUND", "favorite target not found")
	} else if errors.Is(err, ErrInvalid) {
		outErr(w, r, 422, "VALIDATION_ERROR", "invalid favorite")
	} else {
		outErr(w, r, 500, "INTERNAL_ERROR", "internal server error")
	}
	return true
}
func outErr(w http.ResponseWriter, r *http.Request, s int, c, m string) {
	id := w.Header().Get("X-Request-ID")
	if id == "" {
		id = r.Header.Get("X-Request-ID")
	}
	writeJSON(w, s, map[string]any{"error": map[string]any{"code": c, "message": m, "request_id": id}})
}
func writeJSON(w http.ResponseWriter, s int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(s)
	_ = json.NewEncoder(w).Encode(v)
}
