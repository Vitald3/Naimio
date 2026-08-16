package payments

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// CloudPayments uses a provider-hosted widget cryptogram/token. This adapter
// intentionally receives only that opaque value and never stores card data.
type CloudPaymentsConfig struct {
	PublicID, APISecret, BaseURL string
	Client                       *http.Client
}
type CloudPayments struct {
	publicID, secret, base string
	client                 *http.Client
}

func NewCloudPayments(c CloudPaymentsConfig) (*CloudPayments, error) {
	if c.PublicID == "" || c.APISecret == "" {
		return nil, ErrProviderUnavailable
	}
	if c.BaseURL == "" {
		c.BaseURL = "https://api.cloudpayments.ru"
	}
	if c.Client == nil {
		c.Client = &http.Client{Timeout: 15 * time.Second}
	}
	return &CloudPayments{c.PublicID, c.APISecret, strings.TrimRight(c.BaseURL, "/"), c.Client}, nil
}
func (p *CloudPayments) ChargeToken(ctx context.Context, amount int64, invoiceID, token string) (string, Status, error) {
	if amount <= 0 || token == "" {
		return "", StatusFailed, ErrInvalidAttempt
	}
	var out struct {
		Success bool
		Model   struct {
			TransactionID int64 `json:"TransactionId"`
			Status        string
		}
	}
	// CloudPayments accepts a decimal RUB amount. Keep kopecks at the boundary
	// and serialize only here, so a caller can never accidentally charge 100x.
	e := p.post(ctx, "/payments/tokens/charge", map[string]any{"Amount": rub(amount), "Currency": "RUB", "InvoiceId": invoiceID, "Token": token}, &out)
	if e != nil {
		return "", StatusUnknownReconciliation, e
	}
	s := StatusFailed
	if out.Success {
		s = StatusSucceeded
	}
	return fmt.Sprint(out.Model.TransactionID), s, nil
}
func cloudStatus(raw string) Status {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "COMPLETED":
		return StatusSucceeded
	case "AUTHORIZED":
		return StatusAuthorized
	case "DECLINED":
		return StatusFailed
	case "CANCELLED", "CANCELED", "VOIDED":
		return StatusCanceled
	case "REFUNDED":
		return StatusRefunded
	case "CREATED", "AWAITINGAUTHENTICATION", "PENDING":
		return StatusProcessing
	default:
		return StatusUnknownReconciliation
	}
}

// GetStatus intentionally reconciles by Naimio InvoiceId. Hosted checkout does
// not know the eventual CloudPayments TransactionId until the payer completes
// the form, while InvoiceId is stable across redirect, webhook and recovery.
func (p *CloudPayments) GetStatus(ctx context.Context, id string) (Status, string, error) {
	details, err := p.GetStatusDetails(ctx, id)
	return details.Status, details.RawStatus, err
}

func (p *CloudPayments) GetStatusDetails(ctx context.Context, id string) (StatusDetails, error) {
	var out struct {
		Success bool
		Model   struct {
			TransactionID int64  `json:"TransactionId"`
			Status        string `json:"Status"`
			Token         string `json:"Token"`
			InvoiceID     string `json:"InvoiceId"`
		}
		Message string
	}
	if err := p.post(ctx, "/v2/payments/find", map[string]any{"InvoiceId": id}, &out); err != nil {
		return StatusDetails{Status: StatusUnknownReconciliation}, err
	}
	if !out.Success {
		return StatusDetails{Status: StatusUnknownReconciliation, RawStatus: strings.TrimSpace(out.Message)}, nil
	}
	return StatusDetails{Status: cloudStatus(out.Model.Status), RawStatus: out.Model.Status, SavedMethodRef: strings.TrimSpace(out.Model.Token)}, nil
}

