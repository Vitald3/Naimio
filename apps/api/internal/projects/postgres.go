package projects

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"freelance/apps/api/internal/platform/contentmoderation"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type PostgresRepository struct{ DB *sql.DB }

const projectColumns = `p.id::text, p.customer_user_id::text, u.display_name, COALESCE(u.username, ''),
COALESCE(c.id::text, ''), COALESCE(c.slug, ''), COALESCE(c.name, ''), p.title, p.slug, p.description,
p.budget_type, p.budget_min_kopecks, p.budget_max_kopecks, p.currency, p.deadline_at, COALESCE(p.experience_level, ''),
p.visibility, p.status, p.moderation_status, COALESCE(p.moderation_reason, ''), p.source_type, p.published_at, p.proposal_count, p.created_at, p.updated_at`

func (r PostgresRepository) Create(ctx context.Context, actorID string, input CreateRequest) (Item, error) {
	if contentmoderation.LooksLikeJunk(input.Title, input.Description) {
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
	if err := Validate(item, time.Now().UTC(), false); err != nil {
		return Item{}, err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Item{}, err
	}
	defer tx.Rollback()
	if err := requireCustomer(ctx, tx, actorID); err != nil {
		return Item{}, err
	}
	categoryID, err := validateCategory(ctx, tx, item.Category, false)
	if err != nil {
		return Item{}, err
	}
	sourceType := "MANUAL"
	if input.SourceDraftToken != "" {
		if len(input.SourceDraftToken) != 64 {
			return Item{}, fmt.Errorf("%w: invalid source draft", ErrInvalidInput)
		}
		hash := sha256.Sum256([]byte(input.SourceDraftToken))
		if err := tx.QueryRowContext(ctx, `SELECT source_type FROM project_drafts WHERE guest_token_hash=$1 AND owner_user_id=$2 AND expires_at>now()`, hash[:], actorID).Scan(&sourceType); errors.Is(err, sql.ErrNoRows) {
			return Item{}, fmt.Errorf("%w: invalid source draft", ErrInvalidInput)
		} else if err != nil {
			return Item{}, err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO projects
(id,customer_user_id,category_id,title,slug,description,budget_type,budget_min_kopecks,budget_max_kopecks,currency,
 deadline_at,experience_level,visibility,status,source_type)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'RUB',$10,NULLIF($11,''),$12,'DRAFT',$13)`, id, actorID, categoryID,
		item.Title, item.Slug, item.Description, item.Budget.Type, item.Budget.MinKopecks, item.Budget.MaxKopecks,
		item.DeadlineAt, item.ExperienceLevel, item.Visibility, sourceType)
	if err != nil {
		return Item{}, mapPostgresError(err)
	}
	if err := replaceRelations(ctx, tx, actorID, id, item); err != nil {
		return Item{}, err
	}
	if err := tx.Commit(); err != nil {
		return Item{}, mapPostgresError(err)
	}
	return r.GetOwned(ctx, actorID, id)
}

func (r PostgresRepository) GetOwned(ctx context.Context, actorID, id string) (Item, error) {
	item, err := scanItem(r.DB.QueryRowContext(ctx, `SELECT `+projectColumns+` FROM projects p JOIN users u ON u.id=p.customer_user_id
LEFT JOIN categories c ON c.id=p.category_id WHERE p.id=$1 AND p.customer_user_id=$2 AND p.deleted_at IS NULL`, id, actorID))
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
	var at, id any
	if cursor != nil {
		at, id = cursor.At, cursor.ID
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT `+projectColumns+` FROM projects p JOIN users u ON u.id=p.customer_user_id LEFT JOIN categories c ON c.id=p.category_id
WHERE p.customer_user_id=$1 AND p.deleted_at IS NULL AND ($2::timestamptz IS NULL OR (p.created_at,p.id)<($2,$3::uuid))
ORDER BY p.created_at DESC,p.id DESC LIMIT $4`, actorID, at, id, limit+1)
	if err != nil {
		return Page{}, err
	}
	items, err := collectRows(rows)
	if err != nil {
		return Page{}, err
	}
	return r.finishPage(ctx, items, limit, false, func(item Item) time.Time { return item.CreatedAt })
}
func (r PostgresRepository) Update(ctx context.Context, actorID, id string, patch PatchRequest) (Item, error) {
	existing, err := r.GetOwned(ctx, actorID, id)
	if err != nil {
		return Item{}, err
	}
	if existing.Status != "DRAFT" {
		return Item{}, ErrInvalidState
	}
	item := mergePatch(existing, patch, time.Now().UTC())
	if err := Validate(item, time.Now().UTC(), false); err != nil {
		return Item{}, err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Item{}, err
	}
	defer tx.Rollback()
	categoryID, err := validateCategory(ctx, tx, item.Category, false)
	if err != nil {
		return Item{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE projects SET category_id=$3,title=$4,slug=$5,description=$6,budget_type=$7,
budget_min_kopecks=$8,budget_max_kopecks=$9,currency='RUB',deadline_at=$10,experience_level=NULLIF($11,''),visibility=$12,updated_at=now()
WHERE id=$1 AND customer_user_id=$2 AND status='DRAFT' AND deleted_at IS NULL`, id, actorID, categoryID, item.Title, item.Slug, item.Description, item.Budget.Type, item.Budget.MinKopecks, item.Budget.MaxKopecks, item.DeadlineAt, item.ExperienceLevel, item.Visibility)
	if err != nil {
		return Item{}, mapPostgresError(err)
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return Item{}, ErrInvalidState
	}
	if err := replaceRelations(ctx, tx, actorID, id, item); err != nil {
		return Item{}, err
	}
	if err := tx.Commit(); err != nil {
		return Item{}, mapPostgresError(err)
	}
	return r.GetOwned(ctx, actorID, id)
}
func (r PostgresRepository) Delete(ctx context.Context, actorID, id string) error {
	result, err := r.DB.ExecContext(ctx, `UPDATE projects SET status='ARCHIVED',deleted_at=now(),updated_at=now()
WHERE id=$1 AND customer_user_id=$2 AND status IN ('DRAFT','CANCELLED','COMPLETED') AND deleted_at IS NULL`, id, actorID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		if _, getErr := r.GetOwned(ctx, actorID, id); getErr != nil {
			return getErr
		}
		return ErrInvalidState
	}
	return nil
}

func (r PostgresRepository) Transition(ctx context.Context, actorID, id, action string) (Item, error) {
	current, err := r.GetOwned(ctx, actorID, id)
	if err != nil {
		return Item{}, err
	}
	if action == "complete" && current.Status == "COMPLETED" {
		return current, nil
	}
	target, allowed := transition(current.Status, action)
	if !allowed {
		return Item{}, ErrInvalidState
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Item{}, err
	}
	defer tx.Rollback()
	if action == "publish" {
		if contentmoderation.LooksLikeJunk(current.Title, current.Description) {
			return Item{}, invalid("Материал не прошёл автоматическую проверку. Уберите бессмысленный, повторяющийся или подозрительный текст и попробуйте снова.")
		}
		if err := requireCustomer(ctx, tx, actorID); err != nil {
			return Item{}, err
		}
		if err := Validate(current, time.Now().UTC(), true); err != nil {
			return Item{}, err
		}
		if _, err := validateCategory(ctx, tx, current.Category, true); err != nil {
			return Item{}, err
		}
		if err := validatePublishable(ctx, tx, actorID, id); err != nil {
			return Item{}, err
		}
	}
	var result sql.Result
	switch action {
	case "publish":
		result, err = tx.ExecContext(ctx, `UPDATE projects SET status='OPEN',moderation_status='VISIBLE',moderation_reason=NULL,moderated_by=NULL,moderated_at=NULL,published_at=COALESCE(published_at,now()),updated_at=now() WHERE id=$1 AND customer_user_id=$2 AND status='DRAFT' AND deleted_at IS NULL`, id, actorID)
	case "make-public":
		result, err = tx.ExecContext(ctx, `UPDATE projects SET visibility='PUBLIC',updated_at=now() WHERE id=$1 AND customer_user_id=$2 AND status IN ('OPEN','MATCHING','IN_PROGRESS','COMPLETED') AND published_at IS NOT NULL AND deleted_at IS NULL`, id, actorID)
	case "cancel":
		result, err = tx.ExecContext(ctx, `UPDATE projects SET status='CANCELLED',updated_at=now() WHERE id=$1 AND customer_user_id=$2 AND status IN ('DRAFT','OPEN','MATCHING') AND deleted_at IS NULL`, id, actorID)
	case "complete":
		result, err = tx.ExecContext(ctx, `UPDATE projects SET status='COMPLETED',updated_at=now() WHERE id=$1 AND customer_user_id=$2 AND status='IN_PROGRESS' AND deleted_at IS NULL AND EXISTS(SELECT 1 FROM safe_deals d WHERE d.project_id=projects.id AND d.status='COMPLETED')`, id, actorID)
	}
	if err != nil {
		return Item{}, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		if action == "complete" {
			var status string
			if scanErr := tx.QueryRowContext(ctx, `SELECT status FROM projects WHERE id=$1 AND customer_user_id=$2 AND deleted_at IS NULL`, id, actorID).Scan(&status); scanErr == nil && status == "COMPLETED" {
				_ = tx.Rollback()
				return r.GetOwned(ctx, actorID, id)
			}
		}
		return Item{}, ErrInvalidState
	}
	if err := insertOutbox(ctx, tx, id, eventType(action)); err != nil {
		return Item{}, err
	}
	if action == "complete" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO user_trust_stats(user_id,completed_projects_count,updated_at)
SELECT participant,count(DISTINCT completed.id),now() FROM (
  SELECT p.customer_user_id AS participant FROM projects p WHERE p.id=$1
  UNION SELECT a.freelancer_user_id FROM project_assignments a WHERE a.project_id=$1
) parties JOIN projects completed ON completed.status='COMPLETED'
LEFT JOIN project_assignments assignment ON assignment.project_id=completed.id
WHERE completed.customer_user_id=parties.participant OR assignment.freelancer_user_id=parties.participant
GROUP BY participant ON CONFLICT(user_id) DO UPDATE SET completed_projects_count=EXCLUDED.completed_projects_count,updated_at=EXCLUDED.updated_at`, id); err != nil {
			return Item{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Item{}, err
	}
	_ = target
	return r.GetOwned(ctx, actorID, id)
}

func (r PostgresRepository) ListPublic(ctx context.Context, filter Filter, cursor *Cursor, limit int) (Page, error) {
	if err := ValidateFilter(filter); err != nil {
		return Page{}, err
	}
	limit = boundedLimit(limit)
	var at, id, minBudget, deadlineBefore any
	if cursor != nil {
		at, id = cursor.At, cursor.ID
	}
	if filter.MinBudgetKopecks != nil {
		minBudget = *filter.MinBudgetKopecks
	}
	if filter.DeadlineBefore != nil {
		deadlineBefore = *filter.DeadlineBefore
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT `+projectColumns+` FROM projects p JOIN users u ON u.id=p.customer_user_id JOIN categories c ON c.id=p.category_id AND c.is_active=true
WHERE p.status IN ('OPEN','MATCHING') AND p.moderation_status='VISIBLE' AND p.visibility='PUBLIC' AND p.deleted_at IS NULL AND u.status='ACTIVE' AND u.deleted_at IS NULL
AND ($1='' OR p.search_vector @@ websearch_to_tsquery('simple',$1) OR p.title ILIKE '%'||$1||'%')
AND ($2='' OR c.id::text=$2 OR c.slug=$2) AND ($3='' OR p.budget_type=$3) AND ($4='' OR p.experience_level=$4)
AND ($5::bigint IS NULL OR COALESCE(p.budget_max_kopecks,p.budget_min_kopecks,0)>=$5)
AND ($6::timestamptz IS NULL OR p.deadline_at IS NULL OR p.deadline_at<=$6)
AND ($7::timestamptz IS NULL OR (p.published_at,p.id)<($7,$8::uuid)) ORDER BY p.published_at DESC,p.id DESC LIMIT $9`, filter.Q, filter.Category, filter.BudgetType, filter.ExperienceLevel, minBudget, deadlineBefore, at, id, limit+1)
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
	// The canonical project page must stay reachable for the project's whole
	// lifecycle, not only while it is OPEN/MATCHING. Safe-deal participants,
	// reviewers and anyone opening a shared link would otherwise hit a 404 the
	// moment work is funded, started or completed. Privacy is preserved: only
	// PUBLIC, moderated-visible, non-deleted projects that were actually
	// published are returned, and the customer identity is stripped below.
	// Public listings (ListPublic) remain limited to actively-open projects.
	statusCondition := "p.status IN ('OPEN','MATCHING')"
	condition := `p.slug=$1 AND NOT EXISTS(SELECT 1 FROM projects duplicate WHERE duplicate.slug=p.slug AND duplicate.id<>p.id AND duplicate.status IN ('OPEN','MATCHING') AND duplicate.moderation_status='VISIBLE' AND duplicate.visibility='PUBLIC' AND duplicate.deleted_at IS NULL)`
	if validUUID(reference) {
		condition = "p.id=$1::uuid"
		statusCondition = "p.published_at IS NOT NULL"
	}
	item, err := scanItem(r.DB.QueryRowContext(ctx, `SELECT `+projectColumns+` FROM projects p JOIN users u ON u.id=p.customer_user_id LEFT JOIN categories c ON c.id=p.category_id
WHERE `+statusCondition+` AND p.moderation_status='VISIBLE' AND p.visibility='PUBLIC' AND p.deleted_at IS NULL AND u.status='ACTIVE' AND u.deleted_at IS NULL AND (`+condition+`)`, reference))
	if err != nil {
		return Item{}, err
	}
	if err := loadRelations(ctx, r.DB, &item, true); err != nil {
		return Item{}, err
	}
	item.CustomerID = ""
	return item, nil
}

type dbtx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func requireCustomer(ctx context.Context, database dbtx, actorID string) error {
	var allowed bool
	err := database.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN user_capabilities uc ON uc.user_id=u.id AND uc.capability='CUSTOMER' WHERE u.id=$1 AND u.status='ACTIVE' AND u.deleted_at IS NULL)`, actorID).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrCustomerIneligible
	}
	return nil
}
func validateCategory(ctx context.Context, database dbtx, category *Reference, required bool) (any, error) {
	if category == nil {
		if required {
			return nil, invalid("category is required to publish")
		}
		return nil, nil
	}
	id := normalizeID(category.ID)
	if !validUUID(id) {
		return nil, invalid("invalid category id")
	}
	var exists bool
	if err := database.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM categories WHERE id=$1 AND is_active=true)`, id).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("%w: category is not active", ErrInvalidReference)
	}
	return id, nil
}
func replaceRelations(ctx context.Context, tx *sql.Tx, actorID, id string, item Item) error {
	for _, statement := range []string{"DELETE FROM project_skills WHERE project_id=$1", "DELETE FROM project_media WHERE project_id=$1"} {
		if _, err := tx.ExecContext(ctx, statement, id); err != nil {
			return err
		}
	}
	seen := map[string]struct{}{}
	for _, skill := range item.Skills {
		skillID := normalizeID(skill.ID)
		if !validUUID(skillID) {
			return invalid("invalid skill id")
		}
		if _, ok := seen[skillID]; ok {
			return invalid("duplicate skill id")
		}
		seen[skillID] = struct{}{}
		result, err := tx.ExecContext(ctx, `INSERT INTO project_skills(project_id,skill_id,importance) SELECT $1,id,100 FROM skills WHERE id=$2 AND is_active=true`, id, skillID)
		if err != nil {
			return mapPostgresError(err)
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return fmt.Errorf("%w: skill is not active", ErrInvalidReference)
		}
	}
	seen = map[string]struct{}{}
	for index, medium := range item.Media {
		mediaID := normalizeID(medium.ID)
		if !validUUID(mediaID) {
			return invalid("invalid media id")
		}
		if _, ok := seen[mediaID]; ok {
			return invalid("duplicate media id")
		}
		seen[mediaID] = struct{}{}
		result, err := tx.ExecContext(ctx, `INSERT INTO project_media(project_id,media_object_id,sort_order) SELECT $1,id,$4 FROM media_objects WHERE id=$3 AND owner_user_id=$2 AND purpose='PROJECT' AND uploaded_at IS NOT NULL AND scan_status='CLEAN' AND deleted_at IS NULL`, id, actorID, mediaID, index)
		if err != nil {
			return mapPostgresError(err)
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return fmt.Errorf("%w: media is not attachable", ErrInvalidReference)
		}
	}
	return nil
}
func validatePublishable(ctx context.Context, database dbtx, actorID, id string) error {
	var invalid bool
	err := database.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM project_skills ps LEFT JOIN skills s ON s.id=ps.skill_id AND s.is_active=true WHERE ps.project_id=p.id AND s.id IS NULL)
OR EXISTS(SELECT 1 FROM project_media pm LEFT JOIN media_objects mo ON mo.id=pm.media_object_id AND mo.owner_user_id=p.customer_user_id AND mo.purpose='PROJECT' AND mo.uploaded_at IS NOT NULL AND mo.scan_status='CLEAN' AND mo.deleted_at IS NULL WHERE pm.project_id=p.id AND mo.id IS NULL)
FROM projects p WHERE p.id=$1 AND p.customer_user_id=$2 AND p.deleted_at IS NULL`, id, actorID).Scan(&invalid)
	if err != nil {
		return err
	}
	if invalid {
		return fmt.Errorf("%w: project references are not publishable", ErrInvalidReference)
	}
	return nil
}
func insertOutbox(ctx context.Context, tx *sql.Tx, id, event string) error {
	eventID, err := newUUIDv7()
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]string{"project_id": id})
	_, err = tx.ExecContext(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload) VALUES($1,'PROJECT',$2,$3,$4::jsonb)`, eventID, id, event, string(payload))
	return err
}

func loadRelations(ctx context.Context, database dbtx, item *Item, public bool) error {
	skills, err := database.QueryContext(ctx, `SELECT s.id::text,s.slug,s.name,ps.importance FROM project_skills ps JOIN skills s ON s.id=ps.skill_id AND s.is_active=true WHERE ps.project_id=$1 ORDER BY ps.importance DESC,s.name`, item.ID)
	if err != nil {
		return err
	}
	for skills.Next() {
		var value Skill
		if err := skills.Scan(&value.ID, &value.Slug, &value.Name, &value.Importance); err != nil {
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
	rows, err := database.QueryContext(ctx, `SELECT mo.id::text,COALESCE(mo.original_filename, ''),mo.mime_type,mo.size_bytes,pm.sort_order FROM project_media pm JOIN media_objects mo ON mo.id=pm.media_object_id AND mo.deleted_at IS NULL WHERE pm.project_id=$1 AND ($2::boolean=false OR(mo.purpose='PROJECT' AND mo.uploaded_at IS NOT NULL AND mo.scan_status='CLEAN')) ORDER BY pm.sort_order,mo.id`, item.ID, public)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var value Media
		if err := rows.Scan(&value.ID, &value.OriginalFilename, &value.MIMEType, &value.SizeBytes, &value.SortOrder); err != nil {
			return err
		}
		item.Media = append(item.Media, value)
	}
	return rows.Err()
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
	if public {
		for i := range page.Items {
			page.Items[i].CustomerID = ""
		}
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
	skillRows, err := database.QueryContext(ctx, `SELECT ps.project_id::text,s.id::text,s.slug,s.name,ps.importance
