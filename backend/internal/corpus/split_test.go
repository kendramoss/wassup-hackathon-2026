package corpus

import (
	"strings"
	"testing"
)

func TestSplitFacts(t *testing.T) {
	text := "U.S. Customs\n\nRE: The country of origin of a heat gun\n\nDear Mr. Smith:\n\n" +
		"This is in response to your request of January 14, 2026, for a ruling under Section 304 of the Tariff Act. " +
		"The heat gun is assembled in Thailand from a Chinese motor. The housing is moulded in Thailand. " +
		"You believe that the origin of the heat gun is Thailand. " +
		"The   housing is grey.\n\n" +
		"When determining the country of origin of a product, CBP looks at substantial transformation. " +
		"Therefore, the country of origin of the subject heat gun will be Thailand."
	facts, meta := SplitFacts(text)
	// Expected output produced by split()+redact() in split_facts.py.
	want := "This is in response to your request of January 14, 2026, for a ruling under Section 304 of the Tariff Act. " +
		"The heat gun is assembled in Thailand from a Chinese motor. The housing is moulded in Thailand. " +
		"[REDACTED: origin-conclusion sentence]  The housing is grey."
	if facts != want {
		t.Errorf("facts =\n%q\nwant\n%q", facts, want)
	}

	if !strings.HasPrefix(facts, "This is in response") {
		t.Errorf("facts should start after the salutation: %q", facts[:40])
	}
	// The opening sentence mentions Section 304 (a cue) but is skipped.
	if !strings.Contains(facts, "Chinese motor") || strings.Contains(facts, "When determining") || strings.Contains(facts, "will be Thailand") {
		t.Errorf("cut in the wrong place: %q", facts)
	}
	if meta.Cue != "When determining the country of origin" {
		t.Errorf("cue = %q", meta.Cue)
	}
	if meta.Redacted != 1 || !strings.Contains(facts, redaction) || strings.Contains(facts, "You believe") {
		t.Errorf("redaction failed (n=%d): %q", meta.Redacted, facts)
	}
	if !strings.Contains(facts, "The housing is grey.") {
		t.Errorf("whitespace not normalised: %q", facts)
	}
}

func TestSplitFactsNoCue(t *testing.T) {
	facts, meta := SplitFacts("FACTS: A pump is made in Vietnam.")
	if facts != "A pump is made in Vietnam." || meta.Cue != "" {
		t.Errorf("facts = %q, cue = %q", facts, meta.Cue)
	}
}

func TestTruncateIsRuneBased(t *testing.T) {
	for _, tt := range []struct {
		in   string
		n    int
		want string
	}{{"Türkiye", 2, "Tü"}, {"abc", 10, "abc"}, {"abc", 0, ""}, {"§§§", 1, "§"}} {
		if got := Truncate(tt.in, tt.n); got != tt.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tt.in, tt.n, got, tt.want)
		}
	}
}
