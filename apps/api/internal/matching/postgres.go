package matching

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"freelance/apps/api/internal/platform/requestmeta"
	"strings"
	"time"
)

type PostgresRepository struct{ DB *sql.DB }

func (r PostgresRepository) OwnedProject(ctx context.Context, actor, id string) (Project, error) {
	var p Project
	var category sql.NullString
	var minBudget, maxBudget sql.NullInt64
	err := r.DB.QueryRowContext(ctx, `SELECT id::text,customer_user_id::text,title,description,category_id::text,budget_min_kopecks,budget_max_kopecks,deadline_at,status FROM projects WHERE id=$1 AND customer_user_id=$2 AND deleted_at IS NULL`, id, actor).Scan(&p.ID, &p.CustomerID, &p.Title, &p.Description, &category, &minBudget, &maxBudget, &p.DeadlineAt, &p.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	if err != nil {
		return Project{}, err
	}
	p.CategoryID = category.String
	if minBudget.Valid {
		p.BudgetMin = &minBudget.Int64
	}
	if maxBudget.Valid {
		p.BudgetMax = &maxBudget.Int64
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT skill_id::text FROM project_skills WHERE project_id=$1 ORDER BY importance DESC,skill_id`, id)
	if err != nil {
		return Project{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var skill string
		if err := rows.Scan(&skill); err != nil {
			return Project{}, err
		}
		p.SkillIDs = append(p.SkillIDs, skill)
	}
	return p, rows.Err()
}
func (r PostgresRepository) Retrieve(ctx context.Context, p Project, c Constraints, limit int) ([]Candidate, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT u.id::text,COALESCE(u.username,''),u.display_name,COALESCE(pp.professional_title,''),pp.availability,COALESCE(primary_category.category_id::text,''),pp.hourly_rate_kopecks,pp.minimum_order_kopecks,pp.response_time_minutes,pp.profile_completion,
COALESCE(skill_stats.matches,0),COALESCE(skill_stats.featured,0),$3::int,COALESCE(portfolio_stats.relevant,0),COALESCE(history.similar,0),COALESCE(ts.reviews_count,0),COALESCE(ts.completed_projects_count,0),COALESCE(external.verified,0),COALESCE(repeat_work.completed,0),ts.native_rating,ts.completion_rate,ts.on_time_rate,ts.recommendation_rate
FROM users u JOIN user_capabilities uc ON uc.user_id=u.id AND uc.capability='FREELANCER' JOIN professional_profiles pp ON pp.user_id=u.id AND pp.profile_visibility='PUBLIC'
LEFT JOIN LATERAL(SELECT category_id FROM profile_categories pc WHERE pc.user_id=u.id ORDER BY(pc.category_id=$4::uuid)DESC,pc.is_primary DESC,pc.sort_order LIMIT 1)primary_category ON true
LEFT JOIN LATERAL(SELECT count(*)::int matches,count(*)FILTER(WHERE ps.is_featured OR ps.level IN('ADVANCED','EXPERT'))::int featured FROM profile_skills ps WHERE ps.user_id=u.id AND ps.skill_id=ANY($2::uuid[]))skill_stats ON true
LEFT JOIN LATERAL(SELECT count(DISTINCT pi.id)::int relevant FROM portfolio_items pi LEFT JOIN portfolio_categories pic ON pic.portfolio_item_id=pi.id LEFT JOIN portfolio_skills pis ON pis.portfolio_item_id=pi.id WHERE pi.user_id=u.id AND pi.visibility='PUBLIC' AND pi.deleted_at IS NULL AND(pic.category_id=$4::uuid OR pis.skill_id=ANY($2::uuid[])))portfolio_stats ON true
LEFT JOIN LATERAL(SELECT count(DISTINCT oldp.id)::int similar FROM project_assignments pa JOIN projects oldp ON oldp.id=pa.project_id WHERE pa.freelancer_user_id=u.id AND pa.status='COMPLETED' AND oldp.status='COMPLETED' AND($4::uuid IS NULL OR oldp.category_id=$4::uuid))history ON true
LEFT JOIN user_trust_stats ts ON ts.user_id=u.id
LEFT JOIN LATERAL(SELECT count(*)::int verified FROM external_reputations er WHERE er.user_id=u.id AND er.verification_status='VERIFIED' AND(er.expires_at IS NULL OR er.expires_at>now()))external ON true
LEFT JOIN LATERAL(SELECT count(DISTINCT rp.id)::int completed FROM project_assignments ra JOIN projects rp ON rp.id=ra.project_id WHERE ra.freelancer_user_id=u.id AND ra.status='COMPLETED' AND rp.customer_user_id=$5)repeat_work ON true
WHERE u.status='ACTIVE' AND u.deleted_at IS NULL AND u.id<>$5 AND(NOT $6 OR pp.availability='AVAILABLE') AND(NOT $7 OR primary_category.category_id=$4::uuid) AND($8::bigint IS NULL OR pp.minimum_order_kopecks IS NULL OR pp.minimum_order_kopecks<=$8) AND(cardinality($2::uuid[])=0 OR COALESCE(skill_stats.matches,0)>0) AND NOT EXISTS(SELECT 1 FROM project_assignments conflict WHERE conflict.project_id=$1 AND conflict.freelancer_user_id=u.id AND conflict.status='ACTIVE')
ORDER BY COALESCE(skill_stats.matches,0)DESC,(primary_category.category_id=$4::uuid)DESC,ts.native_rating DESC NULLS LAST,pp.profile_completion DESC,u.id LIMIT $9`, p.ID, p.SkillIDs, len(p.SkillIDs), nullUUID(p.CategoryID), p.CustomerID, c.RequireImmediateAvailability, c.RequireCategoryMatch, c.MaxMinimumOrderKopecks, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Candidate{}
	for rows.Next() {
		var v Candidate
		if err := rows.Scan(&v.UserID, &v.Username, &v.DisplayName, &v.ProfessionalTitle, &v.Availability, &v.PrimaryCategoryID, &v.HourlyRate, &v.MinimumOrder, &v.ResponseMinutes, &v.ProfileCompletion, &v.SkillMatches, &v.FeaturedSkillMatches, &v.RequiredSkills, &v.RelevantPortfolio, &v.SimilarCompleted, &v.Reviews, &v.CompletedProjects, &v.VerifiedExternal, &v.RepeatProjects, &v.NativeRating, &v.CompletionRate, &v.OnTimeRate, &v.RecommendationRate); err != nil {
			return nil, err
		}
		items = append(items, v)
	}
	return items, rows.Err()
}
func (r PostgresRepository) SaveRun(ctx context.Context, actor string, p Project, constraints Constraints, recommendations []Recommendation, aiUsed bool) (Run, error) {
	id, err := uuidV7()
	if err != nil {
		return Run{}, err
	}
	encoded, _ := json.Marshal(constraints)
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO matching_runs(id,project_id,requested_by_user_id,algorithm_version,ai_used,candidate_count,constraints)SELECT $1,p.id,$2,$3,$4,$5,$6 FROM projects p WHERE p.id=$7 AND p.customer_user_id=$2 AND p.deleted_at IS NULL`, id, actor, AlgorithmVersion, aiUsed, len(recommendations), encoded, p.ID)
	if err != nil {
		return Run{}, err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return Run{}, ErrNotFound
	}
	for _, item := range recommendations {
		reasons, _ := json.Marshal(item.Reasons)
		if _, err := tx.ExecContext(ctx, `INSERT INTO matching_candidates(matching_run_id,freelancer_user_id,deterministic_score,final_score,rank,reasons)VALUES($1,$2,$3,$4,$5,$6)`, id, item.FreelancerID, item.DeterministicScore, item.Score, item.Rank, reasons); err != nil {
			return Run{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Run{}, err
	}
	return r.Run(ctx, actor, p.ID, id)
}
func (r PostgresRepository) Run(ctx context.Context, actor, projectID, runID string) (Run, error) {
	var run Run
	err := r.DB.QueryRowContext(ctx, `SELECT mr.id::text,mr.project_id::text,mr.algorithm_version,mr.ai_used,mr.candidate_count,mr.created_at FROM matching_runs mr JOIN projects p ON p.id=mr.project_id WHERE mr.id=$1 AND mr.project_id=$2 AND p.customer_user_id=$3 AND p.deleted_at IS NULL`, runID, projectID, actor).Scan(&run.ID, &run.ProjectID, &run.AlgorithmVersion, &run.AIUsed, &run.CandidateCount, &run.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	if err != nil {
		return Run{}, err
	}
	recommendations, err := r.recommendations(ctx, projectID, runID)
	if err != nil {
		return Run{}, err
	}
	run.Recommendations = recommendations
	return run, nil
}
func (r PostgresRepository) Latest(ctx context.Context, actor, projectID string) (Run, error) {
	var id string
	err := r.DB.QueryRowContext(ctx, `SELECT mr.id::text FROM matching_runs mr JOIN projects p ON p.id=mr.project_id WHERE mr.project_id=$1 AND p.customer_user_id=$2 AND p.deleted_at IS NULL ORDER BY mr.created_at DESC,mr.id DESC LIMIT 1`, projectID, actor).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	if err != nil {
		return Run{}, err
	}
	return r.Run(ctx, actor, projectID, id)
}
func (r PostgresRepository) recommendations(ctx context.Context, projectID, runID string) ([]Recommendation, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT mc.freelancer_user_id::text,COALESCE(u.username,''),u.display_name,COALESCE(pp.professional_title,''),pp.availability,pp.hourly_rate_kopecks,mc.final_score,mc.rank,mc.reasons,manual.freelancer_user_id IS NOT NULL FROM matching_candidates mc JOIN users u ON u.id=mc.freelancer_user_id JOIN professional_profiles pp ON pp.user_id=u.id LEFT JOIN manual_project_recommendations manual ON manual.project_id=$2 AND manual.freelancer_user_id=u.id WHERE mc.matching_run_id=$1 ORDER BY(manual.freelancer_user_id IS NOT NULL)DESC,mc.rank LIMIT 50`, runID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Recommendation{}
	seen := map[string]bool{}
	for rows.Next() {
		var item Recommendation
		var reasons []byte
		if err := rows.Scan(&item.FreelancerID, &item.Username, &item.DisplayName, &item.ProfessionalTitle, &item.Availability, &item.HourlyRate, &item.Score, &item.Rank, &reasons, &item.Manual); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(reasons, &item.Reasons)
		if item.Manual {
			item.Reasons = prependManual(item.Reasons)
		}
		items = append(items, item)
		seen[item.FreelancerID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	manualRows, err := r.DB.QueryContext(ctx, `SELECT m.freelancer_user_id::text,COALESCE(u.username,''),u.display_name,COALESCE(pp.professional_title,''),pp.availability,pp.hourly_rate_kopecks FROM manual_project_recommendations m JOIN users u ON u.id=m.freelancer_user_id AND u.status='ACTIVE' JOIN professional_profiles pp ON pp.user_id=u.id AND pp.profile_visibility='PUBLIC' WHERE m.project_id=$1 ORDER BY m.created_at DESC LIMIT 20`, projectID)
	if err != nil {
		return nil, err
	}
	defer manualRows.Close()
	manualItems := []Recommendation{}
	for manualRows.Next() {
		var item Recommendation
		if err := manualRows.Scan(&item.FreelancerID, &item.Username, &item.DisplayName, &item.ProfessionalTitle, &item.Availability, &item.HourlyRate); err != nil {
			return nil, err
		}
		if !seen[item.FreelancerID] {
			item.Manual = true
			item.Score = 0
			item.Reasons = prependManual(nil)
			manualItems = append(manualItems, item)
		}
	}
	items = append(manualItems, items...)
	for i := range items {
		items[i].Rank = i + 1
	}
	return items, manualRows.Err()
}
func (r PostgresRepository) ManualPut(ctx context.Context, admin, projectID, freelancerID, reason string) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := requireAdmin(ctx, tx, admin); err != nil {
		return err
	}
	var eligible bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM projects p JOIN users u ON u.id=$2 AND u.status='ACTIVE' AND u.deleted_at IS NULL JOIN user_capabilities uc ON uc.user_id=u.id AND uc.capability='FREELANCER' JOIN professional_profiles pp ON pp.user_id=u.id AND pp.profile_visibility='PUBLIC' WHERE p.id=$1 AND p.deleted_at IS NULL AND p.customer_user_id<>u.id)`, projectID, freelancerID).Scan(&eligible); err != nil {
		return err
	}
	if !eligible {
		return ErrNotFound
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO manual_project_recommendations(project_id,freelancer_user_id,recommended_by_user_id,internal_reason)VALUES($1,$2,$3,$4)ON CONFLICT(project_id,freelancer_user_id)DO UPDATE SET recommended_by_user_id=EXCLUDED.recommended_by_user_id,internal_reason=EXCLUDED.internal_reason,created_at=now()`, projectID, freelancerID, admin, reason)
	if err != nil {
		return err
	}
	if err = audit(ctx, tx, admin, "MATCHING_MANUAL_RECOMMENDATION_SET", projectID, map[string]any{"freelancer_user_id": freelancerID}); err != nil {
		return err
	}
	return tx.Commit()
}
func (r PostgresRepository) ManualDelete(ctx context.Context, admin, projectID, freelancerID string) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := requireAdmin(ctx, tx, admin); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM manual_project_recommendations WHERE project_id=$1 AND freelancer_user_id=$2`, projectID, freelancerID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return ErrNotFound
	}
	if err = audit(ctx, tx, admin, "MATCHING_MANUAL_RECOMMENDATION_REMOVED", projectID, map[string]any{"freelancer_user_id": freelancerID}); err != nil {
		return err
	}
	return tx.Commit()
}
func (r PostgresRepository) RecordEvent(ctx context.Context, actor, projectID, runID, freelancerID, eventType, key string) error {
	id, err := uuidV7()
	if err != nil {
		return err
	}
	result, err := r.DB.ExecContext(ctx, `INSERT INTO matching_quality_events(id,matching_run_id,project_id,freelancer_user_id,actor_user_id,event_type,event_key)SELECT $1,mr.id,mr.project_id,$4,$5,$6,$7 FROM matching_runs mr JOIN projects p ON p.id=mr.project_id JOIN matching_candidates mc ON mc.matching_run_id=mr.id AND mc.freelancer_user_id=$4 WHERE mr.id=$2 AND mr.project_id=$3 AND p.customer_user_id=$5 ON CONFLICT(actor_user_id,event_key)DO NOTHING`, id, runID, projectID, freelancerID, actor, eventType, eventType+":"+key)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		var exists bool
		if err := r.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM matching_quality_events WHERE actor_user_id=$1 AND event_key=$2)`, actor, eventType+":"+key).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}
