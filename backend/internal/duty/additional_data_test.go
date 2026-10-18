package duty

import (
	"math"
	"testing"
)

// The embedded table must give the heat gun the rates sourced by hand in
// rates.json (D52): China 4.2 + 25 (List 2) + 12.5 = 41.7%, Thailand 16.7%.
func TestEmbeddedAdditionalMatchesTheHandSourcedHeatGun(t *testing.T) {
	a, err := LoadAdditional()
	if err != nil {
		t.Fatal(err)
	}
	tbl, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	line, ok := tbl.Line("8419.89.95")
	if !ok {
		t.Fatal("rates.json has no heat-gun line")
	}
	for _, r := range line.Rates {
		ex, _ := a.For("8419.89.95", r.Origin, 0.042)
		if got := Total(0.042, ex); math.Abs(got-r.Total) > 1e-9 {
			t.Errorf("%s: additional table gives %.4f (%v), hand-sourced %.4f", r.Origin, got, ex, r.Total)
		}
	}
}

func TestEmbeddedAdditionalSpotChecks(t *testing.T) {
	a, _ := LoadAdditional()
	cases := []struct {
		hts, origin string
		mfn, want   float64
	}{
		{"8419.89.95", "Vietnam", 0.042, 0.167}, // forced-labor only
		{"8419.89.95", "Malaysia", 0.042, 0.142},
		{"8419.89.95", "Japan", 0.042, 0.125},     // floor total 12.5%
		{"8419.89.95", "Germany", 0.042, 0.10},    // EU member: total raised to 10%
		{"8419.89.95", "Hong Kong", 0.042, 0.167}, // its own heading, not China's lists
		{"8419.89.95", "Turkiye", 0.042, 0.167},
		{"8419.89.95", "Brazil", 0.042, 0.167},
		{"8419.89.95", "Canada", 0.042, 0.142},
	}
	for _, c := range cases {
		ex, _ := a.For(c.hts, c.origin, c.mfn)
		if got := Total(c.mfn, ex); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s %s: %.4f (%v), want %.4f", c.hts, c.origin, got, ex, c.want)
		}
	}
}
