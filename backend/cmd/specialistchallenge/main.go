// Command specialistchallenge scores the four model-backed specialists (textile
// 19 CFR 102.21, pharmaceutical, Part 102 marking usmca-v5 and the Japan
// preference engine) and legal-regime routing on frozen, source-grounded
// synthetic cases. Paid runs go through the approved gateway only.
//
//	go run ./cmd/specialistchallenge validate
//	go run ./cmd/specialistchallenge hashes
//	go run ./cmd/specialistchallenge run -feature all -split dev -tag base -cap 5      (gateway)
//	go run ./cmd/specialistchallenge run -feature all -split test -freeze <sha> -cap 15 (gateway, once)
//	go run ./cmd/specialistchallenge summarize -candidate <sha>
//	go run ./cmd/specialistchallenge check
//
// Labels are written by eval/challenges/specialists/*.py (reference oracles
// that share no code with the candidates) before any model runs.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpengine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "validate":
		err = cmdValidate()
	case "hashes":
		err = cmdHashes()
	case "run":
		err = cmdRun(os.Args[2:])
	case "route":
		err = cmdRoute(os.Args[2:])
	case "summarize":
		err = cmdSummarize(os.Args[2:])
	case "check":
		err = cmdCheck()
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "specialistchallenge:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: specialistchallenge validate | hashes | run -feature F -split dev|test [-tag t] [-freeze sha] [-cap usd] | route -split dev|test | summarize -candidate sha | check")
	os.Exit(2)
}

func repoRoot() string {
	if wd, err := os.Getwd(); err == nil {
		for d := wd; ; d = filepath.Dir(d) {
			if _, err := os.Stat(filepath.Join(d, "eval", "challenges", "specialists")); err == nil {
				return d
			}
			if filepath.Dir(d) == d {
				break
			}
		}
	}
	return "."
}

func dataRoot() string { return filepath.Join(repoRoot(), "eval", "challenges", "specialists") }

func git(args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repoRoot()}, args...)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return out.String(), nil
}

func cmdValidate() error {
	dev, test, err := LoadAll(dataRoot())
	if err != nil {
		return err
	}
	probs := CheckIntegrity(dev, test, IntegrityOptions{MinTest: 100})
	for _, c := range append(append([]Case{}, dev...), test...) {
		for _, l := range Leaks(c) {
			probs = append(probs, fmt.Sprintf("%s: %s", c.ID, l))
		}
	}
	for _, f := range Features {
		d, t := byFeature(dev, f.ID), byFeature(test, f.ID)
		fam, shared, tmpl, tshared := familyStats(d, t)
		u := 0
		for _, c := range t {
			if c.Expected.Kind == "uncertain" {
				u++
			}
		}
		fmt.Printf("%-20s dev %3d  test %3d (expected-uncertain %2d)  test source families %2d (shared with dev %2d)  test templates %2d (shared %2d)\n",
			f.ID, len(d), len(t), u, fam, shared, tmpl, tshared)
	}
	rdev, rtest, err := LoadRouting(dataRoot())
	if err != nil {
		return err
	}
	probs = append(probs, CheckIntegrity(rdev, rtest, IntegrityOptions{MinTest: 100})...)
	neg := 0
	for _, c := range rtest {
		if c.ScenarioFamily == "negative-control" {
			neg++
		}
	}
	fmt.Printf("%-20s dev %3d  test %3d (appropriate specialist cases %d, general-origin negative controls %d)\n", RoutingFeature, len(rdev), len(rtest), len(rtest)-neg, neg)
	if len(probs) > 0 {
		return fmt.Errorf("%d problems:\n%s", len(probs), strings.Join(probs, "\n"))
	}
	fmt.Println("datasets are well formed: opaque ids, disjoint normalised inputs, no label or metadata in any model-visible input")
	return nil
}

