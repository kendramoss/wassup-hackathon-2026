package audit

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRecordStoresPseudonymsNeverIdentities(t *testing.T) {
	eachStore(t, func(t *testing.T, _ storeKit, st Store, _ tamperer) {
		s := newTestService(t, st, time.Now)
		d := userDraft(tenA, WorkspaceRoleChange)
		d.Detail, d.TargetID = "viewer", "bob@example.com"
		d.RequestID = "bob@example.com" // a client-chosen request id
		e := mustRecord(t, s, d)
		blob := fmtEvent(e)
		for _, secret := range []string{"alice", "bob", "example.com", "@"} {
			if strings.Contains(blob, secret) {
				t.Errorf("the stored event contains %q: %s", secret, blob)
			}
		}
		if !strings.HasPrefix(e.Actor, "a1_") || !strings.HasPrefix(e.Target, "a1_") || !strings.HasPrefix(e.RequestID, "h_") {
			t.Errorf("event = %+v", e)
		}
		// An admin or anonymous caller has no pseudonym, whatever id was passed.
		adm := Draft{Event: Event{Tenant: "platform", ActorKind: ActorAdmin, Action: APIKeyList, Phase: PhaseSingle, Outcome: Allowed}, ActorID: "secret-admin-token"}
		if e := mustRecord(t, s, adm); e.Actor != "" {
			t.Errorf("admin event has actor %q", e.Actor)
		}
		// A user event without an identity cannot be written.
		bad := userDraft(tenA, CaseUpdate)
		bad.ActorID = ""
		if _, err := s.Record(context.Background(), bad); err == nil {
			t.Error("an event for a user with no identity was written")
		}
	})
}

func fmtEvent(e Event) string {
	b, _ := jsonMarshal(e)
	return string(b)
}

func TestFinishClosesTheIntentAndRetriesOnce(t *testing.T) {
	st := &flaky{Store: NewMemory()}
	s := newTestService(t, st, time.Now)
	d := userDraft(tenA, APIKeyIssue)
	d.Phase, d.Object = PhaseIntent, "apikey:new"
	d.Tenant = tenA
	in := mustRecord(t, s, d)
	st.failNext = 1
	done, err := s.Finish(context.Background(), in, Allowed, ReasonNone, "apikey:0123456789abcdef")
	if err != nil || done.Ref != in.Seq || done.Phase != PhaseCompletion || done.Object != "apikey:0123456789abcdef" || done.Actor != in.Actor {
		t.Fatalf("finish = %+v %v", done, err)
	}
	if st.calls != 3 { // intent, failed completion, retried completion
		t.Errorf("append calls = %d, want 3 (one retry)", st.calls)
	}
	st.failNext = 5
	st.calls = 0
	if _, err := s.Finish(context.Background(), in, Failed, ReasonInternal, ""); err == nil || st.calls != 2 {
		t.Errorf("a persistent failure: err=%v calls=%d, want an error after exactly 2 tries", err, st.calls)
	}
}

type flaky struct {
	Store
	failNext, calls int
}

func (f *flaky) Append(ctx context.Context, tenant string, seal func(Head) (Event, error)) (Event, error) {
	f.calls++
	if f.failNext > 0 {
		f.failNext--
		return Event{}, errors.New("disk on fire")
	}
	return f.Store.Append(ctx, tenant, seal)
}

