package blog

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	ErrForbidden = errors.New("blog admin permission required")
	ErrNotFound  = errors.New("blog resource not found")
	ErrInvalid   = errors.New("invalid blog input")
	ErrConflict  = errors.New("blog conflict")
)
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type Category struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type Tag struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Post struct {
	ID                 string     `json:"id"`
	AuthorUserID       string     `json:"author_user_id"`
	AuthorName         string     `json:"author_name"`
	CategoryID         string     `json:"category_id,omitempty"`
	Category           *Category  `json:"category,omitempty"`
	CoverMediaObjectID string     `json:"cover_media_object_id,omitempty"`
	CoverURL           string     `json:"cover_url,omitempty"`
	Title              string     `json:"title"`
	Slug               string     `json:"slug"`
	Excerpt            string     `json:"excerpt"`
	ContentHTML        string     `json:"content_html"`
	CoverAlt           string     `json:"cover_alt,omitempty"`
	Status             string     `json:"status"`
	SEOTitle           string     `json:"seo_title,omitempty"`
	SEODescription     string     `json:"seo_description,omitempty"`
	CanonicalURL       string     `json:"canonical_url,omitempty"`
	PublishedAt        *time.Time `json:"published_at,omitempty"`
	ScheduledAt        *time.Time `json:"scheduled_at,omitempty"`
	Tags               []Tag      `json:"tags"`
	ReadingMinutes     int        `json:"reading_minutes"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}
type WriteRequest struct {
	Title              string     `json:"title"`
	Slug               string     `json:"slug"`
	Excerpt            string     `json:"excerpt"`
	ContentHTML        string     `json:"content_html"`
	CategoryID         string     `json:"category_id"`
	TagIDs             []string   `json:"tag_ids"`
	CoverMediaObjectID string     `json:"cover_media_object_id"`
	CoverAlt           string     `json:"cover_alt"`
	Status             string     `json:"status"`
	SEOTitle           string     `json:"seo_title"`
	SEODescription     string     `json:"seo_description"`
	CanonicalURL       string     `json:"canonical_url"`
	ScheduledAt        *time.Time `json:"scheduled_at"`
}
type Page struct {
	Items    []Post `json:"items"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Total    int64  `json:"total"`
	HasMore  bool   `json:"has_more"`
}

