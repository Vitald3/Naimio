package runtime

import "testing"

func TestProductionValidation(t *testing.T) {
	values := map[string]string{"APP_ENV": "production"}
	if _, err := Load(func(k string) string { return values[k] }); err == nil {
		t.Fatal("expected missing production configuration failure")
	}
	for _, key := range []string{"DATABASE_URL", "REDIS_URL", "OBJECT_STORAGE_ENDPOINT", "OBJECT_STORAGE_REGION", "OBJECT_STORAGE_BUCKET", "OBJECT_STORAGE_ACCESS_KEY", "OBJECT_STORAGE_SECRET_KEY"} {
		values[key] = "value"
	}
	values["PUBLIC_BASE_URL"] = "https://naimio.ru"
	values["SAFE_DEAL_PROVIDER"] = "disabled"
	if _, err := Load(func(k string) string { return values[k] }); err != nil {
		t.Fatal(err)
	}
	values["SAFE_DEAL_PROVIDER"] = "sandbox"
	if _, err := Load(func(k string) string { return values[k] }); err == nil {
		t.Fatal("expected sandbox rejection")
	}
}
