package payments

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
)

var (
	payoutTokenPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{16,500}$`)
	cardDigitsPattern  = regexp.MustCompile(`^[0-9]{4,6}$`)
)

type PayoutRecipientBinding struct {
	Provider          ProviderName      `json:"provider"`
	Status            string            `json:"status"`
	MaskedDestination string            `json:"masked_destination,omitempty"`
	SafeDetails       map[string]string `json:"safe_details,omitempty"`
	SetupKind         string            `json:"setup_kind"`
	AccountID         string            `json:"account_id,omitempty"`
}

type PayoutRecipientHandler struct {
	DB                     *sql.DB
	Routing                RoutingService
	YooKassaShopID         string
	YooKassaShopIDResolver func(context.Context) string
	ActorID                func(context.Context) (string, bool)
}

func (h PayoutRecipientHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.ActorID(r.Context())
	if !ok {
		routingError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return
	}
	provider, err := h.Routing.Select(r.Context(), DomainSafeDeal, CapabilitySafeDeal, CapabilityPayoutCard)
	if err != nil {
		routingError(w, r, http.StatusServiceUnavailable, "PAYOUT_UNAVAILABLE", "payout setup is temporarily unavailable")
		return
	}
	if r.Method == http.MethodGet {
		binding, err := h.get(r.Context(), actor, provider)
		if err != nil {
			routingError(w, r, 500, "INTERNAL_ERROR", "payout setup unavailable")
			return
		}
		routingReply(w, 200, map[string]any{"data": binding})
		return
	}
	if r.Method != http.MethodPost {
		routingError(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	shopID := strings.TrimSpace(h.YooKassaShopID)
	if h.YooKassaShopIDResolver != nil {
		shopID = strings.TrimSpace(h.YooKassaShopIDResolver(r.Context()))
	}
	if provider != ProviderYooKassa || shopID == "" {
		routingError(w, r, 422, "UNSUPPORTED_ROUTE", "selected provider does not support payout binding in this deployment")
		return
	}
	var in struct {
		PayoutToken   string `json:"payout_token"`
		First6        string `json:"first6"`
		Last4         string `json:"last4"`
		IssuerName    string `json:"issuer_name"`
		IssuerCountry string `json:"issuer_country"`
		CardType      string `json:"card_type"`
	}
	if !routingDecode(w, r, &in) {
		return
	}
	in.PayoutToken = strings.TrimSpace(in.PayoutToken)
	in.First6, in.Last4 = strings.TrimSpace(in.First6), strings.TrimSpace(in.Last4)
	if !payoutTokenPattern.MatchString(in.PayoutToken) || !cardDigitsPattern.MatchString(in.First6) || len(in.First6) != 6 || !cardDigitsPattern.MatchString(in.Last4) || len(in.Last4) != 4 {
		routingError(w, r, 422, "VALIDATION_ERROR", "invalid payout token result")
		return
	}
	details := map[string]string{"first6": in.First6, "last4": in.Last4, "issuer_name": safeText(in.IssuerName, 120), "issuer_country": safeText(in.IssuerCountry, 2), "card_type": safeText(in.CardType, 40)}
	body, _ := json.Marshal(details)
	_, err = h.DB.ExecContext(r.Context(), `INSERT INTO payout_recipient_bindings(user_id,provider,external_reference,status,safe_details) VALUES($1::uuid,$2,$3,'VERIFIED',$4::jsonb) ON CONFLICT(user_id,provider) DO UPDATE SET external_reference=excluded.external_reference,status='VERIFIED',safe_details=excluded.safe_details,updated_at=now()`, actor, provider, in.PayoutToken, body)
	if err != nil {
		routingError(w, r, 500, "INTERNAL_ERROR", "payout setup unavailable")
		return
	}
	binding, _ := h.get(r.Context(), actor, provider)
	routingReply(w, 200, map[string]any{"data": binding})
}

func (h PayoutRecipientHandler) get(ctx context.Context, actor string, provider ProviderName) (PayoutRecipientBinding, error) {
	out := PayoutRecipientBinding{Provider: provider, Status: "NOT_CONFIGURED"}
	if provider == ProviderYooKassa {
		out.SetupKind = "YOOKASSA_PAYOUT_WIDGET"
		out.AccountID = strings.TrimSpace(h.YooKassaShopID)
		if h.YooKassaShopIDResolver != nil {
			out.AccountID = strings.TrimSpace(h.YooKassaShopIDResolver(ctx))
		}
	} else {
		out.SetupKind = "PROVIDER_HOSTED"
	}
	var raw []byte
	err := h.DB.QueryRowContext(ctx, `SELECT status,safe_details FROM payout_recipient_bindings WHERE user_id=$1::uuid AND provider=$2`, actor, provider).Scan(&out.Status, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return PayoutRecipientBinding{}, err
	}
	_ = json.Unmarshal(raw, &out.SafeDetails)
	if first6, last4 := out.SafeDetails["first6"], out.SafeDetails["last4"]; len(first6) == 6 && len(last4) == 4 {
		out.MaskedDestination = first6 + "••••••" + last4
	}
	return out, nil
}

func safeText(v string, max int) string {
	v = strings.TrimSpace(v)
	if len(v) > max {
		return v[:max]
	}
	return v
}
