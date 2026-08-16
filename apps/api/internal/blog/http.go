package blog

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type MediaStorage interface {
	PresignGet(context.Context, string, time.Duration) (string, time.Time, error)
}
type Handler struct {
	Service Service
	ActorID func(context.Context) (string, bool)
	Storage MediaStorage
}

func (h Handler) actor(r *http.Request) (string, bool) {
	if h.ActorID == nil {
		return "", false
	}
	return h.ActorID(r.Context())
}

func (h Handler) Public(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/blog"), "/")
	if strings.HasPrefix(path, "media/") {
		if r.Method != http.MethodGet || h.Storage == nil {
			blogError(w, r, 404, "NOT_FOUND", "media not found")
			return
		}
		key, err := h.Service.Repository.PublicMediaKey(r.Context(), strings.TrimPrefix(path, "media/"))
		if blogHandle(w, r, err) {
			return
		}
		u, _, err := h.Storage.PresignGet(r.Context(), key, 5*time.Minute)
		if err != nil {
			blogHandle(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=300")
		http.Redirect(w, r, u, http.StatusTemporaryRedirect)
		return
	}
	if r.Method != http.MethodGet {
		blogError(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if path == "" {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
		v, err := h.Service.Public(r.Context(), r.URL.Query().Get("category"), page, size)
		if blogHandle(w, r, err) {
			return
		}
		categories, err := h.Service.Repository.ListCategories(r.Context())
		if blogHandle(w, r, err) {
			return
		}
		blogReply(w, 200, map[string]any{"data": v, "categories": categories})
		return
	}
	if strings.Contains(path, "/") {
		blogError(w, r, 404, "NOT_FOUND", "article not found")
		return
	}
	v, related, err := h.Service.Article(r.Context(), path)
	if blogHandle(w, r, err) {
		return
	}
	blogReply(w, 200, map[string]any{"data": v, "related": related})
}

func (h Handler) Admin(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(r)
	if !ok {
		blogError(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/blog"), "/")
	parts := []string{}
	if path != "" {
		parts = strings.Split(path, "/")
	}
	if len(parts) == 0 {
		if r.Method == http.MethodGet {
			posts, err := h.Service.AdminList(r.Context(), actor, r.URL.Query().Get("status"), 1, 100)
			if blogHandle(w, r, err) {
				return
			}
			cats, err := h.Service.Repository.ListCategories(r.Context())
			if blogHandle(w, r, err) {
				return
			}
			tags, err := h.Service.Repository.ListTags(r.Context())
			if blogHandle(w, r, err) {
				return
			}
			blogReply(w, 200, map[string]any{"data": map[string]any{"posts": posts, "categories": cats, "tags": tags}})
			return
		}
		if r.Method == http.MethodPost {
			var in struct {
				Post   WriteRequest `json:"post"`
				Reason string       `json:"reason"`
			}
			if !blogDecode(w, r, &in) {
				return
			}
			v, err := h.Service.Create(r.Context(), actor, in.Post, in.Reason, r.Header.Get("X-Request-ID"))
			if blogHandle(w, r, err) {
				return
			}
			blogReply(w, 201, map[string]any{"data": v})
			return
		}
	}
	if len(parts) >= 1 && parts[0] == "posts" {
		if len(parts) == 2 && r.Method == http.MethodGet {
			v, err := h.Service.AdminGet(r.Context(), actor, parts[1])
			if blogHandle(w, r, err) {
				return
			}
			blogReply(w, 200, map[string]any{"data": v})
			return
		}
		if len(parts) == 2 && r.Method == http.MethodPatch {
			var in struct {
				Post   WriteRequest `json:"post"`
				Reason string       `json:"reason"`
			}
			if !blogDecode(w, r, &in) {
				return
			}
			v, err := h.Service.Update(r.Context(), actor, parts[1], in.Post, in.Reason, r.Header.Get("X-Request-ID"))
			if blogHandle(w, r, err) {
				return
			}
			blogReply(w, 200, map[string]any{"data": v})
			return
		}
		if len(parts) == 2 && r.Method == http.MethodDelete {
			var in struct {
				Reason string `json:"reason"`
			}
			if !blogDecode(w, r, &in) {
				return
			}
			if blogHandle(w, r, h.Service.Delete(r.Context(), actor, parts[1], in.Reason, r.Header.Get("X-Request-ID"))) {
				return
			}
			w.WriteHeader(204)
			return
		}
		if len(parts) == 3 && parts[2] == "archive" && r.Method == http.MethodPost {
			var in struct {
				Reason string `json:"reason"`
			}
			if !blogDecode(w, r, &in) {
				return
			}
			v, err := h.Service.Archive(r.Context(), actor, parts[1], in.Reason, r.Header.Get("X-Request-ID"))
			if blogHandle(w, r, err) {
				return
			}
			blogReply(w, 200, map[string]any{"data": v})
			return
		}
	}
	if len(parts) >= 1 && (parts[0] == "categories" || parts[0] == "tags") {
		if err := h.Service.requireAdmin(r.Context(), actor); blogHandle(w, r, err) {
			return
		}
		kind := parts[0]
		if len(parts) == 1 && r.Method == http.MethodPost {
			if kind == "categories" {
				var in struct {
					Item   Category `json:"item"`
					Reason string   `json:"reason"`
				}
				if !blogDecode(w, r, &in) {
					return
				}
				v, err := h.Service.Repository.SaveCategory(r.Context(), actor, in.Item, in.Reason, r.Header.Get("X-Request-ID"))
				if blogHandle(w, r, err) {
					return
				}
				blogReply(w, 201, map[string]any{"data": v})
				return
			}
			var in struct {
				Item   Tag    `json:"item"`
				Reason string `json:"reason"`
			}
			if !blogDecode(w, r, &in) {
				return
			}
			v, err := h.Service.Repository.SaveTag(r.Context(), actor, in.Item, in.Reason, r.Header.Get("X-Request-ID"))
			if blogHandle(w, r, err) {
				return
			}
			blogReply(w, 201, map[string]any{"data": v})
			return
		}
		if len(parts) == 2 && r.Method == http.MethodPatch {
			if kind == "categories" {
				var in struct {
					Item   Category `json:"item"`
					Reason string   `json:"reason"`
				}
				if !blogDecode(w, r, &in) {
					return
				}
				in.Item.ID = parts[1]
				v, err := h.Service.Repository.SaveCategory(r.Context(), actor, in.Item, in.Reason, r.Header.Get("X-Request-ID"))
				if blogHandle(w, r, err) {
					return
				}
				blogReply(w, 200, map[string]any{"data": v})
				return
			}
			var in struct {
				Item   Tag    `json:"item"`
				Reason string `json:"reason"`
			}
			if !blogDecode(w, r, &in) {
				return
			}
			in.Item.ID = parts[1]
			v, err := h.Service.Repository.SaveTag(r.Context(), actor, in.Item, in.Reason, r.Header.Get("X-Request-ID"))
			if blogHandle(w, r, err) {
				return
			}
			blogReply(w, 200, map[string]any{"data": v})
			return
		}
		if len(parts) == 2 && r.Method == http.MethodDelete {
			var in struct {
				Reason string `json:"reason"`
			}
			if !blogDecode(w, r, &in) {
				return
			}
			var err error
			if kind == "categories" {
				err = h.Service.Repository.DeleteCategory(r.Context(), actor, parts[1], in.Reason, r.Header.Get("X-Request-ID"))
			} else {
				err = h.Service.Repository.DeleteTag(r.Context(), actor, parts[1], in.Reason, r.Header.Get("X-Request-ID"))
			}
			if blogHandle(w, r, err) {
				return
			}
			w.WriteHeader(204)
			return
		}
	}
	blogError(w, r, 404, "NOT_FOUND", "blog route not found")
}

func blogDecode(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 512<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		blogError(w, r, 400, "VALIDATION_ERROR", "invalid request payload")
		return false
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		blogError(w, r, 400, "VALIDATION_ERROR", "invalid request payload")
		return false
	}
	return true
}
func blogReply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func blogError(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	blogReply(w, status, map[string]any{"error": map[string]any{"code": code, "message": msg, "request_id": r.Header.Get("X-Request-ID")}})
}
func blogHandle(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ErrForbidden):
		blogError(w, r, 403, "FORBIDDEN", "admin role required")
	case errors.Is(err, ErrNotFound):
		blogError(w, r, 404, "NOT_FOUND", "content not found")
	case errors.Is(err, ErrInvalid):
		blogError(w, r, 422, "VALIDATION_ERROR", "invalid content data")
	case errors.Is(err, ErrConflict):
		blogError(w, r, 409, "CONFLICT", "content conflicts with an existing record or state")
	default:
		log.Printf("blog failure request_id=%s error_type=%T", r.Header.Get("X-Request-ID"), err)
		blogError(w, r, 500, "INTERNAL_ERROR", "request could not be completed")
	}
	return true
}
