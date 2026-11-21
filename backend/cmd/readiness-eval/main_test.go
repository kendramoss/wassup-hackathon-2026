package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/readiness/budget"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func do(t *testing.T, e map[string]string, args ...string) (int, string, string) {
	t.Helper()
	var o, er bytes.Buffer
	code := run(args, env(e), &o, &er)
	return code, o.String(), er.String()
}

const cfgJSON = `{"prompt_version":"engine-v1","model_alias":"dry-alias","max_output_tokens":256,"seed":20261017,"max_retries":1,"workers":1,"votes":1,"fallback_engine_model":"off","verifier_search":"local","retrieval_cutoff_exclusive":"2026-09-26","run_ends_by":"2026-12-01T00:00:00Z"}`

func write(t *testing.T, dir, name, s string) string {
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPreflightRefusals(t *testing.T) {
	for k, v := range map[string]string{
		"K_SERVICE": "svc", "LLM_PROVIDER": "vertex", "VERIFIER_SEARCH": "cross",
		"FALLBACK_ENGINE_MODEL": "gemini-3.8-flash", "PLANNER_CONCURRENCY": "8", "VERIFIER_VOTES": "3",
	} {
		if code, _, er := do(t, map[string]string{k: v}, "budget", "status"); code != 2 || !strings.Contains(er, "refused") {
			t.Errorf("%s=%s: code=%d %s", k, v, code, er)
		}
	}
	if code, _, _ := do(t, map[string]string{"LLM_PROVIDER": "litellm-eval", "FALLBACK_ENGINE_MODEL": "off", "VERIFIER_SEARCH": "local"}, "bogus"); code == 2 {
		t.Error("allowed env refused")
	}
}

func TestRealCallsRefusedByDefault(t *testing.T) {
	if code, _, er := do(t, nil, "smoke"); code == 0 || !strings.Contains(er, "--allow-model-calls") {
		t.Fatalf("%d %s", code, er)
	}
	d := t.TempDir()
	args := []string{"smoke", "--allow-model-calls", "--attempts", filepath.Join(d, "a.jsonl"), "--shared", filepath.Join(d, "s.json"), "--pricing", filepath.Join(d, "p.json"), "--config", filepath.Join(d, "cfg.json")}
	if code, _, _ := do(t, nil, args...); code == 0 {
		t.Fatal("real smoke ran without a pricing proof")
	}
	write(t, d, "s.json", `{"cap_usd":20,"spent_usd":0,"reserved_usd":0,"attempts_raw":0,"failures_raw":0,"calls_authorized":false}`)
	write(t, d, "p.json", `{"schema":2,"entries":[]}`)
	write(t, d, "cfg.json", cfgJSON)
	// A test binary is not a clean VCS build and the config binds nothing: refused before any credential is touched.
	if code, _, er := do(t, nil, args...); code == 0 || !(strings.Contains(er, "clean VCS") || strings.Contains(er, "must bind")) {
		t.Fatalf("expected a freeze-binding stop, got %d %s", code, er)
	}
	if code, _, er := do(t, nil, "eval", "--fixtures", "x"); code == 0 || !strings.Contains(er, "refused") {
		t.Fatalf("%d %s", code, er)
	}
}

func TestSmokeDryRun(t *testing.T) {
	code, out, er := do(t, nil, "smoke", "--dry-run")
	if code != 0 || !strings.Contains(out, "no network") {
		t.Fatalf("%d %s %s", code, out, er)
	}
}

func TestBudgetStatusAndAckHalt(t *testing.T) {
	d := t.TempDir()
	shr := write(t, d, "SHAREDBUDGET.json", `{"cap_usd":20,"spent_usd":0,"reserved_usd":0,"attempts_raw":0,"failures_raw":0,"calls_authorized":false}`)
	att := filepath.Join(d, "attempts.jsonl")
	code, out, er := do(t, nil, "budget", "status", "--attempts", att, "--shared", shr)
	if code != 0 || !strings.Contains(out, "spent_usd=0.000000000") || !strings.Contains(out, "calls_authorized=false") {
		t.Fatalf("%d %s %s", code, out, er)
	}
	b, _ := os.ReadFile(shr)
	if string(b) != `{"cap_usd":20,"spent_usd":0,"reserved_usd":0,"attempts_raw":0,"failures_raw":0,"calls_authorized":false}` {
		t.Fatal("status wrote to the shared file")
	}
	if _, err := os.Stat(att + ".lock"); err == nil {
		t.Fatal("status took a lock file")
	}
	if code, _, _ := do(t, nil, "budget", "ack-halt", "--attempts", att, "--shared", shr, "--note", "short"); code == 0 {
		t.Fatal("ack-halt without a real note")
	}
}

func fixtures(t *testing.T, dir string) string {
	lines := []map[string]any{
		{"id": "A1", "ruling_date": "2026-10-01", "facts": "f", "gold": "China", "fake": "ok:China"},
		{"id": "A2", "ruling_date": "2026-10-02", "facts": "f", "gold": "China", "fake": "ok:Vietnam"},
		{"id": "A3", "ruling_date": "2026-10-03", "facts": "f", "gold": "China", "fake": "error"},
		{"id": "A4", "ruling_date": "2026-10-04", "facts": "f", "gold": "China", "fake": "abstain"},
		{"id": "A5", "ruling_date": "2026-10-05", "facts": "f", "gold": "China", "retrieved": []map[string]string{{"id": "A1", "date": "2026-10-01"}}},
		{"id": "A6", "ruling_date": "2026-09-01", "facts": "f", "gold": "China"},
		{"id": "A7", "ruling_date": "2026-10-07", "facts": "f", "gold": "China", "retrieved": []map[string]string{{"id": "N1", "date": "2026-09-30"}}},
	}
	var sb strings.Builder
	for _, l := range lines {
		b, _ := json.Marshal(l)
		sb.Write(b)
		sb.WriteByte('\n')
	}
	return write(t, dir, "fx.jsonl", sb.String())
}

func TestEvalDryRunHandlesErrorsAbstentionsLeaksAndResume(t *testing.T) {
	d := t.TempDir()
	fx, cfg, res := fixtures(t, d), write(t, d, "cfg.json", cfgJSON), filepath.Join(d, "res.jsonl")
	code, out, er := do(t, nil, "eval", "--dry-run", "--fixtures", fx, "--config", cfg, "--out", res)
	if code != 0 {
		t.Fatalf("%d %s %s", code, out, er)
	}
	want := "cases=7 ok=2 correct=1 errors=1 abstain=1 excluded_leak=3 halted=0"
	if !strings.Contains(out, want) {
		t.Fatalf("want %q in\n%s", want, out)
	}
	rows := readRows(t, res)
	if len(rows) != 7 {
		t.Fatalf("%d rows", len(rows))
	}
	byID := map[string]Row{}
	for _, r := range rows {
		byID[r.Case] = r
		if len(r.ConfigSHA) != 64 {
			t.Fatal("row without config hash")
		}
	}
	if byID["A3"].Status != "error" || byID["A3"].ErrorClass != "http_504" || byID["A4"].Status != "abstain" ||
		byID["A5"].Status != "excluded_leak" || byID["A6"].Status != "excluded_leak" {
		t.Fatalf("%+v", byID)
	}
	if byID["A5"].Detail == "" {
		t.Fatal("leak exclusion without a reason")
	}
	// Resume: nothing is repeated, including the failed case.
	code, out, _ = do(t, nil, "eval", "--dry-run", "--fixtures", fx, "--config", cfg, "--out", res)
	if code != 0 || !strings.Contains(out, "cases=0") || len(readRows(t, res)) != 7 {
		t.Fatalf("resume repeated work: %s", out)
	}
}

func readRows(t *testing.T, p string) []Row {
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var rows []Row
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var r Row
		if err := json.Unmarshal([]byte(l), &r); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, r)
	}
	return rows
}

