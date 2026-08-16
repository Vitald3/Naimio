package profiles

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

type PostgresRepository struct{ DB *sql.DB }

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (r PostgresRepository) Public(ctx context.Context, username string) (Profile, error) {
	profile, err := scanProfile(r.DB.QueryRowContext(ctx, `
SELECT p.user_id::text, u.username, u.display_name,
       COALESCE(p.professional_title, ''), COALESCE(p.bio, ''), COALESCE(p.location_text, ''),
       COALESCE(p.country_code, ''), p.experience_years, p.hourly_rate_kopecks, p.minimum_order_kopecks,
       p.response_time_minutes,
       p.availability, p.profile_visibility
FROM professional_profiles p
JOIN users u ON u.id = p.user_id
WHERE u.username_normalized = lower($1)
  AND u.status = 'ACTIVE' AND u.deleted_at IS NULL
  AND p.profile_visibility = 'PUBLIC'`, username))
	if err != nil {
		return Profile{}, err
	}
	if err := loadRelations(ctx, r.DB, &profile); err != nil {
		return Profile{}, err
	}
	stats := []Profile{profile}
	if err := loadTrustStats(ctx, r.DB, stats); err == nil {
		profile = stats[0]
	}
	if err := loadEffectivePro(ctx, r.DB, stats); err == nil {
		profile.EffectivePro = stats[0].EffectivePro
	}
	profile.ID = profile.UserID
	return profile, nil
}

