package domain

import (
	"os"
	"regexp"
	"slices"
	"testing"
)

// The Go table is the website picker's: the keys of countryCodes in
// frontend/src/lib/countries.ts, in order.
func TestCountryTableMatchesThePicker(t *testing.T) {
	b, err := os.ReadFile("../../../frontend/src/lib/countries.ts")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	start := regexp.MustCompile(`countryCodes[^{]*\{`).FindStringIndex(s)
	end := regexp.MustCompile(`\n};`).FindStringIndex(s[start[1]:])
	body := s[start[1] : start[1]+end[0]]
	var keys []string
	for _, m := range regexp.MustCompile(`(?m)^\s+(?:"([^"]+)"|([A-Za-z]+)):\s*"[a-z]{2}"`).FindAllStringSubmatch(body, -1) {
		keys = append(keys, m[1]+m[2])
	}
	if !slices.Equal(keys, CountryTable()) {
		t.Errorf("picker %v\n go %v", keys, CountryTable())
	}
}

func TestKnownCountry(t *testing.T) {
	for _, ok := range []string{"China", "south korea", "Viet Nam", "USA", "UK"} {
		if _, got := KnownCountry(ok); !got {
			t.Errorf("%q refused", ok)
		}
	}
	for _, bad := range []string{"", "Unknown", "TBD", "Mars", "European Union", "N/A", "<b>"} {
		if _, got := KnownCountry(bad); got {
			t.Errorf("%q accepted", bad)
		}
	}
}
