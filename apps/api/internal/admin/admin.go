package admin

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
)

var (
	ErrForbidden    = errors.New("admin permission required")
	ErrNotFound     = errors.New("admin resource not found")
	ErrInvalidInput = errors.New("invalid admin input")
	ErrConflict     = errors.New("admin operation conflict")
)

type Cursor struct {
	At time.Time
	ID string
}

type PageInfo struct {
	NextCursor *Cursor
	HasMore    bool
}

type Dashboard struct {
	UsersTotal                    int64 `json:"users_total"`
	UsersNew7d                    int64 `json:"users_new_7d"`
	ProjectsActive                int64 `json:"projects_active"`
	ProjectsOpen                  int64 `json:"projects_open"`
	PendingReputation             int64 `json:"pending_reputation"`
	OpenReports                   int64 `json:"open_reports"`
	OpenFraudSignals              int64 `json:"open_fraud_signals"`
	OpenDisputes                  int64 `json:"open_disputes"`
	ActiveSafeDeals               int64 `json:"active_safe_deals"`
	ServicesActive                int64 `json:"services_active"`
	VacanciesPublished            int64 `json:"vacancies_published"`
	RecentAdministrativeMutations int64 `json:"recent_admin_actions"`
}

type User struct {
	ID             string     `json:"id"`
	Email          string     `json:"email"`
	Username       string     `json:"username,omitempty"`
	DisplayName    string     `json:"display_name"`
	Status         string     `json:"status"`
	EmailVerified  bool       `json:"email_verified"`
	Roles          []string   `json:"roles"`
	Capabilities   []string   `json:"capabilities"`
	CreatedAt      time.Time  `json:"created_at"`
	LastSeenAt     *time.Time `json:"last_seen_at,omitempty"`
	ActiveSessions int        `json:"active_sessions"`
}

type UserFilter struct {
	Q          string
	Status     string
	Role       string
	Capability string
}

type UserPage struct {
	Items []User
	Page  PageInfo
}

type FeatureFlag struct {
	Key         string         `json:"key"`
	Description string         `json:"description,omitempty"`
	Enabled     bool           `json:"enabled"`
	Config      map[string]any `json:"config"`
	UpdatedBy   string         `json:"updated_by,omitempty"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type SiteSettings struct {
	ProjectName                      string `json:"project_name"`
	ProjectDescription               string `json:"project_description"`
	SupportEmail                     string `json:"support_email"`
	SupportPhone                     string `json:"support_phone"`
	LegalCompanyName                 string `json:"legal_company_name"`
	FooterCopyright                  string `json:"footer_copyright"`
	PrimaryButtonColor               string `json:"primary_button_color"`
	ButtonHoverColor                 string `json:"button_hover_color"`
	GreenHeadingColor                string `json:"green_heading_color"`
	BrightBlueColor                  string `json:"bright_blue_color"`
	HeadingColor                     string `json:"heading_color"`
	BodyTextColor                    string `json:"body_text_color"`
	PageBackgroundColor              string `json:"page_background_color"`
	CatalogPageSize                  int    `json:"catalog_page_size"`
	MarketplaceDigestEnabled         bool   `json:"marketplace_digest_enabled"`
	MarketplaceDigestThreshold       int    `json:"marketplace_digest_threshold"`
	MarketplaceDigestIntervalMinutes int    `json:"marketplace_digest_interval_minutes"`
	ProSubscriptionsEnabled          bool   `json:"pro_subscriptions_enabled"`
	BlogEnabled                      bool   `json:"blog_enabled"`
	PrivacyPolicySlug                string `json:"privacy_policy_slug"`
	TermsSlug                        string `json:"terms_slug"`
}

type Report struct {
	ID             string     `json:"id"`
	ReporterUserID string     `json:"reporter_user_id,omitempty"`
	ReporterName   string     `json:"reporter_name,omitempty"`
	EntityType     string     `json:"entity_type"`
	EntityID       string     `json:"entity_id"`
	ReasonCode     string     `json:"reason_code"`
	Description    string     `json:"description,omitempty"`
	Status         string     `json:"status"`
	AssignedTo     string     `json:"assigned_to_user_id,omitempty"`
	Resolution     string     `json:"resolution,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	ReviewedAt     *time.Time `json:"reviewed_at,omitempty"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
}

