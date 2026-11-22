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
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
)

// The families written from CBP precedent (D138, D212), the three earlier
// templates, the six written from Part 102 rulings (D580) and the six of the
// third round (D660). Pinning the set
// makes adding or dropping a family a visible change.
var allFamilies = []string{
	"cables_harnesses", "display_camera_modules", "electrical_control_apparatus", "food_preparations", "furniture",
	"gas_grills_cooking", "golf_sporting_goods", "hand_tools", "hvac_heating_units", "led_lights", "lidar_modules",
	"machinery_parts", "medical_devices", "metal_products", "motorised_tools_small_appliances",
	"motors_pumps_compressors", "offroad_vehicles_machinery", "paper_tissue_products", "pcba_electronic_devices",
	"plastic_articles", "power_supplies_chargers", "semiconductor_devices", "toys_build_kits", "vehicle_parts",
	"water_filters", "wood_panels_flooring",
}

// round3Families are the third round (D660), chosen by counting development
// and pool rulings with no template across both regimes.
var round3Families = []string{
	"golf_sporting_goods", "hvac_heating_units", "offroad_vehicles_machinery", "paper_tissue_products",
	"semiconductor_devices", "toys_build_kits",
}

// part102Families are written from Part 102 rulings (D580).
var part102Families = []string{
	"electrical_control_apparatus", "food_preparations", "machinery_parts", "metal_products", "plastic_articles", "wood_panels_flooring",
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

// Every ruling a template cites is a real ruling in the bundled corpus of the
// template's regime, outside every development and held-out split (the
// families were chosen and written from the rest), not revoked or modified by
// a later ruling or a Customs Bulletin notice (the supersession map, D320,
// D400), and is listed in the template's sources. Each movable group cites at
// least one ruling. Substantial-transformation families cite
// substantial-transformation rulings; Part 102 families (D580) cite Part 102
// rulings from the D560 pool only: dated before 2024 and in no Part 102
// development or held-out split (#619).
func TestTemplateCitationsAreRealRulings(t *testing.T) {
	byID := loadLabels(t, "../../data/labels.jsonl")
	by102 := loadLabels(t, "../../data/labels_usmca.jsonl")
	// Substantial-transformation development and held-out rulings, and the
	// Part 102 ones. A substantial-transformation family may cite a ruling
	// that is also in the Part 102 development set (H333699, a printer ruling
	// that mentions Part 102 marking): it was chosen from the
	// substantial-transformation pool (D138). A Part 102 family may cite
	// neither.
	evalIDs := splitIDs(t, "splits_final.json", "splits_final2.json")
	// Every development, held-out, pilot and tune-check list of every regime
	// (D660): the third-round families are held to all of them, and are
	// written from rulings dated before 2026, the first held-out window.
	allEval := splitIDs(t, "splits_final.json", "splits_final2.json", "splits_tune.json", "splits_usmca.json",
		"splits_usmca_final2.json", "splits_textile.json", "splits_textile_final2.json", "splits_pharma.json",
		"splits_pharma_final2.json", "splits_router.json")
	eval102 := splitIDs(t, "splits_usmca.json", "splits_usmca_final2.json")
	if len(evalIDs) < 900 || len(eval102) < 110 {
		t.Fatalf("only %d and %d development and held-out rulings read; a split file changed shape", len(evalIDs), len(eval102))
	}
	tmpls, err := LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	var got102 []string
	cited := 0
	for fam, tm := range tmpls {
		switch tm.Regime {
		case "":
		case "part102":
			got102 = append(got102, fam)
		default:
			t.Errorf("%s: regime %q", fam, tm.Regime)
		}
		if len(tm.Sources) == 0 {
			// The three earlier templates have no sources list. The groups
			// that cite precedent (motorised tools, D330) are held to the same
			// rules, without the sources check, and never cite the heat gun's
			// own rulings: the backtest is scored against them.
			checkLegacyPrecedent(t, fam, tm, byID, evalIDs)
			continue
		}
		sources := map[string]bool{}
		for _, id := range tm.Sources {
			sources[id] = true
		}
		check := func(where, id string) {
			l, ok := byID[id]
			regime, file := "substantial_transformation", "data/labels.jsonl"
			if tm.Regime == "part102" {
				l, ok = by102[id]
				regime, file = "part102_tariff_shift", "data/labels_usmca.jsonl"
			}
			switch {
			case !ok:
				t.Errorf("%s %s: %s is not in %s", fam, where, id, file)
			case l.Regime != regime:
				t.Errorf("%s %s: %s is a %s ruling", fam, where, id, l.Regime)
			case evalIDs[id] || tm.Regime == "part102" && eval102[id]:
				t.Errorf("%s %s: %s is in a development or held-out split", fam, where, id)
			case tm.Regime == "part102" && l.Date >= "2024":
				t.Errorf("%s %s: %s is dated %s, after the D560 pool", fam, where, id, l.Date)
			case slices.Contains(round3Families, fam) && (allEval[id] || l.Date >= "2026"):
				t.Errorf("%s %s: %s is in a development, held-out or tune-check split, or dated 2026 or later (D660)", fam, where, id)
			case superseded(id):
				t.Errorf("%s %s: %s is revoked or modified (supersession map)", fam, where, id)
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
	sort.Strings(got102)
	if !slices.Equal(got102, part102Families) {
		t.Errorf("Part 102 families = %v, want %v", got102, part102Families)
	}
}

// splitIDs reads the dev, final, final2 and pilot lists of split files in
// eval/splits.
func splitIDs(t *testing.T, files ...string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, f := range files {
		b, err := os.ReadFile("../../../eval/splits/" + f)
		if err != nil {
			t.Fatal(err)
		}
		var splits struct {
			Dev, Final, Final2, Pilot []string
			TuneCheck                 []string `json:"tune_check"`
		}
		if err := json.Unmarshal(b, &splits); err != nil {
			t.Fatal(err)
		}
		for _, id := range slices.Concat(splits.Dev, splits.Final, splits.Final2, splits.Pilot, splits.TuneCheck) {
			out[id] = true
		}
	}
	return out
}

func loadLabels(t *testing.T, path string) map[string]corpus.Label {
	t.Helper()
	labels, err := corpus.LoadLabels(path)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]corpus.Label{}
	for _, l := range labels {
		out[l.Ruling] = l
	}
	return out
}

// superseded reports whether a later ruling or a final Customs Bulletin
// notice in the committed map revokes or modifies id as of today. A template
// documents precedent for any case date, so any supersession counts.
func superseded(id string) bool {
	_, ok := domain.SupersededAsOf(id, time.Now())
	return ok
}

func checkLegacyPrecedent(t *testing.T, fam string, tm Template, byID map[string]corpus.Label, evalIDs map[string]bool) {
	t.Helper()
	for _, g := range tm.Groups {
		if g.Precedent == nil {
			continue
		}
		ids := slices.Concat(g.Precedent.ConfersOrigin, g.Precedent.NotEnough)
		for _, id := range ids {
			l, ok := byID[id]
			switch {
			case !ok:
				t.Errorf("%s/%s: %s is not in data/labels.jsonl", fam, g.ID, id)
			case l.Regime != "substantial_transformation":
				t.Errorf("%s/%s: %s is a %s ruling", fam, g.ID, id, l.Regime)
			case evalIDs[id]:
				t.Errorf("%s/%s: %s is in the dev or held-out split", fam, g.ID, id)
			case superseded(id):
				t.Errorf("%s/%s: %s is revoked or modified (supersession map)", fam, g.ID, id)
			case id == "N328954" || id == "N357771":
				t.Errorf("%s/%s: %s is one of the heat gun's own rulings", fam, g.ID, id)
			}
		}
		for _, id := range rulingID.FindAllString(g.Precedent.Note, -1) {
			if !slices.Contains(ids, id) {
				t.Errorf("%s/%s: note cites %s, which the group's lists do not", fam, g.ID, id)
			}
		}
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
	// The planner evaluation's fixtures (D170, D171, D501) run the same checks.
	for _, dir := range []string{"planner_post", "planner_families", "planner_part102"} {
		more, err := filepath.Glob(filepath.Join("..", "..", "testdata", dir, "*.json"))
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, more...)
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
