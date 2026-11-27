package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/tariffwatch"
)

var now = time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)

func heatGunBasis() *tariffwatch.Basis {
	mfn := 0.042
	return &tariffwatch.Basis{HTS: "8419.89.95", HTSRevision: "HTS 2026 Revision 21", PricedAt: now.AddDate(0, 0, -30), AsOf: "2026-10-28", AnnualImportUSD: 2_000_000,
		Section232: tariffwatch.Section232{Note: "not modelled"},
		Origins: []tariffwatch.OriginBasis{
			{Origin: "China", Rate: 0.417, Source: "table", MFN: &mfn, Components: []tariffwatch.Component{
				{Kind: tariffwatch.KindMFN, Name: "MFN (column 1 general)", Heading: "8419.89.95", Rate: 0.042},
				{Kind: tariffwatch.KindSection301, Name: "Section 301 China, List 2", Heading: "9903.88.02", Rate: 0.25},
				{Kind: tariffwatch.KindSection301, Name: "Section 301 forced-labor (2026)", Heading: "9903.05.31", Rate: 0.125}}},
			{Origin: "Thailand", Rate: 0.167, Source: "table", MFN: &mfn, Components: []tariffwatch.Component{
				{Kind: tariffwatch.KindMFN, Name: "MFN (column 1 general)", Heading: "8419.89.95", Rate: 0.042},
				{Kind: tariffwatch.KindSection301, Name: "Section 301 forced-labor (2026)", Heading: "9903.05.77", Rate: 0.125}}},
		}}
}

func plan() *planner.Result {
	return &planner.Result{Baseline: domain.Plan{Prediction: domain.Prediction{Origin: "China"}},
		Plans: []domain.Plan{{Prediction: domain.Prediction{Origin: "Thailand"}, DutyEffectYear: 500_000, Shown: true}}}
}

