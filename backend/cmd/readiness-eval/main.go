// Command readiness-eval is the eval-only tool for the #275 held-out run and
// the manual #276 smoke. It is never linked into the server. By default every
// subcommand refuses to make a model call: real calls need --allow-model-calls
// AND a shared budget file whose calls_authorized is true AND a verified
// pricing-proof file. Protocol freeze, review and the main handoff happen
// outside this tool.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/readiness/budget"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/readiness/gateway"
)

// Frozen retry policy: one retry above the guard, so at most two raw attempts
// per logical request, and each one is its own reservation.
const frozenMaxRetries = 1

func main() {
	lockDownDefaultHTTP()
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

// lockDownDefaultHTTP replaces the stdlib default transport so accidental
// http.Get calls cannot reach the network. This is defense in depth, not a
// sandbox: sockets and fresh transports are unaffected, so an OS-level egress
// allowlist to the single gateway host is still required before a live run.
func lockDownDefaultHTTP() {
	http.DefaultTransport = budget.DenyTransport{}
	http.DefaultClient.Transport = budget.DenyTransport{}
}

// signalContext cancels on Ctrl-C / SIGTERM. The guard settles an in-flight
// attempt (charged in full) before returning, so an interrupt leaves no open
// reservation and needs no ack-halt.
func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// buildRevision is the clean VCS revision this binary was built from.
func buildRevision() (string, error) {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "", errors.New("no build info")
	}
	rev, modified := "", ""
	for _, kv := range bi.Settings {
		switch kv.Key {
		case "vcs.revision":
			rev = kv.Value
		case "vcs.modified":
			modified = kv.Value
		}
	}
	if rev == "" || modified != "false" {
		return "", errors.New("binary was not built from a clean VCS checkout")
	}
	return rev, nil
}

// verifyFreeze checks the frozen config's bindings at startup. Empty or
// mismatching values refuse the run; nothing is merely declared.
func verifyFreeze(c FrozenConfig, pricingBytes []byte, guardCommit string) error {
	if c.PricingProofSHA256 == "" || c.GuardCommit == "" {
		return errors.New("frozen config must bind pricing_proof_sha256 and guard_commit before a real call")
	}
	h := sha256.Sum256(pricingBytes)
	if hex.EncodeToString(h[:]) != c.PricingProofSHA256 {
		return errors.New("pricing proof does not match the frozen pricing_proof_sha256")
	}
	if guardCommit != c.GuardCommit {
		return errors.New("this binary's revision does not match the frozen guard_commit")
	}
	return nil
}

func run(args []string, getenv func(string) string, out, errw io.Writer) int {
	if err := preflight(getenv); err != nil {
		fmt.Fprintln(errw, "refused:", err)
		return 2
	}
	if len(args) == 0 {
		fmt.Fprintln(errw, "usage: readiness-eval budget status|ack-halt | smoke | eval")
		return 2
	}
	var err error
	switch args[0] {
	case "budget":
		err = budgetCmd(args[1:], out)
	case "smoke":
		err = smokeCmd(args[1:], getenv, out)
	case "eval":
		err = evalCmd(args[1:], out)
	case "selfcheck":
		err = selfcheck(out)
	default:
		err = fmt.Errorf("unknown subcommand %q", args[0])
	}
	if err != nil {
		fmt.Fprintln(errw, "error:", err)
		if errors.Is(err, budget.ErrHalted) || errors.Is(err, budget.ErrBudget) {
			return 3
		}
		return 1
	}
	return 0
}

