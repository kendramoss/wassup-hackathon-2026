package tariffwatch

import (
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/duty"
)

// A defect found on the development cases of the legal-rules challenge study
// (eval/challenges/legal-rules). It failed before the fix.

// Validate accepts a dated action rate up to 500 percent, but the additional
// duty index the evaluation builds from the same action refused any rate above
// 100 percent, so a data set that loaded (or that Revise produced from a
// heading reading "+ 120%") could not be evaluated.
func TestDatedActionAbove100PercentIsEvaluated(t *testing.T) {
	for _, rate := range []float64{1.2, 1.5} {
		ds := embedded(t)
		ds.Actions = []Action{{Kind: KindSection301, Heading: "9903.91.12", Name: "synthetic", Origin: "China", Rate: rate,
			EffectiveFrom: "2027-01-10", Subheadings: []string{"8716.90.30"}}}
		if err := ds.Validate(); err != nil {
			t.Fatalf("rate %v: %v", rate, err)
		}
		ev, err := NewEvaluator(ds)
		if err != nil {
			t.Fatal(err)
		}
		mfn := 0.0
		ob, err := ev.Evaluate("8716.90.30", "China", &mfn, day("2027-02-01"))
		if err != nil {
			t.Fatalf("rate %v: %v", rate, err)
		}
		var got *Component
		for i := range ob.Components {
			if ob.Components[i].Heading == "9903.91.12" {
				got = &ob.Components[i]
			}
		}
		if got == nil || got.Rate != rate {
			t.Fatalf("rate %v: component %+v, want the action's own rate", rate, got)
		}
		sum := 0.0
		for _, c := range ob.Components {
			sum += c.Rate
		}
		if d := ob.Rate - sum; d > 1e-9 || d < -1e-9 {
			t.Fatalf("rate %v: total %v is not the sum of the components %v", rate, ob.Rate, sum)
		}
		before, err := ev.Evaluate("8716.90.30", "China", &mfn, day("2027-01-09"))
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range before.Components {
			if c.Heading == "9903.91.12" {
				t.Fatalf("the action is not in force on 2027-01-09: %+v", c)
			}
		}
	}
}

// Review round 1: an action above 100 percent that reuses the heading of an
// embedded undated list must not overwrite that list's component while the
// action is not in force, and the exclusion and plan paths must carry the
// action's own rate.
func embeddedLine(t *testing.T) (heading, hts string, rate float64) {
	t.Helper()
	f := embedded(t)
	l := f.Additional.Lists[0]
	return l.Heading, l.Subheadings[0], l.Rate
}

func component(ob OriginBasis, heading string) *Component {
	for i := range ob.Components {
		if ob.Components[i].Heading == heading {
			return &ob.Components[i]
		}
	}
	return nil
}

func TestFutureOrExpiredActionDoesNotOverwriteAnEmbeddedListComponent(t *testing.T) {
	heading, hts, listRate := embeddedLine(t)
	for _, c := range []struct{ name, from, through, asOf string }{
		{"not yet in force", "2030-01-01", "", "2026-11-01"},
		{"expired", "2020-01-01", "2021-12-31", "2026-11-01"},
	} {
		ds := embedded(t)
		ds.Actions = []Action{{Kind: KindSection301, Heading: heading, Name: "synthetic", Origin: "China", Rate: 1.5,
			EffectiveFrom: c.from, EffectiveThrough: c.through, Subheadings: []string{hts}}}
		ev, err := NewEvaluator(ds)
		if err != nil {
			t.Fatal(err)
		}
		mfn := 0.0
		ob, err := ev.Evaluate(hts, "China", &mfn, day(c.asOf))
		if err != nil {
			t.Fatal(err)
		}
		got := component(ob, heading)
		if got == nil || got.Rate != listRate || got.From != "" || got.Through != "" {
			t.Fatalf("%s: component %+v, want the undated list at %v", c.name, got, listRate)
		}
	}
}

