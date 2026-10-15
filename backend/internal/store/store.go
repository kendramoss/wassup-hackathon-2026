// Package store persists cases and their results. Cases is the seam: an
// in-memory implementation now, Firestore once the team's GCP project is
// available (same interface, no handler changes).
package store

import (
	"context"
	"errors"
	"sort"
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
	UpdatedAt     time.Time `json:"updated_at"`
	// ExportedAt is set when a ruling-request draft was produced.
	ExportedAt *time.Time `json:"exported_at,omitempty"`
	// Scenarios are alternative production setups predicted side by side.
	Scenarios []Scenario `json:"scenarios,omitempty"`
	// Watch is the latest re-check of the prediction against newer rulings.
	Watch *Watch `json:"watch,omitempty"`
	// Economics are the latest user economics (from a plan search or a
	// scenarios call), reused to price scenarios.
	Economics *planner.Economics `json:"economics,omitempty"`
	// Targets is the latest comparison of target countries.
	Targets *TargetComparison `json:"targets,omitempty"`
	// Chat is the conversation with the case agent, oldest first.
	Chat []ChatTurn `json:"chat,omitempty"`
	// History is the case's audit trail, oldest first (capped at MaxHistory).
	History []Event `json:"history,omitempty"`
}

// MaxHistory caps a case's audit trail; the oldest events are dropped.
const MaxHistory = 200

// Event is one entry in a case's audit trail: what happened, its outcome in
// one line, and the model and prompt versions behind it.
type Event struct {
	At       time.Time `json:"at"`
	Kind     string    `json:"kind"` // predicted | answered | plan_searched | scenarios | targets_compared | watched | asked | exported
	Summary  string    `json:"summary"`
	Provider string    `json:"llm_provider,omitempty"`
	Model    string    `json:"model_id,omitempty"`
	Prompt   string    `json:"prompt_version,omitempty"`
}

// Record appends an event to the audit trail.
func (c *Case) Record(e Event) {
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	c.History = append(c.History, e)
	if len(c.History) > MaxHistory {
		c.History = c.History[len(c.History)-MaxHistory:]
	}
}

// ChatTurn is one question to the case agent and its answer.
type ChatTurn struct {
	Question string    `json:"question"`
	Answer   string    `json:"answer"`
	Cited    []string  `json:"cited_rulings"`
	At       time.Time `json:"at"`
}

// TargetComparison is a plan search per target country, ranked.
type TargetComparison struct {
	CheckedAt time.Time                 `json:"checked_at"`
	Ranked    []TargetSummary           `json:"ranked"`
	Results   map[string]planner.Result `json:"results"` // full plan search per target
}

// TargetSummary is one target country's outcome.
type TargetSummary struct {
	Rank          int          `json:"rank"`
	Target        string       `json:"target"`
	Status        string       `json:"status"` // plan | below_bar | no_plan | no_plant
	Best          *domain.Plan `json:"best"`   // best shown plan, or the closest below-bar plan
	ShownCount    int          `json:"shown_count"`
	MovesSearched int          `json:"moves_searched"`
}

// Scenario is one alternative production setup and its prediction.
type Scenario struct {
	Label      string            `json:"label"`
	Facts      string            `json:"facts"`
	Product    *domain.Product   `json:"product,omitempty"`
	Prediction domain.Prediction `json:"prediction"`
	Triage     domain.Triage     `json:"triage"`
	// Comparison is the yearly duty and cost against the current setup;
	// nil without economics.
	Comparison *planner.Comparison `json:"comparison,omitempty"`
}

// Watch statuses.
const (
	WatchUnchanged = "unchanged" // same origin with today's precedents
	WatchChanged   = "changed"   // a different origin with today's precedents
)

// Watch is a re-run of the engine with every precedent available today, so
// rulings CBP issued after the case date can count.
type Watch struct {
	CheckedAt  time.Time         `json:"checked_at"`
	Status     string            `json:"status"`
	Prediction domain.Prediction `json:"prediction"`
	// Newer are the precedents used that are dated after the case date.
	Newer []string `json:"newer_rulings"`
}

// Step is how far the case has progressed in the UI flow.
func (c Case) Step() string {
	switch {
	case c.ExportedAt != nil:
		return "export"
	case c.Plan != nil:
		return "plan"
	case c.Prediction.Origin != "":
		return "origin"
	}
	return "product"
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
	// List returns all cases, most recently updated first.
	List(ctx context.Context) ([]Case, error)
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

// List implements Cases.
func (m *Memory) List(_ context.Context) ([]Case, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Case, 0, len(m.cases))
	for _, c := range m.cases {
		out = append(out, c)
	}
	sortNewest(out)
	return out, nil
}

func sortNewest(cs []Case) {
	sort.Slice(cs, func(i, j int) bool {
		if !cs[i].UpdatedAt.Equal(cs[j].UpdatedAt) {
			return cs[i].UpdatedAt.After(cs[j].UpdatedAt)
		}
		return cs[i].ID > cs[j].ID
	})
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
