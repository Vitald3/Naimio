package payments

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCloudPaymentsSerializesKopecksAsDecimal(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/payments/tokens/charge" {
			t.Fatal(r.URL.Path)
		}
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["Amount"] != "19.99" {
			t.Fatalf("amount=%#v", in["Amount"])
		}
		_, _ = w.Write([]byte(`{"Success":true,"Model":{"TransactionId":42,"Status":"Completed"}}`))
	}))
	defer s.Close()
	p, err := NewCloudPayments(CloudPaymentsConfig{PublicID: "public", APISecret: "secret", BaseURL: s.URL})
	if err != nil {
		t.Fatal(err)
	}
	id, status, err := p.ChargeToken(context.Background(), 1999, "order", "opaque-token")
	if err != nil || id != "42" || status != StatusSucceeded {
		t.Fatalf("%q %s %v", id, status, err)
	}
}

func TestTBankWebhookRejectsTampering(t *testing.T) {
	p, err := NewTBank(TBankConfig{TerminalKey: "term", Password: "password"})
	if err != nil {
		t.Fatal(err)
	}
	v := map[string]any{"TerminalKey": "term", "PaymentId": "42", "OrderId": "order", "Status": "CONFIRMED"}
	v["Token"] = p.token(v)
	body, _ := json.Marshal(v)
	e, err := p.VerifyWebhook(context.Background(), body, nil)
	if err != nil || e.Status != StatusSucceeded {
		t.Fatalf("%+v %v", e, err)
	}
	v["Status"] = "REJECTED"
	body, _ = json.Marshal(v)
	if _, err = p.VerifyWebhook(context.Background(), body, nil); err == nil {
		t.Fatal("tampered callback accepted")
	}
}

func TestRobokassaResultURLVerification(t *testing.T) {
	p := Robokassa{Password2: "two"}
	sig := md5hex("19.99:42:two")
	e, err := p.VerifyWebhook(context.Background(), []byte("OutSum=19.99&InvId=42&SignatureValue="+sig), nil)
	if err != nil || e.ExternalOperationID != "42" || e.Status != StatusSucceeded {
		t.Fatalf("%+v %v", e, err)
	}
}