func TestListPagesFiltersAndOrder(t *testing.T) {
	eachStore(t, func(t *testing.T, _ storeKit, st Store, _ tamperer) {
		c := &clock{t: t0}
		s := newTestService(t, st, c.now)
		for i := 0; i < 12; i++ {
			c.advance(time.Minute)
			a := CaseUpdate
			if i%3 == 0 {
				a = CaseExport
			}
			d := userDraft(tenA, a)
			if i%4 == 0 {
				d.ActorID = "carol@example.com"
			}
			mustRecord(t, s, d)
		}
		ctx := context.Background()
		// Newest first, two pages, no overlap, no gap.
		p1, err := s.List(ctx, tenA, ListQuery{Limit: 5, Desc: true})
		if err != nil || len(p1.Events) != 5 || p1.Events[0].Seq != 12 || p1.NextCursor == "" {
			t.Fatalf("page1 = %v %q %v", seqs(p1.Events), p1.NextCursor, err)
		}
		p2, err := s.List(ctx, tenA, ListQuery{Limit: 5, Desc: true, Cursor: p1.NextCursor})
		if err != nil || len(p2.Events) != 5 || p2.Events[0].Seq != 7 {
			t.Fatalf("page2 = %v %v", seqs(p2.Events), err)
		}
		p3, _ := s.List(ctx, tenA, ListQuery{Limit: 5, Desc: true, Cursor: p2.NextCursor})
		if len(p3.Events) != 2 || p3.NextCursor != "" {
			t.Fatalf("last page = %v %q", seqs(p3.Events), p3.NextCursor)
		}
		asc, _ := s.List(ctx, tenA, ListQuery{Limit: 3})
		if asc.Events[0].Seq != 1 || asc.Events[2].Seq != 3 {
			t.Errorf("asc = %v", seqs(asc.Events))
		}
		// Filters: action, pseudonym, time window.
		ex, _ := s.List(ctx, tenA, ListQuery{Filter: Filter{Action: CaseExport}})
		if len(ex.Events) != 4 {
			t.Errorf("export filter = %v", seqs(ex.Events))
		}
		carol := s.Pseudonyms().Actor(ActorUser, "carol@example.com")
		by, _ := s.List(ctx, tenA, ListQuery{Filter: Filter{Actor: carol}})
		if len(by.Events) != 3 {
			t.Errorf("actor filter = %v", seqs(by.Events))
		}
		win, _ := s.List(ctx, tenA, ListQuery{Filter: Filter{From: t0.Add(3 * time.Minute), To: t0.Add(6 * time.Minute)}})
		if len(win.Events) != 3 || win.Events[0].Seq != 3 {
			t.Errorf("time window = %v", seqs(win.Events))
		}
		// A cursor cannot be replayed under other filters, order or tenant.
		if _, err := s.List(ctx, tenA, ListQuery{Limit: 5, Desc: true, Cursor: p1.NextCursor, Filter: Filter{Action: CaseExport}}); !errors.Is(err, ErrBadQuery) {
			t.Error("cursor reused under a different filter")
		}
		if _, err := s.List(ctx, tenA, ListQuery{Limit: 5, Cursor: p1.NextCursor}); !errors.Is(err, ErrBadQuery) {
			t.Error("a descending cursor was used ascending")
		}
		if _, err := s.List(ctx, tenB, ListQuery{Limit: 5, Desc: true, Cursor: p1.NextCursor}); !errors.Is(err, ErrBadQuery) {
			t.Error("cursor moved to another tenant")
		}
		for _, lim := range []int{-1, 201} {
			if _, err := s.List(ctx, tenA, ListQuery{Limit: lim}); !errors.Is(err, ErrBadQuery) {
				t.Errorf("limit %d accepted", lim)
			}
		}
		if _, err := s.List(ctx, "alice@example.com", ListQuery{}); !errors.Is(err, ErrBadQuery) {
			t.Error("a non-tenant string was accepted")
		}
	})
}

func TestListScanIsBoundedAndShortPagesCarryACursor(t *testing.T) {
	st := NewMemory()
	c := &clock{t: t0}
	s := NewService(st, pseud(t), Options{Now: c.now, ScanCap: 10})
	for i := 0; i < 35; i++ {
		c.advance(time.Second)
		a := CaseUpdate
		if i == 30 {
			a = CaseDelete
		}
		mustRecord(t, s, userDraft(tenA, a))
	}
	ctx := context.Background()
	q := ListQuery{Limit: 5, Filter: Filter{Action: CaseDelete}}
	pages, found := 0, 0
	for {
		p, err := s.List(ctx, tenA, q)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		found += len(p.Events)
		if p.NextCursor == "" {
			break
		}
		q.Cursor = p.NextCursor
		if pages > 10 {
			t.Fatal("listing does not terminate")
		}
	}
	if found != 1 || pages != 4 {
		t.Errorf("found %d over %d pages: each request must read at most ScanCap=10 events (want 1 over 4)", found, pages)
	}
}

