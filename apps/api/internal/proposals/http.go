package proposals

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

type Handler struct{ Repository Repository }

func (h Handler) PublicProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errOut(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	actor, ok := auth.ActorID(r.Context())
	if !ok {
		errOut(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	projectID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/projects/"), "/proposals")
	if !uuidPattern.MatchString(strings.ToLower(projectID)) || strings.Contains(projectID, "/") {
		errOut(w, r, 404, "NOT_FOUND", "project not found")
		return
	}
	var in Input
	if !body(w, r, &in) {
		return
	}
	v, err := h.Repository.Submit(r.Context(), actor, projectID, in)
	if domain(w, r, err) {
		return
	}
	write(w, 201, map[string]any{"data": v})
}
func (h Handler) Mine(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.ActorID(r.Context())
	if !ok {
		errOut(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	if r.Method != http.MethodGet {
		errOut(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	c, l, ok := paging(w, r)
	if !ok {
		return
	}
	p, err := h.Repository.ListMine(r.Context(), actor, c, l)
	if domain(w, r, err) {
		return
	}
	pageOut(w, p)
}
func (h Handler) MineItem(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.ActorID(r.Context())
	if !ok {
		errOut(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/me/proposals/"), "/"), "/")
	if len(parts) > 0 && !uuidPattern.MatchString(strings.ToLower(parts[0])) {
		errOut(w, r, 404, "NOT_FOUND", "proposal not found")
		return
	}
	if len(parts) == 2 && parts[1] == "withdraw" && r.Method == http.MethodPost {
		if !empty(w, r) {
			return
		}
		v, err := h.Repository.Withdraw(r.Context(), actor, parts[0])
		if domain(w, r, err) {
			return
		}
		write(w, 200, map[string]any{"data": v})
		return
	}
	if len(parts) != 1 {
		errOut(w, r, 404, "NOT_FOUND", "proposal not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		v, err := h.Repository.GetMine(r.Context(), actor, parts[0])
		if domain(w, r, err) {
			return
		}
		write(w, 200, map[string]any{"data": v})
	case http.MethodPatch:
		var in Input
		if !body(w, r, &in) {
			return
		}
		v, err := h.Repository.Update(r.Context(), actor, parts[0], in)
		if domain(w, r, err) {
			return
		}
		write(w, 200, map[string]any{"data": v})
	default:
		errOut(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}
func (h Handler) CustomerProject(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.ActorID(r.Context())
	if !ok {
		errOut(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/me/projects/"), "/"), "/")
	if len(parts) > 0 && !uuidPattern.MatchString(strings.ToLower(parts[0])) {
		errOut(w, r, 404, "NOT_FOUND", "proposal not found")
		return
	}
	if len(parts) == 2 && parts[1] == "proposals" && r.Method == http.MethodGet {
		c, l, valid := paging(w, r)
		if !valid {
			return
		}
		p, err := h.Repository.ListForProject(r.Context(), actor, parts[0], c, l)
		if domain(w, r, err) {
			return
		}
		pageOut(w, p)
		return
	}
	if len(parts) == 4 && parts[1] == "proposals" && r.Method == http.MethodPost && (parts[3] == "shortlist" || parts[3] == "accept" || parts[3] == "reject") {
		if !uuidPattern.MatchString(strings.ToLower(parts[2])) {
			errOut(w, r, 404, "NOT_FOUND", "proposal not found")
			return
		}
		if !empty(w, r) {
			return
		}
		if parts[3] == "accept" && len(r.Header.Get("Idempotency-Key")) > 128 {
			errOut(w, r, 400, "VALIDATION_ERROR", "invalid idempotency key")
			return
		}
		v, err := h.Repository.Act(r.Context(), actor, parts[0], parts[2], parts[3])
		if domain(w, r, err) {
			return
		}
		write(w, 200, map[string]any{"data": v})
		return
	}
	errOut(w, r, 404, "NOT_FOUND", "proposal not found")
}
func body(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		errOut(w, r, 400, "VALIDATION_ERROR", "invalid proposal payload")
		return false
	}
	if e := d.Decode(&struct{}{}); e != io.EOF {
		errOut(w, r, 400, "VALIDATION_ERROR", "invalid proposal payload")
		return false
	}
	return true
}
func empty(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	b, e := io.ReadAll(r.Body)
	if e != nil {
		return false
	}
	if v := strings.TrimSpace(string(b)); v != "" && v != "{}" {
		errOut(w, r, 400, "VALIDATION_ERROR", "action payload must be empty")
		return false
	}
	return true
}

type cp struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

func paging(w http.ResponseWriter, r *http.Request) (*Cursor, int, bool) {
	l := 20
	var e error
	if raw := r.URL.Query().Get("limit"); raw != "" {
		l, e = strconv.Atoi(raw)
		if e != nil || l < 1 || l > 50 {
			errOut(w, r, 400, "VALIDATION_ERROR", "invalid limit")
			return nil, 0, false
		}
	}
	raw := r.URL.Query().Get("cursor")
	if raw == "" {
		return nil, l, true
	}
	if len(raw) > 1024 {
		errOut(w, r, 400, "VALIDATION_ERROR", "invalid cursor")
		return nil, 0, false
	}
	b, e := base64.RawURLEncoding.DecodeString(raw)
	var p cp
	if e != nil || json.Unmarshal(b, &p) != nil || p.At.IsZero() || p.ID == "" {
		errOut(w, r, 400, "VALIDATION_ERROR", "invalid cursor")
		return nil, 0, false
	}
	return &Cursor{At: p.At.UTC(), ID: p.ID}, l, true
}
func pageOut(w http.ResponseWriter, p Page) {
	var next *string
	if p.NextCursor != nil {
		b, _ := json.Marshal(cp{At: p.NextCursor.At.UTC(), ID: p.NextCursor.ID})
		s := base64.RawURLEncoding.EncodeToString(b)
		next = &s
	}
	write(w, 200, map[string]any{"data": p.Items, "page": map[string]any{"next_cursor": next, "has_more": next != nil}})
}
func domain(w http.ResponseWriter, r *http.Request, e error) bool {
	if e == nil {
		return false
	}
	switch {
	case errors.Is(e, ErrNotFound):
		errOut(w, r, 404, "NOT_FOUND", "proposal not found")
	case errors.Is(e, ErrIneligible):
		errOut(w, r, 403, "FORBIDDEN", "freelancer is not eligible")
	case errors.Is(e, ErrConflict):
		errOut(w, r, 409, "CONFLICT", "proposal already exists or was accepted")
	case errors.Is(e, ErrInvalidState):
		errOut(w, r, 409, "INVALID_STATE", "proposal state transition is not allowed")
	case errors.Is(e, ErrInvalid):
		errOut(w, r, 422, "VALIDATION_ERROR", "invalid proposal")
	default:
		errOut(w, r, 500, "INTERNAL_ERROR", "internal server error")
	}
	return true
}
func errOut(w http.ResponseWriter, r *http.Request, s int, c, m string) {
	id := w.Header().Get("X-Request-ID")
	if id == "" {
		id = r.Header.Get("X-Request-ID")
	}
	write(w, s, map[string]any{"error": map[string]any{"code": c, "message": m, "request_id": id}})
}
func write(w http.ResponseWriter, s int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(s)
	_ = json.NewEncoder(w).Encode(v)
}
