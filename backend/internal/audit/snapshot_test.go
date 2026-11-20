package audit

import (
	"context"
	"errors"
	"testing"
	"time"
)

// hookStore runs hooks around State so a test can make a write or a purge land
// exactly between the reads a verification makes.
type hookStore struct {
	Store
	afterState func(n int) // called after the nth State call returns
	states     int
}

func (h *hookStore) State(ctx context.Context, tenant string) (Head, *Checkpoint, error) {
	hd, cp, err := h.Store.State(ctx, tenant)
	h.states++
	if h.afterState != nil {
		h.afterState(h.states)
	}
	return hd, cp, err
}

func TestVerifyIgnoresAppendsAfterTheSnapshot(t *testing.T) {
	eachStore(t, func(t *testing.T, _ storeKit, st Store, _ tamperer) {
		hs := &hookStore{Store: st}
		c := &clock{t: t0}
		s := newTestService(t, hs, c.now)
		for i := 0; i < 4; i++ {
			c.advance(time.Second)
			mustRecord(t, s, userDraft(tenA, CaseUpdate))
		}
		hs.states = 0
		hs.afterState = func(n int) {
			if n == 1 { // right after the head is read, before the events
				mustRecord(t, s, userDraft(tenA, CaseUpdate))
			}
		}
		r, err := s.Verify(context.Background(), tenA, "")
		if err != nil || !r.OK || !r.Complete || r.VerifiedThroughSeq != 4 || r.Checked != 4 {
			t.Fatalf("honest chain with a concurrent append: %+v %v (was a false head_mismatch)", r, err)
		}
		hs.afterState = nil
		if r, _ := s.Verify(context.Background(), tenA, ""); !r.OK || r.VerifiedThroughSeq != 5 {
			t.Errorf("next verification = %+v", r)
		}
	})
}

func TestPagedVerifyIsFrozenAndSelfWritesDoNotStarveIt(t *testing.T) {
	old := verifyPage
	verifyPage = 3
	defer func() { verifyPage = old }()
	eachStore(t, func(t *testing.T, _ storeKit, st Store, _ tamperer) {
		s, c := seedChain(t, st, 9)
		cur, pages := "", 0
		for {
			c.advance(time.Second)
			r, err := s.Verify(context.Background(), tenA, cur)
			if err != nil || !r.OK || r.VerifiedThroughSeq != 9 {
				t.Fatalf("page %d: %+v %v", pages, r, err)
			}
			pages++
			// The caller (and everyone else) keeps writing between pages,
			// including the audit.verify event itself.
			mustRecord(t, s, userDraft(tenA, CaseUpdate))
			if r.Complete {
				break
			}
			cur = r.NextCursor
			if pages > 6 {
				t.Fatal("writes between pages keep a verification from finishing")
			}
		}
		if pages != 3 { // seqs 1-3, 4-6, 7-9: the run ends at the snapshot
			t.Errorf("pages = %d", pages)
		}
	})
}

func TestLegitimatePurgeBetweenPagesIsARestartNotATamperFinding(t *testing.T) {
	old := verifyPage
	verifyPage = 3
	defer func() { verifyPage = old }()
	eachStore(t, func(t *testing.T, _ storeKit, st Store, _ tamperer) {
		c := &clock{t: t0}
		s := NewService(st, pseud(t), Options{Now: c.now, Retention: time.Hour})
		for i := 0; i < 9; i++ {
			mustRecord(t, s, userDraft(tenA, CaseUpdate))
			c.advance(time.Minute)
		}
		ctx := context.Background()
		r1, err := s.Verify(ctx, tenA, "")
		if err != nil || r1.Complete || !r1.OK {
			t.Fatalf("page1 = %+v %v", r1, err)
		}
		c.advance(3 * time.Hour) // everything is past retention
		if n, err := s.PurgeAll(ctx); err != nil || n != 9 {
			t.Fatalf("purge = %d %v", n, err)
		}
		if _, err := s.Verify(ctx, tenA, r1.NextCursor); !errors.Is(err, ErrVerifyRestart) {
			t.Fatalf("page2 after a purge = %v, want ErrVerifyRestart (not a seq_gap finding)", err)
		}
		// Restarting works and is clean.
		if r, err := s.Verify(ctx, tenA, ""); err != nil || !r.OK || !r.Complete {
			t.Errorf("restart = %+v %v", r, err)
		}
	})
}

func TestPurgeInsideAFirstCallRetriesInsteadOfReportingAGap(t *testing.T) {
	eachStore(t, func(t *testing.T, _ storeKit, st Store, _ tamperer) {
		hs := &hookStore{Store: st}
		c := &clock{t: t0}
		s := NewService(hs, pseud(t), Options{Now: c.now, Retention: time.Hour})
		for i := 0; i < 6; i++ {
			mustRecord(t, s, userDraft(tenA, CaseUpdate))
			c.advance(time.Minute)
		}
		c.advance(3 * time.Hour)
		hs.states = 0
		hs.afterState = func(n int) {
			if n == 1 {
				if _, err := st.Purge(context.Background(), tenA, c.now().Add(-time.Hour), c.now(), 3); err != nil {
					t.Error(err)
				}
			}
		}
		r, err := s.Verify(context.Background(), tenA, "")
		if err != nil || !r.OK {
			t.Fatalf("purge during the first call: %+v %v", r, err)
		}
	})
}

