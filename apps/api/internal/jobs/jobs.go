package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotFound     = errors.New("vacancy not found")
	ErrInvalidInput = errors.New("invalid vacancy input")
	ErrInvalidState = errors.New("invalid vacancy state")
	ErrConflict     = errors.New("vacancy conflict")
	ErrIneligible   = errors.New("actor is ineligible")
	ErrForbidden    = errors.New("forbidden")
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type Reference struct {
	ID   string `json:"id"`
	Slug string `json:"slug,omitempty"`
	Name string `json:"name,omitempty"`
}

type Company struct {
	ID                 string    `json:"id"`
	OwnerID            string    `json:"-"`
	Name               string    `json:"name"`
	Slug               string    `json:"slug"`
	LogoObjectKey      string    `json:"-"`
	Website            string    `json:"website,omitempty"`
	Description        string    `json:"description,omitempty"`
	VerificationStatus string    `json:"verification_status"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type CompanyInput struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Website     string `json:"website"`
	Description string `json:"description"`
}

type Item struct {
	ID               string      `json:"id"`
	CustomerID       string      `json:"-"`
	CustomerName     string      `json:"customer_name,omitempty"`
	Company          *Company    `json:"company,omitempty"`
	Category         *Reference  `json:"category,omitempty"`
	Title            string      `json:"title"`
	Slug             string      `json:"slug"`
	Description      string      `json:"description"`
	EmploymentType   string      `json:"employment_type"`
	SalaryMinKopecks *int64      `json:"salary_min_kopecks,omitempty"`
	SalaryMaxKopecks *int64      `json:"salary_max_kopecks,omitempty"`
	Currency         string      `json:"currency"`
	Location         string      `json:"location,omitempty"`
	Remote           bool        `json:"remote"`
	ExperienceLevel  string      `json:"experience_level,omitempty"`
	Status           string      `json:"status"`
	ModerationStatus string      `json:"moderation_status,omitempty"`
	ModerationReason string      `json:"moderation_reason,omitempty"`
	Skills           []Reference `json:"skills"`
	PublishedAt      *time.Time  `json:"published_at,omitempty"`
	CreatedAt        time.Time   `json:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at"`
	DeletedAt        *time.Time  `json:"-"`
}

type CreateRequest struct {
	CompanyID        string   `json:"company_id"`
	CategoryID       string   `json:"category_id"`
	Title            string   `json:"title"`
	Slug             string   `json:"slug"`
	Description      string   `json:"description"`
	EmploymentType   string   `json:"employment_type"`
	SalaryMinKopecks *int64   `json:"salary_min_kopecks"`
	SalaryMaxKopecks *int64   `json:"salary_max_kopecks"`
	Currency         string   `json:"currency"`
	Location         string   `json:"location"`
	Remote           bool     `json:"remote"`
	ExperienceLevel  string   `json:"experience_level"`
	SkillIDs         []string `json:"skill_ids"`
}

type PatchRequest struct {
	CompanyID        *string   `json:"company_id"`
	CategoryID       *string   `json:"category_id"`
	Title            *string   `json:"title"`
	Slug             *string   `json:"slug"`
	Description      *string   `json:"description"`
	EmploymentType   *string   `json:"employment_type"`
	SalaryMinKopecks *int64    `json:"salary_min_kopecks"`
	SalaryMaxKopecks *int64    `json:"salary_max_kopecks"`
	Currency         *string   `json:"currency"`
	Location         *string   `json:"location"`
	Remote           *bool     `json:"remote"`
	ExperienceLevel  *string   `json:"experience_level"`
	SkillIDs         *[]string `json:"skill_ids"`
}

type Filter struct {
	Q, Category, Skill, EmploymentType, Location, Experience, Sort string
	MinSalary                                                      *int64
	Remote                                                         *bool
}

type Cursor struct {
	At time.Time
	ID string
}
type Page struct {
	Items      []Item
	NextCursor *Cursor
}

