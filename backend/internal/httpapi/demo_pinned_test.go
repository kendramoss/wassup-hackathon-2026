package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/demos"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// #412: both list endpoints must say whether a demo has a pinned case here,
// so the Cases page never has to guess from display names or start a live run
// by surprise.
func TestDemoPinnedCaseIsExposedByBothLists(t *testing.T) {
	mem := store.NewMemory()
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.Cases = mem })
	ds := mustDemos(t)
	seeded := ds[0]
	if seeded.Title == "" {
		t.Fatal("fixture has no title")
	}
	if err := mem.Put(context.Background(), store.Case{
		ID: "case_pinned_demo", Date: "2026-10-16", Pinned: true,
		DisplayName: seeded.Title, UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	code, out := hs.do(t, "GET", "/v1/demos", "")
	if code != 200 {
		t.Fatalf("list demos: %d %v", code, out)
	}
	rows, ok := out["demos"].([]any)
	if !ok {
		t.Fatalf("demos is %T", out["demos"])
	}
	var sawSeeded, sawUnseeded bool
	for _, raw := range rows {
		row, _ := raw.(map[string]any)
		if row["id"] == seeded.ID {
			sawSeeded = true
			if row["pinned_case_id"] != "case_pinned_demo" {
				t.Errorf("seeded demo has pinned_case_id %v", row["pinned_case_id"])
			}
		}
		if unseeded := findDemoByID(ds, "gas-grill-2023"); unseeded != nil && row["id"] == unseeded.ID {
			sawUnseeded = true
			if _, present := row["pinned_case_id"]; present {
				t.Errorf("unseeded demo unexpectedly has a pinned_case_id: %v", row)
			}
		}
	}
	if !sawSeeded || !sawUnseeded {
		t.Errorf("missing rows: seeded=%v unseeded=%v", sawSeeded, sawUnseeded)
	}

	code, cases := hs.do(t, "GET", "/v1/cases", "")
	if code != 200 {
		t.Fatalf("list cases: %d %v", code, cases)
	}
	for _, raw := range cases["cases"].([]any) {
		row, _ := raw.(map[string]any)
		if row["id"] == "case_pinned_demo" {
			if row["demo_id"] != seeded.ID {
				t.Errorf("pinned case has demo_id %v, want %s", row["demo_id"], seeded.ID)
			}
			return
		}
	}
	t.Error("pinned case missing from GET /v1/cases")
}

func findDemoByID(ds []demos.Demo, id string) *demos.Demo {
	for i := range ds {
		if ds[i].ID == id {
			return &ds[i]
		}
	}
	return nil
}
