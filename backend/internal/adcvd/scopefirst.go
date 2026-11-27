package adcvd

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

//go:embed data/scopefirst-prompts.json
var scopeFirstPromptsJSON []byte

// The prompts of the scope-first check are the ones D959 measured:
// eval/adcvd/real/screen-prompts.json and the version adcvd-scope-v4 of
// eval/adcvd/scope-reader-prompts.json (a test keeps the copies equal).
const (
	ScreenPromptVersion      = "adcvd-real-screen-v1"
	ScopeReaderPromptVersion = "adcvd-scope-v4"
)

// StatusPerson: the reader could not settle at least one order from the
// part's description and its best guess was that the part is outside; a
// person must read the scope. It is not a flag.
const StatusPerson = "person_must_check"

// Limits of the scope-first check.
const (
	// ScopeFirstK is the number of orders the retrieval proposes per part.
	ScopeFirstK = 12
	// ScopeFirstMaxCalls caps the model calls of one request; past it a part is not checked.
	ScopeFirstMaxCalls = 400

	scopeCallTimeout   = 60 * time.Second
	screenScopeChars   = 400
	scopeFirstScopeLen = 60000
	scopeFirstDescLen  = 600
	scopeFirstCacheMax = 4096
	scopeFirstReaders  = 4
)

type promptPair struct {
	Version string `json:"version"`
	System  string `json:"system"`
	User    string `json:"user"`
}

type scopeFirstPrompts struct {
	Screen promptPair `json:"screen"`
	Reader promptPair `json:"reader"`
}

// Reading is the scope reader's answer for one order and one part.
type Reading struct {
	Verdict Verdict
	// Lean is "in" or "out": the reader's best guess if it had to decide.
	Lean   string
	Quote  string
	Reason string
}

// Flag reports whether the reading flags the order: the reader answered in,
// or could not settle it and leaned in. This is the policy D959 measured.
func (r Reading) Flag() bool {
	return r.Verdict == VerdictIn || (r.Verdict == VerdictUnclear && r.Lean == "in")
}

// ScopeFirst is the scope-first check of D959: the retrieval proposes the
// orders of the part's country by product words, a name screen drops the
// orders the part plainly is not, and a reader reads each remaining order's
// whole written scope against the part. It keeps answers in memory for the
// life of the process and never retries; an error is "no answer".
type ScopeFirst struct {
	model   llm.Model
	modelID string
	prompts scopeFirstPrompts

	mu      sync.Mutex
	screens map[string][]bool
	reads   map[string]Reading
}

// NewScopeFirst returns a check that asks modelID of m.
func NewScopeFirst(m llm.Model, modelID string) (*ScopeFirst, error) {
	s := &ScopeFirst{model: m, modelID: modelID, screens: map[string][]bool{}, reads: map[string]Reading{}}
	p := &s.prompts
	if err := json.Unmarshal(scopeFirstPromptsJSON, p); err != nil || p.Screen.Version != ScreenPromptVersion || p.Reader.Version != ScopeReaderPromptVersion ||
		p.Screen.System == "" || p.Screen.User == "" || p.Reader.System == "" || p.Reader.User == "" {
		return nil, fmt.Errorf("adcvd: scope-first prompts are not %s and %s", ScreenPromptVersion, ScopeReaderPromptVersion)
	}
	return s, nil
}

// Model is the model the check asks.
func (s *ScopeFirst) Model() string { return s.modelID }

func (s *ScopeFirst) ask(ctx context.Context, system, user string) (llm.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, scopeCallTimeout)
	defer cancel()
	return s.model.Generate(ctx, llm.Request{Model: s.modelID, System: system, Messages: []llm.Message{{Role: llm.RoleUser, Content: user}}, MaxOutputTokens: 700})
}

func scopeFirstPart(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func screenKey(desc string, pool []PoolEntry) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s", ScreenPromptVersion, desc)
	for _, e := range pool {
		fmt.Fprintf(h, "\x00%s\x00%v", PairKey(Order{Product: e.Product, Country: e.Country, ScopeText: e.Scope}, desc), e.HTSMatch)
	}
	return hex.EncodeToString(h.Sum(nil))[:24]
}

// screenList is the numbered list of orders the name screen is shown.
func screenList(pool []PoolEntry) string {
	var b strings.Builder
	for i, e := range pool {
		if i > 0 {
			b.WriteString("\n")
		}
		flag := ""
		if e.HTSMatch {
			flag = " [its listed HTS numbers include the product's code]"
		}
		fmt.Fprintf(&b, "%d. %s (%s)%s\n   %s", i+1, e.Product, strings.Join(e.Cases, " / "), flag, scopeFirstPart(strings.Join(strings.Fields(e.Scope), " "), screenScopeChars))
	}
	return b.String()
}