func (r PostgresRepository) PublicList(ctx context.Context, query string, cursor *PublicCursor, limit int) (PublicPage, error) {
	if len([]rune(query)) > 120 {
		return PublicPage{}, ErrInvalidInput
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	var cursorScore, cursorUsername, cursorID any
	if cursor != nil {
		cursorScore, cursorUsername, cursorID = cursor.Score, cursor.Username, cursor.ID
	}
	rows, err := r.DB.QueryContext(ctx, `
WITH base AS (
  SELECT p.user_id, u.username, u.username_normalized, u.display_name,
         COALESCE(p.professional_title, '') AS professional_title,
         COALESCE(p.bio, '') AS bio,
         COALESCE(p.country_code, '') AS country_code, p.experience_years,
         p.hourly_rate_kopecks, p.response_time_minutes,
         p.availability, p.profile_visibility,
         COALESCE(uts.native_rating::float8, 0) AS native_rating,
         COALESCE(uts.reviews_count, 0) AS reviews_count,
         COALESCE(uts.completed_projects_count, 0) AS completed_projects,
         CASE
           WHEN EXISTS (
             SELECT 1 FROM feature_flags ff
             WHERE ff.key='pro_subscriptions_enabled' AND ff.enabled
           ) AND EXISTS (
             SELECT 1 FROM user_subscriptions s
             JOIN subscription_plans sp ON sp.id=s.plan_id AND sp.active AND sp.tier='PRO'
             JOIN subscription_plan_entitlements e ON e.plan_id=sp.id
               AND e.feature_key='search.priority_visibility' AND e.kind='BOOLEAN' AND e.enabled
             WHERE s.user_id=p.user_id AND s.status='ACTIVE'
               AND s.starts_at<=now() AND s.current_period_end>now()
           ) THEN LEAST(12.0, GREATEST(0.0, (
             COALESCE((
               SELECT NULLIF(e.config->>'ranking_multiplier','')::float8
               FROM user_subscriptions s
               JOIN subscription_plans sp ON sp.id=s.plan_id AND sp.active AND sp.tier='PRO'
               JOIN subscription_plan_entitlements e ON e.plan_id=sp.id
                 AND e.feature_key='search.priority_visibility' AND e.kind='BOOLEAN' AND e.enabled
               WHERE s.user_id=p.user_id AND s.status='ACTIVE'
                 AND s.starts_at<=now() AND s.current_period_end>now()
               LIMIT 1
             ), 1.08) - 1.0) * 100.0))
           ELSE 0.0
         END AS pro_boost
  FROM professional_profiles p
  JOIN users u ON u.id = p.user_id
  LEFT JOIN user_trust_stats uts ON uts.user_id = p.user_id
  WHERE u.status = 'ACTIVE' AND u.deleted_at IS NULL
    AND u.username IS NOT NULL AND p.profile_visibility = 'PUBLIC'
    AND ($1='' OR to_tsvector('simple',COALESCE(p.professional_title,'')||' '||COALESCE(p.bio,'')) @@ websearch_to_tsquery('simple',$1)
      OR u.display_name ILIKE '%'||$1||'%' OR p.professional_title ILIKE '%'||$1||'%')
), scored AS (
  SELECT *,
    CASE
      WHEN $1='' THEN 40.0
      WHEN lower(display_name)=lower($1) OR lower(professional_title)=lower($1) THEN 90.0
      WHEN lower(display_name) LIKE lower($1)||'%' OR lower(professional_title) LIKE lower($1)||'%' THEN 75.0
      WHEN display_name ILIKE '%'||$1||'%' OR professional_title ILIKE '%'||$1||'%' THEN 65.0
      ELSE GREATEST(35.0, LEAST(60.0, COALESCE(ts_rank(to_tsvector('simple',professional_title||' '||bio), websearch_to_tsquery('simple',$1)),0)*200))
    END AS relevance,
    LEAST(5.0, GREATEST(0.0, native_rating))*8.0 + LEAST(20.0, GREATEST(0.0, completed_projects::float8))*0.5 AS quality
  FROM base
), ranked AS (
  SELECT *, CASE WHEN $1='' THEN
    CASE WHEN reviews_count>0 THEN 100000.0 + native_rating*10000.0 + LEAST(100.0,reviews_count::float8) + LEAST(50.0,completed_projects::float8)
         ELSE pro_boost + LEAST(5.0,completed_projects::float8)*0.1 END
    ELSE relevance + quality + pro_boost END AS rank_score
  FROM scored
)
SELECT user_id::text, username, display_name, professional_title, '', '', country_code, experience_years,
       hourly_rate_kopecks, NULL::bigint, response_time_minutes, availability, profile_visibility,
       CASE WHEN native_rating > 0 THEN native_rating ELSE NULL END,
       reviews_count, completed_projects, rank_score
FROM ranked
WHERE ($2::float8 IS NULL OR (rank_score, username_normalized, user_id) < ($2::float8, $3::text, $4::uuid))
ORDER BY rank_score DESC, username_normalized ASC, user_id ASC
LIMIT $5`, strings.TrimSpace(query), cursorScore, cursorUsername, cursorID, limit+1)
	if err != nil {
		return PublicPage{}, err
	}
	defer rows.Close()
	type scoredRow struct {
		profile Profile
		score   float64
	}
	items := make([]scoredRow, 0)
	for rows.Next() {
		var (
			profile            Profile
			score              float64
			nativeRating       sql.NullFloat64
			reviews, completed int
			bio, location      string
			experience         sql.NullInt64
			hourly, minimum    sql.NullInt64
			response           sql.NullInt64
		)
		if err = rows.Scan(
			&profile.UserID, &profile.Username, &profile.DisplayName, &profile.ProfessionalTitle,
			&bio, &location, &profile.CountryCode, &experience, &hourly, &minimum, &response,
			&profile.Availability, &profile.ProfileVisibility,
			&nativeRating, &reviews, &completed, &score,
		); err != nil {
			return PublicPage{}, err
		}
		if experience.Valid {
			v := int(experience.Int64)
			profile.ExperienceYears = &v
		}
		if hourly.Valid {
			v := hourly.Int64
			profile.HourlyRateKopecks = &v
		}
		if response.Valid {
			v := int(response.Int64)
			profile.ResponseTimeMinutes = &v
		}
		if nativeRating.Valid {
			v := nativeRating.Float64
			profile.NativeRating = &v
		}
		profile.ReviewsCount = reviews
		profile.CompletedProjects = completed
		profile.ID = profile.UserID
		items = append(items, scoredRow{profile: profile, score: score})
	}
	if err := rows.Err(); err != nil {
		return PublicPage{}, err
	}
	if err := rows.Close(); err != nil {
		return PublicPage{}, err
	}
	pageItems := make([]Profile, len(items))
	for i := range items {
		pageItems[i] = items[i].profile
	}
	page := PublicPage{Items: pageItems}
	if len(items) > limit {
		last := items[limit-1]
		page.Items = pageItems[:limit]
		page.NextCursor = &PublicCursor{Score: last.score, Username: strings.ToLower(last.profile.Username), ID: last.profile.ID}
	}
	if err := loadPublicListSkills(ctx, r.DB, page.Items); err != nil {
		return PublicPage{}, err
	}
	if err := loadEffectivePro(ctx, r.DB, page.Items); err != nil {
		return PublicPage{}, err
	}
	return page, nil
}

func loadEffectivePro(ctx context.Context, database *sql.DB, items []Profile) error {
	if len(items) == 0 {
		return nil
	}
	indexes := map[string]int{}
	ids := make([]string, len(items))
	for i := range items {
		indexes[items[i].UserID] = i
		ids[i] = items[i].UserID
	}
	rows, err := database.QueryContext(ctx, `SELECT s.user_id::text FROM user_subscriptions s JOIN subscription_plans p ON p.id=s.plan_id AND p.active AND p.tier='PRO' JOIN subscription_plan_entitlements e ON e.plan_id=p.id AND e.feature_key='profile.pro_badge' AND e.kind='BOOLEAN' AND e.enabled WHERE s.user_id=ANY($1::uuid[]) AND s.status='ACTIVE' AND s.starts_at<=now() AND s.current_period_end>now() AND EXISTS(SELECT 1 FROM feature_flags WHERE key='pro_subscriptions_enabled' AND enabled) GROUP BY s.user_id`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return err
		}
		if i, ok := indexes[id]; ok {
			items[i].EffectivePro = true
		}
	}
	return rows.Err()
}