// preflight refuses the environments the held-out run must never see: Cloud
// Run, any provider other than the eval-only adapter, live CROSS search,
// fallback models, votes and parallelism.
func preflight(getenv func(string) string) error {
	if getenv("K_SERVICE") != "" {
		return errors.New("K_SERVICE is set: this tool never runs on Cloud Run")
	}
	if p := getenv("LLM_PROVIDER"); p != "" && p != "litellm-eval" {
		return fmt.Errorf("LLM_PROVIDER=%q: only the eval-only adapter is allowed, never native Vertex", p)
	}
	if v := getenv("VERIFIER_SEARCH"); v != "" && v != "local" {
		return errors.New("VERIFIER_SEARCH must be local (live CROSS is denied and would leak post-cutoff text)")
	}
	if f := getenv("FALLBACK_ENGINE_MODEL"); f != "" && f != "off" {
		return errors.New("FALLBACK_ENGINE_MODEL must be off (a second alias means a second price and silent substitution)")
	}
	for _, k := range []string{"PLANNER_CONCURRENCY", "VERIFIER_VOTES", "ENGINE_VOTES"} {
		if v := getenv(k); v != "" && v != "1" {
			return fmt.Errorf("%s must be 1", k)
		}
	}
	return nil
}

// FrozenConfig is the single evaluation configuration. Its file hash goes in
// the freeze manifest and into every result row.
type FrozenConfig struct {
	PromptVersion       string `json:"prompt_version"`
	ModelAlias          string `json:"model_alias"`
	MaxOutputTokens     int    `json:"max_output_tokens"`
	Seed                int    `json:"seed"`
	MaxRetries          int    `json:"max_retries"`
	Workers             int    `json:"workers"`
	Votes               int    `json:"votes"`
	FallbackEngineModel string `json:"fallback_engine_model"`
	VerifierSearch      string `json:"verifier_search"`
	RetrievalCutoff     string `json:"retrieval_cutoff_exclusive"` // corpus items must be dated strictly before
	PricingProofSHA256  string `json:"pricing_proof_sha256,omitempty"`
	GuardCommit         string `json:"guard_commit,omitempty"`
	// RunEndsBy is when the run must be over; the price proof has to stay valid until then.
	RunEndsBy string `json:"run_ends_by"`
}

func loadConfig(path string) (FrozenConfig, string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return FrozenConfig{}, "", err
	}
	var c FrozenConfig
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, "", fmt.Errorf("frozen config: %w", err)
	}
	switch {
	case c.ModelAlias == "" || c.PromptVersion == "":
		return c, "", errors.New("frozen config: model_alias and prompt_version are required")
	case c.MaxOutputTokens <= 0:
		return c, "", errors.New("frozen config: max_output_tokens must be set (hidden reasoning must sit inside it)")
	case c.Seed != llm.Seed:
		return c, "", fmt.Errorf("frozen config: seed must be %d (llm.Seed, D76)", llm.Seed)
	case c.MaxRetries != frozenMaxRetries:
		return c, "", fmt.Errorf("frozen config: max_retries must be %d", frozenMaxRetries)
	case c.Workers != 1 || c.Votes != 1:
		return c, "", errors.New("frozen config: workers and votes must be 1")
	case c.FallbackEngineModel != "off":
		return c, "", errors.New("frozen config: fallback_engine_model must be \"off\"")
	case c.VerifierSearch != "local":
		return c, "", errors.New("frozen config: verifier_search must be \"local\"")
	}
	if _, err := time.Parse(time.RFC3339, c.RunEndsBy); err != nil {
		return c, "", fmt.Errorf("frozen config: run_ends_by: %w", err)
	}
	if _, err := time.Parse(time.DateOnly, c.RetrievalCutoff); err != nil {
		return c, "", fmt.Errorf("frozen config: retrieval_cutoff_exclusive: %w", err)
	}
	h := sha256.Sum256(b)
	return c, hex.EncodeToString(h[:]), nil
}

// selfcheck proves the process-wide network lock-down is active in the real
// main path. It makes no network connection.
func selfcheck(out io.Writer) error {
	_, err := http.Get("http://127.0.0.1:1/")
	if err == nil || !errors.Is(err, budget.ErrNoPermit) {
		return fmt.Errorf("default HTTP client is not locked down: %v", err)
	}
	fmt.Fprintln(out, "default http transport denied")
	return nil
}

