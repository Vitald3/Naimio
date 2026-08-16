package acquisition

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"freelance/apps/api/internal/ai"
	"freelance/apps/api/internal/platform/requestmeta"
)

type PostgresRepository struct{ DB *sql.DB }

func (r PostgresRepository) AdminDefinitions(ctx context.Context) ([]AdminDefinition, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT DISTINCT ON(slug) slug,title,version,enabled,schema,pricing_config FROM calculator_definitions ORDER BY slug,version DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []AdminDefinition{}
	for rows.Next() {
		var item AdminDefinition
		var schema, pricing []byte
		if err = rows.Scan(&item.Slug, &item.Title, &item.Version, &item.Enabled, &schema, &pricing); err != nil {
			return nil, err
		}
		var public struct {
			Intro     string     `json:"intro"`
			Questions []Question `json:"questions"`
		}
		if json.Unmarshal(schema, &public) != nil || json.Unmarshal(pricing, &item.Pricing) != nil {
			return nil, ErrInvalid
		}
		item.Intro, item.Questions = public.Intro, public.Questions
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r PostgresRepository) CreateAdminDefinition(ctx context.Context, actor string, in AdminDefinitionInput) (AdminDefinition, error) {
	return r.writeAdminDefinition(ctx, actor, "", in)
}
func (r PostgresRepository) UpdateAdminDefinition(ctx context.Context, actor, slug string, in AdminDefinitionInput) (AdminDefinition, error) {
	return r.writeAdminDefinition(ctx, actor, slug, in)
}
func (r PostgresRepository) writeAdminDefinition(ctx context.Context, actor, existingSlug string, in AdminDefinitionInput) (AdminDefinition, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return AdminDefinition{}, err
	}
	defer tx.Rollback()
	version := 1
	if existingSlug != "" {
		err = tx.QueryRowContext(ctx, `SELECT version FROM calculator_definitions WHERE slug=$1 ORDER BY version DESC LIMIT 1 FOR UPDATE`, existingSlug).Scan(&version)
		if errors.Is(err, sql.ErrNoRows) {
			return AdminDefinition{}, ErrNotFound
		}
		if err != nil {
			return AdminDefinition{}, err
		}
		version++
	} else {
		var exists bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM calculator_definitions WHERE slug=$1)`, in.Slug).Scan(&exists); err != nil {
			return AdminDefinition{}, err
		}
		if exists {
			return AdminDefinition{}, ErrInvalid
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE calculator_definitions SET enabled=false,updated_at=now() WHERE slug=$1 AND enabled=true`, in.Slug); err != nil {
		return AdminDefinition{}, err
	}
	schema, _ := json.Marshal(map[string]any{"intro": in.Intro, "questions": in.Questions})
	pricing, _ := json.Marshal(in.Pricing)
	var id string
	err = tx.QueryRowContext(ctx, `INSERT INTO calculator_definitions(id,slug,title,category_id,version,schema,pricing_config,enabled) VALUES(gen_random_uuid(),$1,$2,(SELECT id FROM categories WHERE slug=NULLIF($3,'')), $4,$5::jsonb,$6::jsonb,$7) RETURNING id::text`, in.Slug, in.Title, in.Pricing.CategorySlug, version, schema, pricing, in.Enabled).Scan(&id)
	if err != nil {
		return AdminDefinition{}, err
	}
	action := "CALCULATOR_CREATED"
	if existingSlug != "" {
		action = "CALCULATOR_VERSION_CREATED"
	}
	metadata, _ := json.Marshal(map[string]any{"slug": in.Slug, "version": version, "reason": in.Reason, "enabled": in.Enabled})
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_logs(id,actor_user_id,action,target_type,target_id,metadata,ip) VALUES(gen_random_uuid(),$1,$2,'CALCULATOR',$3::uuid,$4::jsonb,NULLIF($5,'')::inet)`, actor, action, id, metadata, requestmeta.FromContext(ctx)); err != nil {
		return AdminDefinition{}, err
	}
	if err = tx.Commit(); err != nil {
		return AdminDefinition{}, err
	}
	return AdminDefinition{Slug: in.Slug, Title: in.Title, Intro: in.Intro, Version: version, Enabled: in.Enabled, Questions: in.Questions, Pricing: in.Pricing}, nil
}

func (r PostgresRepository) Definitions(ctx context.Context) ([]Definition, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT id::text,slug,title,version,schema,updated_at FROM calculator_definitions WHERE enabled=true ORDER BY title,slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Definition{}
	for rows.Next() {
		var item Definition
		var schema []byte
		if err = rows.Scan(&item.ID, &item.Slug, &item.Title, &item.Version, &schema, &item.UpdatedAt); err != nil {
			return nil, err
		}
		var public struct {
			Intro     string     `json:"intro"`
			Questions []Question `json:"questions"`
		}
		if json.Unmarshal(schema, &public) != nil {
			return nil, ErrInvalid
		}
		item.Intro, item.Questions = public.Intro, public.Questions
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r PostgresRepository) Definition(ctx context.Context, slug string) (Definition, error) {
	var d Definition
	var schema, pricing []byte
	var category sql.NullString
	e := r.DB.QueryRowContext(ctx, `SELECT id::text,slug,title,category_id::text,version,schema,pricing_config,updated_at FROM calculator_definitions WHERE slug=$1 AND enabled=true`, slug).Scan(&d.ID, &d.Slug, &d.Title, &category, &d.Version, &schema, &pricing, &d.UpdatedAt)
	if errors.Is(e, sql.ErrNoRows) {
		return Definition{}, ErrNotFound
	}
	if e != nil {
		return Definition{}, e
	}
	var public struct {
		Intro     string     `json:"intro"`
		Questions []Question `json:"questions"`
	}
	if json.Unmarshal(schema, &public) != nil || json.Unmarshal(pricing, &d.Pricing) != nil {
		return Definition{}, ErrInvalid
	}
	d.Intro, d.Questions = public.Intro, public.Questions
	if e = validateDefinition(d); e != nil {
		return Definition{}, e
	}
	return d, nil
}
func (r PostgresRepository) Resolve(ctx context.Context, categorySlug string, skillSlugs []string) (Taxonomy, error) {
	var out Taxonomy
	if categorySlug != "" {
		var c ai.CategoryCandidate
		e := r.DB.QueryRowContext(ctx, `SELECT id::text,slug,name FROM categories WHERE slug=$1 AND is_active=true`, categorySlug).Scan(&c.ID, &c.Slug, &c.Name)
		if e == nil {
			c.Confidence = 1
			out.Category = &c
		} else if !errors.Is(e, sql.ErrNoRows) {
			return Taxonomy{}, e
		}
	}
	out.Skills = []ai.SkillCandidate{}
	if len(skillSlugs) > 0 {
		rows, e := r.DB.QueryContext(ctx, `SELECT id::text,slug,name FROM skills WHERE slug=ANY($1::text[]) AND is_active=true ORDER BY name LIMIT 30`, skillSlugs)
		if e != nil {
			return Taxonomy{}, e
		}
		defer rows.Close()
		for rows.Next() {
			var s ai.SkillCandidate
			if e = rows.Scan(&s.ID, &s.Slug, &s.Name); e != nil {
				return Taxonomy{}, e
			}
			s.Confidence = 1
			out.Skills = append(out.Skills, s)
		}
		if e = rows.Err(); e != nil {
			return Taxonomy{}, e
		}
	}
	return out, nil
}
func (r PostgresRepository) Record(ctx context.Context, e Event) error {
	id, err := newUUID()
	if err != nil {
		return err
	}
	metadata, _ := json.Marshal(e.Metadata)
	var anonymous, user any
	if e.AnonymousID != "" {
		anonymous = e.AnonymousID
	}
	if e.UserID != "" {
		user = e.UserID
	}
	_, err = r.DB.ExecContext(ctx, `INSERT INTO acquisition_events(id,anonymous_id,user_id,event_type,landing_path,utm_source,utm_medium,utm_campaign,utm_content,referral_code,metadata)VALUES($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),$11)`, id, anonymous, user, e.Type, e.LandingPath, e.UTMSource, e.UTMMedium, e.UTMCampaign, e.UTMContent, e.ReferralCode, metadata)
	return err
}
func (r PostgresRepository) Sitemap(ctx context.Context) ([]SitemapItem, error) {
	rows, e := r.DB.QueryContext(ctx, `SELECT path,updated_at FROM(
SELECT '/categories/'||c.slug path,c.updated_at FROM categories c WHERE c.is_active=true AND(
EXISTS(SELECT 1 FROM categories child WHERE child.parent_id=c.id AND child.is_active=true)
OR EXISTS(SELECT 1 FROM profile_categories pc JOIN professional_profiles pp ON pp.user_id=pc.user_id JOIN users pu ON pu.id=pp.user_id WHERE pc.category_id=c.id AND pp.profile_visibility='PUBLIC' AND pu.status='ACTIVE' AND pu.deleted_at IS NULL)
OR EXISTS(SELECT 1 FROM services cs WHERE cs.category_id=c.id AND cs.status='ACTIVE' AND cs.moderation_status='VISIBLE' AND cs.visibility='PUBLIC' AND cs.deleted_at IS NULL)
OR EXISTS(SELECT 1 FROM projects cp WHERE cp.category_id=c.id AND cp.status IN('OPEN','MATCHING') AND cp.visibility='PUBLIC' AND cp.deleted_at IS NULL))
UNION ALL SELECT '/freelancers/'||u.username_normalized,p.updated_at FROM professional_profiles p JOIN users u ON u.id=p.user_id WHERE p.profile_visibility='PUBLIC' AND u.status='ACTIVE' AND u.deleted_at IS NULL AND u.username_normalized IS NOT NULL AND(char_length(COALESCE(p.professional_title,''))>=3 OR char_length(COALESCE(p.bio,''))>=80)
UNION ALL SELECT '/services/'||s.id::text,s.updated_at FROM services s JOIN users u ON u.id=s.seller_user_id WHERE s.status='ACTIVE' AND s.moderation_status='VISIBLE' AND s.visibility='PUBLIC' AND s.deleted_at IS NULL AND u.status='ACTIVE' AND u.deleted_at IS NULL
UNION ALL SELECT '/projects/'||p.id::text,p.updated_at FROM projects p JOIN users u ON u.id=p.customer_user_id WHERE p.status IN('OPEN','MATCHING') AND p.visibility='PUBLIC' AND p.deleted_at IS NULL AND char_length(p.description)>=80 AND u.status='ACTIVE' AND u.deleted_at IS NULL
UNION ALL SELECT '/vacancies/'||j.id::text,j.updated_at FROM jobs j JOIN users u ON u.id=j.customer_user_id WHERE j.status='PUBLISHED' AND j.moderation_status='VISIBLE' AND j.deleted_at IS NULL AND u.status='ACTIVE' AND u.deleted_at IS NULL
UNION ALL SELECT '/price/'||slug,updated_at FROM calculator_definitions WHERE enabled=true
)x ORDER BY path LIMIT 10000`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []SitemapItem{}
	for rows.Next() {
		var v SitemapItem
		if e = rows.Scan(&v.Path, &v.UpdatedAt); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func newUUID() (string, error) {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", e
	}
	b[6] = (b[6] & 0x0f) | 0x70
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[:8], h[8:12], h[12:16], h[16:20], h[20:]), nil
}
