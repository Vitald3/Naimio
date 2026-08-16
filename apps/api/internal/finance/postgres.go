package finance

import (
	"context"
	"database/sql"
	"encoding/json"
	"freelance/apps/api/internal/platform/requestmeta"
)

type PostgresRepository struct{ DB *sql.DB }

// Roles returns the active roles of the actor. Mirrors admin.PostgresRepository
// so finance can authorize independently.
func (r PostgresRepository) Roles(ctx context.Context, actor string) ([]string, error) {
	if r.DB == nil || actor == "" {
		return nil, ErrForbidden
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT role FROM user_roles ur JOIN users u ON u.id=ur.user_id WHERE ur.user_id=$1 AND u.status='ACTIVE' AND u.deleted_at IS NULL ORDER BY role`, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// audit writes one immutable audit_logs entry inside the given transaction.
func audit(ctx context.Context, tx *sql.Tx, actor, action, targetType, reason, requestID string, extra map[string]any) error {
	if extra == nil {
		extra = map[string]any{}
	}
	if reason != "" {
		extra["reason"] = reason
	}
	if requestID != "" {
		extra["request_id"] = requestID
	}
	payload, err := json.Marshal(extra)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_logs(id,actor_user_id,action,target_type,target_id,metadata,ip)VALUES(gen_random_uuid(),$1,$2,$3,NULL,$4::jsonb,NULLIF($5,'')::inet)`, actor, action, targetType, payload, requestmeta.FromContext(ctx))
	return err
}

