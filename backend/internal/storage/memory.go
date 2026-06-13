// Package storage provides an in-memory, thread-safe store for Subscription
// records. We intentionally keep this simple (no database) per assignment
// scope; the interface is small enough that a file-backed or DB-backed
// implementation could be swapped in later.
package storage

import (
	"errors"
	"sync"

	"backend/internal/model"
)

// ErrNotFound is returned when a record is missing.
var ErrNotFound = errors.New("subscription not found")

// MemoryStorage indexes Subscriptions by activation code (primary key for the
// frontend flow) and also by activation token for status lookups by partner
// token.
type MemoryStorage struct {
	mu          sync.RWMutex
	byCode      map[string]*model.Subscription
	codeByToken map[string]string
}

// NewMemoryStorage builds an empty store.
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		byCode:      make(map[string]*model.Subscription),
		codeByToken: make(map[string]string),
	}
}

// Save inserts or updates a Subscription, keyed by its ActivationCode.
// ActivationCode must be non-empty.
func (s *MemoryStorage) Save(sub *model.Subscription) error {
	if sub == nil {
		return errors.New("nil subscription")
	}
	if sub.ActivationCode == "" {
		return errors.New("activation code required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *sub
	s.byCode[sub.ActivationCode] = &cp
	if sub.ActivationToken != "" {
		s.codeByToken[sub.ActivationToken] = sub.ActivationCode
	}
	return nil
}

// GetByCode returns a copy of the subscription identified by activation code.
func (s *MemoryStorage) GetByCode(code string) (*model.Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sub, ok := s.byCode[code]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *sub
	return &cp, nil
}

// GetByToken returns a copy of the subscription identified by activation
// token (provider-issued).
func (s *MemoryStorage) GetByToken(token string) (*model.Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	code, ok := s.codeByToken[token]
	if !ok {
		return nil, ErrNotFound
	}
	sub, ok := s.byCode[code]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *sub
	return &cp, nil
}

// List returns a snapshot of all stored subscriptions. Mainly for debugging.
func (s *MemoryStorage) List() []model.Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Subscription, 0, len(s.byCode))
	for _, sub := range s.byCode {
		out = append(out, *sub)
	}
	return out
}