func (p *CloudPayments) transactionByInvoice(ctx context.Context, invoiceID string) (int64, string, error) {
	var out struct {
		Success bool
		Model   struct {
			TransactionID int64 `json:"TransactionId"`
			Status        string
		}
	}
	if err := p.post(ctx, "/v2/payments/find", map[string]any{"InvoiceId": invoiceID}, &out); err != nil {
		return 0, "", err
	}
	if !out.Success || out.Model.TransactionID == 0 {
		return 0, out.Model.Status, fmt.Errorf("provider payment not found")
	}
	return out.Model.TransactionID, out.Model.Status, nil
}
func (p *CloudPayments) Refund(ctx context.Context, id string, amount int64) (string, error) {
	if amount <= 0 {
		return "", ErrInvalidAttempt
	}
	var out struct {
		Success bool
		Model   struct {
			TransactionID int64 `json:"TransactionId"`
		}
	}
	if err := p.post(ctx, "/payments/refund", map[string]any{"TransactionId": id, "Amount": rub(amount)}, &out); err != nil {
		return "", err
	}
	if !out.Success {
		return "", fmt.Errorf("provider declined")
	}
	return fmt.Sprint(out.Model.TransactionID), nil
}
func (p *CloudPayments) Confirm(ctx context.Context, id string, amount int64) error {
	return p.operation(ctx, "/payments/confirm", map[string]any{"TransactionId": id, "Amount": rub(amount)})
}
func (p *CloudPayments) Void(ctx context.Context, id string) error {
	return p.operation(ctx, "/payments/void", map[string]any{"TransactionId": id})
}
func (p *CloudPayments) CancelPurchase(ctx context.Context, invoiceID, _ string, _ int64) (Status, string, error) {
	tx, raw, err := p.transactionByInvoice(ctx, invoiceID)
	if err != nil {
		return StatusUnknownReconciliation, raw, err
	}
	if err := p.Void(ctx, fmt.Sprint(tx)); err != nil {
		return StatusUnknownReconciliation, raw, err
	}
	return StatusCanceled, "VOIDED", nil
}

func (p *CloudPayments) operation(ctx context.Context, path string, in any) error {
	var out struct{ Success bool }
	if err := p.post(ctx, path, in, &out); err != nil {
		return err
	}
	if !out.Success {
		return fmt.Errorf("provider declined")
	}
	return nil
}
func (p *CloudPayments) VerifyWebhook(_ context.Context, body []byte, headers map[string][]string) (VerifiedEvent, error) {
	v := first(headers["Content-HMAC"])
	if v == "" {
		v = first(headers["X-Content-HMAC"])
	}
	mac := hmac.New(sha256.New, []byte(p.secret))
	_, _ = mac.Write(body)
	if !hmac.Equal([]byte(base64.StdEncoding.EncodeToString(mac.Sum(nil))), []byte(v)) {
		return VerifiedEvent{}, ErrWebhookInvalid
	}
	// CloudPayments can send notifications as either form data or JSON. Parse
	// both without ever accepting card data as an application input.
	var transactionID, invoiceID, raw, token string
	var js struct {
		TransactionID any    `json:"TransactionId"`
		InvoiceID     string `json:"InvoiceId"`
		Status        string `json:"Status"`
		Token         string `json:"Token"`
	}
	if json.Unmarshal(body, &js) == nil && (js.InvoiceID != "" || js.TransactionID != nil) {
		transactionID = strings.TrimSpace(fmt.Sprint(js.TransactionID))
		if transactionID == "<nil>" {
			transactionID = ""
		}
		invoiceID, raw, token = strings.TrimSpace(js.InvoiceID), strings.TrimSpace(js.Status), strings.TrimSpace(js.Token)
	} else if values, err := url.ParseQuery(string(body)); err == nil {
		transactionID = strings.TrimSpace(values.Get("TransactionId"))
		invoiceID, raw, token = strings.TrimSpace(values.Get("InvoiceId")), strings.TrimSpace(values.Get("Status")), strings.TrimSpace(values.Get("Token"))
	} else {
		return VerifiedEvent{}, ErrWebhookInvalid
	}
	if invoiceID == "" && transactionID == "" {
		return VerifiedEvent{}, ErrWebhookInvalid
	}
	if raw == "" {
		// Some notification variants do not include a status. Authenticity alone
		// is not payment authority, so ambiguous callbacks are reconciled through
		// /v2/payments/find instead of being promoted to success.
		raw = "UNKNOWN"
	}
	status := cloudStatus(raw)
	external := invoiceID
	if external == "" {
		external = transactionID
	}
	eventID := "cloudpayments:" + external + ":" + transactionID + ":" + strings.ToUpper(raw)
	return VerifiedEvent{Provider: ProviderCloudPayments, ID: eventID, ExternalOperationID: external, RawStatus: raw, Status: status, SavedMethodRef: token}, nil
}

