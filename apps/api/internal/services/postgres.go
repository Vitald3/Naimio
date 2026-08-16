package services

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

const serviceColumns = `
s.id::text, s.seller_user_id::text, COALESCE(u.username, ''), u.display_name, uts.native_rating::float8, COALESCE(uts.reviews_count,0),
c.id::text, c.slug, c.name, s.service_type, s.title, s.slug, COALESCE(s.short_description, ''), s.description,
s.price_type, s.price_from_kopecks, s.currency, s.delivery_days, s.included_revisions,
s.status, s.moderation_status, COALESCE(s.moderation_reason, ''), s.visibility, s.published_at, s.created_at, s.updated_at`

func (r PostgresRepository) Create(ctx context.Context, actorID string, input CreateRequest) (Item, error) {
	if contentmoderation.LooksLikeJunk(input.Title, input.ShortDescription, input.Description) {
		return Item{}, invalid("Материал не прошёл автоматическую проверку. Уберите бессмысленный, повторяющийся или подозрительный текст и попробуйте снова.")
	}
	if actorID == "" {
		return Item{}, ErrUnauthorized
	}
	id, err := newUUIDv7()
	if err != nil {
		return Item{}, err
	}
	item := itemFromCreate(actorID, id, input, time.Now().UTC())
	if err := Validate(item); err != nil {
		return Item{}, err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Item{}, err
	}
	defer tx.Rollback()
	if err := requireSeller(ctx, tx, actorID, false); err != nil {
		return Item{}, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO services
  (id, seller_user_id, category_id, service_type, title, slug, short_description, description, price_type,
   price_from_kopecks, currency, delivery_days, included_revisions, visibility, status)
SELECT $1, $2, c.id, $4, $5, $6, NULLIF($7, ''), $8, $9, $10, 'RUB', $11, $12, $13, 'DRAFT'
FROM categories c WHERE c.id = $3 AND c.is_active = true`, id, actorID, item.Category.ID, item.ServiceType,
		item.Title, item.Slug, item.ShortDescription, item.Description, item.PriceType, amount(item.PriceFrom),
		item.DeliveryDays, item.IncludedRevisions, item.Visibility)
	if err != nil {
		return Item{}, mapPostgresError(err)
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return Item{}, fmt.Errorf("%w: category is not active", ErrInvalidReference)
	}
	if err := replaceRelations(ctx, tx, actorID, id, item); err != nil {
		return Item{}, err
	}
	if err := tx.Commit(); err != nil {
		return Item{}, mapPostgresError(err)
	}
	return r.GetOwned(ctx, actorID, id)
}

func (r PostgresRepository) GetOwned(ctx context.Context, actorID, serviceID string) (Item, error) {
	item, err := scanItem(r.DB.QueryRowContext(ctx, `SELECT `+serviceColumns+`
FROM services s JOIN users u ON u.id = s.seller_user_id LEFT JOIN user_trust_stats uts ON uts.user_id = u.id JOIN categories c ON c.id = s.category_id
WHERE s.id = $1 AND s.seller_user_id = $2 AND s.deleted_at IS NULL`, serviceID, actorID))
	if err != nil {
		return Item{}, err
	}
	if err := loadRelations(ctx, r.DB, &item, false); err != nil {
		return Item{}, err
	}
	return item, nil
}

func (r PostgresRepository) ListOwned(ctx context.Context, actorID string, cursor *Cursor, limit int) (Page, error) {
	limit = boundedLimit(limit)
	var cursorAt, cursorID any
	if cursor != nil {
		cursorAt, cursorID = cursor.At, cursor.ID
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT `+serviceColumns+`
FROM services s JOIN users u ON u.id = s.seller_user_id LEFT JOIN user_trust_stats uts ON uts.user_id = u.id JOIN categories c ON c.id = s.category_id
WHERE s.seller_user_id = $1 AND s.deleted_at IS NULL
  AND ($2::timestamptz IS NULL OR (s.created_at, s.id) < ($2, $3::uuid))
ORDER BY s.created_at DESC, s.id DESC LIMIT $4`, actorID, cursorAt, cursorID, limit+1)
	if err != nil {
		return Page{}, err
	}
	items, err := collectRows(rows)
	if err != nil {
		return Page{}, err
	}
	return r.finishPage(ctx, items, limit, false, func(item Item) time.Time { return item.CreatedAt })
}

