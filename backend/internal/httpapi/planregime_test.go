package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/router"
)

// D540: a plan search or country comparison on a case routed to Part 102 asks
// the planner for the Part 102 regime, so the Part 102 engine predicts every
// move; every other case asks for none (the general engine), as before.
func TestPlanSearchAsksForThePart102Engine(t *testing.T) {
	for regime, want := range map[string]string{
		router.RegimeUSMCA:   router.RegimeUSMCA,
		RegimeGeneral:        "",
		router.RegimeTextile: "",
		router.RegimePharma:  "",
		"unclear":            "",
	} {
		t.Run(regime, func(t *testing.T) {
			hs := regimeHarness(t, regime, traceEngine{version: engine.PromptVersion, origin: "China"},
				traceEngine{version: engine.USMCAVersion, origin: "China"})
			code, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
			if code != http.StatusCreated {
				t.Fatalf("predict = %d %v", code, c)
			}
			id := c["id"].(string)
			if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand"}`); code != http.StatusOK {
				t.Fatalf("plan = %d %v", code, out)
			}
			if got := hs.planner.got.Regime; got != want {
				t.Errorf("plan search regime = %q, want %q", got, want)
			}
			if code, out := hs.do(t, "POST", "/v1/cases/"+id+"/compare-targets", `{"targets":["Thailand","Vietnam"]}`); code != http.StatusOK {
				t.Fatalf("compare = %d %v", code, out)
			}
			if got := hs.planner.got.Regime; got != want {
				t.Errorf("comparison regime = %q, want %q", got, want)
			}
		})
	}
}

// With the Part 102 engine predicting the plan, the plan draft is asked under
// the Part 102 rules as a Part 102 determination: the engine's own note, not
// the D520 "made by Unde's general engine" note.
func TestPart102PlanDraftComesFromThePart102Engine(t *testing.T) {
	hs := newHarnessWith(t, router.RegimeUSMCA, func(d *Deps) {
		d.ExportCFR177 = true
		d.Engine = traceEngine{version: engine.PromptVersion, origin: "China"}
		d.USMCA = traceEngine{version: engine.USMCAVersion, origin: "China"}
		d.Planner = enginePlanner{fakePlanner: &fakePlanner{}, part102: true}
	})
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	hs.do(t, "POST", "/v1/cases/"+id+"/plan", `{"target":"Thailand"}`)
	for _, format := range []string{"markdown", "cfr177"} {
		code, out := hs.do(t, "POST", "/v1/cases/"+id+"/export", `{"plan_index":0,"format":"`+format+`","preview":true}`)
		if code != http.StatusOK {
			t.Fatalf("%s plan draft = %d %v", format, code, out)
		}
		letter, notes := out["content"].(string), joinNotes(out)
		for _, w := range append(wantUSMCA.has, "product of Thailand") {
			if !strings.Contains(letter, w) {
				t.Errorf("%s letter missing %q", format, w)
			}
		}
		for _, w := range wantUSMCA.not {
			if strings.Contains(letter, w) {
				t.Errorf("%s letter has %q", format, w)
			}
		}
		if !strings.Contains(notes, wantUSMCA.note) {
			t.Errorf("%s notes missing the Part 102 engine's note: %s", format, notes)
		}
		for _, bad := range []string{"Legal framework not confirmed", "general engine"} {
			if strings.Contains(notes, bad) {
				t.Errorf("%s notes say %q: %s", format, bad, notes)
			}
		}
	}
}
