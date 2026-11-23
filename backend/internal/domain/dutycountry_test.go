package domain

import "testing"

func TestDutyCountry(t *testing.T) {
	for in, want := range map[string]string{
		"DPRK": "North Korea", "Democratic People's Republic of Korea": "North Korea", "Korea, North": "North Korea",
		"North Korea": "North Korea", "NORTH KOREA ": "North Korea", "Republic of Cuba": "Cuba", "Cuba": "Cuba",
		"Byelorussia": "Belarus", "Belarus": "Belarus", "UAE": "United Arab Emirates", "United Arab Emirates": "United Arab Emirates",
		"Chinese Taipei": "Taiwan", "Taiwan, China": "Taiwan", "Taiwan": "Taiwan", "Korea, South": "South Korea",
		"South Korea": "South Korea", "Republic of Korea": "South Korea", "Viet Nam": "Vietnam", "PRC": "China",
		"Russia": "Russia", "Russian Federation": "Russian Federation", "Rossiya": "Rossiya", "": "",
		"UAE/Taiwan, China": "United Arab Emirates/Taiwan",
	} {
		if got := DutyCountry(in); got != want {
			t.Errorf("DutyCountry(%q) = %q, want %q", in, got, want)
		}
	}
}

// NormalizeCountry is deliberately unchanged: the aliases are duty-only.
func TestNormalizeCountryUnchangedForDutyAliases(t *testing.T) {
	for in, want := range map[string]string{"DPRK": "Dprk", "UAE": "Uae", "Korea, South": "Korea South", "Chinese Taipei": "Chinese Taipei"} {
		if got := NormalizeCountry(in); got != want {
			t.Errorf("NormalizeCountry(%q) = %q, want %q", in, got, want)
		}
	}
}