func budgetCmd(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("budget status|ack-halt")
	}
	fl := flag.NewFlagSet("budget", flag.ContinueOnError)
	att := fl.String("attempts", "", "attempts.jsonl path")
	shr := fl.String("shared", "", "SHAREDBUDGET.json path")
	note := fl.String("note", "", "ack-halt / ack-failures: what was reconciled and against what")
	operator := fl.Bool("operator-approved", false, "ack-failures: a person decided to resume after a failure streak")
	if err := fl.Parse(args[1:]); err != nil {
		return err
	}
	if *att == "" || *shr == "" {
		return errors.New("--attempts and --shared are required")
	}
	switch args[0] {
	case "status":
		st, capN, auth, err := budget.Inspect(*att, *shr)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "cap_usd=%s spent_usd=%s open_usd=%s attempts=%d failures=%d failure_streak=%d halted=%v calls_authorized=%v records=%d\n",
			budget.FormatNanos(capN), budget.FormatNanos(st.SpentNanos), budget.FormatNanos(st.OpenNanos),
			st.AttemptsRaw, st.FailuresRaw, st.FailureStreak, st.Halted, auth, st.Records)
		return nil
	case "ack-failures":
		// Clears ONLY the consecutive-failure streak. It cannot touch money,
		// counters, authorization or a sticky halt.
		if !*operator || len(strings.TrimSpace(*note)) < 20 {
			return errors.New("--operator-approved and a --note of 20+ characters are required")
		}
		l, err := budget.Open(*att, *shr)
		if err != nil {
			return err
		}
		defer l.Close()
		st := l.Snapshot()
		if st.Halted {
			return errors.New("a sticky halt is active; reconcile and ack-halt first")
		}
		if st.FailureStreak == 0 {
			return errors.New("no failure streak to clear")
		}
		return l.AckFailures(*note)
	case "ack-halt":
		if len(strings.TrimSpace(*note)) < 20 {
			return errors.New("--note must say what was reconciled (20+ characters)")
		}
		l, err := budget.Open(*att, *shr)
		if err != nil {
			return err
		}
		defer l.Close()
		return l.AckHalt(*note)
	}
	return fmt.Errorf("unknown budget subcommand %q", args[0])
}

// dryPrices is the pricing proof used by dry runs only. It is generated in a
// temp dir, never read from or written to the real ledger directory.
const dryPrices = `{"schema":2,"entries":[{"alias":"dry-alias","resolved_model_ids":["dry-model"],"list_usd_per_mtok_in":4,"list_usd_per_mtok_out":20,"gateway_usd_per_mtok_in":4,"gateway_usd_per_mtok_out":20,"source":"dry-run fixture, not a real price","verified_at":"%s","valid_until":"%s","max_output_enforced":true,"output_includes_reasoning":true}]}`

type dryRig struct {
	dir   string
	g     *budget.Guard
	model llm.Model
	l     *budget.Ledger
}

func newDryRig(inner llm.Model, allowed map[string]bool) (*dryRig, error) {
	d, err := os.MkdirTemp("", "readiness-dry-")
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	shr := filepath.Join(d, "SHAREDBUDGET.json")
	if err := os.WriteFile(shr, []byte(`{"cap_usd":20,"spent_usd":0,"reserved_usd":0,"attempts_raw":0,"failures_raw":0,"calls_authorized":true}`), 0o600); err != nil {
		return nil, err
	}
	tbl, err := budget.ParsePriceTable([]byte(fmt.Sprintf(dryPrices, now.Format(time.RFC3339), now.Add(24*time.Hour).Format(time.RFC3339))), 0)
	if err != nil {
		return nil, err
	}
	l, err := budget.Open(filepath.Join(d, "attempts.jsonl"), shr)
	if err != nil {
		return nil, err
	}
	g := budget.NewGuard(inner, l, tbl, budget.Opts{AllowModelCalls: true, RunID: "dry-run", AllowedCases: allowed, RunEndsBy: now.Add(12 * time.Hour)})
	return &dryRig{dir: d, g: g, model: llm.WithRetry(g, frozenMaxRetries, time.Millisecond), l: l}, nil
}

