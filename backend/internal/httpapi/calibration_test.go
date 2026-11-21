package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/calibrate"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// D160: with the calibration map on, a primary engine-v1 prediction carries
// the calibrated probability and is triaged on the calibrated bar; the
// stated confidence is kept as it was.
func TestCalibratedConfidenceInPredictAndTriage(t *testing.T) {
	m := calibrate.Must()
	tests := []struct {
		name     string
		conf     float64
		model    string
		cal      any
		route    string
		contains string
	}{
		{"stated 0.97 -> 0.94, auto", 0.97, "", 0.9355, "auto", "calibrated confidence 0.94 ≥ 0.90 (cal-v2; model stated 0.97)"},
		{"stated 0.92 -> 0.90, auto at the bar", 0.92, "", 0.9, "auto", "calibrated confidence 0.90 ≥ 0.90"},
		{"stated 0.90 -> 0.78, below the bar (clarify asks)", 0.90, "", 0.7778, "needs_info", "calibrated confidence 0.78 below 0.90"},
		{"fallback model: no calibrated value", 0.99, "gemini-fallback", nil, "needs_review", "fallback model"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
				d.Engine = &fakeEngine{conf: tt.conf, modelID: tt.model}
				d.Calibration, d.CalibratedFastPath = &m, m.Bar
			})
			code, out := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
			if code != http.StatusCreated {
				t.Fatalf("predict status %d: %v", code, out)
			}
			pred := out["prediction"].(map[string]any)
			if pred["confidence"] != tt.conf {
				t.Errorf("stated confidence = %v, want %v", pred["confidence"], tt.conf)
			}
			if pred["calibrated_confidence"] != tt.cal {
				t.Errorf("calibrated_confidence = %v, want %v", pred["calibrated_confidence"], tt.cal)
			}
			if tt.cal != nil && pred["calibration"] != "cal-v2" {
				t.Errorf("calibration = %v", pred["calibration"])
			}
			tr := out["triage"].(map[string]any)
			if tr["route"] != tt.route || !strings.Contains(tr["reason"].(string), tt.contains) {
				t.Errorf("triage = %v, want %s containing %q", tr, tt.route, tt.contains)
			}
		})
	}
}

// With the map off (CALIBRATION=off) nothing changes from before D160.
func TestCalibrationOffKeepsStatedBar(t *testing.T) {
	var d Deps
	newHarnessWith(t, "substantial_transformation", func(dd *Deps) {
		dd.Engine = &fakeEngine{conf: 0.92}
		d = *dd
	})
	cl, err := NewCasePredictor(d).Classify(context.Background(), domain.Case{ID: "x", Facts: "heat gun"})
	if err != nil {
		t.Fatal(err)
	}
	if cl.Result.Prediction.CalibratedConfidence != nil || cl.Triage.Route != domain.RouteAuto || !strings.Contains(cl.Triage.Reason, "confidence 0.92 ≥ 0.92") {
		t.Errorf("got %+v / %+v", cl.Result.Prediction.CalibratedConfidence, cl.Triage)
	}
}

// D180: a case saved before the map existed (no calibrated value stored)
// gets one at read time when the map was fitted for its engine and model,
// in the case, its plan and the list row. A prediction from another model
// stays stated-only, and the stored case is not changed.
func TestCalibrationBackfilledAtReadTime(t *testing.T) {
	m := calibrate.Must()
	mem := store.NewMemory()
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.Calibration, d.CalibratedFastPath = &m, m.Bar
		d.Cases = mem
	})
	argon := domain.Trace{Provider: "vertex", ModelID: "gemini-4-argon", PromptVersion: "engine-v1"}
	c := store.Case{ID: "case_old", Facts: "heat gun", Date: "2026-01-14",
		Prediction: domain.Prediction{Origin: "China", Confidence: 0.85, Trace: argon},
		Triage:     domain.Triage{Route: domain.RouteNeedsInfo, Reason: "confidence 0.85 below 0.92"},
		Plan: &planner.Result{Target: "Thailand", Plans: []domain.Plan{
			{Description: "move", Shown: true, Prediction: domain.Prediction{Origin: "Thailand", Confidence: 1, Trace: argon}},
			{Description: "other model", Prediction: domain.Prediction{Origin: "Thailand", Confidence: 1, Trace: domain.Trace{ModelID: "gemini-fallback", PromptVersion: "engine-v1"}}},
		}},
	}
	if err := mem.Put(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	code, out := hs.do(t, "GET", "/v1/cases/case_old", "")
	if code != http.StatusOK {
		t.Fatalf("status %d: %v", code, out)
	}
	pred := out["prediction"].(map[string]any)
	if pred["calibrated_confidence"] != 0.6 || pred["calibration"] != "cal-v2" || pred["confidence"] != 0.85 {
		t.Errorf("prediction = %v", pred)
	}
	plans := out["plan"].(map[string]any)["plans"].([]any)
	if got := plans[0].(map[string]any)["prediction"].(map[string]any)["calibrated_confidence"]; got != 0.99 {
		t.Errorf("plan calibrated = %v, want 0.99", got)
	}
	if got := plans[1].(map[string]any)["prediction"].(map[string]any)["calibrated_confidence"]; got != nil {
		t.Errorf("other model got a calibrated value %v", got)
	}
	_, list := hs.do(t, "GET", "/v1/cases", "")
	row := list["cases"].([]any)[0].(map[string]any)
	if row["calibrated_confidence"] != 0.6 {
		t.Errorf("list row = %v", row)
	}
	stored, err := mem.Get(context.Background(), "case_old")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Prediction.CalibratedConfidence != nil || stored.Plan.Plans[0].Prediction.CalibratedConfidence != nil {
		t.Error("the stored case was changed")
	}
}

