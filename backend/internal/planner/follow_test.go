package planner

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/nearmiss"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/verifier"
)

// adviceSays advises every near miss with the same would_match text.
type adviceSays struct{ text string }

func (a adviceSays) Advise(_ context.Context, t nearmiss.Task) (domain.NearMiss, error) {
	return domain.NearMiss{Difference: "differs", WouldMatch: a.text, Ruling: t.Ruling}, nil
}

type fakeMapper struct {
	mu    sync.Mutex
	calls int
	ids   []string
}

func (f *fakeMapper) MapGroups(context.Context, string, []domain.OperationGroup) ([]string, llm.Usage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.ids, llm.Usage{InputTokens: 300, OutputTokens: 10}, nil
}

// countingVerifier is confByMove, recording each task's variant facts.
type countingVerifier struct {
	mu    sync.Mutex
	facts []string
}

func (c *countingVerifier) Verify(ctx context.Context, t verifier.Task) (verifier.Result, error) {
	c.mu.Lock()
	c.facts = append(c.facts, t.FactsB)
	c.mu.Unlock()
	return confByMove{}.Verify(ctx, t)
}

func followPlan(t *testing.T, opts Options) (Result, *countingVerifier, []Event) {
	t.Helper()
	r, _ := fixture(t, "heatgun_2022.json")
	tmpls, _ := LoadTemplates()
	v := &countingVerifier{}
	opts.PlanConfidence, opts.Concurrency = 0.92, 3
	var events []Event
	res, err := New(tmpls, allThaiEngine{}, v, opts).PlanWithProgress(context.Background(), r, func(e Event) { events = append(events, e) })
	if err != nil {
		t.Fatal(err)
	}
	return res, v, events
}

func followed(res Result) []domain.Plan {
	var out []domain.Plan
	for _, pl := range append(append([]domain.Plan{}, res.Plans...), res.Rejected...) {
		if pl.FromAdvice {
			out = append(out, pl)
		}
	}
	return out
}

func TestMatchGroupsFromAdviceText(t *testing.T) {
	r, tm := fixture(t, "heatgun_2022.json")
	var groups []domain.OperationGroup
	for _, g := range tm.Groups {
		og := g.OperationGroup
		og.Components = groupComponents(r.Product, g.ID)
		groups = append(groups, og)
	}
	cases := map[string][]string{
		"Produce the knob, back cover, support ring, switch, and fasteners in Thailand instead of sourcing them from China.": {"switch_cord_minor_parts"},
		"Make the PCBA in Thailand as well.":                      {"pcba"},
		"Move SMT placement and reflow to the Thai plant.":        {"pcba"},
		"Also wind the rotor and build the stator in Thailand.":   {"motor_fan_assembly"},
		"Keep everything as it is, but import from a new vendor.": nil,
	}
	for advice, want := range cases {
		if got := MatchGroups(advice, groups); !reflect.DeepEqual(got, want) {
			t.Errorf("%q -> %v, want %v", advice, got, want)
		}
	}
}

// twoGroups names the PCBA and the switch group, so following it from the
// motor-fan near misses (0.90) gives the three-group move, which is not in
// the search without triples (MaxMoves 0).
const twoGroups = "Make the PCBA and the knob, power cord and switch in Thailand."

func TestFollowAdviceSearchesOneNewMove(t *testing.T) {
	res, v, events := followPlan(t, Options{NearMiss: adviceSays{twoGroups}, FollowAdvice: true})
	f := followed(res)
	if len(f) != 1 {
		t.Fatalf("followed moves = %d, want 1: %+v", len(f), f)
	}
	got := f[0]
	if got.Move.Key() != "motor_fan_assembly@th+pcba@th+switch_cord_minor_parts@th" {
		t.Errorf("followed move = %s", got.Move.Key())
	}
	if got.Verdict == nil || got.Description == "" || got.VariantFacts == "" {
		t.Errorf("followed move not checked like the others: %+v", got)
	}
	if res.Stats.AdviceFollowed != 1 || res.Stats.MovesSearched != 7 || len(v.facts) != 7 {
		t.Errorf("stats = %+v, verifier runs %d", res.Stats, len(v.facts))
	}
	last := events[len(events)-1]
	var started, doneKeys int
	for _, e := range events {
		if e.Kind == EventMoveStarted && e.Move == got.Move.Key() {
			started++
		}
		if e.Kind == EventMoveDone && e.Move == got.Move.Key() {
			doneKeys++
		}
	}
	if started != 1 || doneKeys != 1 || last.Kind != EventDone || last.Done != 7 || last.Total != 7 {
		t.Errorf("events for the followed move: started %d done %d last %+v", started, doneKeys, last)
	}
}

