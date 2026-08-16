package payments

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// TBank implements the documented internet-acquiring v2 methods. Marketplace
// nominal-account operations deliberately remain outside this adapter until a
// separately contracted API credential is supplied.
type TBankConfig struct {
	TerminalKey, Password, BaseURL string
	Client                         *http.Client
}
type TBank struct {
	terminal, password, base string
	client                   *http.Client
}

func NewTBank(c TBankConfig) (*TBank, error) {
	if c.TerminalKey == "" || c.Password == "" {
		return nil, ErrProviderUnavailable
	}
	if c.BaseURL == "" {
		c.BaseURL = "https://securepay.tinkoff.ru/v2"
	}
	if c.Client == nil {
		c.Client = &http.Client{Timeout: 15 * time.Second}
	}
	return &TBank{c.TerminalKey, c.Password, strings.TrimRight(c.BaseURL, "/"), c.Client}, nil
}
func (p *TBank) CreatePayment(ctx context.Context, amount int64, orderID, successURL, failURL string) (string, string, error) {
	return p.createPayment(ctx, amount, orderID, successURL, failURL, false, "")
}

func (p *TBank) createPayment(ctx context.Context, amount int64, orderID, successURL, failURL string, recurrent bool, customerKey string) (string, string, error) {
	if amount <= 0 {
		return "", "", ErrInvalidAttempt
	}
	v := map[string]any{"TerminalKey": p.terminal, "Amount": amount, "OrderId": orderID, "SuccessURL": successURL, "FailURL": failURL}
	if recurrent {
		if strings.TrimSpace(customerKey) == "" {
			return "", "", ErrInvalidAttempt
		}
		// T-Bank requires the parent card payment to be initialized as
		// recurrent together with CustomerKey before it can issue RebillId.
		v["Recurrent"] = "Y"
		v["CustomerKey"] = customerKey
	}
	v["Token"] = p.token(v)
	var out struct {
		Success    bool
		PaymentID  string `json:"PaymentId"`
		PaymentURL string `json:"PaymentURL"`
		Message    string
	}
	if e := p.post(ctx, "/Init", v, &out); e != nil {
		return "", "", e
	}
	if !out.Success || out.PaymentID == "" {
		return "", "", fmt.Errorf("provider declined")
	}
	return out.PaymentID, out.PaymentURL, nil
}

func (p *TBank) GetStatus(ctx context.Context, id string) (Status, string, error) {
	details, err := p.GetStatusDetails(ctx, id)
	return details.Status, details.RawStatus, err
}
func (p *TBank) Refund(ctx context.Context, id, key string, amount int64) error {
	v := map[string]any{"TerminalKey": p.terminal, "PaymentId": id, "Amount": amount, "RequestKey": key}
	v["Token"] = p.token(v)
	var out struct{ Success bool }
	if e := p.post(ctx, "/Cancel", v, &out); e != nil {
		return e
	}
	if !out.Success {
		return fmt.Errorf("provider declined")
	}
	return nil
}

// VerifyWebhook checks T-Bank's Token before interpreting notification fields.
// The provider signs the same top-level primitive fields used for request tokens.
func (p *TBank) CancelPurchase(ctx context.Context, id, key string, amount int64) (Status, string, error) {
	if amount <= 0 {
		return StatusFailed, "", ErrInvalidAttempt
	}
	if err := p.Refund(ctx, id, key, amount); err != nil {
		return StatusUnknownReconciliation, "", err
	}
	// T-Bank uses /Cancel for both an authorization cancellation and a refund.
	// This method is called only for non-terminal standalone payments, so the
	// normalized operation here is a cancellation.
	return StatusCanceled, "CANCELED", nil
}

func (p *TBank) VerifyWebhook(_ context.Context, body []byte, _ map[string][]string) (VerifiedEvent, error) {
	var values map[string]any
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	if d.Decode(&values) != nil {
		return VerifiedEvent{}, ErrWebhookInvalid
	}
	token, _ := values["Token"].(string)
	if token == "" || !constantEqual(token, p.token(values)) {
		return VerifiedEvent{}, ErrWebhookInvalid
	}
	id := fmt.Sprint(values["PaymentId"])
	status := strings.ToUpper(fmt.Sprint(values["Status"]))
	if id == "" || id == "<nil>" {
		return VerifiedEvent{}, ErrWebhookInvalid
	}
	mapped, ok := map[string]Status{"NEW": StatusPendingUserAction, "FORM_SHOWED": StatusPendingUserAction, "AUTHORIZING": StatusProcessing, "AUTHORIZED": StatusAuthorized, "CONFIRMED": StatusSucceeded, "REJECTED": StatusFailed, "CANCELED": StatusCanceled, "REFUNDED": StatusRefunded}[status]
	if !ok {
		mapped = StatusUnknownReconciliation
	}
	eventID := fmt.Sprint(values["OrderId"]) + ":" + id + ":" + status
	rebillID := ""
	if value, ok := values["RebillId"]; ok && value != nil {
		rebillID = strings.TrimSpace(fmt.Sprint(value))
	}
	return VerifiedEvent{Provider: ProviderTBank, ID: eventID, ExternalOperationID: id, Type: "PAYMENT_STATUS", RawStatus: status, Status: mapped, SavedMethodRef: rebillID}, nil
}
func providerOrderID(prefix, reference, key string) string {
	sum := sha256.Sum256([]byte(prefix + ":" + reference + ":" + key))
	return prefix + "-" + hex.EncodeToString(sum[:12])
}