func TestDenyFloodLimiterIsBoundedAndCountsHonestly(t *testing.T) {
	c := &clock{t: t0}
	st := NewMemory()
	s := NewService(st, pseud(t), Options{Now: c.now, DenyPerTenant: 3, DenyPlatform: 2, DenyGlobal: 6, DenyMaxTenants: 2})
	ctx := context.Background()
	deny := func(tenant string) bool {
		d := Draft{Event: Event{Tenant: tenant, ActorKind: ActorAnonymous, Action: CaseRead, Phase: PhaseSingle, Outcome: Denied, Reason: ReasonHidden}}
		ok, err := s.Deny(ctx, d)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	for i := 0; i < 3; i++ {
		if !deny(tenA) {
			t.Fatalf("denial %d within the cap was dropped", i)
		}
	}
	if deny(tenA) || deny(tenA) {
		t.Fatal("denials over the per-tenant cap were written")
	}
	c.advance(61 * time.Second)
	if !deny(tenA) {
		t.Fatal("the window did not reset")
	}
	last, _ := st.Range(ctx, tenA, Range{Desc: true, Limit: 1})
	if last[0].Suppressed != 2 {
		t.Errorf("suppressed = %d, want the 2 dropped denials reported", last[0].Suppressed)
	}
	// Distinct tenants beyond the tracking bound share one bounded budget; the map does not grow.
	c.advance(61 * time.Second)
	wrote := 0
	for _, ten := range []string{tenB, "ws:ws_aaaaaaaaaaaa", "ws:ws_bbbbbbbbbbbb", "ws:ws_cccccccccccc", "ws:ws_dddddddddddd", "ws:ws_eeeeeeeeeeee"} {
		if deny(ten) {
			wrote++
		}
	}
	if wrote > 4 {
		t.Errorf("%d denials from many tenants were written, the overflow/global ceilings did not hold", wrote)
	}
	s.mu.Lock()
	n := len(s.buckets)
	s.mu.Unlock()
	if n > 3 { // 2 tracked + the shared overflow bucket
		t.Errorf("the limiter tracks %d keys, want at most 3", n)
	}
	// Drops with no bucket of their own surface on the platform chain.
	c.advance(61 * time.Second)
	dp := Draft{Event: Event{Tenant: "platform", ActorKind: ActorAnonymous, Action: AuthDenied, Phase: PhaseSingle, Outcome: Denied, Reason: ReasonInvalidCredential}}
	if ok, _ := s.Deny(ctx, dp); !ok {
		t.Fatal("platform denial dropped")
	}
	p, _ := st.Range(ctx, "platform", Range{Desc: true, Limit: 1})
	if p[0].Suppressed == 0 {
		t.Error("overflow drops were not reported on the platform chain")
	}
}

func TestPurgeAllKeepsALegitimatePurgeFromLookingLikeTamper(t *testing.T) {
	eachStore(t, func(t *testing.T, _ storeKit, st Store, tm tamperer) {
		c := &clock{t: t0}
		s := NewService(st, pseud(t), Options{Now: c.now, Retention: 90 * 24 * time.Hour, PurgeChunk: 4})
		ctx := context.Background()
		for i := 0; i < 10; i++ { // ten events, one a day, from day 0
			mustRecord(t, s, userDraft(tenA, CaseUpdate))
			c.advance(24 * time.Hour)
		}
		c.t = t0.Add(95 * 24 * time.Hour) // events at day 0..4 are older than 90 days, day 5..9 are not
		n, err := s.PurgeAll(ctx)
		if err != nil || n != 5 {
			t.Fatalf("purged %d, %v; want 5 (in chunks of 4)", n, err)
		}
		res, err := s.Verify(ctx, tenA, "")
		if err != nil || !res.OK || res.Checked != 5 || res.FirstSeq != 6 || res.Checkpoint == nil || res.Checkpoint.ThroughSeq != 5 {
			t.Fatalf("after a legitimate purge: %+v %v", res, err)
		}
		if h, cp, _ := st.State(ctx, tenA); h.Seq != 10 || cp == nil || cp.ThroughSeq != 5 {
			t.Errorf("state after purge = %+v %+v", h, cp)
		}
		// Appending after a purge continues the same chain.
		e := mustRecord(t, s, userDraft(tenA, CaseUpdate))
		if e.Seq != 11 {
			t.Errorf("seq after purge = %d", e.Seq)
		}
		if n, _ := s.PurgeAll(ctx); n != 0 {
			t.Errorf("a second purge removed %d", n)
		}
		// Tampering with the retained suffix is still found after a purge.
		tm.edit(t, tenA, func(es []Event) []Event { es[1].Outcome, es[1].Reason = Failed, ReasonInternal; return es })
		if r, _ := s.Verify(ctx, tenA, ""); r.OK || !kinds(r)[FindingHash] {
			t.Errorf("tamper after purge missed: %+v", r)
		}
	})
}

func TestPurgeEverythingKeepsTheHeadAndTheChainContinues(t *testing.T) {
	eachStore(t, func(t *testing.T, _ storeKit, st Store, _ tamperer) {
		c := &clock{t: t0}
		s := NewService(st, pseud(t), Options{Now: c.now, Retention: time.Hour})
		ctx := context.Background()
		for i := 0; i < 3; i++ {
			mustRecord(t, s, userDraft(tenA, CaseUpdate))
			c.advance(time.Minute)
		}
		c.advance(48 * time.Hour)
		if n, _ := s.PurgeAll(ctx); n != 3 {
			t.Fatalf("purged %d", n)
		}
		if r, _ := s.Verify(ctx, tenA, ""); !r.OK || r.Checked != 0 {
			t.Errorf("empty retained chain: %+v", r)
		}
		if e := mustRecord(t, s, userDraft(tenA, CaseUpdate)); e.Seq != 4 {
			t.Errorf("seq = %d after purging everything", e.Seq)
		}
		if r, _ := s.Verify(ctx, tenA, ""); !r.OK || r.Checked != 1 {
			t.Errorf("chain after purge-all then append: %+v", r)
		}
	})
}

func TestCrashBetweenCheckpointAndDeleteIsNotTamper(t *testing.T) {
	// File purge writes the checkpoint first; a crash before the rewrite leaves
	// events the checkpoint already covers. Verify must skip them.
	eachStore(t, func(t *testing.T, k storeKit, st Store, tm tamperer) {
		c := &clock{t: t0}
		s := NewService(st, pseud(t), Options{Now: c.now, Retention: time.Hour})
		ctx := context.Background()
		var all []Event
		for i := 0; i < 5; i++ {
			all = append(all, mustRecord(t, s, userDraft(tenA, CaseUpdate)))
			c.advance(time.Hour)
		}
		c.advance(2 * time.Hour)
		if _, err := s.PurgeAll(ctx); err != nil {
			t.Fatal(err)
		}
		_, cp, _ := st.State(ctx, tenA)
		if cp == nil {
			t.Fatal("no checkpoint")
		}
		// Put the already-purged events back: the "deleted half" never happened.
		tm.edit(t, tenA, func(es []Event) []Event { return append(append([]Event(nil), all[:cp.ThroughSeq]...), es...) })
		if r, _ := s.Verify(ctx, tenA, ""); !r.OK {
			t.Errorf("events covered by the checkpoint were reported: %+v", r.Findings)
		}
	})
}

func TestPurgeAndAppendRaceKeepsTheChainIntact(t *testing.T) {
	eachStore(t, func(t *testing.T, _ storeKit, st Store, _ tamperer) {
		s := NewService(st, pseud(t), Options{Retention: time.Nanosecond, PurgeChunk: 3})
		ctx := context.Background()
		done := make(chan struct{})
		go func() {
			defer close(done)
			for i := 0; i < 30; i++ {
				if _, err := s.Record(ctx, userDraft(tenA, CaseUpdate)); err != nil {
					t.Error(err)
					return
				}
			}
		}()
		for i := 0; i < 10; i++ {
			if _, err := s.PurgeAll(ctx); err != nil {
				t.Error(err)
			}
		}
		<-done
		if _, err := s.PurgeAll(ctx); err != nil {
			t.Error(err)
		}
		r, err := s.Verify(ctx, tenA, "")
		if err != nil || !r.OK {
			t.Errorf("chain after concurrent append and purge: %+v %v", r, err)
		}
		if h, _, _ := st.State(ctx, tenA); h.Seq != 30 {
			t.Errorf("head seq = %d, want 30: an append was lost", h.Seq)
		}
	})
}

func TestFileStoreSurvivesRestartAndATornTail(t *testing.T) {
	dir := t.TempDir()
	f, _ := NewFile(dir)
	s := newTestService(t, f, time.Now)
	for i := 0; i < 3; i++ {
		mustRecord(t, s, userDraft(tenA, CaseUpdate))
	}
	// Simulate a crash mid-write: a partial last line with no newline.
	p := f.base(tenA) + ".jsonl"
	appendRaw(t, p, `{"tenant":"ws:ws_0123456789ab","seq":4,"ti`)
	f2, _ := NewFile(dir) // restart
	s2 := newTestService(t, f2, time.Now)
	e := mustRecord(t, s2, userDraft(tenA, CaseUpdate))
	if e.Seq != 4 {
		t.Fatalf("seq after restart with a torn tail = %d, want 4", e.Seq)
	}
	if r, err := s2.Verify(context.Background(), tenA, ""); err != nil || !r.OK || r.Checked != 4 {
		t.Errorf("verify after restart: %+v %v", r, err)
	}
}
