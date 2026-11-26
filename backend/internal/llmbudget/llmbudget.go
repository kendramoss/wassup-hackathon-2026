// Package llmbudget caps paid model spend across processes. A shared ledger
// file holds one line per reservation and settlement. Every call reserves its
// worst case before the request, under a file lock, so concurrent workers and
// separate runs (admission, smoke, synthetic, held-out) cannot together pass
// the cap. A reservation that is never settled stays at its worst case.
//
// Costs are never invented: a call whose gateway reported no cost is settled
// from token usage and verified rates, or from its worst case, and the
// provenance of every settlement is recorded.
package llmbudget

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// MaxCapUSD is the largest total the approved Japan evaluation may spend.
const MaxCapUSD = 60.0

// ErrStopped is returned for every reservation once a settlement passed its
// reservation. The stop is durable: it survives restarts and only a person
// clearing the ledger can lift it.
var ErrStopped = errors.New("llmbudget: stopped after a cost passed its reservation")

// ErrOverrun reports the settlement that tripped the stop.
var ErrOverrun = errors.New("llmbudget: a call cost more than was reserved; the ledger is stopped")

// ErrClaimed is returned when a protocol run was already started, finished or
// crashed under the same key.
var ErrClaimed = errors.New("llmbudget: this run was already claimed; an incomplete attempt stays recorded and is not repeated")

// ErrBinding is returned when a ledger was opened with a different cap,
// model or rates than its first line binds.
var ErrBinding = errors.New("llmbudget: the ledger is bound to a different cap, model or rates")

// ErrNotPlanned is returned for a phase the ledger's manifest does not list,
// or one whose planned requests differ from the manifest.
var ErrNotPlanned = errors.New("llmbudget: this phase is not in the ledger's plan, or it differs from it")

// ErrNoLedger is returned when the ledger file is missing, empty or has no
// header. Only Create starts a ledger, so a mistyped path cannot start a new
// budget.
var ErrNoLedger = errors.New("llmbudget: no initialised ledger at this path")

// ErrBudget is returned, with no model call made, when a reservation would
// pass the cap.
var ErrBudget = errors.New("llmbudget: budget cap would be exceeded")

// Provenance of a settled cost.
const (
	Reported            = "reported"                       // gateway cost header
	EstimatedFromUsage  = "estimated_from_usage_and_rates" // no header; response usage priced at the verified rates
	UnreportedRejection = "unreported_http_rejection"      // HTTP 4xx without a header; request input bound only
	UnreportedWorstCase = "unreported_worst_case"          // no header and no usable usage; full reservation kept
)

// Rates are the verified prices of the model in USD per million tokens.
// Source names the quote or price page checked before any paid call.
type Rates struct {
	InputPerMTok  float64 `json:"input_usd_per_mtok"`
	OutputPerMTok float64 `json:"output_usd_per_mtok"`
	Source        string  `json:"source"`
}

// Validate refuses missing, zero, negative or non-finite rates.
func (r Rates) Validate() error {
	for _, v := range []float64{r.InputPerMTok, r.OutputPerMTok} {
		if !(v > 0) || math.IsInf(v, 0) || v > 1000 {
			return errors.New("llmbudget: input and output rates must be verified positive USD per million tokens")
		}
	}
	if strings.TrimSpace(r.Source) == "" {
		return errors.New("llmbudget: rates need a source (the quote or price page that was checked)")
	}
	return nil
}

// WorstCase bounds one request: inputTokens at the input rate plus the full
// output cap at the output rate.
func (r Rates) WorstCase(inputTokens, maxOutput int) float64 {
	return (float64(inputTokens)*r.InputPerMTok + float64(maxOutput)*r.OutputPerMTok) / 1e6
}