func TestActionAbove100PercentWithExclusionsAndPlan(t *testing.T) {
	const heading = "9903.88.97"
	hts := "8716.90.30"
	mk := func(excl ...duty.Exclusion) *Evaluator {
		ds := embedded(t)
		ds.Additional.Country, ds.Additional.Lists = nil, nil
		ds.Actions = []Action{{Kind: KindSection301, Heading: heading, Name: "synthetic", Origin: "China", Rate: 1.5,
			EffectiveFrom: "2026-06-01", EffectiveThrough: "2026-12-31", Subheadings: []string{hts}}}
		ds.Additional.Exclusions = append(ds.Additional.Exclusions, excl...)
		ev, err := NewEvaluator(ds)
		if err != nil {
			t.Fatal(err)
		}
		return ev
	}
	mfn := 0.0
	// active, not yet active, expired
	for _, c := range []struct {
		asOf string
		want bool
	}{{"2026-11-01", true}, {"2026-05-31", false}, {"2027-01-01", false}} {
		ob, err := mk().Evaluate(hts, "China", &mfn, day(c.asOf))
		if err != nil {
			t.Fatal(err)
		}
		got := component(ob, heading)
		if c.want != (got != nil) || (got != nil && got.Rate != 1.5) {
			t.Fatalf("as of %s: %+v, want present=%v at 1.5", c.asOf, got, c.want)
		}
	}
	// a whole-subheading exclusion removes the duty while valid, and lapses back to 150 percent
	ex := duty.Exclusion{Heading: heading, HTS: hts, WholeSubheading: true, EndDate: "2026-10-31"}
	if ob, _ := mk(ex).Evaluate(hts, "China", &mfn, day("2026-10-31")); component(ob, heading) != nil {
		t.Fatal("a valid whole-subheading exclusion must remove the duty")
	}
	ob, _ := mk(ex).Evaluate(hts, "China", &mfn, day("2026-11-01"))
	if got := component(ob, heading); got == nil || got.Rate != 1.5 {
		t.Fatalf("lapsed exclusion: %+v, want 150 percent", got)
	}
	// plan effect: moving from China to Vietnam saves the whole 150 points
	old := Basis{HTS: hts, HTSRevision: "x", PricedAt: day("2026-05-01"), AsOf: "2026-05-01", AnnualImportUSD: 1_000_000, Origins: []OriginBasis{
		{Origin: "China", Rate: 0, Source: "table", Components: []Component{{Kind: KindMFN, Heading: "8716.90.30", Rate: 0}}},
		{Origin: "Vietnam", Rate: 0, Source: "table", Components: []Component{{Kind: KindMFN, Heading: "8716.90.30", Rate: 0}}}}}
	mfnp := 0.0
	old.Origins[0].MFN, old.Origins[1].MFN = &mfnp, &mfnp
	res, err := mk().Watch(CaseInput{Basis: old, BaseOrigin: "China", Plan: &PlanInput{To: "Vietnam", DutyEffectYear: 0}}, day("2026-11-01"), day("2026-11-28"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Annual == nil || res.Annual.Change != 1_500_000 || res.Plan == nil || res.Plan.Change != 1_500_000 {
		t.Fatalf("annual %+v plan %+v", res.Annual, res.Plan)
	}
}

// Review round 2: component identity. An in-force dated action that reuses the
// heading of an undated list supersedes that list only for the lines and
// origins the action covers; elsewhere the list stays, and only the component
// that came from the action carries its dates.
func collisionSet(t *testing.T) (ds *Dataset, heading, hts, other string) {
	t.Helper()
	ds = embedded(t)
	ds.Additional.Country, ds.Additional.Exclusions = nil, nil
	l := ds.Additional.Lists[0]
	return ds, l.Heading, l.Subheadings[0], l.Subheadings[1]
}

func collisionAction(heading, origin string, subs ...string) Action {
	return Action{Kind: KindSection301, Heading: heading, Name: "synthetic replacement", Origin: origin, Rate: 1.5,
		EffectiveFrom: "2026-01-01", EffectiveThrough: "2026-12-31", Subheadings: subs}
}

func eval(t *testing.T, ds *Dataset, hts, origin, asOf string) OriginBasis {
	t.Helper()
	ev, err := NewEvaluator(ds)
	if err != nil {
		t.Fatal(err)
	}
	mfn := 0.0
	ob, err := ev.Evaluate(hts, origin, &mfn, day(asOf))
	if err != nil {
		t.Fatal(err)
	}
	return ob
}

func only301(t *testing.T, ob OriginBasis, heading string, rate float64, dated bool) {
	t.Helper()
	n := 0
	for _, c := range ob.Components {
		if c.Heading == heading {
			n++
			if c.Rate != rate || (c.From != "") != dated {
				t.Fatalf("component %+v, want rate %v dated=%v", c, rate, dated)
			}
		}
	}
	if n != 1 {
		t.Fatalf("%d components for %s, want exactly 1: %+v", n, heading, ob.Components)
	}
	if ob.Rate != rate {
		t.Fatalf("total %v, want %v", ob.Rate, rate)
	}
}

func TestInForceActionSupersedesTheListOnlyWhereItCovers(t *testing.T) {
	ds, heading, hts, other := collisionSet(t)
	base := ds.Additional.Lists[0].Rate
	ds.Actions = []Action{collisionAction(heading, "China", hts)}
	only301(t, eval(t, ds, hts, "China", "2026-11-01"), heading, 1.5, true)     // covered line: replaced, not added
	only301(t, eval(t, ds, other, "China", "2026-11-01"), heading, base, false) // uncovered line keeps the baseline, no dates
	only301(t, eval(t, ds, hts, "China", "2027-01-01"), heading, base, false)   // outside the window the baseline is back
	only301(t, eval(t, ds, hts, "China", "2025-12-31"), heading, base, false)
	// the action names another origin: China keeps the list, undated
	ds.Actions = []Action{collisionAction(heading, "Vietnam", hts)}
	only301(t, eval(t, ds, hts, "China", "2026-11-01"), heading, base, false)
	only301(t, eval(t, ds, hts, "Vietnam", "2026-11-01"), heading, 1.5, true)
}

func TestSupersedingActionWithExclusionsAndPlan(t *testing.T) {
	ds, heading, hts, _ := collisionSet(t)
	base := ds.Additional.Lists[0].Rate
	ds.Actions = []Action{collisionAction(heading, "China", hts)}
	ex := duty.Exclusion{Heading: heading, HTS: hts, WholeSubheading: true, EndDate: "2026-10-31"}
	ds.Additional.Exclusions = []duty.Exclusion{ex}
	if ob := eval(t, ds, hts, "China", "2026-10-31"); component(ob, heading) != nil {
		t.Fatalf("valid whole-subheading exclusion must remove the duty: %+v", ob.Components)
	}
	only301(t, eval(t, ds, hts, "China", "2026-11-01"), heading, 1.5, true) // lapsed: the action's rate once
	ds.Additional.Exclusions = nil
	ev, err := NewEvaluator(ds)
	if err != nil {
		t.Fatal(err)
	}
	m := 0.0
	old := Basis{HTS: hts, HTSRevision: "x", PricedAt: day("2025-12-01"), AsOf: "2025-12-01", AnnualImportUSD: 1_000_000, Origins: []OriginBasis{
		{Origin: "China", Rate: base, Source: "table", MFN: &m, Components: []Component{{Kind: KindMFN, Heading: hts, Rate: 0}, {Kind: KindSection301, Heading: heading, Rate: base}}},
		{Origin: "Vietnam", Rate: 0, Source: "table", MFN: &m, Components: []Component{{Kind: KindMFN, Heading: hts, Rate: 0}}}}}
	res, err := ev.Watch(CaseInput{Basis: old, BaseOrigin: "China", Plan: &PlanInput{To: "Vietnam"}}, day("2026-11-01"), day("2026-11-28"))
	if err != nil {
		t.Fatal(err)
	}
	want := 1_000_000 * (1.5 - base)
	if res.Annual == nil || res.Annual.Change != want || res.Plan == nil || res.Plan.Change != want {
		t.Fatalf("annual %+v plan %+v, want change %v (150%% replaces %v%%, not added to it)", res.Annual, res.Plan, want, base*100)
	}
}

func TestOverlappingActionsOnTheSameHeadingAreRefused(t *testing.T) {
	ds, heading, hts, other := collisionSet(t)
	a := collisionAction(heading, "China", hts, other)
	b := collisionAction(heading, "China", other)
	b.Rate = 1.2
	ds.Actions = []Action{a, b}
	if err := ds.Validate(); err == nil {
		t.Fatal("two in-force actions on the same heading and line must be refused, not summed or picked")
	}
	b.EffectiveFrom, b.EffectiveThrough = "2027-01-01", "2027-12-31" // successive windows are fine
	ds.Actions = []Action{a, b}
	if err := ds.Validate(); err != nil {
		t.Fatalf("successive windows: %v", err)
	}
	b.EffectiveFrom, b.EffectiveThrough = "2026-01-01", "2026-12-31"
	b.Subheadings = []string{"8716.90.30"} // disjoint lines are fine
	ds.Actions = []Action{a, b}
	if err := ds.Validate(); err != nil {
		t.Fatalf("disjoint lines: %v", err)
	}
	b.Subheadings, b.Origin = []string{other}, "Vietnam" // disjoint origins are fine
	ds.Actions = []Action{a, b}
	if err := ds.Validate(); err != nil {
		t.Fatalf("disjoint origins: %v", err)
	}
}
