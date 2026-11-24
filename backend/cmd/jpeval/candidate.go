package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpcorpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpengine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jprules"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llmbudget"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/logsafe"
)

type candidateInput struct {
	caseValue domain.Case
	label     jpcorpus.Label
}

type candidateRow struct {
	ID          string            `json:"id"`
	Label       jpcorpus.Label    `json:"label"`
	Prediction  domain.Prediction `json:"prediction"`
	CalledModel bool              `json:"called_model"`
	CostUSD     float64           `json:"cost_usd"`
	LatencyMS   int64             `json:"latency_ms"`
	Error       string            `json:"error,omitempty"`
	// CostProvenance says how the call's cost is known. CostUSD stays the
	// gateway-reported amount only; a call with no reported cost is never
	// presented as free: its budget charge is BudgetUSD. Rows from runs
	// before the budget guard have neither field.
	CostProvenance string  `json:"cost_provenance,omitempty"`
	BudgetUSD      float64 `json:"budget_charged_usd,omitempty"`
}

var runTag = regexp.MustCompile(`^[a-z0-9-]{1,42}$`)
var runID = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,38}[a-z0-9])?$`)
var decisionID = regexp.MustCompile(`^D[0-9]+$`)

// candidateOptions are the validated inputs of one paid candidate run.
type candidateOptions struct {
	split, tag, runID string
	workers, limit    int
	prereg, decision  string
	preregRecordSHA   string
	ledger            *llmbudget.Ledger
	rates             llmbudget.Rates
}

func candidate(args []string) error {
	fs := flag.NewFlagSet("candidate", flag.ExitOnError)
	split := fs.String("split", "dev_eval", "dev_eval, held_out, or synthetic")
	tag := fs.String("tag", "", "required for development runs only")
	id := fs.String("run-id", "", "unique id for a new held_out or synthetic run file (lowercase letters, digits, hyphens); omitted, the original fixed file name is used and an existing file stops the run")
	workers := fs.Int("workers", 8, "number of requests in flight, up to 16")
	limit := fs.Int("limit", 0, "development smoke runs only: the first N cases (0 = all)")
	pre := fs.String("prereg", "", "pushed pre-registration commit for the held-out or synthetic run")
	decision := fs.String("decision", "", "numbered decision for the held-out or synthetic run")
	confirm := fs.Bool("confirm-phase2", false, "confirm the single pre-registered held-out or synthetic run")
	ledgerPath := fs.String("budget-ledger", "", "shared budget ledger file for every paid run (outside the repository)")
	capUSD := fs.Float64("budget-usd", llmbudget.MaxCapUSD, "total USD cap across all runs sharing the ledger")
	rin := fs.Float64("rate-in", 0, "verified input USD per million tokens")
	rout := fs.Float64("rate-out", 0, "verified output USD per million tokens")
	rsrc := fs.String("rates-source", "", "the quote or price page the rates were checked against")
	_ = fs.Parse(args)
	if *workers < 1 || *workers > 16 || fs.NArg() != 0 || *limit < 0 {
		return errors.New("use 1 to 16 workers, a non-negative limit and no positional arguments")
	}
	o := candidateOptions{split: *split, tag: *tag, runID: *id, workers: *workers, limit: *limit, prereg: *pre, decision: *decision,
		rates: llmbudget.Rates{InputPerMTok: *rin, OutputPerMTok: *rout, Source: *rsrc}}
	name, err := candidateName(o.split, o.tag, o.runID)
	if err != nil {
		return err
	}
	if o.limit > 0 && o.split != "dev_eval" {
		return errors.New("-limit is for development smoke runs only")
	}
	if o.split == "held_out" || o.split == "synthetic" {
		if !*confirm {
			return fmt.Errorf("the single %s run requires -confirm-phase2", o.split)
		}
		if o.preregRecordSHA, err = verifyPreregistration(o.prereg, o.decision, o.split, o.runID); err != nil {
			return err
		}
	}
	if err := o.rates.Validate(); err != nil {
		return err
	}
	if *ledgerPath == "" {
		return errors.New("every paid run needs -budget-ledger")
	}
	if o.ledger, err = llmbudget.Open(*ledgerPath, *capUSD); err != nil {
		return err
	}
	// Create nothing and call nothing until the output names are free.
	if err := ensureFree(name); err != nil {
		return err
	}
	base, key := os.Getenv("LITELLM_BASE_URL"), os.Getenv("LITELLM_API_KEY")
	if base == "" || key == "" {
		return errors.New("needs LITELLM_BASE_URL and LITELLM_API_KEY")
	}
	gateway := llm.NewLiteLLM(base, key, 120*time.Second)
	gateway.SkipCache(true)
	return runCandidate(o, name, gateway, gateway)
}

// ensureFree refuses a run whose row or metrics file already exists, with a
// message that tells the caller what to do.
func ensureFree(name string) error {
	for _, p := range []string{candidateRowsPath(name), candidateMetricsPath(name)} {
		if _, err := os.Lstat(p); err == nil {
			return fmt.Errorf("%s already exists, and results are never overwritten: choose a new -run-id (or -tag for development)", filepath.Base(p))
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("cannot check %s", filepath.Base(p))
		}
	}
	return nil
}

// runCandidate plans, checks the budget, then runs one request per eligible
// case with no retry. inner is the model that sends requests; sizer measures
// their exact bodies.
func runCandidate(o candidateOptions, name string, inner llm.Model, sizer interface {
	RequestBody(llm.Request) ([]byte, error)
}) error {
	items, pool, labelHash, err := loadCandidate(o.split)
	if err != nil {
		return err
	}
	if o.limit > 0 && o.limit < len(items) {
		items = items[:o.limit]
	}
	rules, err := jprules.Default()
	if err != nil {
		return err
	}
	size := func(r llm.Request) (int, error) {
		b, err := sizer.RequestBody(r)
		return len(b), err
	}
	// Preflight: the worst case of every eligible request must fit what the
	// shared budget has left, or nothing is sent.
	plan, err := planItems(o.split, items, pool, rules)
	if err != nil {
		return err
	}
	worst := plan.worstCase(o.rates)
	left, err := o.ledger.Remaining()
	if err != nil {
		return err
	}
	fmt.Printf("%s: %d eligible requests of %d cases; worst case $%.4f; budget left $%.4f\n", name, plan.Calls, plan.Cases, worst, left)
	if worst > left+1e-9 {
		return fmt.Errorf("%w: the worst case of this run ($%.4f) is above the $%.4f left; nothing was sent", llmbudget.ErrBudget, worst, left)
	}
	gateway := &llmbudget.Model{Inner: inner, Ledger: o.ledger, Rates: o.rates, Size: size}
	eng := jpengine.New(gateway, jpengine.DefaultModel, pool, rules, jpengine.DefaultK)
	path := candidateRowsPath(name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("cannot start %s because its run file cannot be created exclusively", name)
	}
	start := time.Now()
	runErr := writeCandidateRows(eng, items, f, o.workers)
	closeErr := f.Close()
	rows, err := readCandidateRows(path)
	if err != nil {
		return errors.Join(runErr, closeErr, err)
	}
	s := scoreCandidate(name, o.split, rows, len(items), labelHash)
	fmt.Printf("%s: criterion %d/%d (%d had a model response); %d qualified, %d declined, %d undetermined, %d failed; gateway-reported $%.4f (%d calls without a reported cost); budget charged $%.4f\n",
		name, s.Criterion.K, s.Criterion.N, s.ModelCalls-s.Errors, s.Qualifies, s.DoesNotQualify, s.Undetermined, s.Errors, s.CostUSD, s.UnreportedCostCalls, s.BudgetChargedUSD)
	if s.Cases != s.Expected {
		runErr = errors.Join(runErr, fmt.Errorf("run incomplete: %d of %d rows saved", s.Cases, s.Expected))
	}
	committed, _ := o.ledger.Committed()
	logErr := appendRunlog(map[string]any{
		"time": time.Now().Format(time.RFC3339), "run": name, "run_id": o.runID, "split": o.split, "held_out": o.split == "held_out",
		"decision": o.decision, "prereg_commit": o.prereg, "prereg_record_sha256": o.preregRecordSHA, "git_revision": revision(),
		"code_dirty": codeDirty(), "wall_s": int(time.Since(start).Seconds()),
		"llm_provider": "litellm", "model": jpengine.DefaultModel, "prompt_version": jpengine.PromptVersion, "skip_gateway_cache": true,
		"metric": "criterion match; synthetic refusal for synthetic only", "cases": s.Cases, "expected_cases": s.Expected,
		"scored": s.Criterion.N, "correct": s.Criterion.K, "accuracy": s.Criterion.P, "synthetic_refusals": s.SyntheticRefusals,
		"errors": s.Errors, "model_calls": s.ModelCalls, "cost_usd": s.CostUSD, "latency_p50_ms": s.P50MS,
		"latency_p95_ms": s.P95MS, "complete": runErr == nil && closeErr == nil,
		"unreported_cost_calls": s.UnreportedCostCalls, "budget_charged_usd": s.BudgetChargedUSD, "budget_cap_usd": o.ledger.Cap(),
		"budget_committed_after_usd": committed, "rates_input_usd_per_mtok": o.rates.InputPerMTok,
		"rates_output_usd_per_mtok": o.rates.OutputPerMTok, "rates_source": o.rates.Source, "planned_requests": plan.Calls,
		"planned_requests_sha256": plan.RequestsSHA256, "planned_worst_case_usd": worst, "labels_sha256": labelHash,
		"dataset_sha256": plan.Files,
	})
	return errors.Join(runErr, closeErr, logErr, writeCandidateSummary(s, false, true))
}

func candidateName(split, tag, id string) (string, error) {
	name := "jp-" + split + "-" + jpengine.DefaultModel + "-" + jpengine.PromptVersion + "-litellm"
	switch split {
	case "held_out", "synthetic":
		if tag != "" {
			return "", fmt.Errorf("%s uses one run file per run id; omit -tag", split)
		}
		if id == "" {
			return name, nil
		}
		if !runID.MatchString(id) {
			return "", errors.New("-run-id needs lowercase letters, digits and inner hyphens, at most 40 characters")
		}
		return name + "-" + id, nil
	case "dev_eval":
		if id != "" {
			return "", errors.New("development runs are named with -tag")
		}
		if !runTag.MatchString(tag) {
			return "", errors.New("dev_eval requires a tag with lowercase letters, digits or hyphens")
		}
		return name + "-" + tag, nil
	default:
		return "", fmt.Errorf("unknown candidate split %q", split)
	}
}

func candidateRowsPath(name string) string {
	return filepath.Join(outDir, "runs", name+".jsonl")
}

func candidateMetricsPath(name string) string { return filepath.Join(outDir, name+"-metrics.json") }

// verifyPreregistration checks the pushed pre-registration and returns the
// SHA-256 of the decision record it pins.
func verifyPreregistration(commit, decision, split, id string) (string, error) {
	if commit == "" || !decisionID.MatchString(decision) || strings.HasPrefix(commit, "-") {
		return "", errors.New("the fixed run requires a pushed -prereg commit and a numbered -decision")
	}
	git := func(args ...string) ([]byte, error) {
		return exec.Command("git", append([]string{"-C", ".."}, args...)...).Output()
	}
	remote, err := git("for-each-ref", "--contains="+commit, "refs/remotes/origin")
	if err != nil || len(remote) == 0 {
		return "", errors.New("the pre-registration commit must be visible on an origin tracking ref")
	}
	if _, err := git("merge-base", "--is-ancestor", commit, "HEAD"); err != nil {
		return "", errors.New("the pre-registration must belong to the current branch")
	}
	doc, err := git("show", commit+":docs/DECISIONS.md")
	if err != nil {
		return "", errors.New("could not read DECISIONS.md at the pre-registration commit")
	}
	_, record, ok := strings.Cut("\n"+string(doc), "\n## "+decision+" ")
	record, _, _ = strings.Cut(record, "\n## ")
	if !ok || !strings.Contains(record, jpengine.PromptVersion) || !strings.Contains(record, jpengine.DefaultModel) || !strings.Contains(record, "held_out") {
		return "", fmt.Errorf("%s must name the held_out split, candidate %s, and model %s", decision, jpengine.PromptVersion, jpengine.DefaultModel)
	}
	if id != "" && !strings.Contains(record, "`"+id+"`") {
		return "", fmt.Errorf("%s must name the run id `%s` before any scored output exists", decision, id)
	}
	if prior, err := priorRun(decision, split); err != nil || prior != "" {
		return "", fmt.Errorf("%s already has a logged %s run %q (or the run log is unreadable): the protocol allows one run per split and decision", decision, split, prior)
	}
	// Any difference from the pre-registered commit, however small, stops the
	// run. There is no flag to allow drift: a changed engine needs a new
	// pre-registration decision.
	if _, err := git(append([]string{"diff", "--quiet", commit, "HEAD", "--"}, frozenPaths...)...); err != nil {
		return "", errors.New("the candidate code or data changed after pre-registration")
	}
	status, err := git(append([]string{"status", "--porcelain", "--untracked-files=all", "--"}, frozenPaths...)...)
	if err != nil || len(status) > 0 {
		return "", errors.New("the candidate code and frozen data must match the committed files")
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(record))), nil
}

// frozenPaths is everything a pre-registration pins: the candidate, its rules
// and data, the transport and every package compiled into jpeval, the scorer,
// the budget guard and the failure classifier. TestFrozenPathsCoverJpeval
// fails when jpeval starts to depend on a package that is not listed.
var frozenPaths = []string{
	"backend/cmd/jpeval", "backend/internal/jpengine", "backend/internal/jpcorpus", "backend/internal/jprules",
	"backend/internal/rcep", "backend/internal/domain", "backend/internal/engine", "backend/internal/retrieval",
	"backend/internal/llm", "backend/internal/llmbudget", "backend/internal/logsafe", "backend/internal/evalkit",
	"backend/internal/corpus", "backend/internal/blob", "backend/internal/tariffshift", "backend/internal/planner/opcost",
	"backend/go.mod", "backend/go.sum", "eval/jp/labels.py", "eval/jp/synthetic-negatives.jsonl",
	"data/jp/rulings.jsonl", "data/jp/translations-en.jsonl", "data/jp/splits.json",
	"data/jp/labels-dev.jsonl", "data/jp/labels-heldout.jsonl",
}

// priorRun returns the run name of an earlier run log entry for the split
// under decision.
func priorRun(decision, split string) (string, error) {
	f, err := os.Open(runlogPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		var e struct{ Run, Decision, Split string }
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.Decision == decision && e.Split == split && e.Run != "" {
			return e.Run, nil
		}
	}
	return "", sc.Err()
}

// codeDirty reports whether the working tree differs from HEAD.
func codeDirty() bool {
	out, err := exec.Command("git", "status", "--porcelain", "--untracked-files=no").Output()
	return err != nil || len(out) > 0
}

func loadCandidate(split string) ([]candidateInput, *jpcorpus.Pool, string, error) {
	c, err := jpcorpus.Load(dataDir)
	if err != nil {
		return nil, nil, "", err
	}
	sp, err := jpcorpus.LoadSplits(filepath.Join(dataDir, "splits.json"))
	if err != nil {
		return nil, nil, "", err
	}
	poolIDs := sp.Dev
	if split == "held_out" {
		poolIDs = append(poolIDs, sp.HeldOut...)
	}
	pool, err := jpcorpus.NewPool(c, poolIDs)
	if err != nil {
		return nil, nil, "", err
	}
	if split == "synthetic" {
		items, err := syntheticCases()
		return items, pool, "", err
	}
	ids, labelFile := sp.DevEval, "labels-dev.jsonl"
	if split == "held_out" {
		ids, labelFile = sp.HeldOut, "labels-heldout.jsonl"
	}
	labelPath := filepath.Join(dataDir, labelFile)
	labels, err := jpcorpus.LoadLabels(labelPath)
	if err != nil {
		return nil, nil, "", err
	}
	if split == "held_out" && len(labels) != len(ids) {
		return nil, nil, "", fmt.Errorf("held-out labels have %d ids; expected %d", len(labels), len(ids))
	}
	rawLabels, err := os.ReadFile(labelPath)
	if err != nil {
		return nil, nil, "", err
	}
	items := make([]candidateInput, 0, len(ids))
	for _, id := range ids {
		r, ok := c.Ruling(id)
		label, labelled := labels[id]
		if !ok || !labelled || label.Agreement != r.Agreement {
			return nil, nil, "", fmt.Errorf("case %s is missing or its frozen label does not match", id)
		}
		facts, err := c.CaseFacts(id)
		if err != nil {
			return nil, nil, "", err
		}
		d, err := r.Time()
		if err != nil {
			return nil, nil, "", err
		}
		items = append(items, candidateInput{label: label, caseValue: domain.Case{
			ID: jpcorpus.PrecedentID(id), Date: d, Facts: facts,
			Japan: &domain.JapanInput{Agreement: r.Agreement, HS: r.HS},
		}})
	}
	return items, pool, fmt.Sprintf("%x", sha256.Sum256(rawLabels)), nil
}

func writeCandidateRows(e *jpengine.Engine, items []candidateInput, f *os.File, parallel int) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	jobs := make(chan candidateInput, len(items))
	out := make(chan candidateRow, parallel)
	for _, item := range items {
		jobs <- item
	}
	close(jobs)
	var workers sync.WaitGroup
	for range parallel {
		workers.Go(func() {
			for item := range jobs {
				if ctx.Err() != nil {
					return
				}
				row := predictCandidate(ctx, e, item)
				select {
				case out <- row:
				case <-ctx.Done():
					return
				}
			}
		})
	}
	go func() {
		workers.Wait()
		close(out)
	}()
	enc := json.NewEncoder(f)
	for row := range out {
		if err := enc.Encode(row); err != nil {
			cancel()
			workers.Wait()
			return err
		}
	}
	return f.Sync()
}

func predictCandidate(parent context.Context, e *jpengine.Engine, item candidateInput) candidateRow {
	tally := &llm.CostTally{}
	var cost llmbudget.Cost
	ctx, cancel := context.WithTimeout(llmbudget.WithSink(llm.ContextWithCostTally(parent, tally), &cost), 135*time.Second)
	defer cancel()
	start := time.Now()
	res, err := e.Predict(ctx, item.caseValue)
	refused := errors.Is(err, llmbudget.ErrBudget)
	row := candidateRow{ID: item.label.ID, Label: item.label, Prediction: res.Prediction, CalledModel: res.Votes > 0 && !refused,
		LatencyMS: time.Since(start).Milliseconds(), CostUSD: tally.USD(), CostProvenance: cost.Provenance, BudgetUSD: cost.USD}
	if err != nil {
		// Only a closed class and an HTTP status are saved, never provider text.
		code, status := logsafe.Classify(err)
		row.Error = string(code)
		if status > 0 {
			row.Error = fmt.Sprintf("%s (HTTP %d)", code, status)
		}
		if refused {
			row.Error = "budget_cap_refused"
		}
	}
	return row
}

func readCandidateRows(path string) ([]candidateRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	var rows []candidateRow
	seen := map[string]bool{}
	for sc.Scan() {
		var r candidateRow
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, err
		}
		if r.ID == "" || seen[r.ID] || r.ID != r.Label.ID {
			return nil, fmt.Errorf("missing, mismatched or duplicate candidate row id %q", r.ID)
		}
		seen[r.ID] = true
		rows = append(rows, r)
	}
	return rows, sc.Err()
}

func syntheticCases() ([]candidateInput, error) {
	path := filepath.Join(outDir, "synthetic-negatives.jsonl")
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var items []candidateInput
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var row struct {
			ID        string `json:"id"`
			Date      string `json:"date"`
			Agreement string `json:"agreement"`
			HS        string `json:"hs"`
			Facts     string `json:"facts"`
		}
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			return nil, err
		}
		if !strings.HasPrefix(row.ID, "synthetic-") || seen[row.ID] || row.Agreement == "" || row.HS == "" || row.Facts == "" {
			return nil, fmt.Errorf("invalid or repeated synthetic fixture %q", row.ID)
		}
		d, err := time.Parse(time.DateOnly, row.Date)
		if err != nil || !d.Before(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
			return nil, fmt.Errorf("synthetic fixture %s needs a date before the held-out window", row.ID)
		}
		seen[row.ID] = true
		items = append(items, candidateInput{
			label: jpcorpus.Label{ID: row.ID, Agreement: row.Agreement, Outcome: "negative"},
			caseValue: domain.Case{ID: row.ID, Date: d, Facts: row.Facts,
				Japan: &domain.JapanInput{Agreement: row.Agreement, HS: row.HS}},
		})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, errors.New("no registered synthetic negative fixtures")
	}
	return items, nil
}
