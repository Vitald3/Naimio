//go:build integration

package auth

import (
	"context"
	"database/sql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestRegistrationLoginAndServerSession(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Fatal("DATABASE_URL is required")
	}
	db, e := sql.Open("pgx", url)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	t.Cleanup(func() {
		for _, q := range []string{
			"DELETE FROM login_events WHERE email_normalized='phase5-auth@example.invalid'",
			"DELETE FROM sessions WHERE user_id IN(SELECT id FROM users WHERE email_normalized='phase5-auth@example.invalid')",
			"DELETE FROM email_jobs WHERE user_id IN(SELECT id FROM users WHERE email_normalized='phase5-auth@example.invalid')",
			"DELETE FROM auth_tokens WHERE user_id IN(SELECT id FROM users WHERE email_normalized='phase5-auth@example.invalid')",
			"DELETE FROM user_capabilities WHERE user_id IN(SELECT id FROM users WHERE email_normalized='phase5-auth@example.invalid')",
			"DELETE FROM users WHERE email_normalized='phase5-auth@example.invalid'",
		} {
			_, _ = db.ExecContext(context.Background(), q)
		}
	})
	h := Handler{DB: db, CookieName: "session", Secure: true}
	body := `{"email":"phase5-auth@example.invalid","password":"correct horse battery staple","display_name":"Phase Five","account_type":"CUSTOMER","gender":"FEMALE"}`
	w := httptest.NewRecorder()
	h.Register(w, httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body)))
	if w.Code != 201 {
		t.Fatalf("register=%d %s", w.Code, w.Body.String())
	}
	cookie := w.Result().Cookies()[0]
	login := httptest.NewRecorder()
	h.Login(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"phase5-auth@example.invalid","password":"correct horse battery staple"}`)))
	if login.Code != 200 {
		t.Fatalf("login=%d %s", login.Code, login.Body.String())
	}
	middleware := SessionMiddleware{Repository: PostgresSessionRepository{DB: db}, CookieName: "session"}
	request := httptest.NewRequest(http.MethodGet, "/private", nil)
	request.AddCookie(cookie)
	authorized := httptest.NewRecorder()
	middleware.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := ActorID(r.Context()); !ok {
			t.Fatal("actor missing")
		}
		w.WriteHeader(204)
	})).ServeHTTP(authorized, request)
	if authorized.Code != 204 {
		t.Fatalf("session=%d", authorized.Code)
	}
}

func TestStaffAndMarketplaceLoginPortalsAreSeparated(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Fatal("DATABASE_URL is required")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const email = "staff-portal-test@example.invalid"
	t.Cleanup(func() {
		for _, q := range []string{
			"DELETE FROM login_events WHERE email_normalized=$1",
			"DELETE FROM sessions WHERE user_id IN(SELECT id FROM users WHERE email_normalized=$1)",
			"DELETE FROM email_jobs WHERE user_id IN(SELECT id FROM users WHERE email_normalized=$1)",
			"DELETE FROM auth_tokens WHERE user_id IN(SELECT id FROM users WHERE email_normalized=$1)",
			"DELETE FROM user_roles WHERE user_id IN(SELECT id FROM users WHERE email_normalized=$1)",
			"DELETE FROM user_capabilities WHERE user_id IN(SELECT id FROM users WHERE email_normalized=$1)",
			"DELETE FROM users WHERE email_normalized=$1",
		} {
			_, _ = db.ExecContext(context.Background(), q, email)
		}
	})
	h := Handler{DB: db, CookieName: "session", Secure: true}
	reg := httptest.NewRecorder()
	h.Register(reg, httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"staff-portal-test@example.invalid","password":"correct horse battery staple","display_name":"Staff Test","account_type":"CUSTOMER","gender":"MALE"}`)))
	if reg.Code != http.StatusCreated {
		t.Fatalf("register=%d %s", reg.Code, reg.Body.String())
	}
	var id string
	if err := db.QueryRow(`SELECT id::text FROM users WHERE email_normalized=$1`, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO user_roles(user_id,role,granted_by) VALUES($1,'ADMIN',$1)`, id); err != nil {
		t.Fatal(err)
	}
	marketplace := httptest.NewRecorder()
	h.Login(marketplace, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"staff-portal-test@example.invalid","password":"correct horse battery staple","portal":"marketplace"}`)))
	if marketplace.Code != http.StatusForbidden {
		t.Fatalf("marketplace staff login=%d %s", marketplace.Code, marketplace.Body.String())
	}
	staff := httptest.NewRecorder()
	h.Login(staff, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"staff-portal-test@example.invalid","password":"correct horse battery staple","portal":"admin"}`)))
	if staff.Code != http.StatusOK {
		t.Fatalf("staff login=%d %s", staff.Code, staff.Body.String())
	}
}