func (r *dryRig) close() { r.l.Close() }

// fakeModel answers deterministically from a per-case directive so dry runs
// exercise success, error and abstention handling without any network.
type fakeModel struct{ directives map[string]string }

func (fakeModel) Provider() string { return "fake" }
func (f fakeModel) Generate(ctx context.Context, req llm.Request) (llm.Response, error) {
	d := f.directives[budget.CaseFrom(ctx)]
	switch {
	case d == "error":
		return llm.Response{}, &llm.APIError{Provider: "fake", StatusCode: 504, Body: "dry-run injected"}
	case d == "abstain":
		return llm.Response{Text: `{"origin":""}`, ModelID: "dry-model", Usage: llm.Usage{InputTokens: 50, OutputTokens: 5}}, nil
	case strings.HasPrefix(d, "ok:"):
		return llm.Response{Text: `{"origin":"` + strings.TrimPrefix(d, "ok:") + `"}`, ModelID: "dry-model", Usage: llm.Usage{InputTokens: 50, OutputTokens: 5}}, nil
	}
	return llm.Response{Text: `{"origin":"China"}`, ModelID: "dry-model", Usage: llm.Usage{InputTokens: 50, OutputTokens: 5}}, nil
}

func smokeCmd(args []string, getenv func(string) string, out io.Writer) error {
	fl := flag.NewFlagSet("smoke", flag.ContinueOnError)
	dry := fl.Bool("dry-run", false, "fake model, temp ledger, no network")
	allow := fl.Bool("allow-model-calls", false, "permit real, paid gateway calls (also needs calls_authorized in the shared file)")
	att := fl.String("attempts", "", "attempts.jsonl path")
	shr := fl.String("shared", "", "SHAREDBUDGET.json path")
	pricing := fl.String("pricing", "", "verified pricing-proof file")
	cfgPath := fl.String("config", "", "frozen config (binds the pricing proof hash, guard commit and run window)")
	if err := fl.Parse(args); err != nil {
		return err
	}
	if *dry {
		r, err := newDryRig(fakeModel{}, map[string]bool{"smoke-synthetic": true})
		if err != nil {
			return err
		}
		defer r.close()
		resp, err := r.model.Generate(budget.WithCase(context.Background(), "smoke-synthetic"),
			llm.Request{Model: "dry-alias", System: "Reply with JSON.", Messages: []llm.Message{{Role: llm.RoleUser, Content: "synthetic ping"}}, MaxOutputTokens: 16})
		if err != nil {
			return err
		}
		st := r.l.Snapshot()
		fmt.Fprintf(out, "dry-run smoke ok: model=%s attempts=%d spent_nanos=%d (fake model, temp ledger, no network)\n", resp.ModelID, st.AttemptsRaw, st.SpentNanos)
		return nil
	}
	if !*allow {
		return errors.New("real smoke refused: pass --allow-model-calls (and the shared file must say calls_authorized=true after main's review); use --dry-run otherwise")
	}
	if *att == "" || *shr == "" || *pricing == "" || *cfgPath == "" {
		return errors.New("--attempts, --shared, --pricing and --config are required for a real call")
	}
	cfg, cfgSHA, err := loadConfig(*cfgPath)
	if err != nil {
		return err
	}
	pb, err := os.ReadFile(*pricing) // read once: the bytes hashed are the bytes parsed
	if err != nil {
		return err
	}
	rev, err := buildRevision()
	if err != nil {
		return err
	}
	if err := verifyFreeze(cfg, pb, rev); err != nil {
		return err
	}
	runEnds, _ := time.Parse(time.RFC3339, cfg.RunEndsBy)
	tbl, err := budget.ParsePriceTable(pb, 0)
	if err != nil {
		return err
	}
	ctx, stop := signalContext()
	defer stop()
	gw, err := gateway.FromEnv(2 * time.Minute)
	if err != nil {
		return err
	}
	l, err := budget.Open(*att, *shr)
	if err != nil {
		return err
	}
	defer l.Close()
	// Public, non-secret bindings go into the chain so it shows what the run was frozen to.
	ph := sha256.Sum256(pb)
	if err := l.Bind(map[string]string{"pricing_proof_sha256": hex.EncodeToString(ph[:]), "config_sha256": cfgSHA,
		"guard_commit": rev, "run_ends_by": cfg.RunEndsBy}); err != nil {
		return err
	}
	g := budget.NewGuard(gw, l, tbl, budget.Opts{AllowModelCalls: true, RunID: "smoke", AllowedCases: map[string]bool{"smoke-synthetic": true}, RunEndsBy: runEnds})
	m := llm.WithRetry(g, frozenMaxRetries, 2*time.Second)
	resp, err := m.Generate(budget.WithCase(ctx, "smoke-synthetic"),
		llm.Request{Model: cfg.ModelAlias, System: "Reply with the single word ok.", Messages: []llm.Message{{Role: llm.RoleUser, Content: "synthetic ping"}}, MaxOutputTokens: 16})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "smoke ok: resolved_model=%s in=%d out=%d\n", resp.ModelID, resp.Usage.InputTokens, resp.Usage.OutputTokens)
	return nil
}

