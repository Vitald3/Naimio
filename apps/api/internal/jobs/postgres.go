package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"freelance/apps/api/internal/platform/contentmoderation"
	"freelance/apps/api/internal/platform/requestmeta"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type PostgresRepository struct{ DB *sql.DB }
type dbtx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

const jobColumns = `j.id::text,j.customer_user_id::text,u.display_name,j.title,j.slug,j.description,j.employment_type,
j.salary_min_kopecks,j.salary_max_kopecks,j.currency,COALESCE(j.location_text,''),j.remote,COALESCE(j.experience_level,''),
j.status,j.moderation_status,COALESCE(j.moderation_reason,''),j.published_at,j.created_at,j.updated_at,
c.id::text,c.owner_user_id::text,c.name,c.slug,COALESCE(c.logo_object_key,''),COALESCE(c.website,''),COALESCE(c.description,''),c.verification_status,c.created_at,c.updated_at,
cat.id::text,cat.slug,cat.name`

func (r PostgresRepository) CreateCompany(ctx context.Context, actor string, in CompanyInput) (Company, error) {
	in = normalizeCompany(in)
	if err := validateCompany(in); err != nil {
		return Company{}, err
	}
	id, e := newUUID()
	if e != nil {
		return Company{}, e
	}
	var c Company
	e = r.DB.QueryRowContext(ctx, `INSERT INTO companies(id,owner_user_id,name,slug,website,description)
SELECT $1,$2,$3,$4,NULLIF($5,''),NULLIF($6,'') WHERE EXISTS(SELECT 1 FROM users u JOIN user_capabilities uc ON uc.user_id=u.id AND uc.capability='CUSTOMER' WHERE u.id=$2 AND u.status='ACTIVE' AND u.deleted_at IS NULL)
RETURNING id::text,owner_user_id::text,name,slug,COALESCE(logo_object_key,''),COALESCE(website,''),COALESCE(description,''),verification_status,created_at,updated_at`, id, actor, in.Name, in.Slug, in.Website, in.Description).Scan(&c.ID, &c.OwnerID, &c.Name, &c.Slug, &c.LogoObjectKey, &c.Website, &c.Description, &c.VerificationStatus, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(e, sql.ErrNoRows) {
		return Company{}, ErrIneligible
	}
	if e != nil {
		return Company{}, mapDB(e)
	}
	return c, nil
}
func (r PostgresRepository) ListCompanies(ctx context.Context, actor string) ([]Company, error) {
	rows, e := r.DB.QueryContext(ctx, `SELECT id::text,owner_user_id::text,name,slug,COALESCE(logo_object_key,''),COALESCE(website,''),COALESCE(description,''),verification_status,created_at,updated_at FROM companies WHERE owner_user_id=$1 ORDER BY created_at DESC`, actor)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Company{}
	for rows.Next() {
		var c Company
		if e = rows.Scan(&c.ID, &c.OwnerID, &c.Name, &c.Slug, &c.LogoObjectKey, &c.Website, &c.Description, &c.VerificationStatus, &c.CreatedAt, &c.UpdatedAt); e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r PostgresRepository) Create(ctx context.Context, actor string, in CreateRequest) (Item, error) {
	if contentmoderation.LooksLikeJunk(in.Title, in.Description) {
		return Item{}, fmt.Errorf("%w: Материал не прошёл автоматическую проверку. Уберите бессмысленный, повторяющийся или подозрительный текст и попробуйте снова.", ErrInvalidInput)
	}
	id, e := newUUID()
	if e != nil {
		return Item{}, e
	}
	v := fromCreate(actor, id, in, time.Now().UTC())
	if e = Validate(v); e != nil {
		return Item{}, e
	}
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return Item{}, e
	}
	defer tx.Rollback()
	if e = requireCustomer(ctx, tx, actor); e != nil {
		return Item{}, e
	}
	if e = validateRefs(ctx, tx, actor, &v); e != nil {
		return Item{}, e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO jobs(id,company_id,customer_user_id,category_id,title,slug,description,employment_type,salary_min_kopecks,salary_max_kopecks,currency,location_text,remote,experience_level)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'RUB',NULLIF($11,''),$12,NULLIF($13,''))`, id, nullableCompany(v.Company), actor, nullableRef(v.Category), v.Title, v.Slug, v.Description, v.EmploymentType, v.SalaryMinKopecks, v.SalaryMaxKopecks, v.Location, v.Remote, v.ExperienceLevel)
	if e != nil {
		return Item{}, mapDB(e)
	}
	if e = replaceSkills(ctx, tx, id, v.Skills); e != nil {
		return Item{}, e
	}
	if e = tx.Commit(); e != nil {
		return Item{}, e
	}
	return r.GetOwned(ctx, actor, id)
}
func (r PostgresRepository) GetOwned(ctx context.Context, actor, id string) (Item, error) {
	v, e := r.get(ctx, `j.id=$1 AND j.customer_user_id=$2 AND j.deleted_at IS NULL`, id, actor)
	if e != nil {
		return Item{}, e
	}
	return v, r.loadSkills(ctx, &v)
}
func (r PostgresRepository) ListOwned(ctx context.Context, actor string, c *Cursor, limit int) (Page, error) {
	limit = capped(limit)
	var at, cid any
	if c != nil {
		at, cid = c.At, c.ID
	}
	rows, e := r.DB.QueryContext(ctx, `SELECT `+jobColumns+` FROM jobs j JOIN users u ON u.id=j.customer_user_id LEFT JOIN companies c ON c.id=j.company_id LEFT JOIN categories cat ON cat.id=j.category_id
WHERE j.customer_user_id=$1 AND j.deleted_at IS NULL AND($2::timestamptz IS NULL OR(j.created_at,j.id)<($2,$3::uuid)) ORDER BY j.created_at DESC,j.id DESC LIMIT $4`, actor, at, cid, limit+1)
	if e != nil {
		return Page{}, e
	}
	items, e := scanJobs(rows)
	if e != nil {
		return Page{}, e
	}
	if e = r.loadPageSkills(ctx, items); e != nil {
		return Page{}, e
	}
	return finishPage(items, limit, false), nil
}
func (r PostgresRepository) Update(ctx context.Context, actor, id string, p PatchRequest) (Item, error) {
	v, e := r.GetOwned(ctx, actor, id)
	if e != nil {
		return Item{}, e
	}
	if v.Status != "DRAFT" {
		return Item{}, ErrInvalidState
	}
	merge(&v, p)
	if e = Validate(v); e != nil {
		return Item{}, e
	}
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return Item{}, e
	}
	defer tx.Rollback()
	if e = validateRefs(ctx, tx, actor, &v); e != nil {
		return Item{}, e
	}
	res, e := tx.ExecContext(ctx, `UPDATE jobs SET company_id=$3,category_id=$4,title=$5,slug=$6,description=$7,employment_type=$8,salary_min_kopecks=$9,salary_max_kopecks=$10,currency='RUB',location_text=NULLIF($11,''),remote=$12,experience_level=NULLIF($13,''),updated_at=now() WHERE id=$1 AND customer_user_id=$2 AND status='DRAFT' AND deleted_at IS NULL`, id, actor, nullableCompany(v.Company), nullableRef(v.Category), v.Title, v.Slug, v.Description, v.EmploymentType, v.SalaryMinKopecks, v.SalaryMaxKopecks, v.Location, v.Remote, v.ExperienceLevel)
	if e != nil {
		return Item{}, mapDB(e)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return Item{}, ErrInvalidState
	}
	if e = replaceSkills(ctx, tx, id, v.Skills); e != nil {
		return Item{}, e
	}
	if e = tx.Commit(); e != nil {
		return Item{}, e
	}
	return r.GetOwned(ctx, actor, id)
}
func (r PostgresRepository) Delete(ctx context.Context, actor, id string) error {
	current, e := r.GetOwned(ctx, actor, id)
	if e != nil {
		return e
	}
	if current.Status == "PUBLISHED" {
		return ErrInvalidState
	}
	res, e := r.DB.ExecContext(ctx, `UPDATE jobs SET status='ARCHIVED',deleted_at=now(),updated_at=now() WHERE id=$1 AND customer_user_id=$2 AND status=$3 AND deleted_at IS NULL`, id, actor, current.Status)
	if e != nil {
		return e
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrInvalidState
	}
	return nil
}
func (r PostgresRepository) Transition(ctx context.Context, actor, id, action string) (Item, error) {
	from, to := "DRAFT", "PUBLISHED"
	if action == "close" {
		from, to = "PUBLISHED", "CLOSED"
	} else if action != "publish" {
		return Item{}, ErrInvalidState
	}
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return Item{}, e
	}
	defer tx.Rollback()
	if action == "publish" {
		v, e := r.GetOwned(ctx, actor, id)
		if e != nil {
			return Item{}, e
		}
		if contentmoderation.LooksLikeJunk(v.Title, v.Description) {
			return Item{}, fmt.Errorf("%w: Материал не прошёл автоматическую проверку. Уберите бессмысленный, повторяющийся или подозрительный текст и попробуйте снова.", ErrInvalidInput)
		}
		if e = Validate(v); e != nil {
			return Item{}, e
		}
		if e = validateRefs(ctx, tx, actor, &v); e != nil {
			return Item{}, e
		}
	}
	res, e := tx.ExecContext(ctx, `UPDATE jobs SET status=$3,moderation_status=CASE WHEN $3='PUBLISHED' THEN 'VISIBLE' ELSE moderation_status END,moderation_reason=CASE WHEN $3='PUBLISHED' THEN NULL ELSE moderation_reason END,moderated_by=CASE WHEN $3='PUBLISHED' THEN NULL ELSE moderated_by END,moderated_at=CASE WHEN $3='PUBLISHED' THEN NULL ELSE moderated_at END,published_at=CASE WHEN $3='PUBLISHED' THEN COALESCE(published_at,now()) ELSE published_at END,updated_at=now() WHERE id=$1 AND customer_user_id=$2 AND status=$4 AND deleted_at IS NULL`, id, actor, to, from)
	if e != nil {
		return Item{}, e
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return Item{}, ErrInvalidState
	}
	if action == "publish" {
		_, e = tx.ExecContext(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload)VALUES(gen_random_uuid(),'JOB',$1::uuid,'VACANCY_PUBLISHED',jsonb_build_object('job_id',$1::uuid))`, id)
		if e != nil {
			return Item{}, e
		}
	}
	if e = tx.Commit(); e != nil {
		return Item{}, e
	}
	return r.GetOwned(ctx, actor, id)
}