func familyStats(dev, test []Case) (fam, shared, tmpl, tshared int) {
	df, dt := map[string]bool{}, map[string]bool{}
	for _, c := range dev {
		df[c.SourceFamily], dt[c.Template] = true, true
	}
	tf, tt := map[string]bool{}, map[string]bool{}
	for _, c := range test {
		tf[c.SourceFamily], tt[c.Template] = true, true
	}
	for k := range tf {
		if df[k] {
			shared++
		}
	}
	for k := range tt {
		if dt[k] {
			tshared++
		}
	}
	return len(tf), shared, len(tt), tshared
}

func cmdHashes() error {
	for _, f := range Features {
		for _, s := range []string{"dev", "test"} {
			h, err := fileSHA(filepath.Join(dataRoot(), f.File+"-"+s+".jsonl"))
			if err != nil {
				return err
			}
			fmt.Printf("%s-%s.jsonl %s\n", f.File, s, h)
		}
	}
	return nil
}

// loadPools reads the precedent pools the server builds (production corpus).
func loadPools() (Pools, map[string]string, error) {
	dir := filepath.Join(repoRoot(), "backend", "data")
	labels, err := corpus.LoadLabels(filepath.Join(dir, "labels.jsonl"))
	if err != nil {
		return Pools{}, nil, err
	}
	pool, err := corpus.Load(labels, filepath.Join(dir, "facts"))
	if err != nil {
		return Pools{}, nil, err
	}
	pick := func(file string) ([]domain.Ruling, error) {
		ids, err := corpus.LoadIDList(filepath.Join(dir, file))
		if err != nil {
			return nil, err
		}
		var rs []domain.Ruling
		for _, id := range ids {
			if r, ok := pool.Get(id); ok {
				rs = append(rs, r)
			}
		}
		return rs, nil
	}
	var p Pools
	if p.Textile, err = pick("textile_rulings.json"); err != nil {
		return p, nil, err
	}
	reasons, err := corpus.LoadReasoning(filepath.Join(dir, "textile_reasoning.json"))
	if err != nil {
		return p, nil, err
	}
	p.TextileV2 = corpus.WithReasoning(p.Textile, reasons)
	if p.Pharma, err = pick("pharma_rulings.json"); err != nil {
		return p, nil, err
	}
	ul, err := corpus.LoadLabels(filepath.Join(dir, "labels_usmca.jsonl"))
	if err != nil {
		return p, nil, err
	}
	up, err := corpus.Load(ul, filepath.Join(dir, "facts"))
	if err != nil {
		return p, nil, err
	}
	p.Part102 = up.Rulings()
	prov := map[string]string{
		"textile_pool_revision": corpus.RevisionOf(p.Textile), "pharma_pool_revision": corpus.RevisionOf(p.Pharma), "part102_pool_revision": corpus.RevisionOf(p.Part102),
		"textile_pool_rulings": fmt.Sprint(len(p.Textile)), "pharma_pool_rulings": fmt.Sprint(len(p.Pharma)), "part102_pool_rulings": fmt.Sprint(len(p.Part102)),
	}
	for _, f := range []string{"labels.jsonl", "labels_usmca.jsonl", "textile_rulings.json", "pharma_rulings.json", "textile_reasoning.json"} {
		if prov[f], err = fileSHA(filepath.Join(dir, f)); err != nil {
			return p, nil, err
		}
	}
	return p, prov, nil
}

func newGateway() (llm.Model, error) {
	base, key := os.Getenv("LITELLM_BASE_URL"), os.Getenv("LITELLM_API_KEY")
	if base == "" || key == "" {
		return nil, errors.New("needs LITELLM_BASE_URL and LITELLM_API_KEY (the approved gateway; Vertex is never used)")
	}
	g := llm.NewLiteLLM(base, key, 120*time.Second)
	g.SkipCache(true)
	return g, nil
}

