package payments

import (
	"io"
	"net/http"
	"net/url"
	"strings"
)

type ProviderWebhookHandler struct{ Service WebhookService }

func (h ProviderWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	provider := ParseProvider(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/payments/providers/"), "/"))
	if strings.Contains(string(provider), "/") {
		provider = ParseProvider(strings.Split(string(provider), "/")[0])
	}
	if _, ok := h.Service.Verifiers[provider]; !ok {
		http.NotFound(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid webhook", http.StatusBadRequest)
		return
	}
	_, err = h.Service.Handle(r.Context(), provider, body, map[string][]string(r.Header))
	if err != nil {
		http.Error(w, "invalid webhook", http.StatusBadRequest)
		return
	}
	writeProviderWebhookSuccess(w, provider, body)
}

// Providers differ in the acknowledgement body they expect. These responses
// are transport acknowledgements only: payment authority has already been
// established by VerifyWebhook and the server-side domain transition above.
func writeProviderWebhookSuccess(w http.ResponseWriter, provider ProviderName, body []byte) {
	switch provider {
	case ProviderYandexPay:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success"}`))
	case ProviderCloudPayments:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":0}`))
	case ProviderTBank:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	case ProviderRobokassa:
		values, _ := url.ParseQuery(string(body))
		invoice := strings.TrimSpace(values.Get("InvId"))
		if invoice == "" {
			invoice = strings.TrimSpace(values.Get("InvID"))
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK" + invoice))
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