func (r PostgresRepository) Update(ctx context.Context, actorID, serviceID string, patch PatchRequest) (Item, error) {
	existing, err := r.GetOwned(ctx, actorID, serviceID)
	if err != nil {
		return Item{}, err
	}
	if existing.Status != "DRAFT" && existing.Status != "PAUSED" && existing.Status != "REJECTED" {
		return Item{}, ErrInvalidState
	}
	item := mergePatch(existing, patch, time.Now().UTC())
	if err := Validate(item); err != nil {
		return Item{}, err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Item{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE services s SET category_id = c.id, service_type = $4, title = $5,
slug = $6, short_description = NULLIF($7, ''), description = $8, price_type = $9, price_from_kopecks = $10,
currency = 'RUB', delivery_days = $11, included_revisions = $12, visibility = $13, updated_at = now()
FROM categories c WHERE s.id = $1 AND s.seller_user_id = $2 AND s.deleted_at IS NULL
  AND s.status IN ('DRAFT','PAUSED','REJECTED') AND c.id = $3 AND c.is_active = true`, serviceID, actorID,
		item.Category.ID, item.ServiceType, item.Title, item.Slug, item.ShortDescription, item.Description,
		item.PriceType, amount(item.PriceFrom), item.DeliveryDays, item.IncludedRevisions, item.Visibility)
	if err != nil {
		return Item{}, mapPostgresError(err)
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return Item{}, ErrInvalidState
	}
	if err := replaceRelations(ctx, tx, actorID, serviceID, item); err != nil {
		return Item{}, err
	}
	if err := tx.Commit(); err != nil {
		return Item{}, mapPostgresError(err)
	}
	return r.GetOwned(ctx, actorID, serviceID)
}

func (r PostgresRepository) Delete(ctx context.Context, actorID, serviceID string) error {
	result, err := r.DB.ExecContext(ctx, `UPDATE services SET status = 'ARCHIVED', deleted_at = now(), updated_at = now()
WHERE id = $1 AND seller_user_id = $2 AND deleted_at IS NULL`, serviceID, actorID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrNotFound
	}
	return nil
}

func (r PostgresRepository) Transition(ctx context.Context, actorID, serviceID, action string) (Item, error) {
	current, err := r.GetOwned(ctx, actorID, serviceID)
	if err != nil {
		return Item{}, err
	}
	target, allowed := transition(current.Status, action)
	if !allowed {
		return Item{}, ErrInvalidState
	}
	if action == "publish" || action == "resume" {
		if contentmoderation.LooksLikeJunk(current.Title, current.ShortDescription, current.Description) {
			return Item{}, invalid("Материал не прошёл автоматическую проверку. Уберите бессмысленный, повторяющийся или подозрительный текст и попробуйте снова.")
		}
		if err := requireSeller(ctx, r.DB, actorID, true); err != nil {
			return Item{}, err
		}
		var invalidReferences bool
		err := r.DB.QueryRowContext(ctx, `SELECT
NOT EXISTS (SELECT 1 FROM categories c WHERE c.id = s.category_id AND c.is_active = true)
OR EXISTS (SELECT 1 FROM service_skills ss LEFT JOIN skills sk ON sk.id = ss.skill_id AND sk.is_active = true WHERE ss.service_id = s.id AND sk.id IS NULL)
OR EXISTS (SELECT 1 FROM service_media sm LEFT JOIN media_objects mo ON mo.id = sm.media_object_id
  AND mo.owner_user_id = s.seller_user_id AND mo.purpose = 'SERVICE' AND mo.uploaded_at IS NOT NULL
  AND mo.scan_status = 'CLEAN' AND mo.deleted_at IS NULL WHERE sm.service_id = s.id AND mo.id IS NULL)
FROM services s WHERE s.id = $1 AND s.seller_user_id = $2 AND s.deleted_at IS NULL`, serviceID, actorID).Scan(&invalidReferences)
		if err != nil {
			return Item{}, err
		}
		if invalidReferences {
			return Item{}, fmt.Errorf("%w: service references are not publishable", ErrInvalidReference)
		}
	}
	var result sql.Result
	if action == "publish" {
		result, err = r.DB.ExecContext(ctx, `WITH published AS (
UPDATE services SET status = 'ACTIVE', moderation_status='VISIBLE', moderation_reason=NULL, moderated_by=NULL, moderated_at=NULL, published_at = COALESCE(published_at, now()), updated_at = now()
WHERE id = $1 AND seller_user_id = $2 AND status IN ('DRAFT','REJECTED') AND deleted_at IS NULL RETURNING id
) INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload)
SELECT gen_random_uuid(),'SERVICE',id,'SERVICE_PUBLISHED',jsonb_build_object('service_id',id) FROM published`, serviceID, actorID)
	} else {
		result, err = r.DB.ExecContext(ctx, `UPDATE services SET status = $3, updated_at = now()
WHERE id = $1 AND seller_user_id = $2 AND status = $4 AND deleted_at IS NULL`, serviceID, actorID, target, current.Status)
	}
	if err != nil {
		return Item{}, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return Item{}, ErrInvalidState
	}
	return r.GetOwned(ctx, actorID, serviceID)
}

func (r PostgresRepository) ListPublic(ctx context.Context, filter Filter, cursor *Cursor, limit int) (Page, error) {
	if err := ValidateFilter(filter); err != nil {
		return Page{}, err
	}
	limit = boundedLimit(limit)
	var cursorAt, cursorID, maxDuration any
	if cursor != nil {
		cursorAt, cursorID = cursor.At, cursor.ID
	}
	if filter.MaxDurationMinutes != nil {
		maxDuration = *filter.MaxDurationMinutes
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT `+serviceColumns+`
FROM services s JOIN users u ON u.id = s.seller_user_id LEFT JOIN user_trust_stats uts ON uts.user_id = u.id JOIN categories c ON c.id = s.category_id
JOIN professional_profiles p ON p.user_id = s.seller_user_id AND p.profile_visibility = 'PUBLIC'
WHERE s.status = 'ACTIVE' AND s.moderation_status = 'VISIBLE' AND s.visibility = 'PUBLIC' AND s.deleted_at IS NULL
  AND u.status = 'ACTIVE' AND u.deleted_at IS NULL AND c.is_active = true
  AND ($1 = '' OR s.search_vector @@ websearch_to_tsquery('simple',$1) OR s.title ILIKE '%'||$1||'%')
  AND ($2 = '' OR c.id::text = $2 OR c.slug = $2)
  AND ($3 = '' OR s.service_type = $3) AND ($4 = '' OR s.price_type = $4)
  AND ($5 = '' OR EXISTS(SELECT 1 FROM education_service_details ed WHERE ed.service_id=s.id AND ed.format=$5))
  AND ($6 = '' OR EXISTS(SELECT 1 FROM education_service_details ed WHERE ed.service_id=s.id AND (ed.audience_type=$6 OR ed.audience_type='BOTH')))
  AND ($7::int IS NULL OR NOT EXISTS(SELECT 1 FROM education_service_details ed WHERE ed.service_id=s.id AND ed.duration_minutes IS NOT NULL AND ed.duration_minutes>$7))
  AND ($8::timestamptz IS NULL OR (s.published_at, s.id) < ($8, $9::uuid))
ORDER BY s.published_at DESC, s.id DESC LIMIT $10`, filter.Q, filter.Category, filter.ServiceType, filter.PriceType, filter.Format, filter.Audience, maxDuration, cursorAt, cursorID, limit+1)
	if err != nil {
		return Page{}, err
	}
	items, err := collectRows(rows)
	if err != nil {
		return Page{}, err
	}
	return r.finishPage(ctx, items, limit, true, func(item Item) time.Time { return *item.PublishedAt })
}

