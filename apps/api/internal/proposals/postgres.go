package proposals

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"strings"
	"time"
)

type PostgresRepository struct{ DB *sql.DB }

const columns = `p.id::text,p.project_id::text,pr.title,p.freelancer_user_id::text,u.display_name,p.message,p.price_kopecks,p.currency,p.delivery_days,p.status,p.submitted_at,p.updated_at,p.withdrawn_at,COALESCE((SELECT sd.id::text FROM project_assignments pa JOIN safe_deals sd ON sd.assignment_id=pa.id WHERE pa.proposal_id=p.id ORDER BY sd.created_at DESC LIMIT 1),'')`

func (r PostgresRepository) Submit(ctx context.Context, actor, projectID string, in Input) (Proposal, error) {
	if !uuidPattern.MatchString(strings.ToLower(projectID)) {
		return Proposal{}, ErrInvalid
	}
	if err := validate(in); err != nil {
		return Proposal{}, err
	}
	id, err := uuid()
	if err != nil {
		return Proposal{}, err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Proposal{}, err
	}
	defer tx.Rollback()
	var allowed bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM projects p JOIN users u ON u.id=$1 AND u.status='ACTIVE' AND u.deleted_at IS NULL JOIN user_capabilities uc ON uc.user_id=u.id AND uc.capability='FREELANCER' LEFT JOIN professional_profiles pp ON pp.user_id=u.id WHERE p.id=$2 AND p.status IN('OPEN','MATCHING') AND p.deleted_at IS NULL AND p.customer_user_id<>u.id AND pp.profile_visibility='PUBLIC'AND(p.visibility='PUBLIC'OR EXISTS(SELECT 1 FROM project_invited_users piu WHERE piu.project_id=p.id AND piu.user_id=u.id AND piu.invited_role='FREELANCER')))`, actor, projectID).Scan(&allowed)
	if err != nil {
		return Proposal{}, err
	}
	if !allowed {
		return Proposal{}, ErrIneligible
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO proposals(id,project_id,freelancer_user_id,message,price_kopecks,currency,delivery_days)VALUES($1,$2,$3,trim($4),$5,'RUB',$6)`, id, projectID, actor, in.Message, in.PriceKopecks, in.DeliveryDays)
	if err != nil {
		return Proposal{}, dbError(err)
	}
	_, err = tx.ExecContext(ctx, `UPDATE projects SET proposal_count=proposal_count+1,updated_at=now() WHERE id=$1`, projectID)
	if err != nil {
		return Proposal{}, err
	}
	payload, _ := json.Marshal(map[string]string{"project_id": projectID, "proposal_id": id, "actor_user_id": actor})
	if _, err = tx.ExecContext(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload)VALUES(gen_random_uuid(),'PROJECT',$1,'PROPOSAL_CREATED',$2::jsonb)`, projectID, string(payload)); err != nil {
		return Proposal{}, err
	}
	if err = tx.Commit(); err != nil {
		return Proposal{}, err
	}
	return r.get(ctx, `p.id=$1 AND p.freelancer_user_id=$2`, id, actor)
}
func (r PostgresRepository) ListMine(ctx context.Context, actor string, c *Cursor, l int) (Page, error) {
	return r.list(ctx, `p.freelancer_user_id=$1`, actor, "", c, l, false)
}
func (r PostgresRepository) GetMine(ctx context.Context, actor, id string) (Proposal, error) {
	return r.get(ctx, `p.id=$1 AND p.freelancer_user_id=$2`, id, actor)
}
func (r PostgresRepository) Update(ctx context.Context, actor, id string, in Input) (Proposal, error) {
	if err := validate(in); err != nil {
		return Proposal{}, err
	}
	result, err := r.DB.ExecContext(ctx, `UPDATE proposals SET message=trim($3),price_kopecks=$4,currency='RUB',delivery_days=$5,updated_at=now() WHERE id=$1 AND freelancer_user_id=$2 AND status='PENDING'`, id, actor, in.Message, in.PriceKopecks, in.DeliveryDays)
	if err != nil {
		return Proposal{}, dbError(err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		if _, e := r.GetMine(ctx, actor, id); e != nil {
			return Proposal{}, e
		}
		return Proposal{}, ErrInvalidState
	}
	return r.GetMine(ctx, actor, id)
}
func (r PostgresRepository) Withdraw(ctx context.Context, actor, id string) (Proposal, error) {
	result, err := r.DB.ExecContext(ctx, `UPDATE proposals SET status='WITHDRAWN',withdrawn_at=COALESCE(withdrawn_at,now()),updated_at=now() WHERE id=$1 AND freelancer_user_id=$2 AND status IN('PENDING','SHORTLISTED')`, id, actor)
	if err != nil {
		return Proposal{}, err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		v, e := r.GetMine(ctx, actor, id)
		if e != nil {
			return Proposal{}, e
		}
		if v.Status == "WITHDRAWN" {
			return v, nil
		}
		return Proposal{}, ErrInvalidState
	}
	return r.GetMine(ctx, actor, id)
}
func (r PostgresRepository) ListForProject(ctx context.Context, actor, projectID string, c *Cursor, l int) (Page, error) {
	var owns bool
	if err := r.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM projects WHERE id=$1 AND customer_user_id=$2 AND deleted_at IS NULL)`, projectID, actor).Scan(&owns); err != nil {
		return Page{}, err
	}
	if !owns {
		return Page{}, ErrNotFound
	}
	return r.list(ctx, `p.project_id=$1`, projectID, "", c, l, true)
}
func (r PostgresRepository) Act(ctx context.Context, actor, projectID, id, action string) (Proposal, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Proposal{}, err
	}
	defer tx.Rollback()
	var owner, status string
	err = tx.QueryRowContext(ctx, `SELECT customer_user_id::text,status FROM projects WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, projectID).Scan(&owner, &status)
	if errors.Is(err, sql.ErrNoRows) || owner != actor {
		return Proposal{}, ErrNotFound
	}
	if err != nil {
		return Proposal{}, err
	}
	var current string
	err = tx.QueryRowContext(ctx, `SELECT status FROM proposals WHERE id=$1 AND project_id=$2 FOR UPDATE`, id, projectID).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return Proposal{}, ErrNotFound
	}
	if err != nil {
		return Proposal{}, err
	}
	if action == "accept" && current == "ACCEPTED" {
		if err = ensureProjectConversation(ctx, tx, projectID, id); err != nil {
			return Proposal{}, err
		}
		if err = tx.Commit(); err != nil {
			return Proposal{}, dbError(err)
		}
		return r.get(ctx, `p.id=$1 AND p.project_id=$2`, id, projectID)
	}
	target := ""
	switch action {
	case "shortlist":
		if current != "PENDING" {
			return Proposal{}, ErrInvalidState
		}
		target = "SHORTLISTED"
	case "reject":
		if current != "PENDING" && current != "SHORTLISTED" {
			return Proposal{}, ErrInvalidState
		}
		target = "REJECTED"
	case "accept":
		if status != "OPEN" && status != "MATCHING" || current != "PENDING" && current != "SHORTLISTED" {
			return Proposal{}, ErrInvalidState
		}
		var price sql.NullInt64
		if err = tx.QueryRowContext(ctx, `SELECT price_kopecks FROM proposals WHERE id=$1`, id).Scan(&price); err != nil || !price.Valid || price.Int64 <= 0 {
			return Proposal{}, ErrInvalid
		}
		target = "ACCEPTED"
	default:
		return Proposal{}, ErrInvalidState
	}
	_, err = tx.ExecContext(ctx, `UPDATE proposals SET status=$2,updated_at=now() WHERE id=$1`, id, target)
	if err != nil {
		return Proposal{}, dbError(err)
	}
	if action == "accept" {
		assignmentID, e := uuid()
		if e != nil {
			return Proposal{}, e
		}
		eventID, e := uuid()
		if e != nil {
			return Proposal{}, e
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO project_assignments(id,project_id,proposal_id,freelancer_user_id,agreed_price_kopecks,currency,agreed_deadline_at,started_at) SELECT $1,p.project_id,p.id,p.freelancer_user_id,p.price_kopecks,'RUB',pr.deadline_at,NULL FROM proposals p JOIN projects pr ON pr.id=p.project_id WHERE p.id=$2`, assignmentID, id)
		if err != nil {
			return Proposal{}, dbError(err)
		}
		if _, err = tx.ExecContext(ctx, `UPDATE projects SET status='AWAITING_FUNDING',updated_at=now() WHERE id=$1`, projectID); err != nil {
			return Proposal{}, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE proposals SET status='REJECTED',updated_at=now() WHERE project_id=$1 AND id<>$2 AND status IN('PENDING','SHORTLISTED')`, projectID, id); err != nil {
			return Proposal{}, err
		}
		payload, _ := json.Marshal(map[string]string{"project_id": projectID, "proposal_id": id})
		_, err = tx.ExecContext(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload)VALUES($1,'PROJECT',$2,'PROPOSAL_ACCEPTED',$3::jsonb)`, eventID, projectID, string(payload))
		if err != nil {
			return Proposal{}, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO safe_deal_events(deal_id,event_type,actor_user_id,operation_key,metadata)SELECT id,'DEAL_FUNDING_REQUIRED',$2,'created:'||assignment_id::text,'{}'::jsonb FROM safe_deals WHERE assignment_id=$1`, assignmentID, actor); err != nil {
			return Proposal{}, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload)SELECT gen_random_uuid(),'SAFE_DEAL',id,'DEAL_FUNDING_REQUIRED',jsonb_build_object('deal_id',id::text,'actor_user_id',$2::text)FROM safe_deals WHERE assignment_id=$1`, assignmentID, actor); err != nil {
			return Proposal{}, err
		}
		if err = ensureProjectConversation(ctx, tx, projectID, id); err != nil {
			return Proposal{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return Proposal{}, dbError(err)
	}
	return r.get(ctx, `p.id=$1 AND p.project_id=$2`, id, projectID)
}