func TestFrozenConfigRejections(t *testing.T) {
	d := t.TempDir()
	bad := map[string]string{
		"fallback on":  strings.Replace(cfgJSON, `"off"`, `"gemini-3.8-flash"`, 1),
		"no cap":       strings.Replace(cfgJSON, `"max_output_tokens":256`, `"max_output_tokens":0`, 1),
		"wrong seed":   strings.Replace(cfgJSON, `20261017`, `1`, 1),
		"retries":      strings.Replace(cfgJSON, `"max_retries":1`, `"max_retries":3`, 1),
		"workers":      strings.Replace(cfgJSON, `"workers":1`, `"workers":8`, 1),
		"votes":        strings.Replace(cfgJSON, `"votes":1`, `"votes":3`, 1),
		"cross search": strings.Replace(cfgJSON, `"local"`, `"cross"`, 1),
		"bad cutoff":   strings.Replace(cfgJSON, `2026-09-26`, `soon`, 1),
		"no run end":   strings.Replace(cfgJSON, `"run_ends_by":"2026-12-01T00:00:00Z"`, `"run_ends_by":""`, 1),
		"unknown key":  strings.Replace(cfgJSON, `"seed"`, `"temperature":0.7,"seed"`, 1),
	}
	for name, js := range bad {
		if _, _, err := loadConfig(write(t, d, "c.json", js)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, h, err := loadConfig(write(t, d, "ok.json", cfgJSON)); err != nil || len(h) != 64 {
		t.Fatal(err)
	}
}

func TestEvalHaltsAfterTheCaseThatTrippedIt(t *testing.T) {
	// Every case fails: after three consecutive failures the guard stops and
	// the runner must stop too, not record per-case errors and carry on.
	d := t.TempDir()
	var sb strings.Builder
	for _, id := range []string{"B1", "B2", "B3", "B4", "B5", "B6"} {
		sb.WriteString(`{"id":"` + id + `","ruling_date":"2026-10-01","facts":"f","gold":"China","fake":"error"}` + "\n")
	}
	fx, cfg, res := write(t, d, "fx.jsonl", sb.String()), write(t, d, "cfg.json", cfgJSON), filepath.Join(d, "res.jsonl")
	code, out, _ := do(t, nil, "eval", "--dry-run", "--fixtures", fx, "--config", cfg, "--out", res)
	_ = out
	if code == 0 {
		t.Fatal("a tripped guard must fail the run")
	}
	rows := readRows(t, res)
	st := map[string]int{}
	for _, r := range rows {
		st[r.Status]++
	}
	if st["error"] < 2 || st["halted"] == 0 {
		t.Fatalf("%v", st)
	}
}

// The tool and its packages must stay out of the server build, and the tool
// must not link the live CROSS client or the verifier/engine stack.
func TestDependencyBoundaries(t *testing.T) {
	list := func(pkg string) string {
		c := exec.Command("go", "list", "-deps", pkg)
		c.Env = append(os.Environ(), "GOFLAGS=-mod=readonly")
		b, err := c.Output()
		if err != nil {
			t.Fatalf("go list %s: %v", pkg, err)
		}
		return string(b)
	}
	srv := list("../server")
	if strings.Contains(srv, "internal/readiness") {
		t.Fatal("server build includes the eval-only packages")
	}
	tool := list(".")
	for _, banned := range []string{"internal/cross", "internal/verifier", "internal/engine", "internal/httpapi"} {
		if strings.Contains(tool, banned) {
			t.Errorf("tool links %s", banned)
		}
	}
}

func TestVerifyFreezeBindsHashesAndCommit(t *testing.T) {
	proof := []byte(`{"schema":2}`)
	h := sha256.Sum256(proof)
	good := FrozenConfig{PricingProofSHA256: hex.EncodeToString(h[:]), GuardCommit: "abc123"}
	if err := verifyFreeze(good, proof, "abc123"); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]FrozenConfig{
		"empty":        {},
		"no commit":    {PricingProofSHA256: good.PricingProofSHA256},
		"no hash":      {GuardCommit: "abc123"},
		"wrong hash":   {PricingProofSHA256: strings.Repeat("0", 64), GuardCommit: "abc123"},
		"wrong commit": {PricingProofSHA256: good.PricingProofSHA256, GuardCommit: "zzz"},
	} {
		if err := verifyFreeze(c, proof, "abc123"); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if err := verifyFreeze(good, []byte(`{"schema":2,"x":1}`), "abc123"); err == nil {
		t.Error("changed proof bytes accepted")
	}
}

// C5: the real main path (built binary) locks the default HTTP client down.
func TestBuiltBinaryLocksDownDefaultHTTP(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "readiness-eval")
	if runtime.GOOS == "windows" {
		// go build does not append .exe to an explicit -o name; Windows
		// cannot execute a file without it.
		bin += ".exe"
	}
	c := exec.Command("go", "build", "-o", bin, ".")
	c.Env = append(os.Environ(), "GOFLAGS=-mod=readonly")
	if b, err := c.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, b)
	}
	out, err := exec.Command(bin, "selfcheck").CombinedOutput()
	if err != nil || !strings.Contains(string(out), "default http transport denied") {
		t.Fatalf("%v %s", err, out)
	}
	if out, err := exec.Command(bin, "smoke", "--dry-run").CombinedOutput(); err != nil || !strings.Contains(string(out), "no network") {
		t.Fatalf("dry smoke through the real main: %v %s", err, out)
	}
}

