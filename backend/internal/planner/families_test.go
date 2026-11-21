package planner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// The families written from CBP precedent (D138), with the three earlier
// templates. Pinning the set makes adding or dropping a family a visible
// change.
var allFamilies = []string{
	"cables_harnesses", "display_camera_modules", "furniture", "gas_grills_cooking", "hand_tools",
	"led_lights", "lidar_modules", "medical_devices", "motorised_tools_small_appliances",
	"pcba_electronic_devices", "power_supplies_chargers", "vehicle_parts", "water_filters",
}

func TestTemplateSetIsPinned(t *testing.T) {
	tmpls, err := LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for f := range tmpls {
		got = append(got, f)
	}
	sort.Strings(got)
	if !slices.Equal(got, allFamilies) {
		t.Errorf("families = %v\nwant       %v", got, allFamilies)
	}
}

var rulingID = regexp.MustCompile(`\b[HN]\d{6}\b`)

// Every ruling a template cites is a real substantial-transformation ruling
// in the bundled corpus, outside the dev and held-out splits (the families
// were chosen and written from the rest), and is listed in the template's
// sources. Each movable group cites at least one ruling.
func TestTemplateCitationsAreRealRulings(t *testing.T) {
	labels, err := corpus.LoadLabels("../../data/labels.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]corpus.Label{}
	for _, l := range labels {
		byID[l.Ruling] = l
	}
	b, err := os.ReadFile("../../../eval/splits/splits_final.json")
	if err != nil {
		t.Fatal(err)
	}
	var splits struct{ Dev, Final, Pilot []string }
	if err := json.Unmarshal(b, &splits); err != nil {
		t.Fatal(err)
	}
	evalIDs := map[string]bool{}
	for _, id := range slices.Concat(splits.Dev, splits.Final, splits.Pilot) {
		evalIDs[id] = true
	}
	tmpls, err := LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	cited := 0
	for fam, tm := range tmpls {
		if len(tm.Sources) == 0 {
			continue // the three earlier templates cite rulings in their note only
		}
		sources := map[string]bool{}
		for _, id := range tm.Sources {
			sources[id] = true
		}
		check := func(where, id string) {
			l, ok := byID[id]
			switch {
			case !ok:
				t.Errorf("%s %s: %s is not in data/labels.jsonl", fam, where, id)
			case l.Regime != "substantial_transformation":
				t.Errorf("%s %s: %s is a %s ruling", fam, where, id, l.Regime)
			case evalIDs[id]:
				t.Errorf("%s %s: %s is in the dev or held-out split", fam, where, id)
			case !sources[id]:
				t.Errorf("%s %s: %s is not in sources", fam, where, id)
			}
		}
		used := map[string]bool{}
		last := tm.Groups[len(tm.Groups)-1]
		if last.ID != "final_assembly" || last.Movable {
			t.Errorf("%s: the last group must be final_assembly and not movable", fam)
		}
		for _, g := range tm.Groups {
			if g.Precedent == nil {
				t.Errorf("%s/%s: no precedent", fam, g.ID)
				continue
			}
			ids := slices.Concat(g.Precedent.ConfersOrigin, g.Precedent.NotEnough)
			if g.Movable && len(ids) == 0 {
				t.Errorf("%s/%s: a movable group cites no ruling", fam, g.ID)
			}
			for _, id := range ids {
				check(g.ID, id)
				used[id] = true
				cited++
			}
			for _, id := range rulingID.FindAllString(g.Precedent.Note, -1) {
				if !slices.Contains(ids, id) {
					t.Errorf("%s/%s: note cites %s, which the group's lists do not", fam, g.ID, id)
				}
			}
		}
		for id := range sources {
			if !used[id] {
				t.Errorf("%s: source %s is cited by no group", fam, id)
			}
		}
	}
	if cited == 0 {
		t.Fatal("no template cites a ruling")
	}
}

// anyTargetEngine says the target country whenever the rendered facts
// describe in-house work there, and the current origin otherwise. It only
// exercises the plan pipeline; it knows nothing about origin.
type anyTargetEngine struct {
	target, current string
	mu              sync.Mutex
	calls           int
}

func (e *anyTargetEngine) Predict(_ context.Context, c domain.Case) (engine.Result, error) {
	e.mu.Lock()
	e.calls++
	e.mu.Unlock()
	origin := e.current
	if strings.Contains(c.Facts, "In "+e.target+", the ") {
		origin = e.target
	}
	return engine.Result{Prediction: domain.Prediction{Origin: origin, Confidence: 0.97, CitedRulings: []string{"N340000"}},
		Usage: llm.Usage{InputTokens: 100, OutputTokens: 10}}, nil
}

// Each family fixture (testdata/families) loads, uses only its template's
// groups, enumerates moves of movable groups to the target, renders facts
// that name every component, and runs through the plan search in fake mode
// to a precedent-supported plan.
func TestFamilyFixturesPlan(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "testdata", "families", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	tmpls, err := LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	covered := map[string]bool{}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			r, err := LoadRequest(path)
			if err != nil {
				t.Fatal(err)
			}
			tm, ok := tmpls[r.Family]
			if !ok {
				t.Fatalf("no template %q", r.Family)
			}
			covered[r.Family] = true
			for _, c := range r.Product.Components {
				if _, ok := tm.Group(c.Group); !ok {
					t.Errorf("component %q: group %q not in %s", c.Name, c.Group, r.Family)
				}
			}
			for _, o := range r.Product.Operations {
				if _, ok := tm.Group(o.Group); !ok {
					t.Errorf("operation %q: group %q not in %s", o.Name, o.Group, r.Family)
				}
			}
			moves := Enumerate(r.Product, tm, r.Target)
			if len(moves) == 0 {
				t.Fatal("no move to the target")
			}
			for _, m := range moves {
				if err := m.Validate(); err != nil {
					t.Error(err)
				}
				for _, gm := range m {
					g, _ := tm.Group(gm.Group)
					pl, _ := r.Product.PlantByID(gm.ToPlant)
					if !g.Movable || domain.NormalizeCountry(pl.Country) != domain.NormalizeCountry(r.Target) {
						t.Errorf("bad move %s", m.Key())
					}
				}
			}
			facts := Render(r.Product, tm)
			for _, c := range r.Product.Components {
				if !strings.Contains(facts, c.Name) {
					t.Errorf("rendered facts miss component %q", c.Name)
				}
			}

			current := "China"
			if r.CurrentOrigin != nil {
				current = r.CurrentOrigin.Country
			}
			e := &anyTargetEngine{target: r.Target, current: current}
			v := &fakeVerifier{supported: true, confidence: 0.97}
			res, err := New(tmpls, e, v, Options{PlanConfidence: 0.92, Concurrency: 2}).Plan(context.Background(), r)
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Plans) == 0 {
				t.Fatalf("no plan shown; rejected: %d", len(res.Rejected))
			}
			best := res.Plans[0]
			if !best.Shown || best.Verdict == nil || !best.Verdict.PrecedentSupported {
				t.Errorf("best plan = %+v", best)
			}
			if res.Templates[r.Family] != tm.Version {
				t.Errorf("template versions = %v", res.Templates)
			}
			for _, tk := range v.tasks {
				if !slices.Equal(tk.Exclude, r.Exclude) || tk.Target != r.Target {
					t.Errorf("verifier task = %+v", tk)
				}
			}
			if e.calls < 1+len(moves) {
				t.Errorf("engine calls = %d, want at least %d (current setup and every single and pair move)", e.calls, 1+len(moves))
			}
		})
	}
	for _, fam := range allFamilies {
		if !covered[fam] && !slices.Contains(legacyFamilies, fam) {
			t.Errorf("family %s has no fixture in testdata/families", fam)
		}
	}
}

// legacyFamilies have their own fixtures in testdata (heat gun, LiDAR, tub
// light).
var legacyFamilies = []string{"led_lights", "lidar_modules", "motorised_tools_small_appliances"}
