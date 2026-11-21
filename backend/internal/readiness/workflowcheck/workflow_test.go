package workflowcheck

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	workflowRel = ".github/workflows/smoke-nonprod.yml"
	docRel      = "docs/smoke-workflow.md"
)

func root(t *testing.T) string {
	t.Helper()
	r, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func readRepo(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root(t), rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

type tree = map[string]any

func asMap(t *testing.T, v any, what string) tree {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%s is not a map: %#v", what, v)
	}
	return m
}

func load(t *testing.T) (tree, string) {
	t.Helper()
	src := readRepo(t, workflowRel)
	v, err := Parse(src)
	if err != nil {
		t.Fatalf("parse %s: %v", workflowRel, err)
	}
	return asMap(t, v, "workflow"), src
}

// uncommented drops full-line comments, so prose in them cannot satisfy or
// trip a check.
func uncommented(src string) string {
	var b strings.Builder
	for _, l := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "#") {
			continue
		}
		b.WriteString(l + "\n")
	}
	return b.String()
}

func jobs(t *testing.T, w tree) tree { return asMap(t, w["jobs"], "jobs") }

func stepsOf(t *testing.T, job tree) []tree {
	t.Helper()
	raw, ok := job["steps"].([]any)
	if !ok {
		t.Fatalf("steps is not a list: %#v", job["steps"])
	}
	var out []tree
	for _, s := range raw {
		out = append(out, asMap(t, s, "step"))
	}
	return out
}

func allRuns(t *testing.T, w tree) []string {
	t.Helper()
	var runs []string
	for _, j := range jobs(t, w) {
		for _, s := range stepsOf(t, asMap(t, j, "job")) {
			if r, ok := s["run"].(string); ok {
				runs = append(runs, r)
			}
		}
	}
	return runs
}

