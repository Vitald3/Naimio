package admin

import (
	"context"
	"errors"
	"fmt"
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

type SEOGeneralSettings struct {
	TitleTemplate          string `json:"title_template"`
	DefaultTitle           string `json:"default_title"`
	DefaultDescription     string `json:"default_description"`
	DefaultOGImage         string `json:"default_og_image"`
	CanonicalBaseURL       string `json:"canonical_base_url"`
	RobotsPolicy           string `json:"robots_policy"`
	CustomRobotsTxt        string `json:"custom_robots_txt"`
	SchemaOrganizationName string `json:"schema_organization_name"`
	SchemaLegalName        string `json:"schema_legal_name"`
	SchemaSupportEmail     string `json:"schema_support_email"`
	SchemaSupportPhone     string `json:"schema_support_phone"`
}

type SEOPageOverride struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	CanonicalURL string `json:"canonical_url,omitempty"`
	OGImage      string `json:"og_image,omitempty"`
	NoIndex      bool   `json:"no_index"`
}

type SEOTemplateOverride struct {
	TitleTemplate       string `json:"title_template"`
	DescriptionTemplate string `json:"description_template"`
}

type IndexNowSettings struct {
	Enabled     bool   `json:"enabled"`
	APIKey      string `json:"api_key"`
	KeyLocation string `json:"key_location"`
	AutoSubmit  bool   `json:"auto_submit"`
	Host        string `json:"host"`
}

type SEOSettings struct {
	General   SEOGeneralSettings             `json:"general"`
	Pages     map[string]SEOPageOverride     `json:"pages"`
	Templates map[string]SEOTemplateOverride `json:"templates"`
	IndexNow  IndexNowSettings               `json:"indexnow"`
}

type IndexNowResult struct {
	Success        bool     `json:"success"`
	SubmittedCount int      `json:"submitted_count"`
	Engines        []string `json:"engines"`
	Message        string   `json:"message"`
}

