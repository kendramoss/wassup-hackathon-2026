package audit

import (
	"context"
	"testing"
	"time"
)

func seedChain(t *testing.T, st Store, n int) (*Service, *clock) {
	t.Helper()
	c := &clock{t: t0}
	s := newTestService(t, st, c.now)
	for i := 0; i < n; i++ {
		c.advance(time.Second)
		mustRecord(t, s, userDraft(tenA, CaseUpdate))
	}
	return s, c
}

func kinds(r VerifyResult) map[string]bool {
	m := map[string]bool{}
	for _, f := range r.Findings {
		m[f.Kind] = true
	}
	return m
}

func TestVerifyDetectsEveryKindOfTamper(t *testing.T) {
	tampers := map[string]struct {
		edit func([]Event) []Event
		want string
	}{
		"modified field": {func(es []Event) []Event { es[2].Outcome, es[2].Reason = Failed, ReasonInternal; return es }, FindingHash},
		"modified and rehashed": {func(es []Event) []Event {
			es[2].Object = "case:case_ba9876543210"
			es[2].Hash = es[2].ComputeHash()
			return es
		}, FindingPrev},
		"deleted from the middle": {func(es []Event) []Event { return append(es[:2:2], es[3:]...) }, FindingGap},
		"reordered":               {func(es []Event) []Event { es[1], es[2] = es[2], es[1]; return es }, FindingReorder},
		"inserted forgery": {func(es []Event) []Event {
			f := es[1]
			f.Seq = es[3].Seq
			f.Hash = f.ComputeHash()
			return append(es[:3:3], append([]Event{f}, es[3:]...)...)
		}, FindingReorder},
		"truncated tail":     {func(es []Event) []Event { return es[:len(es)-2] }, FindingHead},
		"foreign tenant row": {func(es []Event) []Event { es[1].Tenant = tenB; es[1].Hash = es[1].ComputeHash(); return es }, FindingTenant},
		"time went back": {func(es []Event) []Event {
			es[3].Time = es[3].Time.Add(-time.Hour)
			es[3].Hash = es[3].ComputeHash()
			es[4].PrevHash = es[3].Hash
			es[4].Hash = es[4].ComputeHash()
			return es
		}, FindingTime},
	}
	for name, tc := range tampers {
		t.Run(name, func(t *testing.T) {
			eachStore(t, func(t *testing.T, _ storeKit, st Store, tm tamperer) {
				s, _ := seedChain(t, st, 6)
				if r, err := s.Verify(context.Background(), tenA, ""); err != nil || !r.OK || !r.Complete || r.Checked != 6 {
					t.Fatalf("clean chain: %+v %v", r, err)
				}
				tm.edit(t, tenA, tc.edit)
				r, err := s.Verify(context.Background(), tenA, "")
				if err != nil {
					t.Fatal(err)
				}
				if r.OK || !kinds(r)[tc.want] {
					t.Errorf("ok=%v findings=%v, want %s", r.OK, r.Findings, tc.want)
				}
			})
		})
	}
}

func TestVerifyMissingOldestEventsWithoutCheckpointIsUnanchored(t *testing.T) {
	eachStore(t, func(t *testing.T, _ storeKit, st Store, tm tamperer) {
		s, _ := seedChain(t, st, 5)
		tm.edit(t, tenA, func(es []Event) []Event { return es[2:] })
		r, _ := s.Verify(context.Background(), tenA, "")
		if r.OK || !kinds(r)[FindingUnanchored] {
			t.Errorf("a deleted prefix with no checkpoint passed: %+v", r)
		}
	})
}

func TestVerifyPagesAndCursorContinues(t *testing.T) {
	old := verifyPage
	verifyPage = 4
	defer func() { verifyPage = old }()
	eachStore(t, func(t *testing.T, _ storeKit, st Store, _ tamperer) {
		s, _ := seedChain(t, st, 10)
		var checked int
		cur := ""
		for i := 0; ; i++ {
			r, err := s.Verify(context.Background(), tenA, cur)
			if err != nil || !r.OK {
				t.Fatalf("page %d: %+v %v", i, r, err)
			}
			checked += r.Checked
			if !r.Complete && r.UnresolvedIntents != nil {
				t.Error("unresolved intents reported before the final page")
			}
			if r.Complete {
				break
			}
			cur = r.NextCursor
			if i > 5 {
				t.Fatal("verification does not terminate")
			}
		}
		if checked != 10 {
			t.Errorf("checked %d events over the pages, want 10", checked)
		}
		r, _ := s.Verify(context.Background(), tenA, "")
		if _, err := s.Verify(context.Background(), tenB, r.NextCursor); err == nil {
			t.Error("a cursor from another tenant was accepted")
		}
	})
}

func TestVerifyTamperOnALaterPageIsStillFound(t *testing.T) {
	old := verifyPage
	verifyPage = 4
	defer func() { verifyPage = old }()
	eachStore(t, func(t *testing.T, _ storeKit, st Store, tm tamperer) {
		s, _ := seedChain(t, st, 10)
		tm.edit(t, tenA, func(es []Event) []Event { es[7].Actor = "a1_" + "0123456789abcdef0123456789abcdef"; return es })
		cur, found := "", false
		for i := 0; i < 5; i++ {
			r, _ := s.Verify(context.Background(), tenA, cur)
			found = found || kinds(r)[FindingHash]
			if r.Complete {
				break
			}
			cur = r.NextCursor
		}
		if !found {
			t.Error("tamper on a later page was not found")
		}
	})
}