func (r PostgresRepository) ListPublic(ctx context.Context, f Filter, c *Cursor, limit int) (Page, error) {
	if e := ValidateFilter(f); e != nil {
		return Page{}, e
	}
	limit = capped(limit)
	var at, cid any
	if c != nil {
		at, cid = c.At, c.ID
	}
	rows, e := r.DB.QueryContext(ctx, `SELECT `+jobColumns+` FROM jobs j JOIN users u ON u.id=j.customer_user_id LEFT JOIN companies c ON c.id=j.company_id LEFT JOIN categories cat ON cat.id=j.category_id
WHERE j.status='PUBLISHED' AND j.moderation_status='VISIBLE' AND j.deleted_at IS NULL AND u.status='ACTIVE' AND u.deleted_at IS NULL
AND($1='' OR j.search_vector@@websearch_to_tsquery('simple',$1) OR j.title ILIKE '%'||$1||'%')
AND($2='' OR cat.id::text=$2 OR cat.slug=$2) AND($3='' OR EXISTS(SELECT 1 FROM job_skills js JOIN skills sk ON sk.id=js.skill_id WHERE js.job_id=j.id AND(sk.id::text=$3 OR sk.slug=$3)))
AND($4='' OR j.employment_type=$4) AND($5::boolean IS NULL OR j.remote=$5) AND($6='' OR j.location_text ILIKE '%'||$6||'%') AND($7='' OR j.experience_level=$7)
AND($8::bigint IS NULL OR j.salary_max_kopecks >= $8) AND($10::timestamptz IS NULL OR(j.published_at,j.id)<($10,$11::uuid))
ORDER BY CASE WHEN $1<>'' AND $9='RELEVANCE' THEN ts_rank(j.search_vector,websearch_to_tsquery('simple',$1)) END DESC,j.published_at DESC,j.id DESC LIMIT $12`, f.Q, f.Category, f.Skill, f.EmploymentType, f.Remote, f.Location, f.Experience, f.MinSalary, f.Sort, at, cid, limit+1)
	if e != nil {
		return Page{}, e
	}
	items, e := scanJobs(rows)
	if e != nil {
		return Page{}, e
	}
	if e = r.loadPageSkills(ctx, items); e != nil {
		return Page{}, e
	}
	for i := range items {
		items[i].CustomerID = ""
		if items[i].Company != nil {
			items[i].Company.OwnerID = ""
		}
	}
	return finishPage(items, limit, true), nil
}
func (r PostgresRepository) GetPublic(ctx context.Context, ref string) (Item, error) {
	condition := `j.slug=$1 AND NOT EXISTS(SELECT 1 FROM jobs d WHERE d.slug=j.slug AND d.id<>j.id AND d.status='PUBLISHED' AND d.moderation_status='VISIBLE' AND d.deleted_at IS NULL)`
	if uuidPattern.MatchString(ref) {
		condition = `j.id=$1::uuid`
	}
	v, e := r.get(ctx, condition+` AND j.status='PUBLISHED' AND j.moderation_status='VISIBLE' AND j.deleted_at IS NULL AND u.status='ACTIVE' AND u.deleted_at IS NULL`, ref)
	if e != nil {
		return Item{}, e
	}
	if e = r.loadSkills(ctx, &v); e != nil {
		return Item{}, e
	}
	v.CustomerID = ""
	if v.Company != nil {
		v.Company.OwnerID = ""
	}
	return v, nil
}

