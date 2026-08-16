package communication

import (
	"context"
	"encoding/json"
	"freelance/apps/api/internal/auth"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type socketClient struct {
	userID string
	conn   *websocket.Conn
	send   chan Event
}
type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[*socketClient]struct{}
}

func NewHub() *Hub { return &Hub{clients: map[string]map[*socketClient]struct{}{}} }
func (h *Hub) add(c *socketClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[c.userID] == nil {
		h.clients[c.userID] = map[*socketClient]struct{}{}
	}
	h.clients[c.userID][c] = struct{}{}
}
func (h *Hub) remove(c *socketClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients[c.userID], c)
	if len(h.clients[c.userID]) == 0 {
		delete(h.clients, c.userID)
	}
	close(c.send)
}
func (h *Hub) Publish(_ context.Context, users []string, event Event) error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	seen := map[string]bool{}
	for _, u := range users {
		if seen[u] {
			continue
		}
		seen[u] = true
		for c := range h.clients[u] {
			select {
			case c.send <- event:
			default:
			}
		}
	}
	return nil
}

type RealtimeHandler struct {
	Service Service
	Hub     *Hub
}

func websocketOriginAllowed(r *http.Request) bool {
	raw := strings.TrimSpace(r.Header.Get("Origin"))
	if raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return false
	}
	publicHost := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if publicHost == "" {
		publicHost = r.Host
	}
	return strings.EqualFold(u.Host, publicHost)
}

var upgrader = websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024, CheckOrigin: websocketOriginAllowed}

func (h RealtimeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	actorID, ok := auth.ActorID(r.Context())
	if !ok {
		problem(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	h.serveAs(w, r, actorID)
}

// ServeSupportHTTP is mounted behind the independent admin-session middleware.
// Staff sockets subscribe to the stable support identity so user->support and
// support->user message events reach the admin console without polling and
// without coupling staff to a marketplace session.
func (h RealtimeHandler) ServeSupportHTTP(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.ActorID(r.Context()); !ok {
		problem(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	h.serveAs(w, r, SupportUserID)
}

func (h RealtimeHandler) serveAs(w http.ResponseWriter, r *http.Request, subscriberID string) {
	if h.Hub == nil {
		problem(w, r, 503, "REALTIME_UNAVAILABLE", "realtime is unavailable")
		return
	}
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	client := &socketClient{userID: subscriberID, conn: c, send: make(chan Event, 64)}
	h.Hub.add(client)
	go h.write(client)
	h.read(r.Context(), client)
	h.Hub.remove(client)
	_ = c.Close()
}
func (h RealtimeHandler) write(c *socketClient) {
	tick := time.NewTicker(25 * time.Second)
	defer tick.Stop()
	for {
		select {
		case e, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if c.conn.WriteJSON(e) != nil {
				return
			}
		case <-tick.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if c.conn.WriteMessage(websocket.PingMessage, nil) != nil {
				return
			}
		}
	}
}
func (h RealtimeHandler) read(ctx context.Context, c *socketClient) {
	c.conn.SetReadLimit(4096)
	_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error { return c.conn.SetReadDeadline(time.Now().Add(60 * time.Second)) })
	for {
		var in struct {
			Event          string `json:"event"`
			ConversationID string `json:"conversation_id"`
		}
		if c.conn.ReadJSON(&in) != nil {
			return
		}
		if in.Event != "typing.start" && in.Event != "typing.stop" && in.Event != "presence.ping" {
			continue
		}
		if in.Event == "presence.ping" {
			if in.ConversationID == "" {
				continue
			}
			if _, err := h.Service.GetConversation(ctx, c.userID, in.ConversationID); err != nil {
				continue
			}
			members, err := h.Service.Repository.Members(ctx, in.ConversationID)
			if err != nil {
				continue
			}
			_ = h.Hub.Publish(ctx, members, Event{Event: "presence.updated", Version: 1, ID: newID(), OccurredAt: time.Now().UTC(), Data: map[string]string{"conversation_id": in.ConversationID, "user_id": c.userID, "state": "online"}})
			continue
		}
		if _, err := h.Service.GetConversation(ctx, c.userID, in.ConversationID); err != nil {
			continue
		}
		members, err := h.Service.Repository.Members(ctx, in.ConversationID)
		if err != nil {
			continue
		}
		event := "typing.started"
		if in.Event == "typing.stop" {
			event = "typing.stopped"
		}
		_ = h.Hub.Publish(ctx, members, Event{Event: event, Version: 1, ID: newID(), OccurredAt: time.Now().UTC(), Data: map[string]string{"conversation_id": in.ConversationID, "user_id": c.userID}})
	}
}

func EncodeEvent(v Event) ([]byte, error) { return json.Marshal(v) }