func loadPublicListSkills(ctx context.Context, database *sql.DB, items []Profile) error {
	if len(items) == 0 {
		return nil
	}
	indexes := make(map[string]int, len(items))
	ids := make([]string, len(items))
	for index := range items {
		indexes[items[index].UserID] = index
		ids[index] = items[index].UserID
	}
	rows, err := database.QueryContext(ctx, `
WITH ranked AS (
  SELECT ps.user_id, s.id, s.slug, s.name, ps.level, ps.years, ps.is_featured,
         row_number() OVER (PARTITION BY ps.user_id ORDER BY ps.is_featured DESC, s.name) AS rank
  FROM profile_skills ps JOIN skills s ON s.id = ps.skill_id AND s.is_active = true
  WHERE ps.user_id=ANY($1::uuid[])
)
SELECT user_id::text, id::text, slug, name, COALESCE(level, ''), years, is_featured
FROM ranked WHERE rank <= 5 ORDER BY user_id, rank`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var userID string
		var skill Skill
		if err := rows.Scan(&userID, &skill.ID, &skill.Slug, &skill.Name, &skill.Level, &skill.Years, &skill.IsFeatured); err != nil {
			return err
		}
		if index, ok := indexes[userID]; ok {
			items[index].Skills = append(items[index].Skills, skill)
		}
	}
	return rows.Err()
}

func (r PostgresRepository) Current(ctx context.Context, actorID string) (Profile, error) {
	if actorID == "" {
		return Profile{}, ErrUnauthorized
	}
	return r.getByUserID(ctx, actorID)
}

func loadTrustStats(ctx context.Context, database *sql.DB, items []Profile) error {
	if len(items) == 0 {
		return nil
	}
	indexes := make(map[string]int, len(items))
	ids := make([]string, len(items))
	for i := range items {
		indexes[items[i].UserID] = i
		ids[i] = items[i].UserID
	}
	rows, err := database.QueryContext(ctx, `SELECT user_id::text,native_rating::float8,reviews_count,completed_projects_count FROM user_trust_stats WHERE user_id=ANY($1::uuid[])`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var userID string
		var rating *float64
		var reviews, completed int
		if err := rows.Scan(&userID, &rating, &reviews, &completed); err != nil {
			return err
		}
		if i, ok := indexes[userID]; ok {
			items[i].NativeRating = rating
			items[i].ReviewsCount = reviews
			items[i].CompletedProjects = completed
		}
	}
	return rows.Err()
}

