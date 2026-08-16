package admin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeRepo struct {
	roles map[string][]string
	user  User
	flags []FeatureFlag
}

func (f *fakeRepo) Roles(_ context.Context, id string) ([]string, error) { return f.roles[id], nil }
func (f *fakeRepo) Dashboard(context.Context) (Dashboard, error) {
	return Dashboard{UsersTotal: 3}, nil
}
func (f *fakeRepo) ListUsers(context.Context, UserFilter, *Cursor, int) (UserPage, error) {
	return UserPage{Items: []User{f.user}}, nil
}
func (f *fakeRepo) GetUser(context.Context, string) (User, error) { return f.user, nil }
func (f *fakeRepo) SetUserStatus(_ context.Context, _, _, status, _, _ string) (User, error) {
	v := f.user
	v.Status = status
	return v, nil
}
func (f *fakeRepo) RevokeSessions(context.Context, string, string, string, string) error { return nil }
func (f *fakeRepo) SetRole(_ context.Context, _, _, role string, enabled bool, _, _ string) (User, error) {
	v := f.user
	if enabled {
		v.Roles = append(v.Roles, role)
	}
	return v, nil
}
func (f *fakeRepo) ListFeatureFlags(context.Context) ([]FeatureFlag, error) { return f.flags, nil }
func (f *fakeRepo) UpdateFeatureFlag(_ context.Context, _, key string, enabled bool, config map[string]any, _, _ string) (FeatureFlag, error) {
	return FeatureFlag{Key: key, Enabled: enabled, Config: config}, nil
}
func (f *fakeRepo) ListReports(context.Context, ListFilter, *Cursor, int) ([]Report, PageInfo, error) {
	return []Report{{ID: "11111111-1111-4111-8111-111111111111", CreatedAt: time.Now()}}, PageInfo{}, nil
}
func (f *fakeRepo) UpdateReport(context.Context, string, string, string, string, string) (Report, error) {
	return Report{}, nil
}
func (f *fakeRepo) ListFraudSignals(context.Context, ListFilter, *Cursor, int) ([]FraudSignal, PageInfo, error) {
	return nil, PageInfo{}, nil
}
func (f *fakeRepo) UpdateFraudSignal(context.Context, string, string, string, string, string) (FraudSignal, error) {
	return FraudSignal{}, nil
}
func (f *fakeRepo) ListAudit(context.Context, ListFilter, *Cursor, int) ([]AuditEntry, PageInfo, error) {
	return nil, PageInfo{}, nil
}
func (f *fakeRepo) ListContent(context.Context, string, ListFilter, *Cursor, int) ([]ContentItem, PageInfo, error) {
	return nil, PageInfo{}, nil
}
func (f *fakeRepo) GetContent(context.Context, string, string) (ContentItem, error) {
	return ContentItem{}, nil
}

func (f *fakeRepo) ModerateContent(context.Context, string, string, string, string, string, string) (ContentItem, error) {
	return ContentItem{}, nil
}
func (f *fakeRepo) ListReviews(context.Context, ListFilter, *Cursor, int) ([]ReviewItem, PageInfo, error) {
	return nil, PageInfo{}, nil
}
func (f *fakeRepo) ListDisputes(context.Context, ListFilter, *Cursor, int) ([]DisputeItem, PageInfo, error) {
	return nil, PageInfo{}, nil
}

func TestRoleMatrix(t *testing.T) {
	repo := &fakeRepo{roles: map[string][]string{"mod": {"MODERATOR"}, "admin": {"ADMIN"}, "super": {"SUPER_ADMIN"}}, user: User{ID: "22222222-2222-4222-8222-222222222222"}}
	s := Service{Repository: repo}
	if _, _, err := s.ListReports(context.Background(), "mod", ListFilter{}, nil, 20); err != nil {
		t.Fatalf("moderator reports: %v", err)
	}
	if _, err := s.ListUsers(context.Background(), "mod", UserFilter{}, nil, 20); err != ErrForbidden {
		t.Fatalf("moderator users err=%v", err)
	}
	if _, err := s.SetRole(context.Background(), "admin", repo.user.ID, "ADMIN", true, "promotion", "req"); err != ErrForbidden {
		t.Fatalf("admin elevated role err=%v", err)
	}
	if _, err := s.SetRole(context.Background(), "super", repo.user.ID, "ADMIN", true, "promotion", "req"); err != nil {
		t.Fatalf("super role: %v", err)
	}
}

func TestPublicSiteSettingsUsesOnlyEnabledAppearanceConfig(t *testing.T) {
	repo := &fakeRepo{flags: []FeatureFlag{{Key: "site_appearance", Enabled: true, Config: map[string]any{"project_name": "Мой проект", "primary_button_color": "#112233", "bright_blue_color": "#3366ff"}}}}
	settings, err := (Service{Repository: repo}).PublicSiteSettings(context.Background())
	if err != nil || settings.ProjectName != "Мой проект" || settings.PrimaryButtonColor != "#112233" || settings.BrightBlueColor != "#3366ff" || settings.HeadingColor == "" {
		t.Fatalf("settings=%#v err=%v", settings, err)
	}
	h := Handler{Service: Service{Repository: repo}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/site-settings", nil)
	res := httptest.NewRecorder()
	h.SiteSettings(res, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "Мой проект") {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestAppearanceSettingsRejectInvalidColors(t *testing.T) {
	repo := &fakeRepo{roles: map[string][]string{"admin": {"ADMIN"}}}
	_, err := (Service{Repository: repo}).UpdateFeatureFlag(context.Background(), "admin", "site_appearance", true, map[string]any{"project_name": "Naimio", "primary_button_color": "red"}, "theme update", "req")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("error=%v", err)
	}
}

func TestAdminHandlerRejectsOrdinaryUser(t *testing.T) {
	repo := &fakeRepo{roles: map[string][]string{"normal": {}}, user: User{ID: "22222222-2222-4222-8222-222222222222"}}
	h := Handler{Service: Service{Repository: repo}, ActorID: func(context.Context) (string, bool) { return "normal", true }}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	res := httptest.NewRecorder()
	h.Users(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}
func TestUserMutationValidatesReason(t *testing.T) {
	repo := &fakeRepo{roles: map[string][]string{"admin": {"ADMIN"}}, user: User{ID: "22222222-2222-4222-8222-222222222222"}}
	h := Handler{Service: Service{Repository: repo}, ActorID: func(context.Context) (string, bool) { return "admin", true }}
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/22222222-2222-4222-8222-222222222222/status", strings.NewReader(`{"status":"BANNED","reason":"abuse"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	h.Users(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}