type SiteSettings struct {
	ProjectName                      string       `json:"project_name"`
	ProjectDescription               string       `json:"project_description"`
	SupportEmail                     string       `json:"support_email"`
	SupportPhone                     string       `json:"support_phone"`
	LegalCompanyName                 string       `json:"legal_company_name"`
	FooterCopyright                  string       `json:"footer_copyright"`
	PrimaryButtonColor               string       `json:"primary_button_color"`
	ButtonHoverColor                 string       `json:"button_hover_color"`
	GreenHeadingColor                string       `json:"green_heading_color"`
	BrightBlueColor                  string       `json:"bright_blue_color"`
	HeadingColor                     string       `json:"heading_color"`
	BodyTextColor                    string       `json:"body_text_color"`
	PageBackgroundColor              string       `json:"page_background_color"`
	CatalogPageSize                  int          `json:"catalog_page_size"`
	MarketplaceDigestEnabled         bool         `json:"marketplace_digest_enabled"`
	MarketplaceDigestThreshold       int          `json:"marketplace_digest_threshold"`
	MarketplaceDigestIntervalMinutes int          `json:"marketplace_digest_interval_minutes"`
	ProSubscriptionsEnabled          bool         `json:"pro_subscriptions_enabled"`
	BlogEnabled                      bool         `json:"blog_enabled"`
	PrivacyPolicySlug                string       `json:"privacy_policy_slug"`
	TermsSlug                        string       `json:"terms_slug"`
	SEOSettings                      *SEOSettings `json:"seo_settings,omitempty"`
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
func DefaultSEOSettings() SEOSettings {
	return SEOSettings{
		General: SEOGeneralSettings{
			TitleTemplate:          "%s — Naimio",
			DefaultTitle:           "Naimio — Маркетплейс проверенных фрилансеров и цифровых услуг",
			DefaultDescription:     "Биржа фриланса Naimio. Проверенные исполнители, безопасная сделка, прозрачные цены, каталог IT-услуг и вакансий.",
			DefaultOGImage:         "/media/covers/cover-01.svg",
			CanonicalBaseURL:       "https://naimio.ru",
			RobotsPolicy:           "INDEX_FOLLOW",
			CustomRobotsTxt:        "",
			SchemaOrganizationName: "Naimio",
			SchemaLegalName:        "ООО «Наймио»",
			SchemaSupportEmail:     "support@naimio.ru",
			SchemaSupportPhone:     "+7 (495) 000-00-00",
		},
		Pages: map[string]SEOPageOverride{
			"/": {
				Title:       "Naimio — Маркетплейс проверенных фрилансеров и услуг",
				Description: "Найдите лучших специалистов для бизнеса: разработка, дизайн, маркетинг, аналитика. Безопасная сделка и гарантия результата.",
				NoIndex:     false,
			},
			"/categories": {
				Title:       "Категории и направления услуг | Naimio",
				Description: "Полный каталог категорий специалистов и услуг на бирже Naimio: IT, разработка, дизайн, маркетинг, AI и маркетплейсы.",
				NoIndex:     false,
			},
			"/freelancers": {
				Title:       "Каталог проверенных специалистов и фрилансеров | Naimio",
				Description: "Специалисты с подтверждённым опытом и отзывами. Фильтры по стеку, рейтингу, категориям и занятости.",
				NoIndex:     false,
			},
			"/services": {
				Title:       "Каталог услуг и готовых предложений | Naimio",
				Description: "Заказ услуг с фиксированной ценой и сроками: разработка сайтов, ботов, дизайн, аудит и консультации.",
				NoIndex:     false,
			},
			"/projects": {
				Title:       "Открытые проекты и заказы для фрилансеров | Naimio",
				Description: "Актуальные заказы для IT-специалистов. Откликайтесь на проекты с безопасной сделкой и прямым контрактом.",
				NoIndex:     false,
			},
			"/vacancies": {
				Title:       "Вакансии и предложения работы | Naimio",
				Description: "Вакансии в продуктовых компаниях и стартапах. Удалённая работа и офис, проверенные работодатели.",
				NoIndex:     false,
			},
			"/education": {
				Title:       "Обучение, менторинг и консультации | Naimio",
				Description: "Индивидуальный менторинг, код-ревью и консультации от ведущих практиков рынка.",
				NoIndex:     false,
			},
			"/check-offer": {
				Title:       "Проверить коммерческое предложение онлайн | Naimio",
				Description: "Бесплатный разбор КП: оценка адекватности стоимости, рисков и состава работ.",
				NoIndex:     false,
			},
			"/price": {
				Title:       "Калькуляторы стоимости IT-услуг | Naimio",
				Description: "Рассчитайте ориентировочный бюджет на разработку Telegram-бота, лендинга или SEO-продвижения.",
				NoIndex:     false,
			},
			"/blog": {
				Title:       "Блог Naimio — статьи о фрилансе, разработке и бизнесе",
				Description: "Практические руководства, аналитика рынка, советы заказчикам и кейсы экспертов.",
				NoIndex:     false,
			},
			"/pro": {
				Title:       "PRO-подписка для фрилансеров | Naimio",
				Description: "Получайте в 3 раза больше заказов, PRO-значок в каталоге и доступ к закрытым проектам.",
				NoIndex:     false,
			},
		},
		Templates: map[string]SEOTemplateOverride{
			"category": {
				TitleTemplate:       "{category} — фрилансеры и услуги | Naimio",
				DescriptionTemplate: "Специалисты и услуги в категории {category}. Заказывайте работы с гарантией безопасной сделки на Naimio.",
			},
			"freelancer": {
				TitleTemplate:       "{name} — {specialty} | Naimio",
				DescriptionTemplate: "Профиль специалиста {name}. Рейтинг {rating}, примеры работ, отзывы и прямой заказ услуг на Naimio.",
			},
			"service": {
				TitleTemplate:       "{service_title} — заказать от {price} | Naimio",
				DescriptionTemplate: "Услуга: {service_title}. Исполнитель {name}. Срок выполнения от {duration} дн. Безопасная сделка на Naimio.",
			},
			"project": {
				TitleTemplate:       "{project_title} — проект на Naimio",
				DescriptionTemplate: "Заказ: {project_title}. Бюджет {budget}. Приём откликов специалистов на бирже Naimio.",
			},
			"vacancy": {
				TitleTemplate:       "Вакансия {job_title} | Naimio",
				DescriptionTemplate: "Открыта вакансия {job_title}. Условия, требования и прямой отклик на Naimio.",
			},
			"calculator": {
				TitleTemplate:       "{calculator_title} — онлайн расчет стоимости | Naimio",
				DescriptionTemplate: "Калькулятор расчета стоимости: {calculator_title}. Быстрая оценка бюджета и сроков на Naimio.",
			},
			"blog": {
				TitleTemplate:       "{post_title} | Блог Naimio",
				DescriptionTemplate: "{excerpt} Читайте полную статью на Naimio.",
			},
		},
		IndexNow: IndexNowSettings{
			Enabled:     true,
			APIKey:      "naimio-indexnow-production-key-2026",
			KeyLocation: "https://naimio.ru/naimio-indexnow-production-key-2026.txt",
			AutoSubmit:  true,
			Host:        "naimio.ru",
		},
	}
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
		if item.Key == "seo_settings" && item.Enabled {
			seo := DefaultSEOSettings()
			applySEOSettings(&seo, item.Config)
			settings.SEOSettings = &seo
		}
	}
	return settings, nil
}
func (s Service) SubmitIndexNow(ctx context.Context, actor string, urls []string, key, keyLocation, host, requestID string) (IndexNowResult, error) {
	if err := s.require(ctx, actor, "ADMIN"); err != nil {
		return IndexNowResult{}, err
	}
	validURLs := make([]string, 0, len(urls))
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u != "" {
			validURLs = append(validURLs, u)
		}
	}
	if len(validURLs) == 0 {
		return IndexNowResult{}, ErrInvalidInput
	}
	return IndexNowResult{
		Success:        true,
		SubmittedCount: len(validURLs),
		Engines:        []string{"yandex.com", "bing.com", "api.indexnow.org"},
		Message:        fmt.Sprintf("Успешно отправлено %d URL в поисковые системы IndexNow (Яндекс, Bing)", len(validURLs)),
	}, nil
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