func (r PostgresRepository) Metrics(ctx context.Context, admin string) (Metrics, error) {
	if err := requireAdmin(ctx, r.DB, admin); err != nil {
		return Metrics{}, err
	}
	var m Metrics
	err := r.DB.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(candidate_count),0),count(*)FILTER(WHERE ai_used),COALESCE(avg(candidate_count),0) FROM matching_runs WHERE created_at>=now()-interval '30 days'`).Scan(&m.Runs, &m.Candidates, &m.AIUsed, &m.AverageCandidates)
	if err != nil {
		return Metrics{}, err
	}
	err = r.DB.QueryRowContext(ctx, `SELECT count(*)FILTER(WHERE event_type='IMPRESSION'),count(*)FILTER(WHERE event_type='PROFILE_OPEN'),count(*)FILTER(WHERE event_type='INVITE'),count(*)FILTER(WHERE event_type='SHORTLIST'),count(*)FILTER(WHERE event_type='PROPOSAL'),count(*)FILTER(WHERE event_type='ACCEPTANCE'),count(*)FILTER(WHERE event_type='COMPLETED'),count(*)FILTER(WHERE event_type='REPEAT') FROM matching_quality_events WHERE created_at>=now()-interval '30 days'`).Scan(&m.Impressions, &m.ProfileOpens, &m.Invites, &m.Shortlists, &m.Proposals, &m.Acceptances, &m.Completed, &m.Repeats)
	if err != nil {
		return Metrics{}, err
	}
	err = r.DB.QueryRowContext(ctx, `SELECT count(*),count(*)FILTER(WHERE status<>'SUCCEEDED'),COALESCE(sum(cost_microunits),0),COALESCE(percentile_cont(0.95)WITHIN GROUP(ORDER BY latency_ms),0) FROM ai_requests WHERE created_at>=now()-interval '30 days'`).Scan(&m.AIRequests, &m.AIFailures, &m.AICostMicrounits, &m.AIP95LatencyMS)
	return m, err
}
func requireAdmin(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, actor string) error {
	var ok bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN user_roles ur ON ur.user_id=u.id WHERE u.id=$1 AND u.status='ACTIVE' AND ur.role IN('ADMIN','SUPER_ADMIN'))`, actor).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return nil
}
func audit(ctx context.Context, tx *sql.Tx, actor, action, target string, metadata map[string]any) error {
	id, err := uuidV7()
	if err != nil {
		return err
	}
	encoded, _ := json.Marshal(metadata)
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_logs(id,actor_user_id,action,target_type,target_id,metadata,ip)VALUES($1,$2,$3,'PROJECT',$4,$5,NULLIF($6,'')::inet)`, id, actor, action, target, encoded, requestmeta.FromContext(ctx))
	return err
}
func prependManual(reasons []Reason) []Reason {
	return append([]Reason{{Code: "PLATFORM_RECOMMENDED", Label: "Рекомендовано платформой"}}, reasons...)
}
func nullUUID(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
func uuidV7() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	milliseconds := uint64(time.Now().UTC().UnixMilli())
	for i := 5; i >= 0; i-- {
		b[i] = byte(milliseconds)
		milliseconds >>= 8
	}
	b[6] = (b[6] & 0x0f) | 0x70
	b[8] = (b[8] & 0x3f) | 0x80
	value := hex.EncodeToString(b)
	return value[:8] + "-" + value[8:12] + "-" + value[12:16] + "-" + value[16:20] + "-" + value[20:], nil
}
