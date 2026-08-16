package reviews

import (
	"context"
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

type Handler struct {
	Service Service
	Notify  func(context.Context, string)
}

func (h Handler) Project(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		out(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	actor, ok := auth.ActorID(r.Context())
	if !ok {
		out(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	project := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/projects/"), "/reviews")
	var in Input
	if !decode(w, r, &in) {
		return
	}
	v, err := h.Service.Create(r.Context(), actor, project, in)
	if handle(w, r, err) {
		return
	}
	write(w, 201, map[string]any{"data": v})
}
func (h Handler) Public(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		out(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	username := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/profiles/"), "/reviews")
	c, l, ok := pageInput(w, r)
	if !ok {
		return
	}
	v, err := h.Service.Public(r.Context(), username, c, l)
	if handle(w, r, err) {
		return
	}
	writePage(w, 200, v.Reviews, v.Trust)
}
func (h Handler) Given(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.ActorID(r.Context())
	if !ok {
		out(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	if r.Method != http.MethodGet {
		out(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	c, l, ok := pageInput(w, r)
	if !ok {
		return
	}
	p, err := h.Service.Given(r.Context(), actor, c, l)
	if handle(w, r, err) {
		return
	}
	writePage(w, 200, p, nil)
}
func (h Handler) Report(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.ActorID(r.Context())
	if !ok {
		out(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	if r.Method != http.MethodPost {
		out(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/reviews/"), "/reports")
	var in ReportInput
	if !decode(w, r, &in) {
		return
	}
	if handle(w, r, h.Service.Report(r.Context(), actor, id, in)) {
		return
	}
	w.WriteHeader(204)
}
func (h Handler) Admin(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.ActorID(r.Context())
	if !ok {
		out(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	if r.Method != http.MethodPost {
		out(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/reviews/"), "/"), "/")
	if len(parts) != 2 || (parts[1] != "hide" && parts[1] != "restore" && parts[1] != "reject" && parts[1] != "delete") {
		out(w, r, 404, "NOT_FOUND", "review target not found")
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if !decode(w, r, &in) {
		return
	}
	v, err := h.Service.Moderate(r.Context(), actor, parts[0], parts[1], in.Reason)
	if handle(w, r, err) {
		return
	}
	if h.Notify != nil && (parts[1] == "reject" || parts[1] == "delete") {
		h.Notify(r.Context(), v.ReviewerID)
	}
	write(w, 200, map[string]any{"data": v})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		var big *http.MaxBytesError
		if errors.As(err, &big) {
			out(w, r, 413, "PAYLOAD_TOO_LARGE", "review payload is too large")
		} else {
			out(w, r, 400, "VALIDATION_ERROR", "invalid review payload")
		}
		return false
	}
	if d.Decode(&struct{}{}) != io.EOF {
		out(w, r, 400, "VALIDATION_ERROR", "invalid review payload")
		return false
	}
	return true
}

type cursorJSON struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

func pageInput(w http.ResponseWriter, r *http.Request) (*Cursor, int, bool) {
	l := 20
	var err error
	if raw := r.URL.Query().Get("limit"); raw != "" {
		l, err = strconv.Atoi(raw)
		if err != nil || l < 1 || l > 50 {
			out(w, r, 400, "VALIDATION_ERROR", "invalid limit")
			return nil, 0, false
		}
	}
	raw := r.URL.Query().Get("cursor")
	if raw == "" {
		return nil, l, true
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	var v cursorJSON
	if len(raw) > 1024 || err != nil || json.Unmarshal(b, &v) != nil || v.At.IsZero() || !validUUID(v.ID) {
		out(w, r, 400, "VALIDATION_ERROR", "invalid cursor")
		return nil, 0, false
	}
	return &Cursor{At: v.At.UTC(), ID: v.ID}, l, true
}
func writePage(w http.ResponseWriter, status int, p Page, trust any) {
	var next *string
	if p.NextCursor != nil {
		b, _ := json.Marshal(cursorJSON{At: p.NextCursor.At, ID: p.NextCursor.ID})
		v := base64.RawURLEncoding.EncodeToString(b)
		next = &v
	}
	body := map[string]any{"data": p.Items, "page": map[string]any{"next_cursor": next, "has_more": next != nil}}
	if trust != nil {
		body["trust"] = trust
	}
	write(w, status, body)
}
func handle(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ErrNotFound):
		out(w, r, 404, "NOT_FOUND", "review target not found")
	case errors.Is(err, ErrConflict):
		out(w, r, 409, "CONFLICT", "review already exists")
	case errors.Is(err, ErrModeration):
		out(w, r, 422, "CONTENT_MODERATION_REJECTED", "Отзыв не прошёл автоматическую проверку. Уберите бессмысленный, повторяющийся или подозрительный текст и попробуйте снова.")
	case errors.Is(err, ErrInvalid), errors.Is(err, ErrIneligible):
		out(w, r, 422, "VALIDATION_ERROR", "review is not eligible or valid")
	case errors.Is(err, ErrForbidden):
		out(w, r, 403, "FORBIDDEN", "moderator role required")
	default:
		out(w, r, 500, "INTERNAL_ERROR", "internal server error")
	}
	return true
}
func out(w http.ResponseWriter, r *http.Request, s int, c, m string) {
	write(w, s, map[string]any{"error": map[string]any{"code": c, "message": m, "request_id": r.Header.Get("X-Request-ID")}})
}
func write(w http.ResponseWriter, s int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(s)
	_ = json.NewEncoder(w).Encode(v)
}
