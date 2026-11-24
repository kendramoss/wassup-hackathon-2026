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
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// MaxCapUSD is the largest total the approved Japan evaluation may spend.
const MaxCapUSD = 50.0

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
	ID    string  `json:"id"`
	USD   float64 `json:"usd"`
	Label string  `json:"label,omitempty"`
	Prov  string  `json:"prov,omitempty"`
	At    string  `json:"at"`
}

// Ledger is a cap and its shared file.
type Ledger struct {
	path string
	cap  float64
	mu   sync.Mutex
	seq  atomic.Int64
	pid  int
}

// Open returns the ledger at path, creating it when absent. The cap must be
// positive and at most MaxCapUSD.
func Open(path string, capUSD float64) (*Ledger, error) {
	if !(capUSD > 0) || capUSD > MaxCapUSD {
		return nil, fmt.Errorf("llmbudget: cap must be above 0 and at most $%.2f", MaxCapUSD)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("llmbudget: cannot open the ledger: %w", errors.Unwrap(err))
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	return &Ledger{path: path, cap: capUSD, pid: os.Getpid()}, nil
}

// Cap returns the total cap in USD.
func (l *Ledger) Cap() float64 { return l.cap }

func (l *Ledger) locked(fn func(f *os.File, committed float64) error) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.OpenFile(l.path, os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("llmbudget: cannot open the ledger: %w", errors.Unwrap(err))
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("llmbudget: cannot lock the ledger")
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	committed, err := sum(f)
	if err != nil {
		return err
	}
	return fn(f, committed)
}

// sum reads every line; an unreadable line fails closed.
func sum(f *os.File) (float64, error) {
	if _, err := f.Seek(0, 0); err != nil {
		return 0, err
	}
	reserved, settled := map[string]float64{}, map[string]float64{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for n := 1; sc.Scan(); n++ {
		var e entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil || e.ID == "" || !(e.USD >= 0) || math.IsInf(e.USD, 0) {
			return 0, fmt.Errorf("llmbudget: ledger line %d is unreadable; refusing to spend", n)
		}
		switch e.Kind {
		case "reserve":
			reserved[e.ID] = e.USD
		case "settle":
			settled[e.ID] += e.USD
		default:
			return 0, fmt.Errorf("llmbudget: ledger line %d has an unknown kind; refusing to spend", n)
		}
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}
	var total float64
	for id, v := range reserved {
		if s, ok := settled[id]; ok {
			total += s
		} else {
			total += v
		}
	}
	for id, s := range settled {
		if _, ok := reserved[id]; !ok {
			total += s
		}
	}
	return total, nil
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
	err := l.locked(func(_ *os.File, c float64) error { out = c; return nil })
	return out, err
}

// Remaining returns the cap minus Committed.
func (l *Ledger) Remaining() (float64, error) {
	c, err := l.Committed()
	return l.cap - c, err
}

// Reservation is held until Settle.
type Reservation struct {
	l    *Ledger
	id   string
	once sync.Once
}

// Reserve holds worstUSD. It fails with ErrBudget, writing nothing, when the
// total would pass the cap.
func (l *Ledger) Reserve(label string, worstUSD float64) (*Reservation, error) {
	if !(worstUSD >= 0) || math.IsInf(worstUSD, 0) {
		return nil, errors.New("llmbudget: invalid reservation")
	}
	r := &Reservation{l: l, id: fmt.Sprintf("%d-%d-%d", l.pid, time.Now().UnixNano(), l.seq.Add(1))}
	err := l.locked(func(f *os.File, committed float64) error {
		if committed+worstUSD > l.cap+1e-9 {
			return fmt.Errorf("%w: committed $%.4f, this call up to $%.4f, cap $%.2f", ErrBudget, committed, worstUSD, l.cap)
		}
		return l.append(f, entry{Kind: "reserve", ID: r.id, USD: worstUSD, Label: label})
	})
	if err != nil {
		return nil, err
	}
	return r, nil
}

// Settle replaces the reservation with the actual cost. Only the first call
// has an effect.
func (r *Reservation) Settle(actualUSD float64, provenance string) (err error) {
	r.once.Do(func() {
		if !(actualUSD >= 0) || math.IsInf(actualUSD, 0) {
			err = errors.New("llmbudget: invalid settlement")
			return
		}
		err = r.l.locked(func(f *os.File, _ float64) error {
			return r.l.append(f, entry{Kind: "settle", ID: r.id, USD: actualUSD, Prov: provenance})
		})
	})
	return err
}

// Cost is what one budgeted call cost and how that is known.
type Cost struct {
	USD        float64
	Provenance string
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
	worst, n, err := m.WorstCase(req)
	if err != nil {
		return llm.Response{}, err
	}
	res, err := m.Ledger.Reserve(req.Model, worst)
	if err != nil {
		return llm.Response{}, err
	}
	inner := &llm.CostTally{}
	resp, callErr := m.Inner.Generate(llm.ContextWithCostTally(ctx, inner), req)
	cost := Cost{USD: worst, Provenance: UnreportedWorstCase}
	var api *llm.APIError
	switch {
	case inner.Reported() > 0:
		cost = Cost{USD: inner.USD(), Provenance: Reported}
	case callErr == nil && resp.Usage.InputTokens+resp.Usage.OutputTokens > 0:
		cost = Cost{USD: m.Rates.WorstCase(resp.Usage.InputTokens, resp.Usage.OutputTokens), Provenance: EstimatedFromUsage}
	case errors.As(callErr, &api) && api.StatusCode >= 400 && api.StatusCode < 500:
		cost = Cost{USD: m.Rates.WorstCase(n, 0), Provenance: UnreportedRejection}
	}
	if outer := llm.CostTallyFrom(ctx); outer != nil && inner.Reported() > 0 {
		outer.Record(inner.USD())
	}
	if sink, _ := ctx.Value(sinkKey{}).(*Cost); sink != nil {
		*sink = cost
	}
	if err := res.Settle(cost.USD, cost.Provenance); err != nil {
		return resp, errors.Join(callErr, err)
	}
	return resp, callErr
}

var _ llm.Model = (*Model)(nil)
