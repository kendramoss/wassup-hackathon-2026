package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/evalkit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/triage"
)

// calRow is one scored engine prediction joined with its router label.
type calRow struct {
	Ruling     string
	Subfamily  string // "" when the router gave no label
	InScope    bool
	Confidence float64
	Correct    bool // false for a failed call
	Failed     bool
}

// familyFit is the dev-only fit for one subfamily.
type familyFit struct {
	Subfamily string  `json:"subfamily"`
	DevN      int     `json:"dev_n"`              // in-scope dev cases
	DevAuto   int     `json:"dev_auto"`           // dev cases at or above the bar
	DevK      int     `json:"dev_correct"`        // correct among them
	Bar       float64 `json:"bar"`                // fitted bar, or the global bar
	Fitted    bool    `json:"fitted"`             // false: falls back to the global bar
	Note      string  `json:"note"`               // why it fell back
	GlobalBar float64 `json:"global_bar"`         // for the table
	GlobalN   int     `json:"dev_auto_global"`    // dev cases at or above the global bar
	GlobalK   int     `json:"dev_correct_global"` // correct among them
}

// fitFamilyBars fits, on dev rows only, the lowest confidence at which a
// subfamily's cumulative accuracy (confidence ≥ bar, in scope) stays at or
// above target, counting only bars that keep at least minN cases. A
// subfamily with no such bar keeps the global bar.
func fitFamilyBars(dev []calRow, target float64, minN int, global float64) []familyFit {
	by := map[string][]calRow{}
	for _, r := range dev {
		if r.InScope && r.Subfamily != "" {
			by[r.Subfamily] = append(by[r.Subfamily], r)
		}
	}
	fams := append([]string(nil), router.Subfamilies...)
	sort.Strings(fams)
	var out []familyFit
	for _, fam := range fams {
		rows := by[fam]
		f := familyFit{Subfamily: fam, DevN: len(rows), Bar: global, GlobalBar: global}
		f.GlobalN, f.GlobalK = atOrAbove(rows, global)
		var grid []float64
		for _, r := range rows {
			if !r.Failed && !slices.Contains(grid, r.Confidence) {
				grid = append(grid, r.Confidence)
			}
		}
		sort.Float64s(grid)
		// Scan from the top: the bar is the lowest grid point from which
		// every bar above it with ≥ minN cases is also ≥ target.
		best := -1.0
		for i := len(grid) - 1; i >= 0; i-- {
			n, k := atOrAbove(rows, grid[i])
			if n < minN {
				continue
			}
			if float64(k) < target*float64(n) {
				break
			}
			best = grid[i]
		}
		switch {
		case len(rows) < minN:
			f.Note = fmt.Sprintf("fewer than %d in-scope dev cases", minN)
		case best < 0:
			f.Note = fmt.Sprintf("no bar with ≥ %d cases reaches %.0f%%", minN, 100*target)
		default:
			f.Bar, f.Fitted = best, true
		}
		f.DevAuto, f.DevK = atOrAbove(rows, f.Bar)
		out = append(out, f)
	}
	return out
}

// atOrAbove counts successful rows with confidence ≥ bar and how many are correct.
func atOrAbove(rows []calRow, bar float64) (n, k int) {
	for _, r := range rows {
		if !r.Failed && r.Confidence >= bar {
			n++
			if r.Correct {
				k++
			}
		}
	}
	return n, k
}

// policyResult is one auto-accept policy on one split.
type policyResult struct {
	Auto        int     `json:"auto"`
	Correct     int     `json:"correct"`
	Coverage    float64 `json:"coverage"`
	CoverageLo  float64 `json:"coverage_ci_low"`
	CoverageHi  float64 `json:"coverage_ci_high"`
	Accuracy    float64 `json:"accuracy"`
	AccuracyLo  float64 `json:"accuracy_ci_low"`
	AccuracyHi  float64 `json:"accuracy_ci_high"`
	WrongAccept int     `json:"wrong_accepted"`
}

// paired is an exact McNemar test on the cases the two policies treat
// differently.
type paired struct {
	OnlyFamily int     `json:"only_family"`
	OnlyGlobal int     `json:"only_global"`
	P          float64 `json:"p"`
}

