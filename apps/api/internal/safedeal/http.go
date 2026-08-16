package safedeal

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

func (h Handler) Mine(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.ActorID(r.Context())
	if !ok {
		fail(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/me/safe-deals"), "/")
	if path == "" {
		if r.Method != http.MethodGet {
			fail(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		items, e := h.Service.List(r.Context(), actor, r.URL.Query().Get("project_id"))
		if domainFail(w, r, e) {
			return
		}
		writeDeal(w, 200, map[string]any{"data": items})
		return
	}
	if path == "quote" {
		if r.Method != http.MethodPost {
			fail(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		var in struct {
			WorkAmountKopecks int64  `json:"work_amount_kopecks"`
			PaymentMethod     string `json:"payment_method"`
		}
		if !decodeDeal(w, r, &in, 4<<10) {
			return
		}
		q, e := h.Service.Quote(r.Context(), in.WorkAmountKopecks, in.PaymentMethod)
		if domainFail(w, r, e) {
			return
		}
		writeDeal(w, 200, map[string]any{"data": q})
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) >= 3 && parts[0] == "disputes" && parts[2] == "evidence" && r.Method == http.MethodPost {
		key, ok := key(w, r)
		if !ok {
			return
		}
		var in EvidenceInput
		if !decodeDeal(w, r, &in, 32<<10) {
			return
		}
		d, e := h.Service.Evidence(r.Context(), actor, parts[1], key, in)
		if domainFail(w, r, e) {
			return
		}
		writeDeal(w, 201, map[string]any{"data": d})
		return
	}
	id := parts[0]
	if len(parts) == 1 && r.Method == http.MethodGet {
		d, e := h.Service.Get(r.Context(), actor, id)
		if domainFail(w, r, e) {
			return
		}
		writeDeal(w, 200, map[string]any{"data": d})
		return
	}
	if len(parts) != 2 || r.Method != http.MethodPost {
		fail(w, r, 404, "NOT_FOUND", "safe deal route not found")
		return
	}
	operation := parts[1]
	idempotency, ok := key(w, r)
	if !ok {
		return
	}
	var d Deal
	var e error
	switch operation {
	case "fund":
		if !emptyDeal(w, r) {
			return
		}
		d, e = h.Service.Funding(r.Context(), actor, id, idempotency)
	case "start":
		if !emptyDeal(w, r) {
			return
		}
		d, e = h.Service.Start(r.Context(), actor, id, idempotency)
	case "submit":
		var in SubmitInput
		if !decodeDeal(w, r, &in, 32<<10) {
			return
		}
		d, e = h.Service.Submit(r.Context(), actor, id, idempotency, in)
	case "revision":
		var in struct {
			Reason string `json:"reason"`
		}
		if !decodeDeal(w, r, &in, 8<<10) {
			return
		}
		d, e = h.Service.Revision(r.Context(), actor, id, idempotency, in.Reason)
	case "accept":
		if !emptyDeal(w, r) {
			return
		}
		d, e = h.Service.Accept(r.Context(), actor, id, idempotency)
	case "cancel":
		if !emptyDeal(w, r) {
			return
		}
		d, e = h.Service.Cancel(r.Context(), actor, id, idempotency)
	case "disputes":
		var in DisputeInput
		if !decodeDeal(w, r, &in, 32<<10) {
			return
		}
		d, e = h.Service.Dispute(r.Context(), actor, id, idempotency, in)
	default:
		fail(w, r, 404, "NOT_FOUND", "safe deal action not found")
		return
	}
	if domainFail(w, r, e) {
		return
	}
	if latest, getErr := h.Service.Get(r.Context(), actor, id); getErr == nil {
		d = latest
	}
	writeDeal(w, 200, map[string]any{"data": d})
}
func (h Handler) Admin(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.ActorID(r.Context())
	if !ok {
		fail(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/safe-deals"), "/")
	if path == "" && r.Method == http.MethodGet {
		items, e := h.Service.AdminList(r.Context(), actor)
		if domainFail(w, r, e) {
			return
		}
		writeDeal(w, 200, map[string]any{"data": items})
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) == 2 && parts[1] == "delete" && r.Method == http.MethodPost {
		var in struct {
			Reason string `json:"reason"`
		}
		if !decodeDeal(w, r, &in, 16<<10) {
			return
		}
		if domainFail(w, r, h.Service.AdminDeleteUnfunded(r.Context(), actor, parts[0], in.Reason)) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(parts) != 2 || r.Method != http.MethodPost {
		fail(w, r, 404, "NOT_FOUND", "admin safe deal route not found")
		return
	}
	idempotency, ok := key(w, r)
	if !ok {
		return
	}
	switch parts[1] {
	case "resolve":
		var in ResolutionInput
		if !decodeDeal(w, r, &in, 16<<10) {
			return
		}
		d, e := h.Service.Resolve(r.Context(), actor, parts[0], idempotency, in)
		if domainFail(w, r, e) {
			return
		}
		writeDeal(w, 200, map[string]any{"data": d})
	case "reconcile":
		if !emptyDeal(w, r) {
			return
		}
		if _, e := h.Service.AdminList(r.Context(), actor); domainFail(w, r, e) {
			return
		}
		count, e := h.Service.Reconcile(r.Context())
		if domainFail(w, r, e) {
			return
		}
		writeDeal(w, 200, map[string]any{"data": map[string]int{"reconciled": count}})
	default:
		fail(w, r, 404, "NOT_FOUND", "admin safe deal action not found")
	}
}
func (h Handler) Webhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	body, e := io.ReadAll(r.Body)
	if e != nil {
		fail(w, r, 413, "PAYLOAD_TOO_LARGE", "webhook payload is too large")
		return
	}
	_, _, e = h.Service.Webhook(r.Context(), map[string][]string(r.Header), body)
	if domainFail(w, r, e) {
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusNoContent)
}
func key(w http.ResponseWriter, r *http.Request) (string, bool) {
	v := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if !validIdempotency.MatchString(v) {
		fail(w, r, 400, "VALIDATION_ERROR", "valid Idempotency-Key is required")
		return "", false
	}
	return v, true
}
func decodeDeal(w http.ResponseWriter, r *http.Request, target any, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(target); e != nil {
		fail(w, r, 400, "VALIDATION_ERROR", "invalid safe deal payload")
		return false
	}
	if e := d.Decode(&struct{}{}); e != io.EOF {
		fail(w, r, 400, "VALIDATION_ERROR", "invalid safe deal payload")
		return false
	}
	return true
}
func emptyDeal(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	b, e := io.ReadAll(r.Body)
	if e != nil {
		return false
	}
	v := strings.TrimSpace(string(b))
	if v != "" && v != "{}" {
		fail(w, r, 400, "VALIDATION_ERROR", "action payload must be empty")
		return false
	}
	return true
}
func domainFail(w http.ResponseWriter, r *http.Request, e error) bool {
	if e == nil {
		return false
	}
	switch {
	case errors.Is(e, ErrNotFound):
		fail(w, r, 404, "NOT_FOUND", "safe deal not found")
	case errors.Is(e, ErrForbidden):
		fail(w, r, 403, "FORBIDDEN", "safe deal action forbidden")
	case errors.Is(e, ErrInvalid):
		fail(w, r, 422, "VALIDATION_ERROR", "invalid safe deal data")
	case errors.Is(e, ErrInvalidState):
		fail(w, r, 409, "INVALID_STATE", "safe deal action is not allowed in current state")
	case errors.Is(e, ErrConflict):
		fail(w, r, 409, "IDEMPOTENCY_CONFLICT", "idempotency key conflicts with an earlier command")
	case errors.Is(e, ErrProvider), errors.Is(e, ErrUnsupported):
		fail(w, r, 503, "PAYMENT_PROVIDER_UNAVAILABLE", "payment provider is temporarily unavailable")
	default:
		log.Printf("safe deal failure request_id=%s error_type=%T", r.Header.Get("X-Request-ID"), e)
		fail(w, r, 500, "INTERNAL_ERROR", "internal server error")
	}
	return true
}
func writeDeal(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeDeal(w, status, map[string]any{"error": map[string]string{"code": code, "message": message, "request_id": r.Header.Get("X-Request-ID")}})
}
