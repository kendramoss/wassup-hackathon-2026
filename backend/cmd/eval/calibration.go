package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalsummary"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/verifier"
)

// calibrationFile matches a verifier-v2 flip result file (D86): the split,
// the run variant (empty for the first run), the mode and the provider.
// verifier-v1 (flips-*-verifier-<mode>) and v3 files do not match.
var calibrationFile = regexp.MustCompile(`^flips-(dev|final)-verifier-v2(?:-(.+?))?-(known-a|unknown-a)-([a-z]+)\.jsonl$`)

// calibrationBands are the confidence bands on the Accuracy page. The top
// band includes 1.0; the 0.92 edge is the plan bar.
var calibrationBands = []struct {
	label    string
	min, max float64
}{
	{"≥ 0.95", 0.95, 1.01},
	{"0.92–0.95", 0.92, 0.95},
	{"0.85–0.92", 0.85, 0.92},
	{"< 0.85", 0, 0.85},
}

const (
	calibrationNoteProxy = "Scored through the LiteLLM proxy with gemini-4-argon, the same model production calls on Vertex AI (D74). The heat-gun pair, the demo product, is excluded from every run."
	calibrationCaveat    = "Not a clean held-out test: the burden-of-proof sentence verifier-v2 keeps from v1 was written after the research team saw failures on these 2026 pairs (D74). A clean test needs CBP rulings published after 25 Sep 2026 (D82)."
)

type calibrationRow struct {
	ID            string  `json:"id"`
	OriginChanged bool    `json:"origin_changed"`
	Changes       bool    `json:"pred_changes_origin"`
	Supported     bool    `json:"precedent_supported"`
	Confidence    float64 `json:"confidence"`
	Error         string  `json:"error"`
}

// supported reports whether a verdict backs the move with a precedent: it
// says origin changes and cites a supporting ruling.
func (r calibrationRow) supported() bool { return r.Changes && r.Supported }

// verifierCalibration builds the precedent check's calibration from every
// verifier-v2 flip result file in dir, pooled by split (D86). bar is the
// plan bar. It returns nil when there are no such files.
func verifierCalibration(dir string, bar float64) (*evalsummary.VerifierCalibration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	type split struct {
		out  evalsummary.VerifierSplit
		rows map[string][]calibrationRow // by pair
	}
	splits := map[string]*split{}
	providers := map[string]bool{}
	for _, e := range entries {
		m := calibrationFile.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		name, variant, mode, provider := m[1], m[2], m[3], m[4]
		if variant == "" {
			variant = "base"
		}
		providers[provider] = true
		s := splits[name]
		if s == nil {
			s = &split{rows: map[string][]calibrationRow{}}
			splits[name] = s
		}
		rows, err := readCalibrationRows(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		run := evalsummary.VerifierRun{Source: "eval/results/" + e.Name(), Mode: mode, Variant: variant}
		for _, r := range rows {
			if r.Error != "" {
				run.Errors++
				continue
			}
			run.Rows++
			s.rows[r.ID] = append(s.rows[r.ID], r)
		}
		s.out.Runs = append(s.out.Runs, run)
		s.out.Rows += run.Rows
		s.out.Errors += run.Errors
	}
	if len(splits) == 0 {
		return nil, nil
	}
	note := calibrationNoteProxy
	if !(len(providers) == 1 && providers["litellm"]) {
		note = "Scored with gemini-4-argon (providers: " + strings.Join(sortedKeys(providers), ", ") + "). The heat-gun pair, the demo product, is excluded from every run."
	}
	cal := &evalsummary.VerifierCalibration{Bar: bar, Prompt: verifier.PromptVersion2, Note: note}
	for _, name := range []string{"dev", "final"} {
		s := splits[name]
		if s == nil {
			continue
		}
		out := s.out
		out.Split = name
		if name == "dev" {
			out.Label = "Development flip pairs (CBP rulings before 2026)"
			out.DevelopmentOnly = true
		} else {
			out.Label = "2026 flip pairs (held out from development)"
			out.Caveat = calibrationCaveat
		}
		sort.Slice(out.Runs, func(i, j int) bool { return out.Runs[i].Source < out.Runs[j].Source })
		out.Pairs = len(s.rows)
		out.Bands, out.Shown, out.PairLevel = scoreCalibration(s.rows, bar)
		cal.Splits = append(cal.Splits, out)
	}
	return cal, nil
}

// scoreCalibration pools the rows of one split by run and by pair.
func scoreCalibration(byPair map[string][]calibrationRow, bar float64) ([]evalsummary.VerifierBand, evalsummary.VerifierShown, evalsummary.VerifierPairs) {
	bands := make([]evalsummary.VerifierBand, len(calibrationBands))
	var real, wrong, realRows int
	var pairs evalsummary.VerifierPairs
	for _, rows := range byPair {
		pairs.Pairs++
		changed := rows[0].OriginChanged
		shown := 0
		for _, r := range rows {
			if r.OriginChanged {
				realRows++
			}
			if r.supported() {
				for i, b := range calibrationBands {
					if r.Confidence >= b.min && r.Confidence < b.max {
						bands[i].Correct.N++
						if r.OriginChanged {
							bands[i].Correct.K++
						}
						break
					}
				}
			}
			if r.supported() && r.Confidence >= bar {
				shown++
				if r.OriginChanged {
					real++
				} else {
					wrong++
				}
			}
		}
		switch {
		case changed:
			pairs.RealChangePairs++
			if shown == len(rows) {
				pairs.ShownEveryRun++
			} else if shown > 0 {
				pairs.ShownSomeRuns++
			}
		case shown > 0:
			pairs.WrongAnyRun++
		}
	}
	for i, b := range calibrationBands {
		bands[i].Label, bands[i].Min, bands[i].Max = b.label, b.min, min(b.max, 1)
		bands[i].Correct = rate(bands[i].Correct.K, bands[i].Correct.N)
	}
	shown := evalsummary.VerifierShown{Real: real, Wrong: wrong, Precision: rate(real, real+wrong), Recall: rate(real, realRows)}
	pairs.Precision = rate(pairs.ShownEveryRun, pairs.ShownEveryRun+pairs.WrongAnyRun)
	pairs.Recall = rate(pairs.ShownEveryRun, pairs.RealChangePairs)
	return bands, shown, pairs
}

// rate is k of n with its Wilson interval, rounded to 4 places.
func rate(k, n int) evalsummary.Rate {
	r4 := func(x float64) float64 { return float64(int(x*10000+0.5)) / 10000 }
	p, lo, hi := evalkit.Wilson(k, n)
	return evalsummary.Rate{K: k, N: n, Value: r4(p), CILow: r4(lo), CIHigh: r4(hi)}
}

func readCalibrationRows(path string) ([]calibrationRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var rows []calibrationRow
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var r calibrationRow
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		rows = append(rows, r)
	}
	return rows, sc.Err()
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