func TestTamperIsStillFoundUnderTheSnapshotRules(t *testing.T) {
	old := verifyPage
	verifyPage = 3
	defer func() { verifyPage = old }()
	for name, edit := range map[string]func([]Event) []Event{
		"deleted prefix, checkpoint untouched": func(es []Event) []Event { return es[3:] },
		"truncated tail":                       func(es []Event) []Event { return es[:len(es)-2] },
		"deleted middle":                       func(es []Event) []Event { return append(es[:4:4], es[5:]...) },
	} {
		t.Run(name, func(t *testing.T) {
			eachStore(t, func(t *testing.T, _ storeKit, st Store, tm tamperer) {
				s, _ := seedChain(t, st, 9)
				r1, _ := s.Verify(context.Background(), tenA, "")
				tm.edit(t, tenA, edit) // between pages
				found := !r1.OK
				cur := r1.NextCursor
				for i := 0; i < 8 && cur != ""; i++ {
					r, err := s.Verify(context.Background(), tenA, cur)
					if errors.Is(err, ErrVerifyRestart) {
						t.Fatal("tamper was reported as a purge restart")
					}
					found = found || !r.OK
					cur = r.NextCursor
				}
				// The deleted prefix was already verified by page 1, so only a fresh
				// verification can see it; the other edits sit in unread range.
				if !found && name != "deleted prefix, checkpoint untouched" {
					t.Error("tamper between pages was not found")
				}
				if r := verifyAll(t, s, tenA); r.OK {
					t.Error("a fresh verification passed over tampered data")
				}
			})
		})
	}
}

// verifyAll follows the cursors to the final page.
func verifyAll(t *testing.T, s *Service, tenant string) VerifyResult {
	t.Helper()
	cur, ok := "", true
	for i := 0; i < 20; i++ {
		r, err := s.Verify(context.Background(), tenant, cur)
		if err != nil {
			t.Fatal(err)
		}
		ok = ok && r.OK
		if r.Complete {
			r.OK = ok
			return r
		}
		cur = r.NextCursor
	}
	t.Fatal("verification does not finish")
	return VerifyResult{}
}

func TestUnresolvedIsAsOfTheSnapshotAndTheAgeCutoffIsFrozen(t *testing.T) {
	old := verifyPage
	verifyPage = 2
	defer func() { verifyPage = old }()
	eachStore(t, func(t *testing.T, _ storeKit, st Store, _ tamperer) {
		c := &clock{t: t0}
		s := newTestService(t, st, c.now)
		d := userDraft(tenA, CaseDelete)
		d.Phase = PhaseIntent
		old := mustRecord(t, s, d)
		c.advance(4 * time.Minute)
		young := mustRecord(t, s, d) // 4 min old at the first call: not yet counted
		for i := 0; i < 4; i++ {
			mustRecord(t, s, userDraft(tenA, CaseUpdate))
		}
		ctx := context.Background()
		c.advance(2 * time.Minute) // old is 6 min, young 2 min
		r, err := s.Verify(ctx, tenA, "")
		if err != nil || r.Complete {
			t.Fatalf("first page = %+v %v", r, err)
		}
		// Time passes and a completion for `old` is appended, between pages.
		c.advance(30 * time.Minute)
		if _, err := s.Finish(ctx, old, Allowed, ReasonNone, ""); err != nil {
			t.Fatal(err)
		}
		var final VerifyResult
		for cur := r.NextCursor; ; {
			r, err = s.Verify(ctx, tenA, cur)
			if err != nil || !r.OK {
				t.Fatalf("page = %+v %v", r, err)
			}
			if r.Complete {
				final = r
				break
			}
			cur = r.NextCursor
		}
		u := final.UnresolvedIntents
		// As of the snapshot: `old` had no completion in the verified range, and
		// `young` was inside the five-minute window at the frozen cutoff even
		// though 30 minutes have passed since.
		if u == nil || u.Count != 1 || u.Seqs[0] != old.Seq || final.VerifiedThroughSeq != 6 {
			t.Errorf("unresolved as of the snapshot = %+v through %d, want only seq %d through 6 (young %d is inside the frozen window)", u, final.VerifiedThroughSeq, old.Seq, young.Seq)
		}
		// A new verification sees the completion.
		if r2 := verifyAll(t, s, tenA); r2.UnresolvedIntents == nil || r2.UnresolvedIntents.Count != 1 || r2.UnresolvedIntents.Seqs[0] != young.Seq {
			t.Errorf("fresh verification = %+v", r2.UnresolvedIntents)
		}
	})
}

// rollbackStore reports a head that has gone backwards after the snapshot.
type rollbackStore struct {
	Store
	n int
}

func (r *rollbackStore) State(ctx context.Context, tenant string) (Head, *Checkpoint, error) {
	h, cp, err := r.Store.State(ctx, tenant)
	r.n++
	if r.n > 1 && h.Seq > 0 {
		h.Seq--
	}
	return h, cp, err
}

func TestLiveHeadGoingBackwardsAfterTheSnapshotIsStillFound(t *testing.T) {
	rs := &rollbackStore{Store: NewMemory()}
	s, _ := seedChain(t, rs, 5)
	rs.n = 0
	r, err := s.Verify(context.Background(), tenA, "")
	if err != nil || r.OK || !kinds(r)[FindingHead] {
		t.Errorf("a head rolled back after the snapshot passed: %+v %v", r, err)
	}
}