func TestUnresolvedIntentsAreReportedOnlyWhenTrulyOpen(t *testing.T) {
	eachStore(t, func(t *testing.T, _ storeKit, st Store, _ tamperer) {
		c := &clock{t: t0}
		s := newTestService(t, st, c.now)
		ctx := context.Background()
		intent := func(a Action) Event {
			d := userDraft(tenA, a)
			d.Phase = PhaseIntent
			return mustRecord(t, s, d)
		}
		closed := intent(CaseDelete)
		if _, err := s.Finish(ctx, closed, Allowed, ReasonNone, ""); err != nil {
			t.Fatal(err)
		}
		open := intent(WorkspaceRoleChange) // its completion is "lost"
		stale := intent(CaseDelete)         // opened before the clock advanced
		c.advance(6 * time.Minute)
		young := intent(CaseDelete) // younger than five minutes: may still be running
		res, err := s.Verify(ctx, tenA, "")
		if err != nil || res.UnresolvedIntents == nil {
			t.Fatal(res, err)
		}
		got := res.UnresolvedIntents
		if got.Count != 2 || got.Seqs[0] != open.Seq || got.Seqs[1] != stale.Seq {
			t.Errorf("unresolved = %+v, want seqs %d and %d (not %d or %d)", got, open.Seq, stale.Seq, closed.Seq, young.Seq)
		}
		if !res.OK {
			t.Errorf("an open intent is not tamper: %+v", res.Findings)
		}
	})
}

func TestUnresolvedIntentsAcrossPagesReflectTheWholeRange(t *testing.T) {
	old := verifyPage
	verifyPage = 3
	defer func() { verifyPage = old }()
	eachStore(t, func(t *testing.T, _ storeKit, st Store, _ tamperer) {
		c := &clock{t: t0}
		s := newTestService(t, st, c.now)
		ctx := context.Background()
		intent := func() Event {
			d := userDraft(tenA, CaseDelete)
			d.Phase = PhaseIntent
			e := mustRecord(t, s, d)
			c.advance(time.Second)
			return e
		}
		filler := func(n int) {
			for i := 0; i < n; i++ {
				mustRecord(t, s, userDraft(tenA, CaseUpdate))
				c.advance(time.Second)
			}
		}
		done := intent() // seq 1: completed far later, on another page
		lost := intent() // seq 2: never completed
		filler(8)
		if _, err := s.Finish(ctx, done, Allowed, ReasonNone, ""); err != nil {
			t.Fatal(err)
		}
		filler(2)
		c.advance(time.Hour)

		cur, pages := "", 0
		var final VerifyResult
		for {
			r, err := s.Verify(ctx, tenA, cur)
			if err != nil || !r.OK {
				t.Fatalf("page %d: %+v %v", pages, r, err)
			}
			pages++
			if !r.Complete {
				if r.UnresolvedIntents != nil {
					t.Fatalf("page %d: unresolved_intents present before the final page", pages)
				}
				cur = r.NextCursor
				continue
			}
			final = r
			break
		}
		if pages < 4 {
			t.Fatalf("only %d pages, the scenario needs several", pages)
		}
		u := final.UnresolvedIntents
		if u == nil || u.Count != 1 || u.Seqs[0] != lost.Seq || !u.Complete {
			t.Errorf("unresolved over the whole range = %+v, want exactly seq %d (seq %d was completed on a later page)", u, lost.Seq, done.Seq)
		}
	})
}

func TestUnresolvedIsAnExplicitLowerBoundWhenStateCannotBeCarried(t *testing.T) {
	old := verifyPage
	verifyPage = 70
	defer func() { verifyPage = old }()
	st := NewMemory()
	c := &clock{t: t0}
	s := newTestService(t, st, c.now)
	ctx := context.Background()
	for i := 0; i < maxCarriedOpen+6; i++ { // more open intents than a cursor carries
		d := userDraft(tenA, CaseDelete)
		d.Phase = PhaseIntent
		mustRecord(t, s, d)
	}
	for i := 0; i < 70; i++ {
		mustRecord(t, s, userDraft(tenA, CaseUpdate))
	}
	c.advance(time.Hour)
	r1, _ := s.Verify(ctx, tenA, "")
	if r1.Complete || r1.NextCursor == "" {
		t.Fatalf("first page = %+v", r1)
	}
	r2, cur := r1, r1.NextCursor
	for i := 0; !r2.Complete; i++ {
		var err error
		if r2, err = s.Verify(ctx, tenA, cur); err != nil || i > 5 {
			t.Fatalf("verify = %+v %v", r2, err)
		}
		cur = r2.NextCursor
	}
	if r2.UnresolvedIntents == nil {
		t.Fatal("no unresolved report on the final page")
	}
	if r2.UnresolvedIntents.Complete {
		t.Error("a count that dropped state at a page boundary is presented as complete")
	}
	// A single-page verification of the same chain is exact.
	verifyPage = 5000
	r3, _ := s.Verify(ctx, tenA, "")
	if u := r3.UnresolvedIntents; u == nil || !u.Complete || u.Count != maxCarriedOpen+6 || len(u.Seqs) != maxUnresolved {
		t.Errorf("single page = %+v", u)
	}
}