func (r PostgresRepository) GetPublic(ctx context.Context, reference string) (Item, error) {
	condition := `s.slug = $1 AND NOT EXISTS (
    SELECT 1 FROM services duplicate WHERE duplicate.slug = s.slug AND duplicate.id <> s.id
      AND duplicate.status = 'ACTIVE' AND duplicate.moderation_status = 'VISIBLE' AND duplicate.visibility = 'PUBLIC' AND duplicate.deleted_at IS NULL)`
	if validUUID(reference) {
		condition = "s.id = $1::uuid"
	}
	item, err := scanItem(r.DB.QueryRowContext(ctx, `SELECT `+serviceColumns+`
FROM services s JOIN users u ON u.id = s.seller_user_id LEFT JOIN user_trust_stats uts ON uts.user_id = u.id JOIN categories c ON c.id = s.category_id
JOIN professional_profiles p ON p.user_id = s.seller_user_id AND p.profile_visibility = 'PUBLIC'
WHERE s.status = 'ACTIVE' AND s.moderation_status = 'VISIBLE' AND s.visibility = 'PUBLIC' AND s.deleted_at IS NULL
  AND u.status = 'ACTIVE' AND u.deleted_at IS NULL AND c.is_active = true
  AND (`+condition+`)`, reference))
	if err != nil {
		return Item{}, err
	}
	if err := loadRelations(ctx, r.DB, &item, true); err != nil {
		return Item{}, err
	}
	return item, nil
}

