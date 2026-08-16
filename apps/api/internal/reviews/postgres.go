package reviews

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"freelance/apps/api/internal/platform/contentmoderation"
	"freelance/apps/api/internal/platform/requestmeta"
	"freelance/apps/api/internal/platform/supportnotify"
)

type PostgresRepository struct{ DB *sql.DB }

const columns = `r.id::text,r.project_id::text,r.reviewer_user_id::text,r.reviewee_user_id::text,r.reviewer_role,r.rating_overall,r.would_work_again,COALESCE(r.text,''),r.status,r.created_at,r.updated_at,ru.display_name,pr.title`
const fromReviews = ` FROM reviews r JOIN users ru ON ru.id=r.reviewer_user_id JOIN projects pr ON pr.id=r.project_id`

type scanner interface{ Scan(...any) error }

func scan(row scanner) (Item, error) {
	var v Item
	err := row.Scan(&v.ID, &v.ProjectID, &v.ReviewerID, &v.RevieweeID, &v.ReviewerRole, &v.RatingOverall, &v.WouldWorkAgain, &v.Text, &v.Status, &v.CreatedAt, &v.UpdatedAt, &v.ReviewerName, &v.ProjectTitle)
	v.Dimensions = map[string]int{}
	return v, err
}

func (r PostgresRepository) Create(ctx context.Context, actor, project string, in Input) (Item, error) {
	if text := strings.TrimSpace(in.Text); text != "" && contentmoderation.LooksLikeJunk(text) {
		return Item{}, ErrModeration
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Item{}, err
	}
	defer tx.Rollback()
	var customer, freelancer, status string
	err = tx.QueryRowContext(ctx, `SELECT p.customer_user_id::text,a.freelancer_user_id::text,p.status FROM projects p JOIN project_assignments a ON a.project_id=p.id JOIN safe_deals d ON d.assignment_id=a.id AND d.status='COMPLETED' WHERE p.id=$1 AND p.deleted_at IS NULL ORDER BY a.created_at DESC LIMIT 1 FOR UPDATE OF p`, project).Scan(&customer, &freelancer, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, err
	}
	if status != "COMPLETED" {
		return Item{}, ErrIneligible
	}
	role, reviewee := "", ""
	if actor == customer {
		role, reviewee = "CUSTOMER", freelancer
	} else if actor == freelancer {
		role, reviewee = "FREELANCER", customer
	} else {
		return Item{}, ErrNotFound
	}
	if actor == reviewee || validateInput(in, role) != nil {
		return Item{}, ErrInvalid
	}
	var id string
	err = tx.QueryRowContext(ctx, `INSERT INTO reviews(project_id,reviewer_user_id,reviewee_user_id,reviewer_role,rating_overall,would_work_again,text)VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''))RETURNING id::text`, project, actor, reviewee, role, in.RatingOverall, in.WouldWorkAgain, strings.TrimSpace(in.Text)).Scan(&id)
	if err != nil {
		return Item{}, mapError(err)
	}
	for dimension, score := range in.Dimensions {
		if _, err = tx.ExecContext(ctx, `INSERT INTO review_dimensions(review_id,dimension,score)VALUES($1,$2,$3)`, id, strings.ToUpper(dimension), score); err != nil {
			return Item{}, err
		}
	}
	if err = recalculateTx(ctx, tx, reviewee); err != nil {
		return Item{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload)VALUES(gen_random_uuid(),'REVIEW',$1,'REVIEW_CREATED',jsonb_build_object('reviewee_user_id',$2::text))`, id, reviewee); err != nil {
		return Item{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO fraud_signals(user_id,entity_type,entity_id,signal_type,severity,evidence)SELECT $1,'REVIEW',$2,'REVIEW_VELOCITY',2,jsonb_build_object('reviews_last_hour',count(*)) FROM reviews WHERE reviewer_user_id=$1 AND created_at>now()-interval '1 hour' HAVING count(*)>5`, actor, id); err != nil {
		return Item{}, err
	}
	if err = tx.Commit(); err != nil {
		return Item{}, err
	}
	return r.get(ctx, id)
}
func (r PostgresRepository) ListPublic(ctx context.Context, username string, c *Cursor, l int) (PublicProfileData, error) {
	var user string
	// Reviews are public artifacts of completed transactions and must be viewable for
	// both sides of the marketplace: freelancers (customer→freelancer reviews) AND
	// customers (freelancer→customer reviews). Customers have no professional_profiles
	// row, so resolve any ACTIVE, non-deleted user by username rather than gating on a
	// public professional profile.
	err := r.DB.QueryRowContext(ctx, `SELECT id::text FROM users WHERE username_normalized=lower($1) AND status='ACTIVE' AND deleted_at IS NULL`, username).Scan(&user)
	if errors.Is(err, sql.ErrNoRows) {
		return PublicProfileData{}, ErrNotFound
	}
	if err != nil {
		return PublicProfileData{}, err
	}
	page, err := r.list(ctx, `r.reviewee_user_id=$1 AND r.status='PUBLISHED'`, user, c, l)
	if err != nil {
		return PublicProfileData{}, err
	}
	stats, err := r.stats(ctx, user)
	if errors.Is(err, sql.ErrNoRows) {
		stats = TrustStats{}
	} else if err != nil {
		return PublicProfileData{}, err
	}
	return PublicProfileData{Reviews: page, Trust: stats}, nil
}
func (r PostgresRepository) ListGiven(ctx context.Context, actor string, c *Cursor, l int) (Page, error) {
	return r.list(ctx, `r.reviewer_user_id=$1`, actor, c, l)
}
func (r PostgresRepository) list(ctx context.Context, condition, user string, c *Cursor, l int) (Page, error) {
	var at, id any
	if c != nil {
		at, id = c.At, c.ID
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT `+columns+fromReviews+` WHERE `+condition+` AND($2::timestamptz IS NULL OR(r.created_at,r.id)<($2,$3::uuid))ORDER BY r.created_at DESC,r.id DESC LIMIT $4`, user, at, id, l+1)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	items := []Item{}
	for rows.Next() {
		v, e := scan(rows)
		if e != nil {
			return Page{}, e
		}
		items = append(items, v)
	}
	if err = rows.Err(); err != nil {
		return Page{}, err
	}
	for i := range items {
		dims, e := r.dimensions(ctx, items[i].ID)
		if e != nil {
			return Page{}, e
		}
		items[i].Dimensions = dims
	}
	p := Page{Items: items}
	if len(items) > l {
		last := items[l-1]
		p.Items = items[:l]
		p.NextCursor = &Cursor{At: last.CreatedAt, ID: last.ID}
	}
	return p, nil
}
func (r PostgresRepository) Recalculate(ctx context.Context, user string) (TrustStats, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return TrustStats{}, err
	}
	defer tx.Rollback()
	if err = recalculateTx(ctx, tx, user); err != nil {
		return TrustStats{}, err
	}
	if err = tx.Commit(); err != nil {
		return TrustStats{}, err
	}
	return r.stats(ctx, user)
}
func (r PostgresRepository) Report(ctx context.Context, actor, id string, in ReportInput) error {
	var exists bool
	if err := r.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM reviews WHERE id=$1)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	_, err := r.DB.ExecContext(ctx, `INSERT INTO reports(reporter_user_id,entity_type,entity_id,reason_code,description)VALUES($1,'REVIEW',$2,$3,NULLIF($4,''))`, actor, id, in.ReasonCode, in.Description)
	return mapError(err)
}
func (r PostgresRepository) Moderate(ctx context.Context, actor, id, action, reason string) (Item, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Item{}, err
	}
	defer tx.Rollback()
	var allowed bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN user_roles ur ON ur.user_id=u.id WHERE u.id=$1 AND u.status='ACTIVE' AND u.deleted_at IS NULL AND ur.role IN('MODERATOR','ADMIN','SUPER_ADMIN'))`, actor).Scan(&allowed); err != nil {
		return Item{}, err
	}
	if !allowed {
		return Item{}, ErrForbidden
	}
	target := "HIDDEN"
	if action == "restore" {
		target = "PUBLISHED"
	}
	var reviewee, reviewer, text string
	err = tx.QueryRowContext(ctx, `UPDATE reviews SET status=$2,updated_at=now() WHERE id=$1 RETURNING reviewee_user_id::text,reviewer_user_id::text,COALESCE(text,'Отзыв')`, id, target).Scan(&reviewee, &reviewer, &text)
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, err
	}
	if err = recalculateTx(ctx, tx, reviewee); err != nil {
		return Item{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_logs(id,actor_user_id,action,target_type,target_id,metadata,ip)VALUES(gen_random_uuid(),$1,$2,'REVIEW',$3,jsonb_build_object('reason',$4::text),NULLIF($5::text,'')::inet)`, actor, "REVIEW_"+target, id, reason, requestmeta.FromContext(ctx)); err != nil {
		return Item{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload)VALUES(gen_random_uuid(),'REVIEW',$1,$2,jsonb_build_object('reviewee_user_id',$3::text))`, id, "REVIEW_"+target, reviewee); err != nil {
		return Item{}, err
	}
	if action == "reject" || action == "delete" {
		label := "Отзыв отклонён"
		if action == "delete" {
			label = "Отзыв удалён"
		}
		if err = supportnotify.ModerationNotice(ctx, tx, reviewer, "REVIEW", id, text, label, reason); err != nil {
			return Item{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return Item{}, err
	}
	return r.get(ctx, id)
}
func recalculateTx(ctx context.Context, tx *sql.Tx, user string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO user_trust_stats(user_id,native_rating,reviews_count,completed_projects_count,recommendation_rate,updated_at)SELECT $1,round(avg(r.rating_overall) FILTER(WHERE r.status='PUBLISHED'),2),count(*) FILTER(WHERE r.status='PUBLISHED'),(SELECT count(DISTINCT p.id) FROM projects p LEFT JOIN project_assignments a ON a.project_id=p.id WHERE p.status='COMPLETED' AND(p.customer_user_id=$1 OR a.freelancer_user_id=$1)),CASE WHEN count(r.would_work_again) FILTER(WHERE r.status='PUBLISHED')>=3 THEN round(100.0*count(*) FILTER(WHERE r.status='PUBLISHED' AND r.would_work_again=true)/NULLIF(count(r.would_work_again) FILTER(WHERE r.status='PUBLISHED'),0),2) END,now() FROM reviews r WHERE r.reviewee_user_id=$1 ON CONFLICT(user_id)DO UPDATE SET native_rating=EXCLUDED.native_rating,reviews_count=EXCLUDED.reviews_count,completed_projects_count=EXCLUDED.completed_projects_count,recommendation_rate=EXCLUDED.recommendation_rate,updated_at=EXCLUDED.updated_at`, user)
	return err
}
func (r PostgresRepository) stats(ctx context.Context, user string) (TrustStats, error) {
	var v TrustStats
	err := r.DB.QueryRowContext(ctx, `SELECT native_rating,reviews_count,completed_projects_count,recommendation_rate,updated_at FROM user_trust_stats WHERE user_id=$1`, user).Scan(&v.NativeRating, &v.ReviewsCount, &v.CompletedProjectsCount, &v.RecommendationRate, &v.UpdatedAt)
	return v, err
}
func (r PostgresRepository) get(ctx context.Context, id string) (Item, error) {
	v, err := scan(r.DB.QueryRowContext(ctx, `SELECT `+columns+fromReviews+` WHERE r.id=$1`, id))
	if err != nil {
		return Item{}, err
	}
	v.Dimensions, err = r.dimensions(ctx, id)
	return v, err
}
func (r PostgresRepository) dimensions(ctx context.Context, id string) (map[string]int, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT dimension,score FROM review_dimensions WHERE review_id=$1 ORDER BY dimension`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	v := map[string]int{}
	for rows.Next() {
		var k string
		var score int
		if err := rows.Scan(&k, &score); err != nil {
			return nil, err
		}
		v[k] = score
	}
	return v, rows.Err()
}
func mapError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return ErrConflict
	}
	if errors.As(err, &pg) && pg.Code == "23514" {
		return ErrInvalid
	}
	return err
}

var _ = time.UTC
