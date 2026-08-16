package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"freelance/apps/api/internal/platform/requestmeta"
	"freelance/apps/api/internal/platform/supportnotify"
)

type PostgresRepository struct{ DB *sql.DB }

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

func (r PostgresRepository) Dashboard(ctx context.Context) (Dashboard, error) {
	var d Dashboard
	err := r.DB.QueryRowContext(ctx, `SELECT
 (SELECT count(*) FROM users WHERE deleted_at IS NULL),
 (SELECT count(*) FROM users WHERE deleted_at IS NULL AND created_at>=now()-interval '7 days'),
 (SELECT count(*) FROM projects WHERE deleted_at IS NULL AND status IN('OPEN','MATCHING','AWAITING_FUNDING','IN_PROGRESS')),
 (SELECT count(*) FROM projects WHERE deleted_at IS NULL AND status IN('OPEN','MATCHING') AND moderation_status='VISIBLE'),
 (SELECT count(*) FROM external_reputations WHERE verification_status='PENDING'),
 (SELECT count(*) FROM reports WHERE status IN('OPEN','IN_REVIEW')),
 (SELECT count(*) FROM fraud_signals WHERE status IN('OPEN','REVIEWING','CONFIRMED')),
 (SELECT count(*) FROM safe_deal_disputes WHERE status IN('OPEN','EVIDENCE_COLLECTION','UNDER_REVIEW')),
 (SELECT count(*) FROM safe_deals WHERE status NOT IN('COMPLETED','CANCELED','REFUNDED','FAILED')),
 (SELECT count(*) FROM services WHERE deleted_at IS NULL AND status='ACTIVE' AND moderation_status='VISIBLE'),
 (SELECT count(*) FROM jobs WHERE deleted_at IS NULL AND status='PUBLISHED' AND moderation_status='VISIBLE'),
 (SELECT count(*) FROM audit_logs WHERE created_at>=now()-interval '24 hours')`).Scan(
		&d.UsersTotal, &d.UsersNew7d, &d.ProjectsActive, &d.ProjectsOpen, &d.PendingReputation, &d.OpenReports, &d.OpenFraudSignals, &d.OpenDisputes, &d.ActiveSafeDeals, &d.ServicesActive, &d.VacanciesPublished, &d.RecentAdministrativeMutations)
	return d, err
}

func parseList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func (r PostgresRepository) ListUsers(ctx context.Context, f UserFilter, c *Cursor, limit int) (UserPage, error) {
	limit = bound(limit)
	var at any
	var id any
	if c != nil {
		at = c.At
		id = c.ID
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT u.id::text,u.email,COALESCE(u.username,''),u.display_name,u.status,u.email_verified_at IS NOT NULL,
 COALESCE((SELECT string_agg(ur.role,',' ORDER BY ur.role) FROM user_roles ur WHERE ur.user_id=u.id),''),
 COALESCE((SELECT string_agg(uc.capability,',' ORDER BY uc.capability) FROM user_capabilities uc WHERE uc.user_id=u.id),''),
 u.created_at,u.last_seen_at,(SELECT count(*) FROM sessions s WHERE s.user_id=u.id AND s.revoked_at IS NULL AND s.expires_at>now())
FROM users u
WHERE u.deleted_at IS NULL
 AND ($1='' OR u.email ILIKE '%'||$1||'%' OR u.display_name ILIKE '%'||$1||'%' OR COALESCE(u.username,'') ILIKE '%'||$1||'%' OR u.id::text=$1)
 AND ($2='' OR u.status=$2)
 AND ($3='' OR EXISTS(SELECT 1 FROM user_roles ur WHERE ur.user_id=u.id AND ur.role=$3))
 AND ($4='' OR EXISTS(SELECT 1 FROM user_capabilities uc WHERE uc.user_id=u.id AND uc.capability=$4))
 AND ($5::timestamptz IS NULL OR (u.created_at,u.id)<($5,$6::uuid))
ORDER BY u.created_at DESC,u.id DESC LIMIT $7`, strings.TrimSpace(f.Q), strings.ToUpper(strings.TrimSpace(f.Status)), strings.ToUpper(strings.TrimSpace(f.Role)), strings.ToUpper(strings.TrimSpace(f.Capability)), at, id, limit+1)
	if err != nil {
		return UserPage{}, err
	}
	defer rows.Close()
	items := make([]User, 0, limit+1)
	for rows.Next() {
		var v User
		var roles, caps string
		if err := rows.Scan(&v.ID, &v.Email, &v.Username, &v.DisplayName, &v.Status, &v.EmailVerified, &roles, &caps, &v.CreatedAt, &v.LastSeenAt, &v.ActiveSessions); err != nil {
			return UserPage{}, err
		}
		v.Roles = parseList(roles)
		v.Capabilities = parseList(caps)
		items = append(items, v)
	}
	if err := rows.Err(); err != nil {
		return UserPage{}, err
	}
	page := PageInfo{}
	if len(items) > limit {
		last := items[limit-1]
		page.HasMore = true
		page.NextCursor = &Cursor{At: last.CreatedAt, ID: last.ID}
		items = items[:limit]
	}
	return UserPage{Items: items, Page: page}, nil
}

func (r PostgresRepository) GetUser(ctx context.Context, id string) (User, error) {
	var v User
	var roles, caps string
	err := r.DB.QueryRowContext(ctx, `SELECT u.id::text,u.email,COALESCE(u.username,''),u.display_name,u.status,u.email_verified_at IS NOT NULL,
 COALESCE((SELECT string_agg(ur.role,',' ORDER BY ur.role) FROM user_roles ur WHERE ur.user_id=u.id),''),
 COALESCE((SELECT string_agg(uc.capability,',' ORDER BY uc.capability) FROM user_capabilities uc WHERE uc.user_id=u.id),''),
 u.created_at,u.last_seen_at,(SELECT count(*) FROM sessions s WHERE s.user_id=u.id AND s.revoked_at IS NULL AND s.expires_at>now())
FROM users u WHERE u.id=$1 AND u.deleted_at IS NULL`, id).Scan(&v.ID, &v.Email, &v.Username, &v.DisplayName, &v.Status, &v.EmailVerified, &roles, &caps, &v.CreatedAt, &v.LastSeenAt, &v.ActiveSessions)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	v.Roles = parseList(roles)
	v.Capabilities = parseList(caps)
	return v, nil
}

func hasRole(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id, role string) (bool, error) {
	var ok bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_roles WHERE user_id=$1 AND role=$2)`, id, role).Scan(&ok)
	return ok, err
}

