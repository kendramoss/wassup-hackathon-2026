package duty

import (
	"strings"
	"testing"
)

const fixtureAdditional = `{"source":"HTS Chapter 99","hts_revision":"Rev 21","checked":"2026-10-18",
"lists":[
 {"heading":"9903.88.02","name":"Section 301 China, List 2","origin":"China","rate":0.25,"subheadings":["8419.89.95","8504.40.95"]},
 {"heading":"9903.88.15","name":"Section 301 China, List 4A","origin":"China","rate":0.075,"subheadings":["9405.42.84","8467.21.00"]}],
"country":[
 {"heading":"9903.05.31","name":"Section 301 forced-labor (2026)","origin":"China","rate":0.125,"exempt":["8419.50","8467.21.00"]},
 {"heading":"9903.05.77","name":"Section 301 forced-labor (2026)","origin":"Thailand","rate":0.125,"exempt":[],
  "conditional":[{"note":"52(e)","condition":"pharmaceutical use only","hts":["3004.90.92"]}]},
 {"heading":"9903.05.49","name":"Section 301 (2026)","origin":"Japan","rate":0.125,"mode":"floor_total","exempt":[]}],
"exclusions":[
 {"heading":"9903.88.02","hts":"8504.40.95","whole_subheading":true,"description":"all","end_date":"2026-11-09"},
 {"heading":"9903.88.15","hts":"9405.42.8440","whole_subheading":false,"description":"LED lamps of a certain kind","end_date":"2026-11-09"}]}`

func TestAdditionalForOrigin(t *testing.T) {
	a, err := parseAdditional([]byte(fixtureAdditional))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		hts, origin string
		want        float64 // total extra
		names       string
		check       int
	}{
		// The heat gun: List 2 and the forced-labor duty stack: 25 + 12.5.
		{"8419.89.9585", "PRC", 0.375, "9903.05.31,9903.88.02", 0},
		// Thailand pays only the forced-labor duty.
		{"8419.89.95", "Thailand", 0.125, "9903.05.77", 0},
		// A whole-subheading exclusion removes the list duty.
		{"8504.40.95", "China", 0.125, "9903.05.31", 0},
		// A described-product exclusion is reported, not applied.
		{"9405.42.84", "China", 0.2, "9903.05.31,9903.88.15", 1},
		// Note 52 exempts this subheading from the forced-labor duty only.
		{"8467.21.00", "China", 0.075, "9903.88.15", 0},
		// A whole 6-digit heading exempted by note 52.
		{"8419.50.10", "China", 0, "", 0},
		// Japan's duty tops the total up to 12.5%: 4.2% MFN + 8.3%.
		{"8419.89.95", "Japan", 0.083, "9903.05.49", 0},
		// A use-conditional exemption is reported, not applied.
		{"3004.90.92", "Thailand", 0.125, "9903.05.77", 1},
		// Not listed, no country duty: nothing.
		{"8419.89.95", "Germany", 0, "", 0},
	}
	for _, c := range cases {
		ex, may := a.For(c.hts, c.origin, 0.042)
		var hs []string
		sum := 0.0
		for _, e := range ex {
			hs = append(hs, e.Heading)
			sum += e.Rate
		}
		if Total(0, ex) != c.want || strings.Join(hs, ",") != c.names || len(may) != c.check {
			t.Errorf("%s %s: extras %v (sum %v) check %v, want %v %q %d", c.hts, c.origin, ex, sum, may, c.want, c.names, c.check)
		}
	}
}

func TestFillWithAddsExtrasToLiveMFN(t *testing.T) {
	a, _ := parseAdditional([]byte(fixtureAdditional))
	r := 0.042
	m := MFN{HTS: "8419.89.95", General: "4.2%", Rate: &r, Supported: true}
	rates, filled, extras, _ := m.FillWith(map[string]float64{"Vietnam": 0.2}, []string{"China", "Thailand", "Vietnam"}, a)
	if rates["China"] != 0.417 || rates["Thailand"] != 0.167 || rates["Vietnam"] != 0.2 || len(filled) != 2 || len(extras["China"]) != 2 {
		t.Errorf("rates %v filled %v extras %v", rates, filled, extras)
	}
	if rates, _, extras, _ := m.FillWith(nil, []string{"China"}, nil); rates["China"] != 0.042 || extras != nil {
		t.Errorf("no adder: MFN only, got %v %v", rates, extras)
	}
}

func TestParseAdditionalRejectsBadData(t *testing.T) {
	for _, bad := range []string{
		`{"lists":[{"heading":"x","origin":"China","rate":2,"subheadings":["1"]}]}`,
		`{"lists":[{"heading":"x","origin":"China","rate":0.25,"subheadings":[]}]}`,
		`{"country":[{"heading":"x","origin":"China","rate":0}]}`,
	} {
		if _, err := parseAdditional([]byte(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}

func TestFloorTotalAddsNothingAboveTheFloor(t *testing.T) {
	a, _ := parseAdditional([]byte(fixtureAdditional))
	if ex, _ := a.For("8419.89.95", "Japan", 0.20); len(ex) != 0 {
		t.Errorf("MFN 20%% is above Japan's 12.5%% floor: %v", ex)
	}
}
