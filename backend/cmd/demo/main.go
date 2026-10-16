// Command demo prepares a running Origin Proof server for recording the demo.
//
//	demo seed [-api http://localhost:8080] [-multi testdata/heatgun_multi.json] [-out FILE]
//
// seed creates a case for every demo (GET /v1/demos), runs its plan search,
// and for the heat gun also the scenarios, a ruling watch, one question to the
// case agent and the multi-country comparison. It then repeats the heat-gun
// plan search and checks the verdicts match: with the server's verdict cache
// on (VERIFIER_CACHE_DIR, the default), replays during recording are
// identical. The first verdict for each move is still one model draw; this
// command records it, it does not pick a favourable one (docs/DEMO.md).
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "seed" {
		fmt.Fprintln(os.Stderr, "usage: demo seed [-api URL] [-multi FILE] [-out FILE]")
		os.Exit(2)
	}
	if err := seed(os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "demo:", err)
		os.Exit(1)
	}
}

type client struct {
	base    string
	http    *http.Client
	token   string // ADMIN_TOKEN: bypasses the rate limits, pins the demo cases
	idToken string // ID_TOKEN: Google identity token for a private Cloud Run service
	dumpDir string // -dump: write the full heat-gun responses here
	// waits are the pauses before each retry of a request that failed because
	// the model provider was unavailable (Vertex 429/504 show up as 502
	// model_unavailable). Completed engine calls and precedent checks are
	// cached server-side (D50, D54), so a retry redoes only what failed and
	// cannot change a verdict already given.
	waits []time.Duration
}

// pin marks a seeded case as a demo (listed first, protected from deletion).
// Without the admin token the case is left unpinned.
func (c client) pin(id string) {
	if c.token == "" {
		return
	}
	if err := c.call("PUT", "/v1/cases/"+id+"/pin", map[string]bool{"pinned": true}, nil); err != nil {
		fmt.Fprintln(os.Stderr, "WARNING: could not pin", id+":", err)
	}
}

func (c client) call(method, path string, body any, out any) error {
	b, err := c.callRaw(method, path, body)
	if err != nil || out == nil {
		return err
	}
	return json.Unmarshal(b, out)
}

// dump writes a raw response as <dir>/demo-seed-heatgun-<name>.json when -dump is set.
func (c client) dump(name string, raw []byte) {
	if c.dumpDir == "" {
		return
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	p := filepath.Join(c.dumpDir, "demo-seed-heatgun-"+name+".json")
	if err := os.WriteFile(p, append(b, '\n'), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "WARNING:", err)
		return
	}
	fmt.Println("wrote", p)
}

func (c client) callRaw(method, path string, body any) ([]byte, error) {
	for i := 0; ; i++ {
		b, status, err := c.once(method, path, body)
		if err == nil || !retryable(status) || i >= len(c.waits) {
			return b, err
		}
		fmt.Fprintf(os.Stderr, "%s %s: model provider unavailable (%d); retry %d of %d in %s\n", method, path, status, i+1, len(c.waits), c.waits[i])
		time.Sleep(c.waits[i])
	}
}

// retryable reports whether a failed request is worth repeating: the model
// provider was unavailable or the request timed out. 0 is a transport error.
func retryable(status int) bool {
	return status == 0 || status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

func (c client) once(method, path string, body any) ([]byte, int, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, -1, err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, r)
	if err != nil {
		return nil, -1, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("X-Admin-Token", c.token)
	}
	if c.idToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.idToken)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, resp.StatusCode, fmt.Errorf("%s %s: %d %s", method, path, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return b, resp.StatusCode, nil
}

type demo struct {
	ID      string          `json:"id"`
	Title   string          `json:"title"`
	CBPHeld string          `json:"cbp_held"`
	Predict json.RawMessage `json:"predict"`
	Plan    json.RawMessage `json:"plan"`
}

type verdict struct {
	Confidence float64 `json:"confidence"`
	Supporting string  `json:"supporting_ruling"`
}

type plan struct {
	Description string   `json:"description"`
	Kind        string   `json:"rejection_kind"`
	Shown       bool     `json:"shown"`
	Net         float64  `json:"net_per_year"`
	Verdict     *verdict `json:"verdict"`
	Prediction  struct {
		Origin string `json:"origin_country"`
	} `json:"prediction"`
}

