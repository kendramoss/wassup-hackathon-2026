// Package store persists cases and their results. Cases is the seam: an
// in-memory implementation now, Firestore once the team's GCP project is
// available (same interface, no handler changes).
package store

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/clarify"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
)

// ErrNotFound means no case has that ID.
var ErrNotFound = errors.New("store: case not found")

// Case is a stored case with everything computed for it.
type Case struct {
	ID            string               `json:"id"`
	Date          string               `json:"date"`
	Facts         string               `json:"facts"`
	Family        string               `json:"family,omitempty"`
	Product       *domain.Product      `json:"product,omitempty"`
	Exclude       []string             `json:"exclude_rulings,omitempty"`
	CurrentOrigin *planner.KnownOrigin `json:"current_origin,omitempty"`
	Prediction    domain.Prediction    `json:"prediction"`
	Triage        domain.Triage        `json:"triage"`
	Router        *router.Label        `json:"router,omitempty"`
	Plan          *planner.Result      `json:"plan,omitempty"`
	Clarify       *clarify.Result      `json:"clarify,omitempty"`
	// Answers are the requester's answers to clarify questions, in order.
	Answers []Answer `json:"answers,omitempty"`
	// OriginalFacts is the facts before answers were appended.
	OriginalFacts string    `json:"original_facts,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// Answer is one requester answer to a clarify question.
type Answer struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// Cases stores cases.
type Cases interface {
	Put(ctx context.Context, c Case) error
	Get(ctx context.Context, id string) (Case, error)
}

// Memory is an in-process Cases store. Data is lost on restart and not shared
// between Cloud Run instances; Firestore replaces it in deployment.
type Memory struct {
	mu    sync.RWMutex
	cases map[string]Case
}

// NewMemory returns an empty store.
func NewMemory() *Memory { return &Memory{cases: map[string]Case{}} }

// Put implements Cases.
func (m *Memory) Put(_ context.Context, c Case) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cases[c.ID] = c
	return nil
}

// Get implements Cases.
func (m *Memory) Get(_ context.Context, id string) (Case, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.cases[id]
	if !ok {
		return Case{}, ErrNotFound
	}
	return c, nil
}

var _ Cases = (*Memory)(nil)
