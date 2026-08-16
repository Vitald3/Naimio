package payments

import (
	"context"
	"database/sql"
	"errors"
	"freelance/apps/api/internal/platform/requestmeta"
	"strings"
	"time"
)

type PostgresRepository struct{ DB *sql.DB }

func (r PostgresRepository) Create(ctx context.Context, in Attempt) (Attempt, bool, error) {
	const insert = `INSERT INTO payment_attempts(domain,internal_reference_id,provider,operation_type,status,amount_kopecks,currency,idempotency_key,payment_method,provider_operation_id,provider_payment_method_ref,provider_confirmation_url,provider_raw_status,error_category,reconciliation_state,terminal_at,created_at,updated_at)
VALUES($1,$2::uuid,$3,$4,$5,$6,$7,$8,NULLIF($9,''),NULLIF($10,''),NULLIF($11,''),NULLIF($12,''),NULLIF($13,''),NULLIF($14,''),$15,$16,$17,$18)
ON CONFLICT(domain,internal_reference_id,operation_type,idempotency_key) DO NOTHING
RETURNING id::text,created_at,updated_at`
	var id string
	err := r.DB.QueryRowContext(ctx, insert, in.Domain, in.InternalReferenceID, in.Provider, in.OperationType, in.Status, in.AmountKopecks, in.Currency, in.IdempotencyKey, in.PaymentMethod, in.ProviderOperationID, in.ProviderPaymentMethodRef, in.ConfirmationURL, in.ProviderRawStatus, in.ErrorCategory, in.ReconciliationState, in.TerminalAt, in.CreatedAt, in.UpdatedAt).Scan(&id, &in.CreatedAt, &in.UpdatedAt)
	if err == nil {
		in.ID = id
		return in, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		// A second concurrent PRO checkout may hit the partial one-open-purchase
		// unique index before it can observe the first attempt. Return the pinned
		// open attempt so Service.Create can enforce idempotency-key equality.
		if in.Domain == DomainProSubscription && in.OperationType == OperationPayment {
			if existing, ok, lookupErr := r.FindOpenPROPurchase(ctx, in.InternalReferenceID); lookupErr == nil && ok {
				return existing, false, nil
			}
		}
		if in.Domain == DomainProSubscription && in.OperationType == OperationRenewal {
			if existing, ok, lookupErr := r.FindOpenPRORenewal(ctx, in.InternalReferenceID); lookupErr == nil && ok {
				return existing, false, nil
			}
		}
		if in.Domain == DomainSafeDeal && (in.OperationType == OperationPayment || in.OperationType == OperationRefund || in.OperationType == OperationPayout) {
			if existing, ok, lookupErr := r.FindOpenSafeDealOperation(ctx, in.InternalReferenceID, in.OperationType); lookupErr == nil && ok {
				return existing, false, nil
			}
		}
		return Attempt{}, false, err
	}
	return r.FindByIdempotencyKey(ctx, in.Domain, in.InternalReferenceID, in.OperationType, in.IdempotencyKey)
}

func (r PostgresRepository) Get(ctx context.Context, id string) (Attempt, error) {
	return r.one(ctx, `WHERE id=$1::uuid`, id)
}
func (r PostgresRepository) FindByIdempotencyKey(ctx context.Context, domain Domain, reference string, operation OperationType, key string) (Attempt, bool, error) {
	v, err := r.one(ctx, `WHERE domain=$1 AND internal_reference_id=$2::uuid AND operation_type=$3 AND idempotency_key=$4`, domain, reference, operation, key)
	if errors.Is(err, sql.ErrNoRows) {
		return Attempt{}, false, nil
	}
	return v, true, err
}
func (r PostgresRepository) FindByProviderExternalID(ctx context.Context, provider ProviderName, externalID string) (Attempt, bool, error) {
	v, err := r.one(ctx, `WHERE provider=$1 AND provider_operation_id=$2`, provider, externalID)
	if errors.Is(err, sql.ErrNoRows) {
		return Attempt{}, false, nil
	}
	return v, true, err
}
func (r PostgresRepository) FindOpenPROPurchase(ctx context.Context, subscriptionID string) (Attempt, bool, error) {
	v, err := r.one(ctx, `WHERE domain='PRO_SUBSCRIPTION' AND internal_reference_id=$1::uuid AND operation_type='PAYMENT' AND status IN('CREATED','PENDING_USER_ACTION','PROCESSING','AUTHORIZED','UNKNOWN_REQUIRES_RECONCILIATION') ORDER BY created_at DESC LIMIT 1`, subscriptionID)
	if errors.Is(err, sql.ErrNoRows) {
		return Attempt{}, false, nil
	}
	return v, true, err
}

