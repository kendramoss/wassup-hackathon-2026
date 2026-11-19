package planner

import (
	"context"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/verifier"
)

// originVerifier answers every task as supported with fixed origins.
type originVerifier struct{ a, b string }

func (o originVerifier) Verify(_ context.Context, t verifier.Task) (verifier.Result, error) {
	return verifier.Result{OriginA: o.a, OriginB: o.b, ChangesOrigin: true,
		Verdict: domain.Verdict{PrecedentSupported: true, SupportingRuling: "N350001", Confidence: 0.99},
		Proof:   verifier.ProofFor(t, "N350001")}, nil
}

// The planner must not promote a supported verdict whose origins do not show a
// change (equivalent origins, or origin_a contradicting the case's known
// China holding), but still shows a real China -> Thailand verdict.
func TestPlannerDoesNotPromoteOriginInconsistentVerdict(t *testing.T) {
	r, _ := fixture(t, "heatgun_2022.json")
	tmpls, _ := LoadTemplates()
	for name, c := range map[string]struct {
		a, b  string
		shown int
	}{
		"equivalent aliases": {"USA", "United States of America", 0},
		"known baseline":     {"Vietnam", "Thailand", 0},
		"distinct":           {"PRC", "Thailand", 1},
	} {
		cache, err := verifier.NewCache(originVerifier{c.a, c.b}, t.TempDir(), "m")
		if err != nil {
			t.Fatal(err)
		}
		res, err := New(tmpls, &fakeEngine{}, cache, Options{PlanConfidence: 0.95, Concurrency: 2}).Plan(context.Background(), r)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Plans) != c.shown {
			t.Errorf("%s: shown plans = %d, want %d", name, len(res.Plans), c.shown)
		}
	}
}
