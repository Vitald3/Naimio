package ai

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

type MemoryRepository struct {
	mu          sync.Mutex
	Drafts      map[string]Draft
	TokenHashes map[[32]byte]string
	Metrics     []RequestMetric
	Categories  []CategoryCandidate
	Skills      []SkillCandidate
	Now         func() time.Time
}

func (r *MemoryRepository) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}
func (r *MemoryRepository) Create(_ context.Context, actor, source string, raw map[string]any) (Draft, string, error) {
	id, err := uuidV7()
	if err != nil {
		return Draft{}, "", err
	}
	token, err := randomHex(32)
	if err != nil {
		return Draft{}, "", err
	}
	now := r.now()
	draft := Draft{ID: id, OwnerUserID: actor, SourceType: source, RawInput: copyMap(raw), NormalizedData: map[string]any{}, ExpiresAt: now.Add(7 * 24 * time.Hour), CreatedAt: now, UpdatedAt: now}
	hash := sha256.Sum256([]byte(token))
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Drafts == nil {
		r.Drafts = map[string]Draft{}
	}
	if r.TokenHashes == nil {
		r.TokenHashes = map[[32]byte]string{}
	}
	r.Drafts[id] = draft
	r.TokenHashes[hash] = id
	return draft, token, nil
}
func (r *MemoryRepository) Get(_ context.Context, actor, token string) (Draft, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	draft, ok := r.byAccess(actor, token)
	if !ok {
		return Draft{}, ErrNotFound
	}
	return draft, nil
}
func (r *MemoryRepository) Update(_ context.Context, actor, token string, raw, normalized map[string]any) (Draft, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	draft, ok := r.byAccess(actor, token)
	if !ok {
		return Draft{}, ErrNotFound
	}
	if raw != nil {
		draft.RawInput = copyMap(raw)
	}
	if normalized != nil {
		draft.NormalizedData = copyMap(normalized)
	}
	draft.UpdatedAt = r.now()
	r.Drafts[draft.ID] = draft
	return draft, nil
}
func (r *MemoryRepository) Claim(_ context.Context, actor, token string) (Draft, error) {
	if actor == "" {
		return Draft{}, ErrForbidden
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	hash := sha256.Sum256([]byte(token))
	id, ok := r.TokenHashes[hash]
	if !ok {
		return Draft{}, ErrNotFound
	}
	draft := r.Drafts[id]
	if !draft.ExpiresAt.After(r.now()) {
		return Draft{}, ErrNotFound
	}
	if draft.OwnerUserID != "" && draft.OwnerUserID != actor {
		return Draft{}, ErrNotFound
	}
	draft.OwnerUserID = actor
	draft.UpdatedAt = r.now()
	r.Drafts[id] = draft
	return draft, nil
}
func (r *MemoryRepository) byAccess(actor, token string) (Draft, bool) {
	hash := sha256.Sum256([]byte(token))
	id, ok := r.TokenHashes[hash]
	if !ok {
		return Draft{}, false
	}
	draft, ok := r.Drafts[id]
	if !ok || !draft.ExpiresAt.After(r.now()) || (draft.OwnerUserID != "" && draft.OwnerUserID != actor) {
		return Draft{}, false
	}
	return draft, true
}
func (r *MemoryRepository) Record(_ context.Context, m RequestMetric) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Metrics = append(r.Metrics, m)
	return nil
}
func (r *MemoryRepository) Candidates(context.Context) ([]CategoryCandidate, []SkillCandidate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]CategoryCandidate(nil), r.Categories...), append([]SkillCandidate(nil), r.Skills...), nil
}

type PostgresRepository struct{ DB *sql.DB }