func TestLockDownDefaultHTTPFunction(t *testing.T) {
	saved, savedC := http.DefaultTransport, http.DefaultClient.Transport
	defer func() { http.DefaultTransport, http.DefaultClient.Transport = saved, savedC }()
	lockDownDefaultHTTP()
	if _, ok := http.DefaultTransport.(budget.DenyTransport); !ok {
		t.Fatal("default transport not replaced")
	}
	var out bytes.Buffer
	if err := selfcheck(&out); err != nil {
		t.Fatal(err)
	}
}

// C8: Ctrl-C cancels the run context instead of killing the process mid-settle.
func TestSignalContextCancelsOnInterrupt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Process.Signal(os.Interrupt) is not supported on Windows")
	}
	ctx, stop := signalContext()
	defer stop()
	// os.Process.Signal(os.Interrupt) works on both Unix and Windows and
	// delivers the same signal the handler above subscribes to.
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("SIGINT did not cancel the run context")
	}
}

// C3: ack-failures clears only the failure streak.
func TestAckFailuresClearsOnlyTheStreak(t *testing.T) {
	d := t.TempDir()
	shr := write(t, d, "SHAREDBUDGET.json", `{"cap_usd":20,"spent_usd":0,"reserved_usd":0,"attempts_raw":0,"failures_raw":0,"calls_authorized":false}`)
	att := filepath.Join(d, "attempts.jsonl")
	// Scratch copy in a temp dir only; never the real ledger.
	write(t, d, "SHAREDBUDGET.json", `{"cap_usd":20,"spent_usd":0,"reserved_usd":0,"attempts_raw":0,"failures_raw":0,"calls_authorized":true}`)
	l, err := budget.Open(att, shr)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		r, err := l.Reserve(budget.ReserveReq{Alias: "a", Nanos: 1000, MaxOut: 1, CaseID: "c" + string(rune('a'+i))})
		if err != nil {
			t.Fatal(err)
		}
		if err := l.Settle(r, budget.SettleReq{Outcome: budget.OutcomeFailure, Charged: 1000}); err != nil {
			t.Fatal(err)
		}
	}
	before := l.Snapshot()
	l.Close()
	if before.FailureStreak != 3 {
		t.Fatalf("%+v", before)
	}
	stat := []string{"--attempts", att, "--shared", shr}
	if code, _, _ := do(t, nil, append([]string{"budget", "ack-failures", "--note", "operator reviewed the 504 burst"}, stat...)...); code == 0 {
		t.Fatal("ack-failures without --operator-approved")
	}
	if code, _, _ := do(t, nil, append([]string{"budget", "ack-failures", "--operator-approved", "--note", "short"}, stat...)...); code == 0 {
		t.Fatal("ack-failures with a short note")
	}
	if code, _, er := do(t, nil, append([]string{"budget", "ack-failures", "--operator-approved", "--note", "operator reviewed the 504 burst"}, stat...)...); code != 0 {
		t.Fatalf("%s", er)
	}
	l, err = budget.Open(att, shr)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	after := l.Snapshot()
	if after.FailureStreak != 0 || after.SpentNanos != before.SpentNanos || after.AttemptsRaw != before.AttemptsRaw ||
		after.FailuresRaw != before.FailuresRaw || after.OpenNanos != before.OpenNanos || after.Halted {
		t.Fatalf("ack-failures changed more than the streak:\nbefore %+v\nafter  %+v", before, after)
	}
	b, _ := os.ReadFile(shr)
	if !strings.Contains(string(b), `"calls_authorized": true`) {
		t.Fatalf("authorization changed: %s", b)
	}
	// And with nothing to clear it refuses.
	if code, _, _ := do(t, nil, append([]string{"budget", "ack-failures", "--operator-approved", "--note", "operator reviewed the 504 burst"}, stat...)...); code == 0 {
		t.Fatal("ack-failures with no streak")
	}
}

