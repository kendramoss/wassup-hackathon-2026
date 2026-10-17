package planner

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/nearmiss"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/verifier"
)

// allThaiEngine says Thailand for every move (China for the current setup).
type allThaiEngine struct{}

func (allThaiEngine) Predict(_ context.Context, c domain.Case) (engine.Result, error) {
	origin := "Thailand"
	if strings.HasSuffix(c.ID, "/baseline") {
		origin = "China"
	}
	return engine.Result{Prediction: domain.Prediction{Origin: origin, Confidence: 0.95}}, nil
}

// confByMove gives each move a supported verdict whose confidence depends on
// which groups it moves, so moves land above, inside and under the band.
type confByMove struct{}

func (confByMove) Verify(_ context.Context, t verifier.Task) (verifier.Result, error) {
	c := 0.70
	switch {
	case strings.Contains(t.FactsB, "In Thailand, the electric motor, fan and motor support will be produced in-house") &&
		strings.Contains(t.FactsB, "In Thailand, the printed circuit board assembly (PCBA) will be produced in-house"):
		c = 0.97 // shown
	case strings.Contains(t.FactsB, "In Thailand, the electric motor, fan and motor support will be produced in-house"):
		c = 0.90
	case strings.Contains(t.FactsB, "In Thailand, the printed circuit board assembly (PCBA) will be produced in-house"):
		c = 0.88
	case strings.Contains(t.FactsB, "In Thailand, the main housing"):
		c = 0.86
	}
	return verifier.Result{Verdict: domain.Verdict{PrecedentSupported: true, SupportingRuling: "N325810", Confidence: c}}, nil
}

type fakeAdvisor struct {
	mu    sync.Mutex
	tasks []nearmiss.Task
	fail  bool
}

func (f *fakeAdvisor) Advise(_ context.Context, t nearmiss.Task) (domain.NearMiss, error) {
	f.mu.Lock()
	f.tasks = append(f.tasks, t)
	f.mu.Unlock()
	if f.fail {
		return domain.NearMiss{}, context.DeadlineExceeded
	}
	return domain.NearMiss{Difference: "differs", WouldMatch: "move it", Ruling: t.Ruling,
		Trace: domain.Trace{PromptVersion: nearmiss.PromptVersion, InputTokens: 1000, OutputTokens: 60}}, nil
}

func planWith(t *testing.T, adv nearmiss.Adviser) Result {
	t.Helper()
	r, _ := fixture(t, "heatgun_2022.json")
	tmpls, _ := LoadTemplates()
	res, err := New(tmpls, allThaiEngine{}, confByMove{}, Options{PlanConfidence: 0.92, Concurrency: 3, NearMiss: adv}).Plan(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func verdicts(res Result) map[string]string {
	out := map[string]string{}
	for _, pl := range append(append([]domain.Plan{}, res.Plans...), res.Rejected...) {
		if pl.Verdict != nil {
			out[pl.Move.Key()] = pl.Description + "|" + string(pl.RejectionKind) + "|" + strings.Repeat("s", btoi(pl.Shown))
		}
	}
	return out
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestNearMissAdvisesTheTwoClosestInTheBand(t *testing.T) {
	adv := &fakeAdvisor{}
	res := planWith(t, adv)
	if len(adv.tasks) != 2 {
		t.Fatalf("advice calls = %d, want 2 (the cap)", len(adv.tasks))
	}
	var advised []float64
	for _, pl := range res.Rejected {
		if pl.NearMiss != nil {
			if pl.RejectionKind != domain.RejectBelowConfidence || pl.NearMiss.Ruling != "N325810" {
				t.Errorf("advice on %+v", pl)
			}
			advised = append(advised, pl.Verdict.Confidence)
		}
	}
	// The two highest checks in [0.85, 0.92) get advice; lower ones in the
	// band are over the cap, 0.70 is under the floor, 0.97 is shown.
	var band []float64
	for _, pl := range res.Rejected {
		if pl.Verdict != nil && pl.Verdict.Confidence >= 0.85 && pl.Verdict.Confidence < 0.92 {
			band = append(band, pl.Verdict.Confidence)
		}
	}
	slices.Sort(band)
	slices.Reverse(band)
	slices.Sort(advised)
	slices.Reverse(advised)
	if len(band) < 3 || len(advised) != 2 || advised[0] != band[0] || advised[1] != band[1] {
		t.Errorf("advised %v, want the top two of the band %v (the band needs more than two to test the cap)", advised, band)
	}
	for _, pl := range res.Plans {
		if pl.NearMiss != nil {
			t.Errorf("a shown plan got advice: %+v", pl)
		}
	}
	for _, tk := range adv.tasks {
		if tk.Before.Format("2006-01-02") != "2026-01-14" || len(tk.Exclude) != 2 || tk.Facts == "" {
			t.Errorf("advice task = %+v", tk)
		}
	}
	if res.Stats.NearMissCalls != 2 || res.Stats.NearMissInTok != 2000 {
		t.Errorf("stats = %+v", res.Stats)
	}
}

func TestNearMissNeverChangesVerdictsOrShown(t *testing.T) {
	without := planWith(t, nil)
	with := planWith(t, &fakeAdvisor{})
	failing := planWith(t, &fakeAdvisor{fail: true})
	a, b, c := verdicts(without), verdicts(with), verdicts(failing)
	if len(a) == 0 || len(a) != len(b) || len(a) != len(c) {
		t.Fatalf("verdict counts %d %d %d", len(a), len(b), len(c))
	}
	for k, v := range a {
		if b[k] != v || c[k] != v {
			t.Errorf("%s: without %q, with %q, failing %q", k, v, b[k], c[k])
		}
	}
	if len(without.Plans) != len(with.Plans) {
		t.Errorf("shown plans changed: %d vs %d", len(without.Plans), len(with.Plans))
	}
	for _, res := range []Result{without, failing} {
		for _, pl := range res.Rejected {
			if pl.NearMiss != nil {
				t.Errorf("unexpected advice: %+v", pl.NearMiss)
			}
		}
	}
	if failing.Stats.NearMissCalls != 0 {
		t.Errorf("failed advice counted: %+v", failing.Stats)
	}
}

func TestNearMissOffByDefault(t *testing.T) {
	res := planWith(t, nil)
	for _, pl := range res.Rejected {
		if pl.NearMiss != nil {
			t.Fatalf("advice without an advisor: %+v", pl)
		}
	}
	if res.Stats.NearMissCalls != 0 {
		t.Errorf("stats = %+v", res.Stats)
	}
}
