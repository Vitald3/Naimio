// Package runtime contains startup-only configuration validation. It deliberately
// has no business-domain dependencies so production failures are clear and early.
package runtime

import (
	"fmt"
	"net/url"
	"strings"
)

type Config struct {
	Environment string
	PublicURL   string
}

func Load(getenv func(string) string) (Config, error) {
	env := strings.ToLower(strings.TrimSpace(getenv("APP_ENV")))
	if env == "" {
		env = "development"
	}
	if env != "development" && env != "test" && env != "staging" && env != "production" {
		return Config{}, fmt.Errorf("APP_ENV must be development, test, staging, or production")
	}
	c := Config{Environment: env, PublicURL: strings.TrimRight(strings.TrimSpace(getenv("PUBLIC_BASE_URL")), "/")}
	if env != "production" {
		return c, nil
	}
	for _, key := range []string{"DATABASE_URL", "REDIS_URL", "PUBLIC_BASE_URL", "OBJECT_STORAGE_ENDPOINT", "OBJECT_STORAGE_REGION", "OBJECT_STORAGE_BUCKET", "OBJECT_STORAGE_ACCESS_KEY", "OBJECT_STORAGE_SECRET_KEY"} {
		if strings.TrimSpace(getenv(key)) == "" {
			return Config{}, fmt.Errorf("%s is required when APP_ENV=production", key)
		}
	}
	u, err := url.Parse(c.PublicURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return Config{}, fmt.Errorf("PUBLIC_BASE_URL must be an absolute HTTPS URL in production")
	}
	if strings.TrimSpace(getenv("SAFE_DEAL_PROVIDER")) == "sandbox" {
		return Config{}, fmt.Errorf("SAFE_DEAL_PROVIDER=sandbox is not allowed in production; use disabled until a real adapter is configured")
	}
	return c, nil
}