func (r PostgresRepository) FindOpenPRORenewal(ctx context.Context, subscriptionID string) (Attempt, bool, error) {
	v, err := r.one(ctx, `WHERE domain='PRO_SUBSCRIPTION' AND internal_reference_id=$1::uuid AND operation_type='RENEWAL' AND status IN('CREATED','PENDING_USER_ACTION','PROCESSING','AUTHORIZED','UNKNOWN_REQUIRES_RECONCILIATION') ORDER BY created_at DESC LIMIT 1`, subscriptionID)
	if errors.Is(err, sql.ErrNoRows) {
		return Attempt{}, false, nil
	}
	return v, true, err
}

func (r PostgresRepository) FindOpenSafeDealOperation(ctx context.Context, dealID string, operation OperationType) (Attempt, bool, error) {
	v, err := r.one(ctx, `WHERE domain='SAFE_DEAL' AND internal_reference_id=$1::uuid AND operation_type=$2 AND status IN('CREATED','PENDING_USER_ACTION','PROCESSING','AUTHORIZED','UNKNOWN_REQUIRES_RECONCILIATION') ORDER BY created_at DESC LIMIT 1`, dealID, operation)
	if errors.Is(err, sql.ErrNoRows) {
		return Attempt{}, false, nil
	}
	return v, true, err
}