type planResult struct {
	Plans    []plan `json:"plans"`
	Rejected []plan `json:"rejected"`
	Stats    struct {
		Cached int     `json:"verifier_cached"`
		Runs   int     `json:"verifier_runs"`
		Wall   float64 `json:"wall_seconds"`
	} `json:"stats"`
}

type caseResp struct {
	ID         string `json:"id"`
	Provider   string `json:"llm_provider"`
	Prediction struct {
		Origin     string  `json:"origin_country"`
		Confidence float64 `json:"confidence"`
	} `json:"prediction"`
	Triage struct {
		Route string `json:"route"`
	} `json:"triage"`
}

// Seeded is one demo's outcome, written to -out.
type Seeded struct {
	Demo       string   `json:"demo"`
	Title      string   `json:"title"`
	CaseID     string   `json:"case_id"`
	CBPHeld    string   `json:"cbp_held"`
	Origin     string   `json:"predicted_origin"`
	Confidence float64  `json:"confidence"`
	Route      string   `json:"route"`
	BestPlan   string   `json:"best_plan,omitempty"`
	PlanStatus string   `json:"plan_status,omitempty"`
	Notes      []string `json:"notes,omitempty"`
}

func verdicts(p planResult) map[string]string {
	out := map[string]string{}
	for _, x := range append(append([]plan{}, p.Plans...), p.Rejected...) {
		c := -1.0
		if x.Verdict != nil {
			c = x.Verdict.Confidence
		}
		out[x.Description] = fmt.Sprintf("%s %.2f", x.Kind, c)
	}
	return out
}

func bestOf(p planResult) (string, string) {
	if len(p.Plans) > 0 && p.Plans[0].Verdict != nil {
		b := p.Plans[0]
		return fmt.Sprintf("%s (check %.2f, net $%.0f/yr)", b.Description, b.Verdict.Confidence, b.Net), "shown"
	}
	for _, x := range p.Rejected {
		if x.Kind == "below_confidence_bar" && x.Verdict != nil {
			return fmt.Sprintf("%s (check %.2f, below the bar)", x.Description, x.Verdict.Confidence), "below_bar"
		}
	}
	return "", "no_plan"
}

