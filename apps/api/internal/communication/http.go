package communication

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

type Handler struct {
	Service     Service
	Attachments AttachmentViewer
}

func (h Handler) Conversations(w http.ResponseWriter, r *http.Request) {
	actor, ok := actor(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		v, err := h.Service.ListConversations(r.Context(), actor)
		if handled(w, r, err) {
			return
		}
		respond(w, 200, map[string]any{"data": v})
	case http.MethodPost:
		var in CreateConversation
		if !input(w, r, &in) {
			return
		}
		v, err := h.Service.CreateConversation(r.Context(), actor, in)
		if handled(w, r, err) {
			return
		}
		respond(w, 201, map[string]any{"data": v})
	default:
		problem(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (h Handler) Conversation(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actor(w, r)
	if !ok {
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/conversations/"), "/"), "/")
	if len(parts) == 1 && r.Method == http.MethodGet {
		v, err := h.Service.GetConversation(r.Context(), actorID, parts[0])
		if handled(w, r, err) {
			return
		}
		respond(w, 200, map[string]any{"data": v})
		return
	}
	if len(parts) != 2 {
		problem(w, r, 404, "NOT_FOUND", "conversation not found")
		return
	}
	switch parts[1] {
	case "messages":
		if r.Method == http.MethodGet {
			c, l, valid := messagePage(r)
			if !valid {
				problem(w, r, 400, "VALIDATION_ERROR", "invalid message cursor")
				return
			}
			p, err := h.Service.Messages(r.Context(), actorID, parts[0], c, l)
			if handled(w, r, err) {
				return
			}
			respondPage(w, p)
			return
		}
		if r.Method == http.MethodPost {
			var in MessageInput
			if !input(w, r, &in) {
				return
			}
			v, err := h.Service.Send(r.Context(), actorID, parts[0], in)
			if handled(w, r, err) {
				return
			}
			respond(w, 201, map[string]any{"data": v})
			return
		}
	case "read":
		if r.Method == http.MethodPost {
			var in struct {
				LastReadMessageID string `json:"last_read_message_id"`
			}
			if !input(w, r, &in) {
				return
			}
			if handled(w, r, h.Service.Read(r.Context(), actorID, parts[0], in.LastReadMessageID)) {
				return
			}
			w.WriteHeader(204)
			return
		}
	}
	problem(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
}

func (h Handler) Message(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actor(w, r)
	if !ok {
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/messages/"), "/")
	switch r.Method {
	case http.MethodPatch:
		var in struct {
			Body string `json:"body"`
		}
		if !input(w, r, &in) {
			return
		}
		v, err := h.Service.Edit(r.Context(), actorID, id, in.Body)
		if handled(w, r, err) {
			return
		}
		respond(w, 200, map[string]any{"data": v})
	case http.MethodDelete:
		v, err := h.Service.Delete(r.Context(), actorID, id)
		if handled(w, r, err) {
			return
		}
		respond(w, 200, map[string]any{"data": v})
	default:
		problem(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (h Handler) Attachment(w http.ResponseWriter, r *http.Request) {
	actorID, ok := actor(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		problem(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/messages/"), "/"), "/")
	if len(parts) != 3 || parts[1] != "attachments" || h.Attachments == nil {
		problem(w, r, 404, "NOT_FOUND", "attachment not found")
		return
	}
	v, err := h.Attachments.View(r.Context(), actorID, parts[0], parts[2])
	if handled(w, r, err) {
		return
	}
	respond(w, 200, map[string]any{"data": v})
}

func actor(w http.ResponseWriter, r *http.Request) (string, bool) {
	v, ok := auth.ActorID(r.Context())
	if !ok {
		problem(w, r, 401, "UNAUTHENTICATED", "authentication required")
	}
	return v, ok
}
func input(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(&struct{}{}) != io.EOF {
		problem(w, r, 400, "VALIDATION_ERROR", "invalid request payload")
		return false
	}
	return true
}
func handled(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ErrUnauthorized):
		problem(w, r, 401, "UNAUTHENTICATED", "authentication required")
	case errors.Is(err, ErrNotFound):
		problem(w, r, 404, "NOT_FOUND", "conversation resource not found")
	case errors.Is(err, ErrConflict):
		problem(w, r, 409, "CONFLICT", "communication conflict")
	case errors.Is(err, ErrInvalid):
		problem(w, r, 422, "VALIDATION_ERROR", "invalid communication input")
	default:
		problem(w, r, 500, "INTERNAL_ERROR", "internal server error")
	}
	return true
}

type cursorJSON struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

func messagePage(r *http.Request) (*Cursor, int, bool) {
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
	var c cursorJSON
	if e != nil || json.Unmarshal(b, &c) != nil || c.At.IsZero() || !uuid(c.ID) {
		return nil, 0, false
	}
	return &Cursor{At: c.At.UTC(), ID: c.ID}, l, true
}
func respondPage(w http.ResponseWriter, p MessagePage) {
	var next *string
	if p.NextCursor != nil {
		b, _ := json.Marshal(cursorJSON{At: p.NextCursor.At, ID: p.NextCursor.ID})
		s := base64.RawURLEncoding.EncodeToString(b)
		next = &s
	}
	respond(w, 200, map[string]any{"data": p.Items, "page": map[string]any{"next_cursor": next, "has_more": next != nil}})
}
func problem(w http.ResponseWriter, r *http.Request, s int, c, m string) {
	respond(w, s, map[string]any{"error": map[string]any{"code": c, "message": m, "request_id": r.Header.Get("X-Request-ID")}})
}
func respond(w http.ResponseWriter, s int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(s)
	_ = json.NewEncoder(w).Encode(v)
}
