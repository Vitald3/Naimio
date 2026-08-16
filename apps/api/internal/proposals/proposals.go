package proposals

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotFound     = errors.New("proposal not found")
	ErrInvalid      = errors.New("invalid proposal")
	ErrInvalidState = errors.New("invalid proposal state")
	ErrConflict     = errors.New("proposal conflict")
	ErrIneligible   = errors.New("freelancer is not eligible")
)
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Proposal struct {
	ID                    string     `json:"id"`
	ProjectID             string     `json:"project_id"`
	ProjectTitle          string     `json:"project_title,omitempty"`
	FreelancerID          string     `json:"-"`
	FreelancerDisplayName string     `json:"freelancer_display_name,omitempty"`
	Message               string     `json:"message"`
	PriceKopecks          *int64     `json:"price_kopecks,omitempty"`
	Currency              string     `json:"currency"`
	DeliveryDays          *int       `json:"delivery_days,omitempty"`
	Status                string     `json:"status"`
	SubmittedAt           time.Time  `json:"submitted_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	WithdrawnAt           *time.Time `json:"withdrawn_at,omitempty"`
	SafeDealID            string     `json:"safe_deal_id,omitempty"`
}
type Input struct {
	Message      string `json:"message"`
	PriceKopecks *int64 `json:"price_kopecks"`
	Currency     string `json:"currency"`
	DeliveryDays *int   `json:"delivery_days"`
}
type Cursor struct {
	At time.Time
	ID string
}
type Page struct {
	Items      []Proposal
	NextCursor *Cursor
}
type Assignment struct {
	ID, ProjectID, ProposalID, FreelancerID string
	AgreedPriceKopecks                      *int64
	StartedAt                               time.Time
}
type Repository interface {
	Submit(context.Context, string, string, Input) (Proposal, error)
	ListMine(context.Context, string, *Cursor, int) (Page, error)
	GetMine(context.Context, string, string) (Proposal, error)
	Update(context.Context, string, string, Input) (Proposal, error)
	Withdraw(context.Context, string, string) (Proposal, error)
	ListForProject(context.Context, string, string, *Cursor, int) (Page, error)
	Act(context.Context, string, string, string, string) (Proposal, error)
}
type Project struct {
	ID, CustomerID, Status string
	ProposalCount          int
}
type Store struct {
	mu                 sync.Mutex
	Items              map[string]Proposal
	Projects           map[string]Project
	Assignments        map[string]Assignment
	FreelancerEligible map[string]bool
	Now                func() time.Time
	DealCreator        interface {
		CreateFromAssignment(context.Context, string, string, string, string, int64) error
		ResolveByProject(context.Context, string) (string, error)
	}
}

func (s *Store) Submit(_ context.Context, actor, projectID string, in Input) (Proposal, error) {
	if !uuidPattern.MatchString(strings.ToLower(projectID)) {
		return Proposal{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.Projects[projectID]
	if !ok || !(p.Status == "OPEN" || p.Status == "MATCHING") {
		return Proposal{}, ErrNotFound
	}
	if actor == p.CustomerID {
		return Proposal{}, ErrIneligible
	}
	if s.FreelancerEligible != nil && !s.FreelancerEligible[actor] {
		return Proposal{}, ErrIneligible
	}
	if err := validate(in); err != nil {
		return Proposal{}, err
	}
	for _, v := range s.Items {
		if v.ProjectID == projectID && v.FreelancerID == actor {
			return Proposal{}, ErrConflict
		}
	}
	id, err := uuid()
	if err != nil {
		return Proposal{}, err
	}
	now := s.now()
	value := Proposal{ID: id, ProjectID: projectID, FreelancerID: actor, Message: strings.TrimSpace(in.Message), PriceKopecks: in.PriceKopecks, Currency: "RUB", DeliveryDays: in.DeliveryDays, Status: "PENDING", SubmittedAt: now, UpdatedAt: now}
	if s.Items == nil {
		s.Items = map[string]Proposal{}
	}
	s.Items[id] = value
	p.ProposalCount++
	s.Projects[projectID] = p
	return value, nil
}
func (s *Store) ListMine(_ context.Context, actor string, c *Cursor, l int) (Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return page(filter(s.Items, func(v Proposal) bool { return v.FreelancerID == actor }), c, l), nil
}
func (s *Store) GetMine(_ context.Context, actor, id string) (Proposal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.Items[id]
	if !ok || v.FreelancerID != actor {
		return Proposal{}, ErrNotFound
	}
	return v, nil
}
func (s *Store) Update(_ context.Context, actor, id string, in Input) (Proposal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.Items[id]
	if !ok || v.FreelancerID != actor {
		return Proposal{}, ErrNotFound
	}
	if v.Status != "PENDING" {
		return Proposal{}, ErrInvalidState
	}
	if err := validate(in); err != nil {
		return Proposal{}, err
	}
	v.Message, v.PriceKopecks, v.Currency, v.DeliveryDays, v.UpdatedAt = strings.TrimSpace(in.Message), in.PriceKopecks, "RUB", in.DeliveryDays, s.now()
	s.Items[id] = v
	return v, nil
}
func (s *Store) Withdraw(_ context.Context, actor, id string) (Proposal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.Items[id]
	if !ok || v.FreelancerID != actor {
		return Proposal{}, ErrNotFound
	}
	if v.Status == "WITHDRAWN" {
		return v, nil
	}
	if v.Status != "PENDING" && v.Status != "SHORTLISTED" {
		return Proposal{}, ErrInvalidState
	}
	now := s.now()
	v.Status, v.WithdrawnAt, v.UpdatedAt = "WITHDRAWN", &now, now
	s.Items[id] = v
	return v, nil
}
func (s *Store) ListForProject(_ context.Context, actor, projectID string, c *Cursor, l int) (Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.Projects[projectID]
	if !ok || p.CustomerID != actor {
		return Page{}, ErrNotFound
	}
	return page(filter(s.Items, func(v Proposal) bool { return v.ProjectID == projectID }), c, l), nil
}
func (s *Store) Act(_ context.Context, actor, projectID, id, action string) (Proposal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	project, ok := s.Projects[projectID]
	if !ok || project.CustomerID != actor {
		return Proposal{}, ErrNotFound
	}
	value, ok := s.Items[id]
	if !ok || value.ProjectID != projectID {
		return Proposal{}, ErrNotFound
	}
	if action == "accept" && value.Status == "ACCEPTED" {
		return value, nil
	}
	target := ""
	switch action {
	case "shortlist":
		if value.Status != "PENDING" {
			return Proposal{}, ErrInvalidState
		}
		target = "SHORTLISTED"
	case "reject":
		if value.Status != "PENDING" && value.Status != "SHORTLISTED" {
			return Proposal{}, ErrInvalidState
		}
		target = "REJECTED"
	case "accept":
		if project.Status != "OPEN" && project.Status != "MATCHING" || value.Status != "PENDING" && value.Status != "SHORTLISTED" {
			return Proposal{}, ErrInvalidState
		}
		if value.PriceKopecks == nil || *value.PriceKopecks <= 0 {
			return Proposal{}, ErrInvalid
		}
		for _, v := range s.Items {
			if v.ProjectID == projectID && v.Status == "ACCEPTED" {
				return Proposal{}, ErrConflict
			}
		}
		target = "ACCEPTED"
	default:
		return Proposal{}, ErrInvalidState
	}
	now := s.now()
	value.Status, value.UpdatedAt = target, now
	s.Items[id] = value
	if action == "accept" {
		assignmentID, err := uuid()
		if err != nil {
			return Proposal{}, err
		}
		if s.Assignments == nil {
			s.Assignments = map[string]Assignment{}
		}
		s.Assignments[projectID] = Assignment{ID: assignmentID, ProjectID: projectID, ProposalID: id, FreelancerID: value.FreelancerID, AgreedPriceKopecks: value.PriceKopecks, StartedAt: now}
		if s.DealCreator != nil {
			if err = s.DealCreator.CreateFromAssignment(context.Background(), projectID, assignmentID, project.CustomerID, value.FreelancerID, *value.PriceKopecks); err != nil {
				delete(s.Assignments, projectID)
				return Proposal{}, err
			}
			if safeDealID, resolveErr := s.DealCreator.ResolveByProject(context.Background(), projectID); resolveErr == nil {
				value.SafeDealID = safeDealID
				s.Items[id] = value
			}
		}
		project.Status = "AWAITING_FUNDING"
		s.Projects[projectID] = project
		for otherID, other := range s.Items {
			if other.ProjectID == projectID && otherID != id && (other.Status == "PENDING" || other.Status == "SHORTLISTED") {
				other.Status, other.UpdatedAt = "REJECTED", now
				s.Items[otherID] = other
			}
		}
	}
	return value, nil
}
func validate(in Input) error {
	n := len([]rune(strings.TrimSpace(in.Message)))
	if n < 1 || n > 5000 || in.Currency != "RUB" {
		return ErrInvalid
	}
	if in.PriceKopecks != nil && *in.PriceKopecks <= 0 {
		return ErrInvalid
	}
	if in.DeliveryDays != nil && (*in.DeliveryDays < 1 || *in.DeliveryDays > 3650) {
		return ErrInvalid
	}
	return nil
}
func filter(items map[string]Proposal, include func(Proposal) bool) []Proposal {
	out := []Proposal{}
	for _, v := range items {
		if include(v) {
			out = append(out, v)
		}
	}
	return out
}
func page(items []Proposal, c *Cursor, l int) Page {
	if l < 1 {
		l = 20
	}
	if l > 50 {
		l = 50
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].SubmittedAt.Equal(items[j].SubmittedAt) {
			return items[i].SubmittedAt.After(items[j].SubmittedAt)
		}
		return items[i].ID > items[j].ID
	})
	if c != nil {
		kept := items[:0]
		for _, v := range items {
			if v.SubmittedAt.Before(c.At) || v.SubmittedAt.Equal(c.At) && v.ID < c.ID {
				kept = append(kept, v)
			}
		}
		items = kept
	}
	p := Page{Items: items}
	if len(items) > l {
		last := items[l-1]
		p.Items = items[:l]
		p.NextCursor = &Cursor{At: last.SubmittedAt, ID: last.ID}
	}
	return p
}
func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func uuid() (string, error) {
	var v [16]byte
	if _, err := rand.Read(v[:]); err != nil {
		return "", err
	}
	v[6] = (v[6] & 15) | 0x40
	v[8] = (v[8] & 63) | 0x80
	var b [36]byte
	hex.Encode(b[0:8], v[0:4])
	b[8] = '-'
	hex.Encode(b[9:13], v[4:6])
	b[13] = '-'
	hex.Encode(b[14:18], v[6:8])
	b[18] = '-'
	hex.Encode(b[19:23], v[8:10])
	b[23] = '-'
	hex.Encode(b[24:36], v[10:16])
	return string(b[:]), nil
}
func invalid(message string) error { return fmt.Errorf("%w: %s", ErrInvalid, message) }
