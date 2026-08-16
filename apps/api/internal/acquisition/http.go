package acquisition

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"freelance/apps/api/internal/auth"
)

type Handler struct{ Service Service }
type estimateInput struct {
	Answers     map[string]any `json:"answers"`
	Attribution Attribution    `json:"attribution"`
}

func (h Handler) AdminCalculators(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/calculators"), "/")
	if path == "" && r.Method == http.MethodGet {
		items, err := h.Service.AdminDefinitions(r.Context())
		if domainError(w, r, err) {
			return
		}
		write(w, http.StatusOK, map[string]any{"data": items})
		return
	}
	if path == "" && r.Method == http.MethodPost {
		var in AdminDefinitionInput
		if !decode(w, r, &in, 128<<10) {
			return
		}
		actor, _ := auth.ActorID(r.Context())
		item, err := h.Service.CreateAdminDefinition(r.Context(), actor, in)
		if domainError(w, r, err) {
			return
		}
		write(w, http.StatusCreated, map[string]any{"data": item})
		return
	}
	if path != "" && !strings.Contains(path, "/") && r.Method == http.MethodPatch {
		var in AdminDefinitionInput
		if !decode(w, r, &in, 128<<10) {
			return
		}
		actor, _ := auth.ActorID(r.Context())
		item, err := h.Service.UpdateAdminDefinition(r.Context(), actor, path, in)
		if domainError(w, r, err) {
			return
		}
		write(w, http.StatusOK, map[string]any{"data": item})
		return
	}
	if path == "" || strings.Contains(path, "/") {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "calculator not found")
		return
	}
	writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
}

func (h Handler) Calculator(w http.ResponseWriter, r *http.Request) {
	if strings.TrimRight(r.URL.Path, "/") == "/api/v1/calculators" && r.Method == http.MethodGet {
		v, e := h.Service.Definitions(r.Context())
		if domainError(w, r, e) {
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=300, s-maxage=3600, stale-while-revalidate=86400")
		write(w, 200, map[string]any{"data": v})
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/calculators/"), "/"), "/")
	if len(parts) < 1 || len(parts) > 2 {
		writeError(w, r, 404, "NOT_FOUND", "calculator not found")
		return
	}
	slug := parts[0]
	if len(parts) == 1 && r.Method == http.MethodGet {
		v, e := h.Service.Definition(r.Context(), slug)
		if domainError(w, r, e) {
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=300, s-maxage=3600, stale-while-revalidate=86400")
		write(w, 200, map[string]any{"data": v})
		return
	}
	if len(parts) == 2 && parts[1] == "estimate" && r.Method == http.MethodPost {
		var in estimateInput
		if !decode(w, r, &in, 64<<10) {
			return
		}
		actor, _ := auth.ActorID(r.Context())
		v, e := h.Service.Estimate(r.Context(), actor, slug, in.Answers, in.Attribution)
		if domainError(w, r, e) {
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		write(w, 200, map[string]any{"data": v})
		return
	}
	writeError(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
}
func (h Handler) Event(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	var e Event
	if !decode(w, r, &e, 16<<10) {
		return
	}
	actor, _ := auth.ActorID(r.Context())
	if err := h.Service.Record(r.Context(), actor, e); domainError(w, r, err) {
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusNoContent)
}
func (h Handler) Sitemap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	items, e := h.Service.Repository.Sitemap(r.Context())
	if domainError(w, r, e) {
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=300, s-maxage=3600, stale-while-revalidate=86400")
	write(w, 200, map[string]any{"data": items})
}
func decode(w http.ResponseWriter, r *http.Request, target any, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(target); e != nil {
		decodeError(w, r, e)
		return false
	}
	if e := d.Decode(&struct{}{}); e != io.EOF {
		decodeError(w, r, e)
		return false
	}
	return true
}
func decodeError(w http.ResponseWriter, r *http.Request, e error) {
	var large *http.MaxBytesError
	if errors.As(e, &large) {
		writeError(w, r, 413, "PAYLOAD_TOO_LARGE", "acquisition payload is too large")
		return
	}
	writeError(w, r, 400, "VALIDATION_ERROR", "invalid acquisition payload")
}
func domainError(w http.ResponseWriter, r *http.Request, e error) bool {
	if e == nil {
		return false
	}
	if errors.Is(e, ErrNotFound) {
		writeError(w, r, 404, "NOT_FOUND", "calculator not found")
	} else if errors.Is(e, ErrInvalid) {
		writeError(w, r, 422, "VALIDATION_ERROR", "invalid acquisition data")
	} else {
		log.Printf("acquisition operation failure request_id=%s error_type=%T", r.Header.Get("X-Request-ID"), e)
		writeError(w, r, 500, "INTERNAL_ERROR", "internal server error")
	}
	return true
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	id := w.Header().Get("X-Request-ID")
	if id == "" {
		id = r.Header.Get("X-Request-ID")
	}
	write(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "request_id": id}})
}
