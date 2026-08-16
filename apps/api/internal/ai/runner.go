package ai

import (
	"context"
	"errors"
	"time"
)

type transientError interface{ Temporary() bool }
type callResult struct {
	Value any
	Usage Usage
}
type providerCall func(context.Context, AIProvider) (callResult, error)

type Runner struct {
	Config    Config
	Providers map[string]AIProvider
	Metrics   MetricRecorder
	Sleep     func(time.Duration)
}

func (r Runner) Run(ctx context.Context, userID string, capability Capability, call providerCall) (any, error) {
	cfg, ok := r.Config[capability]
	if !ok || !cfg.Enabled {
		return nil, ErrUnavailable
	}
	value, err := r.try(ctx, userID, capability, cfg.Provider, cfg.Model, cfg, call)
	if err == nil {
		return value, nil
	}
	if cfg.FallbackProvider != "" {
		fallback := cfg
		fallback.Retries = 0
		if value, fallbackErr := r.try(ctx, userID, capability, cfg.FallbackProvider, cfg.FallbackModel, fallback, call); fallbackErr == nil {
			return value, nil
		}
	}
	return nil, err
}

func (r Runner) try(ctx context.Context, userID string, capability Capability, providerName, model string, cfg CapabilityConfig, call providerCall) (any, error) {
	provider, ok := r.Providers[providerName]
	if !ok {
		if r.Metrics != nil {
			_ = r.Metrics.Record(ctx, RequestMetric{UserID: userID, Capability: capability, Provider: providerName, Model: model, Status: "FAILED", ErrorCode: "PROVIDER_NOT_CONFIGURED"})
		}
		return nil, ErrUnavailable
	}
	var last error
	for attempt := 0; attempt <= cfg.Retries; attempt++ {
		started := time.Now()
		callCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		result, err := call(callCtx, provider)
		cancel()
		status, code := "SUCCEEDED", ""
		if err != nil {
			status, code = "FAILED", "PROVIDER_ERROR"
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(callCtx.Err(), context.DeadlineExceeded) {
				status, code, err = "TIMEOUT", "TIMEOUT", context.DeadlineExceeded
			}
			if errors.Is(err, ErrInvalidOutput) {
				status, code = "INVALID_OUTPUT", "INVALID_OUTPUT"
			}
		}
		metric := RequestMetric{UserID: userID, Capability: capability, Provider: providerName, Model: model, Status: status, ErrorCode: code, InputTokens: result.Usage.InputTokens, OutputTokens: result.Usage.OutputTokens, Latency: time.Since(started)}
		metric.CostMicrounits = (int64(metric.InputTokens)*cfg.InputCostPerMillion + int64(metric.OutputTokens)*cfg.OutputCostPerMillion) / 1_000_000
		if r.Metrics != nil {
			_ = r.Metrics.Record(ctx, metric)
		}
		if err == nil {
			return result.Value, nil
		}
		last = err
		var temporary transientError
		if attempt == cfg.Retries || (!errors.As(err, &temporary) || !temporary.Temporary()) && !errors.Is(err, ErrInvalidOutput) && !errors.Is(err, context.DeadlineExceeded) {
			break
		}
		if r.Sleep != nil {
			r.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
		}
	}
	return nil, last
}
