package payments

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Robokassa struct {
	Login, Password1, Password2, Password3, BaseURL, ServicesBaseURL string
	Test                                                             bool
	Client                                                           *http.Client
}

func (p Robokassa) httpClient() *http.Client {
	if p.Client != nil {
		return p.Client
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (p Robokassa) authBaseURL() string {
	if p.BaseURL != "" {
		if u, err := url.Parse(p.BaseURL); err == nil && u.Scheme != "" && u.Host != "" {
			return u.Scheme + "://" + u.Host
		}
	}
	return "https://auth.robokassa.ru"
}

func (p Robokassa) servicesBaseURL() string {
	if strings.TrimSpace(p.ServicesBaseURL) != "" {
		return strings.TrimRight(p.ServicesBaseURL, "/")
	}
	return "https://services.robokassa.ru"
}

func (p Robokassa) PaymentURL(amount int64, invoiceID int64, description, returnURL, failURL string) (string, error) {
	if amount <= 0 || p.Login == "" || p.Password1 == "" {
		return "", ErrInvalidAttempt
	}
	if p.BaseURL == "" {
		p.BaseURL = "https://auth.robokassa.ru/Merchant/Index.aspx"
	}
	sum := rub(amount)
	sig := md5hex(p.Login + ":" + sum + ":" + strconv.FormatInt(invoiceID, 10) + ":" + p.Password1)
	q := url.Values{"MerchantLogin": {p.Login}, "OutSum": {sum}, "InvId": {strconv.FormatInt(invoiceID, 10)}, "Description": {description}, "SignatureValue": {sig}, "SuccessURL": {returnURL}, "FailURL": {failURL}}
	if p.Test {
		q.Set("IsTest", "1")
	}
	return p.BaseURL + "?" + q.Encode(), nil
}
func (p Robokassa) VerifyResult(amount string, invoiceID int64, signature string) bool {
	return strings.EqualFold(md5hex(amount+":"+strconv.FormatInt(invoiceID, 10)+":"+p.Password2), signature)
}

// VerifyWebhook accepts the application/x-www-form-urlencoded ResultURL
// callback. Robokassa authenticates it with Password2; no browser return is
// treated as payment authority.
func (p Robokassa) VerifyWebhook(_ context.Context, body []byte, _ map[string][]string) (VerifiedEvent, error) {
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return VerifiedEvent{}, ErrWebhookInvalid
	}
	amount, invoice, sig := values.Get("OutSum"), values.Get("InvId"), values.Get("SignatureValue")
	id, err := strconv.ParseInt(invoice, 10, 64)
	if err != nil || amount == "" || !p.VerifyResult(amount, id, sig) {
		return VerifiedEvent{}, ErrWebhookInvalid
	}
	return VerifiedEvent{Provider: ProviderRobokassa, ID: "robokassa:" + invoice + ":" + strings.ToLower(sig), ExternalOperationID: invoice, Type: "PAYMENT_STATUS", RawStatus: "SUCCESS", Status: StatusSucceeded}, nil
}
func md5hex(v string) string { return fmt.Sprintf("%x", md5.Sum([]byte(v))) }

func (p Robokassa) CreatePurchase(_ context.Context, in PurchaseRequest) (PurchaseResult, error) {
	clean := strings.ReplaceAll(in.InternalReferenceID, "-", "")
	var invoiceID int64
	var err error
	if len(clean) >= 12 {
		invoiceID, err = strconv.ParseInt(clean[:12], 16, 64)
	} else {
		err = ErrInvalidAttempt
	}
	if err != nil {
		// Stable enough only as a final fallback for non-UUID internal references;
		// the payment-attempt idempotency layer still prevents duplicate attempts.
		invoiceID = int64(len(in.IdempotencyKey))*100000 + int64(len(in.InternalReferenceID))
	}
	u, err := p.PaymentURL(in.AmountKopecks, invoiceID, in.Description, in.ReturnURL, in.FailURL)
	if err != nil {
		return PurchaseResult{}, err
	}
	if in.SavePaymentMethod || in.BillingPeriod != "" {
		parsed, _ := url.Parse(u)
		q := parsed.Query()
		q.Set("Recurring", "true")
		parsed.RawQuery = q.Encode()
		u = parsed.String()
	}
	return purchaseResult(strconv.FormatInt(invoiceID, 10), StatusPendingUserAction, "PENDING", u, strconv.FormatInt(invoiceID, 10))
}

func (p Robokassa) ChargeRecurring(ctx context.Context, in PurchaseRequest) (PurchaseResult, error) {
	if in.SavedMethodRef == "" || in.AmountKopecks <= 0 || p.Login == "" || p.Password1 == "" {
		return PurchaseResult{}, ErrInvalidAttempt
	}
	previous, err := strconv.ParseInt(in.SavedMethodRef, 10, 64)
	if err != nil {
		return PurchaseResult{}, ErrInvalidAttempt
	}
	// Derive a deterministic positive invoice id from the idempotency key so a
	// retry cannot create a different child invoice. The payment-attempt layer
	// additionally guarantees that the provider call happens only once unless
	// an authoritative failure creates a new retry generation.
	sumHash := sha256.Sum256([]byte(in.IdempotencyKey))
	invoice := int64(sumHash[0])<<40 | int64(sumHash[1])<<32 | int64(sumHash[2])<<24 | int64(sumHash[3])<<16 | int64(sumHash[4])<<8 | int64(sumHash[5])
	if invoice == 0 || invoice == previous {
		invoice = previous + 1
	}
	sum := rub(in.AmountKopecks)
	sig := md5hex(p.Login + ":" + sum + ":" + strconv.FormatInt(invoice, 10) + ":" + p.Password1)
	form := url.Values{
		"MerchantLogin":     {p.Login},
		"OutSum":            {sum},
		"InvoiceID":         {strconv.FormatInt(invoice, 10)},
		"PreviousInvoiceID": {strconv.FormatInt(previous, 10)},
		"Description":       {in.Description},
		"SignatureValue":    {sig},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.authBaseURL()+"/Merchant/Recurring", strings.NewReader(form.Encode()))
	if err != nil {
		return PurchaseResult{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := p.httpClient().Do(req)
	if err != nil {
		return PurchaseResult{}, err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return PurchaseResult{}, fmt.Errorf("provider status %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 4096))
	if err != nil {
		return PurchaseResult{}, err
	}
	// Robokassa documents OK+InvoiceId as acknowledgement that the recurring
	// operation was created, not that funds were charged. ResultURL/OpStateExt
	// remains authoritative, so the normalized state stays PROCESSING.
	want := "OK" + strconv.FormatInt(invoice, 10)
	if strings.TrimSpace(string(body)) != want {
		return PurchaseResult{}, fmt.Errorf("robokassa recurring acknowledgement rejected")
	}
	return purchaseResult(strconv.FormatInt(invoice, 10), StatusProcessing, "CREATED", "", in.SavedMethodRef)
}

type robokassaOperationState struct {
	XMLName xml.Name `xml:"OperationStateResponse"`
	Result  struct {
		Code        int    `xml:"Code"`
		Description string `xml:"Description"`
	} `xml:"Result"`
	State struct {
		Code int `xml:"Code"`
	} `xml:"State"`
	Info struct {
		OutSum string `xml:"OutSum"`
		OpKey  string `xml:"OpKey"`
	} `xml:"Info"`
}

func (p Robokassa) operationState(ctx context.Context, invoiceID string) (robokassaOperationState, error) {
	if p.Test {
		return robokassaOperationState{}, ErrProviderUnavailable // OpStateExt is production-only.
	}
	if p.Login == "" || p.Password2 == "" || strings.TrimSpace(invoiceID) == "" {
		return robokassaOperationState{}, ErrInvalidAttempt
	}
	sig := md5hex(p.Login + ":" + invoiceID + ":" + p.Password2)
	q := url.Values{"MerchantLogin": {p.Login}, "InvoiceID": {invoiceID}, "Signature": {sig}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.authBaseURL()+"/Merchant/WebService/Service.asmx/OpStateExt?"+q.Encode(), nil)
	if err != nil {
		return robokassaOperationState{}, err
	}
	res, err := p.httpClient().Do(req)
	if err != nil {
		return robokassaOperationState{}, err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return robokassaOperationState{}, fmt.Errorf("provider status %d", res.StatusCode)
	}
	var out robokassaOperationState
	if err := xml.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out); err != nil {
		return robokassaOperationState{}, err
	}
	if out.Result.Code != 0 {
		return out, fmt.Errorf("robokassa operation state result %d", out.Result.Code)
	}
	return out, nil
}

func (p Robokassa) GetStatus(ctx context.Context, id string) (Status, string, error) {
	if p.Test {
		// Official OpStateExt does not expose test payments. ResultURL remains the
		// authoritative test signal, while reconciliation keeps the attempt open.
		return StatusUnknownReconciliation, "TEST_STATUS_REQUIRES_CALLBACK", nil
	}
	out, err := p.operationState(ctx, id)
	if err != nil {
		return StatusUnknownReconciliation, "", err
	}
	raw := strconv.Itoa(out.State.Code)
	switch out.State.Code {
	case 5:
		return StatusPendingUserAction, raw, nil
	case 10:
		return StatusCanceled, raw, nil
	case 20:
		return StatusAuthorized, raw, nil
	case 50, 80:
		return StatusProcessing, raw, nil
	case 60:
		return StatusFailed, raw, nil
	case 100:
		return StatusSucceeded, raw, nil
	default:
		return StatusUnknownReconciliation, raw, nil
	}
}

func (p Robokassa) RefundPurchase(ctx context.Context, id, _ string, amount int64) (string, error) {
	if p.Password3 == "" {
		return "", ErrUnsupportedRoute
	}
	if p.Test {
		return "", ErrUnsupportedRoute
	}
	if amount <= 0 {
		return "", ErrInvalidAttempt
	}
	state, err := p.operationState(ctx, id)
	if err != nil {
		return "", err
	}
	if state.State.Code != 100 || state.Info.OpKey == "" {
		return "", ErrInvalidTransition
	}
	payload := map[string]any{"OpKey": state.Info.OpKey}
	if paid, parseErr := decimalKopecks(state.Info.OutSum); parseErr != nil || paid != amount {
		payload["RefundSum"] = decimalRubNumber(amount)
	}
	token, err := robokassaJWT(payload, p.Password3)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.servicesBaseURL()+"/RefundService/Refund/Create", bytes.NewBufferString(token))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/jwt")
	res, err := p.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return "", fmt.Errorf("provider status %d", res.StatusCode)
	}
	var out struct {
		Success   bool   `json:"success"`
		Message   string `json:"message"`
		RequestID string `json:"requestId"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out); err != nil {
		return "", err
	}
	if !out.Success || out.RequestID == "" {
		return "", fmt.Errorf("robokassa refund rejected")
	}
	return out.RequestID, nil
}

func (p Robokassa) GetOperationStatus(ctx context.Context, operation OperationType, id string) (Status, string, error) {
	if operation != OperationRefund {
		return p.GetStatus(ctx, id)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.servicesBaseURL()+"/RefundService/Refund/GetState?id="+url.QueryEscape(id), nil)
	if err != nil {
		return StatusUnknownReconciliation, "", err
	}
	res, err := p.httpClient().Do(req)
	if err != nil {
		return StatusUnknownReconciliation, "", err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return StatusUnknownReconciliation, "", fmt.Errorf("provider status %d", res.StatusCode)
	}
	var out struct {
		Label   string `json:"label"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out); err != nil {
		return StatusUnknownReconciliation, "", err
	}
	raw := strings.ToLower(strings.TrimSpace(out.Label))
	switch raw {
	case "finished", "success", "succeeded":
		return StatusRefunded, raw, nil
	case "failed", "error", "rejected", "cancelled", "canceled":
		return StatusFailed, raw, nil
	case "created", "pending", "processing", "in_progress":
		return StatusProcessing, raw, nil
	default:
		return StatusUnknownReconciliation, raw, nil
	}
}

func robokassaJWT(payload any, secret string) (string, error) {
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding
	unsigned := enc.EncodeToString(header) + "." + enc.EncodeToString(body)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(unsigned))
	return unsigned + "." + enc.EncodeToString(mac.Sum(nil)), nil
}

func decimalKopecks(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("empty amount")
	}
	parts := strings.SplitN(value, ".", 2)
	rubles, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}
	kopecks := int64(0)
	if len(parts) == 2 {
		fraction := parts[1]
		if len(fraction) > 2 {
			fraction = fraction[:2]
		}
		for len(fraction) < 2 {
			fraction += "0"
		}
		kopecks, err = strconv.ParseInt(fraction, 10, 64)
		if err != nil {
			return 0, err
		}
	}
	return rubles*100 + kopecks, nil
}
