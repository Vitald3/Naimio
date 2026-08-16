package matching

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

type Config struct {
	Weights                        Weights
	RetrievalLimit, ShortlistLimit int
}

func LoadConfig(getenv func(string) string) (Config, error) {
	config := Config{Weights: DefaultWeights(), RetrievalLimit: 100, ShortlistLimit: 20}
	if raw := strings.TrimSpace(getenv("MATCHING_WEIGHTS_JSON")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &config.Weights); err != nil {
			return Config{}, errors.New("MATCHING_WEIGHTS_JSON is invalid")
		}
	}
	settings := map[string]struct {
		target   *int
		min, max int
	}{"MATCHING_RETRIEVAL_LIMIT": {&config.RetrievalLimit, 50, 200}, "MATCHING_SHORTLIST_LIMIT": {&config.ShortlistLimit, 1, 20}}
	for key, setting := range settings {
		if raw := getenv(key); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < setting.min || value > setting.max {
				return Config{}, errors.New(key + " is invalid")
			}
			*setting.target = value
		}
	}
	if err := config.Weights.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}
