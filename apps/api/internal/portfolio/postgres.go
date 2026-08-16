package portfolio

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type PostgresRepository struct{ DB *sql.DB }

func (r PostgresRepository) Create(ctx context.Context, actorID string, input WriteRequest) (Item, error) {
	if actorID == "" {
		return Item{}, ErrUnauthorized
	}
	itemID, err := newUUIDv7()
	if err != nil {
		return Item{}, err
	}
	item := itemFromWrite(actorID, itemID, input, time.Now().UTC())
	if err := Validate(item, time.Now().UTC()); err != nil {
		return Item{}, err
	}
	if err := validateIDList(input.MediaObjectIDs, 20, "media"); err != nil {
		return Item{}, err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Item{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
INSERT INTO portfolio_items (id, user_id, title, slug, description, external_url, price_min_kopecks,
  price_max_kopecks, completed_on, visibility, sort_order)
VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), $7, $8, NULLIF($9, '')::date, $10, $11)`,
		item.ID, actorID, item.Title, item.Slug, item.Description, item.ExternalURL, item.PriceMinKopecks,
		item.PriceMaxKopecks, item.CompletedOn, item.Visibility, item.SortOrder)
	if err != nil {
		return Item{}, mapPostgresError(err)
	}
	if err := replaceLinks(ctx, tx, actorID, item.ID, input); err != nil {
		return Item{}, err
	}
	if err := tx.Commit(); err != nil {
		return Item{}, mapPostgresError(err)
	}
	return r.GetOwned(ctx, actorID, item.ID)
}

func (r PostgresRepository) GetOwned(ctx context.Context, actorID, itemID string) (Item, error) {
	item, err := scanItem(r.DB.QueryRowContext(ctx, `
SELECT pi.id::text, pi.user_id::text, COALESCE(u.username, ''), pi.title, pi.slug,
  COALESCE(pi.description, ''), COALESCE(pi.external_url, ''), pi.price_min_kopecks, pi.price_max_kopecks,
  COALESCE(to_char(pi.completed_on, 'YYYY-MM-DD'), ''), pi.visibility, pi.sort_order,
  pi.created_at, pi.updated_at
FROM portfolio_items pi JOIN users u ON u.id = pi.user_id
WHERE pi.id = $1 AND pi.user_id = $2 AND pi.deleted_at IS NULL`, itemID, actorID))
	if err != nil {
		return Item{}, err
	}
	if err := loadRelations(ctx, r.DB, &item, false); err != nil {
		return Item{}, err
	}
	return item, nil
}

func (r PostgresRepository) ListOwned(ctx context.Context, actorID string) ([]Item, error) {
	rows, err := r.DB.QueryContext(ctx, `
SELECT pi.id::text, pi.user_id::text, COALESCE(u.username, ''), pi.title, pi.slug,
  COALESCE(pi.description, ''), COALESCE(pi.external_url, ''), pi.price_min_kopecks, pi.price_max_kopecks,
  COALESCE(to_char(pi.completed_on, 'YYYY-MM-DD'), ''), pi.visibility, pi.sort_order, pi.created_at, pi.updated_at
FROM portfolio_items pi JOIN users u ON u.id=pi.user_id
WHERE pi.user_id=$1 AND pi.deleted_at IS NULL ORDER BY pi.sort_order, pi.created_at DESC, pi.id`, actorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Item{}
	for rows.Next() {
		item, scanErr := scanItem(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range items {
		if err := loadRelations(ctx, r.DB, &items[i], false); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (r PostgresRepository) Update(ctx context.Context, actorID, itemID string, input WriteRequest) (Item, error) {
	item := itemFromWrite(actorID, itemID, input, time.Now().UTC())
	if err := Validate(item, time.Now().UTC()); err != nil {
		return Item{}, err
	}
	if err := validateIDList(input.MediaObjectIDs, 20, "media"); err != nil {
		return Item{}, err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Item{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
UPDATE portfolio_items
SET title = $3, slug = $4, description = NULLIF($5, ''), external_url = NULLIF($6, ''),
    price_min_kopecks = $7, price_max_kopecks = $8, completed_on = NULLIF($9, '')::date,
    visibility = $10, sort_order = $11, updated_at = now()
WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`, itemID, actorID, item.Title, item.Slug,
		item.Description, item.ExternalURL, item.PriceMinKopecks, item.PriceMaxKopecks, item.CompletedOn, item.Visibility, item.SortOrder)
	if err != nil {
		return Item{}, mapPostgresError(err)
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return Item{}, ErrNotFound
	}
	if err := replaceLinks(ctx, tx, actorID, itemID, input); err != nil {
		return Item{}, err
	}
	if err := tx.Commit(); err != nil {
		return Item{}, mapPostgresError(err)
	}
	return r.GetOwned(ctx, actorID, itemID)
}

