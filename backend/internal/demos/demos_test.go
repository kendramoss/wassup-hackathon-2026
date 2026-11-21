package demos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/calibrate"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

func TestDemosAreValidRequests(t *testing.T) {
	ds, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(ds) != 10 {
		t.Fatalf("demos = %d, want 10", len(ds))
	}
	for _, d := range ds {
		var p struct {
			Date    string          `json:"date"`
			Facts   string          `json:"facts"`
			Product *domain.Product `json:"product"`
			Family  string          `json:"family"`
			Exclude []string        `json:"exclude_rulings"`
		}
		if err := json.Unmarshal(d.Predict, &p); err != nil {
			t.Fatalf("%s: %v", d.ID, err)
		}
		if p.Date == "" || (p.Facts == "" && p.Product == nil) || len(p.Exclude) == 0 || d.CBPHeld == "" {
			t.Errorf("%s: incomplete predict request %+v", d.ID, p)
		}
		if len([]rune(p.Facts)) > domain.MaxFactsRunes {
			t.Errorf("%s: facts too long", d.ID)
		}
		if p.Product != nil {
			if err := p.Product.Validate(); err != nil {
				t.Errorf("%s: %v", d.ID, err)
			}
		}
	}
	hg := ds[0]
	if hg.ID != "heatgun-2022" || !strings.Contains(string(hg.Predict), "Each heat gun is assembled in Thailand using 24 components") {
		t.Errorf("heat gun demo should predict from CBP's 2022 facts text: %s", hg.ID)
	}
	var tl struct {
		Exclude []string `json:"exclude_rulings"`
	}
	if ds[1].ID != "tublight-2025" || json.Unmarshal(ds[1].Predict, &tl) != nil || strings.Join(tl.Exclude, ",") != "N350085,N351143" {
		t.Errorf("tub light demo should exclude both tub-light rulings: %s %v", ds[1].ID, tl.Exclude)
	}
	// A demo from a family added in D138 (judge journey): plannable, and it
	// excludes the ruling its facts come from.
	gg := ds[5]
	var ggp struct {
		Family  string   `json:"family"`
		Exclude []string `json:"exclude_rulings"`
	}
	if gg.ID != "gas-grill-2023" || json.Unmarshal(gg.Predict, &ggp) != nil || ggp.Family != "gas_grills_cooking" || strings.Join(ggp.Exclude, ",") != "N335085" || !Summaries(ds)[5].HasPlan {
		t.Errorf("gas grill demo: %s %+v", gg.ID, ggp)
	}
	if !Summaries(ds)[0].HasPlan || Summaries(ds)[3].HasPlan {
		t.Errorf("summaries = %+v", Summaries(ds))
	}
}

// Each recorded run is traceable to its committed result file (D191): the
// engine's answer, its stated confidence and the first shown plan match the
// file, and the calibrated value is the map's.
func TestRecordedRunsMatchResultFiles(t *testing.T) {
	ds, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	m := calibrate.Must()
	n, disagree := 0, 0
	for _, d := range ds {
		r := d.Recorded
		if r == nil {
			continue
		}
		n++
		b, err := os.ReadFile(filepath.Join("..", "..", "..", r.File))
		if err != nil {
			t.Fatalf("%s: %v", d.ID, err)
		}
		var res struct {
			Baseline struct {
				Prediction struct {
					Origin     string  `json:"origin_country"`
					Confidence float64 `json:"confidence"`
					Trace      struct {
						Prompt string `json:"prompt_version"`
						Model  string `json:"model_id"`
					} `json:"trace"`
				} `json:"prediction"`
			} `json:"baseline"`
			Plans []struct {
				Description string `json:"description"`
				Shown       bool   `json:"shown"`
				Verdict     struct {
					Confidence float64 `json:"confidence"`
					Supporting string  `json:"supporting_ruling"`
				} `json:"verdict"`
			} `json:"plans"`
		}
		if err := json.Unmarshal(b, &res); err != nil {
			t.Fatalf("%s: %v", d.ID, err)
		}
		p := res.Baseline.Prediction
		if p.Origin != r.EngineOrigin || p.Confidence != r.StatedConfidence || p.Trace.Prompt != "engine-v1" || p.Trace.Model != "gemini-4-argon" {
			t.Errorf("%s: recorded %s %.2f, file %s %.2f (%s %s)", d.ID, r.EngineOrigin, r.StatedConfidence, p.Origin, p.Confidence, p.Trace.Prompt, p.Trace.Model)
		}
		if r.AgreesWithCBP != (r.EngineOrigin == r.CBPOrigin) || !strings.HasPrefix(d.CBPHeld, r.CBPOrigin) {
			t.Errorf("%s: agreement flag or CBP origin wrong", d.ID)
		}
		if !r.AgreesWithCBP {
			disagree++
		}
		if r.CalibratedConfidence != m.Value(r.StatedConfidence) || r.Calibration != m.Version {
			t.Errorf("%s: calibrated %v, map gives %v", d.ID, r.CalibratedConfidence, m.Value(r.StatedConfidence))
		}
		for _, pl := range res.Plans {
			if pl.Shown {
				if pl.Description != r.BestPlan || pl.Verdict.Confidence != r.PlanCheck || pl.Verdict.Supporting != r.PlanRuling {
					t.Errorf("%s: first shown plan %q %.2f %s, recorded %q %.2f %s", d.ID, pl.Description, pl.Verdict.Confidence, pl.Verdict.Supporting, r.BestPlan, r.PlanCheck, r.PlanRuling)
				}
				break
			}
		}
	}
	// Four new-family demos plus the gas grill; the selection rule (D191)
	// kept the one where the engine disagreed with CBP.
	if n != 5 || disagree != 1 {
		t.Errorf("recorded runs = %d (disagree %d), want 5 (1)", n, disagree)
	}
}