func (r PostgresRepository) Apply(ctx context.Context, actor, jobID, message string) (Application, error) {
	message = strings.TrimSpace(message)
	if len([]rune(message)) > 5000 {
		return Application{}, ErrInvalidInput
	}
	id, e := newUUID()
	if e != nil {
		return Application{}, e
	}
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return Application{}, e
	}
	defer tx.Rollback()
	var allowed bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM jobs j JOIN users u ON u.id=$1 AND u.status='ACTIVE' AND u.deleted_at IS NULL JOIN user_capabilities uc ON uc.user_id=u.id AND uc.capability='FREELANCER' WHERE j.id=$2 AND j.status='PUBLISHED' AND j.moderation_status='VISIBLE' AND j.deleted_at IS NULL AND j.customer_user_id<>u.id)`, actor, jobID).Scan(&allowed)
	if e != nil {
		return Application{}, e
	}
	if !allowed {
		return Application{}, ErrIneligible
	}
	var a Application
	e = tx.QueryRowContext(ctx, `INSERT INTO job_applications(id,job_id,user_id,cover_message)VALUES($1,$2,$3,NULLIF($4,'')) RETURNING id::text,job_id::text,user_id::text,COALESCE(cover_message,''),status,created_at,updated_at`, id, jobID, actor, message).Scan(&a.ID, &a.JobID, &a.UserID, &a.CoverMessage, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if e != nil {
		return Application{}, mapDB(e)
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload)VALUES(gen_random_uuid(),'JOB',$1::uuid,'VACANCY_APPLICATION_SUBMITTED',jsonb_build_object('job_id',$1::uuid,'application_id',$2::uuid))`, jobID, id)
	if e != nil {
		return Application{}, e
	}
	if e = tx.Commit(); e != nil {
		return Application{}, e
	}
	a.UserID = ""
	return a, nil
}
func (r PostgresRepository) ListMine(ctx context.Context, actor string, _ *Cursor, limit int) ([]Application, error) {
	rows, e := r.DB.QueryContext(ctx, `SELECT a.id::text,a.job_id::text,j.title,''::text,''::text,COALESCE(a.cover_message,''),a.status,a.created_at,a.updated_at FROM job_applications a JOIN jobs j ON j.id=a.job_id WHERE a.user_id=$1 ORDER BY a.created_at DESC,a.id DESC LIMIT $2`, actor, capped(limit))
	if e != nil {
		return nil, e
	}
	return scanApplications(rows)
}
func (r PostgresRepository) ListApplicants(ctx context.Context, actor, jobID string) ([]Application, error) {
	rows, e := r.DB.QueryContext(ctx, `SELECT a.id::text,a.job_id::text,j.title,a.user_id::text,u.display_name,COALESCE(a.cover_message,''),a.status,a.created_at,a.updated_at FROM job_applications a JOIN jobs j ON j.id=a.job_id JOIN users u ON u.id=a.user_id WHERE a.job_id=$1 AND j.customer_user_id=$2 AND j.deleted_at IS NULL ORDER BY a.created_at DESC,a.id DESC`, jobID, actor)
	if e != nil {
		return nil, e
	}
	out, e := scanApplications(rows)
	if e == nil && len(out) == 0 {
		var exists bool
		if er := r.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE id=$1 AND customer_user_id=$2 AND deleted_at IS NULL)`, jobID, actor).Scan(&exists); er != nil {
			return nil, er
		} else if !exists {
			return nil, ErrNotFound
		}
	}
	return out, e
}
func (r PostgresRepository) SetApplicationStatus(ctx context.Context, actor, jobID, appID, status string) (Application, error) {
	status = strings.ToUpper(strings.TrimSpace(status))
	if !oneOf(status, "VIEWED", "SHORTLISTED", "REJECTED", "ACCEPTED") {
		return Application{}, ErrInvalidInput
	}
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return Application{}, e
	}
	defer tx.Rollback()
	var a Application
	e = tx.QueryRowContext(ctx, `UPDATE job_applications a SET status=$4,updated_at=now() FROM jobs j WHERE a.id=$3 AND a.job_id=$1 AND j.id=a.job_id AND j.customer_user_id=$2 AND (a.status='SUBMITTED' OR (a.status='VIEWED' AND $4<>'VIEWED') OR (a.status='SHORTLISTED' AND $4 IN('REJECTED','ACCEPTED'))) RETURNING a.id::text,a.job_id::text,a.user_id::text,COALESCE(a.cover_message,''),a.status,a.created_at,a.updated_at`, jobID, actor, appID, status).Scan(&a.ID, &a.JobID, &a.UserID, &a.CoverMessage, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(e, sql.ErrNoRows) {
		return Application{}, ErrNotFound
	}
	if e != nil {
		return Application{}, e
	}
	if status == "SHORTLISTED" {
		_, e = tx.ExecContext(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload)VALUES(gen_random_uuid(),'JOB',$1::uuid,'VACANCY_APPLICATION_SHORTLISTED',jsonb_build_object('application_id',$2::uuid))`, jobID, appID)
		if e != nil {
			return Application{}, e
		}
	}
	if e = tx.Commit(); e != nil {
		return Application{}, e
	}
	return a, nil
}
func (r PostgresRepository) Moderate(ctx context.Context, actor, id, action, reason string) (Item, error) {
	action = strings.ToUpper(strings.TrimSpace(action))
	reason = strings.TrimSpace(reason)
	if !oneOf(action, "HIDE", "RESTORE") || len([]rune(reason)) < 3 || len([]rune(reason)) > 1000 {
		return Item{}, ErrInvalidInput
	}
	target := "VISIBLE"
	if action == "HIDE" {
		target = "HIDDEN"
	}
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return Item{}, e
	}
	defer tx.Rollback()
	var allowed bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN user_roles ur ON ur.user_id=u.id WHERE u.id=$1 AND u.status='ACTIVE' AND u.deleted_at IS NULL AND ur.role IN('MODERATOR','ADMIN','SUPER_ADMIN'))`, actor).Scan(&allowed)
	if e != nil {
		return Item{}, e
	}
	if !allowed {
		return Item{}, ErrForbidden
	}
	res, e := tx.ExecContext(ctx, `UPDATE jobs SET moderation_status=$2,updated_at=now() WHERE id=$1 AND deleted_at IS NULL`, id, target)
	if e != nil {
		return Item{}, e
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return Item{}, ErrNotFound
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO audit_logs(id,actor_user_id,action,target_type,target_id,metadata,ip)VALUES(gen_random_uuid(),$1,$2,'JOB',$3,jsonb_build_object('reason',$4::text),NULLIF($5,'')::inet)`, actor, "JOB_"+action, id, reason, requestmeta.FromContext(ctx))
	if e != nil {
		return Item{}, e
	}
	if e = tx.Commit(); e != nil {
		return Item{}, e
	}
	return r.get(ctx, `j.id=$1`, id)
}