func seed(args []string) error {
	fl := flag.NewFlagSet("seed", flag.ExitOnError)
	api := fl.String("api", "http://localhost:8080", "server base URL")
	multi := fl.String("multi", "testdata/heatgun_multi.json", "multi-country heat-gun fixture")
	out := fl.String("out", "", "write the seeded cases as JSON (default: print only)")
	retries := fl.Int("retries", 4, "retries per request when the model provider is unavailable (waits 1, 2, 4, 8 min)")
	dumpDir := fl.String("dump", "", "also write the full heat-gun predict, plan and scenarios responses to this directory")
	_ = fl.Parse(args)
	c := client{base: strings.TrimRight(*api, "/"), http: &http.Client{Timeout: 10 * time.Minute}, token: os.Getenv("ADMIN_TOKEN"), idToken: os.Getenv("ID_TOKEN"), dumpDir: *dumpDir}
	for i := 0; i < *retries; i++ {
		c.waits = append(c.waits, time.Minute<<i)
	}
	if c.token == "" {
		fmt.Fprintln(os.Stderr, "WARNING: ADMIN_TOKEN is not set: the seed may hit the rate limits (429), and demo cases will not be pinned")
	}
	if err := c.call("GET", "/health", nil, nil); err != nil {
		return fmt.Errorf("server not reachable at %s (start it with make run; a private Cloud Run service needs ID_TOKEN=$(gcloud auth print-identity-token)): %w", c.base, err)
	}
	var demos struct {
		Demos []struct {
			ID string `json:"id"`
		} `json:"demos"`
	}
	if err := c.call("GET", "/v1/demos", nil, &demos); err != nil {
		return err
	}
	var seeded []Seeded
	var heatgunCase string
	var heatgunPlan json.RawMessage
	var heatgunFirst planResult
	for _, d := range demos.Demos {
		var full demo
		if err := c.call("GET", "/v1/demos/"+d.ID, nil, &full); err != nil {
			return err
		}
		var cr caseResp
		raw, err := c.callRaw("POST", "/v1/cases/predict", full.Predict)
		if err == nil {
			err = json.Unmarshal(raw, &cr)
		}
		if err != nil {
			return fmt.Errorf("demo %s: %w", d.ID, err)
		}
		c.pin(cr.ID)
		if d.ID == "heatgun-2022" {
			c.dump("predict", raw)
		}
		s := Seeded{Demo: d.ID, Title: full.Title, CaseID: cr.ID, CBPHeld: full.CBPHeld, Origin: cr.Prediction.Origin,
			Confidence: cr.Prediction.Confidence, Route: cr.Triage.Route}
		fmt.Printf("%-24s case %s: %s %.2f (%s); CBP held %s\n", d.ID, cr.ID, cr.Prediction.Origin, cr.Prediction.Confidence, cr.Triage.Route, full.CBPHeld)
		if len(full.Plan) > 0 && string(full.Plan) != "null" {
			var pr struct {
				Plan planResult `json:"plan"`
			}
			praw, err := c.callRaw("POST", "/v1/cases/"+cr.ID+"/plan", full.Plan)
			if err == nil {
				err = json.Unmarshal(praw, &pr)
			}
			if err != nil {
				return fmt.Errorf("demo %s plan: %w", d.ID, err)
			}
			if d.ID == "heatgun-2022" {
				c.dump("plan", praw)
			}
			s.BestPlan, s.PlanStatus = bestOf(pr.Plan)
			fmt.Printf("%-24s plan: %s %s (%.0f s)\n", "", s.PlanStatus, s.BestPlan, pr.Plan.Stats.Wall)
			if d.ID == "heatgun-2022" {
				heatgunCase, heatgunPlan, heatgunFirst = cr.ID, full.Plan, pr.Plan
			}
		}
		seeded = append(seeded, s)
	}
	if heatgunCase != "" {
		notes, err := heatgunExtras(c, heatgunCase, heatgunPlan, heatgunFirst, *multi)
		if err != nil {
			return err
		}
		for i := range seeded {
			if seeded[i].CaseID == heatgunCase {
				seeded[i].Notes = notes
			}
		}
	}
	if *out != "" {
		b, _ := json.MarshalIndent(seeded, "", "  ")
		if err := os.WriteFile(*out, append(b, '\n'), 0o644); err != nil {
			return err
		}
		fmt.Println("wrote", *out)
	}
	return nil
}