type FraudSignal struct {
	ID         string         `json:"id"`
	UserID     string         `json:"user_id,omitempty"`
	UserName   string         `json:"user_name,omitempty"`
	EntityType string         `json:"entity_type,omitempty"`
	EntityID   string         `json:"entity_id,omitempty"`
	SignalType string         `json:"signal_type"`
	Severity   int            `json:"severity"`
	Evidence   map[string]any `json:"evidence"`
	Status     string         `json:"status"`
	Resolution string         `json:"resolution,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	ReviewedAt *time.Time     `json:"reviewed_at,omitempty"`
}

type AuditEntry struct {
	ID          string         `json:"id"`
	ActorUserID string         `json:"actor_user_id,omitempty"`
	ActorName   string         `json:"actor_name,omitempty"`
	Action      string         `json:"action"`
	TargetType  string         `json:"target_type,omitempty"`
	TargetID    string         `json:"target_id,omitempty"`
	Metadata    map[string]any `json:"metadata"`
	IPAddress   string         `json:"ip,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

type ContentItem struct {
	ID               string    `json:"id"`
	Kind             string    `json:"kind"`
	Title            string    `json:"title"`
	OwnerUserID      string    `json:"owner_user_id"`
	OwnerDisplayName string    `json:"owner_display_name"`
	Status           string    `json:"status"`
	ModerationStatus string    `json:"moderation_status"`
	ModerationReason string    `json:"moderation_reason,omitempty"`
	CategoryName     string    `json:"category_name,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type ReviewItem struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	ReviewerID string    `json:"reviewer_user_id"`
	Reviewer   string    `json:"reviewer_name"`
	RevieweeID string    `json:"reviewee_user_id"`
	Reviewee   string    `json:"reviewee_name"`
	Rating     int       `json:"rating_overall"`
	Text       string    `json:"text,omitempty"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

type DisputeItem struct {
	ID             string    `json:"id"`
	DealID         string    `json:"deal_id"`
	ProjectID      string    `json:"project_id"`
	ProjectTitle   string    `json:"project_title"`
	CustomerID     string    `json:"customer_user_id"`
	CustomerName   string    `json:"customer_name"`
	FreelancerID   string    `json:"freelancer_user_id"`
	FreelancerName string    `json:"freelancer_name"`
	AmountKopecks  int64     `json:"amount_kopecks"`
	DealStatus     string    `json:"deal_status"`
	ReasonCode     string    `json:"reason_code"`
	Description    string    `json:"description"`
	Status         string    `json:"status"`
	OpenedAt       time.Time `json:"opened_at"`
}

type ListFilter struct {
	Q      string
	Status string
	Kind   string
}

type Repository interface {
	Roles(context.Context, string) ([]string, error)
	Dashboard(context.Context) (Dashboard, error)
	ListUsers(context.Context, UserFilter, *Cursor, int) (UserPage, error)
	GetUser(context.Context, string) (User, error)
	SetUserStatus(context.Context, string, string, string, string, string) (User, error)
	RevokeSessions(context.Context, string, string, string, string) error
	SetRole(context.Context, string, string, string, bool, string, string) (User, error)
	ListFeatureFlags(context.Context) ([]FeatureFlag, error)
	UpdateFeatureFlag(context.Context, string, string, bool, map[string]any, string, string) (FeatureFlag, error)
	ListReports(context.Context, ListFilter, *Cursor, int) ([]Report, PageInfo, error)
	UpdateReport(context.Context, string, string, string, string, string) (Report, error)
	ListFraudSignals(context.Context, ListFilter, *Cursor, int) ([]FraudSignal, PageInfo, error)
	UpdateFraudSignal(context.Context, string, string, string, string, string) (FraudSignal, error)
	ListAudit(context.Context, ListFilter, *Cursor, int) ([]AuditEntry, PageInfo, error)
	ListContent(context.Context, string, ListFilter, *Cursor, int) ([]ContentItem, PageInfo, error)
	GetContent(context.Context, string, string) (ContentItem, error)
	ModerateContent(context.Context, string, string, string, string, string, string) (ContentItem, error)
	ListReviews(context.Context, ListFilter, *Cursor, int) ([]ReviewItem, PageInfo, error)
	ListDisputes(context.Context, ListFilter, *Cursor, int) ([]DisputeItem, PageInfo, error)
}

type Service struct{ Repository Repository }

func (s Service) roles(ctx context.Context, actor string) (map[string]bool, error) {
	if actor == "" || s.Repository == nil {
		return nil, ErrForbidden
	}
	items, err := s.Repository.Roles(ctx, actor)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(items))
	for _, role := range items {
		out[strings.ToUpper(role)] = true
	}
	return out, nil
}

func (s Service) require(ctx context.Context, actor string, allowed ...string) error {
	roles, err := s.roles(ctx, actor)
	if err != nil {
		return err
	}
	if roles["SUPER_ADMIN"] {
		return nil
	}
	for _, role := range allowed {
		if roles[role] {
			return nil
		}
	}
	return ErrForbidden
}

func (s Service) Dashboard(ctx context.Context, actor string) (Dashboard, error) {
	if err := s.require(ctx, actor, "MODERATOR", "ADMIN"); err != nil {
		return Dashboard{}, err
	}
	return s.Repository.Dashboard(ctx)
}
func (s Service) ListUsers(ctx context.Context, actor string, filter UserFilter, cursor *Cursor, limit int) (UserPage, error) {
	if err := s.require(ctx, actor, "ADMIN"); err != nil {
		return UserPage{}, err
	}
	return s.Repository.ListUsers(ctx, filter, cursor, limit)
}
func (s Service) GetUser(ctx context.Context, actor, id string) (User, error) {
	if err := s.require(ctx, actor, "ADMIN"); err != nil {
		return User{}, err
	}
	return s.Repository.GetUser(ctx, id)
}
func (s Service) SetUserStatus(ctx context.Context, actor, id, status, reason, requestID string) (User, error) {
	if err := s.require(ctx, actor, "ADMIN"); err != nil {
		return User{}, err
	}
	return s.Repository.SetUserStatus(ctx, actor, id, strings.ToUpper(strings.TrimSpace(status)), strings.TrimSpace(reason), requestID)
}
func (s Service) RevokeSessions(ctx context.Context, actor, id, reason, requestID string) error {
	if err := s.require(ctx, actor, "ADMIN"); err != nil {
		return err
	}
	return s.Repository.RevokeSessions(ctx, actor, id, strings.TrimSpace(reason), requestID)
}
func (s Service) SetRole(ctx context.Context, actor, id, role string, enabled bool, reason, requestID string) (User, error) {
	role = strings.ToUpper(strings.TrimSpace(role))
	if role == "MODERATOR" {
		if err := s.require(ctx, actor, "ADMIN"); err != nil {
			return User{}, err
		}
	} else {
		if err := s.require(ctx, actor, "SUPER_ADMIN"); err != nil {
			return User{}, err
		}
	}
	return s.Repository.SetRole(ctx, actor, id, role, enabled, strings.TrimSpace(reason), requestID)
}
func (s Service) ListFeatureFlags(ctx context.Context, actor string) ([]FeatureFlag, error) {
	if err := s.require(ctx, actor, "ADMIN"); err != nil {
		return nil, err
	}
	return s.Repository.ListFeatureFlags(ctx)
}
func DefaultSiteSettings() SiteSettings {
	return SiteSettings{ProjectName: "Naimio", ProjectDescription: "Маркетплейс профессиональных услуг", FooterCopyright: "© Naimio", PrimaryButtonColor: "#15956a", ButtonHoverColor: "#0d7452", GreenHeadingColor: "#0d7452", BrightBlueColor: "#2563a7", HeadingColor: "#0d1f16", BodyTextColor: "#13261d", PageBackgroundColor: "#ffffff", CatalogPageSize: 50, MarketplaceDigestEnabled: true, MarketplaceDigestThreshold: 10, MarketplaceDigestIntervalMinutes: 60}
}
func (s Service) PublicSiteSettings(ctx context.Context) (SiteSettings, error) {
	settings := DefaultSiteSettings()
	items, err := s.Repository.ListFeatureFlags(ctx)
	if err != nil {
		return settings, err
	}
	for _, item := range items {
		if item.Key == "pro_subscriptions_enabled" {
			settings.ProSubscriptionsEnabled = item.Enabled
		}
		if item.Key == "blog_enabled" {
			settings.BlogEnabled = item.Enabled
		}
		if item.Key == "site_appearance" && item.Enabled {
			applySiteSettings(&settings, item.Config)
		}
	}
	return settings, nil
}
func (s Service) UpdateFeatureFlag(ctx context.Context, actor, key string, enabled bool, config map[string]any, reason, requestID string) (FeatureFlag, error) {
	if err := s.require(ctx, actor, "ADMIN"); err != nil {
		return FeatureFlag{}, err
	}
	if key == "site_appearance" && !validSiteSettings(config) {
		return FeatureFlag{}, ErrInvalidInput
	}
	return s.Repository.UpdateFeatureFlag(ctx, actor, key, enabled, config, strings.TrimSpace(reason), requestID)
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func applySiteSettings(settings *SiteSettings, config map[string]any) {
	values := map[string]*string{"project_name": &settings.ProjectName, "project_description": &settings.ProjectDescription, "support_email": &settings.SupportEmail, "support_phone": &settings.SupportPhone, "legal_company_name": &settings.LegalCompanyName, "footer_copyright": &settings.FooterCopyright, "privacy_policy_slug": &settings.PrivacyPolicySlug, "terms_slug": &settings.TermsSlug, "primary_button_color": &settings.PrimaryButtonColor, "button_hover_color": &settings.ButtonHoverColor, "green_heading_color": &settings.GreenHeadingColor, "bright_blue_color": &settings.BrightBlueColor, "heading_color": &settings.HeadingColor, "body_text_color": &settings.BodyTextColor, "page_background_color": &settings.PageBackgroundColor}
	for key, target := range values {
		if value, ok := config[key].(string); ok {
			*target = strings.TrimSpace(value)
		}
	}
	if value, ok := config["catalog_page_size"].(float64); ok {
		settings.CatalogPageSize = int(value)
	} else if value, ok := config["catalog_page_size"].(int); ok {
		settings.CatalogPageSize = value
	}
	if value, ok := config["marketplace_digest_enabled"].(bool); ok {
		settings.MarketplaceDigestEnabled = value
	}
	if value, ok := config["marketplace_digest_threshold"].(float64); ok {
		settings.MarketplaceDigestThreshold = int(value)
	} else if value, ok := config["marketplace_digest_threshold"].(int); ok {
		settings.MarketplaceDigestThreshold = value
	}
	if value, ok := config["marketplace_digest_interval_minutes"].(float64); ok {
		settings.MarketplaceDigestIntervalMinutes = int(value)
	} else if value, ok := config["marketplace_digest_interval_minutes"].(int); ok {
		settings.MarketplaceDigestIntervalMinutes = value
	}
}
func validSiteSettings(config map[string]any) bool {
	settings := DefaultSiteSettings()
	applySiteSettings(&settings, config)
	if len(settings.ProjectName) < 2 || len(settings.ProjectName) > 80 || len(settings.ProjectDescription) > 240 || len(settings.SupportEmail) > 200 || len(settings.SupportPhone) > 50 || len(settings.LegalCompanyName) > 180 || len(settings.FooterCopyright) > 180 || len(settings.PrivacyPolicySlug) > 160 || len(settings.TermsSlug) > 160 {
		return false
	}
	for _, color := range []string{settings.PrimaryButtonColor, settings.ButtonHoverColor, settings.GreenHeadingColor, settings.BrightBlueColor, settings.HeadingColor, settings.BodyTextColor, settings.PageBackgroundColor} {
		if !hexColor.MatchString(color) {
			return false
		}
	}
	if settings.CatalogPageSize < 10 || settings.CatalogPageSize > 50 {
		return false
	}
	if settings.MarketplaceDigestThreshold < 1 || settings.MarketplaceDigestThreshold > 100 || settings.MarketplaceDigestIntervalMinutes < 5 || settings.MarketplaceDigestIntervalMinutes > 1440 {
		return false
	}
	return true
}
func (s Service) ListReports(ctx context.Context, actor string, f ListFilter, c *Cursor, l int) ([]Report, PageInfo, error) {
	if err := s.require(ctx, actor, "MODERATOR", "ADMIN"); err != nil {
		return nil, PageInfo{}, err
	}
	return s.Repository.ListReports(ctx, f, c, l)
}
func (s Service) UpdateReport(ctx context.Context, actor, id, status, resolution, requestID string) (Report, error) {
	if err := s.require(ctx, actor, "MODERATOR", "ADMIN"); err != nil {
		return Report{}, err
	}
	return s.Repository.UpdateReport(ctx, actor, id, strings.ToUpper(strings.TrimSpace(status)), strings.TrimSpace(resolution), requestID)
}
func (s Service) ListFraudSignals(ctx context.Context, actor string, f ListFilter, c *Cursor, l int) ([]FraudSignal, PageInfo, error) {
	if err := s.require(ctx, actor, "MODERATOR", "ADMIN"); err != nil {
		return nil, PageInfo{}, err
	}
	return s.Repository.ListFraudSignals(ctx, f, c, l)
}
func (s Service) UpdateFraudSignal(ctx context.Context, actor, id, status, resolution, requestID string) (FraudSignal, error) {
	if err := s.require(ctx, actor, "MODERATOR", "ADMIN"); err != nil {
		return FraudSignal{}, err
	}
	return s.Repository.UpdateFraudSignal(ctx, actor, id, strings.ToUpper(strings.TrimSpace(status)), strings.TrimSpace(resolution), requestID)
}
func (s Service) ListAudit(ctx context.Context, actor string, f ListFilter, c *Cursor, l int) ([]AuditEntry, PageInfo, error) {
	if err := s.require(ctx, actor, "ADMIN"); err != nil {
		return nil, PageInfo{}, err
	}
	return s.Repository.ListAudit(ctx, f, c, l)
}
func (s Service) ListContent(ctx context.Context, actor, kind string, f ListFilter, c *Cursor, l int) ([]ContentItem, PageInfo, error) {
	if err := s.require(ctx, actor, "MODERATOR", "ADMIN"); err != nil {
		return nil, PageInfo{}, err
	}
	return s.Repository.ListContent(ctx, kind, f, c, l)
}
func (s Service) GetContent(ctx context.Context, actor, kind, id string) (ContentItem, error) {
	if err := s.require(ctx, actor, "MODERATOR", "ADMIN"); err != nil {
		return ContentItem{}, err
	}
	if !uuidPattern.MatchString(strings.ToLower(strings.TrimSpace(id))) {
		return ContentItem{}, ErrInvalidInput
	}
	return s.Repository.GetContent(ctx, strings.ToUpper(strings.TrimSpace(kind)), strings.ToLower(strings.TrimSpace(id)))
}
func (s Service) ModerateContent(ctx context.Context, actor, kind, id, action, reason, requestID string) (ContentItem, error) {
	if err := s.require(ctx, actor, "MODERATOR", "ADMIN"); err != nil {
		return ContentItem{}, err
	}
	return s.Repository.ModerateContent(ctx, actor, strings.ToUpper(strings.TrimSpace(kind)), id, strings.ToUpper(strings.TrimSpace(action)), strings.TrimSpace(reason), requestID)
}
func (s Service) ListReviews(ctx context.Context, actor string, f ListFilter, c *Cursor, l int) ([]ReviewItem, PageInfo, error) {
	if err := s.require(ctx, actor, "MODERATOR", "ADMIN"); err != nil {
		return nil, PageInfo{}, err
	}
	return s.Repository.ListReviews(ctx, f, c, l)
}
func (s Service) ListDisputes(ctx context.Context, actor string, f ListFilter, c *Cursor, l int) ([]DisputeItem, PageInfo, error) {
	if err := s.require(ctx, actor, "MODERATOR", "ADMIN"); err != nil {
		return nil, PageInfo{}, err
	}
	return s.Repository.ListDisputes(ctx, f, c, l)
}
