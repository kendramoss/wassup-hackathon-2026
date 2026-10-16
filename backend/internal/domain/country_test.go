package domain

import "testing"

// Expected values were produced by running norm() from predict.py on the same
// inputs, except "Türkiye": norm() strips the "ü" and returns "Trkiye", which
// never matches a label. We map it to "Turkey" (see docs/DECISIONS.md).
func TestNormalizeCountry(t *testing.T) {
	tests := []struct{ in, want string }{
		{"USA", "United States"},
		{"U.S.", "United States"},
		{"the Netherlands", "Netherlands"},
		{"Viet Nam", "Vietnam"},
		{"PRC", "China"},
		{"people's republic of china", "China"},
		{"Republic of Korea", "South Korea"},
		{"Taiwan (ROC)", "Taiwan"},
		{"Hong Kong SAR", "Hong Kong"},
		{"Isle of Man", "Isle of Man"},
		{"Bosnia and Herzegovina", "Bosnia And Herzegovina"},
		{"  thailand ", "Thailand"},
		{"Côte d'Ivoire", "Cte D'Ivoire"},
		{"Trinidad-and-Tobago", "Trinidadandtobago"},
		{"the Gambia", "Gambia"},
		{"Korea, Republic of", "Korea Republic of"},
		{"Mexico2", "Mexico"},
		{"", ""},
		{"Türkiye", "Turkey"},
		{"Indonesia/Malaysia", "Indonesia/Malaysia"},
		{"Indonesia or Malaysia", "Indonesia/Malaysia"},
		{"PRC / Viet Nam", "China/Vietnam"},
	}
	for _, tt := range tests {
		if got := NormalizeCountry(tt.in); got != tt.want {
			t.Errorf("NormalizeCountry(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
