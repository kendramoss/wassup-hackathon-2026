package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/duty"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffwatch"
)

const twEconomics = `{"illustrative":true,"annual_import_value_usd":2000000}`

func planWithEconomics(t *testing.T, hs harness, caseID string) {
	t.Helper()
	if code, out := hs.do(t, "POST", "/v1/cases/"+caseID+"/plan", `{"target":"Thailand","economics":`+twEconomics+`}`); code != http.StatusOK {
		t.Fatalf("plan = %d %v", code, out)
	}
}

func storedCase(t *testing.T, cs store.Cases, id string) store.Case {
	t.Helper()
	c, err := cs.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestTariffWatchSnapshotsTheDutyBasisOnATablePricedPlan(t *testing.T) {
	cases := store.NewMemory()
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.Cases, d.TariffWatch = cases, true })
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	planWithEconomics(t, hs, id)
	b := storedCase(t, cases, id).DutyBasis
	if b == nil {
		t.Fatal("no duty basis stored on the case")
	}
	if b.HTS != "8419.89.95" || b.HTSRevision != "HTS 2026 Revision 21" || b.AnnualImportUSD != 2_000_000 || b.Section232.Modelled || b.Section232.Note == "" {
		t.Errorf("basis = %+v", b)
	}
	china, ok := b.Origin("China")
	if !ok || china.Source != "table" || china.Rate != 0.417 {
		t.Fatalf("china = %+v", china)
	}
	var kinds []string
	for _, k := range china.Components {
		kinds = append(kinds, k.Kind+":"+k.Heading)
	}
	if strings.Join(kinds, " ") != "mfn:8419.89.95 section_301:9903.05.31 section_301:9903.88.02" {
		t.Errorf("components = %v", kinds)
	}
	if china.MFN == nil || *china.MFN != 0.042 {
		t.Errorf("mfn = %v", china.MFN)
	}
}

func TestTariffWatchSnapshotsALivePricedPlanWithItsExclusions(t *testing.T) {
	cases := store.NewMemory()
	add, err := duty.LoadAdditional()
	if err != nil {
		t.Fatal(err)
	}
	clk, _ := fixedClock("2026-10-20")
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.Cases, d.TariffWatch, d.LiveMFN, d.DutyAdd, d.Now = cases, true, expiryHTS(t), add, clk
	})
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunWithHTS(t, "8483.50.90"))
	id := c["id"].(string)
	planWithEconomics(t, hs, id)
	b := storedCase(t, cases, id).DutyBasis
	if b == nil || b.AsOf != "2026-10-20" || b.PricedAt.IsZero() {
		t.Fatalf("basis = %+v", b)
	}
	china, ok := b.Origin("China")
	if !ok || china.Source != "live_mfn" || china.Rate != 0.4 {
		t.Fatalf("china = %+v", china)
	}
	if china.MFN == nil || *china.MFN != 0.025 {
		t.Errorf("mfn = %v", china.MFN)
	}
	headings := map[string]float64{}
	for _, k := range china.Components {
		headings[k.Heading] = k.Rate
	}
	if headings["9903.88.01"] != 0.25 || headings["8483.50.90"] != 0.025 || len(headings) != 3 {
		t.Errorf("components = %v", headings)
	}
	var ex []tariffwatch.Exclusion
	for _, e := range china.Exclusions {
		if e.Heading == "9903.88.01" { // the other entry is a conditional exemption of the country-wide duty, with no end date
			ex = append(ex, e)
		}
	}
	if len(ex) != 1 || ex[0].HTS != "8483.50.9040" || ex[0].EndDate != "2026-11-09" || ex[0].Status != "" {
		t.Errorf("exclusions = %+v", china.Exclusions)
	}
}

func TestTariffWatchOffLeavesTheCaseAsItWas(t *testing.T) {
	cases := store.NewMemory()
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.Cases = cases })
	_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
	id := c["id"].(string)
	planWithEconomics(t, hs, id)
	if got := storedCase(t, cases, id); got.DutyBasis != nil || got.TariffWatch != nil {
		t.Errorf("a basis was stored with the flag off: %+v", got.DutyBasis)
	}
	_, out := hs.do(t, "GET", "/v1/cases/"+id, "")
	for _, k := range []string{"duty_basis", "tariff_watch"} {
		if _, ok := out[k]; ok {
			t.Errorf("%s is in the case response", k)
		}
	}
}