func TestParserFailsClosedOutsideItsSubset(t *testing.T) {
	for name, src := range map[string]string{
		"tab":        "a:\n\tb: 1\n",
		"flow map":   "a: {b: 1}\n",
		"flow list":  "a: [b]\n",
		"anchor":     "a: &x 1\n",
		"multi doc":  "a: 1\n---\nb: 2\n",
		"duplicate":  "a: 1\na: 2\n",
		"empty":      "# nothing\n",
		"bad indent": "a:\n  b: 1\n    c: 2\n",
	} {
		if _, err := Parse(src); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	v, err := Parse("a:\n  - x: 1\n    y: |\n      l1\n      # kept\n\n      l2\n  - z\nb: 'q'\n")
	if err != nil {
		t.Fatal(err)
	}
	m := v.(tree)
	items := m["a"].([]any)
	if items[0].(tree)["y"] != "l1\n# kept\n\nl2\n" || items[1] != "z" || m["b"] != "q" {
		t.Errorf("parsed = %#v", v)
	}
}

func TestTriggersAreWorkflowDispatchOnly(t *testing.T) {
	w, src := load(t)
	on := asMap(t, w["on"], "on")
	if len(on) != 1 {
		t.Fatalf("triggers = %v, want only workflow_dispatch", keys(on))
	}
	wd := asMap(t, on["workflow_dispatch"], "workflow_dispatch")
	inputs := asMap(t, wd["inputs"], "inputs")
	if !slices.Equal(keys(inputs), []string{"mode"}) {
		t.Fatalf("inputs = %v, want only mode", keys(inputs))
	}
	mode := asMap(t, inputs["mode"], "mode")
	if mode["type"] != "choice" || mode["default"] != "selfcheck" {
		t.Errorf("mode = %v", mode)
	}
	if opts, _ := mode["options"].([]any); len(opts) != 1 || opts[0] != "selfcheck" {
		t.Errorf("mode options = %v, want only selfcheck", mode["options"])
	}
	for _, bad := range []string{"push:", "pull_request", "schedule:", "workflow_call", "workflow_run", "repository_dispatch", "pull_request_target", "issue_comment", "type: boolean", "type: string", "type: environment"} {
		if strings.Contains(uncommented(src), bad) {
			t.Errorf("workflow contains %q", bad)
		}
	}
}

func TestPermissionsAreReadOnlyAndTokenless(t *testing.T) {
	w, src := load(t)
	want := tree{"contents": "read"}
	if got := asMap(t, w["permissions"], "permissions"); !equalTree(got, want) {
		t.Errorf("top permissions = %v", got)
	}
	for name, j := range jobs(t, w) {
		if p, ok := asMap(t, j, name)["permissions"]; ok && !equalTree(asMap(t, p, "permissions"), want) {
			t.Errorf("job %s permissions = %v", name, p)
		}
	}
	code := uncommented(src)
	for _, bad := range []string{"write", "id-token", "write-all", "read-all", "GITHUB_TOKEN", "workload_identity", "google-github-actions", "gcloud", "credentials_json"} {
		if strings.Contains(code, bad) {
			t.Errorf("workflow mentions %q", bad)
		}
	}
	conc := asMap(t, w["concurrency"], "concurrency")
	if conc["group"] != "smoke-nonprod" || conc["cancel-in-progress"] != "false" {
		t.Errorf("concurrency = %v", conc)
	}
	for name, j := range jobs(t, w) {
		if m := asMap(t, j, name)["timeout-minutes"]; !regexp.MustCompile(`^([1-9]|[12][0-9]|30)$`).MatchString(str(m)) {
			t.Errorf("job %s timeout-minutes = %v, want 1..30", name, m)
		}
	}
}

func TestNoSecretsVariablesEnvironmentsOrNetworkTools(t *testing.T) {
	_, src := load(t)
	code := uncommented(src)
	for _, bad := range []string{"secrets", "inherit", "environment", "vars.", "LITELLM", "LLM_PROVIDER", "API_KEY", "TOKEN", "--allow-model-calls", "--pricing", "--shared", "calls_authorized", "curl", "wget", "docker", "terraform", "deploy", "update-traffic", "set -x", "printenv", "env |", "ACTIONS_RUNTIME", "upload-artifact", "actions/cache"} {
		if strings.Contains(code, bad) {
			t.Errorf("workflow contains %q", bad)
		}
	}
	// Every expression is on an allow-list; inputs are read in exactly one place.
	exprs := regexp.MustCompile(`\$\{\{\s*([^}]*?)\s*\}\}`).FindAllStringSubmatch(code, -1)
	var got []string
	for _, e := range exprs {
		got = append(got, e[1])
	}
	slices.Sort(got)
	if want := []string{"false", "inputs.mode", "job.status"}; !slices.Equal(got, want) {
		t.Errorf("expressions = %v, want %v", got, want)
	}
}

func TestPaidPathIsAbsentAndCannotBeEnabled(t *testing.T) {
	w, src := load(t)
	js := jobs(t, w)
	var blocked tree
	for name, j := range js {
		jm := asMap(t, j, name)
		if ifv, ok := jm["if"]; ok {
			if ifv != "${{ false }}" {
				t.Errorf("job %s has a non-literal if: %v", name, ifv)
			}
			blocked = jm
		}
		if _, ok := jm["needs"]; ok {
			t.Errorf("job %s uses needs", name)
		}
	}
	if blocked == nil {
		t.Fatal("no blocked paid-generation job")
	}
	for k := range blocked {
		if !slices.Contains([]string{"if", "runs-on", "timeout-minutes", "steps"}, k) {
			t.Errorf("blocked job has key %q", k)
		}
	}
	steps := stepsOf(t, blocked)
	if len(steps) != 1 || steps[0]["uses"] != nil || !strings.Contains(str(steps[0]["run"]), "BLOCKED") || !strings.Contains(str(steps[0]["run"]), "exit 1") {
		t.Errorf("blocked steps = %v", steps)
	}
	offline := asMap(t, js["offline"], "offline job")
	if _, ok := offline["if"]; ok {
		t.Error("offline job is conditional")
	}
	// The one inputs reference sets MODE; no run script or condition reads inputs.
	code := uncommented(src)
	if n := strings.Count(code, "inputs."); n != 1 {
		t.Errorf("inputs. appears %d times, want 1", n)
	}
	if env := asMap(t, offline["env"], "env"); env["MODE"] != "${{ inputs.mode }}" || env["SMOKE_TARGET"] != "none" {
		t.Errorf("env = %v", env)
	}
	for _, r := range allRuns(t, w) {
		if strings.Contains(r, "${{") && !strings.Contains(r, "job.status") {
			t.Errorf("run script interpolates an expression: %q", r)
		}
	}
}

func TestActionsPinnedAsInTheRepoAndCheckoutKeepsNoCredentials(t *testing.T) {
	w, _ := load(t)
	repoUses := map[string]bool{}
	for _, f := range []string{"ci.yml", "deploy.yml", "eval-slice.yml"} {
		for _, m := range regexp.MustCompile(`uses:\s*(\S+)`).FindAllStringSubmatch(readRepo(t, ".github/workflows/"+f), -1) {
			repoUses[m[1]] = true
		}
	}
	steps := stepsOf(t, asMap(t, jobs(t, w)["offline"], "offline"))
	var seen []string
	for _, s := range steps {
		u, _ := s["uses"].(string)
		if u == "" {
			continue
		}
		seen = append(seen, u)
		if !repoUses[u] {
			t.Errorf("action %q is not a version already pinned in the repo's workflows", u)
		}
		with, _ := s["with"].(tree)
		switch {
		case strings.HasPrefix(u, "actions/checkout"):
			if with["persist-credentials"] != "false" {
				t.Errorf("checkout persists credentials: %v", with)
			}
		case strings.HasPrefix(u, "actions/setup-go"):
			if with["go-version-file"] != "backend/go.mod" || with["cache"] != "false" {
				t.Errorf("setup-go with = %v", with)
			}
		}
	}
	if !slices.Equal(seen, []string{"actions/checkout@v7", "actions/setup-go@v7"}) {
		t.Errorf("actions = %v", seen)
	}
}

func TestRunCommandsExistInTheToolAndTheRepo(t *testing.T) {
	w, _ := load(t)
	mainSrc := readRepo(t, "backend/cmd/readiness-eval/main.go")
	runFn := mainSrc[strings.Index(mainSrc, "func run("):]
	runFn = runFn[:strings.Index(runFn, "\n}\n")]
	var subs []string
	for _, m := range regexp.MustCompile(`case "([a-z-]+)":`).FindAllStringSubmatch(runFn, -1) {
		subs = append(subs, m[1])
	}
	for _, need := range []string{"selfcheck", "smoke"} {
		if !slices.Contains(subs, need) {
			t.Fatalf("tool has no %q subcommand (has %v)", need, subs)
		}
	}
	if !strings.Contains(mainSrc, `fl.Bool("dry-run"`) {
		t.Fatal("smoke has no --dry-run flag")
	}
	invoked := map[string]bool{}
	for _, r := range allRuns(t, w) {
		for _, m := range regexp.MustCompile(`readiness-eval"? (\S+)`).FindAllStringSubmatch(r, -1) {
			if strings.HasPrefix(m[1], "./") || strings.HasPrefix(m[1], "-") {
				continue
			}
			invoked[m[1]] = true
			if !slices.Contains(subs, m[1]) {
				t.Errorf("workflow runs unknown subcommand %q", m[1])
			}
		}
		for _, m := range regexp.MustCompile(`(?:go build -o \S+|go test [^\n]*?-count=1) ((?:\./\S+ ?)+)`).FindAllStringSubmatch(r, -1) {
			for _, pkg := range strings.Fields(m[1]) {
				if st, err := os.Stat(filepath.Join(root(t), "backend", strings.TrimSuffix(pkg, "/"))); err != nil || !st.IsDir() {
					t.Errorf("workflow references missing package %s", pkg)
				}
			}
		}
	}
	if !invoked["selfcheck"] || !invoked["smoke"] {
		t.Errorf("workflow runs %v, want selfcheck and smoke", invoked)
	}
	all := strings.Join(allRuns(t, w), "\n")
	for _, want := range []string{"smoke --dry-run", "./internal/readiness/smokecheck/", "./internal/readiness/workflowcheck/"} {
		if !strings.Contains(all, want) {
			t.Errorf("workflow does not run %q", want)
		}
	}
	// Every smoke invocation is the dry run or the bare refusal check.
	if n := strings.Count(all, "readiness-eval\" smoke"); n != 2 {
		t.Errorf("smoke invocations = %d, want 2 (dry-run, bare refusal): %s", n, all)
	}
}

// The workflow's own assertions about tool output are checked against the real
// binary, so a change to the tool's wording fails here, not in a dispatch run.
func TestToolBehaviourMatchesWhatTheWorkflowAsserts(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the tool")
	}
	w, _ := load(t)
	all := strings.Join(allRuns(t, w), "\n")
	bin := filepath.Join(t.TempDir(), "readiness-eval")
	if runtime.GOOS == "windows" {
		// go build does not append .exe to an explicit -o name; Windows
		// cannot execute a file without it.
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, "./cmd/readiness-eval")
	build.Dir = filepath.Join(root(t), "backend")
	build.Env = append(os.Environ(), "GOFLAGS=-mod=readonly")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	run := func(env []string, args ...string) (string, string, int) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin, args...)
		// A minimal env must include the temp dirs too: on Windows the child
		// falls back to C:\Windows when TMP/TEMP are absent and its dry-run
		// scratch dir cannot be created there.
		home := t.TempDir()
		cmd.Env = append([]string{
			"PATH=" + os.Getenv("PATH"),
			"HOME=" + home,
			"TMP=" + home,
			"TEMP=" + home,
		}, env...)
		var so, se bytes.Buffer
		cmd.Stdout, cmd.Stderr = &so, &se
		err := cmd.Run()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return so.String(), se.String(), code
	}
	grepArg := func(flag string) string {
		m := regexp.MustCompile(`grep ` + flag + ` '([^']+)'`).FindStringSubmatch(all)
		if m == nil {
			t.Fatalf("workflow has no grep %s assertion", flag)
		}
		return m[1]
	}
	out, _, code := run(nil, "selfcheck")
	if code != 0 || strings.TrimSpace(out) != grepArg("-qx") {
		t.Errorf("selfcheck = %d %q, workflow expects %q", code, out, grepArg("-qx"))
	}
	out, _, code = run(nil, "smoke", "--dry-run")
	if code != 0 || !regexp.MustCompile(grepArg("-q")).MatchString(out) {
		t.Errorf("dry-run = %d %q", code, out)
	}
	if !strings.Contains(all, "grep -q 'real smoke refused'") {
		t.Error("workflow does not assert the bare-smoke refusal")
	}
	_, se, code := run(nil, "smoke")
	if code == 0 || !strings.Contains(se, "real smoke refused") {
		t.Errorf("bare smoke = %d %q", code, se)
	}
	// Native-Vertex, gateway-as-production or Cloud Run settings are refused
	// before any subcommand runs.
	for _, env := range [][]string{{"LLM_PROVIDER=vertex"}, {"LLM_PROVIDER=litellm"}, {"K_SERVICE=backend"}} {
		_, se, code := run(env, "smoke", "--dry-run")
		if code != 2 || !strings.Contains(se, "refused") {
			t.Errorf("%v: exit %d %q, want refusal", env, code, se)
		}
	}
	// Even with the flag, a real call is refused when its inputs are absent.
	_, se, code = run(nil, "smoke", "--allow-model-calls")
	if code == 0 || !strings.Contains(se, "required for a real call") {
		t.Errorf("--allow-model-calls without inputs = %d %q", code, se)
	}
}

