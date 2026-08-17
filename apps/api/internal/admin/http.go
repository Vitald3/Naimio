package admin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Handler struct {
	Service Service
	ActorID func(context.Context) (string, bool)
	Notify  func(context.Context, string)
}

// PostgreSQL accepts canonical UUIDs regardless of the version/variant bits.
// Deterministic seed UUIDs therefore need the same validation as production IDs.
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func (h Handler) actor(w http.ResponseWriter, r *http.Request) (string, bool) {
	if h.ActorID == nil {
		problem(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return "", false
	}
	actor, ok := h.ActorID(r.Context())
	if !ok || actor == "" {
		problem(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return "", false
	}
	return actor, true
}

func (h Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		method(w, r)
		return
	}
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	v, err := h.Service.Dashboard(r.Context(), actor)
	if handle(w, r, err) {
		return
	}
	reply(w, http.StatusOK, map[string]any{"data": v})
}

func (h Handler) Users(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/users"), "/")
	if path == "" {
		if r.Method != http.MethodGet {
			method(w, r)
			return
		}
		cursor, limit, ok := pageInput(w, r)
		if !ok {
			return
		}
		v, err := h.Service.ListUsers(r.Context(), actor, UserFilter{Q: r.URL.Query().Get("q"), Status: r.URL.Query().Get("status"), Role: r.URL.Query().Get("role"), Capability: r.URL.Query().Get("capability")}, cursor, limit)
		if handle(w, r, err) {
			return
		}
		replyPage(w, v.Items, v.Page)
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) < 1 || !uuidPattern.MatchString(parts[0]) {
		notFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 1 && r.Method == http.MethodGet {
		v, err := h.Service.GetUser(r.Context(), actor, id)
		if handle(w, r, err) {
			return
		}
		reply(w, http.StatusOK, map[string]any{"data": v})
		return
	}
	if len(parts) == 2 && parts[1] == "status" && r.Method == http.MethodPatch {
		var in struct {
			Status string `json:"status"`
			Reason string `json:"reason"`
		}
		if !decode(w, r, &in) {
			return
		}
		v, err := h.Service.SetUserStatus(r.Context(), actor, id, in.Status, in.Reason, requestID(r))
		if handle(w, r, err) {
			return
		}
		reply(w, http.StatusOK, map[string]any{"data": v})
		return
	}
	if len(parts) == 2 && parts[1] == "revoke-sessions" && r.Method == http.MethodPost {
		var in struct {
			Reason string `json:"reason"`
		}
		if !decode(w, r, &in) {
			return
		}
		if handle(w, r, h.Service.RevokeSessions(r.Context(), actor, id, in.Reason, requestID(r))) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(parts) == 3 && parts[1] == "roles" && (r.Method == http.MethodPut || r.Method == http.MethodDelete) {
		var in struct {
			Reason string `json:"reason"`
		}
		if !decode(w, r, &in) {
			return
		}
		v, err := h.Service.SetRole(r.Context(), actor, id, parts[2], r.Method == http.MethodPut, in.Reason, requestID(r))
		if handle(w, r, err) {
			return
		}
		reply(w, http.StatusOK, map[string]any{"data": v})
		return
	}
	notFound(w, r)
}

func (h Handler) FeatureFlags(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/feature-flags"), "/")
	if path == "" {
		if r.Method != http.MethodGet {
			method(w, r)
			return
		}
		v, err := h.Service.ListFeatureFlags(r.Context(), actor)
		if handle(w, r, err) {
			return
		}
		reply(w, http.StatusOK, map[string]any{"data": v})
		return
	}
	if r.Method != http.MethodPatch || strings.Contains(path, "/") || len(path) > 100 {
		notFound(w, r)
		return
	}
	var in struct {
		Enabled bool           `json:"enabled"`
		Config  map[string]any `json:"config"`
		Reason  string         `json:"reason"`
	}
	if !decode(w, r, &in) {
		return
	}
	v, err := h.Service.UpdateFeatureFlag(r.Context(), actor, path, in.Enabled, in.Config, in.Reason, requestID(r))
	if handle(w, r, err) {
		return
	}
	reply(w, http.StatusOK, map[string]any{"data": v})
}

func (h Handler) SiteSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		method(w, r)
		return
	}
	v, err := h.Service.PublicSiteSettings(r.Context())
	if handle(w, r, err) {
		return
	}
	reply(w, http.StatusOK, map[string]any{"data": v})
}

func (h Handler) Reports(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/reports"), "/")
	if path == "" {
		if r.Method != http.MethodGet {
			method(w, r)
			return
		}
		c, l, ok := pageInput(w, r)
		if !ok {
			return
		}
		v, p, err := h.Service.ListReports(r.Context(), actor, listFilter(r), c, l)
		if handle(w, r, err) {
			return
		}
		replyPage(w, v, p)
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[1] != "status" || !uuidPattern.MatchString(parts[0]) || r.Method != http.MethodPatch {
		notFound(w, r)
		return
	}
	var in struct {
		Status     string `json:"status"`
		Resolution string `json:"resolution"`
	}
	if !decode(w, r, &in) {
		return
	}
	v, err := h.Service.UpdateReport(r.Context(), actor, parts[0], in.Status, in.Resolution, requestID(r))
	if handle(w, r, err) {
		return
	}
	reply(w, http.StatusOK, map[string]any{"data": v})
}

func (h Handler) Fraud(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/fraud-signals"), "/")
	if path == "" {
		if r.Method != http.MethodGet {
			method(w, r)
			return
		}
		c, l, ok := pageInput(w, r)
		if !ok {
			return
		}
		v, p, err := h.Service.ListFraudSignals(r.Context(), actor, listFilter(r), c, l)
		if handle(w, r, err) {
			return
		}
		replyPage(w, v, p)
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[1] != "status" || !uuidPattern.MatchString(parts[0]) || r.Method != http.MethodPatch {
		notFound(w, r)
		return
	}
	var in struct {
		Status     string `json:"status"`
		Resolution string `json:"resolution"`
	}
	if !decode(w, r, &in) {
		return
	}
	v, err := h.Service.UpdateFraudSignal(r.Context(), actor, parts[0], in.Status, in.Resolution, requestID(r))
	if handle(w, r, err) {
		return
	}
	reply(w, http.StatusOK, map[string]any{"data": v})
}

func (h Handler) Audit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		method(w, r)
		return
	}
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	c, l, ok := pageInput(w, r)
	if !ok {
		return
	}
	v, p, err := h.Service.ListAudit(r.Context(), actor, listFilter(r), c, l)
	if handle(w, r, err) {
		return
	}
	replyPage(w, v, p)
}

func (h Handler) Content(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, ok := h.actor(w, r)
		if !ok {
			return
		}
		plural := strings.ToLower(kind) + "s"
		if kind == "VACANCY" {
			plural = "vacancies"
		}
		prefix := "/api/v1/admin/" + plural
		path := strings.Trim(strings.TrimPrefix(r.URL.Path, prefix), "/")
		if path == "" {
			if r.Method != http.MethodGet {
				method(w, r)
				return
			}
			c, l, ok := pageInput(w, r)
			if !ok {
				return
			}
			v, p, err := h.Service.ListContent(r.Context(), actor, kind, listFilter(r), c, l)
			if handle(w, r, err) {
				return
			}
			replyPage(w, v, p)
			return
		}
		parts := strings.Split(path, "/")
		if len(parts) == 1 && uuidPattern.MatchString(parts[0]) && r.Method == http.MethodGet {
			v, err := h.Service.GetContent(r.Context(), actor, kind, parts[0])
			if handle(w, r, err) {
				return
			}
			reply(w, http.StatusOK, map[string]any{"data": v})
			return
		}
		if len(parts) == 2 && parts[1] == "moderation" && uuidPattern.MatchString(parts[0]) && r.Method == http.MethodPost {
			var in struct {
				Action string `json:"action"`
				Reason string `json:"reason"`
			}
			if !decode(w, r, &in) {
				return
			}
			v, err := h.Service.ModerateContent(r.Context(), actor, kind, parts[0], in.Action, in.Reason, requestID(r))
			if handle(w, r, err) {
				return
			}
			if h.Notify != nil && (strings.EqualFold(in.Action, "REJECT") || strings.EqualFold(in.Action, "DELETE")) {
				h.Notify(r.Context(), v.OwnerUserID)
			}
			reply(w, http.StatusOK, map[string]any{"data": v})
			return
		}
		notFound(w, r)
	}
}