// Screen returns, for each order of the pool, whether the name screen lets
// it through to the reader. Every order the screen does not list is dropped.
func (s *ScopeFirst) Screen(ctx context.Context, desc string, pool []PoolEntry) ([]bool, error) {
	key := screenKey(desc, pool)
	s.mu.Lock()
	v, ok := s.screens[key]
	s.mu.Unlock()
	if ok {
		return v, nil
	}
	user := strings.NewReplacer("{country}", pool[0].Country, "{description}", desc, "{facts}", "", "{orders}", screenList(pool)).Replace(s.prompts.Screen.User)
	resp, err := s.ask(ctx, s.prompts.Screen.System, user)
	if err != nil {
		return nil, err
	}
	var a struct {
		Possible []json.RawMessage `json:"possible"`
	}
	if err := resp.DecodeJSON(&a); err != nil {
		return nil, err
	}
	out := make([]bool, len(pool))
	for _, raw := range a.Possible {
		var n float64
		if json.Unmarshal(raw, &n) != nil {
			var str string
			if json.Unmarshal(raw, &str) != nil {
				continue
			}
			f, err := strconv.ParseFloat(strings.TrimSpace(str), 64)
			if err != nil {
				continue
			}
			n = f
		}
		if i := int(n); float64(i) == n && i >= 1 && i <= len(pool) {
			out[i-1] = true
		}
	}
	s.mu.Lock()
	if len(s.screens) >= scopeFirstCacheMax {
		s.screens = map[string][]bool{}
	}
	s.screens[key] = out
	s.mu.Unlock()
	return out, nil
}

// Read has the reader read one order group's written scope against the part.
func (s *ScopeFirst) Read(ctx context.Context, desc string, c Candidate) (Reading, error) {
	key := PairKey(Order{Product: c.Product, Country: c.Country, ScopeText: c.Scope}, desc)
	s.mu.Lock()
	v, ok := s.reads[key]
	s.mu.Unlock()
	if ok {
		return v, nil
	}
	user := strings.NewReplacer("{product}", c.Product, "{country}", c.Country, "{scope}", scopeFirstPart(c.Scope, scopeFirstScopeLen), "{description}", desc).Replace(s.prompts.Reader.User)
	resp, err := s.ask(ctx, s.prompts.Reader.System, user)
	if err != nil {
		return Reading{}, err
	}
	var a struct {
		Verdict string `json:"verdict"`
		Lean    string `json:"lean"`
		Quote   string `json:"quote"`
		Reason  string `json:"reason"`
	}
	if err := resp.DecodeJSON(&a); err != nil {
		return Reading{}, err
	}
	v = Reading{Quote: clip(a.Quote, 400), Reason: clip(a.Reason, 400)}
	switch Verdict(strings.ToLower(strings.TrimSpace(a.Verdict))) {
	case VerdictIn:
		v.Verdict = VerdictIn
	case VerdictOut:
		v.Verdict = VerdictOut
	default:
		v.Verdict = VerdictUnclear
	}
	if l := strings.ToLower(strings.TrimSpace(a.Lean)); l == "in" || l == "out" {
		v.Lean = l
	}
	s.mu.Lock()
	if len(s.reads) >= scopeFirstCacheMax {
		s.reads = map[string]Reading{}
	}
	s.reads[key] = v
	s.mu.Unlock()
	return v, nil
}

// ScopeFirstLimits says what the scope-first check cannot see.
func ScopeFirstLimits(snapshot string) []string {
	return []string{
		"The orders are those open to review in the twelve months to " + snapshot + ", read from the newest Commerce notice that carries each scope, appendices included. A newer order, revocation or scope change is not in the list.",
		"A model proposes the orders by the part's name and reads each one's written scope against it. It sees only what the name says: a scope that turns on material, size, process or use the name does not state is often unsettled. It can be wrong, and agreement between models is not Commerce's decision.",
		"Measured on everyday imports, the check flagged rightly on cases careful readers agree on; on cases where they disagree (a part near the edge of a scope) it flags few and a person must decide.",
		"Goods finished in a third country from parts of an ordered country can be within an order's scope (circumvention) and are not seen here.",
		"Possible exposure is not a finding that duties are owed, and no match is not a finding that they are not. The written scope governs; ask a customs broker or Commerce for a scope ruling.",
	}
}

// ScopeFirstNotice is shown with every scope-first result.
const ScopeFirstNotice = "Screening only. A model read the written scope of each order that could cover a part against the part's name; the written scope governs, and only the U.S. Department of Commerce decides whether a product is within it. This does not change the origin answer or any duty figure."

type callBudget struct{ left int64 }

func (b *callBudget) take() bool { return atomic.AddInt64(&b.left, -1) >= 0 }

// CheckScopeFirst screens each part by the scope-first method of D959. A
// part needs a name (the description the reader sees) and an origin; its
// HTS code is only a hint to the retrieval and never removes an order.
func (t *Table) CheckScopeFirst(in Input, sf *ScopeFirst) Result {
	return t.CheckScopeFirstCtx(context.Background(), in, sf, ScopeFirstMaxCalls)
}