func (r PostgresRepository) Delete(ctx context.Context, actorID, itemID string) error {
	result, err := r.DB.ExecContext(ctx, `
UPDATE portfolio_items SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`, itemID, actorID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrNotFound
	}
	return nil
}

func (r PostgresRepository) AttachMedia(ctx context.Context, actorID, itemID, mediaID string, sortOrder int) (Item, error) {
	if !validUUID(mediaID) || sortOrder < 0 || sortOrder > 10000 {
		return Item{}, invalid("invalid media reference")
	}
	mediaID = strings.ToLower(strings.TrimSpace(mediaID))
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Item{}, err
	}
	defer tx.Rollback()
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (
SELECT 1 FROM portfolio_items WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL)`, itemID, actorID).Scan(&exists); err != nil {
		return Item{}, err
	}
	if !exists {
		return Item{}, ErrNotFound
	}
	var mediaCount int
	var alreadyAttached bool
	if err := tx.QueryRowContext(ctx, `SELECT count(*), COALESCE(bool_or(media_object_id = $2), false)
FROM portfolio_media WHERE portfolio_item_id = $1`, itemID, mediaID).Scan(&mediaCount, &alreadyAttached); err != nil {
		return Item{}, err
	}
	if mediaCount >= 20 && !alreadyAttached {
		return Item{}, invalid("too many media references")
	}
	result, err := tx.ExecContext(ctx, `
INSERT INTO portfolio_media (portfolio_item_id, media_object_id, sort_order)
SELECT $1, id, $4 FROM media_objects
WHERE id = $3 AND owner_user_id = $2 AND purpose = 'PORTFOLIO' AND uploaded_at IS NOT NULL AND scan_status = 'CLEAN' AND deleted_at IS NULL
ON CONFLICT (portfolio_item_id, media_object_id) DO UPDATE SET sort_order = EXCLUDED.sort_order`, itemID, actorID, mediaID, sortOrder)
	if err != nil {
		return Item{}, mapPostgresError(err)
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return Item{}, fmt.Errorf("%w: media is not attachable", ErrInvalidReference)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE portfolio_items SET updated_at = now() WHERE id = $1", itemID); err != nil {
		return Item{}, err
	}
	if err := tx.Commit(); err != nil {
		return Item{}, err
	}
	return r.GetOwned(ctx, actorID, itemID)
}

func (r PostgresRepository) DetachMedia(ctx context.Context, actorID, itemID, mediaID string) error {
	if !validUUID(mediaID) {
		return invalid("invalid media reference")
	}
	mediaID = strings.ToLower(strings.TrimSpace(mediaID))
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM portfolio_items WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL)", itemID, actorID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM portfolio_media WHERE portfolio_item_id = $1 AND media_object_id = $2", itemID, mediaID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE portfolio_items SET updated_at = now() WHERE id = $1", itemID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r PostgresRepository) ListPublic(ctx context.Context, username string, cursor *Cursor, limit int) (Page, error) {
	if limit < 1 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	var cursorOrder any
	var cursorTime any
	var cursorID any
	if cursor != nil {
		cursorOrder, cursorTime, cursorID = cursor.SortOrder, cursor.CreatedAt, cursor.ID
	}
	rows, err := r.DB.QueryContext(ctx, `