// D251 (#435): the plan search and the country comparison calibrate their
// predictions when they run, so the JSON response, every streamed event
// that carries a plan and the compare response show the same calibrated
// value that GET /v1/cases/{id} shows for the same plan, and the stored
// result carries it too.
func TestPlanAndCompareResponsesCarryCalibration(t *testing.T) {
	m := calibrate.Must()
	mem := store.NewMemory()
	hs := realPlannerHarness(t, func(d *Deps) {
		d.Cases = mem
		d.Calibration, d.CalibratedFastPath = &m, m.Bar
	})
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	pred := func(v any) map[string]any { return v.(map[string]any)["prediction"].(map[string]any) }
	// want checks a plan prediction: calibrated, and as the map gives it.
	want := func(where string, p map[string]any) {
		t.Helper()
		stated, _ := p["confidence"].(float64)
		cal, ok := p["calibrated_confidence"].(float64)
		if p["origin_country"] == "" || !ok || p["calibration"] != m.Version || cal != m.Value(stated) {
			t.Errorf("%s: prediction %v, want calibrated %v (%s)", where, p, m.Value(stated), m.Version)
		}
	}
	readPlans := func() []any {
		t.Helper()
		_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
		return got["plan"].(map[string]any)["plans"].([]any)
	}

	code, out := hs.do(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand"}`)
	if code != http.StatusOK {
		t.Fatalf("plan %d: %v", code, out)
	}
	plans := out["plan"].(map[string]any)["plans"].([]any)
	if len(plans) == 0 {
		t.Fatal("no plan shown")
	}
	read := readPlans()
	for i := range plans {
		want("POST /plan plans["+strconv.Itoa(i)+"]", pred(plans[i]))
		if a, b := pred(plans[i])["calibrated_confidence"], pred(read[i])["calibrated_confidence"]; a != b {
			t.Errorf("plan %d: response calibrated %v, read %v", i, a, b)
		}
	}
	want("POST /plan baseline", pred(out["plan"].(map[string]any)["baseline"]))

	req := httptest.NewRequest("POST", "/v1/cases/"+id+"/plan", strings.NewReader(`{"target":"Thailand"}`))
	req.Header.Set("Accept", "text/event-stream")
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	seen := map[string]int{}
	for _, blk := range strings.Split(rec.Body.String(), "\n\n") {
		var name, data string
		for _, ln := range strings.Split(blk, "\n") {
			if v, ok := strings.CutPrefix(ln, "event: "); ok {
				name = v
			} else if v, ok := strings.CutPrefix(ln, "data: "); ok {
				data = v
			}
		}
		if name != "baseline" && name != "move_done" && name != "done" {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		p, _ := ev["plan"].(map[string]any)
		switch name {
		case "done":
			for i, sp := range p["plans"].([]any) {
				want("stream done plans["+strconv.Itoa(i)+"]", pred(sp))
				if a, b := pred(sp)["calibrated_confidence"], pred(readPlans()[i])["calibrated_confidence"]; a != b {
					t.Errorf("stream plan %d: calibrated %v, read %v", i, a, b)
				}
			}
		default:
			if p != nil && pred(p)["origin_country"] != "" {
				want("stream "+name, pred(p))
			}
		}
		seen[name]++
	}
	if seen["baseline"] != 1 || seen["move_done"] == 0 || seen["done"] != 1 {
		t.Fatalf("stream events %v:\n%s", seen, rec.Body.String())
	}

	code, out = hs.do(t, "POST", "/v1/cases/"+id+"/compare-targets", `{"targets":["Thailand","Vietnam"]}`)
	if code != http.StatusOK {
		t.Fatalf("compare %d: %v", code, out)
	}
	_, got := hs.do(t, "GET", "/v1/cases/"+id, "")
	ranked := got["targets"].(map[string]any)["ranked"].([]any)
	n := 0
	for i, r := range out["targets"].([]any) {
		best, _ := r.(map[string]any)["best"].(map[string]any)
		if best == nil {
			continue
		}
		n++
		want("compare targets["+strconv.Itoa(i)+"].best", pred(best))
		if a, b := pred(best)["calibrated_confidence"], pred(ranked[i].(map[string]any)["best"])["calibrated_confidence"]; a != b {
			t.Errorf("compare %d: response calibrated %v, read %v", i, a, b)
		}
	}
	if n == 0 {
		t.Fatal("the comparison has no best plan to check")
	}

	stored, err := mem.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Plan.Plans[0].Prediction.CalibratedConfidence == nil || stored.Targets.Ranked[0].Best.Prediction.CalibratedConfidence == nil {
		t.Error("the stored plan or comparison has no calibrated value")
	}
}
