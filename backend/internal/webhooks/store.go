package webhooks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

// Store keeps endpoints and deliveries. Implementations: Memory, a file in
// CASES_DIR (local runs) and Firestore (store.Firestore.Webhooks).
type Store interface {
	// PutEndpoint creates or replaces an endpoint.
	PutEndpoint(ctx context.Context, e Endpoint) error
	// GetEndpoint returns the endpoint; ErrNotFound.
	GetEndpoint(ctx context.Context, id string) (Endpoint, error)
	// ListEndpoints returns a workspace's endpoints, oldest first.
	ListEndpoints(ctx context.Context, workspace string) ([]Endpoint, error)
	// DeleteEndpoint removes the endpoint and its deliveries.
	DeleteEndpoint(ctx context.Context, id string) error
	// CreateDelivery stores a new delivery and drops the endpoint's oldest
	// finished ones beyond MaxDeliveriesKept.
	CreateDelivery(ctx context.Context, d Delivery) error
	// UpdateDelivery reads the delivery, runs fn and stores the result
	// atomically; an error from fn stores nothing and is returned.
	UpdateDelivery(ctx context.Context, id string, fn func(*Delivery) error) (Delivery, error)
	// ListDeliveries returns an endpoint's deliveries, newest first.
	ListDeliveries(ctx context.Context, endpointID string, limit int) ([]Delivery, error)
	// ListPending returns the deliveries still to be sent, in any order.
	ListPending(ctx context.Context) ([]Delivery, error)
}

// Memory is an in-process Store; with a path it also keeps its state in one
// JSON file, written atomically after each change.
type Memory struct {
	mu   sync.Mutex
	path string
	st   state
}

type state struct {
	Endpoints  map[string]Endpoint `json:"endpoints"`
	Deliveries map[string]Delivery `json:"deliveries"`
}

// NewMemory returns an empty in-process store.
func NewMemory() *Memory {
	return &Memory{st: state{Endpoints: map[string]Endpoint{}, Deliveries: map[string]Delivery{}}}
}

// NewFile returns a store kept in dir/webhooks.json, loading what is there.
func NewFile(dir string) (*Memory, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("webhooks: %w", err)
	}
	m := NewMemory()
	m.path = filepath.Join(dir, "webhooks.json")
	b, err := os.ReadFile(m.path)
	if errors.Is(err, fs.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, fmt.Errorf("webhooks: read store: %w", err)
	}
	if err := json.Unmarshal(b, &m.st); err != nil {
		return nil, fmt.Errorf("webhooks: decode store: %w", err)
	}
	if m.st.Endpoints == nil {
		m.st.Endpoints = map[string]Endpoint{}
	}
	if m.st.Deliveries == nil {
		m.st.Deliveries = map[string]Delivery{}
	}
	return m, nil
}

// persist writes the state when the store has a file. mu is held.
func (m *Memory) persist() error {
	if m.path == "" {
		return nil
	}
	b, err := json.Marshal(m.st)
	if err != nil {
		return fmt.Errorf("webhooks: encode store: %w", err)
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("webhooks: write store: %w", err)
	}
	return os.Rename(tmp, m.path)
}

func cloneEndpoint(e Endpoint) Endpoint {
	e.Events = slices.Clone(e.Events)
	return e
}

func cloneDelivery(d Delivery) Delivery {
	d.Attempts = slices.Clone(d.Attempts)
	return d
}

// PutEndpoint implements Store.
func (m *Memory) PutEndpoint(_ context.Context, e Endpoint) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.st.Endpoints[e.ID] = cloneEndpoint(e)
	return m.persist()
}

// GetEndpoint implements Store.
func (m *Memory) GetEndpoint(_ context.Context, id string) (Endpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.st.Endpoints[id]
	if !ok {
		return Endpoint{}, ErrNotFound
	}
	return cloneEndpoint(e), nil
}

// ListEndpoints implements Store.
func (m *Memory) ListEndpoints(_ context.Context, workspace string) ([]Endpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Endpoint
	for _, e := range m.st.Endpoints {
		if e.Workspace == workspace {
			out = append(out, cloneEndpoint(e))
		}
	}
	slices.SortFunc(out, func(a, b Endpoint) int {
		if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
			return c
		}
		return cmpString(a.ID, b.ID)
	})
	return out, nil
}

func cmpString(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// DeleteEndpoint implements Store.
func (m *Memory) DeleteEndpoint(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.st.Endpoints, id)
	for did, d := range m.st.Deliveries {
		if d.EndpointID == id {
			delete(m.st.Deliveries, did)
		}
	}
	return m.persist()
}

// CreateDelivery implements Store.
func (m *Memory) CreateDelivery(_ context.Context, d Delivery) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.st.Deliveries[d.ID] = cloneDelivery(d)
	var mine []Delivery
	for _, x := range m.st.Deliveries {
		if x.EndpointID == d.EndpointID && x.Status != StatusPending {
			mine = append(mine, x)
		}
	}
	if len(mine) > MaxDeliveriesKept {
		slices.SortFunc(mine, func(a, b Delivery) int { return a.CreatedAt.Compare(b.CreatedAt) })
		for _, x := range mine[:len(mine)-MaxDeliveriesKept] {
			delete(m.st.Deliveries, x.ID)
		}
	}
	return m.persist()
}

// UpdateDelivery implements Store.
func (m *Memory) UpdateDelivery(_ context.Context, id string, fn func(*Delivery) error) (Delivery, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.st.Deliveries[id]
	if !ok {
		return Delivery{}, ErrNotFound
	}
	d = cloneDelivery(d)
	if err := fn(&d); err != nil {
		return Delivery{}, err
	}
	m.st.Deliveries[id] = cloneDelivery(d)
	if err := m.persist(); err != nil {
		return Delivery{}, err
	}
	return d, nil
}

// ListDeliveries implements Store.
func (m *Memory) ListDeliveries(_ context.Context, endpointID string, limit int) ([]Delivery, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Delivery
	for _, d := range m.st.Deliveries {
		if d.EndpointID == endpointID {
			out = append(out, cloneDelivery(d))
		}
	}
	slices.SortFunc(out, func(a, b Delivery) int {
		if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
			return c
		}
		return cmpString(b.ID, a.ID)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ListPending implements Store.
func (m *Memory) ListPending(_ context.Context) ([]Delivery, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Delivery
	for _, d := range m.st.Deliveries {
		if d.Status == StatusPending {
			out = append(out, cloneDelivery(d))
		}
	}
	return out, nil
}

var _ Store = (*Memory)(nil)
