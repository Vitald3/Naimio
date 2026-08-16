package payments

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// TBankNominal implements the contracted T-Bank OpenAPI nominal-account
// primitives used by marketplace Safe Deal deployments. Authentication is
// Bearer + mTLS: production may provide merchant certificate/key file paths,
// while contract tests inject an explicit local http.Client. Admin-managed PEM material
// is stored only inside the encrypted provider-configuration blob and is never returned by the API.
type TBankNominalConfig struct {
	BearerToken    string
	AccountNumber  string
	BaseURL        string
	Client         *http.Client
	ClientCertFile string
	ClientKeyFile  string
	ClientCertPEM  string
	ClientKeyPEM   string
}

type TBankNominal struct {
	token, account, base string
	client               *http.Client
}

var nominalAccountPattern = regexp.MustCompile(`^[0-9]{20}(?:[0-9]{2})?$`)

func NewTBankNominal(c TBankNominalConfig) (*TBankNominal, error) {
	c.BearerToken = strings.TrimSpace(c.BearerToken)
	c.AccountNumber = strings.TrimSpace(c.AccountNumber)
	if c.BearerToken == "" || !nominalAccountPattern.MatchString(c.AccountNumber) {
		return nil, ErrProviderUnavailable
	}
	if c.BaseURL == "" {
		c.BaseURL = "https://secured-openapi.tbank.ru"
	}
	if c.Client == nil {
		var cert tls.Certificate
		var err error
		certPEM, keyPEM := strings.TrimSpace(c.ClientCertPEM), strings.TrimSpace(c.ClientKeyPEM)
		if certPEM != "" || keyPEM != "" {
			if certPEM == "" || keyPEM == "" {
				return nil, ErrProviderUnavailable
			}
			cert, err = tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
		} else {
			certFile, keyFile := strings.TrimSpace(c.ClientCertFile), strings.TrimSpace(c.ClientKeyFile)
			if certFile == "" || keyFile == "" {
				return nil, ErrProviderUnavailable
			}
			cert, err = tls.LoadX509KeyPair(certFile, keyFile)
		}
		if err != nil {
			return nil, fmt.Errorf("tbank nominal mtls: %w", err)
		}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}}
		c.Client = &http.Client{Timeout: 15 * time.Second, Transport: transport}
	}
	return &TBankNominal{token: c.BearerToken, account: c.AccountNumber, base: strings.TrimRight(c.BaseURL, "/"), client: c.Client}, nil
}

type TBankNominalDeal struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type TBankNominalStep struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type TBankNominalRecipient struct {
	ID string `json:"id"`
}

type TBankNominalPayment struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func (p *TBankNominal) CreateDeal(ctx context.Context, idempotencyKey string) (TBankNominalDeal, error) {
	var out TBankNominalDeal
	if err := p.call(ctx, http.MethodPost, "/api/v1/nominal-accounts/deals", idempotencyKey, map[string]any{"accountNumber": p.account}, &out); err != nil {
		return TBankNominalDeal{}, err
	}
	if out.ID == "" {
		return TBankNominalDeal{}, errors.New("tbank nominal: empty deal id")
	}
	return out, nil
}

func (p *TBankNominal) CreateStep(ctx context.Context, dealID, idempotencyKey, description string) (TBankNominalStep, error) {
	if strings.TrimSpace(dealID) == "" || len(strings.TrimSpace(description)) < 3 {
		return TBankNominalStep{}, ErrInvalidAttempt
	}
	var out TBankNominalStep
	path := "/api/v1/nominal-accounts/deals/" + dealID + "/steps"
	if err := p.call(ctx, http.MethodPost, path, idempotencyKey, map[string]any{"description": description}, &out); err != nil {
		return TBankNominalStep{}, err
	}
	if out.ID == "" {
		return TBankNominalStep{}, errors.New("tbank nominal: empty step id")
	}
	return out, nil
}

func (p *TBankNominal) UpsertDeponent(ctx context.Context, dealID, stepID, beneficiaryID string, amountKopecks int64) error {
	if dealID == "" || stepID == "" || beneficiaryID == "" || amountKopecks <= 0 {
		return ErrInvalidAttempt
	}
	path := fmt.Sprintf("/api/v1/nominal-accounts/deals/%s/steps/%s/deponents/%s", dealID, stepID, beneficiaryID)
	return p.call(ctx, http.MethodPut, path, "", map[string]any{"amount": decimalRubNumber(amountKopecks)}, nil)
}

