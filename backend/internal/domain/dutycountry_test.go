package domain

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestDutyCountry(t *testing.T) {
	for in, want := range map[string]string{
		"DPRK": "North Korea", "Democratic People's Republic of Korea": "North Korea", "Korea, North": "North Korea",
		"North Korea": "North Korea", "NORTH KOREA ": "North Korea", "Republic of Cuba": "Cuba", "Cuba": "Cuba",
		"Byelorussia": "Belarus", "Belarus": "Belarus", "UAE": "United Arab Emirates", "United Arab Emirates": "United Arab Emirates",
		"Chinese Taipei": "Taiwan", "Taiwan, China": "Taiwan", "Taiwan": "Taiwan", "Korea, South": "South Korea",
		"South Korea": "South Korea", "Republic of Korea": "South Korea", "Viet Nam": "Vietnam", "PRC": "China",
		"Russia": "Russia", "Russian Federation": "Russian Federation", "Rossiya": "Rossiya", "": "",
		"UAE/Taiwan, China": "United Arab Emirates/Taiwan",
		"S. Korea":          "South Korea", "s korea": "South Korea", "S Korea/Taiwan, China": "South Korea/Taiwan",
	} {
		if got := DutyCountry(in); got != want {
			t.Errorf("DutyCountry(%q) = %q, want %q", in, got, want)
		}
	}
}

// NormalizeCountry is deliberately unchanged: the aliases are duty-only.
func TestNormalizeCountryUnchangedForDutyAliases(t *testing.T) {
	for in, want := range map[string]string{"DPRK": "Dprk", "UAE": "Uae", "Korea, South": "Korea South", "Chinese Taipei": "Chinese Taipei", "S. Korea": "S Korea"} {
		if got := NormalizeCountry(in); got != want {
			t.Errorf("NormalizeCountry(%q) = %q, want %q", in, got, want)
		}
	}
}

// The shared fixture is also read by the frontend (scripts/duty-rates.test.mjs)
// so the TypeScript port of DutyCountry cannot drift from this one.
func TestDutyCountrySharedFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/duty_country.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Cases [][2]string `json:"cases"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	inputs := map[string]bool{}
	for _, c := range f.Cases {
		inputs[strings.ToLower(strings.TrimSpace(c[0]))] = true
		if got := DutyCountry(c[0]); got != c[1] {
			t.Errorf("DutyCountry(%q) = %q, fixture says %q", c[0], got, c[1])
		}
	}
	// Every alias spelling must be in the fixture, so adding one here forces the
	// TypeScript port to be updated and tested.
	for k := range countryAliases {
		if !inputs[k] {
			t.Errorf("country alias %q is not in testdata/duty_country.json", k)
		}
	}
	for k := range dutyAliases {
		found := false
		for _, c := range f.Cases {
			if strings.ToLower(NormalizeCountry(c[0])) == k {
				found = true
			}
		}
		if !found {
			t.Errorf("duty alias %q has no fixture input", k)
		}
	}
}
