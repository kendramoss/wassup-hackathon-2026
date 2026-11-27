package tariffwatch

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/duty"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func headings(t *testing.T, name string) map[string]Heading {
	t.Helper()
	m := map[string]Heading{}
	for _, h := range ParseHeadings(fixture(t, name)) {
		m[h.Number] = h
	}
	return m
}

func TestParseHeadingsReadsDatesRatesAndLines(t *testing.T) {
	h := headings(t, "ch99_rev23_excerpt.txt")
	if got := h["9903.88.69"]; !got.IsExclusion() || got.EndDate() != "2027-01-09" || got.StartDate() != "2024-06-15" || !got.OfChina() {
		t.Errorf("9903.88.69 = %+v end %q start %q", got, got.EndDate(), got.StartDate())
	}
	if got := h["9903.88.70"]; !got.IsExclusion() || got.EndDate() != "2027-01-09" || got.StartDate() != "2024-01-01" {
		t.Errorf("9903.88.70 end %q start %q", got.EndDate(), got.StartDate())
	}
	chassis := h["9903.91.12"]
	rate, ok := chassis.AddRate()
	if !ok || rate != 1 || chassis.StartDate() != "2027-01-10" || strings.Join(chassis.Subheadings(), ",") != "8716.39.00,8716.90.30,8716.90.50" {
		t.Errorf("9903.91.12 rate %v %v start %q lines %v", rate, ok, chassis.StartDate(), chassis.Subheadings())
	}
	if got := h["9903.91.14"].Subheadings(); strings.Join(got, ",") != "8426.19.00" {
		t.Errorf("9903.91.14 lines = %v", got)
	}
	for _, n := range []string{"9903.91.13", "9903.91.15", "9903.91.16"} {
		if _, ok := h[n].AddRate(); ok {
			t.Errorf("%s is a carve-out: it has no additive rate", n)
		}
	}
	// Dates in the running page headers and the dot leaders do not leak in.
	if strings.Contains(h["9903.88.69"].Description, "...") || strings.Contains(h["9903.91.12"].Description, "99 - III") {
		t.Errorf("noise in %q", h["9903.91.12"].Description)
	}
	if got := RevisionName(fixture(t, "ch99_rev23_excerpt.txt")); got != "HTS 2026 Revision 23" {
		t.Errorf("revision = %q", got)
	}
	// "before" is exclusive.
	if got := (Heading{Description: "on or after January 1, 2021, and before December 1, 2021, articles"}).EndDate(); got != "2021-11-30" {
		t.Errorf("before = %q", got)
	}
}

