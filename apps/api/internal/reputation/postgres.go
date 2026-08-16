package reputation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"freelance/apps/api/internal/platform/requestmeta"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type PostgresRepository struct{ DB *sql.DB }

const ownerColumns = `id::text,user_id::text,platform,profile_url,COALESCE(external_username,''),rating,reviews_count,
completed_orders_count,account_since::text,verification_status,COALESCE(verification_method,''),verified_at,expires_at,
last_checked_at,created_at,updated_at`

type scanner interface{ Scan(...any) error }

func scanItem(row scanner) (Item, error) {
	var item Item
	err := row.Scan(&item.ID, &item.UserID, &item.Platform, &item.ProfileURL, &item.ExternalUsername, &item.Rating,
		&item.ReviewsCount, &item.CompletedOrdersCount, &item.AccountSince, &item.VerificationStatus,
		&item.VerificationMethod, &item.VerifiedAt, &item.ExpiresAt, &item.LastCheckedAt, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (r PostgresRepository) StartVerification(ctx context.Context, actor, id string, input StartVerificationRequest, hash []byte, now time.Time) (Challenge, error) {
	evidence, _ := json.Marshal(input.Evidence)
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Challenge{}, err
	}
	defer tx.Rollback()
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT verification_status FROM external_reputations WHERE id=$1 AND user_id=$2 FOR UPDATE`, id, actor).Scan(&status); errors.Is(err, sql.ErrNoRows) {
		return Challenge{}, ErrNotFound
	} else if err != nil {
		return Challenge{}, err
	}
	if status != "UNVERIFIED" && status != "REJECTED" && status != "EXPIRED" {
		var expired int64
		result, expireErr := tx.ExecContext(ctx, `UPDATE reputation_verification_challenges SET status='EXPIRED' WHERE external_reputation_id=$1 AND status='PENDING' AND expires_at<=$2`, id, now)
		if expireErr != nil {
			return Challenge{}, expireErr
		}
		expired, _ = result.RowsAffected()
		if status != "PENDING" || expired == 0 {
			return Challenge{}, ErrInvalidState
		}
		status = "EXPIRED"
		if _, expireErr = tx.ExecContext(ctx, `UPDATE external_reputations SET verification_status='EXPIRED',updated_at=$2 WHERE id=$1`, id, now); expireErr != nil {
			return Challenge{}, expireErr
		}
	}
	var exists bool
	if _, err = tx.ExecContext(ctx, `UPDATE reputation_verification_challenges SET status='EXPIRED' WHERE external_reputation_id=$1 AND status='PENDING' AND expires_at<=$2`, id, now); err != nil {
		return Challenge{}, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM reputation_verification_challenges WHERE external_reputation_id=$1 AND status='PENDING' AND expires_at>$2)`, id, now).Scan(&exists); err != nil {
		return Challenge{}, err
	}
	if exists {
		return Challenge{}, ErrConflict
	}
	expires := now.Add(24 * time.Hour)
	var c Challenge
	err = tx.QueryRowContext(ctx, `INSERT INTO reputation_verification_challenges(external_reputation_id,method,code_hash,expires_at)VALUES($1,$2,$3,$4)RETURNING id::text,external_reputation_id::text,method,expires_at,attempts,status,created_at,verified_at`, id, input.Method, hash, expires).Scan(&c.ID, &c.ExternalReputationID, &c.Method, &c.ExpiresAt, &c.Attempts, &c.Status, &c.CreatedAt, &c.VerifiedAt)
	if err != nil {
		return Challenge{}, mapPostgresError(err)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE external_reputations SET verification_status='PENDING',verification_method=$2,evidence=$3::jsonb,updated_at=$4 WHERE id=$1`, id, input.Method, string(evidence), now); err != nil {
		return Challenge{}, err
	}
	if err = tx.Commit(); err != nil {
		return Challenge{}, err
	}
	return c, nil
}

func (r PostgresRepository) GetVerification(ctx context.Context, actor, id string, now time.Time) (Challenge, error) {
	_, err := r.DB.ExecContext(ctx, `WITH expired AS(UPDATE reputation_verification_challenges c SET status='EXPIRED' FROM external_reputations er WHERE c.external_reputation_id=er.id AND er.id=$1 AND er.user_id=$2 AND c.status='PENDING' AND c.expires_at<=$3 RETURNING c.external_reputation_id)UPDATE external_reputations er SET verification_status='EXPIRED',updated_at=$3 FROM expired WHERE er.id=expired.external_reputation_id`, id, actor, now)
	if err != nil {
		return Challenge{}, err
	}
	var c Challenge
	err = r.DB.QueryRowContext(ctx, `SELECT c.id::text,c.external_reputation_id::text,c.method,c.expires_at,c.attempts,c.status,c.created_at,c.verified_at FROM reputation_verification_challenges c JOIN external_reputations er ON er.id=c.external_reputation_id WHERE c.external_reputation_id=$1 AND er.user_id=$2 ORDER BY c.created_at DESC LIMIT 1`, id, actor).Scan(&c.ID, &c.ExternalReputationID, &c.Method, &c.ExpiresAt, &c.Attempts, &c.Status, &c.CreatedAt, &c.VerifiedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Challenge{}, ErrNotFound
	}
	return c, err
}

func (r PostgresRepository) ListPending(ctx context.Context, actor string) ([]ModerationItem, error) {
	if err := r.requireModerator(ctx, actor); err != nil {
		return nil, err
	}
	if _, err := r.DB.ExecContext(ctx, `WITH expired AS(UPDATE reputation_verification_challenges SET status='EXPIRED' WHERE status='PENDING' AND expires_at<=now() RETURNING external_reputation_id) UPDATE external_reputations er SET verification_status='EXPIRED',updated_at=now() FROM expired WHERE er.id=expired.external_reputation_id AND er.verification_status='PENDING'`); err != nil {
		return nil, err
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT er.id::text,er.user_id::text,er.platform,er.profile_url,COALESCE(er.external_username,''),er.rating,er.reviews_count,er.completed_orders_count,er.account_since::text,er.verification_status,COALESCE(er.verification_method,''),er.verified_at,er.expires_at,er.last_checked_at,er.created_at,er.updated_at,er.evidence,c.id::text,c.method,c.expires_at,c.attempts,c.status,c.created_at,c.verified_at FROM external_reputations er LEFT JOIN reputation_verification_challenges c ON c.external_reputation_id=er.id AND c.status='PENDING' WHERE er.verification_status='PENDING' ORDER BY er.created_at,er.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ModerationItem{}
	for rows.Next() {
		var v ModerationItem
		var evidence []byte
		var cid, method, status sql.NullString
		var expires, created, verified sql.NullTime
		var attempts sql.NullInt64
		err = rows.Scan(&v.ID, &v.UserID, &v.Platform, &v.ProfileURL, &v.ExternalUsername, &v.Rating, &v.ReviewsCount, &v.CompletedOrdersCount, &v.AccountSince, &v.VerificationStatus, &v.VerificationMethod, &v.VerifiedAt, &v.ExpiresAt, &v.LastCheckedAt, &v.CreatedAt, &v.UpdatedAt, &evidence, &cid, &method, &expires, &attempts, &status, &created, &verified)
		if err != nil {
			return nil, err
		}
		_ = json.Unmarshal(evidence, &v.Evidence)
		if cid.Valid {
			c := Challenge{ID: cid.String, ExternalReputationID: v.ID, Method: method.String, ExpiresAt: expires.Time, Attempts: int(attempts.Int64), Status: status.String, CreatedAt: created.Time}
			if verified.Valid {
				c.VerifiedAt = &verified.Time
			}
			v.Challenge = &c
		}
		items = append(items, v)
	}
	return items, rows.Err()
}

func (r PostgresRepository) Decide(ctx context.Context, actor, id string, input DecisionRequest, now time.Time) (Item, error) {
	if err := r.requireModerator(ctx, actor); err != nil {
		return Item{}, err
	}
	parts := strings.SplitN(input.ReasonCode, ":", 2)
	action := parts[0]
	reason := ""
	if len(parts) > 1 {
		reason = parts[1]
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Item{}, err
	}
	defer tx.Rollback()
	status := "REJECTED"
	if action == "verify" {
		status = "VERIFIED"
		var duplicate bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM external_reputations target JOIN external_reputations duplicate ON duplicate.platform=target.platform AND duplicate.profile_url=target.profile_url AND duplicate.verification_status='VERIFIED' AND duplicate.id<>target.id WHERE target.id=$1)`, id).Scan(&duplicate); err != nil {
			return Item{}, err
		}
		if duplicate {
			_ = tx.Rollback()
			_, _ = r.DB.ExecContext(ctx, `INSERT INTO fraud_signals(entity_type,entity_id,signal_type,severity,evidence)VALUES('EXTERNAL_REPUTATION',$1,'DUPLICATE_EXTERNAL_IDENTITY',3,'{}')`, id)
			return Item{}, ErrConflict
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE external_reputations SET verification_status=$2::text,verified_at=CASE WHEN $2::text='VERIFIED' THEN $3::timestamptz ELSE NULL END,updated_at=$3::timestamptz WHERE id=$1::uuid AND verification_status='PENDING'`, id, status, now)
	if err != nil {
		return Item{}, mapPostgresError(err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return Item{}, ErrNotFound
	}
	_, err = tx.ExecContext(ctx, `UPDATE reputation_verification_challenges SET status=$2::text,verified_at=CASE WHEN $2::text='VERIFIED' THEN $3::timestamptz ELSE NULL END WHERE external_reputation_id=$1::uuid AND status='PENDING'`, id, status, now)
	if err != nil {
		return Item{}, err
	}
	metadata, _ := json.Marshal(map[string]string{"reason_code": reason, "note": input.Note})
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_logs(id,actor_user_id,action,target_type,target_id,metadata,ip)VALUES(gen_random_uuid(),$1::uuid,$2::varchar,'EXTERNAL_REPUTATION',$3::uuid,$4::jsonb,NULLIF($5,'')::inet)`, actor, "EXTERNAL_REPUTATION_"+status, id, string(metadata), requestmeta.FromContext(ctx))
	if err != nil {
		return Item{}, err
	}
	if err = tx.Commit(); err != nil {
		return Item{}, err
	}
	return r.getByID(ctx, id)
}