func (p *TBank) post(ctx context.Context, path string, in, out any) error {
	b, e := json.Marshal(in)
	if e != nil {
		return e
	}
	r, e := http.NewRequestWithContext(ctx, http.MethodPost, p.base+path, strings.NewReader(string(b)))
	if e != nil {
		return e
	}
	r.Header.Set("Content-Type", "application/json")
	res, e := p.client.Do(r)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("provider status %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(out)
}
func (p *TBank) token(values map[string]any) string {
	keys := make([]string, 0, len(values))
	for k, v := range values {
		if k != "Token" {
			switch v.(type) {
			case string, int, int64, json.Number:
				keys = append(keys, k)
			}
		}
	}
	sort.Strings(keys)
	b := strings.Builder{}
	for _, k := range keys {
		b.WriteString(fmt.Sprint(values[k]))
	}
	b.WriteString(p.password)
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func (p *TBank) CreatePurchase(ctx context.Context, in PurchaseRequest) (PurchaseResult, error) {
	id, u, err := p.createPayment(ctx, in.AmountKopecks, in.InternalReferenceID, in.ReturnURL, in.FailURL, in.SavePaymentMethod, in.CustomerID)
	if err != nil {
		return PurchaseResult{}, err
	}
	return purchaseResult(id, StatusPendingUserAction, "NEW", u, "")
}
func (p *TBank) GetStatusDetails(ctx context.Context, id string) (StatusDetails, error) {
	v := map[string]any{"TerminalKey": p.terminal, "PaymentId": id}
	v["Token"] = p.token(v)
	var out struct {
		Success  bool
		Status   string
		RebillID string `json:"RebillId"`
	}
	if err := p.post(ctx, "/GetState", v, &out); err != nil {
		return StatusDetails{}, err
	}
	raw := strings.ToUpper(out.Status)
	mapped, ok := map[string]Status{"NEW": StatusPendingUserAction, "FORM_SHOWED": StatusPendingUserAction, "AUTHORIZING": StatusProcessing, "AUTHORIZED": StatusAuthorized, "CONFIRMED": StatusSucceeded, "REJECTED": StatusFailed, "CANCELED": StatusCanceled, "REFUNDED": StatusRefunded}[raw]
	if !ok {
		mapped = StatusUnknownReconciliation
	}
	return StatusDetails{Status: mapped, RawStatus: raw, SavedMethodRef: out.RebillID}, nil
}

func (p *TBank) RefundPurchase(ctx context.Context, id, key string, amount int64) (string, error) {
	if err := p.Refund(ctx, id, key, amount); err != nil {
		return "", err
	}
	return id, nil
}

// ChargeRecurring performs a COF charge. The caller must first create a new
// payment attempt/payment with Init and then use the provider-issued RebillId.
func (p *TBank) ChargeRecurring(ctx context.Context, in PurchaseRequest) (PurchaseResult, error) {
	if in.SavedMethodRef == "" {
		return PurchaseResult{}, ErrInvalidAttempt
	}
	paymentID, _, err := p.createPayment(ctx, in.AmountKopecks, providerOrderID("renew", in.InternalReferenceID, in.IdempotencyKey), in.ReturnURL, in.FailURL, false, "")
	if err != nil {
		return PurchaseResult{}, err
	}
	v := map[string]any{"TerminalKey": p.terminal, "PaymentId": paymentID, "RebillId": in.SavedMethodRef}
	v["Token"] = p.token(v)
	var out struct {
		Success   bool
		Status    string
		PaymentID string `json:"PaymentId"`
	}
	if err := p.post(ctx, "/Charge", v, &out); err != nil {
		return PurchaseResult{}, err
	}
	if !out.Success {
		return PurchaseResult{}, fmt.Errorf("provider declined")
	}
	if out.PaymentID == "" {
		out.PaymentID = paymentID
	}
	st, ok := map[string]Status{"CONFIRMED": StatusSucceeded, "AUTHORIZED": StatusAuthorized, "REJECTED": StatusFailed, "CANCELED": StatusCanceled}[strings.ToUpper(out.Status)]
	if !ok {
		st = StatusProcessing
	}
	return purchaseResult(out.PaymentID, st, out.Status, "", in.SavedMethodRef)
}