func (r PostgresRepository) Update(ctx context.Context, actorID string, input UpdateRequest) (Profile, error) {
	if actorID == "" {
		return Profile{}, ErrUnauthorized
	}
	candidate := Profile{
		UserID: actorID, ProfessionalTitle: stringsTrim(input.ProfessionalTitle), Bio: stringsTrim(input.Bio),
		LocationText: stringsTrim(input.LocationText), CountryCode: strings.ToUpper(stringsTrim(input.CountryCode)),
		ExperienceYears: input.ExperienceYears, HourlyRateKopecks: input.HourlyRateKopecks, MinimumOrderKopecks: input.MinimumOrderKopecks,
		Availability: input.Availability, ProfileVisibility: input.ProfileVisibility,
	}
	if input.Categories != nil {
		candidate.Categories = categoriesFromSelections(input.Categories)
	}
	if input.Skills != nil {
		candidate.Skills = skillsFromSelections(input.Skills)
	}
	if input.CustomSkills != nil {
		candidate.CustomSkills, _ = normalizeCustomSkills(input.CustomSkills)
	}
	if input.Languages != nil {
		candidate.Languages = normalizeLanguages(input.Languages)
	}
	if err := Validate(candidate); err != nil {
		return Profile{}, err
	}

	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Profile{}, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `
INSERT INTO professional_profiles(user_id,professional_title,bio,location_text,country_code,experience_years,hourly_rate_kopecks,minimum_order_kopecks,availability,profile_visibility)
SELECT id,NULLIF($2,''),NULLIF($3,''),NULLIF($4,''),NULLIF($5,''),$6,$7,$8,$9,$10 FROM users WHERE id=$1 AND status='ACTIVE' AND deleted_at IS NULL
ON CONFLICT(user_id) DO UPDATE SET professional_title=EXCLUDED.professional_title,bio=EXCLUDED.bio,location_text=EXCLUDED.location_text,country_code=EXCLUDED.country_code,experience_years=EXCLUDED.experience_years,hourly_rate_kopecks=EXCLUDED.hourly_rate_kopecks,minimum_order_kopecks=EXCLUDED.minimum_order_kopecks,availability=EXCLUDED.availability,profile_visibility=EXCLUDED.profile_visibility,updated_at=now()`, actorID, candidate.ProfessionalTitle, candidate.Bio, candidate.LocationText, candidate.CountryCode,
		candidate.ExperienceYears, candidate.HourlyRateKopecks, candidate.MinimumOrderKopecks, candidate.Availability, candidate.ProfileVisibility)
	if err != nil {
		return Profile{}, mapPostgresError(err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return Profile{}, ErrNotFound
	}
	if input.Categories != nil {
		if err := replaceCategoriesTx(ctx, tx, actorID, input.Categories); err != nil {
			return Profile{}, err
		}
	}
	if input.Skills != nil {
		if err := replaceSkillsTx(ctx, tx, actorID, input.Skills); err != nil {
			return Profile{}, err
		}
	}
	if input.CustomSkills != nil {
		if err := replaceCustomSkillsTx(ctx, tx, actorID, input.CustomSkills); err != nil {
			return Profile{}, err
		}
	}
	if input.Languages != nil {
		if err := replaceLanguagesTx(ctx, tx, actorID, input.Languages); err != nil {
			return Profile{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Profile{}, err
	}
	return r.getByUserID(ctx, actorID)
}

func (r PostgresRepository) ReplaceCategories(ctx context.Context, actorID string, items []CategorySelection) (Profile, error) {
	if actorID == "" {
		return Profile{}, ErrUnauthorized
	}
	if len(items) > 10 {
		return Profile{}, invalid("too many profile categories")
	}
	if err := validateCategories(categoriesFromSelections(items)); err != nil {
		return Profile{}, err
	}
	return r.replace(ctx, actorID, func(tx *sql.Tx) error { return replaceCategoriesTx(ctx, tx, actorID, items) })
}

func (r PostgresRepository) ReplaceSkills(ctx context.Context, actorID string, items []SkillSelection) (Profile, error) {
	if actorID == "" {
		return Profile{}, ErrUnauthorized
	}
	if len(items) > 50 {
		return Profile{}, invalid("too many profile skills")
	}
	if err := validateSkills(skillsFromSelections(items)); err != nil {
		return Profile{}, err
	}
	return r.replace(ctx, actorID, func(tx *sql.Tx) error { return replaceSkillsTx(ctx, tx, actorID, items) })
}

func (r PostgresRepository) ReplaceLanguages(ctx context.Context, actorID string, items []Language) (Profile, error) {
	if actorID == "" {
		return Profile{}, ErrUnauthorized
	}
	if len(items) > 20 {
		return Profile{}, invalid("too many profile languages")
	}
	items = normalizeLanguages(items)
	if err := validateLanguages(items); err != nil {
		return Profile{}, err
	}
	return r.replace(ctx, actorID, func(tx *sql.Tx) error { return replaceLanguagesTx(ctx, tx, actorID, items) })
}

func (r PostgresRepository) replace(ctx context.Context, actorID string, operation func(*sql.Tx) error) (Profile, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Profile{}, err
	}
	defer tx.Rollback()
	var exists bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM professional_profiles WHERE user_id = $1)", actorID).Scan(&exists); err != nil {
		return Profile{}, err
	}
	if !exists {
		return Profile{}, ErrNotFound
	}
	if err := operation(tx); err != nil {
		return Profile{}, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE professional_profiles SET updated_at = now() WHERE user_id = $1", actorID); err != nil {
		return Profile{}, err
	}
	if err := tx.Commit(); err != nil {
		return Profile{}, err
	}
	return r.getByUserID(ctx, actorID)
}