func (p *TBankNominal) CreateRecipient(ctx context.Context, dealID, stepID, beneficiaryID, bankDetailsID, idempotencyKey, purpose string, amountKopecks int64) (TBankNominalRecipient, error) {
	if dealID == "" || stepID == "" || beneficiaryID == "" || amountKopecks <= 0 || strings.TrimSpace(purpose) == "" {
		return TBankNominalRecipient{}, ErrInvalidAttempt
	}
	body := map[string]any{"beneficiaryId": beneficiaryID, "amount": decimalRubNumber(amountKopecks), "purpose": purpose, "keepOnVirtualAccount": bankDetailsID == ""}
	if bankDetailsID != "" {
		body["bankDetailsId"] = bankDetailsID
	}
	var out TBankNominalRecipient
	path := fmt.Sprintf("/api/v1/nominal-accounts/deals/%s/steps/%s/recipients", dealID, stepID)
	if err := p.call(ctx, http.MethodPost, path, idempotencyKey, body, &out); err != nil {
		return TBankNominalRecipient{}, err
	}
	if out.ID == "" {
		return TBankNominalRecipient{}, errors.New("tbank nominal: empty recipient id")
	}
	return out, nil
}

func (p *TBankNominal) AcceptDeal(ctx context.Context, dealID, idempotencyKey string) error {
	if dealID == "" {
		return ErrInvalidAttempt
	}
	return p.call(ctx, http.MethodPost, "/api/v1/nominal-accounts/deals/"+dealID+"/accept", idempotencyKey, nil, nil)
}

func (p *TBankNominal) CancelDeal(ctx context.Context, dealID, idempotencyKey string) error {
	if dealID == "" {
		return ErrInvalidAttempt
	}
	return p.call(ctx, http.MethodPost, "/api/v1/nominal-accounts/deals/"+dealID+"/cancel", idempotencyKey, nil, nil)
}

func (p *TBankNominal) CompleteStep(ctx context.Context, dealID, stepID, idempotencyKey string) error {
	if dealID == "" || stepID == "" {
		return ErrInvalidAttempt
	}
	return p.call(ctx, http.MethodPost, fmt.Sprintf("/api/v1/nominal-accounts/deals/%s/steps/%s/complete", dealID, stepID), idempotencyKey, nil, nil)
}

func (p *TBankNominal) GetDeal(ctx context.Context, dealID string) (TBankNominalDeal, error) {
	if dealID == "" {
		return TBankNominalDeal{}, ErrInvalidAttempt
	}
	var out TBankNominalDeal
	if err := p.call(ctx, http.MethodGet, "/api/v1/nominal-accounts/deals/"+dealID, "", nil, &out); err != nil {
		return TBankNominalDeal{}, err
	}
	return out, nil
}

// CreatePayout performs a direct nominal-account payout to an already verified
// beneficiary/bank-details reference. It never accepts raw card details.
func (p *TBankNominal) CreatePayout(ctx context.Context, beneficiaryID, bankDetailsID, idempotencyKey, purpose string, amountKopecks int64) (TBankNominalPayment, error) {
	if beneficiaryID == "" || bankDetailsID == "" || amountKopecks <= 0 || strings.TrimSpace(purpose) == "" {
		return TBankNominalPayment{}, ErrInvalidAttempt
	}
	body := map[string]any{"type": "REGULAR", "beneficiaryId": beneficiaryID, "accountNumber": p.account, "bankDetailsId": bankDetailsID, "amount": decimalRubNumber(amountKopecks), "purpose": purpose}
	var out TBankNominalPayment
	if err := p.call(ctx, http.MethodPost, "/api/v1/nominal-accounts/payments", idempotencyKey, body, &out); err != nil {
		return TBankNominalPayment{}, err
	}
	if out.ID == "" {
		return TBankNominalPayment{}, errors.New("tbank nominal: empty payment id")
	}
	return out, nil
}

func (p *TBankNominal) call(ctx context.Context, method, path, idempotencyKey string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.base+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", nominalIdempotencyKey(idempotencyKey))
	}
	res, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
		return fmt.Errorf("provider status %d", res.StatusCode)
	}
	if out == nil || res.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
		return nil
	}
	return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(out)
}

func decimalRubNumber(kopecks int64) json.Number {
	return json.Number(rub(kopecks))
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

// T-Bank nominal-account APIs require a UUID Idempotency-Key. Naimio keeps
// arbitrary internal idempotency keys, so the adapter deterministically maps
// them to an RFC 4122 UUID without weakening internal duplicate protection.
func nominalIdempotencyKey(key string) string {
	key = strings.TrimSpace(key)
	if uuidPattern.MatchString(key) {
		return strings.ToLower(key)
	}
	sum := sha256.Sum256([]byte(key))
	b := sum[:16]
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}