// splitResult compares the global bar with the per-family bars on one split.
type splitResult struct {
	Split  string       `json:"split"`
	N      int          `json:"n"`
	Global policyResult `json:"global"`
	Family policyResult `json:"family"`
	// Coverage: cases auto-accepted under one policy only.
	CoverageTest paired `json:"coverage_mcnemar"`
	// WrongAccept: wrong predictions auto-accepted under one policy only.
	WrongAcceptTest paired        `json:"wrong_accept_mcnemar"`
	ByFamily        []familySplit `json:"by_family"`
}

// familySplit is one subfamily's auto counts on a split under both policies.
type familySplit struct {
	Subfamily string  `json:"subfamily"`
	N         int     `json:"n"`
	Bar       float64 `json:"bar"`
	GlobalN   int     `json:"global_auto"`
	GlobalK   int     `json:"global_correct"`
	FamilyN   int     `json:"family_auto"`
	FamilyK   int     `json:"family_correct"`
}

// auto is the server's fast path: in scope, no error, confidence ≥ bar.
func auto(r calRow, bar float64) bool { return r.InScope && !r.Failed && r.Confidence >= bar }

// evaluateBars applies the frozen bars and the global bar to a split.
func evaluateBars(split string, rows []calRow, bars triage.FamilyBars, global float64) splitResult {
	res := splitResult{Split: split, N: len(rows)}
	var g, f policyResult
	fam := map[string]*familySplit{}
	for _, r := range rows {
		bar, _ := bars.Bar(r.Subfamily, global)
		ag, af := auto(r, global), auto(r, bar)
		name := r.Subfamily
		if name == "" {
			name = "(no label)"
		}
		fs := fam[name]
		if fs == nil {
			fs = &familySplit{Subfamily: name, Bar: bar}
			fam[name] = fs
		}
		fs.N++
		if ag {
			g.Auto++
			fs.GlobalN++
			if r.Correct {
				g.Correct++
				fs.GlobalK++
			}
		}
		if af {
			f.Auto++
			fs.FamilyN++
			if r.Correct {
				f.Correct++
				fs.FamilyK++
			}
		}
		switch {
		case af && !ag:
			res.CoverageTest.OnlyFamily++
			if !r.Correct {
				res.WrongAcceptTest.OnlyFamily++
			}
		case ag && !af:
			res.CoverageTest.OnlyGlobal++
			if !r.Correct {
				res.WrongAcceptTest.OnlyGlobal++
			}
		}
	}
	for _, p := range []*policyResult{&g, &f} {
		p.Coverage, p.CoverageLo, p.CoverageHi = evalkit.Wilson(p.Auto, len(rows))
		p.Accuracy, p.AccuracyLo, p.AccuracyHi = evalkit.Wilson(p.Correct, p.Auto)
		p.WrongAccept = p.Auto - p.Correct
	}
	res.Global, res.Family = g, f
	for _, t := range []*paired{&res.CoverageTest, &res.WrongAcceptTest} {
		t.P = mcnemarExact(t.OnlyFamily, t.OnlyGlobal)
	}
	for _, fs := range fam {
		res.ByFamily = append(res.ByFamily, *fs)
	}
	sort.Slice(res.ByFamily, func(i, j int) bool { return res.ByFamily[i].Subfamily < res.ByFamily[j].Subfamily })
	return res
}

// joinScope reads an engine result file (with scoring adjustments) and the
// router label file of the same split.
func joinScope(resultsPath, scopePath string, adj *evalkit.Adjustments) ([]calRow, error) {
	rows, err := readRows(resultsPath, nil)
	if err != nil {
		return nil, err
	}
	if adj != nil {
		rows = adj.Apply(rows)
	}
	labels, err := readRouterLabels(scopePath)
	if err != nil {
		return nil, err
	}
	out := make([]calRow, 0, len(rows))
	for _, r := range rows {
		c := calRow{Ruling: r.Ruling, InScope: true, Confidence: r.Confidence, Correct: r.Correct && r.Error == "", Failed: r.Error != ""}
		if l, ok := labels[r.Ruling]; ok {
			c.Subfamily, c.InScope = l.Subfamily, l.InScope
		}
		out = append(out, c)
	}
	return out, nil
}

