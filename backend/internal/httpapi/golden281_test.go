package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/verifier"
)

// #281 step 3 legacy goldens through the real handlers and the real planner
// (fake engine and verifier only). Captured with GOLDEN281_UPDATE=1 on the base
// commit, before the operations wiring. After it, the same requests without
// "operations", and with an inactive one (null, {}, {"groups":{}}), must give
// the same bytes.

const goldenDir = "testdata/golden281"

// goldenVerifier supports every move that reaches the target at 0.97.
type goldenVerifier struct{}

func (goldenVerifier) Verify(_ context.Context, t verifier.Task) (verifier.Result, error) {
	return verifier.Result{OriginA: "China", OriginB: "Thailand", ChangesOrigin: true,
		Verdict: domain.Verdict{PrecedentSupported: true, SupportingRuling: "N325810", Confidence: 0.97}, Proof: verifier.ProofFor(t, "N325810")}, nil
}

func goldenEngine(f string) string {
	if strings.Contains(f, "In Thailand, the electric motor") || strings.Contains(f, "In Thailand, the printed circuit board assembly") {
		return "Thailand"
	}
	return ""
}

func realPlannerHarness(t *testing.T, adjust func(*Deps)) harness {
	t.Helper()
	return bomHarness(t, func(d *Deps) {
		eng := d.Engine.(*fakeEngine)
		eng.byFacts = goldenEngine
		d.Planner = planner.New(d.Templates, eng, goldenVerifier{}, planner.Options{PlanConfidence: 0.9, Concurrency: 1, MaxMoves: planner.DefaultMaxMoves})
		if adjust != nil {
			adjust(d)
		}
	})
}

// volatile keys are timestamps and timings; ids are replaced.
var volatile = map[string]bool{"created_at": true, "updated_at": true, "checked_at": true, "at": true, "wall_seconds": true,
	"exported_at": true, "ts": true, "time": true, "started_at": true, "latency_ms": true}

func normalise(v any, ids map[string]bool) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			if volatile[k] {
				continue
			}
			out[k] = normalise(e, ids)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = normalise(e, ids)
		}
		return out
	case string:
		if ids[x] {
			return "<id>"
		}
	}
	return v
}

func goldenCheck(t *testing.T, name string, v any) {
	t.Helper()
	got, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join(goldenDir, name+".json")
	if os.Getenv("GOLDEN281_UPDATE") == "1" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden %s: %v", name, err)
	}
	if !bytes.Equal(got, want) {
		i := 0
		for i < len(got) && i < len(want) && got[i] == want[i] {
			i++
		}
		lo := max(0, i-100)
		t.Errorf("legacy golden %s changed near byte %d\n got: %s\nwant: %s", name, i, got[lo:min(len(got), i+160)], want[lo:min(len(want), i+160)])
	}
}

// legacyEconomics are the money bodies of the golden flows. inject adds an
// operations member (or nothing).
var legacyEconomics = map[string]string{
	"full": `"illustrative":true,"annual_import_value_usd":2000000,"duty_rates":{"China":0.417,"Thailand":0.167},` +
		`"group_costs":{"motor_fan_assembly":{"cn":300000,"th":420000},"pcba":{"cn":150000,"th":210000}}`,
	"no_costs": `"illustrative":true,"annual_import_value_usd":2000000,"duty_rates":{"China":0.417,"Thailand":0.167}`,
	"partial":  `"illustrative":true,"annual_import_value_usd":2000000,"duty_rates":{"China":0.417,"Thailand":0.167},"group_costs":{"motor_fan_assembly":{"cn":300000,"th":420000},"pcba":{"cn":150000}}`,
	"negative": `"illustrative":true,"annual_import_value_usd":2000000,"duty_rates":{"China":0.25,"Thailand":0.2},"group_costs":{"motor_fan_assembly":{"cn":100000,"th":900000},"pcba":{"cn":10,"th":700000}}`,
}

func econJSON(body, inject string) string {
	if inject == "" {
		return "{" + body + "}"
	}
	return "{" + body + "," + inject + "}"
}