func TestFollowAdviceNeverRechecksASearchedMove(t *testing.T) {
	// Advice naming one more group from a single gives a pair, and every
	// pair is searched: nothing to follow.
	res, _, _ := followPlan(t, Options{NearMiss: adviceSays{"Make the knob, power cord and switch in Thailand."}, FollowAdvice: true})
	if f := followed(res); len(f) != 0 {
		t.Errorf("followed an already-searched move: %s", f[0].Move.Key())
	}
	// With triples in the search, the triple the advice names was searched
	// too: nothing is followed and no move is checked twice.
	res, v, _ := followPlan(t, Options{NearMiss: adviceSays{twoGroups}, FollowAdvice: true, MaxMoves: DefaultMaxMoves})
	if f := followed(res); len(f) != 0 {
		t.Errorf("followed %d moves that were already searched: %s", len(f), f[0].Move.Key())
	}
	seen := map[string]int{}
	for _, f := range v.facts {
		seen[f]++
	}
	for f, n := range seen {
		if n > 1 {
			t.Errorf("a move was checked %d times: %.80q", n, f)
		}
	}
}

func TestFollowAdviceUsesTheMapperOnlyWhenTextNamesNoGroup(t *testing.T) {
	m := &fakeMapper{ids: []string{"pcba", "switch_cord_minor_parts", "not_a_group"}}
	res, _, _ := followPlan(t, Options{NearMiss: adviceSays{"Do what the earlier importer did."}, FollowAdvice: true, Mapper: m})
	if m.calls == 0 || len(followed(res)) != 1 {
		t.Fatalf("mapper calls %d, followed %d", m.calls, len(followed(res)))
	}
	if res.Stats.NearMissCalls < 2+1 {
		t.Errorf("the mapping call should be counted with the advice calls: %+v", res.Stats)
	}
	m2 := &fakeMapper{ids: []string{"pcba"}}
	followPlan(t, Options{NearMiss: adviceSays{twoGroups}, FollowAdvice: true, Mapper: m2})
	if m2.calls != 0 {
		t.Errorf("mapper called %d times though the text names a group", m2.calls)
	}
}

func TestFollowAdviceOffByDefaultAndVerdictsUntouched(t *testing.T) {
	adv := adviceSays{twoGroups}
	off, _, _ := followPlan(t, Options{NearMiss: adv})
	on, _, _ := followPlan(t, Options{NearMiss: adv, FollowAdvice: true})
	noAdvice, _, _ := followPlan(t, Options{FollowAdvice: true}) // needs near-miss advice
	if len(followed(off)) != 0 || off.Stats.AdviceFollowed != 0 || len(followed(noAdvice)) != 0 {
		t.Fatal("followed advice while off")
	}
	a, b := verdicts(off), verdicts(on)
	for k, v := range a {
		if b[k] != v {
			t.Errorf("%s: %q became %q", k, v, b[k])
		}
	}
	if len(b) != len(a)+1 {
		t.Errorf("verdicts %d vs %d: the followed move should be the only addition", len(a), len(b))
	}
}

func TestFollowAdviceAtMostOnePerSearch(t *testing.T) {
	// Five near misses advised, each naming groups that give a new move:
	// still only one is followed.
	r, _ := fixture(t, "heatgun_2022.json")
	tmpls, _ := LoadTemplates()
	res, err := New(tmpls, allThaiEngine{}, confByMove{}, Options{PlanConfidence: 0.92, Concurrency: 2, NearMissMax: 5,
		NearMiss: adviceSays{twoGroups + " Also the motor and fan."}, FollowAdvice: true}).Plan(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(followed(res)); n != 1 {
		t.Errorf("followed %d moves, want 1", n)
	}
}

// failFollowed fails the precedent check on the three-group move only.
type failFollowed struct{}

func (failFollowed) Verify(ctx context.Context, t verifier.Task) (verifier.Result, error) {
	if strings.Contains(t.FactsB, "In Thailand, the electric motor") && strings.Contains(t.FactsB, "In Thailand, the printed circuit") &&
		strings.Contains(t.FactsB, "In Thailand, the switch") {
		return verifier.Result{}, errors.New("HTTP 504")
	}
	return confByMove{}.Verify(ctx, t)
}

func TestFollowedMoveWithAFailedCheckIsReportedLikeAnyOther(t *testing.T) {
	r, _ := fixture(t, "heatgun_2022.json")
	tmpls, _ := LoadTemplates()
	res, err := New(tmpls, allThaiEngine{}, failFollowed{}, Options{PlanConfidence: 0.92, Concurrency: 2,
		NearMiss: adviceSays{twoGroups}, FollowAdvice: true}).Plan(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	f := followed(res)
	if len(f) != 1 || f[0].RejectionKind != domain.RejectCheckFailed {
		t.Errorf("followed = %+v", f)
	}
}
