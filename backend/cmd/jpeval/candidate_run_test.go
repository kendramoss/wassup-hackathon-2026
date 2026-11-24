package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jprules"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llmbudget"
)

// The command runs from backend/ (its data paths are relative to it).
func TestMain(m *testing.M) {
	if err := os.Chdir("../.."); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

var testRates = llmbudget.Rates{InputPerMTok: 1, OutputPerMTok: 10, Source: "test quote"}
var testBind = llmbudget.Binding{Model: "gemini-4-argon", Rates: testRates}

// sandbox points every output at a temporary directory and returns it.
func sandbox(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "runs"), 0o755); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join(outDir, "synthetic-negatives.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "synthetic-negatives.jsonl"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	oldOut, oldLog := outDir, runlogPath
	outDir, runlogPath = dir, filepath.Join(dir, "RUNLOG.jsonl")
	t.Cleanup(func() { outDir, runlogPath = oldOut, oldLog })
	return dir
}

// suiteLedger creates the ledger the way budget-init does, for the smoke
// check and the synthetic run.
func suiteLedger(t *testing.T, capUSD float64) *llmbudget.Ledger {
	t.Helper()
	items, pool, _, err := loadCandidate("synthetic")
	if err != nil {
		t.Fatal(err)
	}
	rules, err := jprules.Default()
	if err != nil {
		t.Fatal(err)
	}
	p, err := planItems("synthetic", items, pool, rules)
	if err != nil {
		t.Fatal(err)
	}
	phases := map[string]llmbudget.Phase{
		"smoke":     smokePhase(testRates, 64),
		"synthetic": {WorstUSD: p.worstCase(testRates), Requests: p.Calls, RequestsSHA256: p.RequestsSHA256},
	}
	l, err := llmbudget.Create(filepath.Join(t.TempDir(), "ledger.jsonl"), capUSD, testBind, phases)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func options(t *testing.T, capUSD float64, id string) candidateOptions {
	t.Helper()
	return candidateOptions{split: "synthetic", runID: id, workers: 2, ledger: suiteLedger(t, capUSD), rates: testRates, decision: "D999"}
}

type recorder struct {
	mu     sync.Mutex
	hashes []string
}

func (r *recorder) serve(t *testing.T, h http.HandlerFunc) *llm.LiteLLM {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.hashes = append(r.hashes, fmt.Sprintf("%x", sha256.Sum256(b)))
		r.mu.Unlock()
		h(w, req)
	}))
	t.Cleanup(srv.Close)
	g := llm.NewLiteLLM(srv.URL, "sk-test-key", 5*time.Second)
	g.SkipCache(true)
	return g
}

func TestCandidateNames(t *testing.T) {
	base := "jp-held_out-gemini-4-argon-jp-engine-v1-litellm"
	for _, c := range []struct{ split, tag, id, want string }{
		{"held_out", "", "", base},
		{"held_out", "", "d741-run-1", base + "-d741-run-1"},
		{"synthetic", "", "x", "jp-synthetic-gemini-4-argon-jp-engine-v1-litellm-x"},
		{"dev_eval", "smoke", "", "jp-dev_eval-gemini-4-argon-jp-engine-v1-litellm-smoke"},
	} {
		if got, err := candidateName(c.split, c.tag, c.id); err != nil || got != c.want {
			t.Errorf("candidateName(%q,%q,%q) = %q, %v; want %q", c.split, c.tag, c.id, got, err, c.want)
		}
	}
	for _, bad := range []string{"../x", "a/b", "a b", "A", "-a", "a-", "a.jsonl", strings.Repeat("a", 41), "x\n", "é"} {
		if _, err := candidateName("held_out", "", bad); err == nil {
			t.Errorf("run id %q accepted", bad)
		}
	}
	if _, err := candidateName("held_out", "t", "x"); err == nil {
		t.Error("tag with held_out accepted")
	}
	if _, err := candidateName("dev_eval", "t", "x"); err == nil {
		t.Error("run id with dev_eval accepted")
	}
}

