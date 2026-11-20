package audit

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Memory is an in-process Store. Data is lost on restart.
type Memory struct {
	mu sync.Mutex
	ts map[string]*memChain
}

type memChain struct {
	events []Event // ascending by Seq; the retained suffix
	head   Head
	cp     *Checkpoint
}

// NewMemory returns an empty store.
func NewMemory() *Memory { return &Memory{ts: map[string]*memChain{}} }

func (m *Memory) chain(tenant string) *memChain {
	c := m.ts[tenant]
	if c == nil {
		c = &memChain{}
		m.ts[tenant] = c
	}
	return c
}

// Append implements Store.
func (m *Memory) Append(_ context.Context, tenant string, seal func(Head) (Event, error)) (Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.chain(tenant)
	e, err := seal(c.head)
	if err != nil {
		return Event{}, err
	}
	if e.Tenant != tenant || e.Seq != c.head.Seq+1 {
		return Event{}, ErrInvalid
	}
	c.events = append(c.events, e)
	c.head = HeadOf(e)
	return e, nil
}

// Range implements Store.
func (m *Memory) Range(_ context.Context, tenant string, r Range) ([]Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.ts[tenant]
	if c == nil {
		return nil, nil
	}
	var out []Event
	if r.Desc {
		for i := len(c.events) - 1; i >= 0 && len(out) < r.Limit; i-- {
			if e := c.events[i]; r.Cursor == 0 || e.Seq < r.Cursor {
				out = append(out, e)
			}
		}
		return out, nil
	}
	for _, e := range c.events {
		if e.Seq > r.Cursor {
			out = append(out, e)
			if len(out) >= r.Limit {
				break
			}
		}
	}
	return out, nil
}

// State implements Store.
func (m *Memory) State(_ context.Context, tenant string) (Head, *Checkpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.ts[tenant]
	if c == nil {
		return Head{}, nil, nil
	}
	return c.head, copyCP(c.cp), nil
}

func copyCP(cp *Checkpoint) *Checkpoint {
	if cp == nil {
		return nil
	}
	v := *cp
	return &v
}

// Purge implements Store.
func (m *Memory) Purge(_ context.Context, tenant string, cutoff, now time.Time, limit int) (PurgeResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.ts[tenant]
	if c == nil {
		return PurgeResult{}, nil
	}
	if limit <= 0 {
		limit = DefaultPurgeChunk
	}
	n := 0
	for n < len(c.events) && n < limit && c.events[n].Time.Before(cutoff) {
		n++
	}
	if n > 0 {
		last := c.events[n-1]
		c.cp = &Checkpoint{ThroughSeq: last.Seq, ThroughHash: last.Hash, ThroughTime: last.Time, PurgedAt: now.UTC()}
		c.events = append([]Event(nil), c.events[n:]...)
	}
	return PurgeResult{Deleted: n, Checkpoint: copyCP(c.cp)}, nil
}

// Tenants implements Store.
func (m *Memory) Tenants(_ context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.ts))
	for t := range m.ts {
		out = append(out, t)
	}
	sort.Strings(out)
	return out, nil
}

var _ Store = (*Memory)(nil)