func TestTariffWatchRouteListsAffectedCasesForMembers(t *testing.T) {
	hs, _, cases := teamsHarness(t, func(d *Deps) { d.TariffWatch = true })
	code, ws := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Acme"}`, user("alice"))
	if code != http.StatusCreated {
		t.Fatalf("workspace = %d", code)
	}
	wsID := ws["id"].(string)
	hs.addToWorkspace(t, wsID, "alice", "bob")
	hit, quiet := hs.newCase(t, user("bob")), hs.newCase(t, user("alice"))
	hs.newCase(t, nil) // anonymous: not the workspace's
	path := "/v1/workspaces/" + wsID + "/tariff-watch"

	// Nothing has been checked yet.
	code, out := hs.doAs(t, "GET", path, "", user("alice"))
	if code != http.StatusOK || len(out["cases"].([]any)) != 0 || out["checked"] != 0.0 || out["cases_in_workspace"] != 2.0 {
		t.Fatalf("before any check = %d %v", code, out)
	}

	now := time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)
	set := func(id string, r tariffwatch.Result) {
		t.Helper()
		if _, err := cases.Update(context.Background(), id, func(c *store.Case) error { c.TariffWatch = &r; return nil }); err != nil {
			t.Fatal(err)
		}
	}
	old, up := 0.25, 0.5
	set(hit, tariffwatch.Result{CheckedAt: now, DatasetRevision: "HTS 2026 Revision 24", BasisRevision: "HTS 2026 Revision 21", AsOf: "2026-11-27", Affected: true,
		Changes: []tariffwatch.Change{{Origin: "China", Kind: tariffwatch.ChangeRate, Heading: "9903.88.02", OldRate: &old, NewRate: &up, MovesRate: true}},
		Annual:  &tariffwatch.AnnualDuty{Origin: "China", Before: 834000, After: 1334000, Change: 500000},
		Plan:    &tariffwatch.PlanDuty{From: "China", To: "Thailand", Before: 500000, After: 1000000, Change: 500000}})
	set(quiet, tariffwatch.Result{CheckedAt: now, DatasetRevision: "HTS 2026 Revision 24", BasisRevision: "HTS 2026 Revision 21", AsOf: "2026-11-27", Changes: []tariffwatch.Change{}})

	code, out = hs.doAs(t, "GET", path, "", user("bob"))
	if code != http.StatusOK || out["checked"] != 2.0 || out["affected"] != 1.0 {
		t.Fatalf("list = %d %v", code, out)
	}
	list := out["cases"].([]any)
	if len(list) != 1 {
		t.Fatalf("cases = %v", list)
	}
	row := list[0].(map[string]any)
	if row["case_id"] != hit || row["hts"] != "8419.89.95" {
		t.Errorf("row = %v", row)
	}
	res := row["result"].(map[string]any)
	if res["annual_duty"].(map[string]any)["change_usd"] != 500000.0 || res["plan_duty"].(map[string]any)["change_usd"] != 500000.0 || res["dataset_revision"] != "HTS 2026 Revision 24" {
		t.Errorf("result = %v", res)
	}
	if code, out = hs.doAs(t, "GET", path+"?all=true", "", user("bob")); code != http.StatusOK || len(out["cases"].([]any)) != 2 {
		t.Errorf("all = %d %v", code, out)
	}

	// Outsiders and anonymous callers learn nothing.
	if code, _ := hs.doAs(t, "GET", path, "", user("carol")); code != http.StatusNotFound {
		t.Errorf("outsider = %d", code)
	}
	if code, _ := hs.doAs(t, "GET", path, "", nil); code != http.StatusUnauthorized {
		t.Errorf("anonymous = %d", code)
	}
}

func TestTariffWatchRouteIsOffByDefault(t *testing.T) {
	hs, _, _ := teamsHarness(t, nil)
	_, ws := hs.doAs(t, "POST", "/v1/workspaces", `{"name":"Acme"}`, user("alice"))
	if code, _ := hs.doAs(t, "GET", "/v1/workspaces/"+ws["id"].(string)+"/tariff-watch", "", user("alice")); code != http.StatusNotFound {
		t.Errorf("flag off = %d", code)
	}
}
