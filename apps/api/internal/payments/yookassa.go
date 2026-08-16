package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type YooKassaConfig struct {
	ShopID, SecretKey, BaseURL string
	Client                     *http.Client
}
type YooKassa struct {
	shopID, secret, baseURL string
	client                  *http.Client
}

func NewYooKassa(c YooKassaConfig) (*YooKassa, error) {
	if strings.TrimSpace(c.ShopID) == "" || strings.TrimSpace(c.SecretKey) == "" {
		return nil, ErrProviderUnavailable
	}
	if c.BaseURL == "" {
		c.BaseURL = "https://api.yookassa.ru/v3"
	}
	if c.Client == nil {
		c.Client = &http.Client{Timeout: 15 * time.Second}
	}
	return &YooKassa{c.ShopID, c.SecretKey, strings.TrimRight(c.BaseURL, "/"), c.Client}, nil
}

type CheckoutRequest struct {
	AmountKopecks                                                     int64
	Currency, IdempotencyKey, Description, ReturnURL, PaymentMethodID string
	SavePaymentMethod                                                 bool
}
type CheckoutResult struct{ ExternalID, Status, ConfirmationURL, PaymentMethodID string }

// SafeDealRequest mirrors YooKassa's dedicated marketplace flow. It is kept
// separate from ordinary payouts so callers cannot accidentally model an
// escrow as a card authorization hold.
type SafeDealRequest struct {
	IdempotencyKey, Description string
	FeeMoment                   string // payment_succeeded or deal_closed
}
type SafeDealPaymentRequest struct {
	DealID, IdempotencyKey, Description, ReturnURL string
	AmountKopecks, PayoutKopecks                   int64
	Capture                                        bool
}
type SafeDealPayoutRequest struct {
	DealID, PayoutToken, IdempotencyKey, Description string
	AmountKopecks                                    int64
}

func (p *YooKassa) CreatePayment(ctx context.Context, in CheckoutRequest) (CheckoutResult, error) {
	if in.AmountKopecks <= 0 || in.Currency != "RUB" || len(in.IdempotencyKey) < 8 {
		return CheckoutResult{}, ErrInvalidAttempt
	}
	body := map[string]any{"amount": map[string]string{"value": rub(in.AmountKopecks), "currency": "RUB"}, "capture": true, "description": in.Description, "confirmation": map[string]string{"type": "redirect", "return_url": in.ReturnURL}, "save_payment_method": in.SavePaymentMethod}
	if in.PaymentMethodID != "" {
		body["payment_method_id"] = in.PaymentMethodID
	}
	var out struct {
		ID, Status   string
		Confirmation struct {
			ConfirmationURL string `json:"confirmation_url"`
		} `json:"confirmation"`
		PaymentMethod struct {
			ID string `json:"id"`
		} `json:"payment_method"`
	}
	if err := p.do(ctx, http.MethodPost, "/payments", in.IdempotencyKey, body, &out); err != nil {
		return CheckoutResult{}, err
	}
	return CheckoutResult{out.ID, out.Status, out.Confirmation.ConfirmationURL, out.PaymentMethod.ID}, nil
}

func (p *YooKassa) CreateSafeDeal(ctx context.Context, in SafeDealRequest) (string, string, error) {
	if len(in.IdempotencyKey) < 8 || (in.FeeMoment != "payment_succeeded" && in.FeeMoment != "deal_closed") {
		return "", "", ErrInvalidAttempt
	}
	var out struct{ ID, Status string }
	err := p.do(ctx, http.MethodPost, "/deals", in.IdempotencyKey, map[string]any{"type": "safe_deal", "fee_moment": in.FeeMoment, "description": in.Description}, &out)
	return out.ID, out.Status, err
}