func (r PostgresRepository) requireModerator(ctx context.Context, actor string) error {
	var ok bool
	err := r.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN user_roles ur ON ur.user_id=u.id WHERE u.id=$1 AND u.status='ACTIVE' AND u.deleted_at IS NULL AND ur.role IN('MODERATOR','ADMIN','SUPER_ADMIN'))`, actor).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return nil
}
func (r PostgresRepository) getByID(ctx context.Context, id string) (Item, error) {
	item, err := scanItem(r.DB.QueryRowContext(ctx, `SELECT `+ownerColumns+` FROM external_reputations WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	return item, err
}

func (r PostgresRepository) ListOwned(ctx context.Context, actor string) ([]Item, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT `+ownerColumns+` FROM external_reputations WHERE user_id=$1 ORDER BY created_at DESC,id DESC`, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Item, 0)
	for rows.Next() {
		item, scanErr := scanItem(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r PostgresRepository) Create(ctx context.Context, actor string, input CreateRequest) (Item, error) {
	var id string
	err := r.DB.QueryRowContext(ctx, `INSERT INTO external_reputations(user_id,platform,profile_url,external_username)VALUES($1,$2,$3,NULLIF($4,''))RETURNING id::text`, actor, input.Platform, input.ProfileURL, input.ExternalUsername).Scan(&id)
	if err != nil {
		return Item{}, mapPostgresError(err)
	}
	return r.getOwned(ctx, actor, id)
}

func (r PostgresRepository) Update(ctx context.Context, actor, id string, patch PatchRequest) (Item, error) {
	result, err := r.DB.ExecContext(ctx, `UPDATE external_reputations SET platform=$3,profile_url=$4,external_username=NULLIF($5,''),updated_at=now() WHERE id=$1 AND user_id=$2 AND verification_status='UNVERIFIED'`, id, actor, *patch.Platform, *patch.ProfileURL, *patch.ExternalUsername)
	if err != nil {
		return Item{}, mapPostgresError(err)
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return Item{}, ErrNotFound
	}
	return r.getOwned(ctx, actor, id)
}

func (r PostgresRepository) Delete(ctx context.Context, actor, id string) error {
	result, err := r.DB.ExecContext(ctx, `DELETE FROM external_reputations WHERE id=$1 AND user_id=$2`, id, actor)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrNotFound
	}
	return nil
}

func (r PostgresRepository) ListPublic(ctx context.Context, username string) ([]PublicItem, error) {
	var exists bool
	err := r.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN professional_profiles p ON p.user_id=u.id WHERE u.username_normalized=lower($1) AND u.status='ACTIVE' AND u.deleted_at IS NULL AND p.profile_visibility='PUBLIC')`, username).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT er.platform,er.profile_url,er.rating,er.reviews_count,er.completed_orders_count,er.account_since::text,er.verified_at FROM external_reputations er JOIN users u ON u.id=er.user_id JOIN professional_profiles p ON p.user_id=u.id WHERE u.username_normalized=lower($1) AND u.status='ACTIVE' AND u.deleted_at IS NULL AND p.profile_visibility='PUBLIC' AND er.verification_status='VERIFIED' ORDER BY er.verified_at DESC NULLS LAST,er.id`, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]PublicItem, 0)
	for rows.Next() {
		var item PublicItem
		if err := rows.Scan(&item.Platform, &item.ProfileURL, &item.Rating, &item.ReviewsCount, &item.CompletedOrdersCount, &item.AccountSince, &item.VerifiedAt); err != nil {
			return nil, err
		}
		item.Verified = true
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r PostgresRepository) getOwned(ctx context.Context, actor, id string) (Item, error) {
	item, err := scanItem(r.DB.QueryRowContext(ctx, `SELECT `+ownerColumns+` FROM external_reputations WHERE id=$1 AND user_id=$2`, id, actor))
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	return item, err
}

func mapPostgresError(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return ErrConflict
		case "22001", "23514":
			return ErrInvalid
		}
	}
	return err
}