func auditTx(ctx context.Context, tx *sql.Tx, actor, action, targetType, targetID, reason, requestID string, extra map[string]any) error {
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
	var target any
	if targetID != "" {
		target = targetID
	}
	ip := requestmeta.FromContext(ctx)
	var ipValue any
	if ip != "" {
		ipValue = ip
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_logs(id,actor_user_id,action,target_type,target_id,metadata,ip)VALUES(gen_random_uuid(),$1,$2,$3,$4::uuid,$5::jsonb,$6::inet)`, actor, action, targetType, target, payload, ipValue)
	return err
}

func (r PostgresRepository) SetUserStatus(ctx context.Context, actor, target, status, reason, requestID string) (User, error) {
	if !oneOf(status, "ACTIVE", "SUSPENDED", "BANNED") {
		return User{}, ErrInvalidInput
	}
	if len(reason) < 3 {
		return User{}, ErrInvalidInput
	}
	if actor == target && status != "ACTIVE" {
		return User{}, ErrConflict
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	protected, err := hasAnyProtectedRole(ctx, tx, target)
	if err != nil {
		return User{}, err
	}
	actorSuper, err := hasRole(ctx, tx, actor, "SUPER_ADMIN")
	if err != nil {
		return User{}, err
	}
	if protected && !actorSuper {
		return User{}, ErrForbidden
	}
	res, err := tx.ExecContext(ctx, `UPDATE users SET status=$2,updated_at=now() WHERE id=$1 AND deleted_at IS NULL`, target, status)
	if err != nil {
		return User{}, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return User{}, ErrNotFound
	}
	if status != "ACTIVE" {
		if _, err = tx.ExecContext(ctx, `UPDATE sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1 AND revoked_at IS NULL`, target); err != nil {
			return User{}, err
		}
	}
	if err = auditTx(ctx, tx, actor, "user.status_changed", "USER", target, reason, requestID, map[string]any{"status": status}); err != nil {
		return User{}, err
	}
	if err = tx.Commit(); err != nil {
		return User{}, err
	}
	return r.GetUser(ctx, target)
}

func hasAnyProtectedRole(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string) (bool, error) {
	var ok bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_roles WHERE user_id=$1 AND role IN('ADMIN','SUPER_ADMIN'))`, id).Scan(&ok)
	return ok, err
}