type dbtx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func requireSeller(ctx context.Context, database dbtx, actorID string, publish bool) error {
	var allowed bool
	query := `SELECT EXISTS (SELECT 1 FROM users u JOIN user_capabilities uc ON uc.user_id = u.id AND uc.capability = 'FREELANCER'
WHERE u.id = $1 AND u.status = 'ACTIVE' AND u.deleted_at IS NULL)`
	if publish {
		query = `SELECT EXISTS (SELECT 1 FROM users u JOIN user_capabilities uc ON uc.user_id = u.id AND uc.capability = 'FREELANCER'
JOIN professional_profiles p ON p.user_id = u.id AND p.profile_visibility = 'PUBLIC'
WHERE u.id = $1 AND u.status = 'ACTIVE' AND u.deleted_at IS NULL AND u.username IS NOT NULL)`
	}
	if err := database.QueryRowContext(ctx, query, actorID).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return ErrSellerIneligible
	}
	return nil
}

func (r PostgresRepository) Moderate(ctx context.Context, actorID, serviceID, action, reason string) (Item, error) {
	action, reason = strings.ToUpper(strings.TrimSpace(action)), strings.TrimSpace(reason)
	if !oneOf(action, "HIDE", "RESTORE") || len([]rune(reason)) < 3 || len([]rune(reason)) > 1000 {
		return Item{}, ErrInvalidInput
	}
	target := "VISIBLE"
	if action == "HIDE" {
		target = "HIDDEN"
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Item{}, err
	}
	defer tx.Rollback()
	var allowed bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN user_roles ur ON ur.user_id=u.id WHERE u.id=$1 AND u.status='ACTIVE' AND u.deleted_at IS NULL AND ur.role IN('MODERATOR','ADMIN','SUPER_ADMIN'))`, actorID).Scan(&allowed); err != nil {
		return Item{}, err
	}
	if !allowed {
		return Item{}, ErrUnauthorized
	}
	result, err := tx.ExecContext(ctx, `UPDATE services SET moderation_status=$2,updated_at=now() WHERE id=$1 AND deleted_at IS NULL`, serviceID, target)
	if err != nil {
		return Item{}, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return Item{}, ErrNotFound
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_logs(id,actor_user_id,action,target_type,target_id,metadata,ip)VALUES(gen_random_uuid(),$1,$2,'SERVICE',$3,jsonb_build_object('reason',$4),NULLIF($5,'')::inet)`, actorID, "SERVICE_"+action, serviceID, reason, requestmeta.FromContext(ctx)); err != nil {
		return Item{}, err
	}
	if err = tx.Commit(); err != nil {
		return Item{}, err
	}
	return scanItem(r.DB.QueryRowContext(ctx, `SELECT `+serviceColumns+` FROM services s JOIN users u ON u.id=s.seller_user_id LEFT JOIN user_trust_stats uts ON uts.user_id=u.id JOIN categories c ON c.id=s.category_id WHERE s.id=$1`, serviceID))
}