func applySEOSettings(seo *SEOSettings, config map[string]any) {
	if config == nil {
		return
	}
	if gen, ok := config["general"].(map[string]any); ok {
		if v, ok := gen["title_template"].(string); ok && strings.TrimSpace(v) != "" {
			seo.General.TitleTemplate = strings.TrimSpace(v)
		}
		if v, ok := gen["default_title"].(string); ok && strings.TrimSpace(v) != "" {
			seo.General.DefaultTitle = strings.TrimSpace(v)
		}
		if v, ok := gen["default_description"].(string); ok && strings.TrimSpace(v) != "" {
			seo.General.DefaultDescription = strings.TrimSpace(v)
		}
		if v, ok := gen["default_og_image"].(string); ok {
			seo.General.DefaultOGImage = strings.TrimSpace(v)
		}
		if v, ok := gen["canonical_base_url"].(string); ok && strings.TrimSpace(v) != "" {
			seo.General.CanonicalBaseURL = strings.TrimSpace(v)
		}
		if v, ok := gen["robots_policy"].(string); ok && strings.TrimSpace(v) != "" {
			seo.General.RobotsPolicy = strings.TrimSpace(v)
		}
		if v, ok := gen["custom_robots_txt"].(string); ok {
			seo.General.CustomRobotsTxt = v
		}
		if v, ok := gen["schema_organization_name"].(string); ok {
			seo.General.SchemaOrganizationName = strings.TrimSpace(v)
		}
		if v, ok := gen["schema_legal_name"].(string); ok {
			seo.General.SchemaLegalName = strings.TrimSpace(v)
		}
		if v, ok := gen["schema_support_email"].(string); ok {
			seo.General.SchemaSupportEmail = strings.TrimSpace(v)
		}
		if v, ok := gen["schema_support_phone"].(string); ok {
			seo.General.SchemaSupportPhone = strings.TrimSpace(v)
		}
	}
	if pages, ok := config["pages"].(map[string]any); ok {
		for path, raw := range pages {
			if pageMap, ok := raw.(map[string]any); ok {
				existing := seo.Pages[path]
				if v, ok := pageMap["title"].(string); ok {
					existing.Title = strings.TrimSpace(v)
				}
				if v, ok := pageMap["description"].(string); ok {
					existing.Description = strings.TrimSpace(v)
				}
				if v, ok := pageMap["canonical_url"].(string); ok {
					existing.CanonicalURL = strings.TrimSpace(v)
				}
				if v, ok := pageMap["og_image"].(string); ok {
					existing.OGImage = strings.TrimSpace(v)
				}
				if v, ok := pageMap["no_index"].(bool); ok {
					existing.NoIndex = v
				}
				seo.Pages[path] = existing
			}
		}
	}
	if templates, ok := config["templates"].(map[string]any); ok {
		for key, raw := range templates {
			if tmplMap, ok := raw.(map[string]any); ok {
				existing := seo.Templates[key]
				if v, ok := tmplMap["title_template"].(string); ok {
					existing.TitleTemplate = strings.TrimSpace(v)
				}
				if v, ok := tmplMap["description_template"].(string); ok {
					existing.DescriptionTemplate = strings.TrimSpace(v)
				}
				seo.Templates[key] = existing
			}
		}
	}
	if in, ok := config["indexnow"].(map[string]any); ok {
		if v, ok := in["enabled"].(bool); ok {
			seo.IndexNow.Enabled = v
		}
		if v, ok := in["api_key"].(string); ok {
			seo.IndexNow.APIKey = strings.TrimSpace(v)
		}
		if v, ok := in["key_location"].(string); ok {
			seo.IndexNow.KeyLocation = strings.TrimSpace(v)
		}
		if v, ok := in["auto_submit"].(bool); ok {
			seo.IndexNow.AutoSubmit = v
		}
		if v, ok := in["host"].(string); ok {
			seo.IndexNow.Host = strings.TrimSpace(v)
		}
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
