package monetization

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Handler struct {
	Service           Service
	Provider          SubscriptionPaymentProvider // legacy boundary kept for compatibility
	Billing           *BillingService
	ProviderConnected bool
	ActorID           func(context.Context) (string, bool)
}

// actorFromRequest uses the injected trusted session-context reader.
func (h Handler) actorFromRequest(r *http.Request) (string, bool) {
	if h.ActorID == nil {
		return "", false
	}
	return h.ActorID(r.Context())
}

func (h Handler) billingAvailable(ctx context.Context) bool {
	if h.Billing != nil {
		return h.Billing.Available(ctx)
	}
	return h.ProviderConnected
}

func (h Handler) Public(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		monoError(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	plans, enabled, err := h.Service.PublicPlans(r.Context())
	if monoHandle(w, r, err) {
		return
	}
	monoReply(w, 200, map[string]any{"data": map[string]any{"pro_subscriptions_enabled": enabled, "provider_connected": h.billingAvailable(r.Context()), "plans": plans}})
}

func (h Handler) Mine(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		monoError(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	actor, ok := h.actorFromRequest(r)
	if !ok {
		monoError(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	caps, err := h.Service.Resolve(r.Context(), actor)
	if monoHandle(w, r, err) {
		return
	}
	history, err := h.Service.Repository.SubscriptionHistory(r.Context(), actor)
	if monoHandle(w, r, err) {
		return
	}
	paymentMethodConfigured := caps.Subscription != nil && caps.Subscription.PaymentMethodRef != ""
	monoReply(w, 200, map[string]any{"data": map[string]any{"capabilities": caps, "history": history, "provider_connected": h.billingAvailable(r.Context()), "payment_method_configured": paymentMethodConfigured}})
}

func (h Handler) Admin(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actorFromRequest(r)
	if !ok {
		monoError(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/monetization"), "/")
	if path == "" {
		if r.Method != http.MethodGet {
			monoError(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		o, p, s, err := h.Service.AdminData(r.Context(), actor, r.URL.Query().Get("status"))
		if monoHandle(w, r, err) {
			return
		}
		monoReply(w, 200, map[string]any{"data": map[string]any{"overview": o, "plans": p, "subscriptions": s}})
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) == 1 && parts[0] == "subscriptions" && r.Method == http.MethodPost {
		var in struct {
			UserID   string `json:"user_id"`
			PlanID   string `json:"plan_id"`
			StartsAt string `json:"starts_at"`
			EndsAt   string `json:"ends_at"`
			Reason   string `json:"reason"`
		}
		if !monoDecode(w, r, &in) {
			return
		}
		start, e1 := time.Parse(time.RFC3339, in.StartsAt)
		end, e2 := time.Parse(time.RFC3339, in.EndsAt)
		if e1 != nil || e2 != nil {
			monoError(w, r, 422, "VALIDATION_ERROR", "invalid subscription dates")
			return
		}
		v, err := h.Service.Grant(r.Context(), actor, in.UserID, in.PlanID, start, end, in.Reason, r.Header.Get("X-Request-ID"))
		if monoHandle(w, r, err) {
			return
		}
		monoReply(w, 201, map[string]any{"data": v})
		return
	}
	if len(parts) == 3 && parts[0] == "subscriptions" && parts[2] == "events" && r.Method == http.MethodGet {
		if err := h.Service.requireAdmin(r.Context(), actor); monoHandle(w, r, err) {
			return
		}
		items, err := h.Service.Repository.SubscriptionEvents(r.Context(), parts[1])
		if monoHandle(w, r, err) {
			return
		}
		monoReply(w, 200, map[string]any{"data": items})
		return
	}
	if len(parts) == 3 && parts[0] == "subscriptions" && (parts[2] == "cancel" || parts[2] == "expire") && r.Method == http.MethodPost {
		var in struct {
			Reason string `json:"reason"`
		}
		if !monoDecode(w, r, &in) {
			return
		}
		status := "CANCELED"
		if parts[2] == "expire" {
			status = "EXPIRED"
		}
		v, err := h.Service.Transition(r.Context(), actor, parts[1], status, in.Reason, r.Header.Get("X-Request-ID"))
		if monoHandle(w, r, err) {
			return
		}
		monoReply(w, 200, map[string]any{"data": v})
		return
	}
	if len(parts) == 2 && parts[0] == "plans" && r.Method == http.MethodPatch {
		var p Plan
		if !monoDecode(w, r, &p) {
			return
		}
		p.ID = parts[1]
		reason := r.Header.Get("X-Admin-Reason")
		if decoded, decodeErr := url.QueryUnescape(reason); decodeErr == nil {
			reason = decoded
		}
		v, err := h.Service.UpdatePlan(r.Context(), actor, p, reason, r.Header.Get("X-Request-ID"))
		if monoHandle(w, r, err) {
			return
		}
		monoReply(w, 200, map[string]any{"data": v})
		return
	}
	if len(parts) == 3 && parts[0] == "plans" && parts[2] == "entitlements" && r.Method == http.MethodPut {
		var in struct {
			Entitlement Entitlement `json:"entitlement"`
			Reason      string      `json:"reason"`
		}
		if !monoDecode(w, r, &in) {
			return
		}
		v, err := h.Service.SetEntitlement(r.Context(), actor, parts[1], in.Entitlement, in.Reason, r.Header.Get("X-Request-ID"))
		if monoHandle(w, r, err) {
			return
		}
		monoReply(w, 200, map[string]any{"data": v})
		return
	}
	monoError(w, r, 404, "NOT_FOUND", "monetization route not found")
}

func monoDecode(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		monoError(w, r, 400, "VALIDATION_ERROR", "invalid request payload")
		return false
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		monoError(w, r, 400, "VALIDATION_ERROR", "invalid request payload")
		return false
	}
	return true
}
func monoReply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func monoError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	monoReply(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "request_id": r.Header.Get("X-Request-ID")}})
}
func monoHandle(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ErrForbidden):
		monoError(w, r, 403, "FORBIDDEN", "admin role required")
	case errors.Is(err, ErrNotFound):
		monoError(w, r, 404, "NOT_FOUND", "resource not found")
	case errors.Is(err, ErrInvalid):
		monoError(w, r, 422, "VALIDATION_ERROR", "invalid monetization data")
	case errors.Is(err, ErrConflict):
		monoError(w, r, 409, "CONFLICT", "subscription state conflicts with this operation")
	case errors.Is(err, ErrProviderUnavailable):
		monoError(w, r, 503, "PAYMENTS_UNAVAILABLE", "subscription payments are temporarily unavailable")
	default:
		log.Printf("monetization failure request_id=%s error_type=%T", r.Header.Get("X-Request-ID"), err)
		monoError(w, r, 500, "INTERNAL_ERROR", "request could not be completed")
	}
	return true
}
