// Package evalslice is the regression slice (D72): a fixed 30-case sample of
// the dev split, stratified by the engine's confidence band, and the check
// that a new engine run on it has not dropped below the committed baseline.
// Picking and comparing need no model; only `eval slice` without -in does.
package evalslice

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
)

// Band is a confidence range [Min, Max) and how many slice cases come from it.
type Band struct {
	Label    string  `json:"label"`
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
	Eligible int     `json:"eligible"`
	N        int     `json:"n"`
}

// Bands are evalkit's calibration bins.
func Bands() []Band {
	return []Band{
		{Label: "≥ 0.95", Min: 0.95, Max: 2}, {Label: "0.90–0.95", Min: 0.90, Max: 0.95},
		{Label: "0.80–0.90", Min: 0.80, Max: 0.90}, {Label: "< 0.80", Min: -1, Max: 0.80},
	}
}

// Slice is eval/splits/slice30.json.
type Slice struct {
	Doc      string   `json:"_doc"`
	FrozenAt string   `json:"frozen_at"`
	Source   string   `json:"source"`
	Rule     string   `json:"rule"`
	Bands    []Band   `json:"bands"`
	IDs      []string `json:"slice"`
}

// Pick takes n rulings from a dev result file: rows with an error or a
// scoring adjustment (no single held origin) are skipped, n is shared out
// across the confidence bands in proportion to their size (largest
// remainder, at least one per non-empty band), and within a band rulings are
// taken in FNV-1a order of their ID, which does not depend on correctness.
func Pick(rows []evalkit.Row, adj evalkit.Adjustments, n int) (Slice, error) {
	bands := Bands()
	byBand := make([][]evalkit.Row, len(bands))
	total := 0
	for _, r := range rows {
		if r.Error != "" || r.Split != "dev" {
			continue
		}
		if _, ok := adj.Exclude[r.Ruling]; ok {
			continue
		}
		if _, ok := adj.Alternatives[r.Ruling]; ok {
			continue
		}
		for i, b := range bands {
			if r.Confidence >= b.Min && r.Confidence < b.Max {
				byBand[i] = append(byBand[i], r)
				total++
			}
		}
	}
	if n <= 0 || total < n {
		return Slice{}, fmt.Errorf("evalslice: %d eligible dev rows, want %d", total, n)
	}
	for i := range bands {
		bands[i].Eligible = len(byBand[i])
	}
	alloc := allocate(bands, n)
	s := Slice{Rule: fmt.Sprintf("%d dev rulings without an error or a scoring adjustment, shared across confidence bands in proportion "+
		"(largest remainder, at least 1 per band), first by FNV-1a of the ruling ID within each band", n)}
	for i := range bands {
		rs := byBand[i]
		sort.Slice(rs, func(a, b int) bool {
			ha, hb := hash(rs[a].Ruling), hash(rs[b].Ruling)
			return ha < hb || ha == hb && rs[a].Ruling < rs[b].Ruling
		})
		bands[i].N = alloc[i]
		for _, r := range rs[:alloc[i]] {
			s.IDs = append(s.IDs, r.Ruling)
		}
	}
	s.Bands = bands
	return s, nil
}

func allocate(bands []Band, n int) []int {
	total := 0
	for _, b := range bands {
		total += b.Eligible
	}
	out := make([]int, len(bands))
	rem := make([]float64, len(bands))
	used := 0
	for i, b := range bands {
		exact := float64(n) * float64(b.Eligible) / float64(total)
		out[i] = int(exact)
		if out[i] == 0 && b.Eligible > 0 {
			out[i] = 1
		}
		rem[i] = exact - float64(int(exact))
		used += out[i]
	}
	order := make([]int, len(bands))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return rem[order[a]] > rem[order[b]] })
	for k := 0; used < n; k++ { // hand out the rest by largest remainder
		if i := order[k%len(order)]; out[i] < bands[i].Eligible {
			out[i]++
			used++
		}
	}
	for used > n { // minimums overshot: take back from the largest band
		i := slices.Index(out, slices.Max(out))
		out[i]--
		used--
	}
	return out
}

func hash(id string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(id))
	return h.Sum32()
}

