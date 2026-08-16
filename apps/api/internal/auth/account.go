package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
)

type mePatchInput struct {
	DisplayName         *string `json:"display_name"`
	Locale              *string `json:"locale"`
	Timezone            *string `json:"timezone"`
	AvatarMediaObjectID *string `json:"avatar_media_object_id"`
	Gender              *string `json:"gender"`
}

// Me exposes the authenticated account boundary used by settings/onboarding.
// Professional marketplace profile data remains in the profiles domain.
func (h Handler) Me(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r.Context())
	if !ok || h.DB == nil {
		authProblem(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		data, err := h.currentUser(r.Context(), actor)
		if err != nil {
			authProblem(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
			return
		}
		authJSON(w, http.StatusOK, map[string]any{"data": data})
	case http.MethodPatch:
		var in mePatchInput
		if !authDecode(w, r, &in) {
			return
		}
		if in.DisplayName == nil && in.Locale == nil && in.Timezone == nil && in.AvatarMediaObjectID == nil && in.Gender == nil {
			authProblem(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "no account fields supplied")
			return
		}
		var display, locale, timezone, gender any
		if in.DisplayName != nil {
			v := strings.TrimSpace(*in.DisplayName)
			if len([]rune(v)) < 1 || len([]rune(v)) > 120 {
				authProblem(w, r, 422, "VALIDATION_ERROR", "invalid display name")
				return
			}
			display = v
		}
		if in.Locale != nil {
			v := strings.TrimSpace(*in.Locale)
			if len(v) < 2 || len(v) > 10 {
				authProblem(w, r, 422, "VALIDATION_ERROR", "invalid locale")
				return
			}
			locale = v
		}
		if in.Timezone != nil {
			v := strings.TrimSpace(*in.Timezone)
			if len(v) < 1 || len(v) > 64 {
				authProblem(w, r, 422, "VALIDATION_ERROR", "invalid timezone")
				return
			}
			timezone = v
		}
		if in.Gender != nil {
			v := strings.ToUpper(strings.TrimSpace(*in.Gender))
			if v != "MALE" && v != "FEMALE" && v != "UNSPECIFIED" {
				authProblem(w, r, 422, "VALIDATION_ERROR", "invalid gender")
				return
			}
			gender = v
		}
		var avatar any
		var oldAvatarID sql.NullString
		if in.AvatarMediaObjectID != nil {
			value := strings.TrimSpace(*in.AvatarMediaObjectID)
			if value == "" {
				avatar = ""
			} else {
				var valid bool
				if err := h.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM media_objects WHERE id=$1 AND owner_user_id=$2 AND purpose='AVATAR' AND scan_status='CLEAN' AND uploaded_at IS NOT NULL AND deleted_at IS NULL)`, value, actor).Scan(&valid); err != nil || !valid {
					authProblem(w, r, 422, "VALIDATION_ERROR", "invalid avatar upload")
					return
				}
				avatar = value
			}
			_ = h.DB.QueryRowContext(r.Context(), `SELECT avatar_media_object_id FROM users WHERE id=$1`, actor).Scan(&oldAvatarID)
		}
		_, err := h.DB.ExecContext(r.Context(), `UPDATE users SET display_name=COALESCE($2,display_name),locale=COALESCE($3,locale),timezone=COALESCE($4,timezone),avatar_media_object_id=CASE WHEN $5::text IS NULL THEN avatar_media_object_id WHEN $5='' THEN NULL ELSE $5::uuid END,gender=COALESCE($6,gender),updated_at=now() WHERE id=$1 AND status='ACTIVE' AND deleted_at IS NULL`, actor, display, locale, timezone, avatar, gender)
		if err != nil {
			authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
			return
		}
		if in.AvatarMediaObjectID != nil && oldAvatarID.Valid && strings.TrimSpace(oldAvatarID.String) != "" {
			newAvatarStr, _ := avatar.(string)
			if oldAvatarID.String != newAvatarStr && h.AvatarDeleter != nil {
				_ = h.AvatarDeleter.Delete(r.Context(), actor, oldAvatarID.String)
			}
		}
		data, err := h.currentUser(r.Context(), actor)
		if err != nil {
			authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
			return
		}
		authJSON(w, http.StatusOK, map[string]any{"data": data})
	case http.MethodDelete:
		tx, err := h.DB.BeginTx(r.Context(), nil)
		if err != nil {
			authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
			return
		}
		defer tx.Rollback()
		if _, err = tx.ExecContext(r.Context(), `UPDATE sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1 AND revoked_at IS NULL`, actor); err != nil {
			authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
			return
		}
		if _, err = tx.ExecContext(r.Context(), `UPDATE users SET status='DELETED',deleted_at=COALESCE(deleted_at,now()),updated_at=now() WHERE id=$1 AND deleted_at IS NULL`, actor); err != nil {
			authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
			return
		}
		if err = tx.Commit(); err != nil {
			authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
			return
		}
		h.clearCookie(w)
		w.WriteHeader(http.StatusNoContent)
	default:
		authProblem(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (h Handler) Capability(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		authProblem(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	actor, ok := ActorID(r.Context())
	if !ok || h.DB == nil {
		authProblem(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	name := strings.ToUpper(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/me/capabilities/"), "/"))
	if name != "CUSTOMER" && name != "FREELANCER" {
		authProblem(w, r, 404, "NOT_FOUND", "capability not found")
		return
	}
	if _, err := h.DB.ExecContext(r.Context(), `INSERT INTO user_capabilities(user_id,capability) SELECT id,$2 FROM users WHERE id=$1 AND status='ACTIVE' AND deleted_at IS NULL ON CONFLICT(user_id,capability) DO NOTHING`, actor, name); err != nil {
		authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
		return
	}
	data, err := h.currentUser(r.Context(), actor)
	if err != nil {
		authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
		return
	}
	authJSON(w, http.StatusOK, map[string]any{"data": data})
}

func (h Handler) currentUser(ctx context.Context, actor string) (map[string]any, error) {
	var id, email, name, username, locale, timezone, status, avatarID, gender string
	var verified *time.Time
	err := h.DB.QueryRowContext(ctx, `SELECT id::text,email,display_name,COALESCE(username,''),locale,timezone,status,email_verified_at,COALESCE(avatar_media_object_id::text,''),gender FROM users WHERE id=$1 AND deleted_at IS NULL`, actor).Scan(&id, &email, &name, &username, &locale, &timezone, &status, &verified, &avatarID, &gender)
	if err != nil || status != "ACTIVE" {
		return nil, errors.New("account unavailable")
	}
	roles, capabilities := []string{}, []string{}
	if rows, e := h.DB.QueryContext(ctx, `SELECT role FROM user_roles WHERE user_id=$1 ORDER BY role`, actor); e == nil {
		defer rows.Close()
		for rows.Next() {
			var v string
			if rows.Scan(&v) == nil {
				roles = append(roles, v)
			}
		}
	}
	if rows, e := h.DB.QueryContext(ctx, `SELECT capability FROM user_capabilities WHERE user_id=$1 ORDER BY capability`, actor); e == nil {
		defer rows.Close()
		for rows.Next() {
			var v string
			if rows.Scan(&v) == nil {
				capabilities = append(capabilities, v)
			}
		}
	}
	return map[string]any{"id": id, "email": email, "display_name": name, "username": username, "locale": locale, "timezone": timezone, "email_verified": verified != nil, "avatar_media_object_id": avatarID, "gender": gender, "roles": roles, "capabilities": capabilities}, nil
}
