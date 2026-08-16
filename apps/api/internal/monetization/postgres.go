package monetization

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"freelance/apps/api/internal/payments"
	"freelance/apps/api/internal/platform/requestmeta"

	"github.com/jackc/pgx/v5/pgconn"
)

type PostgresRepository struct{ DB *sql.DB }

func (r PostgresRepository) FeatureEnabled(ctx context.Context, key string) (bool, error) {
	var enabled bool
	err := r.DB.QueryRowContext(ctx, `SELECT enabled FROM feature_flags WHERE key=$1`, key).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return enabled, err
}

func (r PostgresRepository) IsAdmin(ctx context.Context, actor string) (bool, error) {
	var ok bool
	err := r.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN user_roles ur ON ur.user_id=u.id WHERE u.id=$1 AND u.status='ACTIVE' AND u.deleted_at IS NULL AND ur.role IN('ADMIN','SUPER_ADMIN'))`, actor).Scan(&ok)
	return ok, err
}

func scanPlan(row interface{ Scan(...any) error }) (Plan, error) {
	var p Plan
	err := row.Scan(&p.ID, &p.Code, &p.Name, &p.Description, &p.Tier, &p.BillingPeriod, &p.Currency, &p.AmountKopecks, &p.Active, &p.DisplayOrder, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

const planColumns = `id::text,code,name,description,tier,billing_period,currency,amount_kopecks,active,display_order,created_at,updated_at`

func (r PostgresRepository) ListPlans(ctx context.Context, activeOnly bool) ([]Plan, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT `+planColumns+` FROM subscription_plans WHERE ($1=false OR active=true) ORDER BY display_order,created_at,id`, activeOnly)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Plan{}
	ids := []string{}
	for rows.Next() {
		p, e := scanPlan(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, p)
		ids = append(ids, p.ID)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return items, nil
	}
	erows, err := r.DB.QueryContext(ctx, `SELECT plan_id::text,feature_key,kind,enabled,limit_value,unlimited,config::text FROM subscription_plan_entitlements WHERE plan_id=ANY($1::uuid[]) ORDER BY feature_key`, `{`+strings.Join(ids, `,`)+`}`)
	if err != nil {
		return nil, err
	}
	defer erows.Close()
	byID := map[string][]Entitlement{}
	for erows.Next() {
		var id, raw string
		var v Entitlement
		if err = erows.Scan(&id, &v.FeatureKey, &v.Kind, &v.Enabled, &v.LimitValue, &v.Unlimited, &raw); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(raw), &v.Config)
		if v.Config == nil {
			v.Config = map[string]any{}
		}
		byID[id] = append(byID[id], v)
	}
	if err = erows.Err(); err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Entitlements = byID[items[i].ID]
	}
	return items, nil
}

