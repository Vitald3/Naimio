package growth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"freelance/apps/api/internal/platform/requestmeta"
	"github.com/jackc/pgx/v5/pgconn"
	"strings"
	"time"
)

type PostgresRepository struct{ DB *sql.DB }

func (r PostgresRepository) CreateInvite(ctx context.Context, a string, in InviteInput, hash []byte, expires time.Time) (Invite, error) {
	var ownEmail string
	var active bool
	if e := r.DB.QueryRowContext(ctx, `SELECT email_normalized,status='ACTIVE'AND deleted_at IS NULL FROM users WHERE id=$1`, a).Scan(&ownEmail, &active); errors.Is(e, sql.ErrNoRows) || !active {
		return Invite{}, ErrUnauthorized
	} else if e != nil {
		return Invite{}, e
	}
	if in.IntendedEmail != "" && in.IntendedEmail == ownEmail {
		_, _ = r.DB.ExecContext(ctx, `INSERT INTO fraud_signals(id,user_id,entity_type,signal_type,severity,evidence)VALUES(gen_random_uuid(),$1,'INVITE','SELF_REFERRAL_ATTEMPT',2,'{}')`, a)
		return Invite{}, ErrInvalid
	}
	allowed, e := r.inviteAllowed(ctx, a, in)
	if e != nil {
		return Invite{}, e
	}
	if !allowed {
		return Invite{}, ErrNotFound
	}
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return Invite{}, e
	}
	defer tx.Rollback()
	var v Invite
	var p sql.NullString
	e = tx.QueryRowContext(ctx, `INSERT INTO invites(inviter_user_id,invite_type,project_id,token_hash,intended_email,expires_at)VALUES($1,$2,NULLIF($3::text,'')::uuid,$4,NULLIF($5,''),$6)RETURNING id::text,invite_type,project_id::text,expires_at,created_at`, a, in.Type, in.ProjectID, hash, in.IntendedEmail, expires).Scan(&v.ID, &v.Type, &p, &v.ExpiresAt, &v.CreatedAt)
	if e != nil {
		return Invite{}, dbError(e)
	}
	if p.Valid {
		v.ProjectID = &p.String
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload)VALUES(gen_random_uuid(),'INVITE',$1,'INVITE_CREATED',jsonb_build_object('inviter_user_id',$2::text,'invite_type',$3::text))`, v.ID, a, v.Type); e != nil {
		return Invite{}, e
	}
	if e = tx.Commit(); e != nil {
		return Invite{}, e
	}
	return v, nil
}
func (r PostgresRepository) inviteAllowed(ctx context.Context, a string, in InviteInput) (bool, error) {
	var ok bool
	if in.Type == "CUSTOMER" && in.ProjectID == "" {
		e := r.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_capabilities WHERE user_id=$1 AND capability='FREELANCER')`, a).Scan(&ok)
		return ok, e
	}
	if in.Type == "CUSTOMER" {
		e := r.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM projects p JOIN project_assignments pa ON pa.project_id=p.id AND pa.freelancer_user_id=$1 AND pa.status IN('ACTIVE','COMPLETED')WHERE p.id=$2 AND p.deleted_at IS NULL)`, a, in.ProjectID).Scan(&ok)
		return ok, e
	}
	e := r.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM projects p JOIN user_capabilities uc ON uc.user_id=$1 AND uc.capability='CUSTOMER' WHERE p.id=$2 AND p.customer_user_id=$1 AND p.deleted_at IS NULL AND p.status IN('DRAFT','OPEN','MATCHING'))`, a, in.ProjectID).Scan(&ok)
	return ok, e
}
func (r PostgresRepository) Preview(ctx context.Context, hash []byte) (Preview, error) {
	var v Preview
	var p sql.NullString
	e := r.DB.QueryRowContext(ctx, `SELECT i.invite_type,i.project_id::text,COALESCE(p.title,''),COALESCE(c.name,''),u.display_name,CASE WHEN i.invite_type='CUSTOMER'THEN'CUSTOMER'ELSE'FREELANCER'END,i.expires_at,i.accepted_at IS NOT NULL FROM invites i JOIN users u ON u.id=i.inviter_user_id LEFT JOIN projects p ON p.id=i.project_id LEFT JOIN categories c ON c.id=p.category_id WHERE i.token_hash=$1 AND i.expires_at>now()`, hash).Scan(&v.Type, &p, &v.ProjectTitle, &v.CategoryName, &v.InviterDisplayName, &v.InvitedRole, &v.ExpiresAt, &v.Accepted)
	if errors.Is(e, sql.ErrNoRows) {
		return Preview{}, ErrNotFound
	}
	if p.Valid {
		v.ProjectID = &p.String
	}
	return v, e
}
func (r PostgresRepository) Accept(ctx context.Context, a string, hash []byte, key string) (Acceptance, error) {
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return Acceptance{}, e
	}
	defer tx.Rollback()
	var id, kind, inviter, intended, email string
	var project, accepted sql.NullString
	var acceptedAt sql.NullTime
	var expires time.Time
	e = tx.QueryRowContext(ctx, `SELECT i.id::text,i.invite_type,i.inviter_user_id::text,COALESCE(i.intended_email,''),i.project_id::text,i.accepted_by_user_id::text,i.accepted_at,i.expires_at,u.email_normalized FROM invites i JOIN users u ON u.id=$2 AND u.status='ACTIVE'AND u.deleted_at IS NULL WHERE i.token_hash=$1 FOR UPDATE`, hash, a).Scan(&id, &kind, &inviter, &intended, &project, &accepted, &acceptedAt, &expires, &email)
	if errors.Is(e, sql.ErrNoRows) {
		return Acceptance{}, ErrNotFound
	}
	if e != nil {
		return Acceptance{}, e
	}
	if !expires.After(time.Now()) {
		return Acceptance{}, ErrNotFound
	}
	if inviter == a {
		_, _ = tx.ExecContext(ctx, `INSERT INTO fraud_signals(id,user_id,entity_type,entity_id,signal_type,severity,evidence)VALUES(gen_random_uuid(),$1,'INVITE',$2,'SELF_REFERRAL_ATTEMPT',2,jsonb_build_object('idempotency_key_hash',encode(digest($3,'sha256'),'hex')))`, a, id, key)
		if e = tx.Commit(); e != nil {
			return Acceptance{}, e
		}
		return Acceptance{}, ErrInvalid
	}
	if intended != "" && intended != email {
		return Acceptance{}, ErrNotFound
	}
	if accepted.Valid {
		if accepted.String != a {
			return Acceptance{}, ErrNotFound
		}
		var p *string
		if project.Valid {
			p = &project.String
		}
		return Acceptance{InviteID: id, Type: kind, ProjectID: p, AcceptedAt: acceptedAt.Time}, nil
	}
	now := time.Now().UTC()
	if _, e = tx.ExecContext(ctx, `UPDATE invites SET accepted_by_user_id=$2,accepted_at=$3 WHERE id=$1`, id, a, now); e != nil {
		return Acceptance{}, e
	}
	capability := "FREELANCER"
	if kind == "CUSTOMER" {
		capability = "CUSTOMER"
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO user_capabilities(user_id,capability)VALUES($1,$2)ON CONFLICT DO NOTHING`, a, capability); e != nil {
		return Acceptance{}, e
	}
	if project.Valid && kind != "CUSTOMER" {
		if _, e = tx.ExecContext(ctx, `INSERT INTO project_invited_users(project_id,user_id,invite_id,invited_role,accepted_at)VALUES($1,$2,$3,'FREELANCER',$4)ON CONFLICT(project_id,user_id)DO NOTHING`, project.String, a, id, now); e != nil {
			return Acceptance{}, e
		}
	}
	res, e := tx.ExecContext(ctx, `INSERT INTO referral_attributions(inviter_user_id,invited_user_id,invite_id,first_touch_at,source)VALUES($1,$2,$3,$4,'INVITE')ON CONFLICT(invited_user_id)DO NOTHING`, inviter, a, id, now)
	if e != nil {
		return Acceptance{}, e
	}
	attribution, _ := res.RowsAffected()
	rewardRes, e := tx.ExecContext(ctx, `
INSERT INTO reward_ledger(user_id,rule_id,event_key,reward_type,amount,unit,expires_at)
SELECT CASE rr.beneficiary WHEN 'INVITER' THEN $1::uuid ELSE $2::uuid END,
       rr.id,
       rr.code || ':' || $3::text,
       rr.reward_type,
       rr.reward_value,
       rr.reward_unit,
       CASE WHEN rr.config ? 'valid_days'
            THEN now() + make_interval(days => LEAST(3650, GREATEST(1, (rr.config->>'valid_days')::int)))
       END
FROM referral_rules rr
WHERE rr.enabled
  AND rr.event_type='INVITE_ACCEPTED'
  AND (rr.starts_at IS NULL OR rr.starts_at<=now())
  AND (rr.ends_at IS NULL OR rr.ends_at>now())
  AND (rr.max_uses IS NULL OR (SELECT count(*) FROM reward_ledger rl WHERE rl.rule_id=rr.id)<rr.max_uses)
ON CONFLICT(user_id,event_key) DO NOTHING`, inviter, a, id)
	if e != nil {
		return Acceptance{}, e
	}
	rewards, _ := rewardRes.RowsAffected()
	payload, _ := json.Marshal(map[string]any{"inviter_user_id": inviter, "accepted_user_id": a, "invite_type": kind, "project_id": project.String})
	if _, e = tx.ExecContext(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload)VALUES(gen_random_uuid(),'INVITE',$1,'INVITE_ACCEPTED',$2::jsonb)`, id, string(payload)); e != nil {
		return Acceptance{}, e
	}
	if rewards > 0 {
		_, e = tx.ExecContext(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload)VALUES(gen_random_uuid(),'INVITE',$1,'REWARD_GRANTED',jsonb_build_object('inviter_user_id',$2::text,'accepted_user_id',$3::text))`, id, inviter, a)
		if e != nil {
			return Acceptance{}, e
		}
	}
	if e = tx.Commit(); e != nil {
		return Acceptance{}, e
	}
	var p *string
	if project.Valid {
		p = &project.String
	}
	return Acceptance{InviteID: id, Type: kind, ProjectID: p, AcceptedAt: now, AttributionCreated: attribution == 1, RewardsIssued: int(rewards)}, nil
}
func (r PostgresRepository) Referrals(ctx context.Context, a string) (Referrals, error) {
	var out Referrals
	var v Attribution
	var invite sql.NullString
	e := r.DB.QueryRowContext(ctx, `SELECT id::text,inviter_user_id::text,invite_id::text,COALESCE(source,''),first_touch_at FROM referral_attributions WHERE invited_user_id=$1`, a).Scan(&v.ID, &v.InviterUserID, &invite, &v.Source, &v.FirstTouchAt)
	if e == nil {
		if invite.Valid {
			v.InviteID = invite.String
		}
		out.Attribution = &v
	} else if !errors.Is(e, sql.ErrNoRows) {
		return out, e
	}
	rows, e := r.DB.QueryContext(ctx, `SELECT rl.id::text,COALESCE(rr.code,''),rl.event_key,rl.reward_type,rl.amount,rl.unit,rl.expires_at,rl.created_at FROM reward_ledger rl LEFT JOIN referral_rules rr ON rr.id=rl.rule_id WHERE rl.user_id=$1 ORDER BY rl.created_at DESC,rl.id DESC LIMIT 100`, a)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	out.Rewards = []Reward{}
	for rows.Next() {
		var x Reward
		if e = rows.Scan(&x.ID, &x.RuleCode, &x.EventKey, &x.RewardType, &x.Amount, &x.Unit, &x.ExpiresAt, &x.CreatedAt); e != nil {
			return out, e
		}
		out.Rewards = append(out.Rewards, x)
	}
	return out, rows.Err()
}
func (r PostgresRepository) Rules(ctx context.Context, a string) ([]Rule, error) {
	if ok, e := r.admin(ctx, a); e != nil {
		return nil, e
	} else if !ok {
		return nil, ErrForbidden
	}
	rows, e := r.DB.QueryContext(ctx, `SELECT id::text,code,event_type,beneficiary,reward_type,reward_value,reward_unit,max_uses,starts_at,ends_at,enabled,config,created_at,updated_at FROM referral_rules ORDER BY created_at DESC,id DESC`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Rule{}
	for rows.Next() {
		v, e := scanRule(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r PostgresRepository) CreateRule(ctx context.Context, a string, in RuleInput) (Rule, error) {
	if ok, e := r.admin(ctx, a); e != nil {
		return Rule{}, e
	} else if !ok {
		return Rule{}, ErrForbidden
	}
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return Rule{}, e
	}
	defer tx.Rollback()
	raw, _ := json.Marshal(in.Config)
	v, e := scanRule(tx.QueryRowContext(ctx, `INSERT INTO referral_rules(code,event_type,beneficiary,reward_type,reward_value,reward_unit,max_uses,starts_at,ends_at,enabled,config)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb)RETURNING id::text,code,event_type,beneficiary,reward_type,reward_value,reward_unit,max_uses,starts_at,ends_at,enabled,config,created_at,updated_at`, in.Code, in.EventType, in.Beneficiary, in.RewardType, in.RewardValue, in.RewardUnit, in.MaxUses, in.StartsAt, in.EndsAt, in.Enabled, string(raw)))
	if e != nil {
		return Rule{}, dbError(e)
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO audit_logs(id,actor_user_id,action,target_type,target_id,metadata,ip)VALUES(gen_random_uuid(),$1,'REFERRAL_RULE_CREATED','REFERRAL_RULE',$2,jsonb_build_object('code',$3::text),NULLIF($4::text,'')::inet)`, a, v.ID, v.Code, requestmeta.FromContext(ctx)); e != nil {
		return Rule{}, e
	}
	if e = tx.Commit(); e != nil {
		return Rule{}, e
	}
	return v, nil
}
func (r PostgresRepository) UpdateRule(ctx context.Context, a, id string, in RuleInput) (Rule, error) {
	if ok, e := r.admin(ctx, a); e != nil {
		return Rule{}, e
	} else if !ok {
		return Rule{}, ErrForbidden
	}
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return Rule{}, e
	}
	defer tx.Rollback()
	raw, _ := json.Marshal(in.Config)
	v, e := scanRule(tx.QueryRowContext(ctx, `UPDATE referral_rules SET code=$3,event_type=$4,beneficiary=$5,reward_type=$6,reward_value=$7,reward_unit=$8,max_uses=$9,starts_at=$10,ends_at=$11,enabled=$12,config=$13::jsonb,updated_at=now()WHERE id=$1 RETURNING id::text,code,event_type,beneficiary,reward_type,reward_value,reward_unit,max_uses,starts_at,ends_at,enabled,config,created_at,updated_at`, id, a, in.Code, in.EventType, in.Beneficiary, in.RewardType, in.RewardValue, in.RewardUnit, in.MaxUses, in.StartsAt, in.EndsAt, in.Enabled, string(raw)))
	if errors.Is(e, sql.ErrNoRows) {
		return Rule{}, ErrNotFound
	}
	if e != nil {
		return Rule{}, dbError(e)
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO audit_logs(id,actor_user_id,action,target_type,target_id,metadata,ip)VALUES(gen_random_uuid(),$1,'REFERRAL_RULE_UPDATED','REFERRAL_RULE',$2,jsonb_build_object('code',$3::text),NULLIF($4::text,'')::inet)`, a, v.ID, v.Code, requestmeta.FromContext(ctx)); e != nil {
		return Rule{}, e
	}
	if e = tx.Commit(); e != nil {
		return Rule{}, e
	}
	return v, nil
}

type scanner interface{ Scan(...any) error }

func scanRule(s scanner) (Rule, error) {
	var v Rule
	var raw []byte
	e := s.Scan(&v.ID, &v.Code, &v.EventType, &v.Beneficiary, &v.RewardType, &v.RewardValue, &v.RewardUnit, &v.MaxUses, &v.StartsAt, &v.EndsAt, &v.Enabled, &raw, &v.CreatedAt, &v.UpdatedAt)
	if e == nil {
		e = json.Unmarshal(raw, &v.Config)
	}
	return v, e
}
func (r PostgresRepository) Team(ctx context.Context, a string) ([]TeamMember, error) {
	if ok, e := r.capability(ctx, a, "CUSTOMER"); e != nil {
		return nil, e
	} else if !ok {
		return nil, ErrNotFound
	}
	rows, e := r.DB.QueryContext(ctx, `SELECT tm.freelancer_user_id::text,COALESCE(u.username,''),u.display_name,pp.availability,COALESCE(pp.professional_title,''),uts.native_rating,COALESCE(uts.reviews_count,0),COALESCE(tm.label,''),COALESCE(tm.notes,''),COALESCE(last_project.id::text,''),COALESCE(last_project.title,''),tm.created_at,tm.updated_at FROM customer_team_members tm JOIN users u ON u.id=tm.freelancer_user_id AND u.status='ACTIVE'AND u.deleted_at IS NULL JOIN professional_profiles pp ON pp.user_id=u.id LEFT JOIN user_trust_stats uts ON uts.user_id=u.id LEFT JOIN LATERAL(SELECT p.id,p.title FROM projects p JOIN project_assignments pa ON pa.project_id=p.id AND pa.freelancer_user_id=tm.freelancer_user_id WHERE p.customer_user_id=tm.customer_user_id AND p.status='COMPLETED'ORDER BY p.updated_at DESC LIMIT 1)last_project ON true WHERE tm.customer_user_id=$1 ORDER BY tm.updated_at DESC,tm.freelancer_user_id DESC`, a)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []TeamMember{}
	for rows.Next() {
		var v TeamMember
		if e = rows.Scan(&v.FreelancerUserID, &v.Username, &v.DisplayName, &v.Availability, &v.ProfessionalTitle, &v.NativeRating, &v.ReviewsCount, &v.Label, &v.Notes, &v.LastProjectID, &v.LastProjectTitle, &v.CreatedAt, &v.UpdatedAt); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r PostgresRepository) PutTeam(ctx context.Context, a, f string, in TeamInput) (TeamMember, error) {
	if ok, e := r.capability(ctx, a, "CUSTOMER"); e != nil {
		return TeamMember{}, e
	} else if !ok {
		return TeamMember{}, ErrNotFound
	}
	var eligible bool
	e := r.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN user_capabilities uc ON uc.user_id=u.id AND uc.capability='FREELANCER'JOIN professional_profiles pp ON pp.user_id=u.id WHERE u.id=$1 AND u.status='ACTIVE'AND u.deleted_at IS NULL AND pp.profile_visibility='PUBLIC')`, f).Scan(&eligible)
	if e != nil {
		return TeamMember{}, e
	}
	if !eligible {
		return TeamMember{}, ErrNotFound
	}
	_, e = r.DB.ExecContext(ctx, `INSERT INTO customer_team_members(customer_user_id,freelancer_user_id,label,notes)VALUES($1,$2,NULLIF($3,''),NULLIF($4,''))ON CONFLICT(customer_user_id,freelancer_user_id)DO UPDATE SET label=EXCLUDED.label,notes=EXCLUDED.notes,updated_at=now()`, a, f, in.Label, in.Notes)
	if e != nil {
		return TeamMember{}, dbError(e)
	}
	items, e := r.Team(ctx, a)
	if e != nil {
		return TeamMember{}, e
	}
	for _, v := range items {
		if v.FreelancerUserID == f {
			return v, nil
		}
	}
	return TeamMember{}, ErrNotFound
}
func (r PostgresRepository) DeleteTeam(ctx context.Context, a, f string) error {
	res, e := r.DB.ExecContext(ctx, `DELETE FROM customer_team_members WHERE customer_user_id=$1 AND freelancer_user_id=$2`, a, f)
	if e != nil {
		return e
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
func (r PostgresRepository) Repeat(ctx context.Context, a, id string, in RepeatInput, hash []byte, expires time.Time) (RepeatResult, error) {
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return RepeatResult{}, e
	}
	defer tx.Rollback()
	var newID, slug string
	e = tx.QueryRowContext(ctx, `INSERT INTO projects(id,customer_user_id,category_id,title,slug,description,budget_type,budget_min_kopecks,budget_max_kopecks,currency,deadline_at,experience_level,visibility,status,source_type)SELECT gen_random_uuid(),customer_user_id,category_id,title,LEFT(slug||'-repeat-'||substr(replace(gen_random_uuid()::text,'-',''),1,8),240),description,budget_type,budget_min_kopecks,budget_max_kopecks,currency,NULL,experience_level,visibility,'DRAFT','REPEAT'FROM projects WHERE id=$1 AND customer_user_id=$2 AND status='COMPLETED'AND deleted_at IS NULL RETURNING id::text,slug`, id, a).Scan(&newID, &slug)
	if errors.Is(e, sql.ErrNoRows) {
		return RepeatResult{}, ErrNotFound
	}
	if e != nil {
		return RepeatResult{}, dbError(e)
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO project_skills(project_id,skill_id,importance)SELECT $1,skill_id,importance FROM project_skills WHERE project_id=$2`, newID, id); e != nil {
		return RepeatResult{}, e
	}
	result := RepeatResult{ProjectID: newID, SourceProjectID: id, Status: "DRAFT", SourceType: "REPEAT"}
	if in.InvitePreviousFreelancer {
		var freelancer, email string
		e = tx.QueryRowContext(ctx, `SELECT pa.freelancer_user_id::text,u.email_normalized FROM project_assignments pa JOIN users u ON u.id=pa.freelancer_user_id WHERE pa.project_id=$1 AND pa.status='COMPLETED'ORDER BY pa.completed_at DESC LIMIT 1`, id).Scan(&freelancer, &email)
		if errors.Is(e, sql.ErrNoRows) {
			return RepeatResult{}, ErrNotFound
		}
		if e != nil {
			return RepeatResult{}, e
		}
		var invite Invite
		e = tx.QueryRowContext(ctx, `INSERT INTO invites(inviter_user_id,invite_type,project_id,token_hash,intended_email,expires_at)VALUES($1,'FREELANCER',$2,$3,$4,$5)RETURNING id::text,invite_type,project_id::text,expires_at,created_at`, a, newID, hash, email, expires).Scan(&invite.ID, &invite.Type, &newID, &invite.ExpiresAt, &invite.CreatedAt)
		if e != nil {
			return RepeatResult{}, e
		}
		invite.ProjectID = &newID
		result.Invite = &CreatedInvite{Invite: invite}
		if _, e = tx.ExecContext(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload)VALUES(gen_random_uuid(),'INVITE',$1,'PROJECT_INVITE_CREATED',jsonb_build_object('invited_user_id',$2::text,'project_id',$3::text))`, invite.ID, freelancer, newID); e != nil {
			return RepeatResult{}, e
		}
	}
	payload, _ := json.Marshal(map[string]string{"source_project_id": id, "project_id": newID})
	if _, e = tx.ExecContext(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload)VALUES(gen_random_uuid(),'PROJECT',$1,'REPEAT_PROJECT_CREATED',$2::jsonb)`, newID, string(payload)); e != nil {
		return RepeatResult{}, e
	}
	if e = tx.Commit(); e != nil {
		return RepeatResult{}, e
	}
	return result, nil
}
func (r PostgresRepository) Share(ctx context.Context, a, id, base string) (ShareResult, error) {
	var slug string
	e := r.DB.QueryRowContext(ctx, `SELECT slug FROM projects WHERE id=$1 AND customer_user_id=$2 AND visibility='PUBLIC'AND status IN('OPEN','MATCHING')AND deleted_at IS NULL`, id, a).Scan(&slug)
	if errors.Is(e, sql.ErrNoRows) {
		return ShareResult{}, ErrNotFound
	}
	if e != nil {
		return ShareResult{}, e
	}
	_, _ = r.DB.ExecContext(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload)VALUES(gen_random_uuid(),'PROJECT',$1,'PUBLIC_PROJECT_SHARED','{}')`, id)
	return ShareResult{ProjectID: id, URL: base + "/projects/" + slug}, nil
}
func (r PostgresRepository) InvitedProject(ctx context.Context, a, id string) (InvitedProject, error) {
	var v InvitedProject
	e := r.DB.QueryRowContext(ctx, `SELECT p.id::text,p.title,p.description,COALESCE(c.name,''),p.visibility,p.status,u.display_name FROM project_invited_users piu JOIN projects p ON p.id=piu.project_id AND p.deleted_at IS NULL AND p.status IN('DRAFT','OPEN','MATCHING')JOIN users u ON u.id=p.customer_user_id LEFT JOIN categories c ON c.id=p.category_id WHERE piu.project_id=$1 AND piu.user_id=$2 AND piu.invited_role='FREELANCER'`, id, a).Scan(&v.ID, &v.Title, &v.Description, &v.CategoryName, &v.Visibility, &v.Status, &v.CustomerDisplayName)
	if errors.Is(e, sql.ErrNoRows) {
		return InvitedProject{}, ErrNotFound
	}
	return v, e
}
func (r PostgresRepository) admin(ctx context.Context, a string) (bool, error) {
	var ok bool
	e := r.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN user_roles ur ON ur.user_id=u.id WHERE u.id=$1 AND u.status='ACTIVE'AND u.deleted_at IS NULL AND ur.role IN('ADMIN','SUPER_ADMIN'))`, a).Scan(&ok)
	return ok, e
}
func (r PostgresRepository) capability(ctx context.Context, a, c string) (bool, error) {
	var ok bool
	e := r.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN user_capabilities uc ON uc.user_id=u.id WHERE u.id=$1 AND u.status='ACTIVE'AND u.deleted_at IS NULL AND uc.capability=$2)`, a, c).Scan(&ok)
	return ok, e
}
func dbError(e error) error {
	var p *pgconn.PgError
	if errors.As(e, &p) && p.Code == "23505" {
		return ErrConflict
	}
	return e
}

var _ = fmt.Sprintf
var _ = strings.TrimSpace