func embedded(t *testing.T) *Dataset {
	t.Helper()
	d, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestReviseRev21To22ChangesNothingTheTableModels(t *testing.T) {
	// The Rev 22 change record lists only U.S. note 51(b)(3): nothing in the
	// headings the duty table models.
	d, rep, err := Revise(embedded(t), fixture(t, "ch99_rev21_excerpt.txt"), fixture(t, "ch99_rev22_excerpt.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Applied) != 0 || len(d.Actions) != 0 || d.Revision != "HTS 2026 Revision 22" {
		t.Errorf("report %+v actions %d revision %q", rep, len(d.Actions), d.Revision)
	}
}

func TestReviseRev22To23ExtendsExclusionsAndAddsTheMaritimeAction(t *testing.T) {
	base := embedded(t)
	d, rep, err := Revise(base, fixture(t, "ch99_rev22_excerpt.txt"), fixture(t, "ch99_rev23_excerpt.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if d.Revision != "HTS 2026 Revision 23" {
		t.Errorf("revision = %q", d.Revision)
	}
	moved := 0
	for _, e := range d.Additional.Exclusions {
		if e.EndDate == "2027-01-09" {
			moved++
		} else {
			t.Errorf("exclusion %s %s ends %s", e.Heading, e.HTS, e.EndDate)
		}
	}
	if moved != len(base.Additional.Exclusions) || moved == 0 {
		t.Errorf("%d of %d exclusions moved", moved, len(base.Additional.Exclusions))
	}
	for _, e := range base.Additional.Exclusions {
		if e.EndDate != "2026-11-09" {
			t.Fatalf("the base data set was changed: %s", e.EndDate)
		}
	}
	if len(d.Actions) != 2 {
		t.Fatalf("actions = %+v", d.Actions)
	}
	a := d.Actions[0]
	if a.Heading != "9903.91.12" || a.Rate != 1 || a.EffectiveFrom != "2027-01-10" || a.Origin != "China" || a.Kind != KindSection301 || len(a.Subheadings) != 3 {
		t.Errorf("action = %+v", a)
	}
	if len(base.Actions) != 0 {
		t.Errorf("the base data set gained actions")
	}
	if strings.Join(rep.NotModelled, ",") != "9903.91.13,9903.91.15,9903.91.16" {
		t.Errorf("not modelled = %v", rep.NotModelled)
	}
}

func TestReviseNeedsARevisionHeader(t *testing.T) {
	if _, _, err := Revise(embedded(t), "", "9903.91.12\n"); err == nil {
		t.Error("no revision header was accepted")
	}
}

func TestReviseRefusesAmbiguousExclusionMoves(t *testing.T) {
	old := "Harmonized Tariff Schedule of the United States Revision 1 (2026)\n9903.88.69\n1/\nEffective on or after June 15, 2024 and\nthrough November 9, 2026, articles the product of China, each covered by an exclusion granted by the U.S. Trade\nRepresentative...........\n1/\nThe duty\n9903.88.70\n1/\nEffective on or after January 1, 2024, and through November 9, 2026, articles the product of China, each covered by an exclusion granted by the U.S. Trade Representative...........\n1/\nThe duty\n"
	newer := strings.NewReplacer("Revision 1", "Revision 2").Replace(old)
	newer = strings.Replace(newer, "through November 9, 2026, articles the product of China, each covered by an exclusion granted by the U.S. Trade\nRepresentative", "through January 9, 2027, articles the product of China, each covered by an exclusion granted by the U.S. Trade\nRepresentative", 1)
	newer = strings.Replace(newer, "and through November 9, 2026, articles the product of China, each covered by an exclusion granted by the U.S. Trade Representative", "and through March 9, 2027, articles the product of China, each covered by an exclusion granted by the U.S. Trade Representative", 1)
	d, rep, err := Revise(embedded(t), old, newer)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Warnings) == 0 {
		t.Fatal("no warning for exclusion headings that moved the same day apart")
	}
	for _, e := range d.Additional.Exclusions {
		if e.EndDate != "2026-11-09" {
			t.Fatalf("an exclusion moved although the move was ambiguous: %s", e.EndDate)
		}
	}
}

// heat gun, priced from the researched table (D52): MFN 4.2%, Section 301
// List 2 25%, the 2026 forced-labor duty 12.5%.
func heatGunBasis() Basis {
	mfn := 0.042
	return Basis{HTS: "8419.89.95", HTSRevision: "HTS 2026 Revision 21", PricedAt: day("2026-10-20"), AsOf: "2026-10-20", AnnualImportUSD: 2_000_000,
		Origins: []OriginBasis{
			{Origin: "China", Rate: 0.417, Source: "table", MFN: &mfn, Components: []Component{
				{Kind: KindMFN, Name: "MFN (column 1 general)", Heading: "8419.89.95", Rate: 0.042},
				{Kind: KindSection301, Name: "Section 301 China, List 2", Heading: "9903.88.02", Rate: 0.25},
				{Kind: KindSection301, Name: "Section 301 forced-labor (2026)", Heading: "9903.05.31", Rate: 0.125},
			}},
			{Origin: "Thailand", Rate: 0.167, Source: "table", MFN: &mfn, Components: []Component{
				{Kind: KindMFN, Name: "MFN (column 1 general)", Heading: "8419.89.95", Rate: 0.042},
				{Kind: KindSection301, Name: "Section 301 forced-labor (2026)", Heading: "9903.05.77", Rate: 0.125},
			}},
		}}
}

func TestWatchOfTheSameDataIsQuiet(t *testing.T) {
	ev, err := NewEvaluator(embedded(t))
	if err != nil {
		t.Fatal(err)
	}
	res, err := ev.Watch(CaseInput{Basis: heatGunBasis(), BaseOrigin: "China", Plan: &PlanInput{To: "Thailand", DutyEffectYear: 500_000}}, day("2026-10-20"), day("2026-11-27"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Affected || len(res.Changes) != 0 || len(res.Rates) != 0 {
		t.Fatalf("the same revision reported a change: %+v", res.Changes)
	}
	if res.Annual == nil || res.Annual.Change != 0 || res.Plan == nil || res.Plan.Change != 0 || res.Plan.Before != 500_000 || res.Plan.After != 500_000 {
		t.Errorf("annual %+v plan %+v", res.Annual, res.Plan)
	}
}

func TestWatchWorksOutTheDutyChange(t *testing.T) {
	ds := embedded(t)
	// A later revision moves List 2 to 50% and drops the Thai forced-labor
	// duty (a country-wide duty that ends).
	for i := range ds.Additional.Lists {
		if ds.Additional.Lists[i].Heading == "9903.88.02" {
			ds.Additional.Lists[i].Rate = 0.5
		}
	}
	for i := range ds.Additional.Country {
		if ds.Additional.Country[i].Heading == "9903.05.77" {
			ds.Additional.Country[i].Rate = 0.1
		}
	}
	ev, err := NewEvaluator(ds)
	if err != nil {
		t.Fatal(err)
	}
	res, err := ev.Watch(CaseInput{Basis: heatGunBasis(), BaseOrigin: "China", Plan: &PlanInput{To: "Thailand", DutyEffectYear: 500_000}}, day("2026-10-20"), day("2026-11-27"))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Affected {
		t.Fatal("not affected")
	}
	by := map[string]Change{}
	for _, c := range res.Changes {
		by[c.Origin+"|"+c.Heading] = c
	}
	l2 := by["China|9903.88.02"]
	if l2.Kind != ChangeRate || *l2.OldRate != 0.25 || *l2.NewRate != 0.5 || !l2.MovesRate {
		t.Errorf("list 2 change = %+v", l2)
	}
	th := by["Thailand|9903.05.77"]
	if th.Kind != ChangeRate || *th.OldRate != 0.125 || *th.NewRate != 0.1 {
		t.Errorf("Thai change = %+v", th)
	}
	// China 41.7% -> 66.7% on $2M: +$500k a year.
	if res.Annual == nil || res.Annual.Before != 834_000 || res.Annual.After != 1_334_000 || res.Annual.Change != 500_000 {
		t.Errorf("annual = %+v", res.Annual)
	}
	// Plan effect: 2M x ((0.667-0.142) - (0.417-0.167)) = 2M x 0.275 = +$550k.
	if res.Plan == nil || res.Plan.Change != 550_000 || res.Plan.After != 1_050_000 {
		t.Errorf("plan = %+v", res.Plan)
	}
}

func TestWatchReportsADatedActionBeforeItBites(t *testing.T) {
	d, _, err := Revise(embedded(t), fixture(t, "ch99_rev22_excerpt.txt"), fixture(t, "ch99_rev23_excerpt.txt"))
	if err != nil {
		t.Fatal(err)
	}
	ev, err := NewEvaluator(d)
	if err != nil {
		t.Fatal(err)
	}
	base := embedded(t)
	evOld, _ := NewEvaluator(base)
	mfn := 0.057
	old, err := evOld.Evaluate("8716.90.30", "China", &mfn, day("2026-10-20"))
	if err != nil {
		t.Fatal(err)
	}
	old.Source = "live_mfn"
	snap := Basis{HTS: "8716.90.30", HTSRevision: base.Revision, PricedAt: day("2026-10-20"), AsOf: "2026-10-20", AnnualImportUSD: 1_000_000, Origins: []OriginBasis{old}}
	for _, c := range []struct {
		asOf   string
		change float64
		kinds  int
	}{{"2026-11-27", 0, 0}, {"2027-01-10", 1_000_000, 1}} {
		res, err := ev.Watch(CaseInput{Basis: snap, BaseOrigin: "China"}, day(c.asOf), day("2026-11-27"))
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Changes) != c.kinds || res.Annual.Change != c.change {
			t.Errorf("as of %s: %d changes, annual change %v (%+v)", c.asOf, len(res.Changes), res.Annual.Change, res.Changes)
		}
		if c.kinds == 1 && (res.Changes[0].Kind != ChangeAdded || res.Changes[0].Heading != "9903.91.12" || !strings.Contains(res.Changes[0].Note, "2027-01-10")) {
			t.Errorf("change = %+v", res.Changes[0])
		}
	}
}

func TestWatchReportsAnExclusionWindowWithoutMovingTheRate(t *testing.T) {
	base := embedded(t)
	var ex duty.Exclusion
	for _, e := range base.Additional.Exclusions {
		ex = e
		break
	}
	d, _, err := Revise(base, fixture(t, "ch99_rev22_excerpt.txt"), fixture(t, "ch99_rev23_excerpt.txt"))
	if err != nil {
		t.Fatal(err)
	}
	evOld, _ := NewEvaluator(base)
	evNew, _ := NewEvaluator(d)
	mfn := 0.03
	asOf := day("2026-10-20")
	old, err := evOld.Evaluate(ex.HTS, "China", &mfn, asOf)
	if err != nil {
		t.Fatal(err)
	}
	snap := Basis{HTS: ex.HTS, HTSRevision: base.Revision, PricedAt: asOf, AsOf: "2026-10-20", AnnualImportUSD: 500_000, Origins: []OriginBasis{old}}
	snap.Origins[0].Source = "live_mfn"
	res, err := evNew.Watch(CaseInput{Basis: snap, BaseOrigin: "China"}, asOf, day("2026-11-27"))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Affected || len(res.Rates) != 0 || res.Annual.Change != 0 {
		t.Fatalf("affected %v rates %v annual %+v", res.Affected, res.Rates, res.Annual)
	}
	got := 0
	for _, c := range res.Changes {
		if c.Kind == ChangeExclusionWindow {
			got++
			if c.MovesRate || c.Old != "through 2026-11-09" || c.New != "through 2027-01-09" {
				t.Errorf("window change = %+v", c)
			}
		}
	}
	if got == 0 {
		t.Errorf("no exclusion window change in %+v", res.Changes)
	}
}

func TestSection232IsFlaggedNotPriced(t *testing.T) {
	ds := embedded(t)
	ds.Actions = append(ds.Actions, Action{Kind: KindSection232, Heading: "9903.81.91", Name: "Section 232 steel", Origin: "*", Rate: 0.5, Subheadings: []string{"8419.89.95"}})
	ev, err := NewEvaluator(ds)
	if err != nil {
		t.Fatal(err)
	}
	b := heatGunBasis()
	res, err := ev.Watch(CaseInput{Basis: b, BaseOrigin: "China"}, day("2026-10-20"), day("2026-11-27"))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, c := range res.Changes {
		if c.Kind == Change232 {
			n++
			if c.MovesRate {
				t.Error("an unmodelled Section 232 coverage moved the rate")
			}
		} else {
			t.Errorf("unexpected change %+v", c)
		}
	}
	if n != 2 || res.Annual.Change != 0 {
		t.Errorf("232 changes %d, annual %+v", n, res.Annual)
	}
	// When the snapshot modelled Section 232, the same coverage is a rate.
	b.Section232 = Section232{Modelled: true}
	res, err = ev.Watch(CaseInput{Basis: b, BaseOrigin: "China"}, day("2026-10-20"), day("2026-11-27"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Annual.Change != 1_000_000 {
		t.Errorf("modelled 232 annual change = %v", res.Annual.Change)
	}
}

func TestUserRatesAreNotCompared(t *testing.T) {
	b := heatGunBasis()
	b.Origins[0] = OriginBasis{Origin: "China", Rate: 0.3, Source: "user"}
	ev, _ := NewEvaluator(embedded(t))
	res, err := ev.Watch(CaseInput{Basis: b, BaseOrigin: "China"}, day("2026-10-20"), day("2026-11-27"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Notes) == 0 || !strings.Contains(res.Notes[0], "entered by the user") {
		t.Errorf("notes = %v", res.Notes)
	}
}

func TestMFNFromHTSJSON(t *testing.T) {
	js := `[{"htsno":"8419","general":""},{"htsno":"8419.89.95","general":"4.2%"},{"htsno":"8419.89.95.85","general":"4.2%"},{"htsno":"9903.88.01","general":"x"},{"htsno":"0101.21.00","general":"Free"}]`
	got, err := MFNFromHTSJSON(strings.NewReader(js))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].HTS != "0101.21.00" || got[1].General != "4.2%" {
		t.Fatalf("lines = %+v", got)
	}
	ds := embedded(t)
	ds.MFN = []MFNLine{{HTS: "8419.89.95", General: "5%"}}
	ev, _ := NewEvaluator(ds)
	res, err := ev.Watch(CaseInput{Basis: heatGunBasis(), BaseOrigin: "China"}, day("2026-10-20"), day("2026-11-27"))
	if err != nil {
		t.Fatal(err)
	}
	var mfnChanges int
	for _, c := range res.Changes {
		if c.Kind == ChangeMFN {
			mfnChanges++
			if *c.OldRate != 0.042 || *c.NewRate != 0.05 {
				t.Errorf("mfn change = %+v", c)
			}
		}
	}
	if mfnChanges != 2 {
		t.Errorf("mfn changes = %d", mfnChanges)
	}
}

func TestValidateRefusesBadActions(t *testing.T) {
	for name, a := range map[string]Action{
		"kind":  {Kind: "x", Heading: "h", Origin: "China", Rate: 0.1, Subheadings: []string{"8716.90.30"}},
		"rate":  {Kind: KindSection301, Heading: "h", Origin: "China", Rate: 0, Subheadings: []string{"8716.90.30"}},
		"lines": {Kind: KindSection301, Heading: "h", Origin: "China", Rate: 0.1},
		"date":  {Kind: KindSection301, Heading: "h", Origin: "China", Rate: 0.1, Subheadings: []string{"8716.90.30"}, EffectiveFrom: "10 Jan 2027"},
	} {
		ds := embedded(t)
		ds.Actions = []Action{a}
		if err := ds.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := Load(strings.NewReader(`{"additional":{}}`)); err == nil {
		t.Error("a data set with no revision was accepted")
	}
}
