package matching

import (
	"context"
	"sort"
	"sync"
	"time"
)

type Store struct {
	mu         sync.Mutex
	Projects   map[string]Project
	Candidates map[string][]Candidate
	Runs       map[string]Run
	LatestRun  map[string]string
	Manual     map[string]map[string]bool
	Admins     map[string]bool
	Events     map[string]bool
	Now        func() time.Time
}

func (s *Store) OwnedProject(_ context.Context, actor, id string) (Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.Projects[id]
	if !ok || p.CustomerID != actor {
		return Project{}, ErrNotFound
	}
	return p, nil
}
func (s *Store) Retrieve(_ context.Context, p Project, c Constraints, limit int) ([]Candidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := []Candidate{}
	for _, v := range s.Candidates[p.ID] {
		if v.UserID == p.CustomerID {
			continue
		}
		if c.RequireImmediateAvailability && v.Availability != "AVAILABLE" {
			continue
		}
		if c.RequireCategoryMatch && v.PrimaryCategoryID != p.CategoryID {
			continue
		}
		if c.MaxMinimumOrderKopecks != nil && v.MinimumOrder != nil && *v.MinimumOrder > *c.MaxMinimumOrderKopecks {
			continue
		}
		items = append(items, v)
		if len(items) == limit {
			break
		}
	}
	return items, nil
}
func (s *Store) SaveRun(_ context.Context, actor string, p Project, _ Constraints, recommendations []Recommendation, aiUsed bool) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.CustomerID != actor {
		return Run{}, ErrNotFound
	}
	id, _ := uuidV7()
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	run := Run{ID: id, ProjectID: p.ID, AlgorithmVersion: AlgorithmVersion, AIUsed: aiUsed, CandidateCount: len(recommendations), CreatedAt: now, Recommendations: append([]Recommendation(nil), recommendations...)}
	if s.Runs == nil {
		s.Runs = map[string]Run{}
	}
	if s.LatestRun == nil {
		s.LatestRun = map[string]string{}
	}
	s.Runs[id] = run
	s.LatestRun[p.ID] = id
	return run, nil
}
func (s *Store) Run(_ context.Context, actor, projectID, runID string) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.Projects[projectID]
	run, found := s.Runs[runID]
	if !ok || p.CustomerID != actor || !found || run.ProjectID != projectID {
		return Run{}, ErrNotFound
	}
	return s.withManual(run), nil
}
func (s *Store) Latest(_ context.Context, actor, projectID string) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.Projects[projectID]
	id := s.LatestRun[projectID]
	run, found := s.Runs[id]
	if !ok || p.CustomerID != actor || !found {
		return Run{}, ErrNotFound
	}
	return s.withManual(run), nil
}
func (s *Store) ManualPut(_ context.Context, admin, projectID, freelancerID, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Admins[admin] {
		return ErrForbidden
	}
	if _, ok := s.Projects[projectID]; !ok {
		return ErrNotFound
	}
	eligible := false
	for _, candidate := range s.Candidates[projectID] {
		if candidate.UserID == freelancerID {
			eligible = true
		}
	}
	if !eligible {
		return ErrNotFound
	}
	if s.Manual == nil {
		s.Manual = map[string]map[string]bool{}
	}
	if s.Manual[projectID] == nil {
		s.Manual[projectID] = map[string]bool{}
	}
	s.Manual[projectID][freelancerID] = true
	_ = reason
	return nil
}
func (s *Store) ManualDelete(_ context.Context, admin, projectID, freelancerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Admins[admin] {
		return ErrForbidden
	}
	if !s.Manual[projectID][freelancerID] {
		return ErrNotFound
	}
	delete(s.Manual[projectID], freelancerID)
	return nil
}
func (s *Store) RecordEvent(_ context.Context, actor, projectID, runID, freelancerID, eventType, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.Projects[projectID]
	run, ok := s.Runs[runID]
	if p.CustomerID != actor || !ok || run.ProjectID != projectID {
		return ErrNotFound
	}
	candidate := false
	for _, item := range run.Recommendations {
		if item.FreelancerID == freelancerID {
			candidate = true
		}
	}
	if !candidate {
		return ErrNotFound
	}
	if s.Events == nil {
		s.Events = map[string]bool{}
	}
	s.Events[actor+":"+eventType+":"+key] = true
	return nil
}
func (s *Store) Metrics(_ context.Context, admin string) (Metrics, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Admins[admin] {
		return Metrics{}, ErrForbidden
	}
	m := Metrics{Runs: int64(len(s.Runs))}
	for _, run := range s.Runs {
		m.Candidates += int64(run.CandidateCount)
		if run.AIUsed {
			m.AIUsed++
		}
	}
	if m.Runs > 0 {
		m.AverageCandidates = float64(m.Candidates) / float64(m.Runs)
	}
	return m, nil
}
func (s *Store) withManual(run Run) Run {
	manual := s.Manual[run.ProjectID]
	for i := range run.Recommendations {
		if manual[run.Recommendations[i].FreelancerID] {
			run.Recommendations[i].Manual = true
			run.Recommendations[i].Reasons = prependManual(run.Recommendations[i].Reasons)
		}
	}
	sort.SliceStable(run.Recommendations, func(i, j int) bool { return run.Recommendations[i].Manual && !run.Recommendations[j].Manual })
	for i := range run.Recommendations {
		run.Recommendations[i].Rank = i + 1
	}
	return run
}