func (r PostgresRepository) Create(ctx context.Context, actor, source string, raw map[string]any) (Draft, string, error) {
	id, err := uuidV7()
	if err != nil {
		return Draft{}, "", err
	}
	token, err := randomHex(32)
	if err != nil {
		return Draft{}, "", err
	}
	hash := sha256.Sum256([]byte(token))
	rawJSON, _ := json.Marshal(raw)
	var draft Draft
	var owner sql.NullString
	err = r.DB.QueryRowContext(ctx, `INSERT INTO project_drafts(id,owner_user_id,guest_token_hash,source_type,raw_input,expires_at) VALUES($1,NULLIF($2,'')::uuid,$3,$4,$5,now()+interval '7 days') RETURNING id::text,owner_user_id::text,source_type,raw_input,normalized_data,expires_at,created_at,updated_at`, id, actor, hash[:], source, rawJSON).Scan(&draft.ID, &owner, &draft.SourceType, &rawJSON, new([]byte), &draft.ExpiresAt, &draft.CreatedAt, &draft.UpdatedAt)
	if err != nil {
		return Draft{}, "", err
	}
	draft.OwnerUserID = owner.String
	_ = json.Unmarshal(rawJSON, &draft.RawInput)
	draft.NormalizedData = map[string]any{}
	return draft, token, nil
}
func (r PostgresRepository) Get(ctx context.Context, actor, token string) (Draft, error) {
	return r.get(ctx, actor, token)
}
func (r PostgresRepository) Update(ctx context.Context, actor, token string, raw, normalized map[string]any) (Draft, error) {
	draft, err := r.get(ctx, actor, token)
	if err != nil {
		return Draft{}, err
	}
	if raw == nil {
		raw = draft.RawInput
	}
	if normalized == nil {
		normalized = draft.NormalizedData
	}
	rawJSON, _ := json.Marshal(raw)
	normalizedJSON, _ := json.Marshal(normalized)
	result, err := r.DB.ExecContext(ctx, `UPDATE project_drafts SET raw_input=$2,normalized_data=$3,updated_at=now() WHERE id=$1`, draft.ID, rawJSON, normalizedJSON)
	if err != nil {
		return Draft{}, err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return Draft{}, ErrNotFound
	}
	return r.get(ctx, actor, token)
}
func (r PostgresRepository) Claim(ctx context.Context, actor, token string) (Draft, error) {
	if actor == "" {
		return Draft{}, ErrForbidden
	}
	hash := sha256.Sum256([]byte(token))
	result, err := r.DB.ExecContext(ctx, `UPDATE project_drafts SET owner_user_id=$2,updated_at=now() WHERE guest_token_hash=$1 AND expires_at>now() AND (owner_user_id IS NULL OR owner_user_id=$2)`, hash[:], actor)
	if err != nil {
		return Draft{}, err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return Draft{}, ErrNotFound
	}
	return r.get(ctx, actor, token)
}
func (r PostgresRepository) get(ctx context.Context, actor, token string) (Draft, error) {
	hash := sha256.Sum256([]byte(token))
	var draft Draft
	var owner sql.NullString
	var rawJSON, normalizedJSON []byte
	err := r.DB.QueryRowContext(ctx, `SELECT id::text,owner_user_id::text,source_type,raw_input,normalized_data,expires_at,created_at,updated_at FROM project_drafts WHERE guest_token_hash=$1 AND expires_at>now() AND (owner_user_id IS NULL OR owner_user_id=NULLIF($2,'')::uuid)`, hash[:], actor).Scan(&draft.ID, &owner, &draft.SourceType, &rawJSON, &normalizedJSON, &draft.ExpiresAt, &draft.CreatedAt, &draft.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Draft{}, ErrNotFound
	}
	if err != nil {
		return Draft{}, err
	}
	draft.OwnerUserID = owner.String
	_ = json.Unmarshal(rawJSON, &draft.RawInput)
	_ = json.Unmarshal(normalizedJSON, &draft.NormalizedData)
	return draft, nil
}
func (r PostgresRepository) Record(ctx context.Context, m RequestMetric) error {
	id, err := uuidV7()
	if err != nil {
		return err
	}
	var user any
	if m.UserID != "" {
		user = m.UserID
	}
	_, err = r.DB.ExecContext(ctx, `INSERT INTO ai_requests(id,user_id,capability,provider,model,status,input_tokens,output_tokens,cost_microunits,latency_ms,error_code)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,''))`, id, user, string(m.Capability), m.Provider, m.Model, m.Status, m.InputTokens, m.OutputTokens, m.CostMicrounits, m.Latency.Milliseconds(), m.ErrorCode)
	return err
}
func (r PostgresRepository) Candidates(ctx context.Context) ([]CategoryCandidate, []SkillCandidate, error) {
	categoryRows, err := r.DB.QueryContext(ctx, `SELECT id::text,slug,name FROM categories WHERE is_active=true ORDER BY sort_order,name LIMIT 1000`)
	if err != nil {
		return nil, nil, err
	}
	defer categoryRows.Close()
	categories := []CategoryCandidate{}
	for categoryRows.Next() {
		var item CategoryCandidate
		if err := categoryRows.Scan(&item.ID, &item.Slug, &item.Name); err != nil {
			return nil, nil, err
		}
		categories = append(categories, item)
	}
	if err := categoryRows.Err(); err != nil {
		return nil, nil, err
	}
	skillRows, err := r.DB.QueryContext(ctx, `SELECT id::text,slug,name FROM skills WHERE is_active=true ORDER BY name LIMIT 2000`)
	if err != nil {
		return nil, nil, err
	}
	defer skillRows.Close()
	skills := []SkillCandidate{}
	for skillRows.Next() {
		var item SkillCandidate
		if err := skillRows.Scan(&item.ID, &item.Slug, &item.Name); err != nil {
			return nil, nil, err
		}
		skills = append(skills, item)
	}
	return categories, skills, skillRows.Err()
}

func randomHex(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
func uuidV7() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	milliseconds := uint64(time.Now().UTC().UnixMilli())
	for index := 5; index >= 0; index-- {
		b[index] = byte(milliseconds)
		milliseconds >>= 8
	}
	b[6] = (b[6] & 0x0f) | 0x70
	b[8] = (b[8] & 0x3f) | 0x80
	value := hex.EncodeToString(b)
	return value[:8] + "-" + value[8:12] + "-" + value[12:16] + "-" + value[16:20] + "-" + value[20:], nil
}
func copyMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	encoded, _ := json.Marshal(value)
	out := map[string]any{}
	_ = json.Unmarshal(encoded, &out)
	return out
}
