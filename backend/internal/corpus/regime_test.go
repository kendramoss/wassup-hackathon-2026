package corpus

import "testing"

func TestClassifyRegime(t *testing.T) {
	tests := []struct {
		text  string
		multi bool
		want  string
	}{
		{"applying 19 CFR 102.11(a) the origin is", true, RegimePart102},
		{"under 102.21(c)(2) the fabric", true, RegimePart102},
		{"a tariff shift occurs in Mexico", true, RegimePart102},
		{"a change in tariff classification is not dispositive; the name, character and use change, so it is substantially transformed", true, RegimeST},
		{"entered under subheading 9802.00.80 after substantial transformation", true, Regime9802},
		{"the active pharmaceutical ingredient is substantially transformed", true, RegimePharma},
		{"marked with the country of origin", true, RegimeOther},
		{"the parts are substantially transformed in Vietnam", true, RegimeST},
		{"the product is substantially transformed", false, RegimeSingle},
	}
	for _, tt := range tests {
		if got := ClassifyRegime(tt.text, tt.multi); got != tt.want {
			t.Errorf("ClassifyRegime(%q, %v) = %s, want %s", tt.text, tt.multi, got, tt.want)
		}
	}
}