func replaceRelations(ctx context.Context, tx *sql.Tx, actorID, serviceID string, item Item) error {
	for _, statement := range []string{"DELETE FROM service_skills WHERE service_id = $1", "DELETE FROM service_media WHERE service_id = $1", "DELETE FROM education_service_details WHERE service_id = $1"} {
		if _, err := tx.ExecContext(ctx, statement, serviceID); err != nil {
			return err
		}
	}
	seen := map[string]struct{}{}
	for _, skill := range item.Skills {
		id := normalizeID(skill.ID)
		if !validUUID(id) {
			return invalid("invalid skill id")
		}
		if _, ok := seen[id]; ok {
			return invalid("duplicate skill id")
		}
		seen[id] = struct{}{}
		result, err := tx.ExecContext(ctx, `INSERT INTO service_skills (service_id, skill_id)
SELECT $1, id FROM skills WHERE id = $2 AND is_active = true`, serviceID, id)
		if err != nil {
			return mapPostgresError(err)
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return fmt.Errorf("%w: skill is not active", ErrInvalidReference)
		}
	}
	seen = map[string]struct{}{}
	for index, medium := range item.Media {
		id := normalizeID(medium.ID)
		if !validUUID(id) {
			return invalid("invalid media id")
		}
		if _, ok := seen[id]; ok {
			return invalid("duplicate media id")
		}
		seen[id] = struct{}{}
		result, err := tx.ExecContext(ctx, `INSERT INTO service_media (service_id, media_object_id, sort_order)
SELECT $1, id, $4 FROM media_objects WHERE id = $3 AND owner_user_id = $2 AND purpose = 'SERVICE'
  AND uploaded_at IS NOT NULL AND scan_status = 'CLEAN' AND deleted_at IS NULL`, serviceID, actorID, id, index)
		if err != nil {
			return mapPostgresError(err)
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return fmt.Errorf("%w: media is not attachable", ErrInvalidReference)
		}
	}
	if item.Education != nil {
		details := item.Education
		_, err := tx.ExecContext(ctx, `INSERT INTO education_service_details
(service_id, format, duration_minutes, sessions_count, audience_type, group_size_max) VALUES ($1,$2,$3,$4,NULLIF($5,''),$6)`,
			serviceID, details.Format, details.DurationMinutes, details.SessionsCount, details.AudienceType, details.GroupSizeMax)
		if err != nil {
			return mapPostgresError(err)
		}
	}
	return nil
}

func loadRelations(ctx context.Context, database dbtx, item *Item, public bool) error {
	skills, err := database.QueryContext(ctx, `SELECT sk.id::text, sk.slug, sk.name FROM service_skills ss
JOIN skills sk ON sk.id = ss.skill_id AND sk.is_active = true WHERE ss.service_id = $1 ORDER BY sk.name`, item.ID)
	if err != nil {
		return err
	}
	for skills.Next() {
		var value Reference
		if err := skills.Scan(&value.ID, &value.Slug, &value.Name); err != nil {
			skills.Close()
			return err
		}
		item.Skills = append(item.Skills, value)
	}
	if err := skills.Err(); err != nil {
		skills.Close()
		return err
	}
	if err := skills.Close(); err != nil {
		return err
	}
	mediaRows, err := database.QueryContext(ctx, `SELECT mo.id::text, mo.mime_type, mo.size_bytes, sm.sort_order FROM service_media sm
JOIN media_objects mo ON mo.id = sm.media_object_id AND mo.deleted_at IS NULL
WHERE sm.service_id = $1 AND ($2::boolean = false OR (mo.purpose = 'SERVICE' AND mo.uploaded_at IS NOT NULL AND mo.scan_status = 'CLEAN'))
ORDER BY sm.sort_order, mo.id`, item.ID, public)
	if err != nil {
		return err
	}
	for mediaRows.Next() {
		var value Media
		if err := mediaRows.Scan(&value.ID, &value.MIMEType, &value.SizeBytes, &value.SortOrder); err != nil {
			mediaRows.Close()
			return err
		}
		item.Media = append(item.Media, value)
	}
	if err := mediaRows.Err(); err != nil {
		mediaRows.Close()
		return err
	}
	if err := mediaRows.Close(); err != nil {
		return err
	}
	var details EducationDetails
	err = database.QueryRowContext(ctx, `SELECT format, duration_minutes, sessions_count, COALESCE(audience_type, ''), group_size_max
FROM education_service_details WHERE service_id = $1`, item.ID).Scan(&details.Format, &details.DurationMinutes, &details.SessionsCount, &details.AudienceType, &details.GroupSizeMax)
	if err == nil {
		item.Education = &details
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}

func (r PostgresRepository) finishPage(ctx context.Context, items []Item, limit int, public bool, timestamp func(Item) time.Time) (Page, error) {
	page := Page{Items: items}
	if len(items) > limit {
		last := items[limit-1]
		page.Items = items[:limit]
		page.NextCursor = &Cursor{At: timestamp(last), ID: last.ID}
	}
	if err := loadPageRelations(ctx, r.DB, page.Items, public); err != nil {
		return Page{}, err
	}
	return page, nil
}

func loadPageRelations(ctx context.Context, database dbtx, items []Item, public bool) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, len(items))
	indexes := make(map[string]int, len(items))
	for index := range items {
		ids[index], indexes[items[index].ID] = items[index].ID, index
	}
	skillRows, err := database.QueryContext(ctx, `SELECT ss.service_id::text, sk.id::text, sk.slug, sk.name
FROM service_skills ss JOIN skills sk ON sk.id = ss.skill_id AND sk.is_active = true
WHERE ss.service_id = ANY($1::uuid[]) ORDER BY ss.service_id, sk.name`, ids)
	if err != nil {
		return err
	}
	for skillRows.Next() {
		var serviceID string
		var value Reference
		if err := skillRows.Scan(&serviceID, &value.ID, &value.Slug, &value.Name); err != nil {
			skillRows.Close()
			return err
		}
		items[indexes[serviceID]].Skills = append(items[indexes[serviceID]].Skills, value)
	}
	if err := skillRows.Err(); err != nil {
		skillRows.Close()
		return err
	}
	if err := skillRows.Close(); err != nil {
		return err
	}
	mediaRows, err := database.QueryContext(ctx, `SELECT sm.service_id::text, mo.id::text, mo.mime_type, mo.size_bytes, sm.sort_order
FROM service_media sm JOIN media_objects mo ON mo.id = sm.media_object_id AND mo.deleted_at IS NULL
WHERE sm.service_id = ANY($1::uuid[]) AND ($2::boolean = false OR
  (mo.purpose = 'SERVICE' AND mo.uploaded_at IS NOT NULL AND mo.scan_status = 'CLEAN'))
ORDER BY sm.service_id, sm.sort_order, mo.id`, ids, public)
	if err != nil {
		return err
	}
	for mediaRows.Next() {
		var serviceID string
		var value Media
		if err := mediaRows.Scan(&serviceID, &value.ID, &value.MIMEType, &value.SizeBytes, &value.SortOrder); err != nil {
			mediaRows.Close()
			return err
		}
		items[indexes[serviceID]].Media = append(items[indexes[serviceID]].Media, value)
	}
	if err := mediaRows.Err(); err != nil {
		mediaRows.Close()
		return err
	}
	if err := mediaRows.Close(); err != nil {
		return err
	}
	educationRows, err := database.QueryContext(ctx, `SELECT service_id::text, format, duration_minutes, sessions_count,
COALESCE(audience_type, ''), group_size_max FROM education_service_details
WHERE service_id = ANY($1::uuid[])`, ids)
	if err != nil {
		return err
	}
	defer educationRows.Close()
	for educationRows.Next() {
		var serviceID string
		var details EducationDetails
		if err := educationRows.Scan(&serviceID, &details.Format, &details.DurationMinutes, &details.SessionsCount, &details.AudienceType, &details.GroupSizeMax); err != nil {
			return err
		}
		items[indexes[serviceID]].Education = &details
	}
	return educationRows.Err()
}