SELECT pi.id::text, pi.user_id::text, u.username, pi.title, pi.slug,
  COALESCE(pi.description, ''), COALESCE(pi.external_url, ''), pi.price_min_kopecks, pi.price_max_kopecks,
  COALESCE(to_char(pi.completed_on, 'YYYY-MM-DD'), ''), pi.visibility, pi.sort_order,
  pi.created_at, pi.updated_at
FROM portfolio_items pi JOIN users u ON u.id = pi.user_id
WHERE u.username_normalized = lower($1) AND u.status = 'ACTIVE' AND u.deleted_at IS NULL
  AND pi.visibility = 'PUBLIC' AND pi.deleted_at IS NULL
  AND ($2::int IS NULL OR pi.sort_order > $2
    OR (pi.sort_order = $2 AND pi.created_at < $3)
    OR (pi.sort_order = $2 AND pi.created_at = $3 AND pi.id > $4))
ORDER BY pi.sort_order, pi.created_at DESC, pi.id
LIMIT $5`, username, cursorOrder, cursorTime, cursorID, limit+1)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	items := make([]Item, 0, limit+1)
	for rows.Next() {
		item, scanErr := scanItem(rows)
		if scanErr != nil {
			return Page{}, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	if err := rows.Close(); err != nil {
		return Page{}, err
	}
	page := Page{Items: items}
	if len(page.Items) > limit {
		last := page.Items[limit-1]
		page.Items = page.Items[:limit]
		page.NextCursor = &Cursor{SortOrder: last.SortOrder, CreatedAt: last.CreatedAt, ID: last.ID}
	}
	for index := range page.Items {
		if err := loadRelations(ctx, r.DB, &page.Items[index], true); err != nil {
			return Page{}, err
		}
		page.Items[index].UserID = ""
	}
	return page, nil
}

func replaceLinks(ctx context.Context, tx *sql.Tx, actorID, itemID string, input WriteRequest) error {
	for _, statement := range []string{
		"DELETE FROM portfolio_categories WHERE portfolio_item_id = $1",
		"DELETE FROM portfolio_skills WHERE portfolio_item_id = $1",
		"DELETE FROM portfolio_media WHERE portfolio_item_id = $1",
	} {
		if _, err := tx.ExecContext(ctx, statement, itemID); err != nil {
			return err
		}
	}
	for _, categoryID := range input.CategoryIDs {
		result, err := tx.ExecContext(ctx, `INSERT INTO portfolio_categories (portfolio_item_id, category_id)
SELECT $1, id FROM categories WHERE id = $2 AND is_active = true`, itemID, strings.ToLower(strings.TrimSpace(categoryID)))
		if err != nil {
			return mapPostgresError(err)
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return fmt.Errorf("%w: unknown or inactive category", ErrInvalidReference)
		}
	}
	for _, skillID := range input.SkillIDs {
		result, err := tx.ExecContext(ctx, `INSERT INTO portfolio_skills (portfolio_item_id, skill_id)
SELECT $1, id FROM skills WHERE id = $2 AND is_active = true`, itemID, strings.ToLower(strings.TrimSpace(skillID)))
		if err != nil {
			return mapPostgresError(err)
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return fmt.Errorf("%w: unknown or inactive skill", ErrInvalidReference)
		}
	}
	if len(input.MediaObjectIDs) > 20 {
		return invalid("too many media references")
	}
	for index, mediaID := range input.MediaObjectIDs {
		result, err := tx.ExecContext(ctx, `INSERT INTO portfolio_media (portfolio_item_id, media_object_id, sort_order)
SELECT $1, id, $4 FROM media_objects
WHERE id = $3 AND owner_user_id = $2 AND purpose = 'PORTFOLIO' AND uploaded_at IS NOT NULL AND scan_status = 'CLEAN' AND deleted_at IS NULL`, itemID, actorID, strings.ToLower(strings.TrimSpace(mediaID)), index)
		if err != nil {
			return mapPostgresError(err)
		}
		if count, _ := result.RowsAffected(); count != 1 {
			return fmt.Errorf("%w: media is not attachable", ErrInvalidReference)
		}
	}
	return nil
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func loadRelations(ctx context.Context, database queryer, item *Item, public bool) error {
	categories, err := database.QueryContext(ctx, `SELECT c.id::text, c.slug, c.name
