package safedeal

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"freelance/apps/api/internal/platform/requestmeta"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

type PostgresRepository struct{ DB *sql.DB }

// dealColumns lists every column read into a Deal, including the immutable
// per-deal economic snapshot (the quote). The snapshot is read verbatim, never
// recomputed, so editing a live fee rule can never alter an existing deal.
const dealColumns = `d.id::text,d.project_id::text,d.assignment_id::text,d.customer_user_id::text,d.freelancer_user_id::text,d.currency,d.gross_amount_kopecks,d.platform_fee_kopecks,d.freelancer_amount_kopecks,d.status,d.revision_count,d.funded_at,d.work_started_at,d.submitted_at,d.accepted_at,d.completed_at,d.created_at,d.updated_at,COALESCE(c.id::text,''),d.work_amount_kopecks,d.platform_fee_customer_kopecks,d.platform_fee_freelancer_kopecks,d.platform_fee_platform_kopecks,d.provider_fee_kopecks,d.provider_fee_customer_kopecks,d.provider_fee_freelancer_kopecks,d.provider_fee_platform_kopecks,d.customer_discount_kopecks,d.freelancer_bonus_kopecks,d.platform_provider_cost_kopecks,d.platform_subsidy_kopecks,d.platform_net_revenue_kopecks,d.fee_rule_version,d.provider_pricing_version`