func (p *CloudPayments) post(ctx context.Context, path string, in, out any) error {
	b, e := json.Marshal(in)
	if e != nil {
		return e
	}
	r, e := http.NewRequestWithContext(ctx, http.MethodPost, p.base+path, bytes.NewReader(b))
	if e != nil {
		return e
	}
	r.SetBasicAuth(p.publicID, p.secret)
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

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (p *CloudPayments) CreatePurchase(ctx context.Context, in PurchaseRequest) (PurchaseResult, error) {
	if in.AmountKopecks <= 0 || in.Currency != "RUB" || in.InternalReferenceID == "" {
		return PurchaseResult{}, ErrInvalidAttempt
	}
	if in.SavedMethodRef != "" {
		_, st, err := p.ChargeToken(ctx, in.AmountKopecks, in.InternalReferenceID, in.SavedMethodRef)
		if err != nil {
			return PurchaseResult{}, err
		}
		return purchaseResult(in.InternalReferenceID, st, string(st), "", in.SavedMethodRef)
	}
	// First payment uses CloudPayments-hosted checkout. Card credentials never
	// touch Naimio. With AccountId enabled, the verified Pay notification may
	// carry the provider Token that is safe to persist for merchant-managed
	// recurring charges.
	var out struct {
		Success bool
		Model   struct {
			ID     string `json:"Id"`
			URL    string `json:"Url"`
			Status string `json:"Status"`
		} `json:"Model"`
	}
	body := map[string]any{
		"Amount": rub(in.AmountKopecks), "Currency": "RUB", "Description": in.Description,
		"InvoiceId": in.InternalReferenceID, "AccountId": in.CustomerID,
		"SendEmail": false, "SuccessRedirectUrl": in.ReturnURL, "FailRedirectUrl": in.FailURL,
	}
	if err := p.post(ctx, "/orders/create", body, &out); err != nil {
		return PurchaseResult{}, err
	}
	if !out.Success || out.Model.URL == "" {
		return PurchaseResult{}, fmt.Errorf("provider declined")
	}
	return purchaseResult(in.InternalReferenceID, StatusPendingUserAction, out.Model.Status, out.Model.URL, "")
}
func (p *CloudPayments) ChargeRecurring(ctx context.Context, in PurchaseRequest) (PurchaseResult, error) {
	return p.CreatePurchase(ctx, in)
}
func (p *CloudPayments) RefundPurchase(ctx context.Context, id, _ string, amount int64) (string, error) {
	tx, _, err := p.transactionByInvoice(ctx, id)
	if err != nil {
		return "", err
	}
	return p.Refund(ctx, fmt.Sprint(tx), amount)
}

// CloudEscrowOperation is the provider-safe representation of a CloudPayments
// Safe Deal/escrow operation. Naimio keeps all monetary amounts in kopecks;
// AccumulationID and transaction IDs are opaque provider references only.
type CloudEscrowOperation struct {
	TransactionID  string
	AccumulationID string
	Status         Status
	RawStatus      string
}

// ChargeEscrowToken starts or adds to a CloudPayments accumulation using an
// already tokenized customer payment method. It never accepts card details.
// A first payment sets start=true and leaves accumulationID empty; subsequent
// payments reuse the returned accumulation id.
func (p *CloudPayments) ChargeEscrowToken(ctx context.Context, amount int64, invoiceID, accountID, token, accumulationID, escrowType string, start bool) (CloudEscrowOperation, error) {
	if amount <= 0 || strings.TrimSpace(invoiceID) == "" || strings.TrimSpace(accountID) == "" || strings.TrimSpace(token) == "" {
		return CloudEscrowOperation{}, ErrInvalidAttempt
	}
	escrowType = strings.TrimSpace(escrowType)
	if escrowType != "NToOne" && escrowType != "OneToN" && escrowType != "NToN" {
		return CloudEscrowOperation{}, ErrInvalidAttempt
	}
	escrow := map[string]any{"StartAccumulation": start, "EscrowType": escrowType}
	if accumulationID != "" {
		escrow["AccumulationId"] = accumulationID
	}
	var out struct {
		Success bool
		Model   struct {
			TransactionID        int64  `json:"TransactionId"`
			Status               string `json:"Status"`
			EscrowAccumulationID string `json:"EscrowAccumulationId"`
		}
		Message string
	}
	if err := p.post(ctx, "/payments/tokens/charge", map[string]any{
		"Amount": rub(amount), "Currency": "RUB", "InvoiceId": invoiceID,
		"AccountId": accountID, "Token": token, "Escrow": escrow,
	}, &out); err != nil {
		return CloudEscrowOperation{}, err
	}
	if !out.Success || out.Model.TransactionID == 0 {
		return CloudEscrowOperation{}, fmt.Errorf("provider declined")
	}
	acc := strings.TrimSpace(out.Model.EscrowAccumulationID)
	if acc == "" {
		acc = strings.TrimSpace(accumulationID)
	}
	if acc == "" {
		// A missing accumulation id on the first operation cannot safely be used
		// for later release/payout and therefore requires reconciliation rather
		// than pretending escrow is established.
		return CloudEscrowOperation{TransactionID: fmt.Sprint(out.Model.TransactionID), Status: StatusUnknownReconciliation, RawStatus: out.Model.Status}, nil
	}
	status := cloudStatus(out.Model.Status)
	if out.Model.Status == "" && out.Success {
		status = StatusSucceeded
	}
	return CloudEscrowOperation{TransactionID: fmt.Sprint(out.Model.TransactionID), AccumulationID: acc, Status: status, RawStatus: out.Model.Status}, nil
}

// PayoutEscrowToken releases part or all of a OneToN accumulation to a
// tokenized recipient. CloudPayments requires the originating funding
// transaction ids in the Escrow object. final=true closes the deal according
// to the provider's documented FinalPayout semantics.
func (p *CloudPayments) PayoutEscrowToken(ctx context.Context, amount int64, invoiceID, accountID, recipientToken, accumulationID, escrowType string, fundingTransactionIDs []int64, final bool) (CloudEscrowOperation, error) {
	if amount <= 0 || strings.TrimSpace(invoiceID) == "" || strings.TrimSpace(accountID) == "" || strings.TrimSpace(recipientToken) == "" || strings.TrimSpace(accumulationID) == "" || len(fundingTransactionIDs) == 0 {
		return CloudEscrowOperation{}, ErrInvalidAttempt
	}
	if escrowType != "NToOne" && escrowType != "OneToN" && escrowType != "NToN" {
		return CloudEscrowOperation{}, ErrInvalidAttempt
	}
	var out struct {
		Success bool
		Model   struct {
			TransactionID int64  `json:"TransactionId"`
			Status        string `json:"Status"`
		}
		Message string
	}
	body := map[string]any{
		"Token": recipientToken, "Amount": rub(amount), "AccountId": accountID,
		"Currency": "RUB", "InvoiceId": invoiceID,
		"Escrow": map[string]any{"AccumulationId": accumulationID, "TransactionIds": fundingTransactionIDs, "EscrowType": escrowType, "FinalPayout": final},
	}
	if err := p.post(ctx, "/payments/token/topup", body, &out); err != nil {
		return CloudEscrowOperation{}, err
	}
	if !out.Success || out.Model.TransactionID == 0 {
		return CloudEscrowOperation{}, fmt.Errorf("provider declined")
	}
	status := cloudStatus(out.Model.Status)
	if out.Model.Status == "" && out.Success {
		status = StatusProcessing
	}
	return CloudEscrowOperation{TransactionID: fmt.Sprint(out.Model.TransactionID), AccumulationID: accumulationID, Status: status, RawStatus: out.Model.Status}, nil
}

type CloudEscrowInfo struct {
	AccumulationID string
	Status         string
	BalanceKopecks int64
}

// GetEscrowInfo authoritatively reads the provider accumulation. The balance
// parser deliberately rejects sub-kopeck values instead of rounding money.
func (p *CloudPayments) GetEscrowInfo(ctx context.Context, accumulationID string) (CloudEscrowInfo, error) {
	if strings.TrimSpace(accumulationID) == "" {
		return CloudEscrowInfo{}, ErrInvalidAttempt
	}
	var out struct {
		Success bool
		Model   []struct {
			Status               string      `json:"Status"`
			EscrowAccumulationID string      `json:"EscrowAccumulationId"`
			Balance              json.Number `json:"Balance"`
		}
		Message string
	}
	if err := p.post(ctx, "/Escrow/GetEscrowInfo", map[string]any{"EscrowAccumulationIds": []string{accumulationID}}, &out); err != nil {
		return CloudEscrowInfo{}, err
	}
	if !out.Success || len(out.Model) != 1 {
		return CloudEscrowInfo{}, fmt.Errorf("provider escrow not found")
	}
	balance, err := decimalKopecks(out.Model[0].Balance.String())
	if err != nil {
		return CloudEscrowInfo{}, err
	}
	return CloudEscrowInfo{AccumulationID: out.Model[0].EscrowAccumulationID, Status: out.Model[0].Status, BalanceKopecks: balance}, nil
}
