package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/argon2"
	"io"
	"net/http"
	"strings"
	"time"
)

type AvatarDeleter interface {
	Delete(ctx context.Context, actorID, mediaID string) error
}

type Handler struct {
	DB              *sql.DB
	CookieName      string
	AdminCookieName string
	Secure          bool
	SessionLifetime time.Duration
	AvatarDeleter   AvatarDeleter
}
type registerInput struct {
	Email             string `json:"email"`
	Password          string `json:"password"`
	DisplayName       string `json:"display_name"`
	AccountType       string `json:"account_type"`
	Gender            string `json:"gender"`
	ExperienceYears   *int   `json:"experience_years"`
	HourlyRateKopecks *int64 `json:"hourly_rate_kopecks"`
	Availability      string `json:"availability"`
}
type loginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Portal   string `json:"portal"`
}
type changePasswordInput struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (h Handler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		authProblem(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if !sameOrigin(r) {
		authProblem(w, r, 403, "CSRF_REJECTED", "request origin is not allowed")
		return
	}
	var in registerInput
	if !authDecode(w, r, &in) {
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	in.AccountType = strings.ToUpper(strings.TrimSpace(in.AccountType))
	in.Gender = strings.ToUpper(strings.TrimSpace(in.Gender))
	if in.AccountType == "FREELANCER" {
		in.Availability = strings.ToUpper(strings.TrimSpace(in.Availability))
		if in.Availability == "" {
			in.Availability = "AVAILABLE"
		}
	}
	invalidFreelancer := in.AccountType == "FREELANCER" && (in.ExperienceYears == nil || in.HourlyRateKopecks == nil || *in.ExperienceYears < 0 || *in.ExperienceYears > 80 || *in.HourlyRateKopecks < 0 || *in.HourlyRateKopecks > 100000000 || (in.Availability != "AVAILABLE" && in.Availability != "PARTIALLY_BUSY" && in.Availability != "BUSY" && in.Availability != "UNAVAILABLE"))
	if !validEmail(in.Email) || len([]rune(in.DisplayName)) < 1 || len([]rune(in.DisplayName)) > 120 || len(in.Password) < 10 || len(in.Password) > 128 || (in.AccountType != "CUSTOMER" && in.AccountType != "FREELANCER") || (in.Gender != "MALE" && in.Gender != "FEMALE") || invalidFreelancer {
		authProblem(w, r, 422, "VALIDATION_ERROR", "invalid registration data")
		return
	}
	if h.DB == nil {
		authProblem(w, r, 503, "AUTH_UNAVAILABLE", "authentication is temporarily unavailable")
		return
	}
	hash, e := hashPassword(in.Password)
	if e != nil {
		authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
		return
	}
	tx, e := h.DB.BeginTx(r.Context(), nil)
	if e != nil {
		authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
		return
	}
	defer tx.Rollback()
	id := authID()
	if _, e = tx.ExecContext(r.Context(), `INSERT INTO users(id,email,email_normalized,password_hash,display_name,gender)VALUES($1,$2,$2,$3,$4,$5)`, id, in.Email, hash, in.DisplayName, in.Gender); e != nil {
		var p *pgconn.PgError
		if errors.As(e, &p) && p.Code == "23505" {
			authProblem(w, r, 409, "CONFLICT", "account cannot be created")
		} else {
			authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
		}
		return
	}
	if _, e = tx.ExecContext(r.Context(), `INSERT INTO user_capabilities(user_id,capability) VALUES($1,$2)`, id, in.AccountType); e != nil {
		authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
		return
	}
	if in.AccountType == "FREELANCER" {
		if _, e = tx.ExecContext(r.Context(), `INSERT INTO professional_profiles(user_id,experience_years,hourly_rate_kopecks,availability,profile_visibility)VALUES($1,$2,$3,$4,'PUBLIC')`, id, in.ExperienceYears, in.HourlyRateKopecks, in.Availability); e != nil {
			authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
			return
		}
	}
	if e = h.enqueueEmailVerification(r.Context(), tx, id); e != nil {
		authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
		return
	}
	token, tokenHash, e := sessionToken()
	if e != nil {
		authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
		return
	}
	expires := time.Now().UTC().Add(h.lifetime())
	if _, e = tx.ExecContext(r.Context(), `INSERT INTO sessions(id,user_id,token_hash,user_agent,last_used_at,expires_at)VALUES(gen_random_uuid(),$1,$2,$3,now(),$4)`, id, tokenHash, boundedAgent(r.UserAgent()), expires); e != nil {
		authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
		return
	}
	if e = tx.Commit(); e != nil {
		authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
		return
	}
	h.setCookie(w, token, expires)
	authJSON(w, 201, map[string]any{"data": map[string]any{"id": id, "email": in.Email, "display_name": in.DisplayName, "account_type": in.AccountType, "gender": in.Gender, "email_verified": false}})
}
func (h Handler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		authProblem(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if !sameOrigin(r) {
		authProblem(w, r, 403, "CSRF_REJECTED", "request origin is not allowed")
		return
	}
	var in loginInput
	if !authDecode(w, r, &in) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if !validEmail(email) || len(in.Password) > 128 {
		authProblem(w, r, 401, "INVALID_CREDENTIALS", "invalid email or password")
		return
	}
	if h.DB == nil {
		authProblem(w, r, 503, "AUTH_UNAVAILABLE", "authentication is temporarily unavailable")
		return
	}
	var id, status, hash string
	e := h.DB.QueryRowContext(r.Context(), `SELECT id::text,status,password_hash FROM users WHERE email_normalized=$1 AND deleted_at IS NULL`, email).Scan(&id, &status, &hash)
	if errors.Is(e, sql.ErrNoRows) {
		hash = fakePasswordHash
	} else if e != nil {
		authProblem(w, r, 503, "AUTH_UNAVAILABLE", "authentication is temporarily unavailable")
		return
	}
	valid := verifyPassword(in.Password, hash) && e == nil && status == "ACTIVE"
	portal := strings.ToLower(strings.TrimSpace(in.Portal))
	if portal == "" {
		portal = "marketplace"
	}
	if portal != "marketplace" && portal != "admin" {
		authProblem(w, r, 422, "VALIDATION_ERROR", "invalid login portal")
		return
	}
	staff := false
	if valid {
		if roleErr := h.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM user_roles WHERE user_id=$1 AND role IN ('MODERATOR','ADMIN','SUPER_ADMIN'))`, id).Scan(&staff); roleErr != nil {
			authProblem(w, r, 503, "AUTH_UNAVAILABLE", "authentication is temporarily unavailable")
			return
		}
	}
	allowedPortal := valid && ((portal == "admin" && staff) || (portal == "marketplace" && !staff))
	failureCode := "INVALID_CREDENTIALS"
	if valid && !allowedPortal {
		failureCode = "WRONG_PORTAL"
	}
	_, _ = h.DB.ExecContext(r.Context(), `INSERT INTO login_events(id,user_id,email_normalized,success,failure_code,user_agent)VALUES(gen_random_uuid(),NULLIF($1,'')::uuid,$2,$3,$4,$5)`, id, email, allowedPortal, failureCode, boundedAgent(r.UserAgent()))
	if !valid {
		authProblem(w, r, 401, "INVALID_CREDENTIALS", "invalid email or password")
		return
	}
	if !allowedPortal {
		authProblem(w, r, 403, "WRONG_PORTAL", "account is not allowed to use this login portal")
		return
	}
	token, tokenHash, e := sessionToken()
	if e != nil {
		authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
		return
	}
	expires := time.Now().UTC().Add(h.lifetime())
	if _, e = h.DB.ExecContext(r.Context(), `INSERT INTO sessions(id,user_id,token_hash,user_agent,last_used_at,expires_at)VALUES(gen_random_uuid(),$1,$2,$3,now(),$4)`, id, tokenHash, boundedAgent(r.UserAgent()), expires); e != nil {
		authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
		return
	}
	h.setNamedCookie(w, h.cookieForPortal(portal), token, expires)
	authJSON(w, 200, map[string]any{"data": map[string]any{"id": id}})
}
func (h Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		authProblem(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	cookie, e := r.Cookie(h.cookieName())
	if e == nil && h.DB != nil {
		sum := sha256.Sum256([]byte(cookie.Value))
		_, _ = h.DB.ExecContext(r.Context(), `UPDATE sessions SET revoked_at=COALESCE(revoked_at,now())WHERE token_hash=$1`, sum[:])
	}
	h.clearCookie(w)
	w.WriteHeader(204)
}

func (h Handler) AdminLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		authProblem(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	name := h.adminCookieName()
	cookie, e := r.Cookie(name)
	if e == nil && h.DB != nil {
		sum := sha256.Sum256([]byte(cookie.Value))
		_, _ = h.DB.ExecContext(r.Context(), `UPDATE sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE token_hash=$1`, sum[:])
	}
	h.clearNamedCookie(w, name)
	w.WriteHeader(http.StatusNoContent)
}

func (h Handler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		authProblem(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	actor, ok := ActorID(r.Context())
	if !ok {
		authJSON(w, http.StatusOK, map[string]any{"data": nil})
		return
	}
	if _, e := h.DB.ExecContext(r.Context(), `UPDATE sessions SET revoked_at=COALESCE(revoked_at,now())WHERE user_id=$1 AND revoked_at IS NULL`, actor); e != nil {
		authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
		return
	}
	h.clearCookie(w)
	w.WriteHeader(204)
}
func (h Handler) Session(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		authProblem(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	actor, ok := ActorID(r.Context())
	if !ok {
		authJSON(w, 200, map[string]any{"data": nil})
		return
	}
	var email, name, username, avatarID, gender string
	var emailVerified bool
	if e := h.DB.QueryRowContext(r.Context(), `SELECT email,display_name,COALESCE(username,''),COALESCE(avatar_media_object_id::text,''),gender,email_verified_at IS NOT NULL FROM users WHERE id=$1 AND status='ACTIVE'AND deleted_at IS NULL`, actor).Scan(&email, &name, &username, &avatarID, &gender, &emailVerified); e != nil {
		if errors.Is(e, sql.ErrNoRows) {
			// Stale session whose user is gone/suspended: report anonymous rather than error noise.
			authJSON(w, 200, map[string]any{"data": nil})
			return
		}
		authProblem(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	roles, capabilities := []string{}, []string{}
	if rows, e := h.DB.QueryContext(r.Context(), `SELECT role FROM user_roles WHERE user_id=$1 ORDER BY role`, actor); e == nil {
		defer rows.Close()
		for rows.Next() {
			var value string
			if rows.Scan(&value) == nil {
				roles = append(roles, value)
			}
		}
	}
	if rows, e := h.DB.QueryContext(r.Context(), `SELECT capability FROM user_capabilities WHERE user_id=$1 ORDER BY capability`, actor); e == nil {
		defer rows.Close()
		for rows.Next() {
			var value string
			if rows.Scan(&value) == nil {
				capabilities = append(capabilities, value)
			}
		}
	}
	authJSON(w, 200, map[string]any{"data": map[string]any{"id": actor, "email": email, "display_name": name, "username": username, "email_verified": emailVerified, "avatar_media_object_id": avatarID, "gender": gender, "roles": roles, "capabilities": capabilities}})
}

func (h Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		authProblem(w, r, 405, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	actor, ok := ActorID(r.Context())
	if !ok {
		authProblem(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	var in changePasswordInput
	if !authDecode(w, r, &in) {
		return
	}
	if len(in.CurrentPassword) > 128 || len(in.NewPassword) < 12 || len(in.NewPassword) > 128 || in.CurrentPassword == in.NewPassword {
		authProblem(w, r, 422, "VALIDATION_ERROR", "invalid password change data")
		return
	}
	var currentHash string
	if err := h.DB.QueryRowContext(r.Context(), `SELECT password_hash FROM users WHERE id=$1 AND status='ACTIVE' AND deleted_at IS NULL`, actor).Scan(&currentHash); err != nil {
		authProblem(w, r, 401, "UNAUTHENTICATED", "authentication required")
		return
	}
	if !verifyPassword(in.CurrentPassword, currentHash) {
		authProblem(w, r, 422, "CURRENT_PASSWORD_INVALID", "current password is invalid")
		return
	}
	newHash, err := hashPassword(in.NewPassword)
	if err != nil {
		authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(r.Context(), `UPDATE users SET password_hash=$2,updated_at=now() WHERE id=$1`, actor, newHash); err != nil {
		authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1`, actor); err != nil {
		authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
		return
	}
	if err = tx.Commit(); err != nil {
		authProblem(w, r, 500, "INTERNAL_ERROR", "internal server error")
		return
	}
	h.clearCookie(w)
	authJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"changed": true}})
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, e := rand.Read(salt); e != nil {
		return "", e
	}
	hash := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=2$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}
func verifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var version int
	var memory uint32
	var iterations uint32
	var parallelism uint8
	if _, e := fmt.Sscanf(parts[2], "v=%d", &version); e != nil || version != argon2.Version {
		return false
	}
	if _, e := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); e != nil {
		return false
	}
	salt, e1 := base64.RawStdEncoding.DecodeString(parts[4])
	expected, e2 := base64.RawStdEncoding.DecodeString(parts[5])
	if e1 != nil || e2 != nil || len(salt) != 16 || len(expected) != 32 || memory > 128*1024 || iterations > 10 || parallelism > 8 {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

var fakePasswordHash, _ = hashPassword("not-a-real-password")

func sessionToken() (string, []byte, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", nil, e
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(token))
	return token, sum[:], nil
}
func authID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&15 | 64
	b[8] = b[8]&63 | 128
	raw := fmt.Sprintf("%x", b)
	return raw[:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:]
}
func validEmail(v string) bool {
	return len(v) >= 3 && len(v) <= 320 && strings.Count(v, "@") == 1 && !strings.ContainsAny(v, " \t\r\n")
}
func authDecode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(&struct{}{}) != io.EOF {
		authProblem(w, r, 400, "VALIDATION_ERROR", "invalid authentication payload")
		return false
	}
	return true
}
func (h Handler) setCookie(w http.ResponseWriter, token string, expires time.Time) {
	h.setNamedCookie(w, h.cookieName(), token, expires)
}
func (h Handler) setNamedCookie(w http.ResponseWriter, name, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: token, Path: "/", HttpOnly: true, Secure: h.Secure, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: int(time.Until(expires).Seconds())})
}
func (h Handler) clearCookie(w http.ResponseWriter) { h.clearNamedCookie(w, h.cookieName()) }
func (h Handler) clearNamedCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Path: "/", HttpOnly: true, Secure: h.Secure, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0)})
}
func (h Handler) cookieName() string {
	if h.CookieName == "" {
		return "session"
	}
	return h.CookieName
}
func (h Handler) adminCookieName() string {
	if h.AdminCookieName != "" {
		return h.AdminCookieName
	}
	return h.cookieName() + "_admin"
}
func (h Handler) cookieForPortal(portal string) string {
	if portal == "admin" {
		return h.adminCookieName()
	}
	return h.cookieName()
}

func (h Handler) lifetime() time.Duration {
	if h.SessionLifetime <= 0 {
		return 30 * 24 * time.Hour
	}
	return h.SessionLifetime
}
func boundedAgent(v string) string {
	if len(v) > 500 {
		return v[:500]
	}
	return v
}
func authProblem(w http.ResponseWriter, r *http.Request, s int, c, m string) {
	authJSON(w, s, map[string]any{"error": map[string]string{"code": c, "message": m, "request_id": r.Header.Get("X-Request-ID")}})
}
func authJSON(w http.ResponseWriter, s int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(s)
	_ = json.NewEncoder(w).Encode(v)
}
