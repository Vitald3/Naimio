package payments

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrAttemptConflict = errors.New("payment attempt idempotency conflict")

type AttemptRepository interface {
	Create(context.Context, Attempt) (Attempt, bool, error)
	Get(context.Context, string) (Attempt, error)
	Update(context.Context, Attempt) error
}

type Service struct {
	Repository AttemptRepository
	Now        func() time.Time
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s Service) Create(ctx context.Context, in Attempt) (Attempt, error) {
	if err := in.Normalize(); err != nil {
		return Attempt{}, err
	}
	if in.Status == "" {
		in.Status = StatusCreated
	}
	if in.ReconciliationState == "" {
		in.ReconciliationState = ReconciliationNotRequired
	}
	in.CreatedAt = s.now()
	in.UpdatedAt = in.CreatedAt
	v, created, err := s.Repository.Create(ctx, in)
	if err != nil {
		return Attempt{}, err
	}
	if !created && (v.Provider != in.Provider || v.AmountKopecks != in.AmountKopecks || v.Currency != in.Currency || v.IdempotencyKey != in.IdempotencyKey || v.InternalReferenceID != in.InternalReferenceID || v.OperationType != in.OperationType) {
		return Attempt{}, ErrAttemptConflict
	}
	return v, nil
}
func (s Service) MarkUnknown(ctx context.Context, id string) (Attempt, error) {
	v, err := s.Repository.Get(ctx, id)
	if err != nil {
		return Attempt{}, err
	}
	if err = v.Transition(StatusUnknownReconciliation, s.now()); err != nil {
		return Attempt{}, err
	}
	return v, s.Repository.Update(ctx, v)
}

type Store struct {
	mu     sync.Mutex
	values map[string]Attempt
	keys   map[string]string
}

func (s *Store) Create(_ context.Context, in Attempt) (Attempt, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values == nil {
		s.values = map[string]Attempt{}
		s.keys = map[string]string{}
	}
	key := string(in.Domain) + ":" + in.InternalReferenceID + ":" + string(in.OperationType) + ":" + in.IdempotencyKey
	if id, ok := s.keys[key]; ok {
		return s.values[id], false, nil
	}
	if in.ID == "" {
		in.ID = "attempt-" + in.IdempotencyKey
	}
	s.values[in.ID] = in
	s.keys[key] = in.ID
	return in, true, nil
}
func (s *Store) Get(_ context.Context, id string) (Attempt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.values[id]
	if !ok {
		return Attempt{}, ErrUnknownProvider
	}
	return v, nil
}
func (s *Store) Update(_ context.Context, in Attempt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.values[in.ID]; !ok {
		return ErrUnknownProvider
	}
	s.values[in.ID] = in
	return nil
}