type routerLabel struct {
	Subfamily string
	InScope   bool
}

// readRouterLabels reads an `eval scope -out` file: in-scope flag and the
// router's subfamily per ruling (subfamily "" when the router failed).
func readRouterLabels(path string) (map[string]routerLabel, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]routerLabel{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var r struct {
			Ruling  string `json:"ruling"`
			InScope bool   `json:"in_scope"`
			Router  *struct {
				Subfamily string `json:"subfamily"`
			} `json:"router"`
		}
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		l := routerLabel{InScope: r.InScope}
		if r.Router != nil {
			l.Subfamily = r.Router.Subfamily
		}
		out[r.Ruling] = l
	}
	return out, sc.Err()
}

// calibration is calibration-by-family.json.
type calibration struct {
	Command   string             `json:"command"`
	Rule      string             `json:"rule"`
	Target    float64            `json:"target"`
	MinN      int                `json:"min_n"`
	GlobalBar float64            `json:"global_bar"`
	Inputs    map[string]string  `json:"inputs"`
	Bars      map[string]float64 `json:"bars"`
	Families  []familyFit        `json:"families"`
	Splits    []splitResult      `json:"splits"`
}

func calibrateCmd(args []string) error {
	fl := flag.NewFlagSet("calibrate", flag.ExitOnError)
	devIn := fl.String("dev", "../eval/results/dev-gemini-4-argon-prec-engine-v1-vertex.jsonl", "dev engine results (the bars are fitted on these only)")
	finalIn := fl.String("final", "../eval/results/final-gemini-4-argon-prec-engine-v1-vertex.jsonl", "held-out engine results (the frozen bars are applied once; empty to skip)")
	devScope := fl.String("dev-scope", "../eval/results/scope-dev-router-v2-vertex.jsonl", "router labels for dev")
	finalScope := fl.String("final-scope", "../eval/results/scope-final-router-v2-vertex.jsonl", "router labels for final")
	adjustFile := fl.String("adjust", "../eval/data/scoring_adjustments.json", "scoring adjustments (empty for none)")
	target := fl.Float64("target", 0.95, "accuracy each subfamily's bar must keep on dev")
	minN := fl.Int("min-n", 20, "cases a bar must keep on dev, else the subfamily uses the global bar")
	global := fl.Float64("global", config.DefaultFastPath, "global auto-accept bar (D32)")
	outPath := fl.String("out", "../eval/results/calibration-by-family.json", "full result")
	mdPath := fl.String("md", "../eval/results/calibration-by-family.md", "Markdown tables")
	barsPath := fl.String("bars", "internal/triage/family_bars.json", "bars embedded in the server (FAST_PATH_BY_FAMILY=on)")
	_ = fl.Parse(args)
	if *target <= 0 || *target > 1 || *minN < 1 || *global <= 0 || *global > 1 {
		return errors.New("calibrate: need 0 < target ≤ 1, min-n ≥ 1, 0 < global ≤ 1")
	}
	var adj *evalkit.Adjustments
	if *adjustFile != "" {
		a, err := evalkit.LoadAdjustments(*adjustFile)
		if err != nil {
			return err
		}
		adj = &a
	}
	dev, err := joinScope(*devIn, *devScope, adj)
	if err != nil {
		return err
	}
	rule := fmt.Sprintf("dev only: per router subfamily, the lowest confidence c such that, among in-scope dev cases, accuracy at confidence ≥ c' is ≥ %.0f%% for every c' ≥ c that keeps ≥ %d cases (and c itself keeps ≥ %d); else the global %.2f",
		100**target, *minN, *minN, *global)
	fits := fitFamilyBars(dev, *target, *minN, *global)
	bars := triage.FamilyBars{Rule: rule, Source: "eval/results/calibration-by-family.json (eval calibrate, D63)", Bars: map[string]float64{}}
	for _, f := range fits {
		if f.Fitted {
			bars.Bars[f.Subfamily] = f.Bar
		}
	}
	cal := calibration{Command: "eval calibrate", Rule: rule, Target: *target, MinN: *minN, GlobalBar: *global, Bars: bars.Bars, Families: fits,
		Inputs: map[string]string{"dev": rel(*devIn), "dev_scope": rel(*devScope), "adjust": rel(*adjustFile)}}
	cal.Splits = append(cal.Splits, evaluateBars("dev", dev, bars, *global))
	if *finalIn != "" {
		final, err := joinScope(*finalIn, *finalScope, adj)
		if err != nil {
			return err
		}
		cal.Inputs["final"], cal.Inputs["final_scope"] = rel(*finalIn), rel(*finalScope)
		cal.Splits = append(cal.Splits, evaluateBars("final", final, bars, *global))
	}
	for path, v := range map[string]any{*outPath: cal, *barsPath: bars} {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
			return err
		}
	}
	md := calibrationMarkdown(cal)
	if err := os.WriteFile(*mdPath, []byte(md), 0o644); err != nil {
		return err
	}
	fmt.Print(md)
	return nil
}

