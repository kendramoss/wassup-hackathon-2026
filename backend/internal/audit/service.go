package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Options configure a Service. Zero values take the documented defaults.
type Options struct {
	Now       func() time.Time
	Retention time.Duration // default 90 days
	// Denial flood limits (per minute, in this process only). A request that
	// can send bad credentials must not be able to fill the store.
	DenyPerTenant   int // default 120
	DenyPlatform    int // default 30: the shared bucket for unverified callers
	DenyGlobal      int // default 600
	DenyMaxTenants  int // default 256 tracked tenants; beyond that, one shared overflow bucket
	ScanCap         int // events one listing request may read, default 1000
	PurgeChunk      int // events one purge step deletes, default DefaultPurgeChunk
	MaxPageSize     int // default 200
	DefaultPageSize int // default 50
}

func (o Options) withDefaults() Options {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Retention <= 0 {
		o.Retention = 90 * 24 * time.Hour
	}
	def := func(p *int, v int) {
		if *p <= 0 {
			*p = v
		}
	}
	def(&o.DenyPerTenant, 120)
	def(&o.DenyPlatform, 30)
	def(&o.DenyGlobal, 600)
	def(&o.DenyMaxTenants, 256)
	def(&o.ScanCap, 1000)
	def(&o.PurgeChunk, DefaultPurgeChunk)
	def(&o.MaxPageSize, 200)
	def(&o.DefaultPageSize, 50)
	return o
}

// Service writes, lists, verifies and purges audit events on a Store.
type Service struct {
	store Store
	ps    *Pseudonymizer
	opt   Options

	mu            sync.Mutex
	buckets       map[string]*denyBucket
	global        denyBucket
	sharedDropped int // drops with no tenant bucket of their own, reported on the next platform event
}

type denyBucket struct {
	start   time.Time
	count   int
	dropped int
}

// NewService returns a service on st.
func NewService(st Store, ps *Pseudonymizer, o Options) *Service {
	return &Service{store: st, ps: ps, opt: o.withDefaults(), buckets: map[string]*denyBucket{}}
}

// Pseudonyms gives callers the pseudonymizer (hidden untrusted IDs).
func (s *Service) Pseudonyms() *Pseudonymizer { return s.ps }

// Retention is the configured retention.
func (s *Service) Retention() time.Duration { return s.opt.Retention }

// Draft is an event to write plus the raw identities the pseudonyms come from.
// The raw identities are used for the HMAC and discarded: they are never
// stored.
type Draft struct {
	Event
	ActorID  string // verified email (user) or API key id (apikey)
	TargetID string // verified or normalized member email
}

func (s *Service) fill(d Draft) (Event, error) {
	e := d.Event
	switch e.ActorKind {
	case ActorUser, ActorAPIKey:
		if d.ActorID == "" {
			return Event{}, fmt.Errorf("%w: actor id", ErrInvalid)
		}
		e.Actor = s.ps.Actor(e.ActorKind, d.ActorID)
	default:
		e.Actor = ""
	}
	if d.TargetID != "" {
		e.Target = s.ps.Actor(ActorUser, d.TargetID)
	}
	e.RequestID = NormalizeRequestID(e.RequestID)
	return e, nil
}

func (s *Service) append(ctx context.Context, e Event) (Event, error) {
	now := s.opt.Now()
	return s.store.Append(ctx, e.Tenant, func(h Head) (Event, error) { return Seal(e, h, now) })
}

// Record writes d as the next event of its tenant.
func (s *Service) Record(ctx context.Context, d Draft) (Event, error) {
	e, err := s.fill(d)
	if err != nil {
		return Event{}, err
	}
	return s.append(ctx, e)
}

// Finish writes the completion (or failure) that closes intent. object, when
// set, replaces the intent's object (an API key's id is only known afterwards).
// It tries twice and no more.
func (s *Service) Finish(ctx context.Context, intent Event, outcome Outcome, reason Reason, object string) (Event, error) {
	e := Event{Tenant: intent.Tenant, ActorKind: intent.ActorKind, Actor: intent.Actor, Target: intent.Target, Action: intent.Action,
		Phase: PhaseCompletion, Outcome: outcome, Reason: reason, Detail: intent.Detail, Object: intent.Object, Route: intent.Route,
		RequestID: intent.RequestID, Ref: intent.Seq}
	if object != "" {
		e.Object = object
	}
	out, err := s.append(ctx, e)
	if err != nil && !errors.Is(err, ErrInvalid) {
		out, err = s.append(ctx, e)
	}
	return out, err
}

// Deny writes a best-effort denial through the flood limiter. It returns
// false (and no error) when the limiter dropped it; the drop is counted and
// reported in the Suppressed field of a later event.
func (s *Service) Deny(ctx context.Context, d Draft) (bool, error) {
	e, err := s.fill(d)
	if err != nil {
		return false, err
	}
	if !s.admit(e.Tenant) {
		return false, nil
	}
	e.Suppressed = s.takeDropped(e.Tenant)
	if _, err := s.append(ctx, e); err != nil {
		return false, err
	}
	return true, nil
}

