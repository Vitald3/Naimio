package growth

import (
	"encoding/json"
	"errors"
	"freelance/apps/api/internal/auth"
	"io"
	"net/http"
	"strings"
)

type Handler struct{ Service Service }

func (h Handler) Invites(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		problem(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	var in InviteInput
	if !decode(w, r, &in) {
		return
	}
	v, e := h.Service.CreateInvite(r.Context(), a, in)
	if handled(w, r, e) {
		return
	}
	write(w, 201, map[string]any{"data": v})
}
func (h Handler) PublicInvite(w http.ResponseWriter, r *http.Request) {
	path := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/invites/"), "/"), "/")
	if len(path) != 2 || (path[1] != "preview" && path[1] != "accept") {
		problem(w, r, 404, "NOT_FOUND", "invite not found")
		return
	}
	if path[1] == "preview" {
		if r.Method != http.MethodGet {
			problem(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		v, e := h.Service.Preview(r.Context(), path[0])
		if handled(w, r, e) {
			return
		}
		write(w, 200, map[string]any{"data": v})
		return
	}
	a, ok := actor(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		problem(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if !empty(w, r) {
		return
	}
	v, e := h.Service.Accept(r.Context(), a, path[0], r.Header.Get("Idempotency-Key"))
	if handled(w, r, e) {
		return
	}
	write(w, 200, map[string]any{"data": v})
}
func (h Handler) Referrals(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		problem(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	v, e := h.Service.Referrals(r.Context(), a)
	if handled(w, r, e) {
		return
	}
	write(w, 200, map[string]any{"data": v})
}
func (h Handler) Rules(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(w, r)
	if !ok {
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/referral-rules"), "/")
	if path == "" {
		switch r.Method {
		case http.MethodGet:
			v, e := h.Service.Rules(r.Context(), a)
			if handled(w, r, e) {
				return
			}
			write(w, 200, map[string]any{"data": v})
		case http.MethodPost:
			var in RuleInput
			if !decode(w, r, &in) {
				return
			}
			v, e := h.Service.CreateRule(r.Context(), a, in)
			if handled(w, r, e) {
				return
			}
			write(w, 201, map[string]any{"data": v})
		default:
			problem(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		}
		return
	}
	if r.Method != http.MethodPatch {
		problem(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	var in RuleInput
	if !decode(w, r, &in) {
		return
	}
	v, e := h.Service.UpdateRule(r.Context(), a, path, in)
	if handled(w, r, e) {
		return
	}
	write(w, 200, map[string]any{"data": v})
}
func (h Handler) Team(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(w, r)
	if !ok {
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/me/customer-team"), "/")
	if path == "" {
		if r.Method != http.MethodGet {
			problem(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		v, e := h.Service.Team(r.Context(), a)
		if handled(w, r, e) {
			return
		}
		write(w, 200, map[string]any{"data": v})
		return
	}
	switch r.Method {
	case http.MethodPut, http.MethodPatch:
		var in TeamInput
		if !decode(w, r, &in) {
			return
		}
		v, e := h.Service.PutTeam(r.Context(), a, path, in)
		if handled(w, r, e) {
			return
		}
		write(w, 200, map[string]any{"data": v})
	case http.MethodDelete:
		if handled(w, r, h.Service.DeleteTeam(r.Context(), a, path)) {
			return
		}
		w.WriteHeader(204)
	default:
		problem(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}
func (h Handler) ProjectAction(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(w, r)
	if !ok {
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/me/projects/"), "/"), "/")
	if len(parts) != 2 || r.Method != http.MethodPost {
		problem(w, r, 404, "NOT_FOUND", "project not found")
		return
	}
	switch parts[1] {
	case "repeat":
		var in RepeatInput
		if !decode(w, r, &in) {
			return
		}
		v, e := h.Service.Repeat(r.Context(), a, parts[0], in)
		if handled(w, r, e) {
			return
		}
		write(w, 201, map[string]any{"data": v})
	case "share":
		if !empty(w, r) {
			return
		}
		v, e := h.Service.Share(r.Context(), a, parts[0])
		if handled(w, r, e) {
			return
		}
		write(w, 200, map[string]any{"data": v})
	default:
		problem(w, r, 404, "NOT_FOUND", "project not found")
	}
}
func (h Handler) InvitedProject(w http.ResponseWriter, r *http.Request) {
	a, ok := actor(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		problem(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/me/invited-projects/"), "/")
	v, e := h.Service.InvitedProject(r.Context(), a, id)
	if handled(w, r, e) {
		return
	}
	write(w, 200, map[string]any{"data": v})
}
func actor(w http.ResponseWriter, r *http.Request) (string, bool) {
	v, ok := auth.ActorID(r.Context())
	if !ok {
		problem(w, r, 401, "UNAUTHENTICATED", "authentication required")
	}
	return v, ok
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(&struct{}{}) != io.EOF {
		problem(w, r, 400, "VALIDATION_ERROR", "invalid growth payload")
		return false
	}
	return true
}
func empty(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	b, e := io.ReadAll(r.Body)
	if e != nil || (strings.TrimSpace(string(b)) != "" && strings.TrimSpace(string(b)) != "{}") {
		problem(w, r, 400, "VALIDATION_ERROR", "request body must be empty")
		return false
	}
	return true
}
func handled(w http.ResponseWriter, r *http.Request, e error) bool {
	if e == nil {
		return false
	}
	switch {
	case errors.Is(e, ErrUnauthorized):
		problem(w, r, 401, "UNAUTHENTICATED", "authentication required")
	case errors.Is(e, ErrForbidden):
		problem(w, r, 403, "FORBIDDEN", "admin role required")
	case errors.Is(e, ErrNotFound):
		problem(w, r, 404, "NOT_FOUND", "growth resource not found")
	case errors.Is(e, ErrConflict):
		problem(w, r, 409, "CONFLICT", "growth state conflict")
	case errors.Is(e, ErrInvalid):
		problem(w, r, 422, "VALIDATION_ERROR", "invalid growth request")
	default:
		problem(w, r, 500, "INTERNAL_ERROR", "internal server error")
	}
	return true
}
func problem(w http.ResponseWriter, r *http.Request, s int, c, m string) {
	write(w, s, map[string]any{"error": map[string]any{"code": c, "message": m, "request_id": r.Header.Get("X-Request-ID")}})
}
func write(w http.ResponseWriter, s int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(s)
	_ = json.NewEncoder(w).Encode(v)
}