func TestProductionTargetRefusalStepRuns(t *testing.T) {
	// The refusal step is a bash `run:` block from a Linux Actions runner.
	// Without sh there is no faithful way to execute it; Windows hosts that
	// ship Git Bash (sh on PATH) still run the test.
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh on PATH: the workflow refusal step runs on a Linux Actions runner")
	}
	w, _ := load(t)
	steps := stepsOf(t, asMap(t, jobs(t, w)["offline"], "offline"))
	script, first := "", -1
	for i, s := range steps {
		if r := str(s["run"]); r != "" && first < 0 {
			first = i
		}
		if strings.HasPrefix(str(s["name"]), "Refuse non-local targets") {
			script = str(s["run"])
		}
	}
	if script == "" {
		t.Fatal("no refusal step")
	}
	if str(steps[first]["run"]) != script {
		t.Error("the refusal step is not the first run step")
	}
	run := func(mode, target string) (int, string) {
		cmd := exec.Command("sh", "-c", script)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "MODE=" + mode, "SMOKE_TARGET=" + target}
		out, err := cmd.CombinedOutput()
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode(), string(out)
		}
		if err != nil {
			t.Fatal(err)
		}
		return 0, string(out)
	}
	for _, target := range []string{"none", "local-harness"} {
		if code, out := run("selfcheck", target); code != 0 {
			t.Errorf("target %q refused: %s", target, out)
		}
	}
	deploy := readRepo(t, ".github/workflows/deploy.yml")
	prod := regexp.MustCompile(`https://\S+`).FindAllString(deploy, -1)
	if len(prod) < 3 {
		t.Fatalf("expected the production URLs in deploy.yml, found %v", prod)
	}
	prod = append(prod, "https://x.run.app", "origin-proof-api-staging", "origin-proof-api", "undelabs.dev", "wassup-hackathon-2026", "http://localhost:8080", "127.0.0.1", "NONE", "none ", "", "none\nrun.app")
	for _, target := range prod {
		if code, _ := run("selfcheck", target); code == 0 {
			t.Errorf("target %q accepted", target)
		}
	}
	for _, mode := range []string{"", "live", "paid", "selfcheck ", "SELFCHECK", "dry-run"} {
		if code, _ := run(mode, "none"); code == 0 {
			t.Errorf("mode %q accepted", mode)
		}
	}
}

func TestDocsStateTheBoundary(t *testing.T) {
	doc := readRepo(t, docRel)
	for _, must := range []string{"Refs #276", "UNVERIFIED", "not native Vertex", "BLOCKED", "docs/evaluation-readiness.md", "SHAREDBUDGET.json", "smoke-nonprod.yml", "go test -race -count=1", "readiness-eval selfcheck", "smoke --dry-run", "origin-proof-api-staging"} {
		if !strings.Contains(doc, must) {
			t.Errorf("%s does not mention %q", docRel, must)
		}
	}
	for _, bad := range []string{"$20 per run", "$20 a run", "Closes #276", "Fixes #276", "Resolves #276"} {
		if strings.Contains(doc, bad) {
			t.Errorf("%s contains %q", docRel, bad)
		}
	}
}

func keys(m tree) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func equalTree(a, b tree) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func str(v any) string { s, _ := v.(string); return s }