// admit applies the per-tenant, platform and global ceilings.
func (s *Service) admit(tenant string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.opt.Now()
	roll := func(b *denyBucket) {
		if now.Sub(b.start) >= time.Minute || now.Before(b.start) {
			b.start, b.count = now, 0
		}
	}
	roll(&s.global)
	key, limit := tenant, s.opt.DenyPerTenant
	if tenant == "platform" {
		limit = s.opt.DenyPlatform
	}
	b, ok := s.buckets[key]
	if !ok {
		if len(s.buckets) >= s.opt.DenyMaxTenants {
			s.prune(now)
		}
		if len(s.buckets) >= s.opt.DenyMaxTenants {
			// Too many distinct tenants this minute: they share one budget and
			// their drops are reported on the platform chain.
			key, limit = "*overflow*", s.opt.DenyPlatform
			if b, ok = s.buckets[key]; !ok {
				b = &denyBucket{}
				s.buckets[key] = b
			}
		} else {
			b = &denyBucket{}
			s.buckets[key] = b
		}
	}
	roll(b)
	if s.global.count >= s.opt.DenyGlobal || b.count >= limit {
		if key == "*overflow*" || s.global.count >= s.opt.DenyGlobal {
			s.sharedDropped++
		} else {
			b.dropped++
		}
		return false
	}
	b.count++
	s.global.count++
	return true
}

func (s *Service) prune(now time.Time) {
	for k, b := range s.buckets {
		if now.Sub(b.start) >= time.Minute && b.dropped == 0 && k != "*overflow*" {
			delete(s.buckets, k)
		}
	}
}

// takeDropped returns and clears the drop count to report on tenant's next event.
func (s *Service) takeDropped(tenant string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	if b := s.buckets[tenant]; b != nil {
		n, b.dropped = b.dropped, 0
	}
	if tenant == "platform" {
		n += s.sharedDropped
		s.sharedDropped = 0
	}
	return n
}

// Filter narrows a listing. Actor is a pseudonym; there is deliberately no way
// to filter by email.
type Filter struct {
	Action   Action
	Outcome  Outcome
	Phase    Phase
	Actor    string
	Object   string
	From, To time.Time // From inclusive, To exclusive; zero: open
}

func (f Filter) match(e Event) bool {
	return (f.Action == "" || e.Action == f.Action) && (f.Outcome == "" || e.Outcome == f.Outcome) &&
		(f.Phase == "" || e.Phase == f.Phase) && (f.Actor == "" || e.Actor == f.Actor) && (f.Object == "" || e.Object == f.Object) &&
		(f.From.IsZero() || !e.Time.Before(f.From)) && (f.To.IsZero() || e.Time.Before(f.To))
}

func (f Filter) digest(desc bool) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s|%s|%d|%d|%t", f.Action, f.Outcome, f.Phase, f.Actor, f.Object,
		f.From.UnixMicro(), f.To.UnixMicro(), desc)))
	return hex.EncodeToString(sum[:8])
}

// ListQuery is one page request.
type ListQuery struct {
	Limit  int
	Desc   bool
	Cursor string
	Filter Filter
}

// Page is one page of events. A page can be short and still have a NextCursor:
// a filtered scan reads a bounded number of events per request.
type Page struct {
	Events     []Event
	NextCursor string
}

// ErrBadQuery is returned for an invalid limit, filter or cursor.
var ErrBadQuery = errors.New("audit: invalid query")

// List returns a page of tenant's events in sequence order.
func (s *Service) List(ctx context.Context, tenant string, q ListQuery) (Page, error) {
	if !ValidTenant(tenant) {
		return Page{}, ErrBadQuery
	}
	limit := q.Limit
	switch {
	case limit == 0:
		limit = s.opt.DefaultPageSize
	case limit < 0 || limit > s.opt.MaxPageSize:
		return Page{}, ErrBadQuery
	}
	digest := q.Filter.digest(q.Desc)
	var pos uint64
	if q.Cursor != "" {
		c, err := s.ps.DecodeCursor(tenant, "list", digest, q.Cursor)
		if err != nil || c.Desc != q.Desc {
			return Page{}, ErrBadQuery
		}
		pos = c.Seq
	}
	var out []Event
	scanned, exhausted := 0, false
	for len(out) < limit && scanned < s.opt.ScanCap {
		ask := min(s.opt.ScanCap-scanned, 200)
		evs, err := s.store.Range(ctx, tenant, Range{Cursor: pos, Desc: q.Desc, Limit: ask})
		if err != nil {
			return Page{}, err
		}
		for _, e := range evs {
			scanned++
			pos = e.Seq
			if q.Filter.match(e) {
				out = append(out, e)
				if len(out) == limit {
					break
				}
			}
		}
		if len(evs) < ask && len(out) < limit {
			exhausted = true
			break
		}
	}
	page := Page{Events: out}
	if !exhausted {
		if probe, err := s.store.Range(ctx, tenant, Range{Cursor: pos, Desc: q.Desc, Limit: 1}); err != nil {
			return Page{}, err
		} else if len(probe) > 0 {
			page.NextCursor = s.ps.EncodeCursor(tenant, Cursor{Kind: "list", Desc: q.Desc, Seq: pos, Filter: digest})
		}
	}
	return page, nil
}