func replaceCategoriesTx(ctx context.Context, tx *sql.Tx, actorID string, items []CategorySelection) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM profile_categories WHERE user_id = $1", actorID); err != nil {
		return err
	}
	for index, item := range items {
		result, err := tx.ExecContext(ctx, `
INSERT INTO profile_categories (user_id, category_id, is_primary, sort_order)
SELECT $1, id, $3, $4 FROM categories WHERE id = $2 AND is_active = true`, actorID, strings.ToLower(strings.TrimSpace(item.ID)), item.IsPrimary, index)
		if err != nil {
			return mapPostgresError(err)
		}
		if rows, _ := result.RowsAffected(); rows != 1 {
			return fmt.Errorf("%w: unknown or inactive category", ErrInvalidReference)
		}
	}
	return nil
}

func replaceSkillsTx(ctx context.Context, tx *sql.Tx, actorID string, items []SkillSelection) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM profile_skills WHERE user_id = $1", actorID); err != nil {
		return err
	}
	for _, item := range items {
		result, err := tx.ExecContext(ctx, `
INSERT INTO profile_skills (user_id, skill_id, level, years, is_featured)
SELECT $1, id, NULLIF($3, ''), $4, $5 FROM skills WHERE id = $2 AND is_active = true`, actorID, strings.ToLower(strings.TrimSpace(item.ID)), item.Level, item.Years, item.IsFeatured)
		if err != nil {
			return mapPostgresError(err)
		}
		if rows, _ := result.RowsAffected(); rows != 1 {
			return fmt.Errorf("%w: unknown or inactive skill", ErrInvalidReference)
		}
	}
	return nil
}

func replaceCustomSkillsTx(ctx context.Context, tx *sql.Tx, actorID string, items []string) error {
	values, err := normalizeCustomSkills(items)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM profile_custom_skills WHERE user_id = $1", actorID); err != nil {
		return err
	}
	for index, value := range values {
		if _, err = tx.ExecContext(ctx, `INSERT INTO profile_custom_skills(user_id,name,sort_order) VALUES($1,$2,$3)`, actorID, value, index); err != nil {
			return mapPostgresError(err)
		}
	}
	return nil
}

func replaceLanguagesTx(ctx context.Context, tx *sql.Tx, actorID string, items []Language) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM profile_languages WHERE user_id = $1", actorID); err != nil {
		return err
	}
	for _, item := range normalizeLanguages(items) {
		if _, err := tx.ExecContext(ctx, "INSERT INTO profile_languages (user_id, language_code, level) VALUES ($1, $2, $3)", actorID, item.Code, item.Level); err != nil {
			return mapPostgresError(err)
		}
	}
	return nil
}