func (r PostgresRepository) RevokeSessions(ctx context.Context, actor, target, reason, requestID string) error {
	if len(reason) < 3 {
		return ErrInvalidInput
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND deleted_at IS NULL)`, target).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	if _, err = tx.ExecContext(ctx, `UPDATE sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1 AND revoked_at IS NULL`, target); err != nil {
		return err
	}
	if err = auditTx(ctx, tx, actor, "user.sessions_revoked", "USER", target, reason, requestID, nil); err != nil {
		return err
	}
	return tx.Commit()
}

func (r PostgresRepository) SetRole(ctx context.Context, actor, target, role string, enabled bool, reason, requestID string) (User, error) {
	if !oneOf(role, "MODERATOR", "ADMIN", "SUPER_ADMIN") || len(reason) < 3 {
		return User{}, ErrInvalidInput
	}
	if actor == target && !enabled {
		return User{}, ErrConflict
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND status<>'DELETED' AND deleted_at IS NULL)`, target).Scan(&exists); err != nil {
		return User{}, err
	}
	if !exists {
		return User{}, ErrNotFound
	}
	if enabled {
		_, err = tx.ExecContext(ctx, `INSERT INTO user_roles(user_id,role,granted_by)VALUES($1,$2,$3)ON CONFLICT(user_id,role)DO NOTHING`, target, role, actor)
		if err == nil {
			// Privileged staff accounts are operational identities, never marketplace
			// customer/freelancer identities. Converting an account to staff removes
			// marketplace capabilities and revokes existing sessions atomically.
			_, err = tx.ExecContext(ctx, `DELETE FROM user_capabilities WHERE user_id=$1`, target)
		}
		if err == nil {
			_, err = tx.ExecContext(ctx, `UPDATE sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1 AND revoked_at IS NULL`, target)
		}
	} else {
		_, err = tx.ExecContext(ctx, `DELETE FROM user_roles WHERE user_id=$1 AND role=$2`, target, role)
	}
	if err != nil {
		return User{}, err
	}
	action := "user.role_removed"
	if enabled {
		action = "user.role_granted"
	}
	if err = auditTx(ctx, tx, actor, action, "USER", target, reason, requestID, map[string]any{"role": role}); err != nil {
		return User{}, err
	}
	if err = tx.Commit(); err != nil {
		return User{}, err
	}
	return r.GetUser(ctx, target)
}