// Verify checks tenant's retained chain, VerifyPage events per call. Pass the
// previous call's NextCursor to continue.
func (s *Service) Verify(ctx context.Context, tenant, cursor string) (VerifyResult, error) {
	if !ValidTenant(tenant) {
		return VerifyResult{}, ErrBadQuery
	}
	head, cp, err := s.store.State(ctx, tenant)
	if err != nil {
		return VerifyResult{}, err
	}
	anchor, anchored := anchorFor(cp)
	chk := &chainCheck{tenant: tenant, anchored: anchored, fromCP: cp != nil, expect: anchor}
	if cursor != "" {
		c, err := s.ps.DecodeCursor(tenant, "verify", "", cursor)
		if err != nil {
			return VerifyResult{}, ErrBadQuery
		}
		chk.expect = Head{Seq: c.Seq, Hash: c.Hash, Time: time.UnixMicro(c.TimeUS).UTC()}
		chk.fed, chk.anchored, chk.fromCP, chk.overflow = true, true, false, c.OpenOverflow
		chk.open = map[uint64]bool{}
		for _, q := range c.Open {
			chk.open[q] = true
		}
	}
	evs, err := s.store.Range(ctx, tenant, Range{Cursor: chk.expect.Seq, Limit: verifyPage})
	if err != nil {
		return VerifyResult{}, err
	}
	for _, e := range evs {
		chk.feed(e)
	}
	res := VerifyResult{Tenant: tenant, Checked: chk.checked, FirstSeq: chk.first, LastSeq: chk.expect.Seq, Complete: len(evs) < verifyPage}
	if cp != nil {
		res.Checkpoint = &CheckpointInfo{ThroughSeq: cp.ThroughSeq, PurgedAt: cp.PurgedAt}
	}
	if res.Complete {
		chk.finish(head, cp)
		un, err := s.unresolved(ctx, tenant, chk)
		if err != nil {
			return VerifyResult{}, err
		}
		res.UnresolvedIntents = un
	} else {
		last := chk.expect
		cur := Cursor{Kind: "verify", Seq: last.Seq, Hash: last.Hash, TimeUS: last.Time.UnixMicro(), OpenOverflow: chk.overflow}
		if len(chk.open) > maxCarriedOpen {
			cur.OpenOverflow = true // cannot carry them all: the final count becomes a lower bound
		} else {
			cur.Open = sortedKeys(chk.open)
		}
		res.NextCursor = s.ps.EncodeCursor(tenant, cur)
	}
	res.Findings = chk.findings
	if res.Findings == nil {
		res.Findings = []Finding{}
	}
	res.OK = len(res.Findings) == 0
	return res, nil
}

func sortedKeys(m map[uint64]bool) []uint64 {
	out := make([]uint64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// unresolved reports the open intents old enough to count, over the whole
// retained range the verification walked. Each open intent's time is read back
// (at most as many reads as intents).
func (s *Service) unresolved(ctx context.Context, tenant string, chk *chainCheck) (*Unresolved, error) {
	now := s.opt.Now()
	var seqs []uint64
	for _, q := range sortedKeys(chk.open) {
		evs, err := s.store.Range(ctx, tenant, Range{Cursor: q - 1, Limit: 1})
		if err != nil {
			return nil, err
		}
		if len(evs) == 1 && evs[0].Seq == q && now.Sub(evs[0].Time) >= unresolvedAge {
			seqs = append(seqs, q)
		}
	}
	u := &Unresolved{Count: len(seqs), Seqs: []uint64{}, Complete: !chk.overflow}
	for i := 0; i < len(seqs) && i < maxUnresolved; i++ {
		u.Seqs = append(u.Seqs, seqs[i])
	}
	return u, nil
}

// PurgeAll removes, for every tenant, events older than the retention, in
// chunks. It returns the number of events deleted.
func (s *Service) PurgeAll(ctx context.Context) (int, error) {
	tenants, err := s.store.Tenants(ctx)
	if err != nil {
		return 0, err
	}
	now := s.opt.Now()
	cutoff := now.Add(-s.opt.Retention)
	total := 0
	for _, t := range tenants {
		for {
			r, err := s.store.Purge(ctx, t, cutoff, now, s.opt.PurgeChunk)
			if err != nil {
				return total, err
			}
			total += r.Deleted
			if r.Deleted < s.opt.PurgeChunk {
				break
			}
		}
	}
	return total, nil
}

// PurgeEvery runs PurgeAll now and then every interval until ctx ends. report
// receives each outcome (deleted count, error text class only).
func (s *Service) PurgeEvery(ctx context.Context, interval time.Duration, report func(deleted int, err error)) {
	run := func() {
		n, err := s.PurgeAll(ctx)
		if report != nil {
			report(n, err)
		}
	}
	run()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}