func writeRowsFile(path string, rows []Row) error {
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, r := range rows {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func readRows(path string) ([]Row, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows []Row
	for _, line := range bytes.Split(bytes.TrimSpace(b), []byte("\n")) {
		var r Row
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&r); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		rows = append(rows, r)
	}
	return rows, nil
}

// attempt is one line of attempts.jsonl: every scored run, kept.
type attempt struct {
	Time    string  `json:"time"`
	Tag     string  `json:"tag"`
	Feature string  `json:"feature"`
	Split   string  `json:"split"`
	Variant string  `json:"variant"`
	File    string  `json:"file"`
	Head    string  `json:"head"`
	Dirty   bool    `json:"dirty"`
	Cases   int     `json:"cases"`
	Matched int     `json:"matched"`
	Errors  int     `json:"errors"`
	Cost    float64 `json:"cost_usd"`
	Calls   int     `json:"model_calls"`
	Note    string  `json:"note,omitempty"`
}

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	feature := fs.String("feature", "all", "feature id or all")
	split := fs.String("split", "", "dev or test")
	tag := fs.String("tag", "", "run name for dev results (required for dev)")
	freeze := fs.String("freeze", "", "full sha of the frozen commit (required for test)")
	capUSD := fs.Float64("cap", 0, "stop sending after this much reported spend (required)")
	workers := fs.Int("workers", 6, "parallel cases")
	variant := fs.String("variant", "", "dev-selected candidate variant")
	limit := fs.Int("limit", 0, "development only: first N cases per feature")
	japanComparator := fs.Bool("japan-comparator", false, "score the unmodified Japan engine once on the original reserved cases (D971 amendment 4)")
	comparator := fs.Bool("comparator", false, "run the live-textile comparator (D971 amendment 2): textile only, deployed prompt")
	_ = fs.Parse(args)
	if *split != "dev" && *split != "test" {
		return errors.New("-split must be dev or test")
	}
	if *capUSD <= 0 {
		return errors.New("-cap is required: a paid run states its spend limit")
	}
	if *split == "dev" && *tag == "" {
		return errors.New("-tag names a development run")
	}
	if *split == "test" && !*japanComparator {
		if *limit != 0 || *variant != "" {
			return errors.New("a reserved run takes no -limit or -variant: the candidate is frozen")
		}
		*variant = "frozen"
	}
	if *japanComparator {
		if *comparator || *split != "test" || *variant != "" || *limit != 0 {
			return errors.New("the Japan comparator is a single reserved-split run of the unmodified engine: no -comparator, -variant or -limit")
		}
		*feature = "japan_engine"
	}
	if *comparator {
		if *feature != "all" && *feature != "textile_specialist" {
			return errors.New("the comparator is textile only")
		}
		*feature = "textile_specialist"
		if *split == "test" {
			if frozenLive != LiveBase && frozenLive != LiveCandidate {
				return errors.New("the comparator's development-selected variant is not frozen in the code yet")
			}
			*variant = frozenLive
		} else if !strings.HasPrefix(*tag, "live") || (*variant != LiveBase && *variant != LiveCandidate) {
			return errors.New("comparator development runs need a tag starting with live and -variant live-v2 or live-v2-missing")
		}
	} else if *variant == LiveBase || *variant == LiveCandidate {
		return errors.New("live variants run only with -comparator")
	}
	dev, test, err := LoadAll(dataRoot())
	if err != nil {
		return err
	}
	if p := CheckIntegrity(dev, test, IntegrityOptions{MinTest: 100}); len(p) > 0 {
		return fmt.Errorf("datasets are not well formed:\n%s", strings.Join(p, "\n"))
	}
	feats := Features
	if *feature != "all" {
		feats = nil
		for _, f := range Features {
			if f.ID == *feature {
				feats = []Feature{f}
			}
		}
		if feats == nil {
			return fmt.Errorf("unknown feature %q", *feature)
		}
	}
	cases := dev
	if *split == "test" {
		cases = test
	}
	resDir := filepath.Join(dataRoot(), "results")
	if err := os.MkdirAll(resDir, 0o755); err != nil {
		return err
	}
	name := func(f Feature) string {
		if *split == "test" {
			if *comparator {
				return filepath.Join(resDir, comparatorTestFile)
			}
			if *japanComparator {
				return filepath.Join(resDir, japanComparatorTestFile)
			}
			return filepath.Join(resDir, f.File+"-test.jsonl")
		}
		return filepath.Join(resDir, f.File+"-dev-"+*tag+".jsonl")
	}
	for _, f := range feats {
		if *split == "test" {
			if err := FrozenGuard(git, *freeze, name(f)); err != nil {
				return err
			}
		} else if _, err := os.Stat(name(f)); err == nil {
			return fmt.Errorf("%s exists: development runs are never overwritten, choose a new -tag", filepath.Base(name(f)))
		}
	}
	model, err := newGateway()
	if err != nil {
		return err
	}
	pools, prov, err := loadPools()
	if err != nil {
		return err
	}
	cand, err := NewCandidate(model, jpengine.DefaultModel, pools)
	if err != nil {
		return err
	}
	if err := cand.SetVariant(*variant); err != nil {
		return err
	}
	head, _ := git("rev-parse", "HEAD")
	dirty, _ := git("status", "--porcelain", "--", "backend", "eval/challenges/specialists")
	provJSON, _ := json.Marshal(prov)
	fmt.Printf("corpus: %s\n", provJSON)
	var spent float64
	var spentMu sync.Mutex
	for _, f := range feats {
		fc := byFeature(cases, f.ID)
		if *limit > 0 && *limit < len(fc) {
			fc = fc[:*limit]
		}
		rows := make([]Row, 0, len(fc))
		var mu sync.Mutex
		sem := make(chan struct{}, *workers)
		var wg sync.WaitGroup
		for _, c := range fc {
			wg.Add(1)
			sem <- struct{}{}
			go func(c Case) {
				defer wg.Done()
				defer func() { <-sem }()
				spentMu.Lock()
				over := spent >= *capUSD
				spentMu.Unlock()
				var r Row
				if over {
					r = Grade(c, Row{ID: c.ID, Feature: c.Feature, Split: c.Split, Error: "budget_cap_refused"})
				} else {
					ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
					r = RunCase(ctx, cand, c)
					cancel()
					spentMu.Lock()
					spent += r.CostUSD
					spentMu.Unlock()
				}
				mu.Lock()
				rows = append(rows, r)
				appendPartial(name(f)+".partial", r)
				mu.Unlock()
			}(c)
		}
		wg.Wait()
		if err := Gate(rows); err != nil {
			return fmt.Errorf("%s: %w (rows were not written)", f.ID, err)
		}
		if err := writeRowsFile(name(f), rows); err != nil {
			return err
		}
		_ = os.Remove(name(f) + ".partial")
		s, err := SummarizeSplit(fc, rows)
		if err != nil {
			return err
		}
		var cost float64
		var lat []int64
		for _, r := range rows {
			cost += r.CostUSD
			if r.CalledModel {
				lat = append(lat, r.LatencyMS)
			}
		}
		fmt.Printf("%-20s %s %s: %d cases, %d matched, %d mismatched, %d errors; model calls %d; cost $%.4f; latency p50 %d ms p95 %d ms\n",
			f.ID, *split, *tag, s.Cases, s.Matched, s.Mismatched, s.Errors, s.ModelCalls, cost, Percentile(lat, .5), Percentile(lat, .95))
		attemptFeature := f.ID
		if *comparator {
			attemptFeature = ComparatorFeature
		}
		if *japanComparator {
			attemptFeature = JapanComparatorFeature
		}
		if err := appendAttempt(attempt{Time: time.Now().UTC().Format(time.RFC3339), Tag: *tag, Feature: attemptFeature, Split: *split, Variant: *variant,
			File: filepath.Base(name(f)), Head: strings.TrimSpace(head), Dirty: strings.TrimSpace(dirty) != "", Cases: s.Cases, Matched: s.Matched,
			Errors: s.Errors, Cost: round6(cost), Calls: s.ModelCalls}); err != nil {
			return err
		}
	}
	fmt.Printf("total reported cost this invocation: $%.4f\n", spent)
	return nil
}

// appendPartial keeps every finished row on disk as it completes, so a run
// that is killed still leaves its rows (and their cost) on record.
func appendPartial(path string, r Row) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_ = json.NewEncoder(f).Encode(r)
}

func appendAttempt(a attempt) error {
	f, err := os.OpenFile(filepath.Join(dataRoot(), "attempts.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(a)
}