func TestPlanMatchesTheRequestsActuallySent(t *testing.T) {
	sandbox(t)
	items, pool, _, err := loadCandidate("synthetic")
	if err != nil {
		t.Fatal(err)
	}
	rules, err := jprules.Default()
	if err != nil {
		t.Fatal(err)
	}
	plan, err := planItems("synthetic", items, pool, rules)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Cases != 8 || plan.Calls+plan.AbstainedBeforeCall != 8 || plan.Calls == 0 {
		t.Fatalf("plan %+v", plan)
	}
	rec := &recorder{}
	g := rec.serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Litellm-Response-Cost", "0.01")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"{}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`)
	})
	o := options(t, 50, "plan-match")
	name, _ := candidateName(o.split, o.tag, o.runID)
	if err := runCandidate(o, name, g, g); err != nil {
		t.Logf("run error (answers are not valid JSON answers, expected): %v", err)
	}
	got, want := slices.Clone(rec.hashes), slices.Clone(plan.hashes)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("the requests sent differ from the plan: sent %d, planned %d", len(got), len(want))
	}
}

func TestRunCandidateIsExclusiveAndRecordsCostProvenance(t *testing.T) {
	dir := sandbox(t)
	rec := &recorder{}
	g := rec.serve(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "denied for sk-test-key", http.StatusForbidden)
	})
	o := options(t, 50, "first")
	name, _ := candidateName(o.split, o.tag, o.runID)
	_ = runCandidate(o, name, g, g) // every call is refused with 403
	sent := len(rec.hashes)
	if sent == 0 {
		t.Fatal("no requests were sent")
	}
	raw, err := os.ReadFile(candidateRowsPath(name))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sk-test-key") || strings.Contains(string(raw), "127.0.0.1") || strings.Contains(string(raw), "denied") {
		t.Fatalf("saved rows carry provider text:\n%s", raw)
	}
	var rows int
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var r candidateRow
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatal(err)
		}
		rows++
		if r.CalledModel {
			if r.Error != "provider_http_4xx (HTTP 403)" || r.CostProvenance != llmbudget.UnreportedWorstCase || r.CostUSD != 0 || r.BudgetUSD <= 0 {
				t.Errorf("403 row = %+v", r)
			}
		}
	}
	if rows != 8 {
		t.Fatalf("saved %d rows, want all 8", rows)
	}
	m, err := os.ReadFile(candidateMetricsPath(name))
	if err != nil || !strings.Contains(string(m), `"calls_without_reported_cost"`) {
		t.Fatalf("metrics %s: %v", m, err)
	}
	log, _ := os.ReadFile(runlogPath)
	if !strings.Contains(string(log), `"rates_source":"test quote"`) || strings.Contains(string(log), "sk-test-key") {
		t.Fatalf("run log %s", log)
	}
	// The same name again: refused before any request, original untouched.
	before, _ := os.ReadFile(candidateRowsPath(name))
	if err := ensureFree(name); err == nil || !strings.Contains(err.Error(), "-run-id") || strings.Contains(err.Error(), dir) {
		t.Fatalf("ensureFree on an existing run = %v", err)
	}
	if err := runCandidate(o, name, g, g); err == nil || len(rec.hashes) != sent {
		t.Fatalf("second run: err %v, extra requests %d", err, len(rec.hashes)-sent)
	}
	if after, _ := os.ReadFile(candidateRowsPath(name)); string(after) != string(before) {
		t.Fatal("existing rows changed")
	}
}

func TestWhatIsLeftMustCoverThisPhaseAndTheUnstartedOnes(t *testing.T) {
	sandbox(t)
	rec := &recorder{}
	g := rec.serve(t, func(w http.ResponseWriter, _ *http.Request) {})
	o := options(t, 50, "toosmall")
	r, err := o.ledger.Reserve("earlier spend", 49.99) // leaves less than synthetic plus smoke need
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Settle(49.99, llmbudget.Reported)
	name, _ := candidateName(o.split, o.tag, o.runID)
	err = runCandidate(o, name, g, g)
	if !errors.Is(err, llmbudget.ErrBudget) || !strings.Contains(err.Error(), "nothing was sent") || len(rec.hashes) != 0 {
		t.Fatalf("err %v, requests %d", err, len(rec.hashes))
	}
	if _, statErr := os.Stat(candidateRowsPath(name)); statErr == nil {
		t.Fatal("a run file was created")
	}
	// No claim was burned by the refused preflight.
	if err := o.ledger.Claim(o.decision + "|synthetic"); err != nil {
		t.Fatalf("claim burned by a refused run: %v", err)
	}
}

func TestAPlanThatDoesNotFitIsNeverCreated(t *testing.T) {
	phases, err := suitePhases(testRates, 64)
	if err != nil {
		t.Fatal(err)
	}
	var total float64
	for _, p := range phases {
		total += p.WorstUSD
	}
	if phases["held_out"].Requests == 0 || phases["synthetic"].Requests != 8 || phases["smoke"].Requests != 1 || total <= 0 {
		t.Fatalf("suite phases %+v", phases)
	}
	path := filepath.Join(t.TempDir(), "l.jsonl")
	if _, err := llmbudget.Create(path, total/2, testBind, phases); !errors.Is(err, llmbudget.ErrBudget) {
		t.Fatalf("an oversized suite was created: %v", err)
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Fatal("ledger file exists")
	}
}

func TestPhaseOutsideThePlanOrAStoppedLedgerSendsNothingAndKeepsTheClaim(t *testing.T) {
	sandbox(t)
	rec := &recorder{}
	g := rec.serve(t, func(w http.ResponseWriter, _ *http.Request) {})
	o := options(t, 50, "x")
	o.split = "dev_eval"
	o.tag = "smoke"
	o.runID = ""
	if err := runCandidate(o, "jp-dev_eval-x", g, g); !errors.Is(err, llmbudget.ErrNotPlanned) || len(rec.hashes) != 0 {
		t.Fatalf("unplanned phase: %v, requests %d", err, len(rec.hashes))
	}
	o = options(t, 50, "stopped")
	r, _ := o.ledger.Reserve("a", 0.01)
	_ = r.Settle(1, llmbudget.Reported) // an overrun stops the ledger
	name, _ := candidateName(o.split, o.tag, o.runID)
	if err := runCandidate(o, name, g, g); !errors.Is(err, llmbudget.ErrStopped) || len(rec.hashes) != 0 {
		t.Fatalf("stopped ledger: %v, requests %d", err, len(rec.hashes))
	}
	if err := o.ledger.Claim("D999|synthetic"); err != nil {
		t.Fatalf("a stopped ledger burned the claim: %v", err)
	}
}

func TestPlanPathStaysOutOfMeasuredDirectories(t *testing.T) {
	for _, p := range []string{"../eval/jp/plan.json", "../eval/results/x", "../data/jp/x.json"} {
		if safePlanPath(p) == nil {
			t.Errorf("%s accepted", p)
		}
	}
	if err := safePlanPath(filepath.Join(t.TempDir(), "plan.json")); err != nil {
		t.Error(err)
	}
}

// Every package compiled into jpeval must be inside the pre-registration
// freeze, so a change to the transport, the budget guard, the scorer or any
// other code path moves the commit that has to be pre-registered again.
func TestFrozenPathsCoverJpeval(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", "./cmd/jpeval").Output()
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "github.com/kendramoss/wassup-hackathon-2026/backend/"
	for _, imp := range strings.Fields(string(out)) {
		rel, ok := strings.CutPrefix(imp, prefix)
		if !ok {
			continue
		}
		covered := false
		for _, p := range frozenPaths {
			if rest, ok := strings.CutPrefix("backend/"+rel, p); ok && (rest == "" || strings.HasPrefix(rest, "/")) {
				covered = true
			}
		}
		if !covered {
			t.Errorf("%s is compiled into jpeval but is not in frozenPaths", imp)
		}
	}
	for _, p := range []string{"backend/internal/llmbudget", "backend/internal/logsafe", "backend/internal/evalkit", "backend/internal/llm"} {
		if !slices.Contains(frozenPaths, p) {
			t.Errorf("%s missing from frozenPaths", p)
		}
	}
}

func TestSmokeIsOneRequestWithSeparateArtifactAndHonestCost(t *testing.T) {
	for _, c := range []struct {
		name    string
		h       http.HandlerFunc
		wantErr bool
		prov    string
		null    bool
	}{
		{"no cost header", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`)
		}, false, llmbudget.EstimatedFromUsage, true},
		{"403", func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "nope sk-test-key", http.StatusForbidden) }, true, llmbudget.UnreportedWorstCase, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			rec := &recorder{}
			g := rec.serve(t, c.h)
			l := suiteLedger(t, 50)
			out := filepath.Join(t.TempDir(), "smoke.json")
			err := runSmoke("smoke-1", out, 64, l, testRates, g, g)
			if (err != nil) != c.wantErr || len(rec.hashes) != 1 {
				t.Fatalf("err %v, requests %d", err, len(rec.hashes))
			}
			raw, _ := os.ReadFile(out)
			if strings.Contains(string(raw), "sk-test-key") || strings.Contains(string(raw), "127.0.0.1") {
				t.Fatalf("artifact leaks: %s", raw)
			}
			var r smokeResult
			if err := json.Unmarshal(raw, &r); err != nil {
				t.Fatal(err)
			}
			if r.CostProvenance != c.prov || (r.ReportedCost == nil) != c.null || r.BudgetUSD <= 0 || r.MaxOutput != 64 {
				t.Fatalf("result %+v", r)
			}
			if err := runSmoke("smoke-1", out, 64, l, testRates, g, g); err == nil || len(rec.hashes) != 1 {
				t.Fatalf("second smoke over the same file: %v, requests %d", err, len(rec.hashes))
			}
		})
	}
}

