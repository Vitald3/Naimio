package payments

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"freelance/apps/api/internal/platform/requestmeta"
	"time"
)

// Route is deliberately free of credentials. It is read only while a new
// operation is created; its selected provider is then persisted on Attempt.
type Route struct {
	Domain      Domain       `json:"domain"`
	Provider    ProviderName `json:"provider"`
	Enabled     bool         `json:"enabled"`
	Configured  bool         `json:"configured"`
	Environment Environment  `json:"environment"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

type RoutingRepository interface {
	GetRoute(context.Context, Domain) (Route, error)
}

// MutableRoutingRepository is intentionally limited to non-secret deployment
// choices. The process still decides whether a selected provider is actually
// configured from its environment credentials.
type ProviderSetting struct {
	Provider     ProviderName `json:"provider"`
	Enabled      bool         `json:"enabled"`
	Configured   bool         `json:"configured"`
	Environment  Environment  `json:"environment"`
	Capabilities []Capability `json:"capabilities,omitempty"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

type MutableRoutingRepository interface {
	RoutingRepository
	ListRoutes(context.Context) ([]Route, error)
	ListProviders(context.Context) ([]ProviderSetting, error)
	SetProviderEnabled(context.Context, ProviderName, bool, string) error
	SetRoute(context.Context, Domain, ProviderName, string) (Route, error)
}

type RoutingService struct {
	Repository RoutingRepository
	Registry   Registry
	// ApplicationEnvironment is set by the composition root.  A sandbox route
	// is never a valid source of a new production operation.
	ApplicationEnvironment Environment
}

func (s RoutingService) Select(ctx context.Context, domain Domain, required ...Capability) (ProviderName, error) {
	route, err := s.Repository.GetRoute(ctx, domain)
	if err != nil {
		return "", err
	}
	if err := s.Registry.ValidateRoute(route.Provider, route.Enabled, route.Configured, required...); err != nil {
		return "", err
	}
	if s.ApplicationEnvironment == EnvironmentProduction && route.Environment != EnvironmentProduction {
		return "", ErrProviderUnavailable
	}
	return route.Provider, nil
}

type PostgresRoutingRepository struct {
	DB                     *sql.DB
	ConfiguredProviders    map[ProviderName]bool
	IsProviderConfigured   func(ProviderName) bool
	ApplicationEnvironment Environment
}

func (r PostgresRoutingRepository) configured(provider ProviderName) bool {
	if r.IsProviderConfigured != nil {
		return r.IsProviderConfigured(provider)
	}
	return r.ConfiguredProviders[provider]
}

// SyncDeploymentEnvironment keeps the non-secret environment marker aligned
// with the process that actually owns provider credentials. The admin UI cannot
// promote sandbox to production: only a production API process with a constructed
// provider adapter can mark that provider as production-ready. Provider enablement
// remains an explicit audited admin action.
func (r PostgresRoutingRepository) SyncDeploymentEnvironment(ctx context.Context) error {
	if r.DB == nil {
		return nil
	}
	for provider := range r.ConfiguredProviders {
		if !r.configured(provider) || provider == ProviderDisabled {
			continue
		}
		if _, err := r.DB.ExecContext(ctx, `UPDATE payment_provider_settings SET environment=$2,updated_at=CASE WHEN environment<>$2 THEN now() ELSE updated_at END WHERE provider=$1`, provider, r.ApplicationEnvironment); err != nil {
			return err
		}
	}
	return nil
}

func (r PostgresRoutingRepository) GetRoute(ctx context.Context, domain Domain) (Route, error) {
	var v Route
	err := r.DB.QueryRowContext(ctx, `SELECT r.domain,r.provider,s.enabled,s.environment,r.updated_at
FROM payment_provider_routes r JOIN payment_provider_settings s ON s.provider=r.provider WHERE r.domain=$1`, domain).
		Scan(&v.Domain, &v.Provider, &v.Enabled, &v.Environment, &v.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Route{}, ErrProviderUnavailable
	}
	if err != nil {
		return Route{}, err
	}
	// Configuration is intentionally deployment-owned. A route may be enabled
	// only when the process constructed the provider adapter successfully.
	v.Configured = r.configured(v.Provider)
	return v, nil
}

func (r PostgresRoutingRepository) ListRoutes(ctx context.Context) ([]Route, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT r.domain,r.provider,s.enabled,s.environment,r.updated_at
FROM payment_provider_routes r JOIN payment_provider_settings s ON s.provider=r.provider ORDER BY r.domain`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Route{}
	for rows.Next() {
		var v Route
		if err := rows.Scan(&v.Domain, &v.Provider, &v.Enabled, &v.Environment, &v.UpdatedAt); err != nil {
			return nil, err
		}
		v.Configured = r.configured(v.Provider)
		items = append(items, v)
	}
	return items, rows.Err()
}

func (r PostgresRoutingRepository) ListProviders(ctx context.Context) ([]ProviderSetting, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT provider,enabled,environment,updated_at FROM payment_provider_settings ORDER BY provider`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ProviderSetting{}
	for rows.Next() {
		var v ProviderSetting
		if err := rows.Scan(&v.Provider, &v.Enabled, &v.Environment, &v.UpdatedAt); err != nil {
			return nil, err
		}
		v.Configured = r.configured(v.Provider)
		items = append(items, v)
	}
	return items, rows.Err()
}

func (r PostgresRoutingRepository) SetProviderEnabled(ctx context.Context, provider ProviderName, enabled bool, actor string) error {
	if enabled && !r.configured(provider) {
		return ErrProviderUnavailable
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE payment_provider_settings SET enabled=$2,updated_by=NULLIF($3,'')::uuid,updated_at=now() WHERE provider=$1`, provider, enabled, actor)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return ErrUnknownProvider
	}
	if err = insertRoutingAuditTx(ctx, tx, actor, "payment_provider.enabled", "PAYMENT_PROVIDER", map[string]any{"provider": provider, "enabled": enabled}); err != nil {
		return err
	}
	return tx.Commit()
}

func insertRoutingAuditTx(ctx context.Context, tx *sql.Tx, actor, action, targetType string, metadata map[string]any) error {
	payload, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_logs(id,actor_user_id,action,target_type,target_id,metadata,ip) VALUES(gen_random_uuid(),NULLIF($1,'')::uuid,$2,$3,NULL,$4::jsonb,NULLIF($5,'')::inet)`, actor, action, targetType, string(payload), requestmeta.FromContext(ctx))
	if err == nil {
		return nil
	}
	// Compatibility fallback for databases created before actor/IP audit hardening.
	// The mutation remains audited instead of failing provider enablement/routing.
	_, fallbackErr := tx.ExecContext(ctx, `INSERT INTO audit_logs(id,action,target_type,target_id,metadata) VALUES(gen_random_uuid(),$1,$2,NULL,$3::jsonb)`, action, targetType, string(payload))
	if fallbackErr != nil {
		return err
	}
	return nil
}

func (r PostgresRoutingRepository) SetRoute(ctx context.Context, domain Domain, provider ProviderName, actor string) (Route, error) {
	var enabled bool
	var environment Environment
	if err := r.DB.QueryRowContext(ctx, `SELECT enabled,environment FROM payment_provider_settings WHERE provider=$1`, provider).Scan(&enabled, &environment); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Route{}, ErrUnknownProvider
		}
		return Route{}, err
	}
	if !enabled || !r.configured(provider) || r.ApplicationEnvironment == EnvironmentProduction && environment != EnvironmentProduction {
		return Route{}, ErrProviderUnavailable
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Route{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO payment_provider_routes(domain,provider,updated_by,updated_at) VALUES($1,$2,NULLIF($3,'')::uuid,now())
ON CONFLICT(domain) DO UPDATE SET provider=excluded.provider,updated_by=excluded.updated_by,updated_at=now()`, domain, provider, actor)
	if err != nil {
		return Route{}, err
	}
	if err = insertRoutingAuditTx(ctx, tx, actor, "payment_provider.route_changed", "PAYMENT_ROUTE", map[string]any{"domain": domain, "provider": provider}); err != nil {
		return Route{}, err
	}
	if err = tx.Commit(); err != nil {
		return Route{}, err
	}
	return r.GetRoute(ctx, domain)
}
