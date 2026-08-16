package catalog

import (
	"context"
	"database/sql"
	"errors"
	"freelance/apps/api/internal/platform/requestmeta"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

type PostgresRepository struct{ DB *sql.DB }

func (r PostgresRepository) CategoryTree(ctx context.Context) ([]Category, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT id::text,parent_id::text,slug,name,COALESCE(description,''),sort_order,is_active FROM categories WHERE is_active=true ORDER BY sort_order,name LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Category{}
	for rows.Next() {
		var c Category
		var parent sql.NullString
		if err := rows.Scan(&c.ID, &parent, &c.Slug, &c.Name, &c.Description, &c.SortOrder, &c.Active); err != nil {
			return nil, err
		}
		if parent.Valid {
			c.ParentID = &parent.String
		}
		items = append(items, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return buildTree(items), nil
}
func (r PostgresRepository) Category(ctx context.Context, slug string) (Category, error) {
	var c Category
	var parent sql.NullString
	err := r.DB.QueryRowContext(ctx, `SELECT id::text,parent_id::text,slug,name,COALESCE(description,''),sort_order,is_active FROM categories WHERE slug=$1 AND is_active=true`, strings.ToLower(strings.TrimSpace(slug))).Scan(&c.ID, &parent, &c.Slug, &c.Name, &c.Description, &c.SortOrder, &c.Active)
	if errors.Is(err, sql.ErrNoRows) {
		return Category{}, ErrNotFound
	}
	if err != nil {
		return Category{}, err
	}
	if parent.Valid {
		c.ParentID = &parent.String
	}
	tree, err := r.CategoryTree(ctx)
	if err != nil {
		return Category{}, err
	}
	flat := flatten(tree)
	for _, candidate := range flat {
		if candidate.ID == c.ID {
			return candidate, nil
		}
	}
	return c, nil
}
func (r PostgresRepository) SearchSkills(ctx context.Context, q string) ([]Skill, error) {
	if len([]rune(q)) > 100 {
		return nil, ErrInvalid
	}
	q = strings.TrimSpace(q)
	rows, err := r.DB.QueryContext(ctx, `SELECT id::text,slug,name,is_active FROM skills WHERE is_active=true AND ($1='' OR name ILIKE '%'||$1||'%' OR slug ILIKE '%'||$1||'%') ORDER BY CASE WHEN lower(name)=lower($1) THEN 0 ELSE 1 END,name LIMIT 50`, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Skill{}
	for rows.Next() {
		var s Skill
		if err := rows.Scan(&s.ID, &s.Slug, &s.Name, &s.Active); err != nil {
			return nil, err
		}
		items = append(items, s)
	}
	return items, rows.Err()
}
func (r PostgresRepository) AdminCategories(ctx context.Context, actor string) ([]Category, error) {
	if err := r.requireAdmin(ctx, actor); err != nil {
		return nil, err
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT id::text,parent_id::text,slug,name,COALESCE(description,''),sort_order,is_active FROM categories ORDER BY sort_order,name,id LIMIT 2000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		var c Category
		var parent sql.NullString
		if err := rows.Scan(&c.ID, &parent, &c.Slug, &c.Name, &c.Description, &c.SortOrder, &c.Active); err != nil {
			return nil, err
		}
		if parent.Valid {
			c.ParentID = &parent.String
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (r PostgresRepository) AdminSkills(ctx context.Context, actor string) ([]Skill, error) {
	if err := r.requireAdmin(ctx, actor); err != nil {
		return nil, err
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT id::text,slug,name,is_active FROM skills ORDER BY name,id LIMIT 5000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Skill{}
	for rows.Next() {
		var v Skill
		if err := rows.Scan(&v.ID, &v.Slug, &v.Name, &v.Active); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r PostgresRepository) CreateCategory(ctx context.Context, actor string, in CategoryInput) (Category, error) {
	if !validCategoryInput(in) {
		return Category{}, ErrInvalid
	}
	if err := r.requireAdmin(ctx, actor); err != nil {
		return Category{}, err
	}
	var id string
	err := r.DB.QueryRowContext(ctx, `INSERT INTO categories(id,parent_id,slug,name,description,sort_order,is_active) VALUES(gen_random_uuid(),$1,NULLIF(lower(trim($2)),''),trim($3),NULLIF(trim($4),''),$5,$6) RETURNING id::text`, in.ParentID, in.Slug, in.Name, in.Description, in.SortOrder, in.Active).Scan(&id)
	if err != nil {
		return Category{}, mapError(err)
	}
	if err := r.audit(ctx, actor, "CATALOG_CATEGORY_CREATED", "CATEGORY", id); err != nil {
		return Category{}, err
	}
	return r.categoryByID(ctx, id)
}
func (r PostgresRepository) UpdateCategory(ctx context.Context, actor, id string, in CategoryInput) (Category, error) {
	if !validCategoryInput(in) {
		return Category{}, ErrInvalid
	}
	if err := r.requireAdmin(ctx, actor); err != nil {
		return Category{}, err
	}
	result, err := r.DB.ExecContext(ctx, `UPDATE categories SET parent_id=$2,slug=lower(trim($3)),name=trim($4),description=NULLIF(trim($5),''),sort_order=$6,is_active=$7,updated_at=now() WHERE id=$1`, id, in.ParentID, in.Slug, in.Name, in.Description, in.SortOrder, in.Active)
	if err != nil {
		return Category{}, mapError(err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return Category{}, ErrNotFound
	}
	if err := r.audit(ctx, actor, "CATALOG_CATEGORY_UPDATED", "CATEGORY", id); err != nil {
		return Category{}, err
	}
	return r.categoryByID(ctx, id)
}

func (r PostgresRepository) DeleteCategory(ctx context.Context, actor, id string) error {
	if err := r.requireAdmin(ctx, actor); err != nil {
		return err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var used bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM categories c WHERE c.parent_id=$1 UNION ALL SELECT 1 FROM projects WHERE category_id=$1 AND deleted_at IS NULL UNION ALL SELECT 1 FROM services WHERE category_id=$1 AND deleted_at IS NULL UNION ALL SELECT 1 FROM jobs WHERE category_id=$1 AND deleted_at IS NULL UNION ALL SELECT 1 FROM profile_categories WHERE category_id=$1)`, id).Scan(&used)
	if err != nil {
		return err
	}
	if used {
		return ErrInvalid
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM categories WHERE id=$1`, id)
	if err != nil {
		return mapError(err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrNotFound
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_logs(id,actor_user_id,action,target_type,target_id,metadata,ip)VALUES(gen_random_uuid(),$1,'CATALOG_CATEGORY_DELETED','CATEGORY',$2,'{}',NULLIF($3,'')::inet)`, actor, id, requestmeta.FromContext(ctx)); err != nil {
		return err
	}
	return tx.Commit()
}

func (r PostgresRepository) CreateSkill(ctx context.Context, actor string, in SkillInput) (Skill, error) {
	if !validInput(in.Slug, in.Name) {
		return Skill{}, ErrInvalid
	}
	if err := r.requireAdmin(ctx, actor); err != nil {
		return Skill{}, err
	}
	var value Skill
	err := r.DB.QueryRowContext(ctx, `INSERT INTO skills(id,slug,name,is_active) VALUES(gen_random_uuid(),lower(trim($1)),trim($2),$3) RETURNING id::text,slug,name,is_active`, in.Slug, in.Name, in.Active).Scan(&value.ID, &value.Slug, &value.Name, &value.Active)
	if err != nil {
		return Skill{}, mapError(err)
	}
	if err := r.audit(ctx, actor, "CATALOG_SKILL_CREATED", "SKILL", value.ID); err != nil {
		return Skill{}, err
	}
	return value, nil
}
func (r PostgresRepository) UpdateSkill(ctx context.Context, actor, id string, in SkillInput) (Skill, error) {
	if !validInput(in.Slug, in.Name) {
		return Skill{}, ErrInvalid
	}
	if err := r.requireAdmin(ctx, actor); err != nil {
		return Skill{}, err
	}
	var value Skill
	err := r.DB.QueryRowContext(ctx, `UPDATE skills SET slug=lower(trim($2)),name=trim($3),is_active=$4,updated_at=now() WHERE id=$1 RETURNING id::text,slug,name,is_active`, id, in.Slug, in.Name, in.Active).Scan(&value.ID, &value.Slug, &value.Name, &value.Active)
	if errors.Is(err, sql.ErrNoRows) {
		return Skill{}, ErrNotFound
	}
	if err != nil {
		return Skill{}, mapError(err)
	}
	if err := r.audit(ctx, actor, "CATALOG_SKILL_UPDATED", "SKILL", value.ID); err != nil {
		return Skill{}, err
	}
	return value, nil
}

func (r PostgresRepository) DeleteSkill(ctx context.Context, actor, id string) error {
	if err := r.requireAdmin(ctx, actor); err != nil {
		return err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var used bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM project_skills WHERE skill_id=$1 UNION ALL SELECT 1 FROM service_skills WHERE skill_id=$1 UNION ALL SELECT 1 FROM job_skills WHERE skill_id=$1 UNION ALL SELECT 1 FROM profile_skills WHERE skill_id=$1)`, id).Scan(&used)
	if err != nil {
		return err
	}
	if used {
		return ErrInvalid
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM skills WHERE id=$1`, id)
	if err != nil {
		return mapError(err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrNotFound
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_logs(id,actor_user_id,action,target_type,target_id,metadata,ip)VALUES(gen_random_uuid(),$1,'CATALOG_SKILL_DELETED','SKILL',$2,'{}',NULLIF($3,'')::inet)`, actor, id, requestmeta.FromContext(ctx)); err != nil {
		return err
	}
	return tx.Commit()
}

func (r PostgresRepository) audit(ctx context.Context, actor, action, targetType, targetID string) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO audit_logs(id,actor_user_id,action,target_type,target_id,metadata,ip)VALUES(gen_random_uuid(),$1,$2,$3,$4,'{}',NULLIF($5,'')::inet)`, actor, action, targetType, targetID, requestmeta.FromContext(ctx))
	return err
}
func (r PostgresRepository) requireAdmin(ctx context.Context, actor string) error {
	var ok bool
	err := r.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN user_roles ur ON ur.user_id=u.id WHERE u.id=$1 AND u.status='ACTIVE' AND u.deleted_at IS NULL AND ur.role IN('ADMIN','SUPER_ADMIN'))`, actor).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return nil
}
func (r PostgresRepository) categoryByID(ctx context.Context, id string) (Category, error) {
	var c Category
	var parent sql.NullString
	err := r.DB.QueryRowContext(ctx, `SELECT id::text,parent_id::text,slug,name,COALESCE(description,''),sort_order,is_active FROM categories WHERE id=$1`, id).Scan(&c.ID, &parent, &c.Slug, &c.Name, &c.Description, &c.SortOrder, &c.Active)
	if errors.Is(err, sql.ErrNoRows) {
		return Category{}, ErrNotFound
	}
	if parent.Valid {
		c.ParentID = &parent.String
	}
	return c, err
}
func flatten(items []Category) []Category {
	out := []Category{}
	var walk func([]Category)
	walk = func(values []Category) {
		for _, v := range values {
			out = append(out, v)
			walk(v.Children)
		}
	}
	walk(items)
	return out
}
func mapError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && (pg.Code == "23505" || pg.Code == "23503" || pg.Code == "23514") {
		return ErrInvalid
	}
	return err
}