// Case is one baseline answer.
type Case struct {
	Ruling     string  `json:"ruling"`
	Gold       string  `json:"gold"`
	Pred       string  `json:"pred"`
	Correct    bool    `json:"correct"`
	Confidence float64 `json:"confidence"`
}

// Baseline is eval/results/slice30-baseline.json: the committed run the
// slice is compared with.
type Baseline struct {
	Doc           string `json:"_doc"`
	Source        string `json:"source"`
	Provider      string `json:"llm_provider"`
	Model         string `json:"model_id"`
	PromptVersion string `json:"prompt_version"`
	N             int    `json:"n"`
	Correct       int    `json:"correct"`
	// Band95N / Band95K: cases at confidence ≥ 0.95 and how many are right.
	Band95N int    `json:"band95_n"`
	Band95K int    `json:"band95_correct"`
	Cases   []Case `json:"cases"`
}

// BaselineFrom builds the baseline from the slice's rows of a result file.
func BaselineFrom(rows []evalkit.Row, s Slice, source string) (Baseline, error) {
	got := map[string]evalkit.Row{}
	for _, r := range rows {
		got[r.Ruling] = r
	}
	b := Baseline{Source: source}
	var cur []evalkit.Row
	for _, id := range s.IDs {
		r, ok := got[id]
		if !ok || r.Error != "" {
			return Baseline{}, fmt.Errorf("evalslice: %s has no scored row for %s", source, id)
		}
		cur = append(cur, r)
		b.Cases = append(b.Cases, Case{Ruling: r.Ruling, Gold: r.Gold, Pred: r.Pred, Correct: r.Correct, Confidence: r.Confidence})
	}
	sum := evalkit.Summarize(cur)
	b.N, b.Correct, b.Band95N, b.Band95K = sum.N, sum.Correct, sum.AtLeast95.N, sum.AtLeast95.K
	b.Provider, b.Model, b.PromptVersion = strings.Join(sum.Providers, ","), strings.Join(sum.Models, ","), strings.Join(sum.PromptVersions, ",")
	return b, nil
}

// Tolerance is how far a run may fall short of the baseline and still pass.
type Tolerance struct {
	MaxDrop int     // fewer correct cases than the baseline
	BandMin float64 // accuracy at confidence ≥ 0.95
	// MaxErrors is how many cases may fail or be missing before the run says
	// nothing about accuracy (D88). Above it the accuracy check is skipped
	// and the run is inconclusive, unless the answered cases already fail
	// the confidence band.
	MaxErrors int
}

// Check is one pass/fail line of a report.
type Check struct {
	Name   string
	Pass   bool
	Detail string
}

// Report compares a run with the baseline.
type Report struct {
	// Inconclusive: more cases failed than Tolerance.MaxErrors (provider
	// outage), so the accuracy check was not judged (D88).
	Inconclusive       bool
	N, Correct, Errors int
	Band95N, Band95K   int
	Checks             []Check
	// Fixed and Broken are rulings the run gets right / wrong that the
	// baseline got wrong / right.
	Fixed, Broken []string
}

// Passed reports whether every check passed.
func (r Report) Passed() bool {
	for _, c := range r.Checks {
		if !c.Pass {
			return false
		}
	}
	return true
}