FROM portfolio_categories pc JOIN categories c ON c.id = pc.category_id AND c.is_active = true
WHERE pc.portfolio_item_id = $1 ORDER BY c.name`, item.ID)
	if err != nil {
		return err
	}
	for categories.Next() {
		var reference Reference
		if err := categories.Scan(&reference.ID, &reference.Slug, &reference.Name); err != nil {
			categories.Close()
			return err
		}
		item.Categories = append(item.Categories, reference)
	}
	if err := categories.Err(); err != nil {
		categories.Close()
		return err
	}
	if err := categories.Close(); err != nil {
		return err
	}

	skills, err := database.QueryContext(ctx, `SELECT s.id::text, s.slug, s.name
FROM portfolio_skills ps JOIN skills s ON s.id = ps.skill_id AND s.is_active = true
WHERE ps.portfolio_item_id = $1 ORDER BY s.name`, item.ID)
	if err != nil {
		return err
	}
	for skills.Next() {
		var reference Reference
		if err := skills.Scan(&reference.ID, &reference.Slug, &reference.Name); err != nil {
			skills.Close()
			return err
		}
		item.Skills = append(item.Skills, reference)
	}
	if err := skills.Err(); err != nil {
		skills.Close()
		return err
	}
	if err := skills.Close(); err != nil {
		return err
	}

	mediaRows, err := database.QueryContext(ctx, `SELECT mo.id::text, mo.mime_type, mo.size_bytes, mo.scan_status, pm.sort_order
FROM portfolio_media pm JOIN media_objects mo ON mo.id = pm.media_object_id AND mo.deleted_at IS NULL
WHERE pm.portfolio_item_id = $1 AND ($2::boolean = false OR (mo.uploaded_at IS NOT NULL AND mo.scan_status = 'CLEAN'))
ORDER BY pm.sort_order, mo.id`, item.ID, public)
	if err != nil {
		return err
	}
	defer mediaRows.Close()
	for mediaRows.Next() {
		var media Media
		if err := mediaRows.Scan(&media.ID, &media.MIMEType, &media.SizeBytes, &media.ScanStatus, &media.SortOrder); err != nil {
			return err
		}
		if public {
			media.ScanStatus = ""
		}
		item.Media = append(item.Media, media)
	}
	return mediaRows.Err()
}

type rowScanner interface{ Scan(...any) error }

func scanItem(row rowScanner) (Item, error) {
	item := Item{Categories: []Reference{}, Skills: []Reference{}, Media: []Media{}}
	err := row.Scan(&item.ID, &item.UserID, &item.Username, &item.Title, &item.Slug, &item.Description,
		&item.ExternalURL, &item.PriceMinKopecks, &item.PriceMaxKopecks, &item.CompletedOn,
		&item.Visibility, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	return item, err
}

func itemFromWrite(actorID, itemID string, input WriteRequest, now time.Time) Item {
	return Item{ID: itemID, UserID: actorID, Title: strings.TrimSpace(input.Title), Slug: strings.ToLower(strings.TrimSpace(input.Slug)),
		Description: strings.TrimSpace(input.Description), ExternalURL: strings.TrimSpace(input.ExternalURL),
		PriceMinKopecks: input.PriceMinKopecks, PriceMaxKopecks: input.PriceMaxKopecks,
		CompletedOn: strings.TrimSpace(input.CompletedOn), Visibility: input.Visibility, SortOrder: input.SortOrder,
		Categories: references(input.CategoryIDs), Skills: references(input.SkillIDs), Media: []Media{}, CreatedAt: now, UpdatedAt: now}
}

func mapPostgresError(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23505":
			return ErrConflict
		case "23503":
			return fmt.Errorf("%w: unknown portfolio reference", ErrInvalidReference)
		}
	}
	return err
}