// dataset writes a data set that raises List 2 to 50%.
func dataset(t *testing.T, dir string, listRate float64) string {
	t.Helper()
	ds, err := tariffwatch.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	ds.Revision = "HTS 2026 Revision 24"
	for i := range ds.Additional.Lists {
		if ds.Additional.Lists[i].Heading == "9903.88.02" {
			ds.Additional.Lists[i].Rate = listRate
		}
	}
	b, _ := json.Marshal(ds)
	p := filepath.Join(dir, "rev24.json")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func setup(t *testing.T) (config.Config, store.Cases, string) {
	t.Helper()
	dir := t.TempDir()
	cs, err := store.NewFile(filepath.Join(dir, "cases"))
	if err != nil {
		t.Fatal(err)
	}
	put := func(c store.Case) {
		if err := cs.Put(context.Background(), c); err != nil {
			t.Fatal(err)
		}
	}
	put(store.Case{ID: "case_000000000001", Workspace: "ws_aaaaaaaaaaaa", Product: &domain.Product{Name: "Heat gun", HTS: "8419.89.95"}, Plan: plan(), DutyBasis: heatGunBasis(), CreatedAt: now, UpdatedAt: now})
	put(store.Case{ID: "case_000000000002", Workspace: "ws_aaaaaaaaaaaa", CreatedAt: now, UpdatedAt: now}) // never priced by a plan search
	put(store.Case{ID: "case_000000000003", Workspace: "ws_bbbbbbbbbbbb", Plan: plan(), DutyBasis: heatGunBasis(), CreatedAt: now, UpdatedAt: now})
	return config.Config{TariffWatch: true, CasesStore: config.CasesFile, CasesDir: filepath.Join(dir, "cases")}, cs, dir
}

type gotReport struct {
	Apply           bool   `json:"apply"`
	DatasetRevision string `json:"dataset_revision"`
	Checked         int    `json:"checked"`
	Affected        int    `json:"affected"`
	NoBasis         int    `json:"no_basis"`
	Cases           []struct {
		CaseID string             `json:"case_id"`
		Result tariffwatch.Result `json:"result"`
	} `json:"cases"`
}

func TestCheckIsADryRunUntilApplied(t *testing.T) {
	cfg, cs, dir := setup(t)
	ctx := context.Background()
	ds := dataset(t, dir, 0.5)
	var out bytes.Buffer
	if err := run(ctx, cfg, []string{"check", "-dataset", ds, "-as-of", "2026-11-27"}, &out, now); err != nil {
		t.Fatal(err)
	}
	var rep gotReport
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	if rep.Apply || rep.DatasetRevision != "HTS 2026 Revision 24" || rep.Checked != 2 || rep.Affected != 2 || rep.NoBasis != 1 || len(rep.Cases) != 2 {
		t.Fatalf("report = %+v", rep)
	}
	r := rep.Cases[0].Result
	if r.Annual == nil || r.Annual.Change != 500_000 || r.Plan == nil || r.Plan.Change != 500_000 || r.Plan.Before != 500_000 || r.Plan.After != 1_000_000 {
		t.Errorf("result = annual %+v plan %+v", r.Annual, r.Plan)
	}
	for _, id := range []string{"case_000000000001", "case_000000000003"} {
		if c, _ := cs.Get(ctx, id); c.TariffWatch != nil {
			t.Fatal("the dry run wrote a result")
		}
	}

	out.Reset()
	if err := run(ctx, cfg, []string{"check", "-dataset", ds, "-as-of", "2026-11-27", "-apply", "-workspace", "ws_aaaaaaaaaaaa"}, &out, now); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil || !rep.Apply || rep.Checked != 1 {
		t.Fatalf("applied report = %+v %v", rep, err)
	}
	cs2, _ := store.NewFile(cfg.CasesDir)
	c, _ := cs2.Get(ctx, "case_000000000001")
	if c.TariffWatch == nil || !c.TariffWatch.Affected || c.TariffWatch.Annual.Change != 500_000 || !c.UpdatedAt.Equal(now) {
		t.Errorf("stored result = %+v (updated %v)", c.TariffWatch, c.UpdatedAt)
	}
	if other, _ := cs2.Get(ctx, "case_000000000003"); other.TariffWatch != nil {
		t.Error("another workspace's case was written")
	}

	// A later check against the same revision, with nothing changed, clears it.
	same := dataset(t, dir, 0.25)
	out.Reset()
	if err := run(ctx, cfg, []string{"check", "-dataset", same, "-as-of", "2026-11-27", "-apply"}, &out, now); err != nil {
		t.Fatal(err)
	}
	c, _ = cs2.Get(ctx, "case_000000000001")
	if c.TariffWatch == nil || c.TariffWatch.Affected || len(c.TariffWatch.Changes) != 0 {
		t.Errorf("unchanged data left a change: %+v", c.TariffWatch)
	}
}

func TestCheckNeedsTheFlagAFileStoreAndADataSet(t *testing.T) {
	cfg, _, dir := setup(t)
	ds := dataset(t, dir, 0.5)
	var out bytes.Buffer
	off := cfg
	off.TariffWatch = false
	if err := run(context.Background(), off, []string{"check", "-dataset", ds}, &out, now); err == nil || !strings.Contains(err.Error(), "TARIFF_WATCH") {
		t.Errorf("flag off: %v", err)
	}
	mem := cfg
	mem.CasesStore = config.CasesMemory
	if err := run(context.Background(), mem, []string{"check", "-dataset", ds}, &out, now); err == nil {
		t.Error("an in-memory store was accepted")
	}
	if err := run(context.Background(), cfg, []string{"check"}, &out, now); err == nil || !strings.Contains(err.Error(), "-dataset") {
		t.Errorf("no data set: %v", err)
	}
	if err := run(context.Background(), cfg, []string{"check", "-dataset", ds, "-as-of", "27 Nov"}, &out, now); err == nil {
		t.Error("a bad -as-of was accepted")
	}
	if err := run(context.Background(), cfg, []string{"nope"}, &out, now); err == nil {
		t.Error("an unknown command was accepted")
	}
}

func TestDatasetCommandBuildsTheNewRevisionFromChapter99Text(t *testing.T) {
	td := filepath.Join("..", "..", "internal", "tariffwatch", "testdata")
	var out bytes.Buffer
	err := run(context.Background(), config.Config{}, []string{"dataset", "-old", filepath.Join(td, "ch99_rev22_excerpt.txt"), "-new", filepath.Join(td, "ch99_rev23_excerpt.txt")}, &out, now)
	if err != nil {
		t.Fatal(err)
	}
	ds, err := tariffwatch.Load(&out)
	if err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	if ds.Revision != "HTS 2026 Revision 23" || len(ds.Actions) != 2 || ds.Additional.Exclusions[0].EndDate != "2027-01-09" {
		t.Errorf("revision %q, %d actions, first exclusion ends %s", ds.Revision, len(ds.Actions), ds.Additional.Exclusions[0].EndDate)
	}
}