// CheckScopeFirstCtx is CheckScopeFirst with a context (a part whose calls
// run past it is not checked) and a cap on the model calls of the request.
func (t *Table) CheckScopeFirstCtx(ctx context.Context, in Input, sf *ScopeFirst, maxCalls int) Result {
	res := Result{Snapshot: t.snapshot, Fetched: t.fetched, Orders: len(t.orders), Notice: ScopeFirstNotice, Limits: ScopeFirstLimits(t.snapshot), Parts: make([]PartResult, len(in.Parts))}
	budget := &callBudget{left: int64(maxCalls)}
	var wg sync.WaitGroup
	sem := make(chan struct{}, readerParallel)
	for i, p := range in.Parts {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			res.Parts[i] = t.scopeFirstPart(ctx, p, sf, budget)
		}()
	}
	wg.Wait()
	return res
}

func (t *Table) scopeFirstMatch(c Candidate, rd Reading) Match {
	m := Match{Cases: c.Cases, Types: c.Types, Country: c.Country, Product: c.Product, ScopeClause: rd.Quote}
	if o, ok := t.orderOf(c.Cases[0]); ok {
		m.OrderDate, m.Source = o.OrderDate, o.ScopeNotice
	}
	if rd.Verdict == VerdictIn {
		m.Reading = "in"
	} else {
		m.Reading = "unclear"
		m.Why = rd.Reason
	}
	return m
}

func (t *Table) scopeFirstPart(ctx context.Context, p Part, sf *ScopeFirst, budget *callBudget) PartResult {
	r := PartResult{Name: p.Name, HTS: strings.TrimSpace(p.HTS), Origin: strings.TrimSpace(p.Origin)}
	desc := clip(p.Name, scopeFirstDescLen)
	switch {
	case desc == "":
		r.Status, r.Reason = StatusNotChecked, "No part name or description given: the check reads each order's written scope against it."
		return r
	case r.Origin == "":
		r.Status, r.Reason = StatusNotChecked, "No country of origin given for this part."
		return r
	case !t.HasCountry(r.Origin):
		r.Status, r.Reason = StatusNone, "No active order covers products made in this country."
		return r
	}
	pool := t.Pool(Query{Description: desc, Country: r.Origin, HTS: r.HTS}, ScopeFirstK)
	if len(pool) == 0 {
		r.Status, r.Reason = StatusNone, "No active order for this country names or describes a product word of this part."
		return r
	}
	notRead := func(why string) PartResult {
		r.Status, r.Reason = StatusNotChecked, why
		return r
	}
	if !budget.take() {
		return notRead("The request asked for more model reads than one request allows; check this part in a request of its own.")
	}
	keep, err := sf.Screen(ctx, desc, pool)
	if err != nil {
		return notRead("The scope reader did not answer; try again.")
	}
	type job struct {
		i  int
		rd Reading
		ok bool
	}
	var jobs []*job
	for i, e := range pool {
		if keep[i] && e.Rank >= 1 && e.Rank <= ScopeFirstK {
			jobs = append(jobs, &job{i: i})
		}
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, scopeFirstReaders)
	for _, j := range jobs {
		if !budget.take() {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if rd, err := sf.Read(ctx, desc, pool[j.i].Candidate); err == nil {
				j.rd, j.ok = rd, true
			}
		}()
	}
	wg.Wait()
	var review []Match
	unread := 0
	for _, j := range jobs {
		switch {
		case !j.ok:
			unread++
		case j.rd.Flag():
			r.Matches = append(r.Matches, t.scopeFirstMatch(pool[j.i].Candidate, j.rd))
		case j.rd.Verdict == VerdictUnclear:
			review = append(review, t.scopeFirstMatch(pool[j.i].Candidate, j.rd))
		}
	}
	r.Review = review
	switch {
	case len(r.Matches) > 0:
		r.Status = StatusPossible
		r.Reason = fmt.Sprintf("%d active order(s) whose written scope may cover this part; the written scope governs.", len(r.Matches))
		if n := countUnclear(r.Matches); n > 0 {
			r.Reason += fmt.Sprintf(" For %d of them the reader could not settle it from the part's name: a person must check.", n)
		}
	case len(review) > 0:
		r.Status = StatusPerson
		r.Reason = fmt.Sprintf("The reader could not settle %d active order(s) from the part's name and leaned away from them; a person must check the written scope.", len(review))
	case unread > 0:
		return notRead("The scope reader did not answer for every order it was asked about; try again.")
	default:
		r.Status = StatusNone
		r.Reason = "No active order's written scope, read against this part's name, may cover it."
	}
	if unread > 0 && r.Status != StatusNotChecked {
		r.Reason += fmt.Sprintf(" The reader did not answer for %d order(s).", unread)
	}
	return r
}

func countUnclear(ms []Match) int {
	n := 0
	for _, m := range ms {
		if m.Reading == "unclear" {
			n++
		}
	}
	return n
}