func collectRows(rows *sql.Rows) ([]Item, error) {
	defer rows.Close()
	items := make([]Item, 0)
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return items, nil
}

type rowScanner interface{ Scan(...any) error }

func scanItem(row rowScanner) (Item, error) {
	item := Item{Skills: []Reference{}, Media: []Media{}}
	var price sql.NullInt64
	var currency string
	err := row.Scan(&item.ID, &item.SellerID, &item.SellerUsername, &item.SellerDisplayName, &item.SellerNativeRating, &item.SellerReviewsCount,
		&item.Category.ID, &item.Category.Slug, &item.Category.Name, &item.ServiceType, &item.Title, &item.Slug,
		&item.ShortDescription, &item.Description, &item.PriceType, &price, &currency, &item.DeliveryDays,
		&item.IncludedRevisions, &item.Status, &item.ModerationStatus, &item.ModerationReason, &item.Visibility, &item.PublishedAt, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, err
	}
	if price.Valid {
		item.PriceFrom = &Money{AmountKopecks: price.Int64, Currency: currency}
	}
	return item, nil
}

func boundedLimit(limit int) int {
	if limit < 1 {
		return 20
	}
	if limit > 50 {
		return 50
	}
	return limit
}
func amount(value *Money) any {
	if value == nil {
		return nil
	}
	return value.AmountKopecks
}
func mapPostgresError(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return ErrConflict
		case "23503":
			return fmt.Errorf("%w: unknown service reference", ErrInvalidReference)
		case "23514":
			return ErrInvalidInput
		}
	}
	return err
}