// CreateSafeDealPayment funds a previously-created YooKassa deal and records
// the settlement that is later released through the dedicated payout endpoint.
func (p *YooKassa) CreateSafeDealPayment(ctx context.Context, in SafeDealPaymentRequest) (CheckoutResult, error) {
	if in.DealID == "" || in.AmountKopecks <= 0 || in.PayoutKopecks <= 0 || in.PayoutKopecks > in.AmountKopecks || len(in.IdempotencyKey) < 8 {
		return CheckoutResult{}, ErrInvalidAttempt
	}
	body := map[string]any{"amount": map[string]string{"value": rub(in.AmountKopecks), "currency": "RUB"}, "capture": in.Capture, "description": in.Description, "confirmation": map[string]string{"type": "redirect", "return_url": in.ReturnURL}, "deal": map[string]any{"id": in.DealID, "settlements": []any{map[string]any{"type": "payout", "amount": map[string]string{"value": rub(in.PayoutKopecks), "currency": "RUB"}}}}}
	var out struct {
		ID, Status   string
		Confirmation struct {
			ConfirmationURL string `json:"confirmation_url"`
		} `json:"confirmation"`
		PaymentMethod struct {
			ID string `json:"id"`
		} `json:"payment_method"`
	}
	if err := p.do(ctx, http.MethodPost, "/payments", in.IdempotencyKey, body, &out); err != nil {
		return CheckoutResult{}, err
	}
	return CheckoutResult{out.ID, out.Status, out.Confirmation.ConfirmationURL, out.PaymentMethod.ID}, nil
}

func (p *YooKassa) CreateSafeDealPayout(ctx context.Context, in SafeDealPayoutRequest) (string, string, error) {
	if in.DealID == "" || in.PayoutToken == "" || in.AmountKopecks <= 0 || len(in.IdempotencyKey) < 8 {
		return "", "", ErrInvalidAttempt
	}
	var out struct{ ID, Status string }
	body := map[string]any{"amount": map[string]string{"value": rub(in.AmountKopecks), "currency": "RUB"}, "payout_token": in.PayoutToken, "description": in.Description, "deal": map[string]string{"id": in.DealID}}
	err := p.do(ctx, http.MethodPost, "/payouts", in.IdempotencyKey, body, &out)
	return out.ID, out.Status, err
}

func (p *YooKassa) GetDeal(ctx context.Context, id string) (string, error) {
	var out struct{ Status string }
	err := p.do(ctx, http.MethodGet, "/deals/"+id, "", nil, &out)
	return out.Status, err
}
func (p *YooKassa) GetPayout(ctx context.Context, id string) (string, error) {
	var out struct{ Status string }
	err := p.do(ctx, http.MethodGet, "/payouts/"+id, "", nil, &out)
	return out.Status, err
}

func (p *YooKassa) GetRefund(ctx context.Context, id string) (Status, string, error) {
	var out struct {
		Status string `json:"status"`
	}
	if err := p.do(ctx, http.MethodGet, "/refunds/"+id, "", nil, &out); err != nil {
		return StatusUnknownReconciliation, "", err
	}
	raw := strings.ToLower(out.Status)
	switch raw {
	case "succeeded":
		return StatusRefunded, out.Status, nil
	case "canceled":
		return StatusFailed, out.Status, nil
	default:
		return StatusProcessing, out.Status, nil
	}
}

func yooPayoutStatus(v string) Status {
	switch strings.ToLower(v) {
	case "succeeded":
		return StatusSucceeded
	case "canceled":
		return StatusFailed
	default:
		return StatusProcessing
	}
}

func (p *YooKassa) GetPayment(ctx context.Context, id string) (Status, string, error) {
	s, raw, _, err := p.getPaymentDetails(ctx, id)
	return s, raw, err
}
func (p *YooKassa) getPaymentDetails(ctx context.Context, id string) (Status, string, string, error) {
	var out struct {
		Status        string `json:"status"`
		PaymentMethod struct {
			ID    string `json:"id"`
			Saved bool   `json:"saved"`
		} `json:"payment_method"`
	}
	if err := p.do(ctx, http.MethodGet, "/payments/"+id, "", nil, &out); err != nil {
		return "", "", "", err
	}
	s, ok := yooStatus(out.Status)
	if !ok {
		s = StatusUnknownReconciliation
	}
	ref := ""
	if out.PaymentMethod.Saved {
		ref = out.PaymentMethod.ID
	}
	return s, out.Status, ref, nil
}
func (p *YooKassa) GetStatus(ctx context.Context, id string) (Status, string, error) {
	return p.GetPayment(ctx, id)
}

