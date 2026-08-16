package payments

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestYooKassaPaymentContract(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "shop" || p != "secret" || r.Header.Get("Idempotence-Key") != "stable-key" {
			t.Fatal("authentication/idempotency missing")
		}
		if r.URL.Path != "/payments" {
			t.Fatal(r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"pay_1","status":"pending","confirmation":{"confirmation_url":"https://pay"}}`))
	}))
	defer s.Close()
	p, e := NewYooKassa(YooKassaConfig{ShopID: "shop", SecretKey: "secret", BaseURL: s.URL})
	if e != nil {
		t.Fatal(e)
	}
	v, e := p.CreatePayment(context.Background(), CheckoutRequest{AmountKopecks: 199900, Currency: "RUB", IdempotencyKey: "stable-key", ReturnURL: "https://naimio.ru/return"})
	if e != nil || v.ExternalID != "pay_1" {
		t.Fatalf("%+v %v", v, e)
	}
}

func TestYooKassaWebhookAuthoritativelyChecksPayment(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/payments/pay_1" {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":"pay_1","status":"succeeded"}`))
	}))
	defer s.Close()
	p, err := NewYooKassa(YooKassaConfig{ShopID: "shop", SecretKey: "secret", BaseURL: s.URL})
	if err != nil {
		t.Fatal(err)
	}
	v, err := p.VerifyWebhook(context.Background(), []byte(`{"event":"payment.succeeded","object":{"id":"pay_1","status":"succeeded"}}`), nil)
	if err != nil || v.Status != StatusSucceeded {
		t.Fatalf("%+v %v", v, err)
	}
}

func TestYooKassaSafeDealContract(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		switch r.URL.Path {
		case "/deals":
			if in["type"] != "safe_deal" {
				t.Fatal(in)
			}
			_, _ = w.Write([]byte(`{"id":"deal-1","status":"opened"}`))
		case "/payments":
			deal, ok := in["deal"].(map[string]any)
			if !ok || deal["id"] != "deal-1" {
				t.Fatal(in)
			}
			_, _ = w.Write([]byte(`{"id":"pay-1","status":"pending","confirmation":{"confirmation_url":"https://pay"}}`))
		case "/payouts":
			if in["payout_token"] != "token" {
				t.Fatal(in)
			}
			_, _ = w.Write([]byte(`{"id":"payout-1","status":"pending"}`))
		default:
			t.Fatal(r.URL.Path)
		}
	}))
	defer s.Close()
	p, err := NewYooKassa(YooKassaConfig{ShopID: "shop", SecretKey: "secret", BaseURL: s.URL})
	if err != nil {
		t.Fatal(err)
	}
	if id, _, e := p.CreateSafeDeal(context.Background(), SafeDealRequest{IdempotencyKey: "deal-key-1", FeeMoment: "payment_succeeded"}); e != nil || id != "deal-1" {
		t.Fatal(id, e)
	}
	if v, e := p.CreateSafeDealPayment(context.Background(), SafeDealPaymentRequest{DealID: "deal-1", IdempotencyKey: "payment-key", AmountKopecks: 10000, PayoutKopecks: 8000, ReturnURL: "https://naimio.ru/return", Capture: true}); e != nil || v.ExternalID != "pay-1" {
		t.Fatal(v, e)
	}
	if id, _, e := p.CreateSafeDealPayout(context.Background(), SafeDealPayoutRequest{DealID: "deal-1", PayoutToken: "token", IdempotencyKey: "payout-key", AmountKopecks: 8000}); e != nil || id != "payout-1" {
		t.Fatal(id, e)
	}
}

func TestYooKassaOperationStatusUsesRefundAndPayoutResources(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/refunds/ref-1":
			_, _ = w.Write([]byte(`{"id":"ref-1","status":"succeeded"}`))
		case "/payouts/out-1":
			_, _ = w.Write([]byte(`{"id":"out-1","status":"succeeded"}`))
		default:
			t.Fatalf("unexpected provider status resource: %s", r.URL.Path)
		}
	}))
	defer s.Close()
	p, err := NewYooKassa(YooKassaConfig{ShopID: "shop", SecretKey: "secret", BaseURL: s.URL})
	if err != nil {
		t.Fatal(err)
	}
	status, raw, err := p.GetOperationStatus(context.Background(), OperationRefund, "ref-1")
	if err != nil || status != StatusRefunded || raw != "succeeded" {
		t.Fatalf("refund: status=%s raw=%q err=%v", status, raw, err)
	}
	status, raw, err = p.GetOperationStatus(context.Background(), OperationPayout, "out-1")
	if err != nil || status != StatusSucceeded || raw != "succeeded" {
		t.Fatalf("payout: status=%s raw=%q err=%v", status, raw, err)
	}
}