func (r PostgresRepository) GetPlan(ctx context.Context, id string) (Plan, error) {
	p, err := scanPlan(r.DB.QueryRowContext(ctx, `SELECT `+planColumns+` FROM subscription_plans WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Plan{}, ErrNotFound
	}
	if err != nil {
		return Plan{}, err
	}
	p.Entitlements, err = r.PlanEntitlements(ctx, p.ID)
	return p, err
}

func (r PostgresRepository) PlanEntitlements(ctx context.Context, planID string) ([]Entitlement, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT feature_key,kind,enabled,limit_value,unlimited,config::text FROM subscription_plan_entitlements WHERE plan_id=$1 ORDER BY feature_key`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entitlement{}
	for rows.Next() {
		var v Entitlement
		var raw string
		if err = rows.Scan(&v.FeatureKey, &v.Kind, &v.Enabled, &v.LimitValue, &v.Unlimited, &raw); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(raw), &v.Config)
		if v.Config == nil {
			v.Config = map[string]any{}
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

const subscriptionColumns = `s.id::text,s.user_id::text,COALESCE(u.display_name,''),s.plan_id::text,p.code,p.name,s.status,s.starts_at,s.current_period_start,s.current_period_end,s.cancel_at_period_end,s.canceled_at,COALESCE(s.provider,''),COALESCE(s.provider_customer_id,''),COALESCE(s.provider_subscription_id,''),COALESCE(s.payment_method_ref,''),s.next_retry_at,s.created_at,s.updated_at`

func scanSubscription(row interface{ Scan(...any) error }) (Subscription, error) {
	var s Subscription
	err := row.Scan(&s.ID, &s.UserID, &s.UserName, &s.PlanID, &s.PlanCode, &s.PlanName, &s.Status, &s.StartsAt, &s.CurrentPeriodStart, &s.CurrentPeriodEnd, &s.CancelAtPeriodEnd, &s.CanceledAt, &s.Provider, &s.ProviderCustomerID, &s.ProviderSubscriptionID, &s.PaymentMethodRef, &s.NextRetryAt, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

func (r PostgresRepository) CurrentSubscription(ctx context.Context, userID string) (*Subscription, error) {
	s, err := scanSubscription(r.DB.QueryRowContext(ctx, `SELECT `+subscriptionColumns+` FROM user_subscriptions s JOIN subscription_plans p ON p.id=s.plan_id JOIN users u ON u.id=s.user_id WHERE s.user_id=$1 AND s.status IN('PENDING','ACTIVE','PAST_DUE') ORDER BY s.created_at DESC LIMIT 1`, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}
func (r PostgresRepository) SubscriptionHistory(ctx context.Context, userID string) ([]Subscription, error) {
	return r.listSubscriptions(ctx, `s.user_id=$1`, userID, 100)
}
func (r PostgresRepository) ListSubscriptions(ctx context.Context, status string, limit int) ([]Subscription, error) {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	return r.listSubscriptions(ctx, `($1='' OR s.status=$1)`, status, limit)
}
func (r PostgresRepository) listSubscriptions(ctx context.Context, where string, arg any, limit int) ([]Subscription, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT `+subscriptionColumns+` FROM user_subscriptions s JOIN subscription_plans p ON p.id=s.plan_id JOIN users u ON u.id=s.user_id WHERE `+where+` ORDER BY s.created_at DESC,s.id DESC LIMIT $2`, arg, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Subscription{}
	for rows.Next() {
		v, e := scanSubscription(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r PostgresRepository) SubscriptionEvents(ctx context.Context, id string) ([]Event, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT id::text,subscription_id::text,event_type,COALESCE(from_status,''),COALESCE(to_status,''),COALESCE(actor_user_id::text,''),COALESCE(reason,''),created_at FROM subscription_events WHERE subscription_id=$1 ORDER BY created_at DESC,id DESC LIMIT 200`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var v Event
		if err = rows.Scan(&v.ID, &v.SubscriptionID, &v.EventType, &v.FromStatus, &v.ToStatus, &v.ActorUserID, &v.Reason, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r PostgresRepository) Overview(ctx context.Context) (Overview, error) {
	var o Overview
	err := r.DB.QueryRowContext(ctx, `SELECT COALESCE((SELECT enabled FROM feature_flags WHERE key='pro_subscriptions_enabled'),false),(SELECT count(*) FROM user_subscriptions WHERE status='ACTIVE' AND starts_at<=now() AND current_period_end>now()),(SELECT count(*) FROM user_subscriptions WHERE created_at>=now()-interval '30 days'),(SELECT count(*) FROM user_subscriptions WHERE status='ACTIVE' AND current_period_end>now() AND current_period_end<=now()+interval '7 days')`).Scan(&o.ProSystemEnabled, &o.ActiveCount, &o.New30Days, &o.Expiring7Days)
	o.ProviderConnected = false
	return o, err
}

func auditSubscription(ctx context.Context, tx *sql.Tx, actor, action, target, reason, requestID string, meta map[string]any) error {
	if meta == nil {
		meta = map[string]any{}
	}
	meta["reason"] = reason
	meta["request_id"] = requestID
	raw, _ := json.Marshal(meta)
	_, err := tx.ExecContext(ctx, `INSERT INTO audit_logs(id,actor_user_id,action,target_type,target_id,metadata,ip) VALUES(gen_random_uuid(),$1,$2,'SUBSCRIPTION',$3,$4::jsonb,NULLIF($5,'')::inet)`, actor, action, target, raw, requestmeta.FromContext(ctx))
	return err
}

func (r PostgresRepository) Grant(ctx context.Context, actor, userID, planID string, startsAt, endsAt time.Time, reason, requestID string) (Subscription, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Subscription{}, err
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(ctx, `INSERT INTO user_subscriptions(user_id,plan_id,status,starts_at,current_period_start,current_period_end) SELECT u.id,p.id,'ACTIVE',$3,$3,$4 FROM users u,subscription_plans p WHERE u.id=$1 AND u.status='ACTIVE' AND u.deleted_at IS NULL AND p.id=$2 AND p.active AND p.tier='PRO' RETURNING id::text`, userID, planID, startsAt, endsAt).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return Subscription{}, ErrNotFound
	}
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			return Subscription{}, ErrConflict
		}
		return Subscription{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO subscription_events(subscription_id,event_type,to_status,actor_user_id,reason,metadata) VALUES($1,'ADMIN_GRANTED','ACTIVE',$2,$3,'{}')`, id, actor, reason); err != nil {
		return Subscription{}, err
	}
	if err = auditSubscription(ctx, tx, actor, "subscription.granted", id, reason, requestID, map[string]any{"user_id": userID, "plan_id": planID, "ends_at": endsAt}); err != nil {
		return Subscription{}, err
	}
	if err = tx.Commit(); err != nil {
		return Subscription{}, err
	}
	return r.getSubscription(ctx, id)
}

func (r PostgresRepository) Transition(ctx context.Context, actor, id, status, reason, requestID string) (Subscription, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Subscription{}, err
	}
	defer tx.Rollback()
	var from string
	err = tx.QueryRowContext(ctx, `SELECT status FROM user_subscriptions WHERE id=$1 FOR UPDATE`, id).Scan(&from)
	if errors.Is(err, sql.ErrNoRows) {
		return Subscription{}, ErrNotFound
	}
	if err != nil {
		return Subscription{}, err
	}
	if from == status {
		return Subscription{}, ErrConflict
	}
	if from == "CANCELED" || from == "EXPIRED" {
		return Subscription{}, ErrConflict
	}
	_, err = tx.ExecContext(ctx, `UPDATE user_subscriptions SET status=$2,canceled_at=now(),cancel_at_period_end=false,updated_at=now() WHERE id=$1`, id, status)
	if err != nil {
		return Subscription{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO subscription_events(subscription_id,event_type,from_status,to_status,actor_user_id,reason,metadata) VALUES($1,$2,$3,$4,$5,$6,'{}')`, id, "ADMIN_"+status, from, status, actor, reason); err != nil {
		return Subscription{}, err
	}
	if err = auditSubscription(ctx, tx, actor, "subscription."+strings.ToLower(status), id, reason, requestID, map[string]any{"from_status": from}); err != nil {
		return Subscription{}, err
	}
	if err = tx.Commit(); err != nil {
		return Subscription{}, err
	}
	return r.getSubscription(ctx, id)
}
func (r PostgresRepository) getSubscription(ctx context.Context, id string) (Subscription, error) {
	s, err := scanSubscription(r.DB.QueryRowContext(ctx, `SELECT `+subscriptionColumns+` FROM user_subscriptions s JOIN subscription_plans p ON p.id=s.plan_id JOIN users u ON u.id=s.user_id WHERE s.id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Subscription{}, ErrNotFound
	}
	return s, err
}

func (r PostgresRepository) UpdatePlan(ctx context.Context, actor string, p Plan, reason, requestID string) (Plan, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Plan{}, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE subscription_plans SET name=$2,description=$3,amount_kopecks=$4,active=$5,display_order=$6,updated_at=now() WHERE id=$1`, p.ID, p.Name, p.Description, p.AmountKopecks, p.Active, p.DisplayOrder)
	if err != nil {
		return Plan{}, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return Plan{}, ErrNotFound
	}
	if err = auditSubscription(ctx, tx, actor, "subscription_plan.updated", p.ID, reason, requestID, map[string]any{"amount_kopecks": p.AmountKopecks, "active": p.Active}); err != nil {
		return Plan{}, err
	}
	if err = tx.Commit(); err != nil {
		return Plan{}, err
	}
	return r.GetPlan(ctx, p.ID)
}
func (r PostgresRepository) SetEntitlement(ctx context.Context, actor, planID string, v Entitlement, reason, requestID string) (Entitlement, error) {
	raw, err := json.Marshal(v.Config)
	if err != nil {
		return Entitlement{}, ErrInvalid
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Entitlement{}, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO subscription_plan_entitlements(plan_id,feature_key,kind,enabled,limit_value,unlimited,config) SELECT id,$2,$3,$4,$5,$6,$7::jsonb FROM subscription_plans WHERE id=$1 ON CONFLICT(plan_id,feature_key) DO UPDATE SET kind=excluded.kind,enabled=excluded.enabled,limit_value=excluded.limit_value,unlimited=excluded.unlimited,config=excluded.config,updated_at=now()`, planID, v.FeatureKey, v.Kind, v.Enabled, v.LimitValue, v.Unlimited, raw)
	if err != nil {
		return Entitlement{}, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return Entitlement{}, ErrNotFound
	}
	if err = auditSubscription(ctx, tx, actor, "subscription_plan.entitlement_updated", planID, reason, requestID, map[string]any{"feature_key": v.FeatureKey}); err != nil {
		return Entitlement{}, err
	}
	if err = tx.Commit(); err != nil {
		return Entitlement{}, err
	}
	items, err := r.PlanEntitlements(ctx, planID)
	if err != nil {
		return Entitlement{}, err
	}
	for _, item := range items {
		if item.FeatureKey == v.FeatureKey {
			return item, nil
		}
	}
	return Entitlement{}, ErrNotFound
}

func (r PostgresRepository) CreatePendingSubscription(ctx context.Context, userID string, p Plan, provider payments.ProviderName, startsAt, endsAt time.Time) (Subscription, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Subscription{}, err
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(ctx, `INSERT INTO user_subscriptions(user_id,plan_id,status,starts_at,current_period_start,current_period_end,provider)
SELECT u.id,p.id,'PENDING',$3,$3,$4,$5 FROM users u,subscription_plans p
WHERE u.id=$1 AND u.status='ACTIVE' AND u.deleted_at IS NULL AND p.id=$2 AND p.active AND p.tier='PRO'
ON CONFLICT DO NOTHING RETURNING id::text`, userID, p.ID, startsAt, endsAt, string(provider)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		var existing string
		if e := tx.QueryRowContext(ctx, `SELECT id::text FROM user_subscriptions WHERE user_id=$1 AND status IN('PENDING','ACTIVE','PAST_DUE') ORDER BY created_at DESC LIMIT 1`, userID).Scan(&existing); e != nil {
			return Subscription{}, ErrConflict
		}
		if e := tx.Commit(); e != nil {
			return Subscription{}, e
		}
		return r.getSubscription(ctx, existing)
	}
	if err != nil {
		return Subscription{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO subscription_events(subscription_id,event_type,to_status,reason,metadata) VALUES($1,'PAYMENT_STARTED','PENDING','provider checkout started','{}')`, id)
	if err != nil {
		return Subscription{}, err
	}
	if err = tx.Commit(); err != nil {
		return Subscription{}, err
	}
	return r.getSubscription(ctx, id)
}

func (r PostgresRepository) SavePaymentMethod(ctx context.Context, subscriptionID, provider, ref string) error {
	if ref == "" {
		return nil
	}
	_, err := r.DB.ExecContext(ctx, `UPDATE user_subscriptions SET provider=$2,payment_method_ref=$3,updated_at=now() WHERE id=$1`, subscriptionID, provider, ref)
	return err
}
func (r PostgresRepository) ActivatePaidSubscription(ctx context.Context, id, attemptID string, start, end time.Time, provider string) (Subscription, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Subscription{}, err
	}
	defer tx.Rollback()
	var from string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM user_subscriptions WHERE id=$1 FOR UPDATE`, id).Scan(&from); err != nil {
		return Subscription{}, err
	}
	if from == "ACTIVE" {
		_ = tx.Commit()
		return r.getSubscription(ctx, id)
	}
	if from != "PENDING" && from != "PAST_DUE" {
		return Subscription{}, ErrConflict
	}
	_, err = tx.ExecContext(ctx, `UPDATE user_subscriptions SET status='ACTIVE',starts_at=LEAST(starts_at,$3),current_period_start=$3,current_period_end=$4,provider=$5,last_payment_attempt_id=$2,next_retry_at=NULL,past_due_since=NULL,updated_at=now() WHERE id=$1`, id, attemptID, start, end, provider)
	if err != nil {
		return Subscription{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO subscription_billing_periods(subscription_id,period_start,period_end,payment_attempt_id,status) VALUES($1,$2,$3,$4,'SUCCEEDED') ON CONFLICT(subscription_id,period_start) DO NOTHING`, id, start, end, attemptID)
	if err != nil {
		return Subscription{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO subscription_events(subscription_id,event_type,from_status,to_status,reason,metadata) VALUES($1,'PAYMENT_SUCCEEDED',$2,'ACTIVE','authoritative provider payment succeeded',jsonb_build_object('payment_attempt_id',$3::text))`, id, from, attemptID)
	if err != nil {
		return Subscription{}, err
	}
	if err = tx.Commit(); err != nil {
		return Subscription{}, err
	}
	return r.getSubscription(ctx, id)
}
func (r PostgresRepository) RenewPaidSubscription(ctx context.Context, id, attemptID string, start, end time.Time) (Subscription, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Subscription{}, err
	}
	defer tx.Rollback()
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM user_subscriptions WHERE id=$1 FOR UPDATE`, id).Scan(&status); err != nil {
		return Subscription{}, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO subscription_billing_periods(subscription_id,period_start,period_end,payment_attempt_id,status) VALUES($1,$2,$3,$4,'SUCCEEDED') ON CONFLICT(subscription_id,period_start) DO NOTHING`, id, start, end, attemptID)
	if err != nil {
		return Subscription{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		_ = tx.Commit()
		return r.getSubscription(ctx, id)
	}
	_, err = tx.ExecContext(ctx, `UPDATE user_subscriptions SET status='ACTIVE',current_period_start=$2,current_period_end=$3,last_payment_attempt_id=$4,next_retry_at=NULL,past_due_since=NULL,updated_at=now() WHERE id=$1`, id, start, end, attemptID)
	if err != nil {
		return Subscription{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO subscription_events(subscription_id,event_type,from_status,to_status,reason,metadata) VALUES($1,'RENEWAL_SUCCEEDED',$2,'ACTIVE','authoritative recurring payment succeeded',jsonb_build_object('payment_attempt_id',$3::text))`, id, status, attemptID)
	if err != nil {
		return Subscription{}, err
	}
	if err = tx.Commit(); err != nil {
		return Subscription{}, err
	}
	return r.getSubscription(ctx, id)
}
func (r PostgresRepository) MarkSubscriptionPastDue(ctx context.Context, id, attemptID string) error {
	_, err := r.DB.ExecContext(ctx, `WITH current AS (SELECT id,status FROM user_subscriptions WHERE id=$1 AND status IN('ACTIVE','PAST_DUE') FOR UPDATE), updated AS (UPDATE user_subscriptions s SET status='PAST_DUE',last_payment_attempt_id=$2,next_retry_at=now()+interval '1 day',past_due_since=COALESCE(past_due_since,now()),updated_at=now() FROM current c WHERE s.id=c.id RETURNING s.id,c.status AS previous_status) INSERT INTO subscription_events(subscription_id,event_type,from_status,to_status,reason,metadata) SELECT id,'RENEWAL_FAILED',previous_status,'PAST_DUE','recurring payment failed',jsonb_build_object('payment_attempt_id',$2::text) FROM updated`, id, attemptID)
	return err
}
func (r PostgresRepository) FailInitialSubscription(ctx context.Context, id, attemptID, reason string) error {
	if strings.TrimSpace(reason) == "" {
		reason = "initial provider payment failed"
	}
	_, err := r.DB.ExecContext(ctx, `WITH current AS (
 SELECT id,status FROM user_subscriptions WHERE id=$1 AND status='PENDING' FOR UPDATE
), updated AS (
 UPDATE user_subscriptions s SET status='CANCELED',canceled_at=now(),last_payment_attempt_id=$2,renewal_claimed_at=NULL,updated_at=now()
 FROM current c WHERE s.id=c.id RETURNING s.id,c.status AS previous_status
)
INSERT INTO subscription_events(subscription_id,event_type,from_status,to_status,reason,metadata)
SELECT id,'PAYMENT_FAILED',previous_status,'CANCELED',$3,jsonb_build_object('payment_attempt_id',$2::text) FROM updated`, id, attemptID, reason)
	return err
}
func (r PostgresRepository) SetCancelAtPeriodEnd(ctx context.Context, userID string, value bool) (Subscription, error) {
	var id string
	err := r.DB.QueryRowContext(ctx, `UPDATE user_subscriptions SET cancel_at_period_end=$2,canceled_at=NULL,updated_at=now() WHERE id=(SELECT id FROM user_subscriptions WHERE user_id=$1 AND status IN('ACTIVE','PAST_DUE') ORDER BY created_at DESC LIMIT 1) RETURNING id::text`, userID, value).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return Subscription{}, ErrNotFound
	}
	if err != nil {
		return Subscription{}, err
	}
	return r.getSubscription(ctx, id)
}
func (r PostgresRepository) GetSubscriptionForBilling(ctx context.Context, id string) (Subscription, error) {
	return r.getSubscription(ctx, id)
}
func (r PostgresRepository) UserOwnsSubscription(ctx context.Context, userID, subscriptionID string) (bool, error) {
	var ok bool
	err := r.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_subscriptions WHERE id=$2 AND user_id=$1)`, userID, subscriptionID).Scan(&ok)
	return ok, err
}
func (r PostgresRepository) ListBillingAttempts(ctx context.Context, userID string, limit int) ([]payments.Attempt, error) {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT p.id::text,p.domain,p.internal_reference_id::text,p.provider,p.operation_type,p.status,p.amount_kopecks,p.currency,p.idempotency_key,COALESCE(p.payment_method,''),COALESCE(p.provider_operation_id,''),COALESCE(p.provider_payment_method_ref,''),COALESCE(p.provider_confirmation_url,''),COALESCE(p.provider_raw_status,''),COALESCE(p.error_category,''),p.reconciliation_state,p.created_at,p.updated_at,p.terminal_at FROM payment_attempts p JOIN user_subscriptions s ON s.id=p.internal_reference_id WHERE p.domain='PRO_SUBSCRIPTION' AND s.user_id=$1 ORDER BY p.created_at DESC,p.id DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []payments.Attempt{}
	for rows.Next() {
		var v payments.Attempt
		var terminal sql.NullTime
		if err := rows.Scan(&v.ID, &v.Domain, &v.InternalReferenceID, &v.Provider, &v.OperationType, &v.Status, &v.AmountKopecks, &v.Currency, &v.IdempotencyKey, &v.PaymentMethod, &v.ProviderOperationID, &v.ProviderPaymentMethodRef, &v.ConfirmationURL, &v.ProviderRawStatus, &v.ErrorCategory, &v.ReconciliationState, &v.CreatedAt, &v.UpdatedAt, &terminal); err != nil {
			return nil, err
		}
		if terminal.Valid {
			t := terminal.Time.UTC()
			v.TerminalAt = &t
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r PostgresRepository) ClaimDueRenewals(ctx context.Context, limit int, now time.Time) ([]Subscription, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := r.DB.QueryContext(ctx, `WITH due AS (
 SELECT s.id FROM user_subscriptions s
 WHERE s.status IN('ACTIVE','PAST_DUE') AND NOT s.cancel_at_period_end
   AND s.payment_method_ref IS NOT NULL AND s.payment_method_ref<>''
   AND ((s.status='ACTIVE' AND s.current_period_end <= $1 + interval '15 minutes') OR (s.status='PAST_DUE' AND COALESCE(s.next_retry_at,s.current_period_end) <= $1))
   AND (s.renewal_claimed_at IS NULL OR s.renewal_claimed_at < $1-interval '5 minutes')
 ORDER BY COALESCE(s.next_retry_at,s.current_period_end),s.id LIMIT $2 FOR UPDATE SKIP LOCKED
), claimed AS (
 UPDATE user_subscriptions s SET renewal_claimed_at=$1,updated_at=now() FROM due WHERE s.id=due.id RETURNING s.id
)
SELECT `+subscriptionColumns+` FROM user_subscriptions s JOIN claimed c ON c.id=s.id JOIN subscription_plans p ON p.id=s.plan_id JOIN users u ON u.id=s.user_id
ORDER BY COALESCE(s.next_retry_at,s.current_period_end),s.id`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Subscription{}
	for rows.Next() {
		v, e := scanSubscription(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r PostgresRepository) ExpireDueSubscriptions(ctx context.Context, limit int, now time.Time) (int, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `WITH due AS (
 SELECT id,status FROM user_subscriptions
 WHERE (status IN('ACTIVE','PAST_DUE') AND cancel_at_period_end AND current_period_end <= $1)
    OR (status='PAST_DUE' AND past_due_since IS NOT NULL AND past_due_since <= $1-interval '3 days')
 ORDER BY COALESCE(past_due_since,current_period_end),id LIMIT $2 FOR UPDATE SKIP LOCKED
), updated AS (
 UPDATE user_subscriptions s SET status='EXPIRED',canceled_at=COALESCE(canceled_at,$1),renewal_claimed_at=NULL,next_retry_at=NULL,updated_at=now()
 FROM due d WHERE s.id=d.id RETURNING s.id,d.status AS previous_status
)
SELECT id::text,previous_status FROM updated`, now, limit)
	if err != nil {
		return 0, err
	}
	type expired struct{ id, from string }
	items := []expired{}
	for rows.Next() {
		var item expired
		if err := rows.Scan(&item.id, &item.from); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	for _, item := range items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_events(subscription_id,event_type,from_status,to_status,reason,metadata) VALUES($1,'PERIOD_ENDED',$2,'EXPIRED','paid period ended or renewal grace expired','{}')`, item.id, item.from); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(items), nil
}

func (r PostgresRepository) ReleaseRenewalClaim(ctx context.Context, id string) error {
	_, err := r.DB.ExecContext(ctx, `UPDATE user_subscriptions SET renewal_claimed_at=NULL,updated_at=now() WHERE id=$1`, id)
	return err
}
