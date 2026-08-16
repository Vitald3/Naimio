package auth

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
)

type PresenceHandler struct{ DB *sql.DB }

func (h PresenceHandler) Public(w http.ResponseWriter, r *http.Request) {
	identity := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/v1/presence/"))
	if r.Method != http.MethodGet || identity == "" || len(identity) > 160 || h.DB == nil {
		writePresence(w, false)
		return
	}
	var online bool
	err := h.DB.QueryRowContext(r.Context(), `SELECT EXISTS(
SELECT 1 FROM users u JOIN sessions s ON s.user_id=u.id
WHERE (u.id::text=$1 OR u.username_normalized=lower($1)) AND u.status='ACTIVE' AND u.deleted_at IS NULL
AND s.revoked_at IS NULL AND s.expires_at>now() AND s.last_used_at>=now()-interval '90 seconds')`, identity).Scan(&online)
	if err != nil {
		online = false
	}
	writePresence(w, online)
}

func (h PresenceHandler) Batch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h.DB == nil {
		http.Error(w, "presence unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var input struct {
		IDs []string `json:"ids"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || len(input.IDs) == 0 || len(input.IDs) > 100 {
		http.Error(w, "invalid presence batch", http.StatusBadRequest)
		return
	}
	unique := make([]string, 0, len(input.IDs))
	seen := make(map[string]struct{}, len(input.IDs))
	for _, raw := range input.IDs {
		id := strings.TrimSpace(raw)
		if id == "" || len(id) > 160 || strings.ContainsAny(id, "\r\n") {
			http.Error(w, "invalid presence batch", http.StatusBadRequest)
			return
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	// Bind the whole list as JSON and expand it inside PostgreSQL. This avoids
	// dynamic placeholders and the text/varchar inference problems that caused
	// /presence/batch to surface as 502 through nginx on some pgx versions.
	rawIDs, marshalErr := json.Marshal(unique)
	if marshalErr != nil {
		writePresenceBatch(w, unique, nil)
		return
	}
	rows, err := h.DB.QueryContext(r.Context(), `WITH requested(identity) AS (
  SELECT value FROM jsonb_array_elements_text($1::jsonb)
)
SELECT r.identity, EXISTS(
  SELECT 1 FROM users u JOIN sessions s ON s.user_id=u.id
  WHERE (u.id::text=r.identity OR u.username_normalized=lower(r.identity))
    AND u.status='ACTIVE' AND u.deleted_at IS NULL
    AND s.revoked_at IS NULL AND s.expires_at>now()
    AND s.last_used_at>=now()-interval '90 seconds'
) FROM requested r`, string(rawIDs))
	if err != nil {
		// Presence is an auxiliary UI signal. A transient database/read failure must
		// never take catalog cards down or bubble through nginx as a gateway error.
		// Fail closed (everyone offline) while keeping the endpoint shape stable.
		writePresenceBatch(w, unique, nil)
		return
	}
	defer rows.Close()
	result := make(map[string]bool, len(unique))
	for rows.Next() {
		var id string
		var online bool
		if err := rows.Scan(&id, &online); err != nil {
			writePresenceBatch(w, unique, nil)
			return
		}
		result[id] = online
	}
	if err := rows.Err(); err != nil {
		writePresenceBatch(w, unique, nil)
		return
	}
	writePresenceBatch(w, unique, result)
}

func writePresenceBatch(w http.ResponseWriter, ids []string, values map[string]bool) {
	result := make(map[string]bool, len(ids))
	for _, id := range ids {
		result[id] = values != nil && values[id]
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": result})
}

func (h PresenceHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writePresence(w http.ResponseWriter, online bool) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]bool{"online": online}})
}