// Fixture is one local evaluation case.
type Fixture struct {
	ID        string `json:"id"`
	Date      string `json:"ruling_date"`
	Facts     string `json:"facts"`
	Gold      string `json:"gold"`
	Retrieved []struct {
		ID   string `json:"id"`
		Date string `json:"date"`
	} `json:"retrieved"`
	Fake string `json:"fake,omitempty"`
}

// Row is one per-case result. Errors, abstentions and leak exclusions are
// recorded explicitly and never dropped or imputed.
type Row struct {
	Case       string `json:"case"`
	ConfigSHA  string `json:"config_sha256"`
	Status     string `json:"status"` // ok | error | abstain | excluded_leak | halted
	Pred       string `json:"pred,omitempty"`
	Gold       string `json:"gold"`
	Correct    bool   `json:"correct"`
	ErrorClass string `json:"error_class,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

// leakCheck is the pre-call hook: the case must be dated at or after the
// cutoff, and nothing retrievable for it may be dated on or after the cutoff
// or be any test case.
func leakCheck(f Fixture, cutoff string, testIDs map[string]bool) string {
	if f.Date < cutoff {
		return "case dated before the cutoff"
	}
	for _, r := range f.Retrieved {
		switch {
		case r.Date >= cutoff:
			return "retrieved " + r.ID + " dated on or after the cutoff"
		case testIDs[r.ID]:
			return "retrieved " + r.ID + " is a test case"
		}
	}
	return ""
}

func evalCmd(args []string, out io.Writer) error {
	fl := flag.NewFlagSet("eval", flag.ContinueOnError)
	dry := fl.Bool("dry-run", false, "fake model, temp ledger, local fixtures only")
	fixtures := fl.String("fixtures", "", "fixtures JSONL")
	cfgPath := fl.String("config", "", "frozen config JSON")
	outPath := fl.String("out", "", "per-case results JSONL (appended; completed cases are skipped on resume)")
	if err := fl.Parse(args); err != nil {
		return err
	}
	if !*dry {
		return errors.New("real evaluation refused: not available until the protocol is frozen, the guard is reviewed and main issues the run handoff; use --dry-run")
	}
	if *fixtures == "" || *cfgPath == "" || *outPath == "" {
		return errors.New("--fixtures, --config and --out are required")
	}
	cfg, cfgSHA, err := loadConfig(*cfgPath)
	if err != nil {
		return err
	}
	fb, err := os.ReadFile(*fixtures)
	if err != nil {
		return err
	}
	var fx []Fixture
	for i, line := range strings.Split(strings.TrimSpace(string(fb)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var f Fixture
		if err := json.Unmarshal([]byte(line), &f); err != nil || f.ID == "" {
			return fmt.Errorf("fixtures line %d: bad record", i+1)
		}
		fx = append(fx, f)
	}
	sort.SliceStable(fx, func(i, j int) bool { return fx[i].ID < fx[j].ID })
	testIDs, allowed, dirs := map[string]bool{}, map[string]bool{}, map[string]string{}
	for _, f := range fx {
		testIDs[f.ID], allowed[f.ID], dirs[f.ID] = true, true, f.Fake
	}
	done := map[string]bool{}
	if b, err := os.ReadFile(*outPath); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			var r Row
			if json.Unmarshal([]byte(line), &r) == nil && r.ConfigSHA == cfgSHA {
				done[r.Case] = true // a recorded case is never repeated, whatever its status
			}
		}
	}
	rig, err := newDryRig(fakeModel{directives: dirs}, allowed)
	if err != nil {
		return err
	}
	defer rig.close()
	of, err := os.OpenFile(*outPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer of.Close()
	emit := func(r Row) error {
		r.ConfigSHA = cfgSHA
		b, _ := json.Marshal(r)
		if _, err := of.Write(append(b, '\n')); err != nil {
			return err
		}
		return of.Sync()
	}
	ctx, stop := signalContext()
	defer stop()
	var n, ok, errs, abst, leak, correct, halted int
	var haltErr error
	for _, f := range fx {
		if done[f.ID] {
			continue
		}
		if ctx.Err() != nil {
			return fmt.Errorf("interrupted: %w (completed cases are recorded; resume skips them)", ctx.Err())
		}
		n++
		if reason := leakCheck(f, cfg.RetrievalCutoff, testIDs); reason != "" {
			leak++
			if err := emit(Row{Case: f.ID, Status: "excluded_leak", Gold: f.Gold, Detail: reason}); err != nil {
				return err
			}
			continue
		}
		if haltErr != nil {
			halted++
			if err := emit(Row{Case: f.ID, Status: "halted", Gold: f.Gold, Detail: haltErr.Error()}); err != nil {
				return err
			}
			continue
		}
		resp, err := rig.model.Generate(budget.WithCase(ctx, f.ID), llm.Request{
			Model: cfg.ModelAlias, System: cfg.PromptVersion, Messages: []llm.Message{{Role: llm.RoleUser, Content: f.Facts}},
			MaxOutputTokens: cfg.MaxOutputTokens})
		row := Row{Case: f.ID, Gold: f.Gold}
		switch {
		case err != nil:
			row.Status, row.ErrorClass = "error", errClass(err)
			errs++
		default:
			var a struct {
				Origin string `json:"origin"`
			}
			if derr := resp.DecodeJSON(&a); derr != nil {
				row.Status, row.ErrorClass = "error", "bad_response"
				errs++
			} else if a.Origin == "" {
				row.Status = "abstain"
				abst++
			} else {
				row.Status, row.Pred, row.Correct = "ok", a.Origin, a.Origin == f.Gold
				ok++
				if row.Correct {
					correct++
				}
			}
		}
		if err := emit(row); err != nil {
			return err
		}
		if h := rig.g.Halted(); h != nil {
			haltErr = h // stop after this case; do not record per-case errors and continue
		}
	}
	fmt.Fprintf(out, "dry-run eval: config_sha256=%s cases=%d ok=%d correct=%d errors=%d abstain=%d excluded_leak=%d halted=%d (errors and abstentions stay in the denominator %d; no network)\n",
		cfgSHA, n, ok, correct, errs, abst, leak, halted, n-leak-halted)
	if haltErr != nil {
		return haltErr
	}
	return nil
}

func errClass(err error) string {
	var api *llm.APIError
	switch {
	case errors.As(err, &api):
		return fmt.Sprintf("http_%d", api.StatusCode)
	case errors.Is(err, budget.ErrCaseNotEligible):
		return "case_not_eligible"
	}
	return "other"
}
