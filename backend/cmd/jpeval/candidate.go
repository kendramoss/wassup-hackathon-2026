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
}

var runTag = regexp.MustCompile(`^[a-z0-9-]{1,42}$`)
var decisionID = regexp.MustCompile(`^D[0-9]+$`)

func candidate(args []string) error {
	fs := flag.NewFlagSet("candidate", flag.ExitOnError)
	split := fs.String("split", "dev_eval", "dev_eval, held_out, or synthetic")
	tag := fs.String("tag", "", "required for development runs only")
	workers := fs.Int("workers", 8, "number of requests in flight, up to 16")
	pre := fs.String("prereg", "", "pushed pre-registration commit for the held-out or synthetic run")
	decision := fs.String("decision", "", "numbered decision for the held-out or synthetic run")
	confirm := fs.Bool("confirm-phase2", false, "confirm the single pre-registered held-out or synthetic run")
	_ = fs.Parse(args)
	if *workers < 1 || *workers > 16 || fs.NArg() != 0 {
		return errors.New("use 1 to 16 workers and no positional arguments")
	}
	name, err := candidateName(*split, *tag)
	if err != nil {
		return err
	}
	if *split == "held_out" || *split == "synthetic" {
		if !*confirm {
			return fmt.Errorf("the single %s run requires -confirm-phase2", *split)
		}
		if err := verifyPreregistration(*pre, *decision); err != nil {
			return err
		}
	}
	base, key := os.Getenv("LITELLM_BASE_URL"), os.Getenv("LITELLM_API_KEY")
	if base == "" || key == "" {
		return errors.New("needs LITELLM_BASE_URL and LITELLM_API_KEY")
	}
	items, pool, labelHash, err := loadCandidate(*split)
	if err != nil {
		return err
	}
	rules, err := jprules.Default()
	if err != nil {
		return err
	}
	gateway := llm.NewLiteLLM(base, key, 120*time.Second)
	gateway.SkipCache(true)
	eng := jpengine.New(gateway, jpengine.DefaultModel, pool, rules, jpengine.DefaultK)
	path := candidateRowsPath(name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("cannot start %s because its run file cannot be created exclusively: %w", name, err)
	}
	start := time.Now()
	runErr := writeCandidateRows(eng, items, f, *workers)
	closeErr := f.Close()
	rows, err := readCandidateRows(path)
	if err != nil {
		return errors.Join(runErr, closeErr, err)
	}
	s := scoreCandidate(name, *split, rows, len(items), labelHash)
	fmt.Printf("%s: criterion %d/%d (%d had a model response); %d qualified, %d declined, %d undetermined, %d failed; gateway $%.4f; p50 %d ms, p95 %d ms\n",
		name, s.Criterion.K, s.Criterion.N, s.ModelCalls-s.Errors, s.Qualifies, s.DoesNotQualify, s.Undetermined, s.Errors, s.CostUSD, s.P50MS, s.P95MS)
	if s.Cases != s.Expected {
		runErr = errors.Join(runErr, fmt.Errorf("run incomplete: %d of %d rows saved", s.Cases, s.Expected))
	}
	logErr := appendRunlog(map[string]any{
		"time": time.Now().Format(time.RFC3339), "run": name, "split": *split, "held_out": *split == "held_out",
		"decision": *decision, "prereg_commit": *pre, "git_revision": revision(), "wall_s": int(time.Since(start).Seconds()),
		"llm_provider": "litellm", "model": jpengine.DefaultModel, "prompt_version": jpengine.PromptVersion, "skip_gateway_cache": true,
		"metric": "criterion match; synthetic refusal for synthetic only", "cases": s.Cases, "expected_cases": s.Expected,
		"scored": s.Criterion.N, "correct": s.Criterion.K, "accuracy": s.Criterion.P, "synthetic_refusals": s.SyntheticRefusals,
		"errors": s.Errors, "model_calls": s.ModelCalls, "cost_usd": s.CostUSD, "latency_p50_ms": s.P50MS,
		"latency_p95_ms": s.P95MS, "complete": runErr == nil && closeErr == nil,
	})
	return errors.Join(runErr, closeErr, logErr, writeCandidateSummary(s, false))
}

func candidateName(split, tag string) (string, error) {
	name := "jp-" + split + "-" + jpengine.DefaultModel + "-" + jpengine.PromptVersion + "-litellm"
	switch split {
	case "held_out", "synthetic":
		if tag != "" {
			return "", fmt.Errorf("%s uses one fixed run file per candidate; omit -tag", split)
		}
		return name, nil
	case "dev_eval":
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

func verifyPreregistration(commit, decision string) error {
	if commit == "" || !decisionID.MatchString(decision) || strings.HasPrefix(commit, "-") {
		return errors.New("the fixed run requires a pushed -prereg commit and a numbered -decision")
	}
	git := func(args ...string) ([]byte, error) {
		return exec.Command("git", append([]string{"-C", ".."}, args...)...).Output()
	}
	remote, err := git("for-each-ref", "--contains="+commit, "refs/remotes/origin")
	if err != nil || len(remote) == 0 {
		return errors.New("the pre-registration commit must be visible on an origin tracking ref")
	}
	if _, err := git("merge-base", "--is-ancestor", commit, "HEAD"); err != nil {
		return errors.New("the pre-registration must belong to the current branch")
	}
	doc, err := git("show", commit+":docs/DECISIONS.md")
	if err != nil {
		return errors.New("could not read DECISIONS.md at the pre-registration commit")
	}
	_, record, ok := strings.Cut("\n"+string(doc), "\n## "+decision+" ")
	record, _, _ = strings.Cut(record, "\n## ")
	if !ok || !strings.Contains(record, jpengine.PromptVersion) || !strings.Contains(record, jpengine.DefaultModel) || !strings.Contains(record, "held_out") {
		return fmt.Errorf("%s must name the held_out split, candidate %s, and model %s", decision, jpengine.PromptVersion, jpengine.DefaultModel)
	}
	paths := []string{
		"backend/cmd/jpeval", "backend/internal/jpengine", "backend/internal/jpcorpus", "backend/internal/jprules",
		"backend/internal/rcep", "backend/internal/domain", "backend/internal/engine", "backend/internal/retrieval",
		"backend/internal/llm", "backend/go.mod", "backend/go.sum", "eval/jp/labels.py", "eval/jp/synthetic-negatives.jsonl",
		"data/jp/rulings.jsonl", "data/jp/translations-en.jsonl", "data/jp/splits.json",
	}
	if _, err := git(append([]string{"diff", "--quiet", commit, "HEAD", "--"}, paths...)...); err != nil {
		return errors.New("the candidate code or data changed after pre-registration")
	}
	status, err := git(append([]string{"status", "--porcelain", "--untracked-files=all", "--"}, paths...)...)
	if err != nil || len(status) > 0 {
		return errors.New("the candidate code and frozen data must match the committed files")
	}
	return nil
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
	ctx, cancel := context.WithTimeout(llm.ContextWithCostTally(parent, tally), 135*time.Second)
	defer cancel()
	start := time.Now()
	res, err := e.Predict(ctx, item.caseValue)
	row := candidateRow{ID: item.label.ID, Label: item.label, Prediction: res.Prediction, CalledModel: res.Votes > 0,
		LatencyMS: time.Since(start).Milliseconds(), CostUSD: tally.USD()}
	if err != nil {
		code, status := logsafe.Classify(err)
		row.Error = string(code)
		if status > 0 {
			row.Error = fmt.Sprintf("%s (HTTP %d)", code, status)
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