// rel shortens ../eval/... to eval/... for the record.
func rel(p string) string { return strings.TrimPrefix(filepath.ToSlash(p), "../") }

func calibrationMarkdown(c calibration) string {
	var b strings.Builder
	pct := func(x float64) string { return fmt.Sprintf("%.1f", 100*x) }
	fmt.Fprintf(&b, "### Bars fitted on dev (target %.0f%%, at least %d cases; else global %.2f)\n\n", 100*c.Target, c.MinN, c.GlobalBar)
	b.WriteString("| Subfamily | dev n (in scope) | bar | dev at bar | dev at global bar |\n|---|---:|---:|---|---|\n")
	for _, f := range c.Families {
		bar := fmt.Sprintf("%.2f", f.Bar)
		if !f.Fitted {
			bar = fmt.Sprintf("%.2f (global: %s)", f.Bar, f.Note)
		}
		fmt.Fprintf(&b, "| %s | %d | %s | %s | %s |\n", f.Subfamily, f.DevN, bar, frac(f.DevK, f.DevAuto), frac(f.GlobalK, f.GlobalN))
	}
	for _, s := range c.Splits {
		label := "dev (the bars were fitted here)"
		if s.Split == "final" {
			label = "final (held out; bars frozen from dev, applied once)"
		}
		fmt.Fprintf(&b, "\n### %s, n = %d\n\n", label, s.N)
		b.WriteString("| Policy | auto-accepted (coverage, 95% CI) | accuracy of auto (95% CI) | wrong auto-accepted |\n|---|---|---|---:|\n")
		for _, p := range []struct {
			name string
			r    policyResult
		}{{fmt.Sprintf("global %.2f", c.GlobalBar), s.Global}, {"per-family bars", s.Family}} {
			fmt.Fprintf(&b, "| %s | %d / %d = %s%% (%s–%s) | %d / %d = %s%% (%s–%s) | %d |\n", p.name, p.r.Auto, s.N, pct(p.r.Coverage), pct(p.r.CoverageLo), pct(p.r.CoverageHi),
				p.r.Correct, p.r.Auto, pct(p.r.Accuracy), pct(p.r.AccuracyLo), pct(p.r.AccuracyHi), p.r.WrongAccept)
		}
		fmt.Fprintf(&b, "\nPaired (exact McNemar on the cases the policies treat differently): auto only under family bars %d, only under global %d, p = %.3f; wrong answers auto-accepted only under family bars %d, only under global %d, p = %.3f.\n",
			s.CoverageTest.OnlyFamily, s.CoverageTest.OnlyGlobal, s.CoverageTest.P, s.WrongAcceptTest.OnlyFamily, s.WrongAcceptTest.OnlyGlobal, s.WrongAcceptTest.P)
		b.WriteString("\n| Subfamily | n | bar | global: auto correct / auto | family: auto correct / auto |\n|---|---:|---:|---|---|\n")
		for _, f := range s.ByFamily {
			fmt.Fprintf(&b, "| %s | %d | %.2f | %s | %s |\n", f.Subfamily, f.N, f.Bar, frac(f.GlobalK, f.GlobalN), frac(f.FamilyK, f.FamilyN))
		}
	}
	return b.String()
}

func frac(k, n int) string {
	if n == 0 {
		return "0 / 0"
	}
	return fmt.Sprintf("%d / %d = %.1f%%", k, n, 100*float64(k)/float64(n))
}
