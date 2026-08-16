package communication

import (
	"net/http"
	"strings"
)

const SupportUserID = "00000000-0000-4000-8000-000000000011"

// AdminSupport exposes the existing support-system conversations through the
// independently-authenticated staff area. Staff never joins marketplace chats
// as their personal account; replies are authored by the stable support identity.
type AdminSupportHandler struct{ Service Service }

func (h AdminSupportHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/support"), "/")
	if path == "conversations" && r.Method == http.MethodGet {
		items, err := h.Service.ListConversations(r.Context(), SupportUserID)
		if handled(w, r, err) {
			return
		}
		respond(w, 200, map[string]any{"data": items})
		return
	}
	parts := strings.Split(path, "/")

	if len(parts) == 3 && parts[0] == "conversations" && parts[2] == "read" && r.Method == http.MethodPost {
		var in struct {
			LastReadMessageID string `json:"last_read_message_id"`
		}
		if !input(w, r, &in) {
			return
		}
		if handled(w, r, h.Service.Read(r.Context(), SupportUserID, parts[1], in.LastReadMessageID)) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(parts) == 3 && parts[0] == "conversations" && parts[2] == "messages" {
		id := parts[1]
		switch r.Method {
		case http.MethodGet:
			cursor, limit, ok := messagePage(r)
			if !ok {
				problem(w, r, 400, "VALIDATION_ERROR", "invalid message cursor")
				return
			}
			page, err := h.Service.Messages(r.Context(), SupportUserID, id, cursor, limit)
			if handled(w, r, err) {
				return
			}
			respondPage(w, page)
			return
		case http.MethodPost:
			var in MessageInput
			if !input(w, r, &in) {
				return
			}
			message, err := h.Service.Send(r.Context(), SupportUserID, id, in)
			if handled(w, r, err) {
				return
			}
			respond(w, 201, map[string]any{"data": message})
			return
		}
	}
	problem(w, r, 404, "NOT_FOUND", "support conversation not found")
}