type entry struct {
	Kind  string  `json:"t"`
	ID    string  `json:"id,omitempty"`
	USD   float64 `json:"usd"`
	Label string  `json:"label,omitempty"`
	Prov  string  `json:"prov,omitempty"`
	At    string  `json:"at"`
	// Header fields (first line only).
	Cap   float64 `json:"cap,omitempty"`
	Model string  `json:"model,omitempty"`
	In    float64 `json:"in_usd_per_mtok,omitempty"`
	Out   float64 `json:"out_usd_per_mtok,omitempty"`
	Src   string  `json:"rates_source,omitempty"`
	// Phases is the suite manifest, also fixed by the header.
	Phases map[string]Phase `json:"phases,omitempty"`
}

// Phase is one planned stage of the suite (smoke, held_out, synthetic): its
// worst-case cost at the bound rates and the exact requests it will send.
type Phase struct {
	WorstUSD       float64 `json:"worst_usd"`
	Requests       int     `json:"requests"`
	RequestsSHA256 string  `json:"requests_sha256"`
}

// Binding is what the first line of a ledger fixes for good.
type Binding struct {
	Model string
	Rates Rates
}

// Ledger is a cap and its shared file.
type Ledger struct {
	path     string
	cap      float64
	bind     Binding
	phases   map[string]Phase
	creating bool
	mu       sync.Mutex
	seq      atomic.Int64
	pid      int
}

func (l *Ledger) header() entry {
	return entry{Kind: "header", Cap: l.cap, Model: l.bind.Model, In: l.bind.Rates.InputPerMTok,
		Out: l.bind.Rates.OutputPerMTok, Src: l.bind.Rates.Source, Phases: l.phases}
}

// Phase returns a planned phase from the manifest.
func (l *Ledger) Phase(name string) (Phase, bool) { p, ok := l.phases[name]; return p, ok }

func newLedger(path string, capUSD float64, b Binding) (*Ledger, error) {
	if !(capUSD > 0) || capUSD > MaxCapUSD {
		return nil, fmt.Errorf("llmbudget: cap must be above 0 and at most $%.2f", MaxCapUSD)
	}
	if err := b.Rates.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(b.Model) == "" {
		return nil, errors.New("llmbudget: the ledger needs a model name")
	}
	return &Ledger{path: path, cap: capUSD, bind: b, pid: os.Getpid()}, nil
}