func nullableMax(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func (r PostgresRepository) ListFeeRules(ctx context.Context) ([]FeeRule, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT version,commission_basis_points,minimum_fee_kopecks,maximum_fee_kopecks,platform_fee_payer_mode,platform_customer_share_basis_points,provider_fee_payer_mode,provider_customer_share_basis_points,enabled,effective_from,created_at FROM safe_deal_fee_rules ORDER BY version DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FeeRule{}
	for rows.Next() {
		var v FeeRule
		var max sql.NullInt64
		if err := rows.Scan(&v.Version, &v.CommissionBasisPoints, &v.MinimumFeeKopecks, &max, &v.PlatformFeePayerMode, &v.PlatformCustomerShareBasisPoints, &v.ProviderFeePayerMode, &v.ProviderCustomerShareBasisPoints, &v.Enabled, &v.EffectiveFrom, &v.CreatedAt); err != nil {
			return nil, err
		}
		if max.Valid {
			m := max.Int64
			v.MaximumFeeKopecks = &m
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// CreateFeeRule inserts a NEW versioned fee rule and makes it the active one.
// It never mutates the economic content of a historical rule: it only flips
// the previously-enabled rule's `enabled` flag off (the single-active-rule
// invariant is enforced by a partial unique index) and inserts a fresh
// version. Existing deals snapshot their own economics and are untouched.
func (r PostgresRepository) CreateFeeRule(ctx context.Context, actor string, in FeeRule, reason, requestID string) (FeeRule, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return FeeRule{}, err
	}
	defer tx.Rollback()

	var version int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0)+1 FROM safe_deal_fee_rules`).Scan(&version); err != nil {
		return FeeRule{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE safe_deal_fee_rules SET enabled=false WHERE enabled`); err != nil {
		return FeeRule{}, err
	}
	var out FeeRule
	var max sql.NullInt64
	err = tx.QueryRowContext(ctx, `INSERT INTO safe_deal_fee_rules(version,commission_basis_points,minimum_fee_kopecks,maximum_fee_kopecks,platform_fee_payer_mode,platform_customer_share_basis_points,provider_fee_payer_mode,provider_customer_share_basis_points,enabled,effective_from)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,true,now())
RETURNING version,commission_basis_points,minimum_fee_kopecks,maximum_fee_kopecks,platform_fee_payer_mode,platform_customer_share_basis_points,provider_fee_payer_mode,provider_customer_share_basis_points,enabled,effective_from,created_at`,
		version, in.CommissionBasisPoints, in.MinimumFeeKopecks, nullableMax(in.MaximumFeeKopecks), in.PlatformFeePayerMode, in.PlatformCustomerShareBasisPoints, in.ProviderFeePayerMode, in.ProviderCustomerShareBasisPoints).
		Scan(&out.Version, &out.CommissionBasisPoints, &out.MinimumFeeKopecks, &max, &out.PlatformFeePayerMode, &out.PlatformCustomerShareBasisPoints, &out.ProviderFeePayerMode, &out.ProviderCustomerShareBasisPoints, &out.Enabled, &out.EffectiveFrom, &out.CreatedAt)
	if err != nil {
		return FeeRule{}, err
	}
	if max.Valid {
		m := max.Int64
		out.MaximumFeeKopecks = &m
	}
	if err := audit(ctx, tx, actor, "finance.fee_rule.created", "SAFE_DEAL_FEE_RULE", reason, requestID, map[string]any{
		"version":                 out.Version,
		"commission_basis_points": out.CommissionBasisPoints,
		"platform_fee_payer_mode": out.PlatformFeePayerMode,
		"provider_fee_payer_mode": out.ProviderFeePayerMode,
	}); err != nil {
		return FeeRule{}, err
	}
	if err := tx.Commit(); err != nil {
		return FeeRule{}, err
	}
	return out, nil
}

func (r PostgresRepository) ListProviderPricing(ctx context.Context) ([]ProviderPricingRule, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT version,provider,payment_method,percent_basis_points,fixed_fee_kopecks,minimum_fee_kopecks,maximum_fee_kopecks,enabled,effective_from,created_at FROM safe_deal_provider_pricing ORDER BY provider,payment_method,version DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProviderPricingRule{}
	for rows.Next() {
		var v ProviderPricingRule
		var max sql.NullInt64
		if err := rows.Scan(&v.Version, &v.Provider, &v.PaymentMethod, &v.PercentBasisPoints, &v.FixedFeeKopecks, &v.MinimumFeeKopecks, &max, &v.Enabled, &v.EffectiveFrom, &v.CreatedAt); err != nil {
			return nil, err
		}
		if max.Valid {
			m := max.Int64
			v.MaximumFeeKopecks = &m
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// CreateProviderPricing inserts a NEW versioned pricing row for the given
// provider + payment method and makes it the active one, deactivating any
// previously-enabled row for that same provider + method. Historical pricing
// rows are never mutated.
func (r PostgresRepository) CreateProviderPricing(ctx context.Context, actor string, in ProviderPricingRule, reason, requestID string) (ProviderPricingRule, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return ProviderPricingRule{}, err
	}
	defer tx.Rollback()

	var version int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0)+1 FROM safe_deal_provider_pricing WHERE provider=$1 AND payment_method=$2`, in.Provider, in.PaymentMethod).Scan(&version); err != nil {
		return ProviderPricingRule{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE safe_deal_provider_pricing SET enabled=false WHERE enabled AND provider=$1 AND payment_method=$2`, in.Provider, in.PaymentMethod); err != nil {
		return ProviderPricingRule{}, err
	}
	var out ProviderPricingRule
	var max sql.NullInt64
	err = tx.QueryRowContext(ctx, `INSERT INTO safe_deal_provider_pricing(version,provider,payment_method,percent_basis_points,fixed_fee_kopecks,minimum_fee_kopecks,maximum_fee_kopecks,enabled,effective_from)
VALUES($1,$2,$3,$4,$5,$6,$7,true,now())
RETURNING version,provider,payment_method,percent_basis_points,fixed_fee_kopecks,minimum_fee_kopecks,maximum_fee_kopecks,enabled,effective_from,created_at`,
		version, in.Provider, in.PaymentMethod, in.PercentBasisPoints, in.FixedFeeKopecks, in.MinimumFeeKopecks, nullableMax(in.MaximumFeeKopecks)).
		Scan(&out.Version, &out.Provider, &out.PaymentMethod, &out.PercentBasisPoints, &out.FixedFeeKopecks, &out.MinimumFeeKopecks, &max, &out.Enabled, &out.EffectiveFrom, &out.CreatedAt)
	if err != nil {
		return ProviderPricingRule{}, err
	}
	if max.Valid {
		m := max.Int64
		out.MaximumFeeKopecks = &m
	}
	if err := audit(ctx, tx, actor, "finance.provider_pricing.created", "SAFE_DEAL_PROVIDER_PRICING", reason, requestID, map[string]any{
		"version":        out.Version,
		"provider":       out.Provider,
		"payment_method": out.PaymentMethod,
	}); err != nil {
		return ProviderPricingRule{}, err
	}
	if err := tx.Commit(); err != nil {
		return ProviderPricingRule{}, err
	}
	return out, nil
}
