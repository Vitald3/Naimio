package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"freelance/apps/api/internal/auth"
)

type Handler struct{ Service Service }
type draftInput struct {
	SourceType     string         `json:"source_type"`
	RawInput       map[string]any `json:"raw_input"`
	NormalizedData map[string]any `json:"normalized_data"`
}
type aiInput struct {
	DraftToken   string     `json:"draft_token"`
	Text         string     `json:"text"`
	Goal         string     `json:"goal"`
	Materials    []Material `json:"materials"`
	CategorySlug string     `json:"category_slug"`
	SkillSlugs   []string   `json:"skill_slugs"`
	Features     []string   `json:"features"`
}

func (h Handler) DraftCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	var input draftInput
	if !decode(w, r, &input, 128<<10) {
		return
	}
	actor, _ := auth.ActorID(r.Context())
	draft, token, err := h.Service.CreateDraft(r.Context(), actor, input.SourceType, input.RawInput)
	if writeDomainError(w, r, err) {
		return
	}
	write(w, http.StatusCreated, map[string]any{"data": draft, "draft_token": token})
}
func (h Handler) DraftItem(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/project-drafts/"), "/"), "/")
	if len(parts) < 1 || !validToken(parts[0]) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "draft not found")
		return
	}
	token := parts[0]
	actor, _ := auth.ActorID(r.Context())
	if len(parts) == 2 && parts[1] == "claim" {
		if r.Method != http.MethodPost {
			writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		if actor == "" {
			writeError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
			return
		}
		if !empty(w, r) {
			return
		}
		draft, err := h.Service.ClaimDraft(r.Context(), actor, token)
		if writeDomainError(w, r, err) {
			return
		}
		write(w, http.StatusOK, map[string]any{"data": draft})
		return
	}
	if len(parts) != 1 {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "draft not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		draft, err := h.Service.GetDraft(r.Context(), actor, token)
		if writeDomainError(w, r, err) {
			return
		}
		write(w, http.StatusOK, map[string]any{"data": draft})
	case http.MethodPatch:
		var input draftInput
		if !decode(w, r, &input, 128<<10) {
			return
		}
		draft, err := h.Service.UpdateDraft(r.Context(), actor, token, input.RawInput, input.NormalizedData)
		if writeDomainError(w, r, err) {
			return
		}
		write(w, http.StatusOK, map[string]any{"data": draft})
	default:
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}
func (h Handler) Tool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	var input aiInput
	if !decode(w, r, &input, 512<<10) {
		return
	}
	actor, _ := auth.ActorID(r.Context())
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/ai/")
	switch path {
	case "project-brief":
		result, draft, err := h.Service.Brief(r.Context(), actor, input.DraftToken, input.Text)
		h.respond(w, r, result, draft, err)
	case "project-import":
		result, draft, err := h.Service.Import(r.Context(), actor, input.DraftToken, input.Materials)
		h.respond(w, r, result, draft, err)
	case "project-estimate":
		result, draft, err := h.Service.Estimate(r.Context(), actor, input.DraftToken, EstimateRequest{Text: input.Text, CategorySlug: input.CategorySlug, SkillSlugs: input.SkillSlugs, Features: input.Features})
		h.respond(w, r, result, draft, err)
	case "commercial-offer-analysis":
		result, draft, err := h.Service.AnalyzeOffer(r.Context(), actor, input.DraftToken, input.Text, input.Goal)
		h.respond(w, r, result, draft, err)
	case "taxonomy-suggestions":
		result, err := h.Service.Suggest(r.Context(), actor, input.Text)
		if writeDomainError(w, r, err) {
			return
		}
		write(w, http.StatusOK, map[string]any{"data": result, "generated": true})
	default:
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "ai tool not found")
	}
}
func (h Handler) respond(w http.ResponseWriter, r *http.Request, result any, draft Draft, err error) {
	if writeDomainError(w, r, err) {
		return
	}
	write(w, http.StatusOK, map[string]any{"data": result, "draft": draft, "generated": true, "editable": true})
}
func decode(w http.ResponseWriter, r *http.Request, target any, max int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, max)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, r, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "ai input is too large")
		} else {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid ai input")
		}
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid ai input")
		return false
	}
	return true
}
func empty(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	value, err := io.ReadAll(r.Body)
	if err != nil || strings.TrimSpace(string(value)) != "" && strings.TrimSpace(string(value)) != "{}" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "request body must be empty")
		return false
	}
	return true
}
func writeDomainError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ErrInvalidInput):
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid ai input")
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrForbidden):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "draft not found")
	case errors.Is(err, ErrUnavailable), errors.Is(err, ErrInvalidOutput), errors.Is(err, context.DeadlineExceeded):
		writeError(w, r, http.StatusServiceUnavailable, "AI_UNAVAILABLE", "AI assistance is temporarily unavailable; continue manually")
	default:
		log.Printf("ai operation failure request_id=%s error_type=%T", r.Header.Get("X-Request-ID"), err)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
	return true
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
