// Package evaldrift decides whether a month's re-scoring (D931) shows that
// accuracy has fallen compared with the months before it (#921, D948).
//
// The rule is a one-sided Fisher exact test of the month's correct and
// incorrect counts against all earlier figures pooled, at alpha 0.01, for a
// month of at least 20 cases and a baseline of at least 30. A month that also
// lies wholly below the pooled earlier 95% Wilson interval is marked
// "below_interval". Everything is computed from the history file, so an alert
// can be recomputed offline with no model and no network.
package evaldrift

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
)

// Kinds of point in a history.
const (
	KindBaseline = "baseline" // a held-out figure from before monthly re-scoring
	KindMonthly  = "monthly"  // one month's re-score, id YYYY-MM
)

// Statuses of a month's result.
const (
	StatusOK         = "ok"
	StatusAlert      = "alert"
	StatusTooFew     = "too_few_cases"
	StatusNoBaseline = "no_baseline"
)

// Severities of an alert.
const (
	SeverityDrop          = "drop"           // the test fires
	SeverityBelowInterval = "below_interval" // and the month's interval is wholly below the pooled one
)

// Point is one measured accuracy: Correct of Cases.
type Point struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Correct int    `json:"correct"`
	Cases   int    `json:"cases"`
	Source  string `json:"source,omitempty"`
	Run     string `json:"run,omitempty"`
}

// Rule holds the thresholds.
type Rule struct {
	Alpha       float64 `json:"alpha"`
	MinCases    int     `json:"min_cases"`
	MinBaseline int     `json:"min_baseline"`
}

// DefaultRule is the rule of D948.
var DefaultRule = Rule{Alpha: 0.01, MinCases: 20, MinBaseline: 30}

// Figure is a count with its accuracy and 95% Wilson interval.
type Figure struct {
	Correct  int        `json:"correct"`
	Cases    int        `json:"cases"`
	Accuracy float64    `json:"accuracy"`
	CI95     [2]float64 `json:"ci95"`
}

// Result is the verdict on one month of one series.
type Result struct {
	Series    string  `json:"series"`
	Month     string  `json:"month"`
	Status    string  `json:"status"`
	Alert     bool    `json:"alert"`
	Severity  string  `json:"severity,omitempty"`
	PValue    float64 `json:"p_value"`
	ThisMonth Figure  `json:"this_month"`
	Baseline  Figure  `json:"baseline"`
}

// History is the persisted monthly history, by series ("cbp", "japan").
type History struct {
	Rule   Rule               `json:"rule"`
	Series map[string][]Point `json:"series"`
}

var monthID = regexp.MustCompile(`^20[0-9]{2}-(0[1-9]|1[0-2])$`)

func figure(k, n int) Figure {
	f := Figure{Correct: k, Cases: n}
	if n == 0 {
		return f
	}
	f.Accuracy = round4(float64(k) / float64(n))
	lo, hi := wilson(k, n)
	f.CI95 = [2]float64{round4(lo), round4(hi)}
	return f
}

func round4(x float64) float64 { return math.Round(x*1e4) / 1e4 }

func wilson(k, n int) (float64, float64) {
	const z = 1.96
	p, nn := float64(k)/float64(n), float64(n)
	d := 1 + z*z/nn
	c := (p + z*z/(2*nn)) / d
	h := z * math.Sqrt(p*(1-p)/nn+z*z/(4*nn*nn)) / d
	return c - h, c + h
}

func lchoose(n, k int) float64 {
	a, _ := math.Lgamma(float64(n + 1))
	b, _ := math.Lgamma(float64(k + 1))
	c, _ := math.Lgamma(float64(n - k + 1))
	return a - b - c
}

// fisherLower is P(X <= k) for X hypergeometric: the chance of at most k
// correct among n new cases when the new and the b-correct-of-m earlier cases
// are one population (one-sided Fisher exact test, lower tail).
func fisherLower(k, n, b, m int) float64 {
	N, K := n+m, k+b
	den := lchoose(N, n)
	var p float64
	for x := 0; x <= k; x++ {
		if x > K || n-x > N-K {
			continue
		}
		p += math.Exp(lchoose(K, x) + lchoose(N-K, n-x) - den)
	}
	return math.Min(1, p)
}

