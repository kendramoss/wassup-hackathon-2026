package export

import (
	"strings"
	"testing"
)

// D180: the draft's facts drop CBP's reply boilerplate pasted with a
// ruling's facts, and keep every fact sentence as written.
func TestCleanFactsDropsCBPReplyBoilerplate(t *testing.T) {
	in := "In your letter dated October 25, 2022, you requested a country of origin ruling determination on a heat gun.\n\n" +
		"The item under consideration is the Kobalt 12.5-amp heat gun. The main housing is produced in Thailand.\n\n" +
		"Each heat gun is assembled in Thailand. Each heat gun is tested before it is labeled and packaged.\n\n" +
		"With regard to your request for the appropriate country of origin of the heat gun,"
	want := "The item under consideration is the Kobalt 12.5-amp heat gun. The main housing is produced in Thailand.\n\n" +
		"Each heat gun is assembled in Thailand. Each heat gun is tested before it is labeled and packaged."
	if got := CleanFacts(in); got != want {
		t.Errorf("CleanFacts =\n%q\nwant\n%q", got, want)
	}
}

func TestCleanFactsKeepsPlainFacts(t *testing.T) {
	for _, in := range []string{
		"Steel brackets are stamped in Vietnam from Chinese coil and zinc plated in Vietnam.",
		"You state that the motor is wound in China. The PCBA is populated in Thailand.",
		"In your letter dated 1 May, you requested a ruling.", // nothing else: kept rather than emptied
	} {
		if got := CleanFacts(in); got != strings.TrimSpace(in) {
			t.Errorf("CleanFacts(%q) = %q", in, got)
		}
	}
}

// The draft for the demo's own facts never speaks in CBP's voice.
func TestDraftHasNoCBPReplyBoilerplate(t *testing.T) {
	facts := "In your letter dated October 25, 2022, you requested a country of origin ruling determination on a heat gun.\n\nThe heat gun is assembled in Thailand.\n\nWith regard to your request for the appropriate country of origin of the heat gun,"
	in := Input{ProductName: "heat gun", Facts: facts, Requested: "Thailand"}
	for name, out := range map[string]string{"markdown": Markdown(in), "cfr177": CFR177(in)} {
		for _, bad := range []string{"In your letter", "you requested", "With regard to your request"} {
			if strings.Contains(out, bad) {
				t.Errorf("%s draft contains %q", name, bad)
			}
		}
		if !strings.Contains(out, "The heat gun is assembled in Thailand.") {
			t.Errorf("%s draft lost a fact", name)
		}
	}
}
