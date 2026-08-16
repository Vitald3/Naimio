package reviews

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"freelance/apps/api/internal/platform/contentmoderation"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrUnauthorized = errors.New("authentication required")
	ErrNotFound     = errors.New("review target not found")
	ErrInvalid      = errors.New("invalid review")
	ErrModeration   = errors.New("review failed automatic moderation")
	ErrIneligible   = errors.New("review not eligible")
	ErrConflict     = errors.New("review already exists")
	ErrForbidden    = errors.New("moderator role required")
)

type Input struct {
	RatingOverall  int            `json:"rating_overall"`
	WouldWorkAgain *bool          `json:"would_work_again"`
	Text           string         `json:"text"`
	Dimensions     map[string]int `json:"dimensions"`
}
type Item struct {
	ID             string         `json:"id"`
	ProjectID      string         `json:"project_id"`
	ProjectTitle   string         `json:"project_title,omitempty"`
	ReviewerID     string         `json:"-"`
	RevieweeID     string         `json:"-"`
	ReviewerName   string         `json:"reviewer_name,omitempty"`
	ReviewerRole   string         `json:"reviewer_role"`
	RatingOverall  int            `json:"rating_overall"`
	WouldWorkAgain *bool          `json:"would_work_again,omitempty"`
	Text           string         `json:"text,omitempty"`
	Dimensions     map[string]int `json:"dimensions"`
	Status         string         `json:"status"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}
type Cursor struct {
	At time.Time
	ID string
}
type Page struct {
	Items      []Item
	NextCursor *Cursor
}
type TrustStats struct {
	NativeRating           *float64  `json:"native_rating,omitempty"`
	ReviewsCount           int       `json:"reviews_count"`
	CompletedProjectsCount int       `json:"completed_projects_count"`
	RecommendationRate     *float64  `json:"recommendation_rate,omitempty"`
	UpdatedAt              time.Time `json:"updated_at"`
}
type PublicProfileData struct {
	Reviews Page
	Trust   TrustStats
}

type Repository interface {
	Create(context.Context, string, string, Input) (Item, error)
	ListPublic(context.Context, string, *Cursor, int) (PublicProfileData, error)
	ListGiven(context.Context, string, *Cursor, int) (Page, error)
	Recalculate(context.Context, string) (TrustStats, error)
	Report(context.Context, string, string, ReportInput) error
	Moderate(context.Context, string, string, string, string) (Item, error)
}
type ReportInput struct {
	ReasonCode  string `json:"reason_code"`
	Description string `json:"description"`
}
type Service struct{ Repository Repository }

func (s Service) Create(ctx context.Context, actor, project string, in Input) (Item, error) {
	if strings.TrimSpace(in.Text) != "" && contentmoderation.LooksLikeJunk(in.Text) {
		return Item{}, ErrModeration
	}
	if actor == "" {
		return Item{}, ErrUnauthorized
	}
	if !validUUID(project) || validateInput(in, "CUSTOMER") != nil && validateInput(in, "FREELANCER") != nil {
		return Item{}, ErrInvalid
	}
	in.Text = strings.TrimSpace(in.Text)
	return s.Repository.Create(ctx, actor, strings.ToLower(project), in)
}
func (s Service) Public(ctx context.Context, username string, c *Cursor, l int) (PublicProfileData, error) {
	username = strings.TrimSpace(username)
	if username == "" || len(username) > 40 {
		return PublicProfileData{}, ErrNotFound
	}
	return s.Repository.ListPublic(ctx, username, c, bound(l))
}
func (s Service) Given(ctx context.Context, actor string, c *Cursor, l int) (Page, error) {
	if actor == "" {
		return Page{}, ErrUnauthorized
	}
	return s.Repository.ListGiven(ctx, actor, c, bound(l))
}
func (s Service) Recalculate(ctx context.Context, user string) (TrustStats, error) {
	if !validUUID(user) {
		return TrustStats{}, ErrInvalid
	}
	return s.Repository.Recalculate(ctx, strings.ToLower(user))
}
func (s Service) Report(ctx context.Context, actor, id string, in ReportInput) error {
	if actor == "" {
		return ErrUnauthorized
	}
	if !validUUID(id) || !validReportReason(in.ReasonCode) || len([]rune(in.Description)) > 2000 {
		return ErrInvalid
	}
	in.Description = strings.TrimSpace(in.Description)
	return s.Repository.Report(ctx, actor, strings.ToLower(id), in)
}
func (s Service) Moderate(ctx context.Context, actor, id, action, reason string) (Item, error) {
	if actor == "" {
		return Item{}, ErrUnauthorized
	}
	if !validUUID(id) || (action != "hide" && action != "restore" && action != "reject" && action != "delete") || strings.TrimSpace(reason) == "" || len([]rune(reason)) > 1000 {
		return Item{}, ErrInvalid
	}
	return s.Repository.Moderate(ctx, actor, strings.ToLower(id), action, strings.TrimSpace(reason))
}
func validReportReason(v string) bool {
	switch v {
	case "HARASSMENT_ABUSE", "SPAM", "PERSONAL_DATA", "IRRELEVANT", "MANIPULATION_CONFLICT", "OTHER":
		return true
	}
	return false
}
func bound(v int) int {
	if v < 1 {
		return 20
	}
	if v > 50 {
		return 50
	}
	return v
}
func validateInput(in Input, role string) error {
	if in.RatingOverall < 1 || in.RatingOverall > 5 || len([]rune(in.Text)) > 5000 || len(in.Dimensions) < 1 || len(in.Dimensions) > 4 {
		return ErrInvalid
	}
	allowed := map[string]bool{}
	if role == "CUSTOMER" {
		for _, v := range []string{"QUALITY", "DEADLINE", "COMMUNICATION", "BUDGET_ACCURACY"} {
			allowed[v] = true
		}
	} else {
		for _, v := range []string{"BRIEF_QUALITY", "COMMUNICATION", "PAYMENT_BEHAVIOR", "REASONABLE_REVISIONS"} {
			allowed[v] = true
		}
	}
	for k, v := range in.Dimensions {
		if !allowed[strings.ToUpper(k)] || v < 1 || v > 5 {
			return ErrInvalid
		}
	}
	return nil
}
func validUUID(v string) bool {
	if len(v) != 36 {
		return false
	}
	for i, c := range strings.ToLower(v) {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("secure random unavailable")
	}
	b[6] = b[6]&15 | 64
	b[8] = b[8]&63 | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

type Relationship struct{ Customer, Freelancer, Status string }
type Store struct {
	mu            sync.Mutex
	Items         map[string]Item
	Projects      map[string]Relationship
	Usernames     map[string]string
	Names         map[string]string
	ProjectTitles map[string]string
	Stats         map[string]TrustStats
	Admins        map[string]bool
	Reports       map[string]bool
	Audits        []string
	Signals       []string
	Now           func() time.Time
}

func (s *Store) Create(_ context.Context, actor, project string, in Input) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rel, ok := s.Projects[project]
	if !ok || rel.Status != "COMPLETED" {
		return Item{}, ErrIneligible
	}
	role, reviewee := "", ""
	if actor == rel.Customer {
		role, reviewee = "CUSTOMER", rel.Freelancer
	} else if actor == rel.Freelancer {
		role, reviewee = "FREELANCER", rel.Customer
	} else {
		return Item{}, ErrNotFound
	}
	if actor == reviewee {
		return Item{}, ErrIneligible
	}
	if err := validateInput(in, role); err != nil {
		return Item{}, err
	}
	for _, v := range s.Items {
		if v.ProjectID == project && v.ReviewerID == actor {
			return Item{}, ErrConflict
		}
	}
	now := s.now()
	dims := map[string]int{}
	for k, v := range in.Dimensions {
		dims[strings.ToUpper(k)] = v
	}
	item := Item{ID: newID(), ProjectID: project, ReviewerID: actor, RevieweeID: reviewee, ReviewerRole: role, RatingOverall: in.RatingOverall, WouldWorkAgain: in.WouldWorkAgain, Text: in.Text, Dimensions: dims, Status: "PUBLISHED", CreatedAt: now, UpdatedAt: now}
	if s.Items == nil {
		s.Items = map[string]Item{}
	}
	s.Items[item.ID] = item
	s.recalc(reviewee)
	velocity := 0
	for _, candidate := range s.Items {
		if candidate.ReviewerID == actor && candidate.CreatedAt.After(now.Add(-time.Hour)) {
			velocity++
		}
	}
	if velocity > 5 {
		s.Signals = append(s.Signals, "REVIEW_VELOCITY:"+actor)
	}
	return item, nil
}
func (s *Store) ListPublic(_ context.Context, username string, c *Cursor, l int) (PublicProfileData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.Usernames[strings.ToLower(username)]
	if !ok {
		return PublicProfileData{}, ErrNotFound
	}
	return PublicProfileData{Reviews: s.page(user, "", c, l), Trust: s.Stats[user]}, nil
}
func (s *Store) ListGiven(_ context.Context, actor string, c *Cursor, l int) (Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.page("", actor, c, l), nil
}
func (s *Store) Recalculate(_ context.Context, user string) (TrustStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recalc(user)
	return s.Stats[user], nil
}
func (s *Store) Report(_ context.Context, actor, id string, in ReportInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Items[id]; !ok {
		return ErrNotFound
	}
	key := actor + ":" + id + ":" + in.ReasonCode
	if s.Reports == nil {
		s.Reports = map[string]bool{}
	}
	if s.Reports[key] {
		return ErrConflict
	}
	s.Reports[key] = true
	return nil
}
func (s *Store) Moderate(_ context.Context, actor, id, action, reason string) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Admins[actor] {
		return Item{}, ErrForbidden
	}
	v, ok := s.Items[id]
	if !ok {
		return Item{}, ErrNotFound
	}
	target := "HIDDEN"
	if action == "restore" {
		target = "PUBLISHED"
	}
	if v.Status == target {
		return v, nil
	}
	v.Status = target
	v.UpdatedAt = s.now()
	s.Items[id] = v
	s.recalc(v.RevieweeID)
	s.Audits = append(s.Audits, actor+":"+action+":"+id+":"+reason)
	return v, nil
}
func (s *Store) page(reviewee, reviewer string, c *Cursor, l int) Page {
	items := []Item{}
	for _, v := range s.Items {
		if v.Status == "PUBLISHED" && (reviewee == "" || v.RevieweeID == reviewee) && (reviewer == "" || v.ReviewerID == reviewer) && (c == nil || v.CreatedAt.Before(c.At) || v.CreatedAt.Equal(c.At) && v.ID < c.ID) {
			v.ReviewerName = s.Names[v.ReviewerID]
			v.ProjectTitle = s.ProjectTitles[v.ProjectID]
			items = append(items, v)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].ID > items[j].ID
	})
	p := Page{Items: items}
	if len(items) > l {
		last := items[l-1]
		p.Items = items[:l]
		p.NextCursor = &Cursor{At: last.CreatedAt, ID: last.ID}
	}
	return p
}
func (s *Store) recalc(user string) {
	count, total, answers, positive, completed := 0, 0, 0, 0, 0
	seen := map[string]bool{}
	for _, r := range s.Items {
		if r.RevieweeID == user && r.Status == "PUBLISHED" {
			count++
			total += r.RatingOverall
			if r.WouldWorkAgain != nil {
				answers++
				if *r.WouldWorkAgain {
					positive++
				}
			}
		}
	}
	for id, p := range s.Projects {
		if p.Status == "COMPLETED" && (p.Customer == user || p.Freelancer == user) && !seen[id] {
			seen[id] = true
			completed++
		}
	}
	var rating, recommendation *float64
	if count > 0 {
		v := float64(total) / float64(count)
		rating = &v
	}
	if answers >= 3 {
		v := float64(positive) * 100 / float64(answers)
		recommendation = &v
	}
	s.Stats[user] = TrustStats{NativeRating: rating, ReviewsCount: count, CompletedProjectsCount: completed, RecommendationRate: recommendation, UpdatedAt: s.now()}
}
func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