// flows runs every legacy flow for one economics body against a fresh case and
// returns the normalised outputs.
func legacyFlows(t *testing.T, body, inject string) map[string]any {
	t.Helper()
	mem := store.NewMemory()
	hs := realPlannerHarness(t, func(d *Deps) { d.Cases = mem })
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	ids := map[string]bool{id: true}
	econ := econJSON(body, inject)
	out := map[string]any{}
	grab := func(name string, code int, v map[string]any) {
		if code != http.StatusOK {
			t.Fatalf("%s: status %d %v", name, code, v)
		}
		out[name] = normalise(v, ids)
	}

	// Seed a Thailand scenario with this economics, as the UI does.
	code, prev := hs.uploadBOM(t, id, motorInThailand(t), "", nil)
	if code != http.StatusOK {
		t.Fatalf("preview %d %v", code, prev)
	}
	pb, _ := json.Marshal(prev["product"])
	code, sc := hs.do(t, "POST", "/v1/cases/"+id+"/scenarios", `{"economics":`+econ+`,"scenarios":[{"label":"Motor in Thailand","product":`+string(pb)+`}]}`)
	grab("scenarios", code, sc)

	code, pe := hs.do(t, "PUT", "/v1/cases/"+id+"/economics", econ)
	grab("put_economics", code, pe)

	code, pl := hs.do(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand","economics":`+econ+`}`)
	grab("plan_json", code, pl)

	code, tg := hs.do(t, "POST", "/v1/cases/"+id+"/compare-targets", `{"targets":["Thailand","Malaysia"],"economics":`+econ+`}`)
	grab("compare_targets", code, tg)

	code, gc := hs.do(t, "GET", "/v1/cases/"+id, "")
	grab("get_case", code, gc)

	cs, err := mem.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	out["scenario_basis"] = scenarioBasis(cs)
	stored, _ := json.Marshal(cs.Economics)
	out["stored_economics"] = string(stored)

	// Streamed plan on a fresh case (the first one now holds a plan).
	hs2 := realPlannerHarness(t, nil)
	_, c2 := hs2.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id2 := c2["id"].(string)
	ids[id2] = true
	req := httptest.NewRequest("POST", "/v1/cases/"+id2+"/plan", strings.NewReader(`{"target":"Thailand","economics":`+econ+`}`))
	req.Header.Set("Accept", "text/event-stream")
	rec := httptest.NewRecorder()
	hs2.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("stream %d %s", rec.Code, rec.Body.String())
	}
	out["plan_sse"] = sseGolden(t, rec.Body.String(), ids)
	return out
}

// sseGolden parses the stream. The move_started/move_done events are keyed by
// move (their arrival order is the goroutines'), and the order rules are
// asserted: start first, a move's started before its done, done last.
func sseGolden(t *testing.T, body string, ids map[string]bool) map[string]any {
	t.Helper()
	type ev struct {
		name string
		data map[string]any
	}
	var evs []ev
	for _, blk := range strings.Split(body, "\n\n") {
		var name, data string
		for _, ln := range strings.Split(blk, "\n") {
			switch {
			case strings.HasPrefix(ln, "event: "):
				name = strings.TrimPrefix(ln, "event: ")
			case strings.HasPrefix(ln, "data: "):
				data = strings.TrimPrefix(ln, "data: ")
			}
		}
		if name == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			t.Fatalf("event %s: %v: %q", name, err, data)
		}
		evs = append(evs, ev{name, m})
	}
	if len(evs) < 3 || evs[0].name != "start" || evs[len(evs)-1].name != "done" {
		t.Fatalf("stream shape: %v", evs)
	}
	started := map[string]bool{}
	out := map[string]any{"start": normalise(evs[0].data, ids), "done": normalise(evs[len(evs)-1].data, ids)}
	moves := map[string]any{}
	for _, e := range evs[1 : len(evs)-1] {
		key, _ := e.data["key"].(string)
		switch e.name {
		case "move_started":
			started[key] = true
		case "move_done":
			if !started[key] {
				t.Errorf("move_done %s before move_started", key)
			}
			// The counter is the order of arrival (goroutines): assert it, do not record it.
			if n, _ := e.data["done"].(float64); int(n) != len(moves)+1 {
				t.Errorf("move_done %s counts %v, want %d", key, e.data["done"], len(moves)+1)
			}
			delete(e.data, "done")
			moves[key] = normalise(e.data, ids)
		case "baseline":
			out["baseline"] = normalise(e.data, ids)
		default:
			t.Errorf("unexpected event %q", e.name)
		}
	}
	out["move_done"] = moves
	return out
}

func TestLegacyGoldenHandlers(t *testing.T) {
	for name, body := range legacyEconomics {
		t.Run(name, func(t *testing.T) {
			goldenCheck(t, "flows_"+name, legacyFlows(t, body, ""))
		})
	}
}