func (r PostgresRepository) get(ctx context.Context, where string, args ...any) (Item, error) {
	return scanJob(r.DB.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM jobs j JOIN users u ON u.id=j.customer_user_id LEFT JOIN companies c ON c.id=j.company_id LEFT JOIN categories cat ON cat.id=j.category_id WHERE `+where, args...))
}
func scanJob(row interface{ Scan(...any) error }) (Item, error) {
	var v Item
	var companyID, companyOwner, companyName, companySlug, logo, website, companyDescription, verification sql.NullString
	var companyCreated, companyUpdated sql.NullTime
	var categoryID, categorySlug, categoryName sql.NullString
	e := row.Scan(&v.ID, &v.CustomerID, &v.CustomerName, &v.Title, &v.Slug, &v.Description, &v.EmploymentType, &v.SalaryMinKopecks, &v.SalaryMaxKopecks, &v.Currency, &v.Location, &v.Remote, &v.ExperienceLevel, &v.Status, &v.ModerationStatus, &v.ModerationReason, &v.PublishedAt, &v.CreatedAt, &v.UpdatedAt, &companyID, &companyOwner, &companyName, &companySlug, &logo, &website, &companyDescription, &verification, &companyCreated, &companyUpdated, &categoryID, &categorySlug, &categoryName)
	if errors.Is(e, sql.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if e != nil {
		return Item{}, e
	}
	v.Skills = []Reference{}
	if companyID.Valid {
		v.Company = &Company{ID: companyID.String, OwnerID: companyOwner.String, Name: companyName.String, Slug: companySlug.String, LogoObjectKey: logo.String, Website: website.String, Description: companyDescription.String, VerificationStatus: verification.String, CreatedAt: companyCreated.Time, UpdatedAt: companyUpdated.Time}
	}
	if categoryID.Valid {
		v.Category = &Reference{ID: categoryID.String, Slug: categorySlug.String, Name: categoryName.String}
	}
	return v, nil
}
func scanJobs(rows *sql.Rows) ([]Item, error) {
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		v, e := scanJob(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r PostgresRepository) loadSkills(ctx context.Context, v *Item) error {
	rows, e := r.DB.QueryContext(ctx, `SELECT sk.id::text,sk.slug,sk.name FROM job_skills js JOIN skills sk ON sk.id=js.skill_id AND sk.is_active=true WHERE js.job_id=$1 ORDER BY sk.name`, v.ID)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var s Reference
		if e = rows.Scan(&s.ID, &s.Slug, &s.Name); e != nil {
			return e
		}
		v.Skills = append(v.Skills, s)
	}
	return rows.Err()
}
func (r PostgresRepository) loadPageSkills(ctx context.Context, items []Item) error {
	for i := range items {
		if e := r.loadSkills(ctx, &items[i]); e != nil {
			return e
		}
	}
	return nil
}
func finishPage(items []Item, limit int, published bool) Page {
	p := Page{Items: items}
	if len(items) > limit {
		last := items[limit-1]
		at := last.CreatedAt
		if published {
			at = *last.PublishedAt
		}
		p.Items = items[:limit]
		p.NextCursor = &Cursor{At: at, ID: last.ID}
	}
	return p
}
func validateRefs(ctx context.Context, q dbtx, actor string, v *Item) error {
	if v.Company != nil && v.Company.ID != "" {
		var c Company
		e := q.QueryRowContext(ctx, `SELECT id::text,owner_user_id::text,name,slug,COALESCE(logo_object_key,''),COALESCE(website,''),COALESCE(description,''),verification_status,created_at,updated_at FROM companies WHERE id=$1 AND owner_user_id=$2`, v.Company.ID, actor).Scan(&c.ID, &c.OwnerID, &c.Name, &c.Slug, &c.LogoObjectKey, &c.Website, &c.Description, &c.VerificationStatus, &c.CreatedAt, &c.UpdatedAt)
		if errors.Is(e, sql.ErrNoRows) {
			return ErrInvalidInput
		}
		if e != nil {
			return e
		}
		v.Company = &c
	}
	if v.Category != nil {
		var c Reference
		e := q.QueryRowContext(ctx, `SELECT id::text,slug,name FROM categories WHERE id=$1 AND is_active=true`, v.Category.ID).Scan(&c.ID, &c.Slug, &c.Name)
		if errors.Is(e, sql.ErrNoRows) {
			return ErrInvalidInput
		}
		if e != nil {
			return e
		}
		v.Category = &c
	}
	seen := map[string]bool{}
	for i, s := range v.Skills {
		if !uuidPattern.MatchString(s.ID) || seen[s.ID] {
			return ErrInvalidInput
		}
		seen[s.ID] = true
		var x Reference
		e := q.QueryRowContext(ctx, `SELECT id::text,slug,name FROM skills WHERE id=$1 AND is_active=true`, s.ID).Scan(&x.ID, &x.Slug, &x.Name)
		if errors.Is(e, sql.ErrNoRows) {
			return ErrInvalidInput
		}
		if e != nil {
			return e
		}
		v.Skills[i] = x
	}
	return nil
}
func replaceSkills(ctx context.Context, tx *sql.Tx, id string, skills []Reference) error {
	if _, e := tx.ExecContext(ctx, `DELETE FROM job_skills WHERE job_id=$1`, id); e != nil {
		return e
	}
	for _, s := range skills {
		if _, e := tx.ExecContext(ctx, `INSERT INTO job_skills(job_id,skill_id)VALUES($1,$2)`, id, s.ID); e != nil {
			return mapDB(e)
		}
	}
	return nil
}
func requireCustomer(ctx context.Context, q dbtx, actor string) error {
	var ok bool
	e := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN user_capabilities uc ON uc.user_id=u.id AND uc.capability='CUSTOMER' WHERE u.id=$1 AND u.status='ACTIVE' AND u.deleted_at IS NULL)`, actor).Scan(&ok)
	if e != nil {
		return e
	}
	if !ok {
		return ErrIneligible
	}
	return nil
}
func scanApplications(rows *sql.Rows) ([]Application, error) {
	defer rows.Close()
	out := []Application{}
	for rows.Next() {
		var a Application
		if e := rows.Scan(&a.ID, &a.JobID, &a.JobTitle, &a.UserID, &a.ApplicantName, &a.CoverMessage, &a.Status, &a.CreatedAt, &a.UpdatedAt); e != nil {
			return nil, e
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func nullableCompany(v *Company) any {
	if v == nil || v.ID == "" {
		return nil
	}
	return v.ID
}
func nullableRef(v *Reference) any {
	if v == nil || v.ID == "" {
		return nil
	}
	return v.ID
}
func mapDB(e error) error {
	var p *pgconn.PgError
	if errors.As(e, &p) {
		switch p.Code {
		case "23505":
			return ErrConflict
		case "23503", "23514", "22001":
			return fmt.Errorf("%w: invalid reference or value", ErrInvalidInput)
		}
	}
	return e
}