// Create starts a new ledger and writes its header: the cap, model, rates and
// the suite manifest. It fails when the path exists, and it fails when the
// phases together would not fit the cap, before anything is written.
func Create(path string, capUSD float64, b Binding, phases map[string]Phase) (*Ledger, error) {
	l, err := newLedger(path, capUSD, b)
	if err != nil {
		return nil, err
	}
	var total float64
	for name, p := range phases {
		if strings.TrimSpace(name) == "" || !(p.WorstUSD >= 0) || p.Requests < 0 {
			return nil, errors.New("llmbudget: invalid phase in the plan")
		}
		total += p.WorstUSD
	}
	if total > capUSD+1e-9 {
		return nil, fmt.Errorf("%w: the whole plan needs up to $%.4f, the cap is $%.2f", ErrBudget, total, capUSD)
	}
	l.phases, l.creating = phases, true
	if l.phases == nil {
		l.phases = map[string]Phase{}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return nil, errors.New("llmbudget: the ledger cannot be created exclusively; it may already exist")
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	if err := l.locked(func(*os.File, state) error { return nil }); err != nil { // writes the header
		return nil, err
	}
	l.creating = false
	return l, nil
}

// Open opens an existing, initialised ledger. The cap, model and rates must
// equal what its header fixed. A missing, empty or headerless file is an
// error: a new path never starts another budget.
func Open(path string, capUSD float64, b Binding) (*Ledger, error) {
	l, err := newLedger(path, capUSD, b)
	if err != nil {
		return nil, err
	}
	if err := l.locked(func(*os.File, state) error { return nil }); err != nil {
		return nil, err
	}
	return l, nil
}

// Cap returns the total cap in USD.
func (l *Ledger) Cap() float64 { return l.cap }

type state struct {
	committed float64
	stopped   bool
	claims    map[string]bool
}

func (l *Ledger) locked(fn func(f *os.File, st state) error) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.OpenFile(l.path, os.O_RDWR|os.O_APPEND, 0o600)
	if errors.Is(err, os.ErrNotExist) {
		return ErrNoLedger
	}
	if err != nil {
		return fmt.Errorf("llmbudget: cannot open the ledger: %w", errors.Unwrap(err))
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("llmbudget: cannot lock the ledger")
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	st, empty, err := l.scan(f)
	if err != nil {
		return err
	}
	if empty {
		if l.phases == nil || !l.creating {
			return ErrNoLedger
		}
		if err := l.append(f, l.header()); err != nil {
			return err
		}
	}
	return fn(f, st)
}

// scan reads every line and checks the header; anything unreadable fails
// closed. It never repairs or rewrites the file.
func (l *Ledger) scan(f *os.File) (st state, empty bool, err error) {
	if _, err := f.Seek(0, 0); err != nil {
		return st, false, err
	}
	st.claims = map[string]bool{}
	reserved, settled := map[string]float64{}, map[string]float64{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	n := 0
	for sc.Scan() {
		n++
		var e entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil || !(e.USD >= 0) || math.IsInf(e.USD, 0) {
			return st, false, fmt.Errorf("llmbudget: ledger line %d is unreadable; refusing to spend", n)
		}
		if (n == 1) != (e.Kind == "header") {
			return st, false, fmt.Errorf("llmbudget: ledger line %d: the header must be the first line and appear once; refusing to spend", n)
		}
		switch e.Kind {
		case "header":
			if e.Cap != l.cap || e.Model != l.bind.Model || e.In != l.bind.Rates.InputPerMTok ||
				e.Out != l.bind.Rates.OutputPerMTok || e.Src != l.bind.Rates.Source {
				return st, false, ErrBinding
			}
			if l.phases == nil {
				l.phases = e.Phases
				if l.phases == nil {
					l.phases = map[string]Phase{}
				}
			} else if !reflect.DeepEqual(l.phases, e.Phases) && !(len(l.phases) == 0 && len(e.Phases) == 0) {
				return st, false, ErrBinding
			}
		case "reserve":
			if e.ID == "" {
				return st, false, fmt.Errorf("llmbudget: ledger line %d has no id; refusing to spend", n)
			}
			if _, dup := reserved[e.ID]; dup {
				return st, false, fmt.Errorf("llmbudget: ledger line %d repeats a reservation id; refusing to spend", n)
			}
			reserved[e.ID] = e.USD
		case "settle":
			if e.ID == "" {
				return st, false, fmt.Errorf("llmbudget: ledger line %d has no id; refusing to spend", n)
			}
			settled[e.ID] += e.USD
		case "stop":
			st.stopped = true
		case "claim":
			st.claims[e.ID] = true
		default:
			return st, false, fmt.Errorf("llmbudget: ledger line %d has an unknown kind; refusing to spend", n)
		}
	}
	if err := sc.Err(); err != nil {
		return st, false, err
	}
	for id, v := range reserved {
		if sv, ok := settled[id]; ok {
			st.committed += sv
		} else {
			st.committed += v
		}
	}
	for id, sv := range settled {
		if rv, ok := reserved[id]; !ok {
			st.committed += sv
		} else if sv > rv+1e-9 {
			// Derived from the amounts, so a crash between the settlement and
			// the stop line cannot leave an overrun without its stop.
			st.stopped = true
		}
	}
	return st, n == 0, nil
}

func (l *Ledger) append(f *os.File, e entry) error {
	e.At = time.Now().UTC().Format(time.RFC3339Nano)
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

// Committed returns settled spend plus unsettled reservations.
func (l *Ledger) Committed() (float64, error) {
	var out float64
	err := l.locked(func(_ *os.File, st state) error { out = st.committed; return nil })
	return out, err
}

// Remaining returns the cap minus Committed.
func (l *Ledger) Remaining() (float64, error) {
	c, err := l.Committed()
	return l.cap - c, err
}

// Reservation is held until Settle.
type Reservation struct {
	l     *Ledger
	id    string
	worst float64
	once  sync.Once
}

// Claim records, before any request, that the run under key has started. A
// second claim of the same key fails with ErrClaimed whether the first run
// finished, failed or crashed. Claims are never released.
func (l *Ledger) Claim(key string) error {
	if strings.TrimSpace(key) == "" {
		return errors.New("llmbudget: empty claim key")
	}
	id := "claim:" + key
	return l.locked(func(f *os.File, st state) error {
		if st.claims[id] {
			return ErrClaimed
		}
		return l.append(f, entry{Kind: "claim", ID: id})
	})
}

// Affordable checks, before the first billable call, that what is left covers
// this phase's planned worst case AND the planned worst case of every other
// phase that has not been claimed yet, so the whole suite fits together and
// no phase can spend what a later one needs.
func (l *Ledger) Affordable(phase string) error {
	p, ok := l.phases[phase]
	if !ok {
		return fmt.Errorf("%w: %q", ErrNotPlanned, phase)
	}
	return l.locked(func(_ *os.File, st state) error {
		need := p.WorstUSD
		for name, other := range l.phases {
			if name == phase {
				continue
			}
			claimed := false
			for id := range st.claims {
				if strings.HasSuffix(id, "|"+name) {
					claimed = true
				}
			}
			if !claimed {
				need += other.WorstUSD
			}
		}
		if st.committed+need > l.cap+1e-9 {
			return fmt.Errorf("%w: committed $%.4f, this phase and the unstarted phases need up to $%.4f, cap $%.2f", ErrBudget, st.committed, need, l.cap)
		}
		return nil
	})
}

// Stopped reports whether the durable stop is set.
func (l *Ledger) Stopped() (bool, error) {
	var out bool
	err := l.locked(func(_ *os.File, st state) error { out = st.stopped; return nil })
	return out, err
}

// Reserve holds worstUSD. It fails with ErrStopped after an overrun and with
// ErrBudget, writing nothing, when the total would pass the cap.
func (l *Ledger) Reserve(label string, worstUSD float64) (*Reservation, error) {
	if !(worstUSD >= 0) || math.IsInf(worstUSD, 0) {
		return nil, errors.New("llmbudget: invalid reservation")
	}
	r := &Reservation{l: l, worst: worstUSD, id: newID(l)}
	err := l.locked(func(f *os.File, st state) error {
		if st.stopped {
			return ErrStopped
		}
		if st.committed+worstUSD > l.cap+1e-9 {
			return fmt.Errorf("%w: committed $%.4f, this call up to $%.4f, cap $%.2f", ErrBudget, st.committed, worstUSD, l.cap)
		}
		return l.append(f, entry{Kind: "reserve", ID: r.id, USD: worstUSD, Label: label})
	})
	if err != nil {
		return nil, err
	}
	return r, nil
}

// Settle replaces the reservation with the actual cost. Only the first call
// has an effect. A cost above the reservation is recorded as it was, the
// durable stop is written in the same step, and ErrOverrun is returned: money
// already charged cannot be undone, but nothing further is reserved.
func (r *Reservation) Settle(actualUSD float64, provenance string) (err error) {
	r.once.Do(func() {
		if !(actualUSD >= 0) || math.IsInf(actualUSD, 0) {
			err = errors.New("llmbudget: invalid settlement")
			return
		}
		err = r.l.locked(func(f *os.File, _ state) error {
			if err := r.l.append(f, entry{Kind: "settle", ID: r.id, USD: actualUSD, Prov: provenance}); err != nil {
				return err
			}
			if actualUSD > r.worst+1e-9 {
				if err := r.l.append(f, entry{Kind: "stop", ID: r.id, USD: actualUSD, Label: fmt.Sprintf("settled $%.6f above reserved $%.6f", actualUSD, r.worst)}); err != nil {
					return err
				}
				return fmt.Errorf("%w: settled $%.6f, reserved $%.6f", ErrOverrun, actualUSD, r.worst)
			}
			return nil
		})
	})
	return err
}

// Cost is what one budgeted call cost and how that is known.
type Cost struct {
	USD        float64
	Provenance string
	// Overrun is set when this call cost more than its reservation; the
	// ledger is then stopped for good.
	Overrun bool
}

type sinkKey struct{}

// WithSink makes calls made with the returned context report their settled
// cost into c.
func WithSink(ctx context.Context, c *Cost) context.Context {
	return context.WithValue(ctx, sinkKey{}, c)
}

// Model wraps inner so every call reserves its worst case first. size must
// give the exact request body length in bytes; a token never takes less than
// a byte, so bytes bound input tokens from above. The wrapper never retries.
type Model struct {
	Inner  llm.Model
	Ledger *Ledger
	Rates  Rates
	Size   func(llm.Request) (int, error)
}

// Provider implements llm.Model.
func (m *Model) Provider() string { return m.Inner.Provider() }

// WorstCase prices req at its byte bound and output cap.
func (m *Model) WorstCase(req llm.Request) (float64, int, error) {
	if req.MaxOutputTokens <= 0 {
		return 0, 0, errors.New("llmbudget: the request has no output cap")
	}
	n, err := m.Size(req)
	if err != nil {
		return 0, 0, err
	}
	return m.Rates.WorstCase(n, req.MaxOutputTokens), n, nil
}

// Generate implements llm.Model.
func (m *Model) Generate(ctx context.Context, req llm.Request) (llm.Response, error) {
	worst, _, err := m.WorstCase(req)
	if err != nil {
		return llm.Response{}, err
	}
	res, err := m.Ledger.Reserve(req.Model, worst)
	if err != nil {
		return llm.Response{}, err
	}
	inner := &llm.CostTally{}
	resp, callErr := m.Inner.Generate(llm.ContextWithCostTally(ctx, inner), req)
	// Policy: only a positive reported cost, or positive token usage priced at
	// the verified rates, settles below the reservation. A zero or missing
	// cost with no usage, and every error without usage (a 4xx can follow
	// generation), keep the full worst case. Zero is never taken as free.
	cost := Cost{USD: worst, Provenance: UnreportedWorstCase}
	switch {
	case inner.Reported() > 0 && inner.USD() > 0:
		cost = Cost{USD: inner.USD(), Provenance: Reported}
	case resp.Usage.InputTokens+resp.Usage.OutputTokens > 0:
		cost = Cost{USD: m.Rates.WorstCase(resp.Usage.InputTokens, resp.Usage.OutputTokens), Provenance: EstimatedFromUsage}
	}
	if outer := llm.CostTallyFrom(ctx); outer != nil && inner.Reported() > 0 {
		outer.Record(inner.USD())
	}
	if sink, _ := ctx.Value(sinkKey{}).(*Cost); sink != nil {
		*sink = cost
	}
	if err := res.Settle(cost.USD, cost.Provenance); err != nil {
		if errors.Is(err, ErrOverrun) {
			// The answer is kept and the overrun is reported with the row.
			cost.Overrun = true
			if sink, _ := ctx.Value(sinkKey{}).(*Cost); sink != nil {
				*sink = cost
			}
			return resp, callErr
		}
		return resp, errors.Join(callErr, err)
	}
	return resp, callErr
}

var _ llm.Model = (*Model)(nil)

func newID(l *Ledger) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("llmbudget: no randomness")
	}
	return fmt.Sprintf("%d-%d-%d-%x", l.pid, time.Now().UnixNano(), l.seq.Add(1), b)
}