type Application struct {
	ID            string    `json:"id"`
	JobID         string    `json:"job_id"`
	JobTitle      string    `json:"job_title,omitempty"`
	UserID        string    `json:"user_id,omitempty"`
	ApplicantName string    `json:"applicant_name,omitempty"`
	CoverMessage  string    `json:"cover_message,omitempty"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Repository interface {
	CreateCompany(context.Context, string, CompanyInput) (Company, error)
	ListCompanies(context.Context, string) ([]Company, error)
	Create(context.Context, string, CreateRequest) (Item, error)
	ListOwned(context.Context, string, *Cursor, int) (Page, error)
	GetOwned(context.Context, string, string) (Item, error)
	Update(context.Context, string, string, PatchRequest) (Item, error)
	Delete(context.Context, string, string) error
	Transition(context.Context, string, string, string) (Item, error)
	ListPublic(context.Context, Filter, *Cursor, int) (Page, error)
	GetPublic(context.Context, string) (Item, error)
	Apply(context.Context, string, string, string) (Application, error)
	ListMine(context.Context, string, *Cursor, int) ([]Application, error)
	ListApplicants(context.Context, string, string) ([]Application, error)
	SetApplicationStatus(context.Context, string, string, string, string) (Application, error)
	Moderate(context.Context, string, string, string, string) (Item, error)
}

type Store struct {
	mu           sync.RWMutex
	Companies    map[string]Company
	Items        map[string]Item
	Applications map[string]Application
	Categories   map[string]Reference
	Skills       map[string]Reference
	Customers    map[string]bool
	Applicants   map[string]bool
	Admins       map[string]bool
	Events       []string
	Now          func() time.Time
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Store) CreateCompany(_ context.Context, actor string, in CompanyInput) (Company, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Customers[actor] {
		return Company{}, ErrIneligible
	}
	in = normalizeCompany(in)
	if err := validateCompany(in); err != nil {
		return Company{}, err
	}
	for _, c := range s.Companies {
		if c.OwnerID == actor && c.Slug == in.Slug {
			return Company{}, ErrConflict
		}
	}
	id, _ := newUUID()
	now := s.now()
	c := Company{ID: id, OwnerID: actor, Name: in.Name, Slug: in.Slug, Website: in.Website, Description: in.Description, VerificationStatus: "UNVERIFIED", CreatedAt: now, UpdatedAt: now}
	if s.Companies == nil {
		s.Companies = map[string]Company{}
	}
	s.Companies[id] = c
	return c, nil
}

func (s *Store) ListCompanies(_ context.Context, actor string) ([]Company, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []Company{}
	for _, c := range s.Companies {
		if c.OwnerID == actor {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) Create(_ context.Context, actor string, in CreateRequest) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Customers[actor] {
		return Item{}, ErrIneligible
	}
	id, _ := newUUID()
	item := fromCreate(actor, id, in, s.now())
	if err := s.resolve(&item); err != nil {
		return Item{}, err
	}
	if err := Validate(item); err != nil {
		return Item{}, err
	}
	for _, v := range s.Items {
		if v.CustomerID == actor && v.Slug == item.Slug && v.DeletedAt == nil {
			return Item{}, ErrConflict
		}
	}
	if s.Items == nil {
		s.Items = map[string]Item{}
	}
	s.Items[id] = item
	return item, nil
}

func (s *Store) GetOwned(_ context.Context, actor, id string) (Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.Items[id]
	if !ok || v.CustomerID != actor || v.DeletedAt != nil {
		return Item{}, ErrNotFound
	}
	return v, nil
}
func (s *Store) ListOwned(_ context.Context, actor string, c *Cursor, limit int) (Page, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []Item{}
	for _, v := range s.Items {
		if v.CustomerID == actor && v.DeletedAt == nil && after(v.CreatedAt, v.ID, c) {
			out = append(out, v)
		}
	}
	return page(out, capped(limit), false), nil
}
func (s *Store) Update(_ context.Context, actor, id string, p PatchRequest) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.Items[id]
	if !ok || v.CustomerID != actor || v.DeletedAt != nil {
		return Item{}, ErrNotFound
	}
	if v.Status != "DRAFT" {
		return Item{}, ErrInvalidState
	}
	merge(&v, p)
	if err := s.resolve(&v); err != nil {
		return Item{}, err
	}
	if err := Validate(v); err != nil {
		return Item{}, err
	}
	for oid, o := range s.Items {
		if oid != id && o.CustomerID == actor && o.Slug == v.Slug && o.DeletedAt == nil {
			return Item{}, ErrConflict
		}
	}
	s.Items[id] = v
	return v, nil
}
func (s *Store) Delete(_ context.Context, actor, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.Items[id]
	if !ok || v.CustomerID != actor || v.DeletedAt != nil {
		return ErrNotFound
	}
	if v.Status == "PUBLISHED" {
		return ErrInvalidState
	}
	n := s.now()
	v.Status = "ARCHIVED"
	v.DeletedAt = &n
	v.UpdatedAt = n
	s.Items[id] = v
	return nil
}
func (s *Store) Transition(_ context.Context, actor, id, action string) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.Items[id]
	if !ok || v.CustomerID != actor || v.DeletedAt != nil {
		return Item{}, ErrNotFound
	}
	target, ok := transition(v.Status, action)
	if !ok {
		return Item{}, ErrInvalidState
	}
	if action == "publish" {
		if err := s.resolve(&v); err != nil {
			return Item{}, err
		}
		if err := Validate(v); err != nil {
			return Item{}, err
		}
	}
	n := s.now()
	v.Status = target
	v.UpdatedAt = n
	if action == "publish" && v.PublishedAt == nil {
		v.PublishedAt = &n
		s.Events = append(s.Events, "vacancy_published")
	}
	s.Items[id] = v
	return v, nil
}
func (s *Store) ListPublic(_ context.Context, f Filter, c *Cursor, limit int) (Page, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := ValidateFilter(f); err != nil {
		return Page{}, err
	}
	out := []Item{}
	for _, v := range s.Items {
		if v.Status == "PUBLISHED" && v.ModerationStatus == "VISIBLE" && v.DeletedAt == nil && v.PublishedAt != nil && after(*v.PublishedAt, v.ID, c) && match(v, f) {
			v.CustomerID = ""
			out = append(out, v)
		}
	}
	return page(out, capped(limit), true), nil
}
func (s *Store) GetPublic(_ context.Context, ref string) (Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, v := range s.Items {
		if (v.ID == ref || v.Slug == ref) && v.Status == "PUBLISHED" && v.ModerationStatus == "VISIBLE" && v.DeletedAt == nil {
			v.CustomerID = ""
			return v, nil
		}
	}
	return Item{}, ErrNotFound
}

func (s *Store) Apply(_ context.Context, actor, jobID, message string) (Application, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.Items[jobID]
	if !ok || v.Status != "PUBLISHED" || v.ModerationStatus != "VISIBLE" || v.DeletedAt != nil {
		return Application{}, ErrNotFound
	}
	if !s.Applicants[actor] || v.CustomerID == actor {
		return Application{}, ErrIneligible
	}
	message = strings.TrimSpace(message)
	if len([]rune(message)) > 5000 {
		return Application{}, ErrInvalidInput
	}
	for _, a := range s.Applications {
		if a.JobID == jobID && a.UserID == actor {
			return Application{}, ErrConflict
		}
	}
	id, _ := newUUID()
	n := s.now()
	a := Application{ID: id, JobID: jobID, JobTitle: v.Title, UserID: actor, CoverMessage: message, Status: "SUBMITTED", CreatedAt: n, UpdatedAt: n}
	if s.Applications == nil {
		s.Applications = map[string]Application{}
	}
	s.Applications[id] = a
	s.Events = append(s.Events, "vacancy_application_submitted")
	a.UserID = ""
	return a, nil
}
func (s *Store) ListMine(_ context.Context, actor string, _ *Cursor, _ int) ([]Application, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []Application{}
	for _, a := range s.Applications {
		if a.UserID == actor {
			a.UserID = ""
			a.CoverMessage = ""
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
func (s *Store) ListApplicants(_ context.Context, actor, jobID string) ([]Application, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.Items[jobID]
	if !ok || v.CustomerID != actor {
		return nil, ErrNotFound
	}
	out := []Application{}
	for _, a := range s.Applications {
		if a.JobID == jobID {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
func (s *Store) SetApplicationStatus(_ context.Context, actor, jobID, appID, status string) (Application, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.Items[jobID]
	if !ok || v.CustomerID != actor {
		return Application{}, ErrNotFound
	}
	a, ok := s.Applications[appID]
	if !ok || a.JobID != jobID {
		return Application{}, ErrNotFound
	}
	status = strings.ToUpper(strings.TrimSpace(status))
	if !oneOf(status, "VIEWED", "SHORTLISTED", "REJECTED", "ACCEPTED") || !applicationTransition(a.Status, status) {
		return Application{}, ErrInvalidState
	}
	a.Status = status
	a.UpdatedAt = s.now()
	s.Applications[appID] = a
	if status == "SHORTLISTED" {
		s.Events = append(s.Events, "vacancy_application_shortlisted")
	}
	return a, nil
}
func (s *Store) Moderate(_ context.Context, actor, id, action, reason string) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Admins[actor] {
		return Item{}, ErrForbidden
	}
	v, ok := s.Items[id]
	if !ok || v.DeletedAt != nil {
		return Item{}, ErrNotFound
	}
	action = strings.ToUpper(strings.TrimSpace(action))
	if len([]rune(strings.TrimSpace(reason))) < 3 || len([]rune(reason)) > 1000 || !oneOf(action, "HIDE", "RESTORE") {
		return Item{}, ErrInvalidInput
	}
	if action == "HIDE" {
		v.ModerationStatus = "HIDDEN"
	} else {
		v.ModerationStatus = "VISIBLE"
	}
	v.UpdatedAt = s.now()
	s.Items[id] = v
	s.Events = append(s.Events, "vacancy_moderated")
	return v, nil
}

func Validate(v Item) error {
	if !uuidPattern.MatchString(v.CustomerID) || len([]rune(v.Title)) < 1 || len([]rune(v.Title)) > 200 || !slugPattern.MatchString(v.Slug) || len(v.Slug) > 240 || len([]rune(v.Description)) < 1 || len([]rune(v.Description)) > 20000 {
		return ErrInvalidInput
	}
	if !oneOf(v.EmploymentType, "FULL_TIME", "PART_TIME", "CONTRACT", "INTERNSHIP") || !oneOf(v.ExperienceLevel, "", "JUNIOR", "MIDDLE", "SENIOR", "LEAD", "ANY") || v.Currency != "RUB" || len([]rune(v.Location)) > 160 {
		return ErrInvalidInput
	}
	if v.SalaryMinKopecks != nil && *v.SalaryMinKopecks <= 0 || v.SalaryMaxKopecks != nil && *v.SalaryMaxKopecks <= 0 || v.SalaryMinKopecks != nil && v.SalaryMaxKopecks != nil && *v.SalaryMinKopecks > *v.SalaryMaxKopecks || len(v.Skills) > 30 {
		return ErrInvalidInput
	}
	return nil
}
func ValidateFilter(f Filter) error {
	if len([]rune(f.Q)) > 120 || len([]rune(f.Location)) > 160 || (f.EmploymentType != "" && !oneOf(f.EmploymentType, "FULL_TIME", "PART_TIME", "CONTRACT", "INTERNSHIP")) || (f.Experience != "" && !oneOf(f.Experience, "JUNIOR", "MIDDLE", "SENIOR", "LEAD", "ANY")) || (f.Sort != "" && !oneOf(f.Sort, "NEWEST", "RELEVANCE")) || (f.MinSalary != nil && *f.MinSalary < 0) {
		return ErrInvalidInput
	}
	for _, v := range []string{f.Category, f.Skill} {
		if v != "" && !uuidPattern.MatchString(v) && !slugPattern.MatchString(v) {
			return ErrInvalidInput
		}
	}
	return nil
}
func validateCompany(v CompanyInput) error {
	if len([]rune(v.Name)) < 1 || len([]rune(v.Name)) > 180 || !slugPattern.MatchString(v.Slug) || len(v.Slug) > 220 || len([]rune(v.Description)) > 4000 {
		return ErrInvalidInput
	}
	if v.Website != "" {
		u, e := url.ParseRequestURI(v.Website)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return ErrInvalidInput
		}
	}
	return nil
}
func normalizeCompany(v CompanyInput) CompanyInput {
	v.Name = strings.TrimSpace(v.Name)
	v.Slug = strings.ToLower(strings.TrimSpace(v.Slug))
	v.Website = strings.TrimSpace(v.Website)
	v.Description = strings.TrimSpace(v.Description)
	return v
}
func fromCreate(actor, id string, in CreateRequest, n time.Time) Item {
	return Item{ID: id, CustomerID: actor, Company: &Company{ID: normalizeID(in.CompanyID)}, Category: optionalRef(in.CategoryID), Title: strings.TrimSpace(in.Title), Slug: strings.ToLower(strings.TrimSpace(in.Slug)), Description: strings.TrimSpace(in.Description), EmploymentType: strings.ToUpper(strings.TrimSpace(in.EmploymentType)), SalaryMinKopecks: in.SalaryMinKopecks, SalaryMaxKopecks: in.SalaryMaxKopecks, Currency: strings.ToUpper(strings.TrimSpace(in.Currency)), Location: strings.TrimSpace(in.Location), Remote: in.Remote, ExperienceLevel: strings.ToUpper(strings.TrimSpace(in.ExperienceLevel)), Status: "DRAFT", ModerationStatus: "VISIBLE", Skills: refs(in.SkillIDs), CreatedAt: n, UpdatedAt: n}
}
func merge(v *Item, p PatchRequest) {
	if p.CompanyID != nil {
		v.Company = &Company{ID: normalizeID(*p.CompanyID)}
	}
	if p.CategoryID != nil {
		v.Category = optionalRef(*p.CategoryID)
	}
	if p.Title != nil {
		v.Title = strings.TrimSpace(*p.Title)
	}
	if p.Slug != nil {
		v.Slug = strings.ToLower(strings.TrimSpace(*p.Slug))
	}
	if p.Description != nil {
		v.Description = strings.TrimSpace(*p.Description)
	}
	if p.EmploymentType != nil {
		v.EmploymentType = strings.ToUpper(strings.TrimSpace(*p.EmploymentType))
	}
	if p.SalaryMinKopecks != nil {
		v.SalaryMinKopecks = p.SalaryMinKopecks
	}
	if p.SalaryMaxKopecks != nil {
		v.SalaryMaxKopecks = p.SalaryMaxKopecks
	}
	if p.Currency != nil {
		v.Currency = strings.ToUpper(strings.TrimSpace(*p.Currency))
	}
	if p.Location != nil {
		v.Location = strings.TrimSpace(*p.Location)
	}
	if p.Remote != nil {
		v.Remote = *p.Remote
	}
	if p.ExperienceLevel != nil {
		v.ExperienceLevel = strings.ToUpper(strings.TrimSpace(*p.ExperienceLevel))
	}
	if p.SkillIDs != nil {
		v.Skills = refs(*p.SkillIDs)
	}
	v.UpdatedAt = time.Now().UTC()
}
func (s *Store) resolve(v *Item) error {
	if v.Company != nil && v.Company.ID != "" {
		c, ok := s.Companies[v.Company.ID]
		if !ok || c.OwnerID != v.CustomerID {
			return ErrInvalidInput
		}
		c.OwnerID = ""
		v.Company = &c
	} else {
		v.Company = nil
	}
	if v.Category != nil {
		c, ok := s.Categories[v.Category.ID]
		if !ok {
			return ErrInvalidInput
		}
		v.Category = &c
	}
	seen := map[string]bool{}
	for i, r := range v.Skills {
		if seen[r.ID] {
			return ErrInvalidInput
		}
		seen[r.ID] = true
		resolved, ok := s.Skills[r.ID]
		if !ok {
			return ErrInvalidInput
		}
		v.Skills[i] = resolved
	}
	return nil
}
func match(v Item, f Filter) bool {
	q := strings.ToLower(f.Q)
	if q != "" && !strings.Contains(strings.ToLower(v.Title+" "+v.Description), q) {
		return false
	}
	if f.Category != "" && (v.Category == nil || (v.Category.ID != f.Category && v.Category.Slug != f.Category)) {
		return false
	}
	if f.Skill != "" {
		ok := false
		for _, s := range v.Skills {
			if s.ID == f.Skill || s.Slug == f.Skill {
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	if f.EmploymentType != "" && v.EmploymentType != f.EmploymentType || f.Experience != "" && v.ExperienceLevel != f.Experience || f.Remote != nil && v.Remote != *f.Remote || f.Location != "" && !strings.Contains(strings.ToLower(v.Location), strings.ToLower(f.Location)) {
		return false
	}
	if f.MinSalary != nil && (v.SalaryMaxKopecks == nil || *v.SalaryMaxKopecks < *f.MinSalary) {
		return false
	}
	return true
}
func page(v []Item, limit int, published bool) Page {
	sort.Slice(v, func(i, j int) bool {
		a, b := v[i].CreatedAt, v[j].CreatedAt
		if published {
			a, b = *v[i].PublishedAt, *v[j].PublishedAt
		}
		if !a.Equal(b) {
			return a.After(b)
		}
		return v[i].ID > v[j].ID
	})
	p := Page{Items: v}
	if len(v) > limit {
		last := v[limit-1]
		at := last.CreatedAt
		if published {
			at = *last.PublishedAt
		}
		p.Items = v[:limit]
		p.NextCursor = &Cursor{At: at, ID: last.ID}
	}
	return p
}
func transition(s, a string) (string, bool) {
	switch a {
	case "publish":
		return "PUBLISHED", s == "DRAFT"
	case "close":
		return "CLOSED", s == "PUBLISHED"
	}
	return "", false
}
func applicationTransition(from, to string) bool {
	switch from {
	case "SUBMITTED":
		return true
	case "VIEWED":
		return to != "VIEWED"
	case "SHORTLISTED":
		return to == "REJECTED" || to == "ACCEPTED"
	default:
		return false
	}
}
func refs(ids []string) []Reference {
	out := make([]Reference, len(ids))
	for i, id := range ids {
		out[i] = Reference{ID: normalizeID(id)}
	}
	return out
}
func optionalRef(id string) *Reference {
	id = normalizeID(id)
	if id == "" {
		return nil
	}
	return &Reference{ID: id}
}
func after(at time.Time, id string, c *Cursor) bool {
	return c == nil || at.Before(c.At) || at.Equal(c.At) && id < c.ID
}
func capped(v int) int {
	if v < 1 {
		return 20
	}
	if v > 50 {
		return 50
	}
	return v
}
func normalizeID(v string) string { return strings.ToLower(strings.TrimSpace(v)) }
func oneOf(v string, values ...string) bool {
	for _, x := range values {
		if v == x {
			return true
		}
	}
	return false
}
func newUUID() (string, error) {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", e
	}
	b[6] = (b[6] & 0x0f) | 0x70
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[:8], h[8:12], h[12:16], h[16:20], h[20:]), nil
}
