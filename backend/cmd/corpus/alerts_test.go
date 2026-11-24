package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// TestCommittedAlertCount ties the NUMBERS row for revocation alerts (D481,
// recounted in D490) to eval/supersession/alerts.json; make reproduce checks the file itself.
func TestCommittedAlertCount(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	b, err := os.ReadFile(filepath.Join(root, "eval", "supersession", "alerts.json"))
	if err != nil {
		t.Fatal(err)
	}
	var d struct {
		Rows  map[string]int   `json:"rows"`
		Cases map[string]int   `json:"cases"`
		Demos []map[string]any `json:"demo_fixtures"`
	}
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"rows_with_rulings": 14102, "final_after_case": 2, "final_before_case": 7, "read_only_final": 68, "rows_undated": 388}
	for k, v := range want {
		if d.Rows[k] != v {
			t.Errorf("rows %s = %d, docs/NUMBERS.md says %d", k, d.Rows[k], v)
		}
	}
	if d.Rows["proposed"] != 0 || d.Rows["final_undated"] != 0 || d.Cases["cases"] != 7 || len(d.Cases) != 1 || len(d.Demos) != 16 {
		t.Errorf("rows %v cases %v demos %d", d.Rows, d.Cases, len(d.Demos))
	}
	for _, x := range d.Demos {
		if x["category"] != "none" {
			t.Errorf("demo %v has an alert", x["demo"])
		}
	}
	nums, err := os.ReadFile(filepath.Join(root, "docs", "NUMBERS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(nums), "of 14,102 result rows naming rulings: 2 rely on a ruling changed on or after their case date") {
		t.Error("docs/NUMBERS.md row changed: update this test with it")
	}
}

func TestAlertCategoryRanksTheStrongest(t *testing.T) {
	day := func(s string) time.Time { d, _ := time.Parse(time.DateOnly, s); return d }
	today := day(alertsAsOf)
	cases := []struct {
		date string
		refs []domain.RulingRef
		want string
	}{
		{"2025-03-01", nil, "none"},
		{"2025-03-01", []domain.RulingRef{{ID: "N345891", Role: domain.RoleRead}}, "read_only_final"},
		{"2025-03-01", []domain.RulingRef{{ID: "080651", Role: domain.RoleCited}, {ID: "N345891", Role: domain.RoleRead}}, "proposed"},
		{"2025-06-01", []domain.RulingRef{{ID: "N345891", Role: domain.RoleCited}, {ID: "080651", Role: domain.RoleCited}}, "final_before_case"},
		{"2025-03-01", []domain.RulingRef{{ID: "080651", Role: domain.RoleCited}, {ID: "N345891", Role: domain.RolePlanSupport}}, "final_after_case"},
	}
	for _, tc := range cases {
		if got := alertCategory(domain.RulingAlerts(day(tc.date), today, tc.refs), true); got != tc.want {
			t.Errorf("%s %v: %s, want %s", tc.date, tc.refs, got, tc.want)
		}
	}
	if got := alertCategory(domain.RulingAlerts(time.Time{}, today, []domain.RulingRef{{ID: "N345891", Role: domain.RoleCited}}), false); got != "final_undated" {
		t.Error(got)
	}
}