func (h Handler) Reviews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		method(w, r)
		return
	}
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	c, l, ok := pageInput(w, r)
	if !ok {
		return
	}
	v, p, err := h.Service.ListReviews(r.Context(), actor, listFilter(r), c, l)
	if handle(w, r, err) {
		return
	}
	replyPage(w, v, p)
}
func (h Handler) Disputes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		method(w, r)
		return
	}
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	c, l, ok := pageInput(w, r)
	if !ok {
		return
	}
	v, p, err := h.Service.ListDisputes(r.Context(), actor, listFilter(r), c, l)
	if handle(w, r, err) {
		return
	}
	replyPage(w, v, p)
}

func listFilter(r *http.Request) ListFilter {
	return ListFilter{Q: strings.TrimSpace(r.URL.Query().Get("q")), Status: strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status"))), Kind: strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("kind")))}
}

type cursorPayload struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

func encodeCursor(c Cursor) string {
	b, _ := json.Marshal(cursorPayload{At: c.At.UTC(), ID: c.ID})
	return base64.RawURLEncoding.EncodeToString(b)
}
func decodeCursor(v string) (*Cursor, error) {
	if v == "" {
		return nil, nil
	}
	if len(v) > 1024 {
		return nil, ErrInvalidInput
	}
	b, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil {
		return nil, ErrInvalidInput
	}
	var p cursorPayload
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if err = d.Decode(&p); err != nil || p.At.IsZero() || !uuidPattern.MatchString(p.ID) {
		return nil, ErrInvalidInput
	}
	if err = d.Decode(&struct{}{}); err != io.EOF {
		return nil, ErrInvalidInput
	}
	return &Cursor{At: p.At.UTC(), ID: p.ID}, nil
}
func pageInput(w http.ResponseWriter, r *http.Request) (*Cursor, int, bool) {
	c, err := decodeCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		problem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid cursor")
		return nil, 0, false
	}
	limit := 30
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			problem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid limit")
			return nil, 0, false
		}
	}
	return c, limit, true
}
func replyPage(w http.ResponseWriter, data any, p PageInfo) {
	var next *string
	if p.NextCursor != nil {
		v := encodeCursor(*p.NextCursor)
		next = &v
	}
	reply(w, http.StatusOK, map[string]any{"data": data, "page": map[string]any{"next_cursor": next, "has_more": p.HasMore}})
}

