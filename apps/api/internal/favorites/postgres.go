package favorites

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

type PostgresRepository struct{ DB *sql.DB }

func (r PostgresRepository) Put(ctx context.Context, actor, kind, id string) (Item, error) {
	kind = strings.ToUpper(kind)
	if !validType(kind) || !uuidPattern.MatchString(strings.ToLower(id)) {
		return Item{}, ErrInvalid
	}
	ok, err := r.visible(ctx, kind, id)
	if err != nil {
		return Item{}, err
	}
	if !ok {
		return Item{}, ErrNotFound
	}
	var v Item
	err = r.DB.QueryRowContext(ctx, `INSERT INTO favorites(user_id,entity_type,entity_id)VALUES($1,$2,$3)ON CONFLICT(user_id,entity_type,entity_id)DO UPDATE SET entity_type=EXCLUDED.entity_type RETURNING entity_type,entity_id::text,created_at`, actor, kind, id).Scan(&v.EntityType, &v.EntityID, &v.CreatedAt)
	return v, err
}
func (r PostgresRepository) Delete(ctx context.Context, actor, kind, id string) error {
	kind = strings.ToUpper(kind)
	if !validType(kind) || !uuidPattern.MatchString(strings.ToLower(id)) {
		return ErrInvalid
	}
	_, err := r.DB.ExecContext(ctx, `DELETE FROM favorites WHERE user_id=$1 AND entity_type=$2 AND entity_id=$3`, actor, kind, id)
	return err
}
func (r PostgresRepository) List(ctx context.Context, actor, kind string, c *Cursor, l int) (Page, error) {
	kind = strings.ToUpper(strings.TrimSpace(kind))
	if kind != "" && !validType(kind) {
		return Page{}, ErrInvalid
	}
	if l < 1 {
		l = 20
	}
	if l > 50 {
		l = 50
	}
	var at, id any
	if c != nil {
		at, id = c.At, c.ID
	}
	rows, err := r.DB.QueryContext(ctx, `
SELECT f.entity_type,f.entity_id::text,f.created_at,
CASE WHEN f.entity_type='FREELANCER' THEN COALESCE(fu.display_name,'') WHEN f.entity_type='SERVICE' THEN COALESCE(s.title,'') WHEN f.entity_type='PROJECT' THEN COALESCE(p.title,'') ELSE '' END title,
CASE WHEN f.entity_type='FREELANCER' THEN COALESCE(pp.professional_title,'') WHEN f.entity_type='SERVICE' THEN COALESCE(s.short_description,'') WHEN f.entity_type='PROJECT' THEN COALESCE(left(p.description,240),'') ELSE '' END subtitle,
CASE WHEN f.entity_type='FREELANCER' THEN COALESCE(fu.username,'') ELSE '' END username,
CASE WHEN f.entity_type='SERVICE' THEN COALESCE(s.slug,'') WHEN f.entity_type='PROJECT' THEN COALESCE(p.slug,'') ELSE '' END slug,
COALESCE(c.name,''),
CASE WHEN f.entity_type='FREELANCER' THEN pp.hourly_rate_kopecks WHEN f.entity_type='SERVICE' THEN s.price_from_kopecks WHEN f.entity_type='PROJECT' THEN COALESCE(p.budget_max_kopecks,p.budget_min_kopecks) END amount_kopecks,
CASE WHEN f.entity_type='FREELANCER' THEN ts.native_rating::float8 ELSE NULL END rating,
CASE WHEN f.entity_type='FREELANCER' THEN COALESCE(ts.reviews_count,0) ELSE 0 END reviews_count
FROM favorites f
LEFT JOIN users fu ON f.entity_type='FREELANCER' AND fu.id=f.entity_id AND fu.status='ACTIVE' AND fu.deleted_at IS NULL
LEFT JOIN professional_profiles pp ON f.entity_type='FREELANCER' AND pp.user_id=fu.id AND pp.profile_visibility='PUBLIC'
LEFT JOIN user_trust_stats ts ON ts.user_id=fu.id
LEFT JOIN services s ON f.entity_type='SERVICE' AND s.id=f.entity_id AND s.status='ACTIVE' AND s.visibility='PUBLIC' AND s.deleted_at IS NULL
LEFT JOIN projects p ON f.entity_type='PROJECT' AND p.id=f.entity_id AND p.status IN('OPEN','MATCHING') AND p.visibility='PUBLIC' AND p.deleted_at IS NULL
LEFT JOIN categories c ON c.id=COALESCE(s.category_id,p.category_id) AND c.is_active=true
WHERE f.user_id=$1 AND($2='' OR f.entity_type=$2) AND($3::timestamptz IS NULL OR(f.created_at,f.entity_id)<($3,$4::uuid))
AND ((f.entity_type='FREELANCER' AND fu.id IS NOT NULL AND pp.user_id IS NOT NULL) OR (f.entity_type='SERVICE' AND s.id IS NOT NULL) OR (f.entity_type='PROJECT' AND p.id IS NOT NULL))
ORDER BY f.created_at DESC,f.entity_id DESC LIMIT $5`, actor, kind, at, id, l+1)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	items := []Item{}
	for rows.Next() {
		var v Item
		if err := rows.Scan(&v.EntityType, &v.EntityID, &v.CreatedAt, &v.Title, &v.Subtitle, &v.Username, &v.Slug, &v.Category, &v.AmountKopecks, &v.Rating, &v.ReviewsCount); err != nil {
			return Page{}, err
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
		p.NextCursor = &Cursor{At: last.CreatedAt, ID: last.EntityID}
	}
	return p, nil
}

func (r PostgresRepository) visible(ctx context.Context, kind, id string) (bool, error) {
	query := map[string]string{"FREELANCER": `SELECT EXISTS(SELECT 1 FROM users u JOIN professional_profiles p ON p.user_id=u.id WHERE u.id=$1 AND u.status='ACTIVE'AND u.deleted_at IS NULL AND p.profile_visibility='PUBLIC')`, "SERVICE": `SELECT EXISTS(SELECT 1 FROM services s JOIN users u ON u.id=s.seller_user_id JOIN professional_profiles pp ON pp.user_id=u.id JOIN categories c ON c.id=s.category_id WHERE s.id=$1 AND s.status='ACTIVE'AND s.visibility='PUBLIC'AND s.deleted_at IS NULL AND u.status='ACTIVE'AND u.deleted_at IS NULL AND pp.profile_visibility='PUBLIC'AND c.is_active=true)`, "PROJECT": `SELECT EXISTS(SELECT 1 FROM projects p JOIN users u ON u.id=p.customer_user_id JOIN categories c ON c.id=p.category_id WHERE p.id=$1 AND p.status IN('OPEN','MATCHING')AND p.visibility='PUBLIC'AND p.deleted_at IS NULL AND u.status='ACTIVE'AND u.deleted_at IS NULL AND c.is_active=true)`}[kind]
	var ok bool
	err := r.DB.QueryRowContext(ctx, query, id).Scan(&ok)
	return ok, err
}

var _ = time.UTC
