package audit

import (
	"context"
	"errors"
	"testing"
	"time"
)

// setHead rewrites the stored head behind the Store's back.
func setHead(t *testing.T, st Store, tenant string, h Head) {
	t.Helper()
	switch s := st.(type) {
	case *Memory:
		s.mu.Lock()
		s.ts[tenant].head = h
		s.mu.Unlock()
	case *File:
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := s.writeHead(tenant, h); err != nil {
			t.Fatal(err)
		}
		delete(s.heads, tenant)
	default:
		t.Fatalf("no head editor for %T", st)
	}
}

// forgedTail appends a validly sealed, correctly chained event past the head
// without moving the head.
func forgedTail(t *testing.T, tm tamperer, tenant string) {
	t.Helper()
	tm.edit(t, tenant, func(es []Event) []Event {
		last := es[len(es)-1]
		d := last
		d.Seq, d.Hash, d.PrevHash = 0, "", ""
		f, err := Seal(d, HeadOf(last), last.Time)
		if err != nil {
			t.Fatal(err)
		}
		return append(es, f)
	})
}

func withPage(n int, f func()) {
	old := verifyPage
	verifyPage = n
	defer func() { verifyPage = old }()
	f()
}

// The page sizes cover the default, one event per page, and a page that ends
// exactly on the frozen head (5 events), where the ordinary clip observes
// nothing beyond it.
var tailPages = []int{VerifyPage, 1, 2, 5, 6}

func TestStoredHeadRolledBackBelowRetainedEventsIsFound(t *testing.T) {
	for _, page := range tailPages {
		withPage(page, func() {
			eachStore(t, func(t *testing.T, _ storeKit, st Store, _ tamperer) {
				s, _ := seedChain(t, st, 5)
				evs, _ := st.Range(context.Background(), tenA, Range{Limit: 10})
				setHead(t, st, tenA, HeadOf(evs[2])) // head says 3; events 4 and 5 are still stored
				r := verifyAll(t, s, tenA)
				if r.OK || !kinds(r)[FindingHead] {
					t.Errorf("page %d: head rolled back to 3 over 5 events: ok=%v %v", page, r.OK, r.Findings)
				}
			})
		})
	}
}

func TestForgedChainedEventPastTheHeadIsFound(t *testing.T) {
	for _, page := range tailPages {
		withPage(page, func() {
			eachStore(t, func(t *testing.T, _ storeKit, st Store, tm tamperer) {
				s, _ := seedChain(t, st, 5)
				forgedTail(t, tm, tenA)
				r := verifyAll(t, s, tenA)
				if r.OK || !kinds(r)[FindingHead] {
					t.Errorf("page %d: an event at seq 6 with the head at 5: ok=%v %v", page, r.OK, r.Findings)
				}
			})
		})
	}
}

func TestEmptyHeadOverRetainedEventsIsFound(t *testing.T) {
	for _, page := range []int{VerifyPage, 1, 5} {
		withPage(page, func() {
			eachStore(t, func(t *testing.T, _ storeKit, st Store, _ tamperer) {
				s, _ := seedChain(t, st, 5)
				setHead(t, st, tenA, Head{})
				r := verifyAll(t, s, tenA)
				if r.OK || !kinds(r)[FindingHead] {
					t.Errorf("page %d: an empty head over 5 stored events: ok=%v %v", page, r.OK, r.Findings)
				}
			})
		})
	}
}

func TestForgedTailThatDoesNotChainIsFound(t *testing.T) {
	eachStore(t, func(t *testing.T, _ storeKit, st Store, tm tamperer) {
		s, _ := seedChain(t, st, 5)
		tm.edit(t, tenA, func(es []Event) []Event {
			f := es[len(es)-1]
			f.Seq, f.PrevHash = 6, Genesis
			f.Hash = f.ComputeHash()
			return append(es, f)
		})
		if r := verifyAll(t, s, tenA); r.OK || !kinds(r)[FindingPrev] {
			t.Errorf("a seq 6 event that does not chain to the head: %v", r.Findings)
		}
	})
}

func TestTrueConcurrentAppendsStayGreenAtEveryPageSize(t *testing.T) {
	for _, page := range tailPages {
		withPage(page, func() {
			eachStore(t, func(t *testing.T, _ storeKit, st Store, _ tamperer) {
				hs := &hookStore{Store: st}
				c := &clock{t: t0}
				s := newTestService(t, hs, c.now)
				for i := 0; i < 5; i++ {
					c.advance(time.Second)
					mustRecord(t, s, userDraft(tenA, CaseUpdate))
				}
				// An append lands after every State read, including the final
				// head read after the tail peek: the head and the event move together.
				hs.states = 0
				hs.afterState = func(int) { mustRecord(t, s, userDraft(tenA, CaseUpdate)) }
				cur := ""
				for i := 0; ; i++ {
					r, err := s.Verify(context.Background(), tenA, cur)
					if err != nil || !r.OK || r.VerifiedThroughSeq != 5 {
						t.Fatalf("page %d call %d: %+v %v", page, i, r, err)
					}
					if r.Complete {
						break
					}
					if cur = r.NextCursor; i > 30 {
						t.Fatal("concurrent appends starve the verification")
					}
				}
			})
		})
	}
}

func TestACursorWithoutAFrozenHeadIsRejectedNotJudged(t *testing.T) {
	st := NewMemory()
	s, _ := seedChain(t, st, 5)
	old := s.ps.EncodeCursor(tenA, Cursor{Kind: "verify", Seq: 2, Hash: Genesis}) // the shape written before snapshots existed
	if _, err := s.Verify(context.Background(), tenA, old); !errors.Is(err, ErrBadQuery) {
		t.Errorf("pre-snapshot cursor = %v, want ErrBadQuery (HTTP 400: start again)", err)
	}
}