func ensureProjectConversation(ctx context.Context, tx *sql.Tx, projectID, proposalID string) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO conversations(kind,project_id)VALUES('PROJECT',$1)ON CONFLICT(project_id)WHERE project_id IS NOT NULL DO NOTHING`, projectID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO conversation_members(conversation_id,user_id)SELECT c.id,p.customer_user_id FROM conversations c JOIN projects p ON p.id=c.project_id WHERE c.project_id=$1 ON CONFLICT DO NOTHING`, projectID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO conversation_members(conversation_id,user_id)SELECT c.id,p.freelancer_user_id FROM conversations c JOIN proposals p ON p.project_id=c.project_id WHERE c.project_id=$1 AND p.id=$2 ON CONFLICT DO NOTHING`, projectID, proposalID)
	return err
}
func (r PostgresRepository) get(ctx context.Context, condition string, args ...any) (Proposal, error) {
	return scan(r.DB.QueryRowContext(ctx, `SELECT `+columns+` FROM proposals p JOIN projects pr ON pr.id=p.project_id JOIN users u ON u.id=p.freelancer_user_id WHERE `+condition, args...))
}
func (r PostgresRepository) list(ctx context.Context, condition string, a, b string, c *Cursor, l int, customer bool) (Page, error) {
	if l < 1 {
		l = 20
	}
	if l > 50 {
		l = 50
	}
	var at, curID any
	if c != nil {
		at, curID = c.At, c.ID
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT `+columns+` FROM proposals p JOIN projects pr ON pr.id=p.project_id JOIN users u ON u.id=p.freelancer_user_id WHERE `+condition+` AND ($2::timestamptz IS NULL OR(p.submitted_at,p.id)<($2,$3::uuid)) ORDER BY p.submitted_at DESC,p.id DESC LIMIT $4`, a, at, curID, l+1)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	items := []Proposal{}
	for rows.Next() {
		v, e := scan(rows)
		if e != nil {
			return Page{}, e
		}
		if !customer {
			v.FreelancerDisplayName = ""
		}
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		return Page{}, err
	}
	p := Page{Items: items}
	if len(items) > l {
		last := items[l-1]
		p.Items = items[:l]
		p.NextCursor = &Cursor{At: last.SubmittedAt, ID: last.ID}
	}
	return p, nil
}

type scanner interface{ Scan(...any) error }

func scan(s scanner) (Proposal, error) {
	var v Proposal
	err := s.Scan(&v.ID, &v.ProjectID, &v.ProjectTitle, &v.FreelancerID, &v.FreelancerDisplayName, &v.Message, &v.PriceKopecks, &v.Currency, &v.DeliveryDays, &v.Status, &v.SubmittedAt, &v.UpdatedAt, &v.WithdrawnAt, &v.SafeDealID)
	if errors.Is(err, sql.ErrNoRows) {
		return Proposal{}, ErrNotFound
	}
	return v, err
}
func dbError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		if pg.Code == "23505" {
			return ErrConflict
		}
		if pg.Code == "23514" || pg.Code == "23503" {
			return ErrInvalid
		}
	}
	return err
}

var _ = time.UTC
