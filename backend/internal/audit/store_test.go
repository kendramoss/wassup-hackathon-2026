package audit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

const (
	tenA = "ws:ws_0123456789ab"
	tenB = "ws:ws_ba9876543210"
)

// storeKit is a store plus the test-only hooks to tamper with what it holds.
type storeKit struct {
	name   string
	make   func(t *testing.T) (Store, tamperer)
	tailOK bool // detects a removed tail via the head (memory and file do)
}

// tamperer edits stored events behind the Store's back.
type tamperer interface {
	// edit replaces the retained events of tenant with f(events).
	edit(t *testing.T, tenant string, f func([]Event) []Event)
}

type memTamper struct{ m *Memory }

func (mt memTamper) edit(t *testing.T, tenant string, f func([]Event) []Event) {
	mt.m.mu.Lock()
	defer mt.m.mu.Unlock()
	c := mt.m.ts[tenant]
	c.events = f(append([]Event(nil), c.events...))
}

type fileTamper struct{ f *File }

func (ft fileTamper) edit(t *testing.T, tenant string, f func([]Event) []Event) {
	t.Helper()
	ft.f.mu.Lock()
	defer ft.f.mu.Unlock()
	evs, err := ft.f.readEvents(tenant)
	if err != nil {
		t.Fatal(err)
	}
	var out []byte
	for _, e := range f(evs) {
		line, _ := jsonMarshal(e)
		out = append(out, append(line, '\n')...)
	}
	if err := os.WriteFile(ft.f.base(tenant)+".jsonl", out, 0o644); err != nil {
		t.Fatal(err)
	}
}

func kits() []storeKit {
	return []storeKit{
		{name: "memory", tailOK: true, make: func(t *testing.T) (Store, tamperer) { m := NewMemory(); return m, memTamper{m} }},
		{name: "file", tailOK: true, make: func(t *testing.T) (Store, tamperer) {
			f, err := NewFile(filepath.Join(t.TempDir(), "audit"))
			if err != nil {
				t.Fatal(err)
			}
			return f, fileTamper{f}
		}},
	}
}

func eachStore(t *testing.T, run func(t *testing.T, k storeKit, st Store, tm tamperer)) {
	for _, k := range kits() {
		t.Run(k.name, func(t *testing.T) {
			st, tm := k.make(t)
			run(t, k, st, tm)
		})
	}
}

func TestStoreAppendAssignsConsecutiveSeqAndChains(t *testing.T) {
	eachStore(t, func(t *testing.T, _ storeKit, st Store, _ tamperer) {
		c := &clock{t: t0}
		s := newTestService(t, st, c.now)
		var prev Event
		for i := 1; i <= 5; i++ {
			c.advance(time.Second)
			e := mustRecord(t, s, userDraft(tenA, CaseUpdate))
			if e.Seq != uint64(i) || e.Validate() != nil {
				t.Fatalf("event %d = %+v", i, e)
			}
			if i > 1 && e.PrevHash != prev.Hash {
				t.Fatalf("event %d does not chain to %d", i, i-1)
			}
			prev = e
		}
		other := mustRecord(t, s, userDraft(tenB, CaseUpdate))
		if other.Seq != 1 || other.PrevHash != Genesis {
			t.Errorf("tenants share a chain: %+v", other)
		}
		h, cp, err := st.State(context.Background(), tenA)
		if err != nil || h.Seq != 5 || h.Hash != prev.Hash || cp != nil {
			t.Errorf("state = %+v %v %v", h, cp, err)
		}
		if h, _, _ := st.State(context.Background(), "ws:ws_ffffffffffff"); h.Seq != 0 {
			t.Errorf("unknown tenant has a head: %+v", h)
		}
	})
}

func TestStoreConcurrentAppendsAreAtomicPerTenant(t *testing.T) {
	eachStore(t, func(t *testing.T, _ storeKit, st Store, _ tamperer) {
		s := newTestService(t, st, time.Now)
		const workers, each = 8, 10
		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()
				for i := 0; i < each; i++ {
					ten := tenA
					if w%2 == 1 {
						ten = tenB
					}
					if _, err := s.Record(context.Background(), userDraft(ten, CaseUpdate)); err != nil {
						t.Error(err)
					}
				}
			}(w)
		}
		wg.Wait()
		for _, ten := range []string{tenA, tenB} {
			res, err := s.Verify(context.Background(), ten, "")
			want := workers / 2 * each
			if err != nil || !res.OK || res.Checked != want || res.LastSeq != uint64(want) {
				t.Errorf("%s after concurrent appends: %+v %v", ten, res, err)
			}
		}
	})
}

func TestStoreRangeOrderAndLimit(t *testing.T) {
	eachStore(t, func(t *testing.T, _ storeKit, st Store, _ tamperer) {
		c := &clock{t: t0}
		s := newTestService(t, st, c.now)
		for i := 0; i < 7; i++ {
			c.advance(time.Second)
			mustRecord(t, s, userDraft(tenA, CaseUpdate))
		}
		ctx := context.Background()
		asc, _ := st.Range(ctx, tenA, Range{Cursor: 2, Limit: 3})
		if len(asc) != 3 || asc[0].Seq != 3 || asc[2].Seq != 5 {
			t.Errorf("asc = %v", seqs(asc))
		}
		desc, _ := st.Range(ctx, tenA, Range{Desc: true, Limit: 3})
		if len(desc) != 3 || desc[0].Seq != 7 || desc[2].Seq != 5 {
			t.Errorf("desc from head = %v", seqs(desc))
		}
		desc, _ = st.Range(ctx, tenA, Range{Desc: true, Cursor: 5, Limit: 10})
		if len(desc) != 4 || desc[0].Seq != 4 || desc[3].Seq != 1 {
			t.Errorf("desc before 5 = %v", seqs(desc))
		}
		if none, _ := st.Range(ctx, "ws:ws_ffffffffffff", Range{Limit: 5}); len(none) != 0 {
			t.Error("events for an unknown tenant")
		}
	})
}

func seqs(es []Event) []uint64 {
	out := make([]uint64, len(es))
	for i, e := range es {
		out[i] = e.Seq
	}
	return out
}

func TestStoreTenants(t *testing.T) {
	eachStore(t, func(t *testing.T, _ storeKit, st Store, _ tamperer) {
		s := newTestService(t, st, time.Now)
		mustRecord(t, s, userDraft(tenB, CaseUpdate))
		mustRecord(t, s, userDraft(tenA, CaseUpdate))
		mustRecord(t, s, userDraft("platform", CaseUpdate))
		got, err := st.Tenants(context.Background())
		want := fmt.Sprint([]string{"platform", tenA, tenB})
		if err != nil || fmt.Sprint(got) != want {
			t.Errorf("tenants = %v %v, want %s", got, err, want)
		}
	})
}

func TestStoreRejectsAForeignOrSkippedEvent(t *testing.T) {
	eachStore(t, func(t *testing.T, _ storeKit, st Store, _ tamperer) {
		bad := func(h Head) (Event, error) {
			e, _ := Seal(userDraft(tenB, CaseUpdate).Event, h, t0) // wrong tenant for the chain it is appended to
			return e, nil
		}
		if _, err := st.Append(context.Background(), tenA, bad); err == nil {
			t.Error("an event of another tenant was appended")
		}
	})
}
