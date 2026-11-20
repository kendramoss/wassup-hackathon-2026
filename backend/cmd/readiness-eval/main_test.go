package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/readiness/budget"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func do(t *testing.T, e map[string]string, args ...string) (int, string, string) {
	t.Helper()
	var o, er bytes.Buffer
	code := run(args, env(e), &o, &er)
	return code, o.String(), er.String()
}

const cfgJSON = `{"prompt_version":"engine-v1","model_alias":"dry-alias","max_output_tokens":256,"seed":20261017,"max_retries":1,"workers":1,"votes":1,"fallback_engine_model":"off","verifier_search":"local","retrieval_cutoff_exclusive":"2026-09-26"}`

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
	args := []string{"smoke", "--allow-model-calls", "--attempts", filepath.Join(d, "a.jsonl"), "--shared", filepath.Join(d, "s.json"), "--pricing", filepath.Join(d, "p.json"), "--alias", "x"}
	if code, _, _ := do(t, nil, args...); code == 0 {
		t.Fatal("real smoke ran without a pricing proof")
	}
	write(t, d, "s.json", `{"cap_usd":20,"spent_usd":0,"reserved_usd":0,"attempts_raw":0,"failures_raw":0,"calls_authorized":false}`)
	write(t, d, "p.json", `{"schema":2,"entries":[]}`)
	if code, _, er := do(t, nil, args...); code == 0 || !strings.Contains(er, "LITELLM") {
		t.Fatalf("expected a missing-credentials stop, got %d %s", code, er)
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

func TestDefaultTransportIsDeniedByMain(t *testing.T) {
	saved, savedC := http.DefaultTransport, http.DefaultClient.Transport
	defer func() { http.DefaultTransport, http.DefaultClient.Transport = saved, savedC }()
	http.DefaultTransport, http.DefaultClient.Transport = budget.DenyTransport{}, budget.DenyTransport{}
	if _, err := http.Get("http://127.0.0.1:1/"); err == nil {
		t.Fatal("DefaultClient reached the network")
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