func (r PostgresRepository) ListFeatureFlags(ctx context.Context) ([]FeatureFlag, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT f.key,COALESCE(f.description,''),f.enabled,f.config::text,COALESCE(u.display_name,''),f.updated_at FROM feature_flags f LEFT JOIN users u ON u.id=f.updated_by ORDER BY f.key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FeatureFlag{}
	for rows.Next() {
		var v FeatureFlag
		var raw string
		if err := rows.Scan(&v.Key, &v.Description, &v.Enabled, &raw, &v.UpdatedBy, &v.UpdatedAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &v.Config); err != nil {
			v.Config = map[string]any{}
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r PostgresRepository) UpdateFeatureFlag(ctx context.Context, actor, key string, enabled bool, config map[string]any, reason, requestID string) (FeatureFlag, error) {
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 100 || len(reason) < 3 {
		return FeatureFlag{}, ErrInvalidInput
	}
	if config == nil {
		config = map[string]any{}
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return FeatureFlag{}, ErrInvalidInput
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return FeatureFlag{}, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE feature_flags SET enabled=$2,config=$3::jsonb,updated_by=$4,updated_at=now() WHERE key=$1`, key, enabled, raw, actor)
	if err != nil {
		return FeatureFlag{}, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return FeatureFlag{}, ErrNotFound
	}
	if err = auditTx(ctx, tx, actor, "feature_flag.updated", "FEATURE_FLAG", "", reason, requestID, map[string]any{"key": key, "enabled": enabled}); err != nil {
		return FeatureFlag{}, err
	}
	if err = tx.Commit(); err != nil {
		return FeatureFlag{}, err
	}
	items, err := r.ListFeatureFlags(ctx)
	if err != nil {
		return FeatureFlag{}, err
	}
	for _, v := range items {
		if v.Key == key {
			return v, nil
		}
	}
	return FeatureFlag{}, ErrNotFound
}

func (r PostgresRepository) ListReports(ctx context.Context, f ListFilter, c *Cursor, limit int) ([]Report, PageInfo, error) {
	limit = bound(limit)
	var at, id any
	if c != nil {
		at = c.At
		id = c.ID
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT rep.id::text,COALESCE(rep.reporter_user_id::text,''),COALESCE(u.display_name,''),rep.entity_type,rep.entity_id::text,rep.reason_code,COALESCE(rep.description,''),rep.status,COALESCE(rep.assigned_to_user_id::text,''),COALESCE(rep.resolution,''),rep.created_at,rep.reviewed_at,rep.resolved_at FROM reports rep LEFT JOIN users u ON u.id=rep.reporter_user_id WHERE ($1='' OR rep.status=$1) AND ($2='' OR rep.entity_type=$2) AND ($3='' OR rep.reason_code ILIKE '%'||$3||'%' OR COALESCE(rep.description,'') ILIKE '%'||$3||'%' OR rep.entity_id::text=$3) AND ($4::timestamptz IS NULL OR (rep.created_at,rep.id)<($4,$5::uuid)) ORDER BY rep.created_at DESC,rep.id DESC LIMIT $6`, strings.ToUpper(f.Status), strings.ToUpper(f.Kind), strings.TrimSpace(f.Q), at, id, limit+1)
	if err != nil {
		return nil, PageInfo{}, err
	}
	defer rows.Close()
	items := []Report{}
	for rows.Next() {
		var v Report
		if err := rows.Scan(&v.ID, &v.ReporterUserID, &v.ReporterName, &v.EntityType, &v.EntityID, &v.ReasonCode, &v.Description, &v.Status, &v.AssignedTo, &v.Resolution, &v.CreatedAt, &v.ReviewedAt, &v.ResolvedAt); err != nil {
			return nil, PageInfo{}, err
		}
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		return nil, PageInfo{}, err
	}
	return pageReports(items, limit)
}

func pageReports(items []Report, limit int) ([]Report, PageInfo, error) {
	page := PageInfo{}
	if len(items) > limit {
		last := items[limit-1]
		page.HasMore = true
		page.NextCursor = &Cursor{At: last.CreatedAt, ID: last.ID}
		items = items[:limit]
	}
	return items, page, nil
}
func (r PostgresRepository) getReport(ctx context.Context, id string) (Report, error) {
	var v Report
	err := r.DB.QueryRowContext(ctx, `SELECT rep.id::text,COALESCE(rep.reporter_user_id::text,''),COALESCE(u.display_name,''),rep.entity_type,rep.entity_id::text,rep.reason_code,COALESCE(rep.description,''),rep.status,COALESCE(rep.assigned_to_user_id::text,''),COALESCE(rep.resolution,''),rep.created_at,rep.reviewed_at,rep.resolved_at FROM reports rep LEFT JOIN users u ON u.id=rep.reporter_user_id WHERE rep.id=$1`, id).Scan(&v.ID, &v.ReporterUserID, &v.ReporterName, &v.EntityType, &v.EntityID, &v.ReasonCode, &v.Description, &v.Status, &v.AssignedTo, &v.Resolution, &v.CreatedAt, &v.ReviewedAt, &v.ResolvedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Report{}, ErrNotFound
	}
	return v, err
}
func (r PostgresRepository) UpdateReport(ctx context.Context, actor, id, status, resolution, requestID string) (Report, error) {
	if !oneOf(status, "OPEN", "IN_REVIEW", "RESOLVED", "DISMISSED") {
		return Report{}, ErrInvalidInput
	}
	if (status == "RESOLVED" || status == "DISMISSED") && len(resolution) < 3 {
		return Report{}, ErrInvalidInput
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Report{}, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE reports SET status=$2,assigned_to_user_id=$3,reviewed_by_user_id=$3,reviewed_at=now(),resolution=NULLIF($4,''),resolved_at=CASE WHEN $2 IN('RESOLVED','DISMISSED') THEN now() ELSE NULL END WHERE id=$1`, id, status, actor, resolution)
	if err != nil {
		return Report{}, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return Report{}, ErrNotFound
	}
	if err = auditTx(ctx, tx, actor, "report.status_changed", "REPORT", id, resolution, requestID, map[string]any{"status": status}); err != nil {
		return Report{}, err
	}
	if err = tx.Commit(); err != nil {
		return Report{}, err
	}
	return r.getReport(ctx, id)
}

func (r PostgresRepository) ListFraudSignals(ctx context.Context, f ListFilter, c *Cursor, limit int) ([]FraudSignal, PageInfo, error) {
	limit = bound(limit)
	var at, id any
	if c != nil {
		at = c.At
		id = c.ID
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT fs.id::text,COALESCE(fs.user_id::text,''),COALESCE(u.display_name,''),COALESCE(fs.entity_type,''),COALESCE(fs.entity_id::text,''),fs.signal_type,fs.severity,fs.evidence::text,fs.status,COALESCE(fs.resolution,''),fs.created_at,fs.reviewed_at FROM fraud_signals fs LEFT JOIN users u ON u.id=fs.user_id WHERE ($1='' OR fs.status=$1) AND ($2='' OR fs.signal_type ILIKE '%'||$2||'%' OR COALESCE(u.email,'') ILIKE '%'||$2||'%' OR COALESCE(u.display_name,'') ILIKE '%'||$2||'%' OR COALESCE(fs.entity_id::text,'')=$2) AND ($3::timestamptz IS NULL OR (fs.created_at,fs.id)<($3,$4::uuid)) ORDER BY fs.created_at DESC,fs.id DESC LIMIT $5`, strings.ToUpper(f.Status), strings.TrimSpace(f.Q), at, id, limit+1)
	if err != nil {
		return nil, PageInfo{}, err
	}
	defer rows.Close()
	items := []FraudSignal{}
	for rows.Next() {
		var v FraudSignal
		var raw string
		if err := rows.Scan(&v.ID, &v.UserID, &v.UserName, &v.EntityType, &v.EntityID, &v.SignalType, &v.Severity, &raw, &v.Status, &v.Resolution, &v.CreatedAt, &v.ReviewedAt); err != nil {
			return nil, PageInfo{}, err
		}
		_ = json.Unmarshal([]byte(raw), &v.Evidence)
		if v.Evidence == nil {
			v.Evidence = map[string]any{}
		}
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		return nil, PageInfo{}, err
	}
	page := PageInfo{}
	if len(items) > limit {
		last := items[limit-1]
		page.HasMore = true
		page.NextCursor = &Cursor{At: last.CreatedAt, ID: last.ID}
		items = items[:limit]
	}
	return items, page, nil
}
func (r PostgresRepository) getFraud(ctx context.Context, id string) (FraudSignal, error) {
	var v FraudSignal
	var raw string
	err := r.DB.QueryRowContext(ctx, `SELECT fs.id::text,COALESCE(fs.user_id::text,''),COALESCE(u.display_name,''),COALESCE(fs.entity_type,''),COALESCE(fs.entity_id::text,''),fs.signal_type,fs.severity,fs.evidence::text,fs.status,COALESCE(fs.resolution,''),fs.created_at,fs.reviewed_at FROM fraud_signals fs LEFT JOIN users u ON u.id=fs.user_id WHERE fs.id=$1`, id).Scan(&v.ID, &v.UserID, &v.UserName, &v.EntityType, &v.EntityID, &v.SignalType, &v.Severity, &raw, &v.Status, &v.Resolution, &v.CreatedAt, &v.ReviewedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return FraudSignal{}, ErrNotFound
	}
	if err != nil {
		return FraudSignal{}, err
	}
	_ = json.Unmarshal([]byte(raw), &v.Evidence)
	if v.Evidence == nil {
		v.Evidence = map[string]any{}
	}
	return v, nil
}
func (r PostgresRepository) UpdateFraudSignal(ctx context.Context, actor, id, status, resolution, requestID string) (FraudSignal, error) {
	if !oneOf(status, "OPEN", "REVIEWING", "CONFIRMED", "DISMISSED", "RESOLVED") {
		return FraudSignal{}, ErrInvalidInput
	}
	if (status == "DISMISSED" || status == "RESOLVED") && len(resolution) < 3 {
		return FraudSignal{}, ErrInvalidInput
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return FraudSignal{}, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE fraud_signals SET status=$2,reviewed_by_user_id=$3,reviewed_at=CASE WHEN $2='OPEN' THEN reviewed_at ELSE now() END,resolution=NULLIF($4,'') WHERE id=$1`, id, status, actor, resolution)
	if err != nil {
		return FraudSignal{}, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return FraudSignal{}, ErrNotFound
	}
	if err = auditTx(ctx, tx, actor, "fraud_signal.status_changed", "FRAUD_SIGNAL", id, resolution, requestID, map[string]any{"status": status}); err != nil {
		return FraudSignal{}, err
	}
	if err = tx.Commit(); err != nil {
		return FraudSignal{}, err
	}
	return r.getFraud(ctx, id)
}

func (r PostgresRepository) ListAudit(ctx context.Context, f ListFilter, c *Cursor, limit int) ([]AuditEntry, PageInfo, error) {
	limit = bound(limit)
	var at, id any
	if c != nil {
		at = c.At
		id = c.ID
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT a.id::text,COALESCE(a.actor_user_id::text,''),COALESCE(u.display_name,''),a.action,COALESCE(a.target_type,''),COALESCE(a.target_id::text,''),a.metadata::text,COALESCE(a.ip::text,''),a.created_at FROM audit_logs a LEFT JOIN users u ON u.id=a.actor_user_id WHERE ($1='' OR a.action ILIKE '%'||$1||'%' OR COALESCE(a.target_type,'') ILIKE '%'||$1||'%' OR COALESCE(a.target_id::text,'')=$1 OR COALESCE(u.email,'') ILIKE '%'||$1||'%') AND ($2='' OR a.target_type=$2) AND ($3::timestamptz IS NULL OR (a.created_at,a.id)<($3,$4::uuid)) ORDER BY a.created_at DESC,a.id DESC LIMIT $5`, strings.TrimSpace(f.Q), strings.ToUpper(strings.TrimSpace(f.Kind)), at, id, limit+1)
	if err != nil {
		return nil, PageInfo{}, err
	}
	defer rows.Close()
	items := []AuditEntry{}
	for rows.Next() {
		var v AuditEntry
		var raw string
		if err := rows.Scan(&v.ID, &v.ActorUserID, &v.ActorName, &v.Action, &v.TargetType, &v.TargetID, &raw, &v.IPAddress, &v.CreatedAt); err != nil {
			return nil, PageInfo{}, err
		}
		_ = json.Unmarshal([]byte(raw), &v.Metadata)
		if v.Metadata == nil {
			v.Metadata = map[string]any{}
		}
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		return nil, PageInfo{}, err
	}
	page := PageInfo{}
	if len(items) > limit {
		last := items[limit-1]
		page.HasMore = true
		page.NextCursor = &Cursor{At: last.CreatedAt, ID: last.ID}
		items = items[:limit]
	}
	return items, page, nil
}

func (r PostgresRepository) ListContent(ctx context.Context, kind string, f ListFilter, c *Cursor, limit int) ([]ContentItem, PageInfo, error) {
	kind = strings.ToUpper(strings.TrimSpace(kind))
	limit = bound(limit)
	var at, id any
	if c != nil {
		at = c.At
		id = c.ID
	}
	var query string
	switch kind {
	case "PROJECT":
		query = `SELECT p.id::text,'PROJECT',p.title,p.customer_user_id::text,u.display_name,p.status,p.moderation_status,COALESCE(p.moderation_reason,''),COALESCE(c.name,''),p.created_at,p.updated_at FROM projects p JOIN users u ON u.id=p.customer_user_id LEFT JOIN categories c ON c.id=p.category_id WHERE p.deleted_at IS NULL AND ($1='' OR p.status=$1) AND ($2='' OR p.title ILIKE '%'||$2||'%' OR u.email ILIKE '%'||$2||'%' OR u.display_name ILIKE '%'||$2||'%' OR p.id::text=$2) AND ($3::timestamptz IS NULL OR (p.created_at,p.id)<($3,$4::uuid)) ORDER BY p.created_at DESC,p.id DESC LIMIT $5`
	case "SERVICE":
		query = `SELECT s.id::text,'SERVICE',s.title,s.seller_user_id::text,u.display_name,s.status,s.moderation_status,COALESCE(s.moderation_reason,''),COALESCE(c.name,''),s.created_at,s.updated_at FROM services s JOIN users u ON u.id=s.seller_user_id LEFT JOIN categories c ON c.id=s.category_id WHERE s.deleted_at IS NULL AND ($1='' OR s.status=$1) AND ($2='' OR s.title ILIKE '%'||$2||'%' OR u.email ILIKE '%'||$2||'%' OR u.display_name ILIKE '%'||$2||'%' OR s.id::text=$2) AND ($3::timestamptz IS NULL OR (s.created_at,s.id)<($3,$4::uuid)) ORDER BY s.created_at DESC,s.id DESC LIMIT $5`
	case "VACANCY":
		query = `SELECT j.id::text,'VACANCY',j.title,j.customer_user_id::text,u.display_name,j.status,j.moderation_status,COALESCE(j.moderation_reason,''),COALESCE(c.name,''),j.created_at,j.updated_at FROM jobs j JOIN users u ON u.id=j.customer_user_id LEFT JOIN categories c ON c.id=j.category_id WHERE j.deleted_at IS NULL AND ($1='' OR j.status=$1) AND ($2='' OR j.title ILIKE '%'||$2||'%' OR u.email ILIKE '%'||$2||'%' OR u.display_name ILIKE '%'||$2||'%' OR j.id::text=$2) AND ($3::timestamptz IS NULL OR (j.created_at,j.id)<($3,$4::uuid)) ORDER BY j.created_at DESC,j.id DESC LIMIT $5`
	default:
		return nil, PageInfo{}, ErrInvalidInput
	}
	rows, err := r.DB.QueryContext(ctx, query, strings.ToUpper(f.Status), strings.TrimSpace(f.Q), at, id, limit+1)
	if err != nil {
		return nil, PageInfo{}, err
	}
	defer rows.Close()
	items := []ContentItem{}
	for rows.Next() {
		var v ContentItem
		if err := rows.Scan(&v.ID, &v.Kind, &v.Title, &v.OwnerUserID, &v.OwnerDisplayName, &v.Status, &v.ModerationStatus, &v.ModerationReason, &v.CategoryName, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, PageInfo{}, err
		}
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		return nil, PageInfo{}, err
	}
	page := PageInfo{}
	if len(items) > limit {
		last := items[limit-1]
		page.HasMore = true
		page.NextCursor = &Cursor{At: last.CreatedAt, ID: last.ID}
		items = items[:limit]
	}
	return items, page, nil
}

func (r PostgresRepository) GetContent(ctx context.Context, kind, id string) (ContentItem, error) {
	return r.getContent(ctx, kind, id)
}

func (r PostgresRepository) getContent(ctx context.Context, kind, id string) (ContentItem, error) {
	var v ContentItem
	var query string
	switch kind {
	case "PROJECT":
		query = `SELECT p.id::text,'PROJECT',p.title,p.customer_user_id::text,u.display_name,p.status,p.moderation_status,COALESCE(p.moderation_reason,''),COALESCE(c.name,''),p.created_at,p.updated_at FROM projects p JOIN users u ON u.id=p.customer_user_id LEFT JOIN categories c ON c.id=p.category_id WHERE p.id=$1 AND p.deleted_at IS NULL`
	case "SERVICE":
		query = `SELECT s.id::text,'SERVICE',s.title,s.seller_user_id::text,u.display_name,s.status,s.moderation_status,COALESCE(s.moderation_reason,''),COALESCE(c.name,''),s.created_at,s.updated_at FROM services s JOIN users u ON u.id=s.seller_user_id LEFT JOIN categories c ON c.id=s.category_id WHERE s.id=$1 AND s.deleted_at IS NULL`
	case "VACANCY":
		query = `SELECT j.id::text,'VACANCY',j.title,j.customer_user_id::text,u.display_name,j.status,j.moderation_status,COALESCE(j.moderation_reason,''),COALESCE(c.name,''),j.created_at,j.updated_at FROM jobs j JOIN users u ON u.id=j.customer_user_id LEFT JOIN categories c ON c.id=j.category_id WHERE j.id=$1 AND j.deleted_at IS NULL`
	default:
		return ContentItem{}, ErrInvalidInput
	}
	err := r.DB.QueryRowContext(ctx, query, id).Scan(&v.ID, &v.Kind, &v.Title, &v.OwnerUserID, &v.OwnerDisplayName, &v.Status, &v.ModerationStatus, &v.ModerationReason, &v.CategoryName, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ContentItem{}, ErrNotFound
	}
	return v, err
}

func (r PostgresRepository) ModerateContent(ctx context.Context, actor, kind, id, action, reason, requestID string) (ContentItem, error) {
	if !oneOf(kind, "PROJECT", "SERVICE", "VACANCY") || !oneOf(action, "HIDE", "RESTORE", "REJECT", "DELETE") || len(strings.TrimSpace(reason)) < 3 {
		return ContentItem{}, ErrInvalidInput
	}
	before, err := r.getContent(ctx, kind, id)
	if err != nil {
		return ContentItem{}, err
	}
	if action == "REJECT" && before.ModerationStatus == "HIDDEN" {
		return ContentItem{}, ErrConflict
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return ContentItem{}, err
	}
	defer tx.Rollback()
	table := "projects"
	if kind == "SERVICE" {
		table = "services"
	}
	if kind == "VACANCY" {
		table = "jobs"
	}
	target := "VISIBLE"
	if action == "HIDE" || action == "REJECT" {
		target = "HIDDEN"
	}
	var result sql.Result
	if action == "DELETE" {
		result, err = tx.ExecContext(ctx, `UPDATE `+table+` SET deleted_at=now(),moderation_status='HIDDEN',moderation_reason=$2,moderated_by=$3,moderated_at=now(),updated_at=now() WHERE id=$1 AND deleted_at IS NULL`, id, reason, actor)
	} else if action == "REJECT" {
		// Rejected content must stop behaving as published content for its owner as well.
		// Put editable entities back into their pre-publication state so the owner can fix and republish.
		switch kind {
		case "PROJECT":
			result, err = tx.ExecContext(ctx, `UPDATE projects SET status='DRAFT',published_at=NULL,moderation_status='HIDDEN',moderation_reason=$2,moderated_by=$3,moderated_at=now(),updated_at=now() WHERE id=$1 AND deleted_at IS NULL`, id, reason, actor)
		case "SERVICE":
			result, err = tx.ExecContext(ctx, `UPDATE services SET status='REJECTED',moderation_status='HIDDEN',moderation_reason=$2,moderated_by=$3,moderated_at=now(),updated_at=now() WHERE id=$1 AND deleted_at IS NULL`, id, reason, actor)
		case "VACANCY":
			result, err = tx.ExecContext(ctx, `UPDATE jobs SET status='DRAFT',published_at=NULL,moderation_status='HIDDEN',moderation_reason=$2,moderated_by=$3,moderated_at=now(),updated_at=now() WHERE id=$1 AND deleted_at IS NULL`, id, reason, actor)
		}
	} else {
		result, err = tx.ExecContext(ctx, `UPDATE `+table+` SET moderation_status=$2,moderation_reason=$3,moderated_by=$4,moderated_at=now(),updated_at=now() WHERE id=$1 AND deleted_at IS NULL`, id, target, reason, actor)
	}
	if err != nil {
		return ContentItem{}, err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return ContentItem{}, ErrNotFound
	}
	if err = auditTx(ctx, tx, actor, strings.ToLower(kind)+".moderated", kind, id, reason, requestID, map[string]any{"action": action, "moderation_status": target}); err != nil {
		return ContentItem{}, err
	}
	if action == "REJECT" || action == "DELETE" {
		if err = supportnotify.ModerationNotice(ctx, tx, before.OwnerUserID, kind, id, before.Title, map[string]string{"REJECT": "Контент отклонён", "DELETE": "Контент удалён"}[action], reason); err != nil {
			return ContentItem{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return ContentItem{}, err
	}
	if action == "DELETE" {
		before.ModerationStatus = "DELETED"
		before.ModerationReason = reason
		return before, nil
	}
	return r.getContent(ctx, kind, id)
}

func (r PostgresRepository) ListReviews(ctx context.Context, f ListFilter, c *Cursor, limit int) ([]ReviewItem, PageInfo, error) {
	limit = bound(limit)
	var at, id any
	if c != nil {
		at = c.At
		id = c.ID
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT rv.id::text,rv.project_id::text,rv.reviewer_user_id::text,ru.display_name,rv.reviewee_user_id::text,ee.display_name,rv.rating_overall,COALESCE(rv.text,''),rv.status,rv.created_at FROM reviews rv JOIN users ru ON ru.id=rv.reviewer_user_id JOIN users ee ON ee.id=rv.reviewee_user_id WHERE ($1='' OR rv.status=$1) AND ($2='' OR rv.text ILIKE '%'||$2||'%' OR ru.display_name ILIKE '%'||$2||'%' OR ee.display_name ILIKE '%'||$2||'%' OR rv.id::text=$2) AND ($3::timestamptz IS NULL OR (rv.created_at,rv.id)<($3,$4::uuid)) ORDER BY rv.created_at DESC,rv.id DESC LIMIT $5`, strings.ToUpper(f.Status), strings.TrimSpace(f.Q), at, id, limit+1)
	if err != nil {
		return nil, PageInfo{}, err
	}
	defer rows.Close()
	items := []ReviewItem{}
	for rows.Next() {
		var v ReviewItem
		if err := rows.Scan(&v.ID, &v.ProjectID, &v.ReviewerID, &v.Reviewer, &v.RevieweeID, &v.Reviewee, &v.Rating, &v.Text, &v.Status, &v.CreatedAt); err != nil {
			return nil, PageInfo{}, err
		}
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		return nil, PageInfo{}, err
	}
	page := PageInfo{}
	if len(items) > limit {
		last := items[limit-1]
		page.HasMore = true
		page.NextCursor = &Cursor{At: last.CreatedAt, ID: last.ID}
		items = items[:limit]
	}
	return items, page, nil
}

func (r PostgresRepository) ListDisputes(ctx context.Context, f ListFilter, c *Cursor, limit int) ([]DisputeItem, PageInfo, error) {
	limit = bound(limit)
	var at, id any
	if c != nil {
		at = c.At
		id = c.ID
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT d.id::text,d.deal_id::text,sd.project_id::text,p.title,sd.customer_user_id::text,cu.display_name,sd.freelancer_user_id::text,fu.display_name,sd.gross_amount_kopecks,sd.status,d.reason_code,d.description,d.status,d.opened_at FROM safe_deal_disputes d JOIN safe_deals sd ON sd.id=d.deal_id JOIN projects p ON p.id=sd.project_id JOIN users cu ON cu.id=sd.customer_user_id JOIN users fu ON fu.id=sd.freelancer_user_id WHERE ($1='' OR d.status=$1) AND ($2='' OR p.title ILIKE '%'||$2||'%' OR cu.display_name ILIKE '%'||$2||'%' OR fu.display_name ILIKE '%'||$2||'%' OR d.id::text=$2 OR sd.id::text=$2) AND ($3::timestamptz IS NULL OR (d.opened_at,d.id)<($3,$4::uuid)) ORDER BY d.opened_at DESC,d.id DESC LIMIT $5`, strings.ToUpper(f.Status), strings.TrimSpace(f.Q), at, id, limit+1)
	if err != nil {
		return nil, PageInfo{}, err
	}
	defer rows.Close()
	items := []DisputeItem{}
	for rows.Next() {
		var v DisputeItem
		if err := rows.Scan(&v.ID, &v.DealID, &v.ProjectID, &v.ProjectTitle, &v.CustomerID, &v.CustomerName, &v.FreelancerID, &v.FreelancerName, &v.AmountKopecks, &v.DealStatus, &v.ReasonCode, &v.Description, &v.Status, &v.OpenedAt); err != nil {
			return nil, PageInfo{}, err
		}
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		return nil, PageInfo{}, err
	}
	page := PageInfo{}
	if len(items) > limit {
		last := items[limit-1]
		page.HasMore = true
		page.NextCursor = &Cursor{At: last.OpenedAt, ID: last.ID}
		items = items[:limit]
	}
	return items, page, nil
}

func bound(limit int) int {
	if limit < 1 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}
func oneOf(value string, allowed ...string) bool {
	for _, v := range allowed {
		if value == v {
			return true
		}
	}
	return false
}

var _ Repository = PostgresRepository{}
