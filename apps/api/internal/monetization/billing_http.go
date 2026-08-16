package monetization

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

func (h Handler) BillingRoutes(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actorFromRequest(r)
	if !ok {
		monoError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return
	}
	if h.Billing == nil {
		monoError(w, r, http.StatusServiceUnavailable, "PAYMENTS_UNAVAILABLE", "subscription payments are temporarily unavailable")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/me/pro-billing"), "/")
	switch {
	case path == "checkout" && r.Method == http.MethodPost:
		var in struct {
			PlanID string `json:"plan_id"`
		}
		if !monoDecode(w, r, &in) {
			return
		}
		key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if len(key) < 8 {
			monoError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "valid Idempotency-Key is required")
			return
		}
		out, err := h.Billing.StartPurchase(r.Context(), actor, in.PlanID, key)
		if billingHandle(w, r, err) {
			return
		}
		monoReply(w, http.StatusCreated, map[string]any{"data": out})
		return
	case path == "recover" && r.Method == http.MethodPost:
		key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if len(key) < 8 {
			monoError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "valid Idempotency-Key is required")
			return
		}
		out, err := h.Billing.RecoverPurchase(r.Context(), actor, key)
		if billingHandle(w, r, err) {
			return
		}
		monoReply(w, http.StatusCreated, map[string]any{"data": out})
		return
	case path == "status" && r.Method == http.MethodGet:
		id := strings.TrimSpace(r.URL.Query().Get("attempt_id"))
		if id == "" {
			monoError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "attempt_id is required")
			return
		}
		out, err := h.Billing.Status(r.Context(), actor, id)
		if billingHandle(w, r, err) {
			return
		}
		monoReply(w, http.StatusOK, map[string]any{"data": out})
		return
	case path == "history" && r.Method == http.MethodGet:
		items, err := h.Billing.History(r.Context(), actor)
		if billingHandle(w, r, err) {
			return
		}
		monoReply(w, http.StatusOK, map[string]any{"data": items})
		return
	case path == "cancel" && r.Method == http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		var empty map[string]any
		if err := json.NewDecoder(r.Body).Decode(&empty); err != nil && r.ContentLength > 0 {
			monoError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request payload")
			return
		}
		out, err := h.Billing.CancelAutoRenew(r.Context(), actor)
		if billingHandle(w, r, err) {
			return
		}
		monoReply(w, http.StatusOK, map[string]any{"data": out})
		return
	default:
		monoError(w, r, http.StatusNotFound, "NOT_FOUND", "billing route not found")
	}
}

func billingHandle(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ErrInvalid):
		monoError(w, r, 422, "VALIDATION_ERROR", "invalid billing request")
	case errors.Is(err, ErrForbidden):
		monoError(w, r, 403, "FORBIDDEN", "billing resource is not available")
	case errors.Is(err, ErrNotFound):
		monoError(w, r, 404, "NOT_FOUND", "billing resource not found")
	case errors.Is(err, ErrConflict):
		monoError(w, r, 409, "CONFLICT", "billing state conflicts with this operation")
	case errors.Is(err, ErrProviderUnavailable), errors.Is(err, ErrBillingUnavailable):
		monoError(w, r, 503, "PAYMENTS_UNAVAILABLE", "subscription payments are temporarily unavailable")
	default:
		monoHandle(w, r, err)
	}
	return true
}
