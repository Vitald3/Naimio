package finance

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

type Handler struct {
	Service Service
	ActorID func(context.Context) (string, bool)
}

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

// Fees serves GET (list versioned commission rules) and POST (create a new
// commission-rule version) at /api/v1/admin/finance/fees.
func (h Handler) Fees(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	if strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/finance/fees"), "/") != "" {
		notFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		v, err := h.Service.ListFeeRules(r.Context(), actor)
		if handle(w, r, err) {
			return
		}
		reply(w, http.StatusOK, map[string]any{"data": v})
	case http.MethodPost:
		var in struct {
			CommissionBasisPoints            int    `json:"commission_basis_points"`
			MinimumFeeKopecks                int64  `json:"minimum_fee_kopecks"`
			MaximumFeeKopecks                *int64 `json:"maximum_fee_kopecks"`
			PlatformFeePayerMode             string `json:"platform_fee_payer_mode"`
			PlatformCustomerShareBasisPoints int    `json:"platform_customer_share_basis_points"`
			ProviderFeePayerMode             string `json:"provider_fee_payer_mode"`
			ProviderCustomerShareBasisPoints int    `json:"provider_customer_share_basis_points"`
			Confirm                          bool   `json:"confirm"`
			Reason                           string `json:"reason"`
		}
		if !decode(w, r, &in) {
			return
		}
		rule := FeeRule{
			CommissionBasisPoints:            in.CommissionBasisPoints,
			MinimumFeeKopecks:                in.MinimumFeeKopecks,
			MaximumFeeKopecks:                in.MaximumFeeKopecks,
			PlatformFeePayerMode:             in.PlatformFeePayerMode,
			PlatformCustomerShareBasisPoints: in.PlatformCustomerShareBasisPoints,
			ProviderFeePayerMode:             in.ProviderFeePayerMode,
			ProviderCustomerShareBasisPoints: in.ProviderCustomerShareBasisPoints,
		}
		v, err := h.Service.CreateFeeRule(r.Context(), actor, rule, in.Confirm, in.Reason, requestID(r))
		if handle(w, r, err) {
			return
		}
		reply(w, http.StatusCreated, map[string]any{"data": v})
	default:
		method(w, r)
	}
}

// ProviderPricing serves GET (list versioned provider-cost rules) and POST
// (create a new pricing version) at /api/v1/admin/finance/provider-pricing.
func (h Handler) ProviderPricing(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	if strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/finance/provider-pricing"), "/") != "" {
		notFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		v, err := h.Service.ListProviderPricing(r.Context(), actor)
		if handle(w, r, err) {
			return
		}
		reply(w, http.StatusOK, map[string]any{"data": v})
	case http.MethodPost:
		var in struct {
			Provider           string `json:"provider"`
			PaymentMethod      string `json:"payment_method"`
			PercentBasisPoints int    `json:"percent_basis_points"`
			FixedFeeKopecks    int64  `json:"fixed_fee_kopecks"`
			MinimumFeeKopecks  int64  `json:"minimum_fee_kopecks"`
			MaximumFeeKopecks  *int64 `json:"maximum_fee_kopecks"`
			Confirm            bool   `json:"confirm"`
			Reason             string `json:"reason"`
		}
		if !decode(w, r, &in) {
			return
		}
		rule := ProviderPricingRule{
			Provider:           in.Provider,
			PaymentMethod:      in.PaymentMethod,
			PercentBasisPoints: in.PercentBasisPoints,
			FixedFeeKopecks:    in.FixedFeeKopecks,
			MinimumFeeKopecks:  in.MinimumFeeKopecks,
			MaximumFeeKopecks:  in.MaximumFeeKopecks,
		}
		v, err := h.Service.CreateProviderPricing(r.Context(), actor, rule, in.Confirm, in.Reason, requestID(r))
		if handle(w, r, err) {
			return
		}
		reply(w, http.StatusCreated, map[string]any{"data": v})
	default:
		method(w, r)
	}
}

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
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
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func method(w http.ResponseWriter, r *http.Request) {
	problem(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
}
func notFound(w http.ResponseWriter, r *http.Request) {
	problem(w, r, http.StatusNotFound, "NOT_FOUND", "finance resource not found")
}
func handle(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ErrForbidden):
		problem(w, r, http.StatusForbidden, "FORBIDDEN", "finance admin permission required")
	case errors.Is(err, ErrConfirmationRequired):
		problem(w, r, http.StatusUnprocessableEntity, "CONFIRMATION_REQUIRED", "confirmation is required to change payment economics")
	case errors.Is(err, ErrReasonRequired):
		problem(w, r, http.StatusUnprocessableEntity, "REASON_REQUIRED", "a reason is required to change payment economics")
	case errors.Is(err, ErrInvalidInput):
		problem(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid finance rule")
	default:
		problem(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
	return true
}
func problem(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	reply(w, status, map[string]any{"error": map[string]string{"code": code, "message": message, "request_id": requestID(r)}})
}
