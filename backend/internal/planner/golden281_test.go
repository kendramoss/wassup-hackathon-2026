package planner

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// #281 step 3 legacy goldens. They were captured from the production
// economics(), Compare() and Plan code BEFORE the operations wiring, by
// running this test with GOLDEN281_UPDATE=1 on the base commit. Afterwards the
// same requests (no "operations") must produce the same bytes. The files are
// the output of the real functions, not a second implementation of the maths.
// Run with GOLDEN281_UPDATE=1 only to capture on a base that predates a change.

const goldenDir = "testdata/golden281"

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
		t.Errorf("legacy golden %s changed.\n got: %s\nwant: %s", name, firstDiff(got, want), "see testdata/golden281/"+name+".json")
	}
}

func firstDiff(a, b []byte) string {
	n := min(len(a), len(b))
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	lo := max(0, i-80)
	return string(a[lo:min(len(a), i+120)])
}

// goldenEconomics are legacy money inputs (no operations).
func goldenEconomics() map[string]func(*Economics) {
	return map[string]func(*Economics){
		"fixture":        func(*Economics) {},
		"no_group_costs": func(e *Economics) { e.GroupCosts = nil },
		"partial_costs": func(e *Economics) {
			e.GroupCosts = map[string]map[string]float64{"motor_fan_assembly": {"cn": 300000, "th": 420000}, "pcba": {"cn": 150000}}
		},
		"no_money": func(e *Economics) { *e = Economics{} },
		"zero_costs": func(e *Economics) {
			e.GroupCosts = map[string]map[string]float64{"motor_fan_assembly": {"cn": 0, "th": 0}, "pcba": {"cn": 0, "th": 0}}
		},
		"negative_net": func(e *Economics) {
			e.GroupCosts = map[string]map[string]float64{"motor_fan_assembly": {"cn": 100000, "th": 900000}, "pcba": {"cn": 50000, "th": 700000}, "housing_moulding": {"cn": 0, "th": 1000}}
		},
		"no_thai_rate": func(e *Economics) { e.DutyRates = map[string]float64{"China": 0.25} },
		"sub_cent": func(e *Economics) {
			e.GroupCosts = map[string]map[string]float64{"motor_fan_assembly": {"cn": 100000.004, "th": 120000.011}, "pcba": {"cn": 1.005, "th": 2.0149}}
		},
	}
}

func goldenPlanner(t *testing.T, conc int, opts Options) *Planner {
	t.Helper()
	tmpls, err := LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	opts.PlanConfidence, opts.Concurrency = 0.85, conc
	return New(tmpls, allThaiEngine{}, confByMove{}, opts)
}

func TestLegacyGoldenPlanJSON(t *testing.T) {
	for name, mut := range goldenEconomics() {
		t.Run(name, func(t *testing.T) {
			r, _ := fixture(t, "heatgun_2022.json")
			mut(&r.Economics)
			res, err := goldenPlanner(t, 3, Options{MaxMoves: DefaultMaxMoves}).Plan(context.Background(), r)
			if err != nil {
				t.Fatal(err)
			}
			res.Stats.WallSeconds = 0
			goldenCheck(t, "plan_"+name, res)
		})
	}
}

// Events: moves run in goroutines, so their order is not fixed even at
// concurrency 1. The golden records what each event carries by move key (the
// plan inside move_done, the started and done bodies) and the order rules are
// asserted separately: started first, a move's started before its done, the
// done counter counting up, done last.
func TestLegacyGoldenPlanEvents(t *testing.T) {
	for _, name := range []string{"fixture", "negative_net", "partial_costs"} {
		t.Run(name, func(t *testing.T) {
			r, _ := fixture(t, "heatgun_2022.json")
			goldenEconomics()[name](&r.Economics)
			var seq []Event
			p := goldenPlanner(t, 1, Options{MaxMoves: DefaultMaxMoves, NearMiss: adviceSays{twoGroups}, FollowAdvice: true})
			if _, err := p.PlanWithProgress(context.Background(), r, func(e Event) { seq = append(seq, e) }); err != nil {
				t.Fatal(err)
			}
			rec := map[string]any{}
			done := map[string]*domain.Plan{}
			started := map[string]bool{}
			counter := 0
			for i, e := range seq {
				switch e.Kind {
				case EventStarted:
					if i != 0 {
						t.Errorf("started is event %d, want 0", i)
					}
					rec["started"] = e
				case EventBaseline:
					rec["baseline"] = e
				case EventMoveStarted:
					started[e.Move] = true
				case EventMoveDone:
					if !started[e.Move] {
						t.Errorf("move_done %s before its move_started", e.Move)
					}
					counter++
					if e.Done != counter {
						t.Errorf("move_done %s counts %d, want %d", e.Move, e.Done, counter)
					}
					done[e.Move] = e.Plan
				case EventDone:
					if i != len(seq)-1 {
						t.Errorf("done is event %d of %d", i, len(seq))
					}
					e.Result.Stats.WallSeconds = 0
					rec["done"] = e
				}
			}
			rec["move_done"] = done
			goldenCheck(t, "events_"+name, rec)
		})
	}
}

func TestLegacyGoldenFollowedAdvice(t *testing.T) {
	r, _ := fixture(t, "heatgun_2022.json")
	res, err := goldenPlanner(t, 3, Options{NearMiss: adviceSays{twoGroups}, FollowAdvice: true}).Plan(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	res.Stats.WallSeconds = 0
	goldenCheck(t, "plan_followed_advice", res)
}

func TestLegacyGoldenCompare(t *testing.T) {
	r, tm := fixture(t, "heatgun_2022.json")
	moves := Enumerate(r.Product, tm, "Thailand")
	if len(moves) == 0 {
		t.Fatal("no moves")
	}
	out := map[string]any{}
	for name, mut := range goldenEconomics() {
		e := r.Economics
		mut(&e)
		rows := map[string]Comparison{}
		for _, m := range moves {
			v := Apply(r.Product, m)
			rows[m.Key()] = Compare(e, &r.Product, &v, "China", "Thailand")
		}
		pair := domain.Move{{Group: "motor_fan_assembly", ToPlant: "th"}, {Group: "pcba", ToPlant: "th"}}
		pv := Apply(r.Product, pair)
		rows["pair"] = Compare(e, &r.Product, &pv, "China", "Thailand")
		same := r.Product
		rows["no_move"] = Compare(e, &r.Product, &same, "China", "Thailand")
		rows["no_variant"] = Compare(e, &r.Product, nil, "China", "Thailand")
		rows["no_products"] = Compare(e, nil, nil, "China", "Thailand")
		out[name] = rows
	}
	goldenCheck(t, "compare_all", out)
}
