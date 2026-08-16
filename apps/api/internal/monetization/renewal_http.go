package monetization

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

type RenewalHandler struct {
	Billing *BillingService
	Token   string
}

func (h RenewalHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || h.Billing == nil || h.Token == "" {
		http.NotFound(w, r)
		return
	}
	got := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if len(got) != len(h.Token) || subtle.ConstantTimeCompare([]byte(got), []byte(h.Token)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	n, err := h.Billing.RunRenewals(r.Context(), limit)
	if err != nil {
		http.Error(w, "renewal processing failed", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]int{"processed": n})
}
