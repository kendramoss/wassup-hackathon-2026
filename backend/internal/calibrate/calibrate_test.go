package calibrate

import (
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

func TestEmbeddedMapLoads(t *testing.T) {
	m, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if m.Version != "cal-v2" || m.Model != "gemini-4-argon" || m.EnginePrompt != "engine-v1" {
		t.Fatalf("map identity = %s %s %s", m.Version, m.Model, m.EnginePrompt)
	}
	if m.Bar != config.DefaultCalibratedFastPath {
		t.Fatalf("map bar %v, config default %v: regenerate with build_accuracy.py or update DefaultCalibratedFastPath", m.Bar, config.DefaultCalibratedFastPath)
	}
}

// The step rule must match eval/charts/build_accuracy.py isotonic_apply.
func TestValueSteps(t *testing.T) {
	m := Must()
	cases := map[float64]float64{
		0.10: 0.6, 0.65: 0.6, 0.85: 0.6, 0.89: 0.6,
		0.90: 0.7778, 0.91: 0.7778,
		0.92: 0.9, 0.94: 0.9,
		0.95: 0.9016, 0.96: 0.9355, 0.97: 0.9355,
		0.98: 0.99, 0.99: 0.99, 1.0: 0.99,
	}
	for in, want := range cases {
		if got := m.Value(in); got != want {
			t.Errorf("Value(%v) = %v, want %v", in, got, want)
		}
	}
}

// On this map the calibrated bar accepts exactly what stated 0.92 did.
func TestBarMatchesStated092(t *testing.T) {
	m := Must()
	for c := 0.0; c <= 1.0001; c += 0.01 {
		if (m.Value(c) >= m.Bar) != (c >= 0.92-1e-9) {
			t.Fatalf("stated %.2f: calibrated %v vs bar %v disagrees with the 0.92 bar", c, m.Value(c), m.Bar)
		}
	}
}

func TestAppliesOnlyToFittedIdentity(t *testing.T) {
	m := Must()
	base := domain.Prediction{Origin: "China", Confidence: 0.96, Trace: domain.Trace{PromptVersion: "engine-v1", RequestedModel: "gemini-4-argon", ModelID: "gemini-4-argon-001"}}
	got := m.Apply(base)
	if got.CalibratedConfidence == nil || *got.CalibratedConfidence != 0.9355 || got.Calibration != "cal-v2" {
		t.Fatalf("primary engine-v1: %+v", got)
	}
	for name, p := range map[string]domain.Prediction{
		"fallback model": {Confidence: 0.96, Trace: domain.Trace{PromptVersion: "engine-v1", RequestedModel: "gemini-3.8-flash"}},
		"textile":        {Confidence: 0.96, Trace: domain.Trace{PromptVersion: "textile-v2", RequestedModel: "gemini-4-argon"}},
		"rules":          {Confidence: 0.96, Trace: domain.Trace{PromptVersion: "engine-v1+rules-v1", RequestedModel: "gemini-4-argon"}},
		"voted":          {Confidence: 0.96, VoteAgreement: "3/5", Trace: domain.Trace{PromptVersion: "engine-v1", RequestedModel: "gemini-4-argon"}},
	} {
		v := 0.5
		p.CalibratedConfidence, p.Calibration = &v, "stale"
		if got := m.Apply(p); got.CalibratedConfidence != nil || got.Calibration != "" {
			t.Errorf("%s: calibrated %v %q, want none", name, got.CalibratedConfidence, got.Calibration)
		}
	}
}

func TestParseRejectsBrokenMaps(t *testing.T) {
	for name, b := range map[string]string{
		"decreasing": `{"version":"x","engine_prompt":"e","model":"m","bar":0.9,"steps":[{"from":0.5,"calibrated":0.8},{"from":0.9,"calibrated":0.7}]}`,
		"unsorted":   `{"version":"x","engine_prompt":"e","model":"m","bar":0.9,"steps":[{"from":0.9,"calibrated":0.7},{"from":0.5,"calibrated":0.8}]}`,
		"no bar":     `{"version":"x","engine_prompt":"e","model":"m","steps":[{"from":0.5,"calibrated":0.8}]}`,
		"no steps":   `{"version":"x","engine_prompt":"e","model":"m","bar":0.9,"steps":[]}`,
	} {
		if _, err := Parse([]byte(b)); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
}

func TestEvaluate(t *testing.T) {
	m := Must()
	// Two rows at stated 0.98 (calibrated 0.99), one right one wrong; one
	// row at 0.90 (calibrated 0.7778), right.
	r := m.Evaluate([]Scored{{0.98, true}, {0.98, false}, {0.90, true}})
	if r.N != 3 || r.Accuracy != 2.0/3 {
		t.Fatalf("n/accuracy = %d/%v", r.N, r.Accuracy)
	}
	// stated: bin 0.98 |0.5-0.98|*2 = 0.96, bin 0.90 |1-0.9| = 0.1 -> 1.06/3
	if d := r.ECEStated - 1.06/3; d > 1e-9 || d < -1e-9 {
		t.Errorf("ECE stated = %v", r.ECEStated)
	}
	// calibrated: |0.5-0.99|*2 = 0.98, |1-0.7778| = 0.2222 -> 1.2022/3
	if d := r.ECECalibrated - 1.2022/3; d > 1e-9 || d < -1e-9 {
		t.Errorf("ECE calibrated = %v", r.ECECalibrated)
	}
}

// D262 / #493: the map's cap ties three stated levels into one. The own-bin
// ECE pair therefore compares 3 bins with 1; scoring the stated values on the
// calibrated bin gives the same error as the calibrated value, which isolates
// the bin-merging part of the drop.
func TestEvaluateSameBinPartition(t *testing.T) {
	m := Must()
	r := m.Evaluate([]Scored{{0.98, true}, {0.99, true}, {1.0, false}})
	if r.StatedLevels != 3 || r.CalibratedLevels != 1 {
		t.Fatalf("levels = %d stated, %d calibrated; want 3, 1", r.StatedLevels, r.CalibratedLevels)
	}
	// One calibrated bin at 0.99 with 2/3 right: |2/3 - 0.99| = 97/300.
	if d := r.ECEStatedOnCalibratedBins - 97.0/300; d > 1e-9 || d < -1e-9 {
		t.Errorf("same-bin ECE = %v, want %v", r.ECEStatedOnCalibratedBins, 97.0/300)
	}
	if r.ECEStatedOnCalibratedBins >= r.ECEStated {
		t.Errorf("same-bin ECE %v should be below own-bin stated ECE %v", r.ECEStatedOnCalibratedBins, r.ECEStated)
	}
}
