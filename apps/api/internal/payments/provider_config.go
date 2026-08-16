package payments

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"freelance/apps/api/internal/platform/requestmeta"
	"io"
	"strings"
	"sync"
)

// ProviderConfig is encrypted at rest. Secrets are never returned by the admin API.
type ProviderConfig struct {
	Provider    ProviderName      `json:"provider"`
	Environment Environment       `json:"environment"`
	Values      map[string]string `json:"values"`
}

type ProviderConfigView struct {
	Provider    ProviderName        `json:"provider"`
	Environment Environment         `json:"environment"`
	Configured  bool                `json:"configured"`
	Fields      []ProviderFieldView `json:"fields"`
}

type ProviderFieldView struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	Secret     bool   `json:"secret"`
	Required   bool   `json:"required"`
	Configured bool   `json:"configured"`
	Value      string `json:"value,omitempty"`
}

type providerField struct {
	Key, Label       string
	Secret, Required bool
}

var providerFields = map[ProviderName][]providerField{
	ProviderYooKassa: {
		{Key: "shop_id", Label: "Shop ID", Required: true},
		{Key: "secret_key", Label: "Secret key", Secret: true, Required: true},
		{Key: "base_url", Label: "API Base URL"},
	},
	ProviderTBank: {
		{Key: "terminal_key", Label: "Terminal key", Required: true},
		{Key: "password", Label: "Acquiring password", Secret: true, Required: true},
		{Key: "base_url", Label: "Acquiring Base URL"},
		{Key: "nominal_bearer_token", Label: "Nominal Bearer token", Secret: true},
		{Key: "nominal_account_number", Label: "Nominal account number"},
		{Key: "nominal_base_url", Label: "Nominal API Base URL"},
		{Key: "nominal_client_cert_pem", Label: "mTLS client certificate (PEM)", Secret: true},
		{Key: "nominal_client_key_pem", Label: "mTLS private key (PEM)", Secret: true},
	},
	ProviderCloudPayments: {
		{Key: "public_id", Label: "Public ID", Required: true},
		{Key: "api_secret", Label: "API secret", Secret: true, Required: true},
		{Key: "base_url", Label: "API Base URL"},
	},
	ProviderYandexPay: {
		{Key: "api_key", Label: "API key", Secret: true, Required: true},
		{Key: "merchant_id", Label: "Merchant ID", Required: true},
		{Key: "jwks_url", Label: "JWKS URL", Required: true},
		{Key: "base_url", Label: "API Base URL"},
	},
	ProviderRobokassa: {
		{Key: "login", Label: "Merchant login", Required: true},
		{Key: "password1", Label: "Password #1", Secret: true, Required: true},
		{Key: "password2", Label: "Password #2", Secret: true, Required: true},
		{Key: "password3", Label: "Password #3", Secret: true},
		{Key: "base_url", Label: "Checkout Base URL"},
		{Key: "services_base_url", Label: "Services Base URL"},
	},
}

func ProviderConfigTemplate(provider ProviderName) ([]ProviderFieldView, error) {
	fields, ok := providerFields[provider]
	if !ok {
		return nil, ErrUnknownProvider
	}
	out := make([]ProviderFieldView, 0, len(fields))
	for _, f := range fields {
		out = append(out, ProviderFieldView{Key: f.Key, Label: f.Label, Secret: f.Secret, Required: f.Required})
	}
	return out, nil
}

func ValidateProviderConfig(c ProviderConfig) error {
	if c.Environment != EnvironmentSandbox && c.Environment != EnvironmentProduction {
		return errors.New("invalid provider environment")
	}
	fields, ok := providerFields[c.Provider]
	if !ok {
		return ErrUnknownProvider
	}
	allowed := map[string]bool{}
	for _, f := range fields {
		allowed[f.Key] = true
		if f.Required && strings.TrimSpace(c.Values[f.Key]) == "" {
			return fmt.Errorf("missing required field %s", f.Key)
		}
	}
	for k := range c.Values {
		if !allowed[k] {
			return fmt.Errorf("unknown provider config field %s", k)
		}
	}
	return nil
}

type ProviderConfigStore struct {
	DB  *sql.DB
	key [32]byte
}