func (r PostgresRepository) getByUserID(ctx context.Context, userID string) (Profile, error) {
	profile, err := scanProfile(r.DB.QueryRowContext(ctx, `
SELECT p.user_id::text, COALESCE(u.username, ''), u.display_name,
       COALESCE(p.professional_title, ''), COALESCE(p.bio, ''), COALESCE(p.location_text, ''),
       COALESCE(p.country_code, ''), p.experience_years, p.hourly_rate_kopecks, p.minimum_order_kopecks,
       p.response_time_minutes,
       p.availability, p.profile_visibility
FROM professional_profiles p JOIN users u ON u.id = p.user_id
WHERE p.user_id = $1 AND u.deleted_at IS NULL`, userID))
	if err != nil {
		return Profile{}, err
	}
	if err := loadRelations(ctx, r.DB, &profile); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

type rowScanner interface{ Scan(...any) error }

func scanProfile(row rowScanner) (Profile, error) {
	var profile Profile
	err := row.Scan(&profile.UserID, &profile.Username, &profile.DisplayName, &profile.ProfessionalTitle, &profile.Bio, &profile.LocationText,
		&profile.CountryCode, &profile.ExperienceYears, &profile.HourlyRateKopecks, &profile.MinimumOrderKopecks,
		&profile.ResponseTimeMinutes, &profile.Availability, &profile.ProfileVisibility)
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	profile.Categories = make([]Category, 0)
	profile.Skills = make([]Skill, 0)
	profile.CustomSkills = make([]string, 0)
	profile.Languages = make([]Language, 0)
	return profile, err
}

func loadRelations(ctx context.Context, database queryer, profile *Profile) error {
	categoryRows, err := database.QueryContext(ctx, `
SELECT c.id::text, c.slug, c.name, pc.is_primary
FROM profile_categories pc JOIN categories c ON c.id = pc.category_id AND c.is_active = true
WHERE pc.user_id = $1 ORDER BY pc.sort_order, c.name`, profile.UserID)
	if err != nil {
		return err
	}
	for categoryRows.Next() {
		var item Category
		if err := categoryRows.Scan(&item.ID, &item.Slug, &item.Name, &item.IsPrimary); err != nil {
			categoryRows.Close()
			return err
		}
		profile.Categories = append(profile.Categories, item)
	}
	if err := categoryRows.Err(); err != nil {
		categoryRows.Close()
		return err
	}
	if err := categoryRows.Close(); err != nil {
		return err
	}

	skillRows, err := database.QueryContext(ctx, `
SELECT s.id::text, s.slug, s.name, COALESCE(ps.level, ''), ps.years, ps.is_featured
FROM profile_skills ps JOIN skills s ON s.id = ps.skill_id AND s.is_active = true
WHERE ps.user_id = $1 ORDER BY ps.is_featured DESC, s.name`, profile.UserID)
	if err != nil {
		return err
	}
	for skillRows.Next() {
		var item Skill
		if err := skillRows.Scan(&item.ID, &item.Slug, &item.Name, &item.Level, &item.Years, &item.IsFeatured); err != nil {
			skillRows.Close()
			return err
		}
		profile.Skills = append(profile.Skills, item)
	}
	if err := skillRows.Err(); err != nil {
		skillRows.Close()
		return err
	}
	if err := skillRows.Close(); err != nil {
		return err
	}

	customSkillRows, err := database.QueryContext(ctx, `SELECT name FROM profile_custom_skills WHERE user_id=$1 ORDER BY sort_order,name`, profile.UserID)
	if err != nil {
		return err
	}
	for customSkillRows.Next() {
		var value string
		if err := customSkillRows.Scan(&value); err != nil {
			customSkillRows.Close()
			return err
		}
		profile.CustomSkills = append(profile.CustomSkills, value)
	}
	if err := customSkillRows.Err(); err != nil {
		customSkillRows.Close()
		return err
	}
	if err := customSkillRows.Close(); err != nil {
		return err
	}

	languageRows, err := database.QueryContext(ctx, "SELECT language_code, level FROM profile_languages WHERE user_id = $1 ORDER BY language_code", profile.UserID)
	if err != nil {
		return err
	}
	defer languageRows.Close()
	for languageRows.Next() {
		var item Language
		if err := languageRows.Scan(&item.Code, &item.Level); err != nil {
			return err
		}
		profile.Languages = append(profile.Languages, item)
	}
	return languageRows.Err()
}

func mapPostgresError(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23503" {
		return fmt.Errorf("%w: unknown profile reference", ErrInvalidReference)
	}
	return err
}

func stringsTrim(value string) string {
	return strings.TrimSpace(value)
}
