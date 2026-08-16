package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type CapabilityConfig struct {
	Enabled                                          bool
	Provider, Model, FallbackProvider, FallbackModel string
	Timeout                                          time.Duration
	Retries, MaxInputChars, MaxOutputTokens          int
	InputCostPerMillion, OutputCostPerMillion        int64
}
type Config map[Capability]CapabilityConfig

func LoadBaselines(raw string) (map[string]Range, error) {
	values := map[string]Range{"default": {MinKopecks: 5_000_000, MaxKopecks: 12_000_000, Currency: "RUB", Confidence: "LOW"}}
	if strings.TrimSpace(raw) == "" {
		return values, nil
	}
	if err := json.Unmarshal([]byte(raw), &values); err != nil || len(values) == 0 || len(values) > 500 {
		return nil, errors.New("AI_ESTIMATE_BASELINES_JSON is invalid")
	}
	for slug, value := range values {
		if strings.TrimSpace(slug) == "" || validateRange(value) != nil {
			return nil, errors.New("AI_ESTIMATE_BASELINES_JSON is invalid")
		}
	}
	if _, ok := values["default"]; !ok {
		return nil, errors.New("AI_ESTIMATE_BASELINES_JSON requires default")
	}
	return values, nil
}

func DefaultConfig() Config {
	config := Config{}
	for _, capability := range []Capability{ProjectBrief, ProjectImport, ProjectEstimate, OfferAnalysis, TaxonomySuggestion, MatchRerank} {
		config[capability] = CapabilityConfig{Enabled: true, Provider: "deterministic", Model: "rules-v1", Timeout: 3 * time.Second, Retries: 1, MaxInputChars: 30000, MaxOutputTokens: 2000}
	}
	config[ProjectImport] = CapabilityConfig{Enabled: true, Provider: "deterministic", Model: "rules-v1", Timeout: 5 * time.Second, Retries: 1, MaxInputChars: 100000, MaxOutputTokens: 3000}
	config[MatchRerank] = CapabilityConfig{Enabled: false, Provider: "deterministic", Model: "rules-v1", Timeout: 2 * time.Second, Retries: 0, MaxInputChars: 12000, MaxOutputTokens: 1000}
	return config
}

func LoadConfig(getenv func(string) string) (Config, error) {
	config := DefaultConfig()
	for capability, value := range config {
		prefix := "AI_" + strings.ToUpper(string(capability)) + "_"
		if raw := getenv(prefix + "ENABLED"); raw != "" {
			parsed, err := strconv.ParseBool(raw)
			if err != nil {
				return nil, fmt.Errorf("%sENABLED must be boolean", prefix)
			}
			value.Enabled = parsed
		}
		if raw := strings.TrimSpace(getenv(prefix + "PROVIDER")); raw != "" {
			value.Provider = raw
		}
		if raw := strings.TrimSpace(getenv(prefix + "MODEL")); raw != "" {
			value.Model = raw
		}
		value.FallbackProvider = strings.TrimSpace(getenv(prefix + "FALLBACK_PROVIDER"))
		value.FallbackModel = strings.TrimSpace(getenv(prefix + "FALLBACK_MODEL"))
		if raw := getenv(prefix + "TIMEOUT"); raw != "" {
			parsed, err := time.ParseDuration(raw)
			if err != nil || parsed <= 0 || parsed > time.Minute {
				return nil, fmt.Errorf("%sTIMEOUT is invalid", prefix)
			}
			value.Timeout = parsed
		}
		for suffix, target := range map[string]*int{"RETRIES": &value.Retries, "MAX_INPUT_CHARS": &value.MaxInputChars, "MAX_OUTPUT_TOKENS": &value.MaxOutputTokens} {
			if raw := getenv(prefix + suffix); raw != "" {
				parsed, err := strconv.Atoi(raw)
				if err != nil || parsed < 0 {
					return nil, fmt.Errorf("%s%s is invalid", prefix, suffix)
				}
				*target = parsed
			}
		}
		for suffix, target := range map[string]*int64{"INPUT_COST_PER_MILLION": &value.InputCostPerMillion, "OUTPUT_COST_PER_MILLION": &value.OutputCostPerMillion} {
			if raw := getenv(prefix + suffix); raw != "" {
				parsed, err := strconv.ParseInt(raw, 10, 64)
				if err != nil || parsed < 0 {
					return nil, fmt.Errorf("%s%s is invalid", prefix, suffix)
				}
				*target = parsed
			}
		}
		if value.Provider == "" || value.Model == "" || value.MaxInputChars < 100 || value.MaxOutputTokens < 1 || value.Retries > 3 {
			return nil, fmt.Errorf("%s capability configuration is invalid", capability)
		}
		if (value.FallbackProvider == "") != (value.FallbackModel == "") {
			return nil, fmt.Errorf("%s fallback provider and model must be configured together", capability)
		}
		config[capability] = value
	}
	return config, nil
}