func NewProviderConfigStore(db *sql.DB, masterKey string) (*ProviderConfigStore, error) {
	if db == nil {
		return nil, errors.New("provider config store requires database")
	}
	if strings.TrimSpace(masterKey) == "" {
		return nil, errors.New("payment provider config encryption key is not configured")
	}
	return &ProviderConfigStore{DB: db, key: sha256.Sum256([]byte(masterKey))}, nil
}

func (s *ProviderConfigStore) encrypt(v map[string]string) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, raw, nil)
	return base64.RawStdEncoding.EncodeToString(sealed), nil
}
func (s *ProviderConfigStore) decrypt(raw string) (map[string]string, error) {
	blob, err := base64.RawStdEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(blob) < gcm.NonceSize() {
		return nil, errors.New("invalid encrypted provider config")
	}
	plain, err := gcm.Open(nil, blob[:gcm.NonceSize()], blob[gcm.NonceSize():], nil)
	if err != nil {
		return nil, err
	}
	var values map[string]string
	err = json.Unmarshal(plain, &values)
	return values, err
}
func (s *ProviderConfigStore) Get(ctx context.Context, provider ProviderName) (ProviderConfig, bool, error) {
	var env Environment
	var encrypted string
	err := s.DB.QueryRowContext(ctx, `SELECT environment,encrypted_config FROM payment_provider_credentials WHERE provider=$1`, provider).Scan(&env, &encrypted)
	if errors.Is(err, sql.ErrNoRows) {
		return ProviderConfig{}, false, nil
	}
	if err != nil {
		return ProviderConfig{}, false, err
	}
	values, err := s.decrypt(encrypted)
	if err != nil {
		return ProviderConfig{}, false, err
	}
	return ProviderConfig{Provider: provider, Environment: env, Values: values}, true, nil
}
func (s *ProviderConfigStore) Put(ctx context.Context, c ProviderConfig, actor string) error {
	if err := ValidateProviderConfig(c); err != nil {
		return err
	}
	enc, err := s.encrypt(c.Values)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO payment_provider_settings(provider,enabled,environment,updated_by,updated_at)
		VALUES($1,false,$2,NULLIF($3,'')::uuid,now())
		ON CONFLICT(provider) DO UPDATE SET environment=excluded.environment,updated_by=excluded.updated_by,updated_at=now()`, c.Provider, c.Environment, actor)
	if err != nil {
		return fmt.Errorf("provider settings upsert: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO payment_provider_credentials(provider,environment,encrypted_config,updated_by,updated_at) VALUES($1,$2,$3,NULLIF($4,'')::uuid,now()) ON CONFLICT(provider) DO UPDATE SET environment=excluded.environment,encrypted_config=excluded.encrypted_config,updated_by=excluded.updated_by,updated_at=now()`, c.Provider, c.Environment, enc, actor)
	if err != nil {
		return fmt.Errorf("provider credentials upsert: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_logs(id,actor_user_id,action,target_type,target_id,metadata,ip) VALUES(gen_random_uuid(),NULLIF($1,'')::uuid,'payment_provider.config_updated','PAYMENT_PROVIDER',NULL,jsonb_build_object('provider',$2::text,'environment',$3::text),NULLIF($4,'')::inet)`, actor, c.Provider, c.Environment, requestmeta.FromContext(ctx))
	if err != nil {
		return fmt.Errorf("provider config audit: %w", err)
	}
	return tx.Commit()
}
func (s *ProviderConfigStore) View(ctx context.Context, provider ProviderName) (ProviderConfigView, error) {
	tpl, err := ProviderConfigTemplate(provider)
	if err != nil {
		return ProviderConfigView{}, err
	}
	c, ok, err := s.Get(ctx, provider)
	if err != nil {
		return ProviderConfigView{}, err
	}
	v := ProviderConfigView{Provider: provider, Environment: EnvironmentSandbox, Fields: tpl}
	if !ok {
		return v, nil
	}
	v.Environment = c.Environment
	v.Configured = ValidateProviderConfig(c) == nil
	for i := range v.Fields {
		val := strings.TrimSpace(c.Values[v.Fields[i].Key])
		v.Fields[i].Configured = val != ""
		if !v.Fields[i].Secret {
			v.Fields[i].Value = val
		}
	}
	return v, nil
}

// ProviderRuntime allows admin-saved DB credentials to become effective immediately
// in the current API process. Database configuration remains authoritative; every
// admin read refreshes the runtime, and process startup loads it before serving.
type ProviderRuntime struct {
	mu         sync.RWMutex
	set        ProviderSet
	yoo        *YooKassa
	nominal    *TBankNominal
	configured map[ProviderName]bool
	store      *ProviderConfigStore
}

func NewProviderRuntime() *ProviderRuntime {
	return &ProviderRuntime{set: ProviderSet{Purchases: map[ProviderName]PurchaseProvider{}, Recurring: map[ProviderName]RecurringProvider{}, Webhooks: map[ProviderName]WebhookVerifier{}, Statuses: map[ProviderName]StatusProvider{}}, configured: map[ProviderName]bool{}}
}
func NewProviderRuntimeFromSet(set ProviderSet, yoo *YooKassa, nominal *TBankNominal, configured map[ProviderName]bool) *ProviderRuntime {
	r := NewProviderRuntime()
	r.set = ProviderSet{Purchases: clonePurchases(set.Purchases), Recurring: cloneRecurring(set.Recurring), Webhooks: cloneWebhooks(set.Webhooks), Statuses: cloneStatuses(set.Statuses)}
	for k, v := range configured {
		r.configured[k] = v
	}
	r.yoo, r.nominal = yoo, nominal
	return r
}

func (r *ProviderRuntime) AttachStore(store *ProviderConfigStore) {
	r.mu.Lock()
	r.store = store
	r.mu.Unlock()
}
func (r *ProviderRuntime) refresh(p ProviderName) {
	r.mu.RLock()
	store := r.store
	r.mu.RUnlock()
	if store == nil {
		return
	}
	cfg, ok, err := store.Get(context.Background(), p)
	if err == nil && ok {
		_ = r.Apply(cfg)
	}
}
func (r *ProviderRuntime) Purchase(p ProviderName) (PurchaseProvider, error) {
	r.refresh(p)
	r.mu.RLock()
	defer r.mu.RUnlock()
	v := r.set.Purchases[p]
	if v == nil {
		return nil, ErrProviderUnavailable
	}
	return v, nil
}
func (r *ProviderRuntime) Recurring(p ProviderName) (RecurringProvider, error) {
	r.refresh(p)
	r.mu.RLock()
	defer r.mu.RUnlock()
	v := r.set.Recurring[p]
	if v == nil {
		return nil, ErrUnsupportedRoute
	}
	return v, nil
}
func (r *ProviderRuntime) Status(p ProviderName) (StatusProvider, error) {
	r.refresh(p)
	r.mu.RLock()
	defer r.mu.RUnlock()
	v := r.set.Statuses[p]
	if v == nil {
		return nil, ErrProviderUnavailable
	}
	return v, nil
}
func (r *ProviderRuntime) Webhook(p ProviderName) (WebhookVerifier, error) {
	r.refresh(p)
	r.mu.RLock()
	defer r.mu.RUnlock()
	v := r.set.Webhooks[p]
	if v == nil {
		return nil, ErrProviderUnavailable
	}
	return v, nil
}
func (r *ProviderRuntime) IsConfigured(p ProviderName) bool {
	r.refresh(p)
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.configured[p]
}
func (r *ProviderRuntime) Snapshot() ProviderSet {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return ProviderSet{Purchases: clonePurchases(r.set.Purchases), Recurring: cloneRecurring(r.set.Recurring), Webhooks: cloneWebhooks(r.set.Webhooks), Statuses: cloneStatuses(r.set.Statuses), Runtime: r}
}
func (r *ProviderRuntime) YooKassa() *YooKassa {
	r.refresh(ProviderYooKassa)
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.yoo
}
func (r *ProviderRuntime) TBankNominal() *TBankNominal {
	r.refresh(ProviderTBank)
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.nominal
}
func (r *ProviderRuntime) Apply(c ProviderConfig) error {
	if err := ValidateProviderConfig(c); err != nil {
		return err
	}
	var purchase PurchaseProvider
	var recurring RecurringProvider
	var webhook WebhookVerifier
	var status StatusProvider
	var yoo *YooKassa
	var nominal *TBankNominal
	var err error
	v := c.Values
	switch c.Provider {
	case ProviderYooKassa:
		yoo, err = NewYooKassa(YooKassaConfig{ShopID: v["shop_id"], SecretKey: v["secret_key"], BaseURL: v["base_url"]})
		purchase, recurring, webhook, status = yoo, yoo, yoo, yoo
	case ProviderTBank:
		var p *TBank
		p, err = NewTBank(TBankConfig{TerminalKey: v["terminal_key"], Password: v["password"], BaseURL: v["base_url"]})
		purchase, recurring, webhook, status = p, p, p, p
		if err == nil {
			nominalToken := strings.TrimSpace(v["nominal_bearer_token"])
			nominalAccount := strings.TrimSpace(v["nominal_account_number"])
			nominalCert := strings.TrimSpace(v["nominal_client_cert_pem"])
			nominalKey := strings.TrimSpace(v["nominal_client_key_pem"])
			// Nominal-account credentials are an optional marketplace extension.
			// Partial values must not break ordinary acquiring/PRO configuration;
			// the nominal adapter becomes available only after the whole mTLS set is present.
			if nominalToken != "" && nominalAccount != "" && nominalCert != "" && nominalKey != "" {
				// Nominal-account/mTLS is an optional Safe Deal extension. Invalid or
				// not-yet-activated marketplace credentials must not prevent the
				// acquiring/PRO adapter from being configured and enabled.
				if candidate, nominalErr := NewTBankNominal(TBankNominalConfig{BearerToken: nominalToken, AccountNumber: nominalAccount, BaseURL: v["nominal_base_url"], ClientCertPEM: nominalCert, ClientKeyPEM: nominalKey}); nominalErr == nil {
					nominal = candidate
				}
			}
		}
	case ProviderCloudPayments:
		var p *CloudPayments
		p, err = NewCloudPayments(CloudPaymentsConfig{PublicID: v["public_id"], APISecret: v["api_secret"], BaseURL: v["base_url"]})
		purchase, recurring, webhook, status = p, p, p, p
	case ProviderYandexPay:
		var p *YandexPay
		p, err = NewYandexPay(YandexPayConfig{JWKSURL: v["jwks_url"], APIKey: v["api_key"], MerchantID: v["merchant_id"], BaseURL: v["base_url"]})
		purchase, recurring, webhook, status = p, p, p, p
	case ProviderRobokassa:
		p := &Robokassa{Login: v["login"], Password1: v["password1"], Password2: v["password2"], Password3: v["password3"], BaseURL: v["base_url"], ServicesBaseURL: v["services_base_url"], Test: c.Environment == EnvironmentSandbox}
		purchase, recurring, webhook, status = p, p, p, p
	default:
		return ErrUnknownProvider
	}
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.set.Purchases[c.Provider] = purchase
	r.set.Recurring[c.Provider] = recurring
	r.set.Webhooks[c.Provider] = webhook
	r.set.Statuses[c.Provider] = status
	r.configured[c.Provider] = true
	if c.Provider == ProviderYooKassa {
		r.yoo = yoo
	}
	if c.Provider == ProviderTBank {
		r.nominal = nominal
	}
	return nil
}
func (r *ProviderRuntime) LoadAll(ctx context.Context, store *ProviderConfigStore) error {
	for _, p := range []ProviderName{ProviderYooKassa, ProviderTBank, ProviderCloudPayments, ProviderYandexPay, ProviderRobokassa} {
		c, ok, err := store.Get(ctx, p)
		if err != nil {
			return err
		}
		if ok {
			if err := r.Apply(c); err != nil {
				return fmt.Errorf("%s: %w", p, err)
			}
		}
	}
	return nil
}
func clonePurchases(in map[ProviderName]PurchaseProvider) map[ProviderName]PurchaseProvider {
	out := map[ProviderName]PurchaseProvider{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
func cloneRecurring(in map[ProviderName]RecurringProvider) map[ProviderName]RecurringProvider {
	out := map[ProviderName]RecurringProvider{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
func cloneWebhooks(in map[ProviderName]WebhookVerifier) map[ProviderName]WebhookVerifier {
	out := map[ProviderName]WebhookVerifier{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
func cloneStatuses(in map[ProviderName]StatusProvider) map[ProviderName]StatusProvider {
	out := map[ProviderName]StatusProvider{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
