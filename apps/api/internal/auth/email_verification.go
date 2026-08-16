package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

func (h Handler) enqueueEmailVerification(ctx context.Context, tx *sql.Tx, userID string) error {
	token, tokenHash, err := sessionToken()
	if err != nil {
		return err
	}
	tokenID := authID()
	if _, err = tx.ExecContext(ctx, `UPDATE auth_tokens SET consumed_at=COALESCE(consumed_at,now()) WHERE user_id=$1 AND purpose='VERIFY_EMAIL' AND consumed_at IS NULL`, userID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO auth_tokens(id,user_id,purpose,token_hash,expires_at) VALUES($1,$2,'VERIFY_EMAIL',$3,$4)`, tokenID, userID, tokenHash, time.Now().UTC().Add(24*time.Hour)); err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]string{"token": token})
	_, err = tx.ExecContext(ctx, `INSERT INTO email_jobs(dedupe_key,user_id,template,payload) VALUES($1,$2,'verify_email',$3::jsonb)`, "verify-email:"+tokenID, userID, string(payload))
	return err
}

func (h Handler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		authProblem(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if !sameOrigin(r) {
		authProblem(w, r, http.StatusForbidden, "CSRF_REJECTED", "request origin is not allowed")
		return
	}
	var in struct {
		Token string `json:"token"`
	}
	if !authDecode(w, r, &in) {
		return
	}
	in.Token = strings.TrimSpace(in.Token)
	if len(in.Token) < 32 || len(in.Token) > 256 || h.DB == nil {
		authProblem(w, r, http.StatusUnprocessableEntity, "INVALID_VERIFICATION_TOKEN", "verification link is invalid or expired")
		return
	}
	sum := sha256.Sum256([]byte(in.Token))
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
		return
	}
	defer tx.Rollback()
	var userID string
	err = tx.QueryRowContext(r.Context(), `UPDATE auth_tokens SET consumed_at=now() WHERE token_hash=$1 AND purpose='VERIFY_EMAIL' AND consumed_at IS NULL AND expires_at>now() RETURNING user_id::text`, sum[:]).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		authProblem(w, r, http.StatusUnprocessableEntity, "INVALID_VERIFICATION_TOKEN", "verification link is invalid or expired")
		return
	}
	if err != nil {
		authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE users SET email_verified_at=COALESCE(email_verified_at,now()),updated_at=now() WHERE id=$1 AND status='ACTIVE' AND deleted_at IS NULL`, userID); err != nil {
		authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
		return
	}
	if err = tx.Commit(); err != nil {
		authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
		return
	}
	authJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"email_verified": true}})
}

func (h Handler) ResendEmailVerification(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		authProblem(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	actor, ok := ActorID(r.Context())
	if !ok || h.DB == nil {
		authProblem(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
		return
	}
	defer tx.Rollback()
	var verified bool
	if err = tx.QueryRowContext(r.Context(), `SELECT email_verified_at IS NOT NULL FROM users WHERE id=$1 AND status='ACTIVE' AND deleted_at IS NULL FOR UPDATE`, actor).Scan(&verified); err != nil {
		authProblem(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return
	}
	if !verified {
		if err = h.enqueueEmailVerification(r.Context(), tx, actor); err != nil {
			authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
			return
		}
	}
	if err = tx.Commit(); err != nil {
		authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