FROM project_skills ps JOIN skills s ON s.id=ps.skill_id AND s.is_active=true
WHERE ps.project_id=ANY($1::uuid[]) ORDER BY ps.project_id,ps.importance DESC,s.name`, ids)
	if err != nil {
		return err
	}
	for skillRows.Next() {
		var projectID string
		var value Skill
		if err := skillRows.Scan(&projectID, &value.ID, &value.Slug, &value.Name, &value.Importance); err != nil {
			skillRows.Close()
			return err
		}
		items[indexes[projectID]].Skills = append(items[indexes[projectID]].Skills, value)
	}
	if err := skillRows.Err(); err != nil {
		skillRows.Close()
		return err
	}
	if err := skillRows.Close(); err != nil {
		return err
	}
	mediaRows, err := database.QueryContext(ctx, `SELECT pm.project_id::text,mo.id::text,COALESCE(mo.original_filename, ''),mo.mime_type,mo.size_bytes,pm.sort_order
FROM project_media pm JOIN media_objects mo ON mo.id=pm.media_object_id AND mo.deleted_at IS NULL
WHERE pm.project_id=ANY($1::uuid[]) AND ($2::boolean=false OR(mo.purpose='PROJECT' AND mo.uploaded_at IS NOT NULL AND mo.scan_status='CLEAN'))
ORDER BY pm.project_id,pm.sort_order,mo.id`, ids, public)
	if err != nil {
		return err
	}
	defer mediaRows.Close()
	for mediaRows.Next() {
		var projectID string
		var value Media
		if err := mediaRows.Scan(&projectID, &value.ID, &value.OriginalFilename, &value.MIMEType, &value.SizeBytes, &value.SortOrder); err != nil {
			return err
		}
		items[indexes[projectID]].Media = append(items[indexes[projectID]].Media, value)
	}
	return mediaRows.Err()
}
func collectRows(rows *sql.Rows) ([]Item, error) {
	defer rows.Close()
	items := []Item{}
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
	item := Item{Skills: []Skill{}, Media: []Media{}}
	var categoryID, categorySlug, categoryName string
	err := row.Scan(&item.ID, &item.CustomerID, &item.CustomerDisplayName, &item.CustomerUsername, &categoryID, &categorySlug, &categoryName, &item.Title, &item.Slug, &item.Description, &item.Budget.Type, &item.Budget.MinKopecks, &item.Budget.MaxKopecks, &item.Budget.Currency, &item.DeadlineAt, &item.ExperienceLevel, &item.Visibility, &item.Status, &item.ModerationStatus, &item.ModerationReason, &item.SourceType, &item.PublishedAt, &item.ProposalCount, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, err
	}
	if categoryID != "" {
		item.Category = &Reference{ID: categoryID, Slug: categorySlug, Name: categoryName}
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
func mapPostgresError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505":
			return ErrConflict
		case "23503":
			return fmt.Errorf("%w: unknown project reference", ErrInvalidReference)
		case "23514":
			return ErrInvalidInput
		}
	}
	return err
}
