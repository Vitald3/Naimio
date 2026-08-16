package payments

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strconv"
)

// ReconciliationHandler is intentionally an internal endpoint: provider
// credentials remain in the API process while the worker only schedules jobs.
// It does not accept a provider name, preventing cross-provider reconciliation.
type ReconciliationHandler struct {
	Reconciler Reconciler
	Token      string
}

func (h ReconciliationHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || h.Token == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+h.Token)) != 1 {
		http.NotFound(w, r)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 100
	}
	processed, err := h.Reconciler.Run(r.Context(), limit)
	if err != nil {
		// Provider failures are contained by the reconciler. A repository failure
		// is retried by the worker without disclosing implementation details.
		http.Error(w, "reconciliation unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]int{"processed": processed}})
}