// GetStatusDetails preserves a saved payment-method reference discovered by an
// authoritative provider lookup. This is used by PRO billing so recurring
// renewals never depend on browser-return data.
func (p *YooKassa) GetStatusDetails(ctx context.Context, id string) (StatusDetails, error) {
	status, raw, savedMethod, err := p.getPaymentDetails(ctx, id)
	return StatusDetails{Status: status, RawStatus: raw, SavedMethodRef: savedMethod}, err
}

// GetOperationStatus distinguishes YooKassa resource types. Refund and payout
// attempts have their own identifiers/endpoints and therefore must never be
// reconciled through GET /payments/{id}.
func (p *YooKassa) GetOperationStatus(ctx context.Context, operation OperationType, id string) (Status, string, error) {
	switch operation {
	case OperationRefund:
		return p.GetRefund(ctx, id)
	case OperationPayout:
		raw, err := p.GetPayout(ctx, id)
		if err != nil {
			return StatusUnknownReconciliation, "", err
		}
		return yooPayoutStatus(raw), raw, nil
	default:
		return p.GetPayment(ctx, id)
	}
}

func (p *YooKassa) GetOperationStatusDetails(ctx context.Context, operation OperationType, id string) (StatusDetails, error) {
	if operation == OperationPayment || operation == OperationRenewal {
		return p.GetStatusDetails(ctx, id)
	}
	status, raw, err := p.GetOperationStatus(ctx, operation, id)
	return StatusDetails{Status: status, RawStatus: raw}, err
}