func (h Handler) IndexNowSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		method(w, r)
		return
	}
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	var in struct {
		URLs        []string `json:"urls"`
		Key         string   `json:"key"`
		KeyLocation string   `json:"key_location,omitempty"`
		Host        string   `json:"host,omitempty"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.URLs) == 0 {
		problem(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "список URL не может быть пустым")
		return
	}
	v, err := h.Service.SubmitIndexNow(r.Context(), actor, in.URLs, in.Key, in.KeyLocation, in.Host, requestID(r))
	if handle(w, r, err) {
		return
	}
	reply(w, http.StatusOK, map[string]any{"data": v})
}
func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		problem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return false
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		problem(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return false
	}
	return true
}
func requestID(r *http.Request) string { return r.Header.Get("X-Request-ID") }
func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func method(w http.ResponseWriter, r *http.Request) {
	problem(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
}
func notFound(w http.ResponseWriter, r *http.Request) {
	problem(w, r, http.StatusNotFound, "NOT_FOUND", "admin resource not found")
}
func handle(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ErrForbidden):
		problem(w, r, http.StatusForbidden, "FORBIDDEN", "admin permission required")
	case errors.Is(err, ErrNotFound):
		notFound(w, r)
	case errors.Is(err, ErrInvalidInput):
		problem(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid admin operation")
	case errors.Is(err, ErrConflict):
		problem(w, r, http.StatusConflict, "CONFLICT", "admin operation conflicts with current state")
	default:
		problem(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
	return true
}
func problem(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	reply(w, status, map[string]any{"error": map[string]string{"code": code, "message": message, "request_id": requestID(r)}})
}