// Evaluate judges every monthly point of one series against the points that
// come before it in calendar order (baselines first), pooled.
func Evaluate(points []Point, r Rule) []Result {
	var base, months []Point
	for _, p := range points {
		if p.Kind == KindMonthly {
			months = append(months, p)
		} else {
			base = append(base, p)
		}
	}
	sort.SliceStable(months, func(i, j int) bool { return months[i].ID < months[j].ID })
	pool := Point{}
	for _, p := range base {
		pool.Correct += p.Correct
		pool.Cases += p.Cases
	}
	var out []Result
	for _, m := range months {
		res := Result{Month: m.ID, ThisMonth: figure(m.Correct, m.Cases), Baseline: figure(pool.Correct, pool.Cases), PValue: 1}
		switch {
		case pool.Cases < r.MinBaseline:
			res.Status = StatusNoBaseline
		case m.Cases < r.MinCases:
			res.Status = StatusTooFew
		default:
			res.PValue = fisherLower(m.Correct, m.Cases, pool.Correct, pool.Cases)
			res.Status = StatusOK
			if res.PValue <= r.Alpha {
				res.Status, res.Alert, res.Severity = StatusAlert, true, SeverityDrop
				if res.ThisMonth.CI95[1] < res.Baseline.CI95[0] {
					res.Severity = SeverityBelowInterval
				}
			}
		}
		res.PValue = math.Round(res.PValue*1e6) / 1e6
		out = append(out, res)
		pool.Correct += m.Correct
		pool.Cases += m.Cases
	}
	return out
}

// Report is every month of every series, as stored in drift.json.
type Report struct {
	Rule    Rule     `json:"rule"`
	Results []Result `json:"results"`
}

// Compute evaluates every series of h, series in name order.
func Compute(h *History, r Rule) Report {
	names := make([]string, 0, len(h.Series))
	for n := range h.Series {
		names = append(names, n)
	}
	sort.Strings(names)
	rep := Report{Rule: r, Results: []Result{}}
	for _, n := range names {
		for _, res := range Evaluate(h.Series[n], r) {
			res.Series = n
			rep.Results = append(rep.Results, res)
		}
	}
	return rep
}

// Alerts returns the results that alert.
func (r Report) Alerts() []Result {
	var out []Result
	for _, x := range r.Results {
		if x.Alert {
			out = append(out, x)
		}
	}
	return out
}

// Line is the one-line summary of a result, for the re-score summary.
func (r Result) Line() string {
	if r.Alert {
		return fmt.Sprintf("DRIFT %s %s: %d/%d (%.1f%%) against %.1f%% earlier (%d cases), one-sided p=%.4g, %s",
			r.Series, r.Month, r.ThisMonth.Correct, r.ThisMonth.Cases, 100*r.ThisMonth.Accuracy, 100*r.Baseline.Accuracy, r.Baseline.Cases, r.PValue, r.Severity)
	}
	return fmt.Sprintf("ok %s %s: %d/%d (%.1f%%) against %.1f%% earlier (%d cases), %s, p=%.4g",
		r.Series, r.Month, r.ThisMonth.Correct, r.ThisMonth.Cases, 100*r.ThisMonth.Accuracy, 100*r.Baseline.Accuracy, r.Baseline.Cases, r.Status, r.PValue)
}

// Record adds a point to a series. Recording the same month with the same
// figures again changes nothing (added false); a different figure for a month
// already recorded is an error: a month is scored once.
func (h *History) Record(series string, p Point) (added bool, err error) {
	if p.Kind != KindMonthly || !monthID.MatchString(p.ID) {
		return false, fmt.Errorf("evaldrift: %q is not a month (YYYY-MM)", p.ID)
	}
	if p.Cases <= 0 || p.Correct < 0 || p.Correct > p.Cases {
		return false, fmt.Errorf("evaldrift: %s %s: %d correct of %d cases", series, p.ID, p.Correct, p.Cases)
	}
	if h.Series == nil {
		h.Series = map[string][]Point{}
	}
	for _, q := range h.Series[series] {
		if q.ID == p.ID {
			if q.Correct == p.Correct && q.Cases == p.Cases {
				return false, nil
			}
			return false, fmt.Errorf("evaldrift: %s %s is already recorded as %d of %d; a month is scored once", series, p.ID, q.Correct, q.Cases)
		}
	}
	h.Series[series] = append(h.Series[series], p)
	return true, nil
}

// Load reads a history file.
func Load(path string) (*History, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var h History
	if err := json.Unmarshal(b, &h); err != nil {
		return nil, fmt.Errorf("evaldrift: %s: %w", path, err)
	}
	if h.Series == nil {
		return nil, errors.New("evaldrift: no series in " + path)
	}
	return &h, nil
}

// Save writes the history file.
func (h *History) Save(path string) error { return writeJSON(path, h) }

// SaveReport writes the report file.
func SaveReport(path string, r Report) error { return writeJSON(path, r) }

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
