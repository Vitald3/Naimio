package blog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"freelance/apps/api/internal/platform/requestmeta"
	"github.com/jackc/pgx/v5/pgconn"
	"regexp"
	"strings"
	"time"
)

type PostgresRepository struct{ DB *sql.DB }

func (r PostgresRepository) CanUsePurpose(ctx context.Context, actor, purpose string) (bool, error) {
	if purpose != "BLOG_COVER" && purpose != "BLOG_CONTENT" {
		return false, nil
	}
	return r.IsAdmin(ctx, actor)
}
func (r PostgresRepository) FeatureEnabled(ctx context.Context) (bool, error) {
	var v bool
	err := r.DB.QueryRowContext(ctx, `SELECT enabled FROM feature_flags WHERE key='blog_enabled'`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return v, err
}
func (r PostgresRepository) IsAdmin(ctx context.Context, actor string) (bool, error) {
	var ok bool
	err := r.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN user_roles ur ON ur.user_id=u.id WHERE u.id=$1 AND u.status='ACTIVE' AND u.deleted_at IS NULL AND ur.role IN('ADMIN','SUPER_ADMIN'))`, actor).Scan(&ok)
	return ok, err
}
func (r PostgresRepository) PublishDue(ctx context.Context, now time.Time) error {
	_, err := r.DB.ExecContext(ctx, `WITH due AS(UPDATE blog_posts SET status='PUBLISHED',published_at=scheduled_at,updated_at=$1 WHERE status='SCHEDULED' AND scheduled_at<=$1 RETURNING id) INSERT INTO audit_logs(id,action,target_type,target_id,metadata) SELECT gen_random_uuid(),'blog_post.auto_published','BLOG_POST',id,'{"source":"schedule"}'::jsonb FROM due`, now)
	return err
}

const postColumns = `p.id::text,p.author_user_id::text,u.display_name,COALESCE(p.category_id::text,''),COALESCE(c.id::text,''),COALESCE(c.name,''),COALESCE(c.slug,''),COALESCE(c.description,''),c.created_at,c.updated_at,COALESCE(p.cover_media_object_id::text,''),p.title,p.slug,p.excerpt,p.content_html,COALESCE(p.cover_alt,''),p.status,COALESCE(p.seo_title,''),COALESCE(p.seo_description,''),COALESCE(p.canonical_url,''),p.published_at,p.scheduled_at,p.created_at,p.updated_at`

var tagsPattern = regexp.MustCompile(`<[^>]+>`)

func scanPost(row interface{ Scan(...any) error }) (Post, error) {
	var p Post
	var catID, catName, catSlug, catDesc string
	var catCreated, catUpdated sql.NullTime
	err := row.Scan(&p.ID, &p.AuthorUserID, &p.AuthorName, &p.CategoryID, &catID, &catName, &catSlug, &catDesc, &catCreated, &catUpdated, &p.CoverMediaObjectID, &p.Title, &p.Slug, &p.Excerpt, &p.ContentHTML, &p.CoverAlt, &p.Status, &p.SEOTitle, &p.SEODescription, &p.CanonicalURL, &p.PublishedAt, &p.ScheduledAt, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return p, err
	}
	if catID != "" {
		p.Category = &Category{ID: catID, Name: catName, Slug: catSlug, Description: catDesc}
		if catCreated.Valid {
			p.Category.CreatedAt = catCreated.Time
		}
		if catUpdated.Valid {
			p.Category.UpdatedAt = catUpdated.Time
		}
	}
	words := len(strings.Fields(tagsPattern.ReplaceAllString(p.ContentHTML, " ")))
	p.ReadingMinutes = (words + 179) / 180
	if p.ReadingMinutes < 1 {
		p.ReadingMinutes = 1
	}
	if p.CoverMediaObjectID != "" {
		p.CoverURL = "/api/v1/blog/media/" + p.CoverMediaObjectID
	}
	p.Tags = []Tag{}
	return p, nil
}
func (r PostgresRepository) loadTags(ctx context.Context, items []Post) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, len(items))
	index := map[string]int{}
	for i := range items {
		ids[i] = items[i].ID
		index[items[i].ID] = i
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT pt.post_id::text,t.id::text,t.name,t.slug,t.created_at,t.updated_at FROM blog_post_tags pt JOIN blog_tags t ON t.id=pt.tag_id WHERE pt.post_id=ANY($1::uuid[]) ORDER BY t.name`, `{`+strings.Join(ids, ",")+`}`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var pid string
		var t Tag
		if err = rows.Scan(&pid, &t.ID, &t.Name, &t.Slug, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return err
		}
		i := index[pid]
		items[i].Tags = append(items[i].Tags, t)
	}
	return rows.Err()
}
func (r PostgresRepository) ListPublic(ctx context.Context, category string, page, size int) (Page, error) {
	offset := (page - 1) * size
	rows, err := r.DB.QueryContext(ctx, `SELECT `+postColumns+`,count(*) OVER() FROM blog_posts p JOIN users u ON u.id=p.author_user_id LEFT JOIN blog_categories c ON c.id=p.category_id WHERE p.status='PUBLISHED' AND p.published_at<=now() AND ($1='' OR c.slug=$1) ORDER BY p.published_at DESC,p.id DESC LIMIT $2 OFFSET $3`, category, size, offset)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	items := []Post{}
	var total int64
	for rows.Next() {
		var p Post
		var catID, catName, catSlug, catDesc string
		var cc, cu sql.NullTime
		err = rows.Scan(&p.ID, &p.AuthorUserID, &p.AuthorName, &p.CategoryID, &catID, &catName, &catSlug, &catDesc, &cc, &cu, &p.CoverMediaObjectID, &p.Title, &p.Slug, &p.Excerpt, &p.ContentHTML, &p.CoverAlt, &p.Status, &p.SEOTitle, &p.SEODescription, &p.CanonicalURL, &p.PublishedAt, &p.ScheduledAt, &p.CreatedAt, &p.UpdatedAt, &total)
		if err != nil {
			return Page{}, err
		}
		if catID != "" {
			p.Category = &Category{ID: catID, Name: catName, Slug: catSlug, Description: catDesc}
			if cc.Valid {
				p.Category.CreatedAt = cc.Time
			}
			if cu.Valid {
				p.Category.UpdatedAt = cu.Time
			}
		}
		words := len(strings.Fields(tagsPattern.ReplaceAllString(p.ContentHTML, " ")))
		p.ReadingMinutes = (words + 179) / 180
		if p.ReadingMinutes < 1 {
			p.ReadingMinutes = 1
		}
		if p.CoverMediaObjectID != "" {
			p.CoverURL = "/api/v1/blog/media/" + p.CoverMediaObjectID
		}
		p.Tags = []Tag{}
		items = append(items, p)
	}
	if err = rows.Err(); err != nil {
		return Page{}, err
	}
	if err = r.loadTags(ctx, items); err != nil {
		return Page{}, err
	}
	return Page{Items: items, Page: page, PageSize: size, Total: total, HasMore: int64(offset+len(items)) < total}, nil
}
func (r PostgresRepository) GetPublic(ctx context.Context, slug string) (Post, error) {
	p, err := scanPost(r.DB.QueryRowContext(ctx, `SELECT `+postColumns+` FROM blog_posts p JOIN users u ON u.id=p.author_user_id LEFT JOIN blog_categories c ON c.id=p.category_id WHERE p.slug=$1 AND p.status='PUBLISHED' AND p.published_at<=now()`, slug))
	if errors.Is(err, sql.ErrNoRows) {
		return Post{}, ErrNotFound
	}
	if err != nil {
		return p, err
	}
	items := []Post{p}
	err = r.loadTags(ctx, items)
	return items[0], err
}
func (r PostgresRepository) Related(ctx context.Context, id, categoryID string, limit int) ([]Post, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT `+postColumns+` FROM blog_posts p JOIN users u ON u.id=p.author_user_id LEFT JOIN blog_categories c ON c.id=p.category_id WHERE p.id<>$1 AND p.status='PUBLISHED' AND p.published_at<=now() AND ($2='' OR p.category_id=$2::uuid) ORDER BY p.published_at DESC LIMIT $3`, id, categoryID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Post{}
	for rows.Next() {
		p, e := scanPost(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (r PostgresRepository) ListAdmin(ctx context.Context, status string, page, size int) (Page, error) {
	offset := (page - 1) * size
	rows, err := r.DB.QueryContext(ctx, `SELECT `+postColumns+`,count(*) OVER() FROM blog_posts p JOIN users u ON u.id=p.author_user_id LEFT JOIN blog_categories c ON c.id=p.category_id WHERE ($1='' OR p.status=$1) ORDER BY p.updated_at DESC,p.id DESC LIMIT $2 OFFSET $3`, status, size, offset)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	out := []Post{}
	var total int64
	for rows.Next() {
		var p Post
		var catID, catName, catSlug, catDesc string
		var cc, cu sql.NullTime
		if err = rows.Scan(&p.ID, &p.AuthorUserID, &p.AuthorName, &p.CategoryID, &catID, &catName, &catSlug, &catDesc, &cc, &cu, &p.CoverMediaObjectID, &p.Title, &p.Slug, &p.Excerpt, &p.ContentHTML, &p.CoverAlt, &p.Status, &p.SEOTitle, &p.SEODescription, &p.CanonicalURL, &p.PublishedAt, &p.ScheduledAt, &p.CreatedAt, &p.UpdatedAt, &total); err != nil {
			return Page{}, err
		}
		if catID != "" {
			p.Category = &Category{ID: catID, Name: catName, Slug: catSlug}
		}
		if p.CoverMediaObjectID != "" {
			p.CoverURL = "/api/v1/blog/media/" + p.CoverMediaObjectID
		}
		p.Tags = []Tag{}
		out = append(out, p)
	}
	if err = rows.Err(); err != nil {
		return Page{}, err
	}
	if err = r.loadTags(ctx, out); err != nil {
		return Page{}, err
	}
	return Page{Items: out, Page: page, PageSize: size, Total: total, HasMore: int64(offset+len(out)) < total}, nil
}
func (r PostgresRepository) GetAdmin(ctx context.Context, id string) (Post, error) {
	p, err := scanPost(r.DB.QueryRowContext(ctx, `SELECT `+postColumns+` FROM blog_posts p JOIN users u ON u.id=p.author_user_id LEFT JOIN blog_categories c ON c.id=p.category_id WHERE p.id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Post{}, ErrNotFound
	}
	if err != nil {
		return p, err
	}
	items := []Post{p}
	err = r.loadTags(ctx, items)
	return items[0], err
}