func TestYandexPayES256Webhook(t *testing.T) {
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	b64 := base64.RawURLEncoding.EncodeToString
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{"kid": "key-1", "kty": "EC", "crv": "P-256", "x": b64(private.X.Bytes()), "y": b64(private.Y.Bytes())}}})
	}))
	defer s.Close()
	p, err := NewYandexPay(YandexPayConfig{JWKSURL: s.URL, Now: func() time.Time { return fixedTime() }})
	if err != nil {
		t.Fatal(err)
	}
	header := b64([]byte(`{"alg":"ES256","kid":"key-1"}`))
	payload := b64([]byte(`{"exp":1790000000,"event":"ORDER_STATUS_UPDATED","order":{"orderId":"order-1","paymentStatus":"CAPTURED"}}`))
	hash := []byte(header + "." + payload)
	r, sigS, err := ecdsa.Sign(rand.Reader, private, hash)
	if err != nil {
		t.Fatal(err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	sigS.FillBytes(sig[32:])
	encoded := header + "." + payload + "." + b64(sig)
	e, err := p.VerifyWebhook(context.Background(), []byte(encoded), nil)
	if err != nil || e.ExternalOperationID != "order-1" || e.Status != StatusSucceeded {
		t.Fatalf("%+v %v", e, err)
	}
	broken := append([]byte(nil), sig...)
	broken[0] ^= 1
	if _, err = p.VerifyWebhook(context.Background(), []byte(header+"."+payload+"."+b64(broken)), nil); err == nil {
		t.Fatal("invalid signature accepted")
	}
}

func TestRobokassaProductionStatusAndRefundContract(t *testing.T) {
	var sawRefund bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Merchant/WebService/Service.asmx/OpStateExt":
			if r.URL.Query().Get("MerchantLogin") != "shop" || r.URL.Query().Get("InvoiceID") != "42" {
				t.Fatalf("bad status query: %s", r.URL.RawQuery)
			}
			want := md5hex("shop:42:two")
			if r.URL.Query().Get("Signature") != want {
				t.Fatalf("bad status signature")
			}
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<OperationStateResponse><Result><Code>0</Code></Result><State><Code>100</Code></State><Info><OutSum>19.99</OutSum><OpKey>operation-key</OpKey></Info></OperationStateResponse>`))
		case "/RefundService/Refund/Create":
			sawRefund = true
			body, _ := io.ReadAll(r.Body)
			parts := strings.Split(string(body), ".")
			if len(parts) != 3 {
				t.Fatalf("refund is not jwt: %q", string(body))
			}
			unsigned := parts[0] + "." + parts[1]
			mac := hmac.New(sha256.New, []byte("three"))
			_, _ = mac.Write([]byte(unsigned))
			if parts[2] != base64.RawURLEncoding.EncodeToString(mac.Sum(nil)) {
				t.Fatal("bad refund jwt signature")
			}
			payload, err := base64.RawURLEncoding.DecodeString(parts[1])
			if err != nil {
				t.Fatal(err)
			}
			var in map[string]any
			if err := json.Unmarshal(payload, &in); err != nil {
				t.Fatal(err)
			}
			if in["OpKey"] != "operation-key" {
				t.Fatalf("bad op key: %#v", in)
			}
			// Full refund intentionally omits RefundSum.
			if _, ok := in["RefundSum"]; ok {
				t.Fatalf("unexpected partial amount: %#v", in)
			}
			_, _ = w.Write([]byte(`{"success":true,"requestId":"refund-1"}`))
		case "/RefundService/Refund/GetState":
			if r.URL.Query().Get("id") != "refund-1" {
				t.Fatalf("bad refund id")
			}
			_, _ = w.Write([]byte(`{"requestId":"refund-1","amount":19.99,"label":"finished"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer s.Close()
	p := Robokassa{Login: "shop", Password1: "one", Password2: "two", Password3: "three", BaseURL: s.URL + "/Merchant/Index.aspx", ServicesBaseURL: s.URL, Client: s.Client()}
	status, raw, err := p.GetStatus(context.Background(), "42")
	if err != nil || status != StatusSucceeded || raw != "100" {
		t.Fatalf("status=%s raw=%q err=%v", status, raw, err)
	}
	refundID, err := p.RefundPurchase(context.Background(), "42", "refund-key", 1999)
	if err != nil || refundID != "refund-1" || !sawRefund {
		t.Fatalf("refund=%q saw=%v err=%v", refundID, sawRefund, err)
	}
	status, raw, err = p.GetOperationStatus(context.Background(), OperationRefund, refundID)
	if err != nil || status != StatusRefunded || raw != "finished" {
		t.Fatalf("refund status=%s raw=%q err=%v", status, raw, err)
	}
}

func TestTBankRecurringContract(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		switch r.URL.Path {
		case "/Init":
			if in["Amount"] != float64(1999) {
				t.Fatalf("amount=%#v", in["Amount"])
			}
			_, _ = w.Write([]byte(`{"Success":true,"PaymentId":"p-1","PaymentURL":"https://provider.test/p-1"}`))
		case "/Charge":
			if in["PaymentId"] != "p-1" || in["RebillId"] != "rebill-1" {
				t.Fatalf("charge=%#v", in)
			}
			_, _ = w.Write([]byte(`{"Success":true,"Status":"CONFIRMED","PaymentId":"p-1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer s.Close()
	p, err := NewTBank(TBankConfig{TerminalKey: "term", Password: "password", BaseURL: s.URL, Client: s.Client()})
	if err != nil {
		t.Fatal(err)
	}
	out, err := p.ChargeRecurring(context.Background(), PurchaseRequest{AmountKopecks: 1999, Currency: "RUB", InternalReferenceID: "sub-1", IdempotencyKey: "renewal-123456", SavedMethodRef: "rebill-1"})
	if err != nil || out.Status != StatusSucceeded || out.ExternalID != "p-1" || calls != 2 {
		t.Fatalf("out=%+v calls=%d err=%v", out, calls, err)
	}
}

func TestYandexPaySubscriptionAndRecurringContract(t *testing.T) {
	var recurring bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Api-Key ") {
			t.Fatal("missing api auth")
		}
		switch r.URL.Path {
		case "/v1/subscriptions":
			_, _ = w.Write([]byte(`{"status":"success","data":{"paymentUrl":"https://provider.test/sub","subscriptionId":"subscription-1"}}`))
		case "/v1/subscriptions/recur":
			recurring = true
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in["parentOrderId"] != "internal-parent" || in["amount"] != "19.99" {
				t.Fatalf("recur=%#v", in)
			}
			_, _ = w.Write([]byte(`{"status":"success","data":{"operationId":"operation-1"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer s.Close()
	p, err := NewYandexPay(YandexPayConfig{JWKSURL: s.URL + "/jwks", APIKey: "secret", MerchantID: "merchant", BaseURL: s.URL, Client: s.Client()})
	if err != nil {
		t.Fatal(err)
	}
	initial, err := p.CreatePurchase(context.Background(), PurchaseRequest{AmountKopecks: 1999, Currency: "RUB", InternalReferenceID: "internal-parent", IdempotencyKey: "initial-123456", Description: "Naimio PRO", ReturnURL: "https://naimio.test/return", FailURL: "https://naimio.test/fail", SavePaymentMethod: true, BillingPeriod: "MONTH"})
	if err != nil || initial.Status != StatusPendingUserAction || initial.SavedMethodRef == "" {
		t.Fatalf("initial=%+v err=%v", initial, err)
	}
	recur, err := p.ChargeRecurring(context.Background(), PurchaseRequest{AmountKopecks: 1999, Currency: "RUB", InternalReferenceID: "subscription-internal", IdempotencyKey: "renewal-123456", Description: "Naimio PRO renewal", SavedMethodRef: initial.SavedMethodRef})
	if err != nil || recur.Status != StatusProcessing || !recurring {
		t.Fatalf("recur=%+v recurring=%v err=%v", recur, recurring, err)
	}
}

func TestTBankNominalProductionCompositionRequiresMTLS(t *testing.T) {
	_, err := NewTBankNominal(TBankNominalConfig{BearerToken: "token", AccountNumber: "40702810900000000001"})
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("expected missing mTLS files to disable nominal adapter, got %v", err)
	}
}

func TestTBankNominalSafeDealContract(t *testing.T) {
	const account = "40702810900000000001"
	var seen = map[string]bool{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer nominal-token" {
			t.Fatalf("bad auth: %q", r.Header.Get("Authorization"))
		}
		key := r.Method + " " + r.URL.Path
		seen[key] = true
		w.Header().Set("Content-Type", "application/json")
		switch key {
		case "POST /api/v1/nominal-accounts/deals":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["accountNumber"] != account || r.Header.Get("Idempotency-Key") != nominalIdempotencyKey("deal-key") {
				t.Fatalf("bad deal request: %#v key=%q", body, r.Header.Get("Idempotency-Key"))
			}
			_, _ = w.Write([]byte(`{"id":"deal-1","status":"DRAFT"}`))
		case "POST /api/v1/nominal-accounts/deals/deal-1/steps":
			_, _ = w.Write([]byte(`{"id":"step-1","status":"DRAFT"}`))
		case "PUT /api/v1/nominal-accounts/deals/deal-1/steps/step-1/deponents/beneficiary-1":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["amount"] != float64(19.99) {
				t.Fatalf("bad deponent amount: %#v", body["amount"])
			}
			w.WriteHeader(http.StatusNoContent)
		case "POST /api/v1/nominal-accounts/deals/deal-1/steps/step-1/recipients":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["beneficiaryId"] != "beneficiary-1" || body["bankDetailsId"] != "bank-1" || body["amount"] != float64(18) {
				t.Fatalf("bad recipient: %#v", body)
			}
			_, _ = w.Write([]byte(`{"id":"recipient-1"}`))
		case "POST /api/v1/nominal-accounts/deals/deal-1/accept":
			w.WriteHeader(http.StatusNoContent)
		case "POST /api/v1/nominal-accounts/deals/deal-1/steps/step-1/complete":
			w.WriteHeader(http.StatusNoContent)
		case "POST /api/v1/nominal-accounts/payments":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["accountNumber"] != account || body["beneficiaryId"] != "beneficiary-1" || body["bankDetailsId"] != "bank-1" || body["amount"] != float64(18) {
				t.Fatalf("bad payout: %#v", body)
			}
			_, _ = w.Write([]byte(`{"id":"payout-1","status":"PENDING"}`))
		default:
			http.Error(w, "unexpected "+key, http.StatusNotFound)
		}
	}))
	defer s.Close()

	p, err := NewTBankNominal(TBankNominalConfig{BearerToken: "nominal-token", AccountNumber: account, BaseURL: s.URL, Client: s.Client()})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	deal, err := p.CreateDeal(ctx, "deal-key")
	if err != nil || deal.ID != "deal-1" {
		t.Fatalf("deal=%+v err=%v", deal, err)
	}
	step, err := p.CreateStep(ctx, deal.ID, "step-key", "Naimio Safe Deal")
	if err != nil || step.ID != "step-1" {
		t.Fatalf("step=%+v err=%v", step, err)
	}
	if err := p.UpsertDeponent(ctx, deal.ID, step.ID, "beneficiary-1", 1999); err != nil {
		t.Fatal(err)
	}
	recipient, err := p.CreateRecipient(ctx, deal.ID, step.ID, "beneficiary-1", "bank-1", "recipient-key", "Payout for completed work", 1800)
	if err != nil || recipient.ID != "recipient-1" {
		t.Fatalf("recipient=%+v err=%v", recipient, err)
	}
	if err := p.AcceptDeal(ctx, deal.ID, "accept-key"); err != nil {
		t.Fatal(err)
	}
	if err := p.CompleteStep(ctx, deal.ID, step.ID, "complete-key"); err != nil {
		t.Fatal(err)
	}
	payout, err := p.CreatePayout(ctx, "beneficiary-1", "bank-1", "payout-key", "Payout for completed work", 1800)
	if err != nil || payout.ID != "payout-1" {
		t.Fatalf("payout=%+v err=%v", payout, err)
	}
	for _, key := range []string{"POST /api/v1/nominal-accounts/deals", "POST /api/v1/nominal-accounts/deals/deal-1/steps", "PUT /api/v1/nominal-accounts/deals/deal-1/steps/step-1/deponents/beneficiary-1", "POST /api/v1/nominal-accounts/deals/deal-1/steps/step-1/recipients", "POST /api/v1/nominal-accounts/deals/deal-1/accept", "POST /api/v1/nominal-accounts/deals/deal-1/steps/step-1/complete", "POST /api/v1/nominal-accounts/payments"} {
		if !seen[key] {
			t.Fatalf("request not seen: %s", key)
		}
	}
}

func TestCloudPaymentsHostedCheckoutRecurringStatusRefundAndWebhook(t *testing.T) {
	var sawTokenCharge, sawRefund bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, ok := r.BasicAuth(); !ok || user != "public" || pass != "secret" {
			t.Fatalf("bad basic auth user=%q ok=%v", user, ok)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/orders/create":
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in["Amount"] != "99.90" || in["InvoiceId"] != "sub-1" || in["AccountId"] != "user-1" {
				t.Fatalf("checkout=%#v", in)
			}
			_, _ = w.Write([]byte(`{"Success":true,"Model":{"Id":"order-1","Url":"https://provider.test/order-1","Status":"Created"}}`))
		case "/v2/payments/find":
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in["InvoiceId"] != "sub-1" && in["InvoiceId"] != "renew-1" {
				t.Fatalf("find=%#v", in)
			}
			_, _ = w.Write([]byte(`{"Success":true,"Model":{"TransactionId":42,"Status":"Completed","Token":"token-1","InvoiceId":"sub-1"}}`))
		case "/payments/tokens/charge":
			sawTokenCharge = true
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in["Amount"] != "99.90" || in["InvoiceId"] != "renew-1" || in["Token"] != "token-1" {
				t.Fatalf("token charge=%#v", in)
			}
			_, _ = w.Write([]byte(`{"Success":true,"Model":{"TransactionId":43,"Status":"Completed"}}`))
		case "/payments/refund":
			sawRefund = true
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in["TransactionId"] != "42" || in["Amount"] != "99.90" {
				t.Fatalf("refund=%#v", in)
			}
			_, _ = w.Write([]byte(`{"Success":true,"Model":{"TransactionId":44}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer s.Close()
	p, err := NewCloudPayments(CloudPaymentsConfig{PublicID: "public", APISecret: "secret", BaseURL: s.URL, Client: s.Client()})
	if err != nil {
		t.Fatal(err)
	}
	checkout, err := p.CreatePurchase(context.Background(), PurchaseRequest{AmountKopecks: 9990, Currency: "RUB", InternalReferenceID: "sub-1", IdempotencyKey: "checkout-123456", Description: "Naimio PRO", CustomerID: "user-1", ReturnURL: "https://naimio.test/ok", FailURL: "https://naimio.test/fail", SavePaymentMethod: true})
	if err != nil || checkout.Status != StatusPendingUserAction || checkout.ConfirmationURL == "" {
		t.Fatalf("checkout=%+v err=%v", checkout, err)
	}
	details, err := p.GetStatusDetails(context.Background(), "sub-1")
	if err != nil || details.Status != StatusSucceeded || details.SavedMethodRef != "token-1" {
		t.Fatalf("details=%+v err=%v", details, err)
	}
	recur, err := p.ChargeRecurring(context.Background(), PurchaseRequest{AmountKopecks: 9990, Currency: "RUB", InternalReferenceID: "renew-1", IdempotencyKey: "renew-123456", SavedMethodRef: "token-1"})
	if err != nil || recur.Status != StatusSucceeded || !sawTokenCharge {
		t.Fatalf("recur=%+v err=%v", recur, err)
	}
	refund, err := p.RefundPurchase(context.Background(), "sub-1", "refund-123456", 9990)
	if err != nil || refund != "44" || !sawRefund {
		t.Fatalf("refund=%q err=%v", refund, err)
	}
	body := []byte(`{"TransactionId":42,"InvoiceId":"sub-1","Status":"Completed","Token":"token-1"}`)
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write(body)
	event, err := p.VerifyWebhook(context.Background(), body, map[string][]string{"Content-HMAC": {base64.StdEncoding.EncodeToString(mac.Sum(nil))}})
	if err != nil || event.Status != StatusSucceeded || event.SavedMethodRef != "token-1" {
		t.Fatalf("event=%+v err=%v", event, err)
	}
}

func TestTBankCheckoutStatusRefundAndSavedRebillContract(t *testing.T) {
	var cancelSeen bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/Init":
			if in["Recurrent"] != "Y" || in["CustomerKey"] != "user-1" || in["Amount"] != float64(9990) {
				t.Fatalf("init=%#v", in)
			}
			_, _ = w.Write([]byte(`{"Success":true,"PaymentId":"pay-1","PaymentURL":"https://provider.test/pay-1"}`))
		case "/GetState":
			_, _ = w.Write([]byte(`{"Success":true,"Status":"CONFIRMED","RebillId":"rebill-1"}`))
		case "/Cancel":
			cancelSeen = true
			_, _ = w.Write([]byte(`{"Success":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer s.Close()
	p, err := NewTBank(TBankConfig{TerminalKey: "term", Password: "pass", BaseURL: s.URL, Client: s.Client()})
	if err != nil {
		t.Fatal(err)
	}
	out, err := p.CreatePurchase(context.Background(), PurchaseRequest{AmountKopecks: 9990, Currency: "RUB", InternalReferenceID: "sub-1", IdempotencyKey: "checkout-123456", ReturnURL: "https://naimio.test/ok", FailURL: "https://naimio.test/fail", SavePaymentMethod: true, CustomerID: "user-1"})
	if err != nil || out.ExternalID != "pay-1" || out.ConfirmationURL == "" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	details, err := p.GetStatusDetails(context.Background(), "pay-1")
	if err != nil || details.Status != StatusSucceeded || details.SavedMethodRef != "rebill-1" {
		t.Fatalf("details=%+v err=%v", details, err)
	}
	if _, err := p.RefundPurchase(context.Background(), "pay-1", "refund-123456", 9990); err != nil || !cancelSeen {
		t.Fatalf("refund err=%v", err)
	}
}

func TestYandexPayStatusAndRefundContract(t *testing.T) {
	var refunded bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Api-Key secret" {
			t.Fatal("missing api key")
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/orders/order-1":
			_, _ = w.Write([]byte(`{"status":"success","data":{"order":{"paymentStatus":"CAPTURED"}}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v2/orders/order-1/refund":
			refunded = true
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in["refundAmount"] != "19.99" {
				t.Fatalf("refund=%#v", in)
			}
			_, _ = w.Write([]byte(`{"status":"success","data":{"operation":{"operationId":"refund-1"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer s.Close()
	p, err := NewYandexPay(YandexPayConfig{JWKSURL: s.URL + "/jwks", APIKey: "secret", MerchantID: "merchant", BaseURL: s.URL, Client: s.Client()})
	if err != nil {
		t.Fatal(err)
	}
	st, raw, err := p.GetStatus(context.Background(), "order-1")
	if err != nil || st != StatusSucceeded || raw != "CAPTURED" {
		t.Fatalf("status=%s raw=%q err=%v", st, raw, err)
	}
	id, err := p.RefundPurchase(context.Background(), "order-1", "refund-123456", 1999)
	if err != nil || id != "refund-1" || !refunded {
		t.Fatalf("id=%q err=%v", id, err)
	}
}

func TestRobokassaRecurringActuallyCallsProvider(t *testing.T) {
	var called bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Merchant/Recurring" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		called = true
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("MerchantLogin") != "shop" || r.Form.Get("PreviousInvoiceID") != "154" || r.Form.Get("OutSum") != "19.99" {
			t.Fatalf("form=%v", r.Form)
		}
		invoice := r.Form.Get("InvoiceID")
		wantSig := md5hex("shop:19.99:" + invoice + ":one")
		if r.Form.Get("SignatureValue") != wantSig {
			t.Fatal("bad recurring signature")
		}
		_, _ = w.Write([]byte("OK" + invoice))
	}))
	defer s.Close()
	p := Robokassa{Login: "shop", Password1: "one", Password2: "two", BaseURL: s.URL + "/Merchant/Index.aspx", Client: s.Client()}
	out, err := p.ChargeRecurring(context.Background(), PurchaseRequest{AmountKopecks: 1999, Currency: "RUB", InternalReferenceID: "sub-1", IdempotencyKey: "renewal-robokassa-123456", Description: "Naimio PRO renewal", SavedMethodRef: "154"})
	if err != nil || !called || out.Status != StatusProcessing || out.ConfirmationURL != "" {
		t.Fatalf("out=%+v called=%v err=%v", out, called, err)
	}
}

func TestCloudPaymentsEscrowContract(t *testing.T) {
	var sawCharge, sawPayout, sawInfo bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/payments/tokens/charge":
			sawCharge = true
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			escrow, _ := in["Escrow"].(map[string]any)
			if in["Amount"] != "100.00" || escrow["StartAccumulation"] != true || escrow["EscrowType"] != "OneToN" {
				t.Fatalf("charge=%#v", in)
			}
			_, _ = w.Write([]byte(`{"Success":true,"Model":{"TransactionId":1001,"Status":"Completed","EscrowAccumulationId":"acc-1"}}`))
		case "/payments/token/topup":
			sawPayout = true
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			escrow, _ := in["Escrow"].(map[string]any)
			if in["Amount"] != "90.00" || escrow["AccumulationId"] != "acc-1" || escrow["FinalPayout"] != true {
				t.Fatalf("payout=%#v", in)
			}
			_, _ = w.Write([]byte(`{"Success":true,"Model":{"TransactionId":2001,"Status":"Completed"}}`))
		case "/Escrow/GetEscrowInfo":
			sawInfo = true
			_, _ = w.Write([]byte(`{"Success":true,"Model":[{"Status":"Closed","EscrowAccumulationId":"acc-1","Balance":0.00}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer s.Close()
	p, err := NewCloudPayments(CloudPaymentsConfig{PublicID: "public", APISecret: "secret", BaseURL: s.URL, Client: s.Client()})
	if err != nil {
		t.Fatal(err)
	}
	fund, err := p.ChargeEscrowToken(context.Background(), 10000, "deal-1", "customer-1", "payer-token", "", "OneToN", true)
	if err != nil || fund.AccumulationID != "acc-1" || fund.TransactionID != "1001" || fund.Status != StatusSucceeded {
		t.Fatalf("fund=%+v err=%v", fund, err)
	}
	payout, err := p.PayoutEscrowToken(context.Background(), 9000, "deal-1-payout", "freelancer-1", "recipient-token", "acc-1", "OneToN", []int64{1001}, true)
	if err != nil || payout.TransactionID != "2001" {
		t.Fatalf("payout=%+v err=%v", payout, err)
	}
	info, err := p.GetEscrowInfo(context.Background(), "acc-1")
	if err != nil || info.Status != "Closed" || info.BalanceKopecks != 0 || !sawCharge || !sawPayout || !sawInfo {
		t.Fatalf("info=%+v err=%v flags=%v/%v/%v", info, err, sawCharge, sawPayout, sawInfo)
	}
}