// heatgunExtras fills the heat-gun case for every screen of the demo and
// checks that a repeated plan search gives the same verdicts.
func heatgunExtras(c client, id string, planBody json.RawMessage, first planResult, multiPath string) ([]string, error) {
	var notes []string
	var again struct {
		Plan planResult `json:"plan"`
	}
	if err := c.call("POST", "/v1/cases/"+id+"/plan", planBody, &again); err != nil {
		return nil, fmt.Errorf("heat-gun repeat plan: %w", err)
	}
	a, b := verdicts(first), verdicts(again.Plan)
	same := len(a) == len(b)
	for k, v := range a {
		if b[k] != v {
			same = false
		}
	}
	msg := fmt.Sprintf("repeat plan search: verdicts identical=%v, %d/%d from cache, %.1f s", same, again.Plan.Stats.Cached, again.Plan.Stats.Runs, again.Plan.Stats.Wall)
	fmt.Printf("%-24s %s\n", "", msg)
	notes = append(notes, msg)
	if !same {
		notes = append(notes, "WARNING: verdicts changed between runs; is VERIFIER_CACHE_DIR off on the server?")
	}

	var d demo
	if err := c.call("GET", "/v1/demos/heatgun-2022", nil, &d); err != nil {
		return nil, err
	}
	var pred struct {
		Product map[string]any `json:"product"`
	}
	_ = json.Unmarshal(d.Predict, &pred)
	var pb struct {
		Economics json.RawMessage `json:"economics"`
	}
	_ = json.Unmarshal(planBody, &pb)
	if pred.Product != nil {
		scen := []map[string]any{{"label": "As today", "product": pred.Product}}
		for _, groups := range [][]string{{"motor_fan_assembly"}, {"motor_fan_assembly", "pcba"}} {
			scen = append(scen, map[string]any{"label": strings.Join(groups, " + ") + " in Thailand", "product": moved(pred.Product, groups)})
		}
		var sr struct {
			Scenarios []struct {
				Label      string `json:"label"`
				Prediction struct {
					Origin     string  `json:"origin_country"`
					Confidence float64 `json:"confidence"`
				} `json:"prediction"`
				Comparison *struct {
					Net *float64 `json:"net_usd"`
				} `json:"comparison"`
			} `json:"scenarios"`
		}
		sraw, err := c.callRaw("POST", "/v1/cases/"+id+"/scenarios", map[string]any{"scenarios": scen, "economics": pb.Economics})
		if err == nil {
			err = json.Unmarshal(sraw, &sr)
		}
		if err != nil {
			return nil, fmt.Errorf("heat-gun scenarios: %w", err)
		}
		c.dump("scenarios", sraw)
		for _, s := range sr.Scenarios {
			net := "n/a"
			if s.Comparison != nil && s.Comparison.Net != nil {
				net = fmt.Sprintf("$%.0f", *s.Comparison.Net)
			}
			m := fmt.Sprintf("scenario %q: %s %.2f, net %s/yr", s.Label, s.Prediction.Origin, s.Prediction.Confidence, net)
			fmt.Printf("%-24s %s\n", "", m)
			notes = append(notes, m)
		}
	}
	var w struct {
		Watch struct {
			Status string `json:"status"`
		} `json:"watch"`
	}
	if err := c.call("POST", "/v1/cases/"+id+"/watch", nil, &w); err != nil {
		return nil, fmt.Errorf("heat-gun watch: %w", err)
	}
	notes = append(notes, "ruling watch: "+w.Watch.Status)
	fmt.Printf("%-24s ruling watch: %s\n", "", w.Watch.Status)
	var ask struct {
		Reply struct {
			Answer string   `json:"answer"`
			Cited  []string `json:"cited_rulings"`
		} `json:"reply"`
	}
	if err := c.call("POST", "/v1/cases/"+id+"/ask", map[string]any{"question": "Why this origin, and which earlier ruling is closest?"}, &ask); err != nil {
		notes = append(notes, "ask failed: "+err.Error())
	} else {
		notes = append(notes, fmt.Sprintf("ask: answered, cites %v", ask.Reply.Cited))
		fmt.Printf("%-24s ask: cites %v\n", "", ask.Reply.Cited)
	}

	if multiPath == "" {
		return notes, nil
	}
	raw, err := os.ReadFile(multiPath)
	if errors.Is(err, os.ErrNotExist) {
		return append(notes, "multi-country fixture not found; skipped"), nil
	} else if err != nil {
		return nil, err
	}
	var fx map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fx); err != nil {
		return nil, err
	}
	body := map[string]json.RawMessage{}
	for _, k := range []string{"date", "product", "family", "exclude_rulings", "current_origin"} {
		body[k] = fx[k]
	}
	var mc caseResp
	if err := c.call("POST", "/v1/cases/predict", body, &mc); err != nil {
		return nil, fmt.Errorf("multi-country case: %w", err)
	}
	c.pin(mc.ID)
	var cmp struct {
		Targets []struct {
			Rank   int    `json:"rank"`
			Target string `json:"target"`
			Status string `json:"status"`
		} `json:"targets"`
	}
	if err := c.call("POST", "/v1/cases/"+mc.ID+"/compare-targets", map[string]any{"targets": []string{"Thailand", "Vietnam", "Malaysia"}, "economics": fx["economics"]}, &cmp); err != nil {
		return nil, fmt.Errorf("compare targets: %w", err)
	}
	var parts []string
	for _, t := range cmp.Targets {
		parts = append(parts, fmt.Sprintf("#%d %s %s", t.Rank, t.Target, t.Status))
	}
	m := fmt.Sprintf("compare targets (case %s): %s", mc.ID, strings.Join(parts, ", "))
	fmt.Printf("%-24s %s\n", "", m)
	return append(notes, m), nil
}

// moved copies a product with every component of the given groups made at
// the Thailand plant "th".
func moved(product map[string]any, groups []string) map[string]any {
	b, _ := json.Marshal(product)
	var p map[string]any
	_ = json.Unmarshal(b, &p)
	want := map[string]bool{}
	for _, g := range groups {
		want[g] = true
	}
	comps, _ := p["components"].([]any)
	for _, c := range comps {
		m, _ := c.(map[string]any)
		if g, _ := m["group"].(string); want[g] {
			m["plant"], m["made_in"] = "th", "Thailand"
		}
	}
	return p
}