func auditBlog(ctx context.Context, tx *sql.Tx, actor, action, targetType, target, reason, requestID string, meta map[string]any) error {
	if meta == nil {
		meta = map[string]any{}
	}
	meta["reason"] = reason
	meta["request_id"] = requestID
	raw, _ := json.Marshal(meta)
	_, err := tx.ExecContext(ctx, `INSERT INTO audit_logs(id,actor_user_id,action,target_type,target_id,metadata,ip)VALUES(gen_random_uuid(),$1,$2,$3,$4,$5::jsonb,NULLIF($6,'')::inet)`, actor, action, targetType, target, raw, requestmeta.FromContext(ctx))
	return err
}
func replacePostTags(ctx context.Context, tx *sql.Tx, id string, tagIDs []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM blog_post_tags WHERE post_id=$1`, id); err != nil {
		return err
	}
	if len(tagIDs) == 0 {
		return nil
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO blog_post_tags(post_id,tag_id) SELECT $1,id FROM blog_tags WHERE id=ANY($2::uuid[]) ON CONFLICT DO NOTHING`, id, `{`+strings.Join(tagIDs, ",")+`}`)
	if err != nil {
		return ErrInvalid
	}
	n, _ := res.RowsAffected()
	if int(n) != len(tagIDs) {
		return ErrInvalid
	}
	return nil
}
func savePost(ctx context.Context, tx *sql.Tx, actor, id string, in WriteRequest, create bool) (string, error) {
	var category any = nil
	if in.CategoryID != "" {
		category = in.CategoryID
	}
	var cover any = nil
	if in.CoverMediaObjectID != "" {
		cover = in.CoverMediaObjectID
	}
	var published any = nil
	var scheduled any = nil
	if in.Status == "PUBLISHED" {
		published = time.Now().UTC()
	}
	if in.Status == "SCHEDULED" {
		scheduled = in.ScheduledAt
	}
	if create {
		err := tx.QueryRowContext(ctx, `INSERT INTO blog_posts(author_user_id,category_id,cover_media_object_id,title,slug,excerpt,content_html,cover_alt,status,seo_title,seo_description,canonical_url,published_at,scheduled_at) SELECT $1,$2::uuid,m.id,$4,$5,$6,$7,NULLIF($8,''),$9,NULLIF($10,''),NULLIF($11,''),NULLIF($12,''),$13,$14 FROM (SELECT 1)x LEFT JOIN media_objects m ON m.id=NULLIF($3::text,'')::uuid AND m.owner_user_id=$1 AND m.purpose='BLOG_COVER' AND m.scan_status='CLEAN' AND m.uploaded_at IS NOT NULL AND m.deleted_at IS NULL WHERE ($3::text='' OR m.id IS NOT NULL) RETURNING id::text`, actor, category, func() any {
			if cover == nil {
				return ""
			}
			return cover
		}(), in.Title, in.Slug, in.Excerpt, in.ContentHTML, in.CoverAlt, in.Status, in.SEOTitle, in.SEODescription, in.CanonicalURL, published, scheduled).Scan(&id)
		return id, mapBlogError(err)
	}
	res, err := tx.ExecContext(ctx, `UPDATE blog_posts p SET category_id=$3::uuid,cover_media_object_id=m.id,title=$4,slug=$5,excerpt=$6,content_html=$7,cover_alt=NULLIF($8,''),status=$9,seo_title=NULLIF($10,''),seo_description=NULLIF($11,''),canonical_url=NULLIF($12,''),published_at=CASE WHEN $9='PUBLISHED' THEN COALESCE(p.published_at,now()) ELSE NULL END,scheduled_at=$13,updated_at=now() FROM (SELECT mo.id FROM (SELECT 1)x LEFT JOIN media_objects mo ON mo.id=NULLIF($2::text,'')::uuid AND mo.owner_user_id=$14 AND mo.purpose='BLOG_COVER' AND mo.scan_status='CLEAN' AND mo.uploaded_at IS NOT NULL AND mo.deleted_at IS NULL WHERE ($2::text='' OR mo.id IS NOT NULL))m WHERE p.id=$1`, id, func() any {
		if cover == nil {
			return ""
		}
		return cover
	}(), category, in.Title, in.Slug, in.Excerpt, in.ContentHTML, in.CoverAlt, in.Status, in.SEOTitle, in.SEODescription, in.CanonicalURL, scheduled, actor)
	if err != nil {
		return id, mapBlogError(err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return id, ErrNotFound
	}
	return id, nil
}
func mapBlogError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalid
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		if pg.Code == "23505" {
			return ErrConflict
		}
		if strings.HasPrefix(pg.Code, "22") || strings.HasPrefix(pg.Code, "23") {
			return ErrInvalid
		}
	}
	return err
}
func (r PostgresRepository) Create(ctx context.Context, actor string, in WriteRequest, reason, requestID string) (Post, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Post{}, err
	}
	defer tx.Rollback()
	id, err := savePost(ctx, tx, actor, "", in, true)
	if err != nil {
		return Post{}, err
	}
	if err = replacePostTags(ctx, tx, id, in.TagIDs); err != nil {
		return Post{}, err
	}
	action := "blog_post.created"
	if in.Status == "PUBLISHED" {
		action = "blog_post.published"
	}
	if err = auditBlog(ctx, tx, actor, action, "BLOG_POST", id, reason, requestID, map[string]any{"status": in.Status}); err != nil {
		return Post{}, err
	}
	if err = tx.Commit(); err != nil {
		return Post{}, err
	}
	return r.GetAdmin(ctx, id)
}
func (r PostgresRepository) Update(ctx context.Context, actor, id string, in WriteRequest, reason, requestID string) (Post, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Post{}, err
	}
	defer tx.Rollback()
	var old string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM blog_posts WHERE id=$1 FOR UPDATE`, id).Scan(&old); errors.Is(err, sql.ErrNoRows) {
		return Post{}, ErrNotFound
	}
	if err != nil {
		return Post{}, err
	}
	if _, err = savePost(ctx, tx, actor, id, in, false); err != nil {
		return Post{}, err
	}
	if err = replacePostTags(ctx, tx, id, in.TagIDs); err != nil {
		return Post{}, err
	}
	action := "blog_post.updated"
	if old != "PUBLISHED" && in.Status == "PUBLISHED" {
		action = "blog_post.published"
	}
	if old == "PUBLISHED" && in.Status != "PUBLISHED" {
		action = "blog_post.unpublished"
	}
	if err = auditBlog(ctx, tx, actor, action, "BLOG_POST", id, reason, requestID, map[string]any{"from_status": old, "to_status": in.Status}); err != nil {
		return Post{}, err
	}
	if err = tx.Commit(); err != nil {
		return Post{}, err
	}
	return r.GetAdmin(ctx, id)
}
func (r PostgresRepository) Archive(ctx context.Context, actor, id, reason, requestID string) (Post, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Post{}, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE blog_posts SET status='ARCHIVED',scheduled_at=NULL,updated_at=now() WHERE id=$1 AND status<>'ARCHIVED'`, id)
	if err != nil {
		return Post{}, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return Post{}, ErrConflict
	}
	if err = auditBlog(ctx, tx, actor, "blog_post.archived", "BLOG_POST", id, reason, requestID, nil); err != nil {
		return Post{}, err
	}
	if err = tx.Commit(); err != nil {
		return Post{}, err
	}
	return r.GetAdmin(ctx, id)
}
func (r PostgresRepository) Delete(ctx context.Context, actor, id, reason, requestID string) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM blog_posts WHERE id=$1 FOR UPDATE`, id).Scan(&status); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status != "DRAFT" && status != "ARCHIVED" {
		return ErrConflict
	}
	if err = auditBlog(ctx, tx, actor, "blog_post.deleted", "BLOG_POST", id, reason, requestID, map[string]any{"status": status}); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM blog_posts WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (r PostgresRepository) ListCategories(ctx context.Context) ([]Category, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT id::text,name,slug,description,created_at,updated_at FROM blog_categories ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		var v Category
		if err = rows.Scan(&v.ID, &v.Name, &v.Slug, &v.Description, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r PostgresRepository) ListTags(ctx context.Context) ([]Tag, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT id::text,name,slug,created_at,updated_at FROM blog_tags ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tag{}
	for rows.Next() {
		var v Tag
		if err = rows.Scan(&v.ID, &v.Name, &v.Slug, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r PostgresRepository) SaveCategory(ctx context.Context, actor string, v Category, reason, requestID string) (Category, error) {
	v.Name = strings.TrimSpace(v.Name)
	v.Slug = strings.ToLower(strings.TrimSpace(v.Slug))
	v.Description = strings.TrimSpace(v.Description)
	if v.Name == "" || len(v.Name) > 100 || !slugPattern.MatchString(v.Slug) || len(v.Description) > 500 {
		return Category{}, ErrInvalid
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Category{}, err
	}
	defer tx.Rollback()
	if v.ID == "" {
		err = tx.QueryRowContext(ctx, `INSERT INTO blog_categories(name,slug,description)VALUES($1,$2,$3)RETURNING id::text`, v.Name, v.Slug, v.Description).Scan(&v.ID)
	} else {
		var res sql.Result
		res, err = tx.ExecContext(ctx, `UPDATE blog_categories SET name=$2,slug=$3,description=$4,updated_at=now()WHERE id=$1`, v.ID, v.Name, v.Slug, v.Description)
		if err == nil {
			if n, _ := res.RowsAffected(); n != 1 {
				err = ErrNotFound
			}
		}
	}
	if err != nil {
		return Category{}, mapBlogError(err)
	}
	if err = auditBlog(ctx, tx, actor, "blog_category.saved", "BLOG_CATEGORY", v.ID, reason, requestID, nil); err != nil {
		return Category{}, err
	}
	if err = tx.Commit(); err != nil {
		return Category{}, err
	}
	items, err := r.ListCategories(ctx)
	for _, x := range items {
		if x.ID == v.ID {
			return x, err
		}
	}
	return Category{}, ErrNotFound
}
func (r PostgresRepository) SaveTag(ctx context.Context, actor string, v Tag, reason, requestID string) (Tag, error) {
	v.Name = strings.TrimSpace(v.Name)
	v.Slug = strings.ToLower(strings.TrimSpace(v.Slug))
	if v.Name == "" || len(v.Name) > 80 || !slugPattern.MatchString(v.Slug) {
		return Tag{}, ErrInvalid
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Tag{}, err
	}
	defer tx.Rollback()
	if v.ID == "" {
		err = tx.QueryRowContext(ctx, `INSERT INTO blog_tags(name,slug)VALUES($1,$2)RETURNING id::text`, v.Name, v.Slug).Scan(&v.ID)
	} else {
		var res sql.Result
		res, err = tx.ExecContext(ctx, `UPDATE blog_tags SET name=$2,slug=$3,updated_at=now()WHERE id=$1`, v.ID, v.Name, v.Slug)
		if err == nil {
			if n, _ := res.RowsAffected(); n != 1 {
				err = ErrNotFound
			}
		}
	}
	if err != nil {
		return Tag{}, mapBlogError(err)
	}
	if err = auditBlog(ctx, tx, actor, "blog_tag.saved", "BLOG_TAG", v.ID, reason, requestID, nil); err != nil {
		return Tag{}, err
	}
	if err = tx.Commit(); err != nil {
		return Tag{}, err
	}
	items, err := r.ListTags(ctx)
	for _, x := range items {
		if x.ID == v.ID {
			return x, err
		}
	}
	return Tag{}, ErrNotFound
}
func deleteTaxonomy(ctx context.Context, db *sql.DB, actor, id, reason, requestID, table, target string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = auditBlog(ctx, tx, actor, "blog_"+strings.ToLower(strings.TrimPrefix(target, "BLOG_"))+".deleted", target, id, reason, requestID, nil); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrNotFound
	}
	return tx.Commit()
}
func (r PostgresRepository) DeleteCategory(ctx context.Context, actor, id, reason, requestID string) error {
	return deleteTaxonomy(ctx, r.DB, actor, id, reason, requestID, "blog_categories", "BLOG_CATEGORY")
}
func (r PostgresRepository) DeleteTag(ctx context.Context, actor, id, reason, requestID string) error {
	return deleteTaxonomy(ctx, r.DB, actor, id, reason, requestID, "blog_tags", "BLOG_TAG")
}
func (r PostgresRepository) PublicMediaKey(ctx context.Context, id string) (string, error) {
	var key string
	err := r.DB.QueryRowContext(ctx, `SELECT m.object_key FROM media_objects m WHERE m.id=$1 AND m.purpose IN('BLOG_COVER','BLOG_CONTENT') AND m.scan_status='CLEAN' AND m.uploaded_at IS NOT NULL AND m.deleted_at IS NULL AND EXISTS(SELECT 1 FROM blog_posts p WHERE p.status='PUBLISHED' AND p.published_at<=now() AND (p.cover_media_object_id=m.id OR p.content_html LIKE '%/api/v1/blog/media/'||m.id::text||'%'))`, id).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return key, err
}
