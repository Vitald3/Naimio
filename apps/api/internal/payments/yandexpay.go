package payments

// Yandex Pay Merchant API webhook verification. Notifications are compact ES256
// JWTs sent as application/octet-stream. A JWK set is intentionally fetched
// from the configured Yandex endpoint rather than copied into settings: public
// key rotation then needs no database secret or deployment change.

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type YandexPayConfig struct {
	JWKSURL, APIKey, MerchantID, BaseURL string
	Client                               *http.Client
	Now                                  func() time.Time
}
type YandexPay struct {
	jwksURL, apiKey, merchantID, baseURL string
	client                               *http.Client
	now                                  func() time.Time
}

func NewYandexPay(c YandexPayConfig) (*YandexPay, error) {
	if strings.TrimSpace(c.JWKSURL) == "" {
		return nil, ErrProviderUnavailable
	}
	if c.BaseURL == "" {
		c.BaseURL = "https://pay.yandex.ru/api/merchant"
	}
	if c.Client == nil {
		c.Client = &http.Client{Timeout: 10 * time.Second}
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	return &YandexPay{jwksURL: c.JWKSURL, apiKey: c.APIKey, merchantID: c.MerchantID, baseURL: strings.TrimRight(c.BaseURL, "/"), client: c.Client, now: c.Now}, nil
}
func (p *YandexPay) VerifyWebhook(ctx context.Context, body []byte, _ map[string][]string) (VerifiedEvent, error) {
	parts := strings.Split(string(body), ".")
	if len(parts) != 3 || len(body) > 256<<10 {
		return VerifiedEvent{}, ErrWebhookInvalid
	}
	decode := func(v string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(v) }
	h, err := decode(parts[0])
	if err != nil {
		return VerifiedEvent{}, ErrWebhookInvalid
	}
	var header struct{ Alg, Kid string }
	if json.Unmarshal(h, &header) != nil || header.Alg != "ES256" || header.Kid == "" {
		return VerifiedEvent{}, ErrWebhookInvalid
	}
	key, err := p.key(ctx, header.Kid)
	if err != nil {
		return VerifiedEvent{}, ErrWebhookInvalid
	}
	sig, err := decode(parts[2])
	if err != nil || len(sig) != 64 {
		return VerifiedEvent{}, ErrWebhookInvalid
	}
	if !ecdsa.Verify(key, []byte(parts[0]+"."+parts[1]), new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		return VerifiedEvent{}, ErrWebhookInvalid
	}
	payload, err := decode(parts[1])
	if err != nil {
		return VerifiedEvent{}, ErrWebhookInvalid
	}
	var in struct {
		Event     string    `json:"event"`
		EventTime time.Time `json:"eventTime"`
		Exp       int64     `json:"exp"`
		Order     struct {
			ID     string `json:"orderId"`
			Status string `json:"paymentStatus"`
		} `json:"order"`
		Operation struct {
			ID      string `json:"operationId"`
			OrderID string `json:"orderId"`
			Status  string `json:"status"`
			Type    string `json:"operationType"`
		} `json:"operation"`
		Subscription struct {
			ID     string `json:"customerSubscriptionId"`
			Status string `json:"status"`
		} `json:"subscription"`
	}
	if json.Unmarshal(payload, &in) != nil || in.Exp > 0 && !p.now().UTC().Before(time.Unix(in.Exp, 0).UTC()) {
		return VerifiedEvent{}, ErrWebhookInvalid
	}
	id, raw, eventID := in.Order.ID, in.Order.Status, ""
	if in.Event == "OPERATION_STATUS_UPDATED" {
		id, raw, eventID = in.Operation.OrderID, in.Operation.Status, in.Operation.ID
	} else if in.Event == "SUBSCRIPTION_STATUS_UPDATED" {
		// Subscription lifecycle notifications are acknowledged and persisted,
		// but they are not payment authority. The payment/order webhook or an
		// authoritative order status lookup drives the internal subscription.
		id, raw, eventID = in.Subscription.ID, in.Subscription.Status, "subscription:"+in.Subscription.ID+":"+strings.ToUpper(in.Subscription.Status)
	}
	if id == "" || raw == "" {
		return VerifiedEvent{}, ErrWebhookInvalid
	}
	statuses := map[string]Status{"CAPTURED": StatusSucceeded, "AUTHORIZED": StatusAuthorized, "FAILED": StatusFailed, "CANCELLED": StatusCanceled, "CANCELED": StatusCanceled, "REFUNDED": StatusRefunded, "SUCCESS": StatusSucceeded, "FAIL": StatusFailed}
	status, ok := statuses[strings.ToUpper(raw)]
	if !ok {
		status = StatusUnknownReconciliation
	}
	if eventID == "" {
		eventID = in.Event + ":" + id + ":" + strings.ToUpper(raw)
	}
	return VerifiedEvent{Provider: ProviderYandexPay, ID: eventID, ExternalOperationID: id, Type: in.Event, RawStatus: raw, Status: status, OccurredAt: in.EventTime.UTC()}, nil
}
func (p *YandexPay) key(ctx context.Context, kid string) (*ecdsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.jwksURL, nil)
	if err != nil {
		return nil, err
	}
	res, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return nil, ErrWebhookInvalid
	}
	var doc struct {
		Keys []struct{ KID, KTY, CRV, X, Y string } `json:"keys"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&doc) != nil {
		return nil, ErrWebhookInvalid
	}
	for _, v := range doc.Keys {
		if v.KID == kid && v.KTY == "EC" && v.CRV == "P-256" {
			x, ex := base64.RawURLEncoding.DecodeString(v.X)
			y, ey := base64.RawURLEncoding.DecodeString(v.Y)
			if ex != nil || ey != nil {
				return nil, ErrWebhookInvalid
			}
			k := &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
			if !k.Curve.IsOnCurve(k.X, k.Y) {
				return nil, ErrWebhookInvalid
			}
			return k, nil
		}
	}
	return nil, ErrWebhookInvalid
}

func (p *YandexPay) CreatePurchase(ctx context.Context, in PurchaseRequest) (PurchaseResult, error) {
	if p.apiKey == "" || p.merchantID == "" {
		return PurchaseResult{}, ErrProviderUnavailable
	}
	if in.AmountKopecks <= 0 || in.Currency != "RUB" || in.InternalReferenceID == "" {
		return PurchaseResult{}, ErrInvalidAttempt
	}
	cart := map[string]any{
		"externalId": in.InternalReferenceID,
		"total":      map[string]string{"amount": rub(in.AmountKopecks)},
		"items": []map[string]any{{
			"productId": "naimio-pro", "title": in.Description,
			"quantity":  map[string]string{"count": "1"},
			"unitPrice": rub(in.AmountKopecks), "subtotal": rub(in.AmountKopecks), "total": rub(in.AmountKopecks),
		}},
	}
	redirects := map[string]string{"onSuccess": in.ReturnURL, "onError": in.FailURL, "onAbort": in.FailURL}
	if in.SavePaymentMethod {
		unit := "MONTH"
		if strings.EqualFold(in.BillingPeriod, "YEAR") {
			unit = "YEAR"
		}
		body := map[string]any{
			"orderId":              in.InternalReferenceID,
			"currencyCode":         "RUB",
			"cart":                 cart,
			"futureWriteOffAmount": rub(in.AmountKopecks),
			"intervalUnit":         unit,
			"intervalCount":        1,
			"title":                yandexTitle(in.Description),
			"purpose":              in.Description,
			"redirectUrls":         redirects,
			"metadata":             in.InternalReferenceID,
			"orderSource":          "WEBSITE",
		}
		var out struct {
			Status string `json:"status"`
			Data   struct {
				PaymentURL     string `json:"paymentUrl"`
				SubscriptionID string `json:"subscriptionId"`
			} `json:"data"`
		}
		if err := p.api(ctx, http.MethodPost, "/v1/subscriptions", in.IdempotencyKey, body, &out); err != nil {
			return PurchaseResult{}, err
		}
		if out.Data.PaymentURL == "" || out.Data.SubscriptionID == "" {
			return PurchaseResult{}, fmt.Errorf("yandex pay returned incomplete subscription checkout")
		}
		return purchaseResult(in.InternalReferenceID, StatusPendingUserAction, "PENDING", out.Data.PaymentURL, encodeYandexSubscriptionRef(out.Data.SubscriptionID, in.InternalReferenceID))
	}
	body := map[string]any{
		"orderId":                 in.InternalReferenceID,
		"currencyCode":            "RUB",
		"availablePaymentMethods": []string{"CARD", "SBP"},
		"cart":                    cart,
		"redirectUrls":            redirects,
		"metadata":                in.InternalReferenceID,
	}
	var out struct {
		Status string `json:"status"`
		Data   struct {
			PaymentURL string `json:"paymentUrl"`
		} `json:"data"`
	}
	if err := p.api(ctx, http.MethodPost, "/v1/orders", in.IdempotencyKey, body, &out); err != nil {
		return PurchaseResult{}, err
	}
	return purchaseResult(in.InternalReferenceID, StatusPendingUserAction, "PENDING", out.Data.PaymentURL, "")
}

func (p *YandexPay) GetStatus(ctx context.Context, id string) (Status, string, error) {
	var out struct {
		Data struct {
			Order struct {
				PaymentStatus string `json:"paymentStatus"`
			} `json:"order"`
		} `json:"data"`
	}
	if err := p.api(ctx, http.MethodGet, "/v1/orders/"+url.PathEscape(id), id, nil, &out); err != nil {
		return StatusUnknownReconciliation, "", err
	}
	raw := strings.ToUpper(out.Data.Order.PaymentStatus)
	m := map[string]Status{"PENDING": StatusProcessing, "AUTHORIZED": StatusAuthorized, "CAPTURED": StatusSucceeded, "FAILED": StatusFailed, "CANCELLED": StatusCanceled, "CANCELED": StatusCanceled, "REFUNDED": StatusRefunded, "PARTIALLY_REFUNDED": StatusPartiallyRefunded}
	st, ok := m[raw]
	if !ok {
		st = StatusUnknownReconciliation
	}
	return st, raw, nil
}
func (p *YandexPay) RefundPurchase(ctx context.Context, id, key string, amount int64) (string, error) {
	if amount <= 0 {
		return "", ErrInvalidAttempt
	}
	var out struct {
		Data struct {
			Operation struct {
				OperationID string `json:"operationId"`
			} `json:"operation"`
		} `json:"data"`
	}
	if err := p.api(ctx, http.MethodPost, "/v2/orders/"+url.PathEscape(id)+"/refund", key, map[string]any{"refundAmount": rub(amount)}, &out); err != nil {
		return "", err
	}
	return out.Data.Operation.OperationID, nil
}
func (p *YandexPay) ChargeRecurring(ctx context.Context, in PurchaseRequest) (PurchaseResult, error) {
	if in.SavedMethodRef == "" || in.AmountKopecks <= 0 || in.Currency != "RUB" {
		return PurchaseResult{}, ErrInvalidAttempt
	}
	_, parentOrderID, ok := decodeYandexSubscriptionRef(in.SavedMethodRef)
	if !ok {
		return PurchaseResult{}, ErrInvalidAttempt
	}
	orderID := yandexRenewalOrderID(in.IdempotencyKey)
	cart := map[string]any{
		"externalId": orderID,
		"total":      map[string]string{"amount": rub(in.AmountKopecks)},
		"items": []map[string]any{{
			"productId": "naimio-pro", "title": in.Description,
			"quantity":  map[string]string{"count": "1"},
			"unitPrice": rub(in.AmountKopecks), "subtotal": rub(in.AmountKopecks), "total": rub(in.AmountKopecks),
		}},
	}
	body := map[string]any{
		"orderId":       orderID,
		"parentOrderId": parentOrderID,
		"amount":        rub(in.AmountKopecks),
		"currencyCode":  "RUB",
		"cart":          cart,
		"metadata":      in.InternalReferenceID,
		"purpose":       in.Description,
	}
	var out struct {
		Data struct {
			OperationID string `json:"operationId"`
		} `json:"data"`
	}
	if err := p.api(ctx, http.MethodPost, "/v1/subscriptions/recur", in.IdempotencyKey, body, &out); err != nil {
		return PurchaseResult{}, err
	}
	if out.Data.OperationID == "" {
		return PurchaseResult{}, fmt.Errorf("yandex pay returned empty recurring operation")
	}
	// Persist the merchant order id as the provider reference: order status is
	// queryable and reconciliation-safe, while operationId is provider-internal.
	return purchaseResult(orderID, StatusProcessing, "PENDING", "", in.SavedMethodRef)
}

func encodeYandexSubscriptionRef(subscriptionID, parentOrderID string) string {
	return subscriptionID + "|" + parentOrderID
}

func decodeYandexSubscriptionRef(value string) (subscriptionID, parentOrderID string, ok bool) {
	parts := strings.SplitN(value, "|", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func yandexRenewalOrderID(key string) string {
	sum := sha256.Sum256([]byte(key))
	return "naimio-renew-" + hex.EncodeToString(sum[:12])
}

func yandexTitle(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "Naimio PRO"
	}
	runes := []rune(value)
	if len(runes) > 30 {
		runes = runes[:30]
	}
	return string(runes)
}

func (p *YandexPay) api(ctx context.Context, method, path, key string, in, out any) error {
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
	r.Header.Set("Authorization", "Api-Key "+p.apiKey)
	r.Header.Set("X-Request-Id", key)
	r.Header.Set("X-Request-Timeout", "15000")
	r.Header.Set("X-Request-Attempt", "0")
	if in != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	res, e := p.client.Do(r)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("provider status %d", res.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(out)
}