// VerifyWebhook uses YooKassa's documented authoritative status lookup. Their
// HTTP Basic Auth notifications do not carry a shared signature, so accepting
// a body alone would be forgeable; no state is emitted until GET /payments/id
// confirms the status with the merchant credential.
func (p *YooKassa) VerifyWebhook(ctx context.Context, body []byte, _ map[string][]string) (VerifiedEvent, error) {
	if len(body) == 0 || len(body) > 256<<10 {
		return VerifiedEvent{}, ErrWebhookInvalid
	}
	var in struct {
		Event  string `json:"event"`
		Object struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"object"`
	}
	if json.Unmarshal(body, &in) != nil || in.Object.ID == "" || in.Event == "" {
		return VerifiedEvent{}, ErrWebhookInvalid
	}
	var status Status
	var raw string
	var err error
	var savedMethod string
	switch {
	case strings.HasPrefix(in.Event, "payout."):
		raw, err = p.GetPayout(ctx, in.Object.ID)
		status = yooPayoutStatus(raw)
	case strings.HasPrefix(in.Event, "refund."):
		status, raw, err = p.GetRefund(ctx, in.Object.ID)
	default:
		status, raw, savedMethod, err = p.getPaymentDetails(ctx, in.Object.ID)
	}
	if err != nil || raw != in.Object.Status || status == StatusUnknownReconciliation {
		return VerifiedEvent{}, ErrWebhookInvalid
	}
	return VerifiedEvent{Provider: ProviderYooKassa, ID: in.Event + ":" + in.Object.ID + ":" + raw, ExternalOperationID: in.Object.ID, Type: in.Event, RawStatus: raw, Status: status, SavedMethodRef: savedMethod}, nil
}
func (p *YooKassa) CancelPayment(ctx context.Context, paymentID, key string) (Status, string, error) {
	if strings.TrimSpace(paymentID) == "" || len(key) < 8 {
		return StatusUnknownReconciliation, "", ErrInvalidAttempt
	}
	var out struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := p.do(ctx, http.MethodPost, "/payments/"+paymentID+"/cancel", key, map[string]any{}, &out); err != nil {
		return StatusUnknownReconciliation, "", err
	}
	status, ok := yooStatus(out.Status)
	if !ok {
		status = StatusUnknownReconciliation
	}
	return status, out.Status, nil
}

func (p *YooKassa) Refund(ctx context.Context, paymentID, key string, amount int64) (string, error) {
	var out struct{ ID string }
	err := p.do(ctx, http.MethodPost, "/refunds", key, map[string]any{"payment_id": paymentID, "amount": map[string]string{"value": rub(amount), "currency": "RUB"}}, &out)
	return out.ID, err
}

func (p *YooKassa) CancelPurchase(ctx context.Context, paymentID, key string, _ int64) (Status, string, error) {
	return p.CancelPayment(ctx, paymentID, key)
}

func (p *YooKassa) RefundSafeDeal(ctx context.Context, paymentID, dealID, key string, amount, payoutAmount int64) (string, error) {
	if paymentID == "" || dealID == "" || amount <= 0 || payoutAmount < 0 || payoutAmount > amount || len(key) < 8 {
		return "", ErrInvalidAttempt
	}
	var out struct{ ID string }
	body := map[string]any{"payment_id": paymentID, "amount": map[string]string{"value": rub(amount), "currency": "RUB"}, "deal": map[string]any{"id": dealID, "refund_settlements": []any{map[string]any{"type": "payout", "amount": map[string]string{"value": rub(payoutAmount), "currency": "RUB"}}}}}
	err := p.do(ctx, http.MethodPost, "/refunds", key, body, &out)
	return out.ID, err
}
func (p *YooKassa) do(ctx context.Context, method, path, key string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, e := json.Marshal(in)
		if e != nil {
			return e
		}
		body = bytes.NewReader(b)
	}
	r, e := http.NewRequestWithContext(ctx, method, p.baseURL+path, body)
	if e != nil {
		return e
	}
	r.SetBasicAuth(p.shopID, p.secret)
	r.Header.Set("Accept", "application/json")
	if in != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		r.Header.Set("Idempotence-Key", key)
	}
	resp, e := p.client.Do(r)
	if e != nil {
		return fmt.Errorf("provider request: %w", e)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("provider status %d", resp.StatusCode)
	}
	if e = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); e != nil {
		return fmt.Errorf("provider response: %w", e)
	}
	return nil
}
func rub(k int64) string { return fmt.Sprintf("%d.%02d", k/100, k%100) }
func yooStatus(v string) (Status, bool) {
	switch v {
	case "pending":
		return StatusPendingUserAction, true
	case "waiting_for_capture":
		return StatusAuthorized, true
	case "succeeded":
		return StatusSucceeded, true
	case "canceled":
		return StatusCanceled, true
	default:
		return "", false
	}
}

// CreatePurchase exposes YooKassa through the provider-neutral commerce boundary.
func (p *YooKassa) CreatePurchase(ctx context.Context, in PurchaseRequest) (PurchaseResult, error) {
	out, err := p.CreatePayment(ctx, CheckoutRequest{AmountKopecks: in.AmountKopecks, Currency: in.Currency, IdempotencyKey: in.IdempotencyKey, Description: in.Description, ReturnURL: in.ReturnURL, PaymentMethodID: in.SavedMethodRef, SavePaymentMethod: in.SavePaymentMethod})
	if err != nil {
		return PurchaseResult{}, err
	}
	status, ok := yooStatus(out.Status)
	if !ok {
		status = StatusUnknownReconciliation
	}
	return purchaseResult(out.ExternalID, status, out.Status, out.ConfirmationURL, out.PaymentMethodID)
}
func (p *YooKassa) ChargeRecurring(ctx context.Context, in PurchaseRequest) (PurchaseResult, error) {
	if in.SavedMethodRef == "" {
		return PurchaseResult{}, ErrInvalidAttempt
	}
	return p.CreatePurchase(ctx, in)
}
func (p *YooKassa) RefundPurchase(ctx context.Context, id, key string, amount int64) (string, error) {
	return p.Refund(ctx, id, key, amount)
}