func (r PostgresRepository) Update(ctx context.Context, in Attempt) error {
	_, err := r.DB.ExecContext(ctx, `UPDATE payment_attempts SET status=$2,provider_operation_id=NULLIF($3,''),provider_payment_method_ref=NULLIF($4,''),provider_confirmation_url=NULLIF($5,''),provider_raw_status=NULLIF($6,''),error_category=NULLIF($7,''),reconciliation_state=$8,terminal_at=$9,updated_at=$10,
reconciliation_claimed_at=CASE WHEN $8='IN_PROGRESS' THEN reconciliation_claimed_at ELSE NULL END,
next_reconciliation_at=CASE WHEN $8 IN('REQUIRED','IN_PROGRESS') THEN now()+interval '1 minute' ELSE now() END WHERE id=$1::uuid`, in.ID, in.Status, in.ProviderOperationID, in.ProviderPaymentMethodRef, in.ConfirmationURL, in.ProviderRawStatus, in.ErrorCategory, in.ReconciliationState, in.TerminalAt, in.UpdatedAt)
	return err
}
func (r PostgresRepository) ListPendingReconciliation(ctx context.Context, limit int) ([]Attempt, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	// Atomically lease a bounded batch. SKIP LOCKED avoids duplicate polls
	// across worker processes; an expired lease makes interrupted jobs recover.
	rows, err := r.DB.QueryContext(ctx, `WITH due AS (
 SELECT id FROM payment_attempts
 WHERE (reconciliation_state IN('REQUIRED','IN_PROGRESS') OR status IN('PENDING_USER_ACTION','PROCESSING','UNKNOWN_REQUIRES_RECONCILIATION'))
   AND next_reconciliation_at<=now()
   AND (reconciliation_claimed_at IS NULL OR reconciliation_claimed_at<now()-interval '2 minutes')
 ORDER BY next_reconciliation_at,id LIMIT $1 FOR UPDATE SKIP LOCKED
), claimed AS (
 UPDATE payment_attempts p SET reconciliation_state='IN_PROGRESS', reconciliation_claimed_at=now(), reconciliation_attempts=p.reconciliation_attempts+1, updated_at=now()
 FROM due WHERE p.id=due.id RETURNING p.*
)
SELECT id::text,domain,internal_reference_id::text,provider,operation_type,status,amount_kopecks,currency,idempotency_key,COALESCE(payment_method,''),COALESCE(provider_operation_id,''),COALESCE(provider_payment_method_ref,''),COALESCE(provider_confirmation_url,''),COALESCE(provider_raw_status,''),COALESCE(error_category,''),reconciliation_state,created_at,updated_at,terminal_at FROM claimed ORDER BY updated_at,id`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Attempt{}
	for rows.Next() {
		v, e := scanAttempt(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

const attemptSelect = `SELECT id::text,domain,internal_reference_id::text,provider,operation_type,status,amount_kopecks,currency,idempotency_key,COALESCE(payment_method,''),COALESCE(provider_operation_id,''),COALESCE(provider_payment_method_ref,''),COALESCE(provider_confirmation_url,''),COALESCE(provider_raw_status,''),COALESCE(error_category,''),reconciliation_state,created_at,updated_at,terminal_at FROM payment_attempts`

func (r PostgresRepository) one(ctx context.Context, condition string, args ...any) (Attempt, error) {
	return scanAttempt(r.DB.QueryRowContext(ctx, attemptSelect+` `+condition, args...))
}

type scanner interface{ Scan(...any) error }

func scanAttempt(row scanner) (Attempt, error) {
	var v Attempt
	var terminal sql.NullTime
	err := row.Scan(&v.ID, &v.Domain, &v.InternalReferenceID, &v.Provider, &v.OperationType, &v.Status, &v.AmountKopecks, &v.Currency, &v.IdempotencyKey, &v.PaymentMethod, &v.ProviderOperationID, &v.ProviderPaymentMethodRef, &v.ConfirmationURL, &v.ProviderRawStatus, &v.ErrorCategory, &v.ReconciliationState, &v.CreatedAt, &v.UpdatedAt, &terminal)
	if terminal.Valid {
		t := terminal.Time.UTC()
		v.TerminalAt = &t
	}
	return v, err
}

type WebhookEvent struct {
	Provider                                                 ProviderName
	EventID, AttemptID, Type, ExternalReference, PayloadHash string
	VerifiedAt, ProcessedAt                                  *time.Time
	Attempts                                                 int
	ProcessingResult                                         string
}

func (r PostgresRepository) PersistWebhookEvent(ctx context.Context, e WebhookEvent) (bool, error) {
	var id string
	// A duplicate event whose domain callback previously failed remains
	// processable. Fully processed replays are ignored. This closes the gap
	// where the payment attempt was stored but PRO/Safe Deal synchronization
	// failed before processed_at could be set.
	err := r.DB.QueryRowContext(ctx, `INSERT INTO payment_provider_events(provider,provider_event_id,payment_attempt_id,event_type,external_reference,payload_hash)
VALUES($1,$2,NULLIF($3,'')::uuid,$4,NULLIF($5,''),$6)
ON CONFLICT(provider,provider_event_id) DO UPDATE SET attempts=payment_provider_events.attempts
WHERE payment_provider_events.processed_at IS NULL AND payment_provider_events.payload_hash=excluded.payload_hash
RETURNING id::text`, e.Provider, e.EventID, e.AttemptID, e.Type, e.ExternalReference, e.PayloadHash).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
func (r PostgresRepository) MarkWebhookVerified(ctx context.Context, provider ProviderName, eventID string) error {
	_, err := r.DB.ExecContext(ctx, `UPDATE payment_provider_events SET verified_at=now(),attempts=attempts+1 WHERE provider=$1 AND provider_event_id=$2`, provider, eventID)
	return err
}
func (r PostgresRepository) AttachWebhookAttempt(ctx context.Context, provider ProviderName, eventID, attemptID string) error {
	res, err := r.DB.ExecContext(ctx, `UPDATE payment_provider_events SET payment_attempt_id=$3::uuid WHERE provider=$1 AND provider_event_id=$2 AND (payment_attempt_id IS NULL OR payment_attempt_id=$3::uuid)`, provider, eventID, attemptID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrWebhookInvalid
	}
	return nil
}
func (r PostgresRepository) MarkWebhookProcessed(ctx context.Context, provider ProviderName, eventID, result string) error {
	_, err := r.DB.ExecContext(ctx, `UPDATE payment_provider_events SET processed_at=now(),processing_result=$3 WHERE provider=$1 AND provider_event_id=$2`, provider, eventID, result)
	return err
}

type AdminAttemptFilter struct {
	Provider  ProviderName
	Domain    Domain
	Status    Status
	Operation OperationType
	Reference string
	From      *time.Time
	To        *time.Time
	Limit     int
}

func (r PostgresRepository) ListAdminAttempts(ctx context.Context, f AdminAttemptFilter) ([]Attempt, error) {
	limit := f.Limit
	if limit < 1 || limit > 200 {
		limit = 100
	}
	ref := strings.TrimSpace(f.Reference)
	rows, err := r.DB.QueryContext(ctx, attemptSelect+` WHERE ($1='' OR provider=$1) AND ($2='' OR domain=$2) AND ($3='' OR status=$3) AND ($4='' OR operation_type=$4) AND ($5='' OR id::text ILIKE '%'||$5||'%' OR internal_reference_id::text ILIKE '%'||$5||'%' OR COALESCE(provider_operation_id,'') ILIKE '%'||$5||'%') AND ($6::timestamptz IS NULL OR created_at >= $6) AND ($7::timestamptz IS NULL OR created_at < $7) ORDER BY created_at DESC,id DESC LIMIT $8`, f.Provider, f.Domain, f.Status, f.Operation, ref, f.From, f.To, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Attempt{}
	for rows.Next() {
		v, err := scanAttempt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

type PaymentAuditEntry struct {
	Action    string    `json:"action"`
	ActorID   string    `json:"actor_id,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	RequestID string    `json:"request_id,omitempty"`
	Details   string    `json:"details,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func (r PostgresRepository) ListRelatedAttempts(ctx context.Context, a Attempt) ([]Attempt, error) {
	rows, err := r.DB.QueryContext(ctx, attemptSelect+` WHERE domain=$1 AND internal_reference_id=$2::uuid AND id<>$3::uuid ORDER BY created_at DESC,id DESC LIMIT 100`, a.Domain, a.InternalReferenceID, a.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Attempt{}
	for rows.Next() {
		v, err := scanAttempt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r PostgresRepository) ListPaymentAudit(ctx context.Context, attemptID string) ([]PaymentAuditEntry, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT action,COALESCE(actor_user_id::text,''),COALESCE(metadata->>'reason',''),COALESCE(metadata->>'request_id',''),COALESCE(metadata->>'details',''),created_at FROM audit_logs WHERE target_type='PAYMENT_ATTEMPT' AND target_id=$1::uuid ORDER BY created_at DESC,id DESC LIMIT 100`, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PaymentAuditEntry{}
	for rows.Next() {
		var v PaymentAuditEntry
		if err := rows.Scan(&v.Action, &v.ActorID, &v.Reason, &v.RequestID, &v.Details, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r PostgresRepository) ListAttemptEvents(ctx context.Context, attemptID string) ([]WebhookEvent, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT provider,provider_event_id,COALESCE(payment_attempt_id::text,''),event_type,COALESCE(external_reference,''),payload_hash,verified_at,processed_at,attempts,COALESCE(processing_result,'') FROM payment_provider_events WHERE payment_attempt_id=$1::uuid ORDER BY received_at DESC,id DESC LIMIT 100`, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WebhookEvent{}
	for rows.Next() {
		var v WebhookEvent
		var verified, processed sql.NullTime
		if err := rows.Scan(&v.Provider, &v.EventID, &v.AttemptID, &v.Type, &v.ExternalReference, &v.PayloadHash, &verified, &processed, &v.Attempts, &v.ProcessingResult); err != nil {
			return nil, err
		}
		if verified.Valid {
			t := verified.Time.UTC()
			v.VerifiedAt = &t
		}
		if processed.Valid {
			t := processed.Time.UTC()
			v.ProcessedAt = &t
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r PostgresRepository) AuditPaymentAdminAction(ctx context.Context, actor, action, attemptID, reason, requestID string, metadata string) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO audit_logs(id,actor_user_id,action,target_type,target_id,metadata,ip) VALUES(gen_random_uuid(),$1::uuid,$2,'PAYMENT_ATTEMPT',$3::uuid,jsonb_build_object('reason',$4,'request_id',$5,'details',$6),NULLIF($7,'')::inet)`, actor, action, attemptID, reason, requestID, metadata, requestmeta.FromContext(ctx))
	return err
}
