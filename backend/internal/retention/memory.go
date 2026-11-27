package retention

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Memory is an in-process Store. File persists the same state as one JSON
// file, for a single local process.
type Memory struct {
	mu         sync.Mutex
	policies   map[string]Policy
	tombstones []Tombstone
	path       string // "" for Memory
}

// NewMemory returns an empty in-process store.
func NewMemory() *Memory { return &Memory{policies: map[string]Policy{}} }

type fileState struct {
	Policies   []Policy    `json:"policies"`
	Tombstones []Tombstone `json:"tombstones"`
}

// NewFile opens (or starts) dir/retention.json.
func NewFile(dir string) (*Memory, error) {
	m := NewMemory()
	m.path = filepath.Join(dir, "retention.json")
	b, err := os.ReadFile(m.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return m, nil
	case err != nil:
		return nil, err
	}
	var st fileState
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	for _, p := range st.Policies {
		m.policies[p.Workspace] = p
	}
	m.tombstones = st.Tombstones
	return m, nil
}

func (m *Memory) save() error {
	if m.path == "" {
		return nil
	}
	st := fileState{Tombstones: m.tombstones}
	for _, p := range m.policies {
		st.Policies = append(st.Policies, p)
	}
	sort.Slice(st.Policies, func(i, j int) bool { return st.Policies[i].Workspace < st.Policies[j].Workspace })
	b, err := json.MarshalIndent(st, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.path)
}

// Policy implements Store.
func (m *Memory) Policy(_ context.Context, ws string) (Policy, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.policies[ws]
	return p, ok, nil
}

// SetPolicy implements Store.
func (m *Memory) SetPolicy(_ context.Context, p Policy) error {
	if err := p.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.policies[p.Workspace] = p
	return m.save()
}

// Policies implements Store.
func (m *Memory) Policies(context.Context) ([]Policy, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Policy, 0, len(m.policies))
	for _, p := range m.policies {
		out = append(out, p)
	}
	return out, nil
}

// AddTombstone implements Store.
func (m *Memory) AddTombstone(_ context.Context, t Tombstone) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tombstones = append(m.tombstones, t)
	return m.save()
}

// Tombstones implements Store.
func (m *Memory) Tombstones(_ context.Context, ws string, limit int) ([]Tombstone, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Tombstone
	for i := len(m.tombstones) - 1; i >= 0; i-- {
		if t := m.tombstones[i]; t.Workspace == ws {
			out = append(out, t)
			if limit > 0 && len(out) == limit {
				break
			}
		}
	}
	return out, nil
}