// Issue #375: a retrieved item's date must be a readable ISO date. A raw
// string comparison let "", "10/01/2026" and "1 Oct 2026" sort before the
// cutoff and pass the leakage gate.
func TestLeakCheckFailsClosedOnUnreadableDates(t *testing.T) {
	const cutoff = "2026-09-26"
	fx := func(rulingDate, retrieved string) Fixture {
		t.Helper()
		doc := `{"id":"X","ruling_date":"` + rulingDate + `","facts":"f","gold":"China"`
		if retrieved != "" {
			doc += `,"retrieved":` + retrieved
		}
		doc += `}`
		var f Fixture
		if err := json.Unmarshal([]byte(doc), &f); err != nil {
			t.Fatal(err)
		}
		return f
	}

	for _, tc := range []struct {
		name, retrieved, want string
	}{
		{"iso before cutoff", `[{"id":"N1","date":"2026-09-25"}]`, ""},
		{"iso on cutoff", `[{"id":"N1","date":"2026-09-26"}]`, "on or after the cutoff"},
		{"absent date", `[{"id":"N1"}]`, "missing or non-ISO"},
		{"empty date", `[{"id":"N1","date":""}]`, "missing or non-ISO"},
		{"us format", `[{"id":"N1","date":"10/01/2026"}]`, "missing or non-ISO"},
		{"day month name", `[{"id":"N1","date":"1 Oct 2026"}]`, "missing or non-ISO"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := leakCheck(fx("2026-10-05", tc.retrieved), cutoff, nil)
			if tc.want == "" {
				if got != "" {
					t.Fatalf("leakCheck = %q, want pass", got)
				}
			} else if !strings.Contains(got, tc.want) {
				t.Fatalf("leakCheck = %q, want %q", got, tc.want)
			}
		})
	}

	for _, bad := range []string{"", "10/05/2026", "5 Oct 2026"} {
		if got := leakCheck(fx(bad, ""), cutoff, nil); !strings.Contains(got, "case ruling_date") {
			t.Errorf("case date %q: leakCheck = %q, want a rejection", bad, got)
		}
	}
	if got := leakCheck(fx("2026-10-05", ""), "not-a-date", nil); !strings.Contains(got, "cutoff is not an ISO date") {
		t.Errorf("bad config cutoff: leakCheck = %q", got)
	}
}

func TestEvalDryRunExcludesUnreadableRetrievedDate(t *testing.T) {
	d := t.TempDir()
	fx := write(t, d, "bad.jsonl", `{"id":"B1","ruling_date":"2026-10-05","facts":"f","gold":"China","fake":"ok:China","retrieved":[{"id":"N9","date":"10/02/2026"}]}`+"\n")
	cfg := write(t, d, "cfg.json", cfgJSON)
	res := filepath.Join(d, "res.jsonl")
	code, out, er := do(t, nil, "eval", "--dry-run", "--fixtures", fx, "--config", cfg, "--out", res)
	if code != 0 {
		t.Fatalf("%d %s %s", code, out, er)
	}
	if !strings.Contains(out, "excluded_leak=1") || strings.Contains(out, "ok=1") {
		t.Fatalf("case was scored instead of excluded: %s", out)
	}
	rows := readRows(t, res)
	if len(rows) != 1 || rows[0].Status != "excluded_leak" || rows[0].Correct {
		t.Fatalf("%+v", rows)
	}
	if !strings.Contains(rows[0].Detail, "non-ISO") {
		t.Fatalf("exclusion without a date reason: %+v", rows[0])
	}
}
