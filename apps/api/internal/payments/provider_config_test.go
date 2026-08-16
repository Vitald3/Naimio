package payments

import "testing"

func TestProviderConfigTemplateNeverContainsSecretValues(t *testing.T) {
	fields, err := ProviderConfigTemplate(ProviderYooKassa)
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) == 0 {
		t.Fatal("missing provider fields")
	}
	for _, field := range fields {
		if field.Secret && field.Value != "" {
			t.Fatalf("secret field %s leaked a value", field.Key)
		}
	}
}

func TestProviderRuntimeAppliesAdminConfiguration(t *testing.T) {
	runtime := NewProviderRuntime()
	cfg := ProviderConfig{Provider: ProviderYooKassa, Environment: EnvironmentSandbox, Values: map[string]string{"shop_id": "shop-test", "secret_key": "secret-test"}}
	if err := runtime.Apply(cfg); err != nil {
		t.Fatal(err)
	}
	if !runtime.IsConfigured(ProviderYooKassa) {
		t.Fatal("provider should be configured")
	}
	if _, err := runtime.Purchase(ProviderYooKassa); err != nil {
		t.Fatal(err)
	}
}

func TestProviderConfigRequiresExplicitEnvironment(t *testing.T) {
	err := ValidateProviderConfig(ProviderConfig{Provider: ProviderCloudPayments, Values: map[string]string{"public_id": "id", "api_secret": "secret"}})
	if err == nil {
		t.Fatal("missing sandbox/production environment must be rejected")
	}
}

func TestProviderRuntimeAppliesEveryAdminProviderConfig(t *testing.T) {
	tests := []ProviderConfig{
		{Provider: ProviderYooKassa, Environment: EnvironmentSandbox, Values: map[string]string{"shop_id": "shop", "secret_key": "secret"}},
		{Provider: ProviderTBank, Environment: EnvironmentSandbox, Values: map[string]string{"terminal_key": "terminal", "password": "password", "nominal_bearer_token": "optional-partial-value"}},
		{Provider: ProviderCloudPayments, Environment: EnvironmentSandbox, Values: map[string]string{"public_id": "public", "api_secret": "secret"}},
		{Provider: ProviderYandexPay, Environment: EnvironmentSandbox, Values: map[string]string{"api_key": "key", "merchant_id": "merchant", "jwks_url": "https://example.invalid/jwks"}},
		{Provider: ProviderRobokassa, Environment: EnvironmentSandbox, Values: map[string]string{"login": "merchant", "password1": "one", "password2": "two"}},
	}
	for _, cfg := range tests {
		t.Run(string(cfg.Provider), func(t *testing.T) {
			runtime := NewProviderRuntime()
			if err := runtime.Apply(cfg); err != nil {
				t.Fatalf("Apply() error = %v", err)
			}
			if !runtime.IsConfigured(cfg.Provider) {
				t.Fatal("provider should be configured")
			}
			if _, err := runtime.Purchase(cfg.Provider); err != nil {
				t.Fatalf("Purchase() error = %v", err)
			}
		})
	}
}

func TestTBankInvalidOptionalNominalDoesNotBlockAcquiring(t *testing.T) {
	runtime := NewProviderRuntime()
	cfg := ProviderConfig{Provider: ProviderTBank, Environment: EnvironmentSandbox, Values: map[string]string{
		"terminal_key": "terminal", "password": "password",
		"nominal_bearer_token": "token", "nominal_account_number": "40702810000000000001",
		"nominal_client_cert_pem": "not-a-certificate", "nominal_client_key_pem": "not-a-private-key",
	}}
	if err := runtime.Apply(cfg); err != nil {
		t.Fatalf("optional nominal credentials blocked acquiring: %v", err)
	}
	if _, err := runtime.Purchase(ProviderTBank); err != nil {
		t.Fatalf("tbank acquiring not configured: %v", err)
	}
	if runtime.TBankNominal() != nil {
		t.Fatal("invalid optional nominal credentials must not activate nominal adapter")
	}
}