// Compare scores the slice's rows of a run against the baseline. A slice
// case missing from the run, or failed with an error, counts as wrong.
func Compare(base Baseline, rows []evalkit.Row, tol Tolerance) Report {
	got := map[string]evalkit.Row{}
	for _, r := range rows {
		got[r.Ruling] = r
	}
	var cur []evalkit.Row
	for _, c := range base.Cases {
		r, ok := got[c.Ruling]
		if !ok {
			r = evalkit.Row{Ruling: c.Ruling, Error: "missing from the run"}
		}
		cur = append(cur, r)
	}
	s := evalkit.Summarize(cur)
	rep := Report{N: s.N, Correct: s.Correct, Errors: s.Errors, Band95N: s.AtLeast95.N, Band95K: s.AtLeast95.K}
	for i, c := range base.Cases {
		now := cur[i].Correct && cur[i].Error == ""
		switch {
		case now && !c.Correct:
			rep.Fixed = append(rep.Fixed, c.Ruling)
		case !now && c.Correct:
			rep.Broken = append(rep.Broken, c.Ruling)
		}
	}
	floor := base.Correct - tol.MaxDrop
	acc := Check{
		Name: "accuracy", Pass: s.Correct >= floor,
		Detail: fmt.Sprintf("%d / %d correct; baseline %d, at least %d to pass (%d-case tolerance)", s.Correct, s.N, base.Correct, floor, tol.MaxDrop),
	}
	if s.Errors > tol.MaxErrors {
		rep.Inconclusive = true
		acc.Pass = true
		acc.Detail = fmt.Sprintf("not judged: %d of %d cases failed or are missing (more than %d), so the model provider, not the code, decided the score; %d correct of the %d answered",
			s.Errors, s.N, tol.MaxErrors, s.Correct, s.N-s.Errors)
	}
	rep.Checks = append(rep.Checks, acc)
	bar := tol.BandMin
	band := Check{Name: "confidence ≥ 0.95"}
	if s.AtLeast95.N == 0 {
		band.Detail = fmt.Sprintf("no case at confidence ≥ 0.95 (baseline %d); the fast path would be empty", base.Band95N)
	} else {
		acc := float64(s.AtLeast95.K) / float64(s.AtLeast95.N)
		band.Pass = acc >= bar-1e-9
		band.Detail = fmt.Sprintf("%d / %d = %.1f%%; baseline %d / %d, at least %.1f%% to pass", s.AtLeast95.K, s.AtLeast95.N, 100*acc,
			base.Band95K, base.Band95N, 100*bar)
	}
	rep.Checks = append(rep.Checks, band)
	return rep
}

// Markdown renders the report.
func (r Report) Markdown(title string, base Baseline) string {
	var b strings.Builder
	verdict := "PASS"
	switch {
	case !r.Passed():
		verdict = "FAIL"
	case r.Inconclusive:
		verdict = "INCONCLUSIVE"
	}
	fmt.Fprintf(&b, "## %s: %s\n\n", title, verdict)
	fmt.Fprintf(&b, "- Baseline: `%s` (%s, %s, %s)\n", base.Source, base.Provider, base.Model, base.PromptVersion)
	if r.Errors > 0 && !r.Inconclusive {
		fmt.Fprintf(&b, "- %d of %d cases failed or are missing; counted as wrong\n", r.Errors, r.N)
	}
	for _, c := range r.Checks {
		mark := "pass"
		if !c.Pass {
			mark = "**FAIL**"
		}
		fmt.Fprintf(&b, "- %s %s: %s\n", mark, c.Name, c.Detail)
	}
	fmt.Fprintf(&b, "- Fixed %d: %s\n- Broken %d: %s\n", len(r.Fixed), list(r.Fixed), len(r.Broken), list(r.Broken))
	return b.String()
}

func list(ids []string) string {
	if len(ids) == 0 {
		return "none"
	}
	return strings.Join(ids, ", ")
}

// LoadSlice reads a slice file.
func LoadSlice(path string) (Slice, error) {
	var s Slice
	if err := readJSON(path, &s); err != nil {
		return Slice{}, err
	}
	if len(s.IDs) == 0 {
		return Slice{}, fmt.Errorf("evalslice: %s lists no rulings", path)
	}
	return s, nil
}

// LoadBaseline reads a baseline file.
func LoadBaseline(path string) (Baseline, error) {
	var b Baseline
	if err := readJSON(path, &b); err != nil {
		return Baseline{}, err
	}
	if len(b.Cases) == 0 || b.N != len(b.Cases) {
		return Baseline{}, fmt.Errorf("evalslice: %s: %d cases, n = %d", path, len(b.Cases), b.N)
	}
	return b, nil
}

// SameRulings checks the baseline covers exactly the slice.
func SameRulings(s Slice, b Baseline) error {
	ids := make([]string, len(b.Cases))
	for i, c := range b.Cases {
		ids[i] = c.Ruling
	}
	if !slices.Equal(slices.Sorted(slices.Values(s.IDs)), slices.Sorted(slices.Values(ids))) {
		return fmt.Errorf("evalslice: the baseline (%d cases) does not cover the same rulings as the slice (%d); rebuild it with eval slice -baseline-from", len(ids), len(s.IDs))
	}
	return nil
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("evalslice: %w", err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("evalslice: %s: %w", path, err)
	}
	return nil
}

// WriteJSON writes v as indented JSON.
func WriteJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