// A crash after the first request leaves no run log entry. A new run id under
// the same decision and split must still be refused, with no request sent.
func TestClaimBlocksReentryUnderANewRunIDAfterACrash(t *testing.T) {
	sandbox(t)
	rec := &recorder{}
	g := rec.serve(t, func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "x", http.StatusForbidden) })
	o := options(t, 50, "first-attempt")
	name, _ := candidateName(o.split, o.tag, o.runID)
	_ = runCandidate(o, name, g, g)
	if err := os.Remove(runlogPath); err != nil { // the crash: the final log never existed
		t.Fatal(err)
	}
	sent := len(rec.hashes)
	o.runID = "second-attempt"
	name2, _ := candidateName(o.split, o.tag, o.runID)
	err := runCandidate(o, name2, g, g)
	if !errors.Is(err, llmbudget.ErrClaimed) || len(rec.hashes) != sent {
		t.Fatalf("re-entry: err %v, extra requests %d", err, len(rec.hashes)-sent)
	}
	if _, statErr := os.Stat(candidateRowsPath(name2)); statErr == nil {
		t.Fatal("a second run file was created")
	}
	// Another decision is a different claim and may proceed.
	o.decision = "D1000"
	before := len(rec.hashes)
	_ = runCandidate(o, name2, g, g)
	if len(rec.hashes) == before {
		t.Fatal("a different decision was blocked")
	}
}

// A call that costs more than it reserved stops all further dispatch.
func TestOverrunStopsFurtherDispatch(t *testing.T) {
	sandbox(t)
	rec := &recorder{}
	g := rec.serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Litellm-Response-Cost", "5")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"{}"},"finish_reason":"stop"}]}`)
	})
	o := options(t, 50, "overrun")
	o.workers = 1
	name, _ := candidateName(o.split, o.tag, o.runID)
	err := runCandidate(o, name, g, g)
	if err == nil || !strings.Contains(err.Error(), "budget overrun") {
		t.Fatalf("err = %v, want a budget overrun", err)
	}
	if len(rec.hashes) != 1 {
		t.Fatalf("%d requests were sent; dispatch must stop after the overrun", len(rec.hashes))
	}
	raw, _ := os.ReadFile(candidateRowsPath(name))
	if !strings.Contains(string(raw), `"budget_overrun":true`) || !strings.Contains(string(raw), `"budget_stopped"`) {
		t.Fatalf("rows do not show the overrun and the stop:\n%s", raw)
	}
	if on, _ := o.ledger.Stopped(); !on {
		t.Fatal("ledger not stopped")
	}
}
