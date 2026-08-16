package payments

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
)

// RoutingHandler exposes only deployment-safe route and enablement controls.
// It accepts no credential material and validates every changed route against
// the compiled capability registry before persisting it.
type RoutingHandler struct {
	Repository  MutableRoutingRepository
	Registry    Registry
	ActorID     func(context.Context) (string, bool)
	ConfigStore *ProviderConfigStore
	Runtime     *ProviderRuntime
}

func (h RoutingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.ActorID(r.Context())
	if !ok {
		routingError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/payment-routing"), "/")
	if path == "" && r.Method == http.MethodGet {
		items, err := h.Repository.ListRoutes(r.Context())
		if err != nil {
			routingError(w, r, 500, "INTERNAL_ERROR", "payment routing unavailable")
			return
		}
		routingReply(w, http.StatusOK, map[string]any{"data": items})
		return
	}
	if path == "providers" && r.Method == http.MethodGet {
		items, err := h.Repository.ListProviders(r.Context())
		if err != nil {
			routingError(w, r, 500, "INTERNAL_ERROR", "payment providers unavailable")
			return
		}
		for i := range items {
			if descriptor, ok := h.Registry.Provider(items[i].Provider); ok {
				for capability, enabled := range descriptor.Capabilities {
					if enabled {
						items[i].Capabilities = append(items[i].Capabilities, capability)
					}
				}
				sort.Slice(items[i].Capabilities, func(a, b int) bool { return items[i].Capabilities[a] < items[i].Capabilities[b] })
			}
		}
		routingReply(w, http.StatusOK, map[string]any{"data": items})
		return
	}

	if strings.HasPrefix(path, "providers/") && strings.HasSuffix(path, "/config") {
		provider := ParseProvider(strings.TrimSuffix(strings.TrimPrefix(path, "providers/"), "/config"))
		if _, found := h.Registry.Provider(provider); !found || provider == ProviderDisabled || h.ConfigStore == nil {
			routingError(w, r, 404, "NOT_FOUND", "provider not found")
			return
		}
		switch r.Method {
		case http.MethodGet:
			view, err := h.ConfigStore.View(r.Context(), provider)
			if err != nil {
				routingError(w, r, 500, "INTERNAL_ERROR", "provider configuration unavailable")
				return
			}
			routingReply(w, http.StatusOK, map[string]any{"data": view})
			return
		case http.MethodPut:
			var in struct {
				Environment Environment       `json:"environment"`
				Values      map[string]string `json:"values"`
			}
			if !routingDecode(w, r, &in) {
				return
			}
			current, exists, err := h.ConfigStore.Get(r.Context(), provider)
			if err != nil {
				routingError(w, r, 500, "INTERNAL_ERROR", "provider configuration unavailable")
				return
			}
			if in.Values == nil {
				in.Values = map[string]string{}
			}
			fields, _ := ProviderConfigTemplate(provider)
			for _, field := range fields {
				if field.Secret && strings.TrimSpace(in.Values[field.Key]) == "" && exists {
					in.Values[field.Key] = current.Values[field.Key]
				}
			}
			cfg := ProviderConfig{Provider: provider, Environment: in.Environment, Values: in.Values}
			if err := ValidateProviderConfig(cfg); err != nil {
				routingError(w, r, 422, "VALIDATION_ERROR", err.Error())
				return
			}
			// Construct a throw-away adapter before committing configuration. Invalid
			// certificate/key material never becomes durable or active.
			probe := NewProviderRuntime()
			if err := probe.Apply(cfg); err != nil {
				routingError(w, r, 422, "VALIDATION_ERROR", "provider configuration is invalid")
				return
			}
			if err := h.ConfigStore.Put(r.Context(), cfg, actor); err != nil {
				log.Printf("payment provider configuration save failed provider=%s error=%v", provider, err)
				routingError(w, r, 500, "INTERNAL_ERROR", "provider configuration unavailable")
				return
			}
			if h.Runtime != nil {
				_ = h.Runtime.Apply(cfg)
			}
			view, _ := h.ConfigStore.View(r.Context(), provider)
			routingReply(w, http.StatusOK, map[string]any{"data": view})
			return
		}
	}
	if strings.HasPrefix(path, "providers/") && r.Method == http.MethodPatch {
		provider := ParseProvider(strings.TrimPrefix(path, "providers/"))
		if _, found := h.Registry.Provider(provider); !found || provider == ProviderDisabled {
			routingError(w, r, 404, "NOT_FOUND", "provider not found")
			return
		}
		var in struct {
			Enabled bool `json:"enabled"`
		}
		if !routingDecode(w, r, &in) {
			return
		}
		if err := h.Repository.SetProviderEnabled(r.Context(), provider, in.Enabled, actor); err != nil {
			if errors.Is(err, ErrProviderUnavailable) {
				routingError(w, r, 422, "PROVIDER_UNAVAILABLE", "provider is not fully configured")
				return
			}
			log.Printf("payment provider enable failed provider=%s enabled=%t error=%v", provider, in.Enabled, err)
			routingError(w, r, 500, "INTERNAL_ERROR", "payment routing unavailable")
			return
		}
		routingReply(w, http.StatusOK, map[string]any{"data": map[string]any{"provider": provider, "enabled": in.Enabled}})
		return
	}
	if strings.HasPrefix(path, "routes/") && r.Method == http.MethodPut {
		domain := Domain(strings.TrimPrefix(path, "routes/"))
		if domain != DomainSafeDeal && domain != DomainProSubscription && domain != DomainPlatformPayment {
			routingError(w, r, 404, "NOT_FOUND", "route not found")
			return
		}
		var in struct {
			Provider ProviderName `json:"provider"`
		}
		if !routingDecode(w, r, &in) {
			return
		}
		required := []Capability{CapabilityOneTimePayment}
		if domain == DomainSafeDeal {
			required = []Capability{CapabilitySafeDeal, CapabilityPayoutCard}
		} else if domain == DomainProSubscription {
			required = []Capability{CapabilityOneTimePayment}
		}
		// Check static capability before mutation; enablement and deployment
		// configuration are checked atomically by the repository immediately
		// before writing the durable route.
		if err := h.Registry.ValidateRoute(in.Provider, true, true, required...); err != nil {
			routingValidationError(w, r, err)
			return
		}
		if domain == DomainProSubscription && !h.Registry.SupportsRecurring(in.Provider) {
			routingValidationError(w, r, ErrUnsupportedRoute)
			return
		}
		v, err := h.Repository.SetRoute(r.Context(), domain, in.Provider, actor)
		if err != nil {
			routingError(w, r, 500, "INTERNAL_ERROR", "payment routing unavailable")
			return
		}
		if err := h.Registry.ValidateRoute(v.Provider, v.Enabled, v.Configured, required...); err != nil {
			routingValidationError(w, r, err)
			return
		}
		if domain == DomainProSubscription && !h.Registry.SupportsRecurring(v.Provider) {
			routingValidationError(w, r, ErrUnsupportedRoute)
			return
		}
		routingReply(w, http.StatusOK, map[string]any{"data": v})
		return
	}
	routingError(w, r, http.StatusNotFound, "NOT_FOUND", "payment routing route not found")
}
func routingDecode(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(target) != nil || d.Decode(&struct{}{}) != io.EOF {
		routingError(w, r, 400, "VALIDATION_ERROR", "invalid request payload")
		return false
	}
	return true
}
func routingValidationError(w http.ResponseWriter, r *http.Request, err error) {
	code, msg := "VALIDATION_ERROR", "invalid payment provider route"
	if errors.Is(err, ErrProviderUnavailable) {
		code, msg = "PROVIDER_UNAVAILABLE", "provider is not enabled and configured"
	}
	if errors.Is(err, ErrUnsupportedRoute) {
		code, msg = "UNSUPPORTED_ROUTE", "provider lacks required capability"
	}
	routingError(w, r, 422, code, msg)
}
func routingReply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func routingError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	routingReply(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "request_id": r.Header.Get("X-Request-ID")}})
}