type Repository interface {
	FeatureEnabled(context.Context) (bool, error)
	IsAdmin(context.Context, string) (bool, error)
	PublishDue(context.Context, time.Time) error
	ListPublic(context.Context, string, int, int) (Page, error)
	GetPublic(context.Context, string) (Post, error)
	Related(context.Context, string, string, int) ([]Post, error)
	ListAdmin(context.Context, string, int, int) (Page, error)
	GetAdmin(context.Context, string) (Post, error)
	Create(context.Context, string, WriteRequest, string, string) (Post, error)
	Update(context.Context, string, string, WriteRequest, string, string) (Post, error)
	Archive(context.Context, string, string, string, string) (Post, error)
	Delete(context.Context, string, string, string, string) error
	ListCategories(context.Context) ([]Category, error)
	SaveCategory(context.Context, string, Category, string, string) (Category, error)
	DeleteCategory(context.Context, string, string, string, string) error
	ListTags(context.Context) ([]Tag, error)
	SaveTag(context.Context, string, Tag, string, string) (Tag, error)
	DeleteTag(context.Context, string, string, string, string) error
	PublicMediaKey(context.Context, string) (string, error)
}
type Service struct {
	Repository Repository
	Now        func() time.Time
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s Service) Enabled(ctx context.Context) (bool, error) { return s.Repository.FeatureEnabled(ctx) }
func (s Service) Public(ctx context.Context, category string, page, size int) (Page, error) {
	enabled, err := s.Enabled(ctx)
	if err != nil {
		return Page{}, err
	}
	if !enabled {
		return Page{}, ErrNotFound
	}
	if err = s.Repository.PublishDue(ctx, s.now()); err != nil {
		return Page{}, err
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 12
	}
	if size > 30 {
		size = 30
	}
	return s.Repository.ListPublic(ctx, strings.TrimSpace(category), page, size)
}
func (s Service) Article(ctx context.Context, slug string) (Post, []Post, error) {
	enabled, err := s.Enabled(ctx)
	if err != nil || !enabled {
		if err == nil {
			err = ErrNotFound
		}
		return Post{}, nil, err
	}
	if err = s.Repository.PublishDue(ctx, s.now()); err != nil {
		return Post{}, nil, err
	}
	p, err := s.Repository.GetPublic(ctx, slug)
	if err != nil {
		return Post{}, nil, err
	}
	related, err := s.Repository.Related(ctx, p.ID, p.CategoryID, 3)
	return p, related, err
}
func (s Service) requireAdmin(ctx context.Context, actor string) error {
	if actor == "" {
		return ErrForbidden
	}
	ok, err := s.Repository.IsAdmin(ctx, actor)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return nil
}
func (s Service) AdminList(ctx context.Context, actor, status string, page, size int) (Page, error) {
	if err := s.requireAdmin(ctx, actor); err != nil {
		return Page{}, err
	}
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 30
	}
	return s.Repository.ListAdmin(ctx, strings.ToUpper(strings.TrimSpace(status)), page, size)
}
func (s Service) AdminGet(ctx context.Context, actor, id string) (Post, error) {
	if err := s.requireAdmin(ctx, actor); err != nil {
		return Post{}, err
	}
	return s.Repository.GetAdmin(ctx, id)
}
func validateWrite(in *WriteRequest) error {
	in.Title = strings.TrimSpace(in.Title)
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	in.Excerpt = strings.TrimSpace(in.Excerpt)
	in.CoverAlt = strings.TrimSpace(in.CoverAlt)
	in.Status = strings.ToUpper(strings.TrimSpace(in.Status))
	in.SEOTitle = strings.TrimSpace(in.SEOTitle)
	in.SEODescription = strings.TrimSpace(in.SEODescription)
	in.CanonicalURL = strings.TrimSpace(in.CanonicalURL)
	if len([]rune(in.Title)) < 1 || len([]rune(in.Title)) > 220 || !slugPattern.MatchString(in.Slug) || len([]rune(in.Excerpt)) < 1 || len([]rune(in.Excerpt)) > 600 || len(in.ContentHTML) < 1 || len(in.ContentHTML) > 200000 || len([]rune(in.CoverAlt)) > 300 || len([]rune(in.SEOTitle)) > 220 || len([]rune(in.SEODescription)) > 320 {
		return ErrInvalid
	}
	switch in.Status {
	case "DRAFT", "PUBLISHED", "SCHEDULED":
	default:
		return ErrInvalid
	}
	if in.Status == "SCHEDULED" && (in.ScheduledAt == nil || !in.ScheduledAt.After(time.Now().UTC())) {
		return ErrInvalid
	}
	if in.CanonicalURL != "" {
		u, err := url.Parse(in.CanonicalURL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return ErrInvalid
		}
	}
	clean, err := Sanitize(in.ContentHTML)
	if err != nil || strings.TrimSpace(clean) == "" {
		return ErrInvalid
	}
	in.ContentHTML = clean
	if len(in.TagIDs) > 20 {
		return ErrInvalid
	}
	return nil
}
func (s Service) Create(ctx context.Context, actor string, in WriteRequest, reason, requestID string) (Post, error) {
	if err := s.requireAdmin(ctx, actor); err != nil {
		return Post{}, err
	}
	if strings.TrimSpace(reason) == "" || validateWrite(&in) != nil {
		return Post{}, ErrInvalid
	}
	return s.Repository.Create(ctx, actor, in, strings.TrimSpace(reason), requestID)
}
func (s Service) Update(ctx context.Context, actor, id string, in WriteRequest, reason, requestID string) (Post, error) {
	if err := s.requireAdmin(ctx, actor); err != nil {
		return Post{}, err
	}
	if strings.TrimSpace(reason) == "" || validateWrite(&in) != nil {
		return Post{}, ErrInvalid
	}
	return s.Repository.Update(ctx, actor, id, in, strings.TrimSpace(reason), requestID)
}
func (s Service) Archive(ctx context.Context, actor, id, reason, requestID string) (Post, error) {
	if err := s.requireAdmin(ctx, actor); err != nil {
		return Post{}, err
	}
	if strings.TrimSpace(reason) == "" {
		return Post{}, ErrInvalid
	}
	return s.Repository.Archive(ctx, actor, id, strings.TrimSpace(reason), requestID)
}
func (s Service) Delete(ctx context.Context, actor, id, reason, requestID string) error {
	if err := s.requireAdmin(ctx, actor); err != nil {
		return err
	}
	if strings.TrimSpace(reason) == "" {
		return ErrInvalid
	}
	return s.Repository.Delete(ctx, actor, id, strings.TrimSpace(reason), requestID)
}
