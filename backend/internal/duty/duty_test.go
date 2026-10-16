package duty

import (
	"strings"
	"testing"
)

func TestTableMatchesDutySources(t *testing.T) {
	tb, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	l, ok := tb.Line("8419.89.9585") // a statistical number matches its subheading
	if !ok {
		t.Fatal("8419.89.95 missing")
	}
	want := map[string]float64{"China": 0.417, "Thailand": 0.167, "Vietnam": 0.167, "Malaysia": 0.142} // docs/DUTY-SOURCES.md
	for _, r := range l.Rates {
		if want[r.Origin] != r.Total {
			t.Errorf("%s total %v, want %v", r.Origin, r.Total, want[r.Origin])
		}
		if r.Unverified {
			t.Errorf("%s has an unverified component; every heat-gun component was confirmed from a primary source on 17 Oct", r.Origin)
		}
		for _, c := range r.Components {
			if c.Heading == "" || !strings.HasPrefix(c.Source, "https://") {
				t.Errorf("%s %s: no heading or source", r.Origin, c.Name)
			}
		}
	}
	if len(l.Rates) != len(want) {
		t.Errorf("%d origins", len(l.Rates))
	}
	lidar, ok := tb.Line("9015.80.2000")
	if !ok || lidar.Checked != "2026-10-17" {
		t.Fatalf("LiDAR line = %v %+v", ok, lidar)
	}
	wantLidar := map[string]float64{"China": 0.375, "Thailand": 0.125, "Vietnam": 0.125, "Malaysia": 0.10} // docs/DUTY-SOURCES.md, LiDAR section
	for _, r := range lidar.Rates {
		if wantLidar[r.Origin] != r.Total || r.Unverified {
			t.Errorf("LiDAR %s total %v unverified %v, want %v verified", r.Origin, r.Total, r.Unverified, wantLidar[r.Origin])
		}
	}
	if _, ok := tb.Line("9999.99.99"); ok {
		t.Error("unknown line found")
	}
}

func TestFillKeepsUserRates(t *testing.T) {
	tb, _ := Load()
	got, filled := tb.Fill(map[string]float64{"china": 0.3}, "8419.89.95")
	if got["china"] != 0.3 || got["Thailand"] != 0.167 || len(filled) != 3 {
		t.Errorf("fill = %v %v", got, filled)
	}
	if _, filled := tb.Fill(nil, "1234.56.78"); filled != nil {
		t.Error("filled from an unknown line")
	}
	if n := tb.Note("8419.89.95", []string{"Vietnam"}); strings.Contains(n, "Not fully confirmed") || !strings.Contains(n, "DUTY-SOURCES") {
		t.Errorf("note = %q", n)
	}
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{"8419.89.95": "8419.89.95", "8419899585": "8419.89.95", " 8419 89 95 ": "8419.89.95", "8419": "8419"} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q", in, got)
		}
	}
}