func (r PostgresRepository) List(ctx context.Context, actor, project string) ([]Deal, error) {
	rows, e := r.DB.QueryContext(ctx, `SELECT `+dealColumns+` FROM safe_deals d LEFT JOIN conversations c ON c.project_id=d.project_id WHERE(d.customer_user_id=$1 OR d.freelancer_user_id=$1)AND($2='' OR d.project_id=$2::uuid)ORDER BY d.updated_at DESC,d.id DESC LIMIT 100`, actor, project)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Deal{}
	for rows.Next() {
		d, e := scanDeal(rows)
		if e != nil {
			return nil, e
		}
		if e = r.enrich(ctx, &d, true); e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func (r PostgresRepository) Get(ctx context.Context, actor, id string, admin bool) (Deal, error) {
	condition := `(d.customer_user_id=$2 OR d.freelancer_user_id=$2)`
	if admin {
		condition = `EXISTS(SELECT 1 FROM user_roles ur JOIN users u ON u.id=ur.user_id WHERE ur.user_id=$2 AND ur.role IN('MODERATOR','ADMIN','SUPER_ADMIN')AND u.status='ACTIVE'AND u.deleted_at IS NULL)`
	}
	d, e := scanDeal(r.DB.QueryRowContext(ctx, `SELECT `+dealColumns+` FROM safe_deals d LEFT JOIN conversations c ON c.project_id=d.project_id WHERE d.id=$1 AND `+condition, id, actor))
	if errors.Is(e, sql.ErrNoRows) {
		return Deal{}, ErrNotFound
	}
	if e != nil {
		return Deal{}, e
	}
	if e = r.enrich(ctx, &d, true); e != nil {
		return Deal{}, e
	}
	return d, nil
}
func (r PostgresRepository) SaveFunding(ctx context.Context, actor, id, key string, p CreateFundingResult) (Deal, error) {
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return Deal{}, e
	}
	defer tx.Rollback()
	d, e := lockDeal(ctx, tx, id, actor, "CUSTOMER")
	if e != nil {
		return Deal{}, e
	}
	if d.Status != "AWAITING_FUNDING" {
		return Deal{}, ErrInvalidState
	}
	hash := requestHash(map[string]any{"provider": p.Provider, "provider_payment_id": p.ProviderPaymentID, "amount": d.GrossAmountKopecks})
	existing, e := command(ctx, tx, d.ID, actor, "FUND", key, hash)
	if e != nil {
		return Deal{}, e
	}
	if existing {
		_ = tx.Rollback()
		return r.Get(ctx, actor, id, false)
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO payment_records(deal_id,provider,provider_payment_id,provider_status,amount_kopecks,currency,idempotency_key,checkout_url)
VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8)
ON CONFLICT(provider,provider_payment_id) WHERE provider_payment_id IS NOT NULL
DO UPDATE SET provider_status=excluded.provider_status,checkout_url=COALESCE(NULLIF(excluded.checkout_url,''),payment_records.checkout_url),updated_at=now()`, id, p.Provider, p.ProviderPaymentID, p.Status, d.GrossAmountKopecks, d.Currency, key, p.CheckoutURL)
	if e != nil {
		return Deal{}, dbErr(e)
	}
	if e = appendEvent(ctx, tx, id, "FUNDING_CREATED", actor, "fund:"+key, map[string]string{"provider": p.Provider}); e != nil {
		return Deal{}, e
	}
	if e = tx.Commit(); e != nil {
		return Deal{}, e
	}
	return r.Get(ctx, actor, id, false)
}
func (r PostgresRepository) Start(ctx context.Context, actor, id, key string) (Deal, error) {
	return r.simple(ctx, actor, id, key, "START", []string{"FUNDED"}, "IN_PROGRESS", "FREELANCER", "DEAL_STARTED", func(ctx context.Context, tx *sql.Tx, d Deal) error {
		_, e := tx.ExecContext(ctx, `UPDATE safe_deals SET work_started_at=COALESCE(work_started_at,now()) WHERE id=$1`, d.ID)
		if e == nil {
			_, e = tx.ExecContext(ctx, `UPDATE project_assignments SET started_at=COALESCE(started_at,now()) WHERE id=$1`, d.AssignmentID)
		}
		if e == nil {
			_, e = tx.ExecContext(ctx, `UPDATE projects SET status='IN_PROGRESS',updated_at=now() WHERE id=$1 AND status='AWAITING_FUNDING'`, d.ProjectID)
		}
		return e
	})
}
func (r PostgresRepository) Submit(ctx context.Context, actor, id, key string, in SubmitInput) (Deal, error) {
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return Deal{}, e
	}
	defer tx.Rollback()
	d, e := lockDeal(ctx, tx, id, actor, "FREELANCER")
	if e != nil {
		return Deal{}, e
	}
	if !oneOf(d.Status, "IN_PROGRESS", "REVISION_REQUESTED") {
		return Deal{}, ErrInvalidState
	}
	hash := requestHash(in)
	existing, e := command(ctx, tx, id, actor, "SUBMIT", key, hash)
	if e != nil {
		return Deal{}, e
	}
	if existing {
		_ = tx.Rollback()
		return r.Get(ctx, actor, id, false)
	}
	var message any
	if in.MessageID != "" {
		message = in.MessageID
		var allowed bool
		e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM messages m JOIN conversations c ON c.id=m.conversation_id JOIN conversation_members cm ON cm.conversation_id=c.id AND cm.user_id=$2 WHERE m.id=$1 AND c.project_id=$3)`, in.MessageID, actor, d.ProjectID).Scan(&allowed)
		if e != nil || !allowed {
			return Deal{}, ErrInvalid
		}
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO safe_deal_submissions(deal_id,revision_number,submitted_by_user_id,summary,message_id)VALUES($1,$2,$3,$4,$5)`, id, d.RevisionCount, actor, in.Summary, message)
	if e != nil {
		return Deal{}, dbErr(e)
	}
	_, e = tx.ExecContext(ctx, `UPDATE safe_deals SET status='SUBMITTED',submitted_at=now(),updated_at=now(),version=version+1 WHERE id=$1`, id)
	if e != nil {
		return Deal{}, e
	}
	if e = appendEvent(ctx, tx, id, "DEAL_SUBMITTED", actor, "submit:"+key, nil); e != nil {
		return Deal{}, e
	}
	if e = tx.Commit(); e != nil {
		return Deal{}, e
	}
	return r.Get(ctx, actor, id, false)
}
func (r PostgresRepository) Revision(ctx context.Context, actor, id, key, reason string) (Deal, error) {
	return r.simpleMeta(ctx, actor, id, key, "REVISION", []string{"SUBMITTED"}, "REVISION_REQUESTED", "CUSTOMER", "DEAL_REVISION_REQUESTED", map[string]string{"reason": reason}, func(ctx context.Context, tx *sql.Tx, d Deal) error {
		res, e := tx.ExecContext(ctx, `UPDATE safe_deals SET revision_count=revision_count+1 WHERE id=$1 AND revision_count<2`, d.ID)
		if e != nil {
			return e
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrInvalidState
		}
		return nil
	})
}
func (r PostgresRepository) OpenDispute(ctx context.Context, actor, id, key string, in DisputeInput) (Deal, error) {
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return Deal{}, e
	}
	defer tx.Rollback()
	d, e := lockDeal(ctx, tx, id, actor, "PARTY")
	if e != nil {
		return Deal{}, e
	}
	if !oneOf(d.Status, "FUNDED", "IN_PROGRESS", "SUBMITTED", "REVISION_REQUESTED") {
		return Deal{}, ErrInvalidState
	}
	hash := requestHash(in)
	existing, e := command(ctx, tx, id, actor, "DISPUTE", key, hash)
	if e != nil {
		return Deal{}, e
	}
	if existing {
		_ = tx.Rollback()
		return r.Get(ctx, actor, id, false)
	}
	var dispute string
	e = tx.QueryRowContext(ctx, `INSERT INTO safe_deal_disputes(deal_id,opened_by_user_id,reason_code,description)VALUES($1,$2,$3,$4)RETURNING id::text`, id, actor, in.ReasonCode, in.Description).Scan(&dispute)
	if e != nil {
		return Deal{}, dbErr(e)
	}
	_, e = tx.ExecContext(ctx, `UPDATE safe_deals SET status='DISPUTED',updated_at=now(),version=version+1 WHERE id=$1`, id)
	if e != nil {
		return Deal{}, e
	}
	if e = appendEvent(ctx, tx, id, "DISPUTE_OPENED", actor, "dispute:"+key, map[string]string{"dispute_id": dispute, "reason_code": in.ReasonCode}); e != nil {
		return Deal{}, e
	}
	if e = tx.Commit(); e != nil {
		return Deal{}, e
	}
	return r.Get(ctx, actor, id, false)
}
func (r PostgresRepository) AddEvidence(ctx context.Context, actor, dispute, key string, in EvidenceInput) (Deal, error) {
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return Deal{}, e
	}
	defer tx.Rollback()
	var deal, customer, freelancer, status string
	e = tx.QueryRowContext(ctx, `SELECT sd.id::text,sd.customer_user_id::text,sd.freelancer_user_id::text,d.status FROM safe_deal_disputes d JOIN safe_deals sd ON sd.id=d.deal_id WHERE d.id=$1 FOR UPDATE OF d`, dispute).Scan(&deal, &customer, &freelancer, &status)
	if errors.Is(e, sql.ErrNoRows) || actor != customer && actor != freelancer && !isAdmin(ctx, tx, actor) {
		return Deal{}, ErrNotFound
	}
	if e != nil {
		return Deal{}, e
	}
	if !oneOf(status, "OPEN", "EVIDENCE_COLLECTION", "UNDER_REVIEW") {
		return Deal{}, ErrInvalidState
	}
	hash := requestHash(in)
	existing, e := command(ctx, tx, deal, actor, "EVIDENCE", key, hash)
	if e != nil {
		return Deal{}, e
	}
	if existing {
		_ = tx.Rollback()
		return r.Get(ctx, actor, deal, actor != customer && actor != freelancer)
	}
	var reference any
	if in.ReferenceID != "" {
		reference = in.ReferenceID
	}
	if in.Kind == "MESSAGE_REFERENCE" {
		var ok bool
		e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM messages m JOIN conversations c ON c.id=m.conversation_id WHERE m.id=$1 AND c.project_id=(SELECT project_id FROM safe_deals WHERE id=$2))`, in.ReferenceID, deal).Scan(&ok)
		if e != nil || !ok {
			return Deal{}, ErrInvalid
		}
	}
	if in.Kind == "SUBMISSION_REFERENCE" {
		var ok bool
		e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM safe_deal_submissions WHERE id=$1 AND deal_id=$2)`, in.ReferenceID, deal).Scan(&ok)
		if e != nil || !ok {
			return Deal{}, ErrInvalid
		}
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO safe_deal_dispute_evidence(dispute_id,author_user_id,kind,body,reference_id)VALUES($1,$2,$3,$4,$5)`, dispute, actor, in.Kind, in.Body, reference)
	if e != nil {
		return Deal{}, e
	}
	_, e = tx.ExecContext(ctx, `UPDATE safe_deal_disputes SET status='EVIDENCE_COLLECTION' WHERE id=$1 AND status='OPEN'`, dispute)
	if e != nil {
		return Deal{}, e
	}
	if e = tx.Commit(); e != nil {
		return Deal{}, e
	}
	return r.Get(ctx, actor, deal, actor != customer && actor != freelancer)
}
func (r PostgresRepository) PrepareAccept(ctx context.Context, actor, id, key string) (Deal, error) {
	d, e := r.simple(ctx, actor, id, key, "ACCEPT", []string{"SUBMITTED"}, "ACCEPTED", "CUSTOMER", "DEAL_ACCEPTED", func(ctx context.Context, tx *sql.Tx, d Deal) error {
		_, e := tx.ExecContext(ctx, `UPDATE safe_deals SET accepted_at=COALESCE(accepted_at,now()) WHERE id=$1`, d.ID)
		return e
	})
	if errors.Is(e, ErrInvalidState) {
		current, g := r.Get(ctx, actor, id, false)
		if g == nil && oneOf(current.Status, "ACCEPTED", "RELEASE_PENDING", "COMPLETED") {
			return current, nil
		}
	}
	return d, e
}
func (r PostgresRepository) MarkReleasePending(ctx context.Context, id, key string, out ReleaseResult, admin bool) (Deal, error) {
	return r.providerPending(ctx, id, key, "RELEASE_PENDING", []string{"ACCEPTED", "DISPUTED"}, out.ProviderOperationID, admin)
}
func (r PostgresRepository) CancelUnfunded(ctx context.Context, actor, id, key string) (Deal, error) {
	return r.simple(ctx, actor, id, key, "CANCEL", []string{"AWAITING_FUNDING"}, "CANCELED", "CUSTOMER", "DEAL_CANCELED", func(ctx context.Context, tx *sql.Tx, d Deal) error {
		var count int
		if e := tx.QueryRowContext(ctx, `SELECT count(*) FROM payment_records WHERE deal_id=$1`, d.ID).Scan(&count); e != nil {
			return e
		}
		if count > 0 {
			return ErrInvalidState
		}
		_, e := tx.ExecContext(ctx, `UPDATE projects SET status='CANCELLED',updated_at=now() WHERE id=$1`, d.ProjectID)
		if e == nil {
			_, e = tx.ExecContext(ctx, `UPDATE project_assignments SET status='CANCELLED' WHERE id=$1`, d.AssignmentID)
		}
		return e
	})
}
func (r PostgresRepository) MarkCancelPending(ctx context.Context, id, key string) (Deal, error) {
	return r.providerPending(ctx, id, key, "CANCEL_PENDING", []string{"AWAITING_FUNDING"}, "", false)
}
func (r PostgresRepository) MarkRefundPending(ctx context.Context, id, key string, out RefundResult, admin bool) (Deal, error) {
	return r.providerPending(ctx, id, key, "REFUND_PENDING", []string{"FUNDED", "DISPUTED"}, out.ProviderOperationID, admin)
}
func (r PostgresRepository) ApplyProviderEvent(ctx context.Context, event VerifiedProviderEvent) (Deal, bool, error) {
	if !event.Verified {
		return Deal{}, false, ErrForbidden
	}
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return Deal{}, false, e
	}
	defer tx.Rollback()
	var payment, deal, status, currency string
	var amount, freelancerAmount int64
	e = tx.QueryRowContext(ctx, `SELECT pr.id::text,pr.deal_id::text,d.status,pr.currency,pr.amount_kopecks,d.freelancer_amount_kopecks FROM payment_records pr JOIN safe_deals d ON d.id=pr.deal_id WHERE pr.provider=$1 AND pr.provider_payment_id=$2 FOR UPDATE OF pr,d`, event.Provider, event.ProviderPaymentID).Scan(&payment, &deal, &status, &currency, &amount, &freelancerAmount)
	if errors.Is(e, sql.ErrNoRows) {
		return Deal{}, false, ErrNotFound
	}
	if e != nil {
		return Deal{}, false, e
	}
	expected := amount
	if event.Type == "RELEASE_CONFIRMED" {
		expected = freelancerAmount
	}
	if event.AmountKopecks == 0 {
		event.AmountKopecks = expected
	}
	if expected != event.AmountKopecks || currency != event.Currency {
		return Deal{}, false, ErrConflict
	}
	payload, _ := json.Marshal(event.Payload)
	res, e := tx.ExecContext(ctx, `INSERT INTO payment_events(provider,provider_event_id,payment_record_id,event_type,verified,payload)VALUES($1,$2,$3,$4,true,$5::jsonb)ON CONFLICT(provider,provider_event_id)DO NOTHING`, event.Provider, event.ProviderEventID, payment, event.Type, string(payload))
	if e != nil {
		return Deal{}, false, e
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		_ = tx.Rollback()
		d, g := r.Get(ctx, "", deal, true)
		if errors.Is(g, ErrNotFound) {
			d, g = r.getSystem(ctx, deal)
		}
		return d, false, g
	}
	target, paymentStatus, outbox := "", "", event.Type
	switch event.Type {
	case "FUNDING_CONFIRMED":
		if status != "AWAITING_FUNDING" {
			return Deal{}, false, ErrInvalidState
		}
		target, paymentStatus, outbox = "FUNDED", "FUNDED", "DEAL_FUNDED"
	case "RELEASE_CONFIRMED":
		if status != "RELEASE_PENDING" {
			return Deal{}, false, ErrInvalidState
		}
		target, paymentStatus, outbox = "COMPLETED", "RELEASED", "DEAL_COMPLETED"
	case "REFUND_CONFIRMED":
		if status != "REFUND_PENDING" {
			return Deal{}, false, ErrInvalidState
		}
		target, paymentStatus, outbox = "REFUNDED", "REFUNDED", "DEAL_REFUNDED"
	case "CANCEL_CONFIRMED":
		if !oneOf(status, "CANCEL_PENDING", "AWAITING_FUNDING") {
			return Deal{}, false, ErrInvalidState
		}
		target, paymentStatus, outbox = "CANCELED", "CANCELED", "DEAL_CANCELED"
	default:
		return Deal{}, false, ErrInvalid
	}
	timeSet := ""
	if target == "FUNDED" {
		timeSet = ",funded_at=COALESCE(funded_at,now())"
	}
	if target == "COMPLETED" {
		timeSet = ",completed_at=COALESCE(completed_at,now())"
	}
	if target == "CANCELED" || target == "REFUNDED" {
		timeSet = ",canceled_at=COALESCE(canceled_at,now())"
	}
	_, e = tx.ExecContext(ctx, `UPDATE safe_deals SET status=$2,updated_at=now(),version=version+1`+timeSet+` WHERE id=$1`, deal, target)
	if e != nil {
		return Deal{}, false, e
	}
	_, e = tx.ExecContext(ctx, `UPDATE payment_records SET provider_status=$2,updated_at=now() WHERE id=$1`, payment, paymentStatus)
	if e != nil {
		return Deal{}, false, e
	}
	if target == "COMPLETED" {
		_, e = tx.ExecContext(ctx, `UPDATE projects SET status='COMPLETED',updated_at=now() WHERE id=(SELECT project_id FROM safe_deals WHERE id=$1)`, deal)
		if e == nil {
			_, e = tx.ExecContext(ctx, `UPDATE project_assignments SET status='COMPLETED',completed_at=COALESCE(completed_at,now()) WHERE id=(SELECT assignment_id FROM safe_deals WHERE id=$1)`, deal)
		}
	} else if target == "REFUNDED" || target == "CANCELED" {
		_, e = tx.ExecContext(ctx, `UPDATE projects SET status='CANCELLED',updated_at=now() WHERE id=(SELECT project_id FROM safe_deals WHERE id=$1)`, deal)
		if e == nil {
			_, e = tx.ExecContext(ctx, `UPDATE project_assignments SET status='CANCELLED' WHERE id=(SELECT assignment_id FROM safe_deals WHERE id=$1)`, deal)
		}
	}
	if e != nil {
		return Deal{}, false, e
	}
	if e = appendEvent(ctx, tx, deal, outbox, "", "provider:"+event.Provider+":"+event.ProviderEventID, map[string]string{"provider": event.Provider, "provider_event_id": event.ProviderEventID}); e != nil {
		return Deal{}, false, e
	}
	_, e = tx.ExecContext(ctx, `UPDATE payment_events SET processed_at=now() WHERE provider=$1 AND provider_event_id=$2`, event.Provider, event.ProviderEventID)
	if e != nil {
		return Deal{}, false, e
	}
	if e = tx.Commit(); e != nil {
		return Deal{}, false, e
	}
	d, e := r.getSystem(ctx, deal)
	return d, true, e
}
func (r PostgresRepository) AdminList(ctx context.Context, actor string) ([]Deal, error) {
	if !isAdmin(ctx, r.DB, actor) {
		return nil, ErrForbidden
	}
	rows, e := r.DB.QueryContext(ctx, `SELECT `+dealColumns+` FROM safe_deals d LEFT JOIN conversations c ON c.project_id=d.project_id ORDER BY CASE WHEN d.status='DISPUTED'THEN 0 ELSE 1 END,d.updated_at DESC LIMIT 200`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Deal{}
	for rows.Next() {
		d, e := scanDeal(rows)
		if e != nil {
			return nil, e
		}
		if e = r.enrich(ctx, &d, true); e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func (r PostgresRepository) AdminDeleteUnfunded(ctx context.Context, actor, id, reason string) error {
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if !isAdmin(ctx, tx, actor) {
		return ErrForbidden
	}
	var status string
	e = tx.QueryRowContext(ctx, `SELECT status FROM safe_deals WHERE id=$1 FOR UPDATE`, id).Scan(&status)
	if errors.Is(e, sql.ErrNoRows) {
		return ErrNotFound
	}
	if e != nil {
		return e
	}
	if status != "AWAITING_FUNDING" {
		return ErrInvalidState
	}
	var hasPayment bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM payment_records WHERE deal_id=$1 UNION ALL SELECT 1 FROM payment_attempts WHERE domain='SAFE_DEAL' AND internal_reference_id=$1::uuid AND status NOT IN('FAILED','CANCELED'))`, id).Scan(&hasPayment)
	if e != nil {
		return e
	}
	if hasPayment {
		return ErrInvalidState
	}
	for _, q := range []string{`DELETE FROM safe_deal_command_results WHERE deal_id=$1`, `DELETE FROM safe_deal_dispute_evidence WHERE dispute_id IN(SELECT id FROM safe_deal_disputes WHERE deal_id=$1)`, `DELETE FROM safe_deal_disputes WHERE deal_id=$1`, `DELETE FROM safe_deal_submissions WHERE deal_id=$1`, `DELETE FROM payment_records WHERE deal_id=$1`, `DELETE FROM safe_deal_events WHERE deal_id=$1`} {
		if _, e = tx.ExecContext(ctx, q, id); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO audit_logs(id,actor_user_id,action,target_type,target_id,metadata,ip)VALUES(gen_random_uuid(),$1,'SAFE_DEAL_DELETED_UNFUNDED','SAFE_DEAL',$2,jsonb_build_object('reason',$3),NULLIF($4,'')::inet)`, actor, id, reason, requestmeta.FromContext(ctx)); e != nil {
		return e
	}
	res, e := tx.ExecContext(ctx, `DELETE FROM safe_deals WHERE id=$1`, id)
	if e != nil {
		return e
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrNotFound
	}
	return tx.Commit()
}

func (r PostgresRepository) PrepareResolution(ctx context.Context, actor, id, key string, in ResolutionInput) (Deal, error) {
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return Deal{}, e
	}
	defer tx.Rollback()
	if !isAdmin(ctx, tx, actor) {
		return Deal{}, ErrForbidden
	}
	d, e := lockDeal(ctx, tx, id, "", "SYSTEM")
	if e != nil {
		return Deal{}, e
	}
	if d.Status != "DISPUTED" {
		return Deal{}, ErrInvalidState
	}
	hash := requestHash(in)
	existing, e := command(ctx, tx, id, actor, "RESOLVE", key, hash)
	if e != nil {
		return Deal{}, e
	}
	if existing {
		_ = tx.Rollback()
		return r.Get(ctx, actor, id, true)
	}
	result, e := tx.ExecContext(ctx, `UPDATE safe_deal_disputes SET status=$2,resolution=$3,resolution_reason=$4,resolved_at=now(),resolved_by_admin_id=$5 WHERE deal_id=$1 AND status IN('OPEN','EVIDENCE_COLLECTION','UNDER_REVIEW')`, id, "RESOLVED_"+in.Outcome, in.Outcome, in.Reason, actor)
	if e != nil {
		return Deal{}, e
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return Deal{}, ErrInvalidState
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO audit_logs(id,actor_user_id,action,target_type,target_id,metadata,ip)VALUES(gen_random_uuid(),$1,'SAFE_DEAL_DISPUTE_RESOLVED','SAFE_DEAL',$2,jsonb_build_object('outcome',$3,'reason',$4),NULLIF($5,'')::inet)`, actor, id, in.Outcome, in.Reason, requestmeta.FromContext(ctx))
	if e != nil {
		return Deal{}, e
	}
	if e = appendEvent(ctx, tx, id, "DISPUTE_RESOLVED", actor, "resolve:"+key, map[string]string{"outcome": in.Outcome}); e != nil {
		return Deal{}, e
	}
	if e = tx.Commit(); e != nil {
		return Deal{}, e
	}
	return r.Get(ctx, actor, id, true)
}

// ActiveEconomics loads the enabled platform fee rule and the enabled provider
// pricing for the requested payment method. The provider payer mode lives on the
// fee rule (the pricing table stores pure cost), so it is copied onto the
// returned ProviderPricing. Absent pricing for the method yields a zero-cost
// provider fee — identical to the assignment trigger's has_pricing=false path.
func (r PostgresRepository) ActiveEconomics(ctx context.Context, paymentMethod string) (FeePolicy, ProviderPricing, error) {
	var fee FeePolicy
	var feeMax sql.NullInt64
	var providerPayerMode string
	var providerShare int
	e := r.DB.QueryRowContext(ctx, `SELECT version,commission_basis_points,minimum_fee_kopecks,maximum_fee_kopecks,platform_fee_payer_mode,platform_customer_share_basis_points,provider_fee_payer_mode,provider_customer_share_basis_points FROM safe_deal_fee_rules WHERE enabled AND effective_from<=now() ORDER BY effective_from DESC LIMIT 1`).
		Scan(&fee.Version, &fee.CommissionBasisPoints, &fee.MinimumFeeKopecks, &feeMax, &fee.PayerMode, &fee.CustomerShareBasisPoints, &providerPayerMode, &providerShare)
	if errors.Is(e, sql.ErrNoRows) {
		return FeePolicy{}, ProviderPricing{}, ErrInvalidState
	}
	if e != nil {
		return FeePolicy{}, ProviderPricing{}, e
	}
	if feeMax.Valid {
		fee.MaximumFeeKopecks = &feeMax.Int64
	}
	// The payer decision for the provider cost always comes from the fee rule.
	// Provider cost follows the provider currently routed for new Safe Deals;
	// changing the route never changes already-snapshotted deal economics.
	provider := "sandbox"
	_ = r.DB.QueryRowContext(ctx, `SELECT provider FROM payment_provider_routes WHERE domain='SAFE_DEAL'`).Scan(&provider)
	pricing := ProviderPricing{Version: 1, Provider: provider, PaymentMethod: paymentMethod, PayerMode: providerPayerMode, CustomerShareBasisPoints: providerShare}
	var priceMax sql.NullInt64
	e = r.DB.QueryRowContext(ctx, `SELECT version,provider,percent_basis_points,fixed_fee_kopecks,minimum_fee_kopecks,maximum_fee_kopecks FROM safe_deal_provider_pricing WHERE enabled AND provider=$1 AND payment_method=$2 AND effective_from<=now() ORDER BY effective_from DESC LIMIT 1`, provider, paymentMethod).
		Scan(&pricing.Version, &pricing.Provider, &pricing.PercentBasisPoints, &pricing.FixedFeeKopecks, &pricing.MinimumFeeKopecks, &priceMax)
	if errors.Is(e, sql.ErrNoRows) {
		return fee, pricing, nil
	}
	if e != nil {
		return FeePolicy{}, ProviderPricing{}, e
	}
	if priceMax.Valid {
		pricing.MaximumFeeKopecks = &priceMax.Int64
	}
	return fee, pricing, nil
}
func (r PostgresRepository) Unresolved(ctx context.Context) ([]Payment, error) {
	rows, e := r.DB.QueryContext(ctx, `SELECT id::text,deal_id::text,provider,provider_payment_id,provider_status,COALESCE(checkout_url,''),amount_kopecks,currency,idempotency_key,updated_at FROM payment_records WHERE provider_payment_id IS NOT NULL AND provider_status IN('PENDING','FUNDED','RELEASE_PENDING','REFUND_PENDING','CANCEL_PENDING') ORDER BY updated_at LIMIT 500`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Payment{}
	for rows.Next() {
		var p Payment
		if e = rows.Scan(&p.ID, &p.DealID, &p.Provider, &p.ProviderPaymentID, &p.ProviderStatus, &p.CheckoutURL, &p.AmountKopecks, &p.Currency, &p.IdempotencyKey, &p.UpdatedAt); e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r PostgresRepository) simple(ctx context.Context, actor, id, key, action string, from []string, to, role, event string, extra func(context.Context, *sql.Tx, Deal) error) (Deal, error) {
	return r.simpleMeta(ctx, actor, id, key, action, from, to, role, event, nil, extra)
}
func (r PostgresRepository) simpleMeta(ctx context.Context, actor, id, key, action string, from []string, to, role, event string, meta map[string]string, extra func(context.Context, *sql.Tx, Deal) error) (Deal, error) {
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return Deal{}, e
	}
	defer tx.Rollback()
	d, e := lockDeal(ctx, tx, id, actor, role)
	if e != nil {
		return Deal{}, e
	}
	if !oneOf(d.Status, from...) {
		return Deal{}, ErrInvalidState
	}
	hash := requestHash(meta)
	existing, e := command(ctx, tx, id, actor, action, key, hash)
	if e != nil {
		return Deal{}, e
	}
	if existing {
		_ = tx.Rollback()
		return r.Get(ctx, actor, id, role == "ADMIN")
	}
	if extra != nil {
		if e = extra(ctx, tx, d); e != nil {
			return Deal{}, e
		}
	}
	_, e = tx.ExecContext(ctx, `UPDATE safe_deals SET status=$2,updated_at=now(),version=version+1 WHERE id=$1`, id, to)
	if e != nil {
		return Deal{}, e
	}
	if e = appendEvent(ctx, tx, id, event, actor, strings.ToLower(action)+":"+key, meta); e != nil {
		return Deal{}, e
	}
	if e = tx.Commit(); e != nil {
		return Deal{}, e
	}
	return r.Get(ctx, actor, id, role == "ADMIN")
}
func (r PostgresRepository) providerPending(ctx context.Context, id, key, to string, from []string, operation string, admin bool) (Deal, error) {
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return Deal{}, e
	}
	defer tx.Rollback()
	d, e := lockDeal(ctx, tx, id, "", "SYSTEM")
	if e != nil {
		return Deal{}, e
	}
	if d.Status == to {
		_ = tx.Rollback()
		return r.getSystem(ctx, id)
	}
	if !oneOf(d.Status, from...) {
		return Deal{}, ErrInvalidState
	}
	_, e = tx.ExecContext(ctx, `UPDATE safe_deals SET status=$2,updated_at=now(),version=version+1 WHERE id=$1`, id, to)
	if e == nil {
		_, e = tx.ExecContext(ctx, `UPDATE payment_records SET provider_status=$2,updated_at=now() WHERE deal_id=$1`, id, to)
	}
	if e != nil {
		return Deal{}, e
	}
	if e = appendEvent(ctx, tx, id, "DEAL_"+to, "", "provider-command:"+key, map[string]string{"provider_operation_id": operation}); e != nil {
		return Deal{}, e
	}
	if e = tx.Commit(); e != nil {
		return Deal{}, e
	}
	return r.getSystem(ctx, id)
}
func lockDeal(ctx context.Context, tx *sql.Tx, id, actor, role string) (Deal, error) {
	var d Deal
	e := tx.QueryRowContext(ctx, `SELECT id::text,project_id::text,assignment_id::text,customer_user_id::text,freelancer_user_id::text,currency,gross_amount_kopecks,platform_fee_kopecks,freelancer_amount_kopecks,status,revision_count,funded_at,work_started_at,submitted_at,accepted_at,completed_at,created_at,updated_at FROM safe_deals WHERE id=$1 FOR UPDATE`, id).Scan(&d.ID, &d.ProjectID, &d.AssignmentID, &d.CustomerUserID, &d.FreelancerUserID, &d.Currency, &d.GrossAmountKopecks, &d.PlatformFeeKopecks, &d.FreelancerAmountKopecks, &d.Status, &d.RevisionCount, &d.FundedAt, &d.WorkStartedAt, &d.SubmittedAt, &d.AcceptedAt, &d.CompletedAt, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(e, sql.ErrNoRows) {
		return Deal{}, ErrNotFound
	}
	if e != nil {
		return Deal{}, e
	}
	if role == "CUSTOMER" && actor != d.CustomerUserID || role == "FREELANCER" && actor != d.FreelancerUserID || role == "PARTY" && actor != d.CustomerUserID && actor != d.FreelancerUserID {
		return Deal{}, ErrNotFound
	}
	return d, nil
}
func (r PostgresRepository) getSystem(ctx context.Context, id string) (Deal, error) {
	d, e := scanDeal(r.DB.QueryRowContext(ctx, `SELECT `+dealColumns+` FROM safe_deals d LEFT JOIN conversations c ON c.project_id=d.project_id WHERE d.id=$1`, id))
	if errors.Is(e, sql.ErrNoRows) {
		return Deal{}, ErrNotFound
	}
	if e != nil {
		return Deal{}, e
	}
	e = r.enrich(ctx, &d, true)
	return d, e
}
func (r PostgresRepository) enrich(ctx context.Context, d *Deal, private bool) error {
	if e := r.DB.QueryRowContext(ctx, `SELECT p.title,cu.display_name,fu.display_name FROM safe_deals sd JOIN projects p ON p.id=sd.project_id JOIN users cu ON cu.id=sd.customer_user_id JOIN users fu ON fu.id=sd.freelancer_user_id WHERE sd.id=$1`, d.ID).Scan(&d.ProjectTitle, &d.CustomerName, &d.FreelancerName); e != nil {
		return e
	}
	var p Payment
	e := r.DB.QueryRowContext(ctx, `SELECT id::text,deal_id::text,provider,COALESCE(provider_payment_id,''),provider_status,COALESCE(checkout_url,''),amount_kopecks,currency,idempotency_key,updated_at FROM payment_records WHERE deal_id=$1 ORDER BY created_at DESC LIMIT 1`, d.ID).Scan(&p.ID, &p.DealID, &p.Provider, &p.ProviderPaymentID, &p.ProviderStatus, &p.CheckoutURL, &p.AmountKopecks, &p.Currency, &p.IdempotencyKey, &p.UpdatedAt)
	if e == nil {
		d.Payment = &p
	} else if !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	var sub Submission
	e = r.DB.QueryRowContext(ctx, `SELECT id::text,deal_id::text,submitted_by_user_id::text,summary,COALESCE(message_id::text,''),revision_number,created_at FROM safe_deal_submissions WHERE deal_id=$1 ORDER BY revision_number DESC LIMIT 1`, d.ID).Scan(&sub.ID, &sub.DealID, &sub.SubmittedByUserID, &sub.Summary, &sub.MessageID, &sub.RevisionNumber, &sub.CreatedAt)
	if e == nil {
		d.Submission = &sub
	} else if !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	if private {
		var v Dispute
		e = r.DB.QueryRowContext(ctx, `SELECT id::text,deal_id::text,opened_by_user_id::text,reason_code,description,status,COALESCE(resolution,''),COALESCE(resolution_reason,''),opened_at,resolved_at FROM safe_deal_disputes WHERE deal_id=$1 ORDER BY opened_at DESC LIMIT 1`, d.ID).Scan(&v.ID, &v.DealID, &v.OpenedByUserID, &v.ReasonCode, &v.Description, &v.Status, &v.Resolution, &v.ResolutionReason, &v.OpenedAt, &v.ResolvedAt)
		if e == nil {
			rows, x := r.DB.QueryContext(ctx, `SELECT id::text,dispute_id::text,author_user_id::text,kind,body,COALESCE(reference_id::text,''),created_at FROM safe_deal_dispute_evidence WHERE dispute_id=$1 ORDER BY created_at,id`, v.ID)
			if x != nil {
				return x
			}
			for rows.Next() {
				var x Evidence
				if xerr := rows.Scan(&x.ID, &x.DisputeID, &x.AuthorUserID, &x.Kind, &x.Body, &x.ReferenceID, &x.CreatedAt); xerr != nil {
					rows.Close()
					return xerr
				}
				v.Evidence = append(v.Evidence, x)
			}
			rows.Close()
			d.Dispute = &v
			d.Evidence = v.Evidence
		} else if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
	}
	rows, e := r.DB.QueryContext(ctx, `SELECT event_type,COALESCE(actor_user_id::text,''),metadata,created_at FROM safe_deal_events WHERE deal_id=$1 ORDER BY created_at,id`, d.ID)
	if e != nil {
		return e
	}
	for rows.Next() {
		var v DomainEvent
		var raw []byte
		if e = rows.Scan(&v.Type, &v.ActorUserID, &raw, &v.CreatedAt); e != nil {
			rows.Close()
			return e
		}
		_ = json.Unmarshal(raw, &v.Metadata)
		d.Events = append(d.Events, v)
	}
	rows.Close()
	d.ProviderOperational = true
	d.ProviderCapabilityNotice = "Финансовые операции подтверждаются платёжным провайдером."
	return nil
}
func scanDeal(s interface{ Scan(...any) error }) (Deal, error) {
	var d Deal
	e := s.Scan(&d.ID, &d.ProjectID, &d.AssignmentID, &d.CustomerUserID, &d.FreelancerUserID, &d.Currency, &d.GrossAmountKopecks, &d.PlatformFeeKopecks, &d.FreelancerAmountKopecks, &d.Status, &d.RevisionCount, &d.FundedAt, &d.WorkStartedAt, &d.SubmittedAt, &d.AcceptedAt, &d.CompletedAt, &d.CreatedAt, &d.UpdatedAt, &d.ConversationID,
		&d.Quote.WorkAmountKopecks,
		&d.Quote.PlatformFee.CustomerKopecks, &d.Quote.PlatformFee.FreelancerKopecks, &d.Quote.PlatformFee.PlatformKopecks,
		&d.Quote.ProviderFee.TotalKopecks, &d.Quote.ProviderFee.CustomerKopecks, &d.Quote.ProviderFee.FreelancerKopecks, &d.Quote.ProviderFee.PlatformKopecks,
		&d.Quote.CustomerDiscountKopecks, &d.Quote.FreelancerBonusKopecks,
		&d.Quote.PlatformProviderCostKopecks, &d.Quote.PlatformSubsidyKopecks, &d.Quote.PlatformNetRevenueKopecks,
		&d.Quote.FeeRuleVersion, &d.Quote.ProviderPricingVersion)
	if e != nil {
		return d, e
	}
	// Fill the quote fields that alias legacy columns so the whole snapshot is
	// reconstructed without recomputation: platform fee total == gross revenue,
	// gross amount == customer total, freelancer amount == payout.
	d.Quote.Currency = d.Currency
	d.Quote.PlatformFee.TotalKopecks = d.PlatformFeeKopecks
	d.Quote.PlatformGrossRevenueKopecks = d.PlatformFeeKopecks
	d.Quote.CustomerTotalKopecks = d.GrossAmountKopecks
	d.Quote.FreelancerPayoutKopecks = d.FreelancerAmountKopecks
	return d, e
}
func command(ctx context.Context, tx *sql.Tx, deal, actor, action, key string, hash []byte) (bool, error) {
	var stored []byte
	e := tx.QueryRowContext(ctx, `SELECT request_hash FROM safe_deal_command_results WHERE deal_id=$1 AND action=$2 AND idempotency_key=$3`, deal, action, key).Scan(&stored)
	if e == nil {
		if string(stored) != string(hash) {
			return false, ErrConflict
		}
		return true, nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return false, e
	}
	var actorValue any
	if actor != "" {
		actorValue = actor
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO safe_deal_command_results(deal_id,actor_user_id,action,idempotency_key,request_hash)VALUES($1,$2,$3,$4,$5)`, deal, actorValue, action, key, hash)
	return false, dbErr(e)
}
func appendEvent(ctx context.Context, tx *sql.Tx, deal, event, actor, operation string, meta map[string]string) error {
	if meta == nil {
		meta = map[string]string{}
	}
	raw, _ := json.Marshal(meta)
	var actorValue any
	if actor != "" {
		actorValue = actor
	}
	_, e := tx.ExecContext(ctx, `INSERT INTO safe_deal_events(deal_id,event_type,actor_user_id,operation_key,metadata)VALUES($1,$2,$3,$4,$5::jsonb)`, deal, event, actorValue, operation, string(raw))
	if e != nil {
		return dbErr(e)
	}
	payload, _ := json.Marshal(map[string]string{"deal_id": deal, "actor_user_id": actor})
	_, e = tx.ExecContext(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload)VALUES(gen_random_uuid(),'SAFE_DEAL',$1,$2,$3::jsonb)`, deal, event, string(payload))
	return e
}
func requestHash(v any) []byte { raw, _ := json.Marshal(v); x := sha256.Sum256(raw); return x[:] }
func isAdmin(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, actor string) bool {
	var ok bool
	_ = q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_roles ur JOIN users u ON u.id=ur.user_id WHERE ur.user_id=$1 AND ur.role IN('MODERATOR','ADMIN','SUPER_ADMIN')AND u.status='ACTIVE'AND u.deleted_at IS NULL)`, actor).Scan(&ok)
	return ok
}
func dbErr(e error) error {
	var pg *pgconn.PgError
	if errors.As(e, &pg) {
		if pg.Code == "23505" {
			return ErrConflict
		}
		if pg.Code == "23514" || pg.Code == "23503" {
			return ErrInvalid
		}
	}
	return e
}

var _ = sort.Strings
